// Command bench is the workflow engine described in
// docs/website/content/docs/architecture/bench-workflow-engine.md.
//
// This is Phases 4 and 5 of the implementation plan in that doc, together (they're
// two entry points into the same long-running scheduler, see scheduler.go's
// StartRun): the webhook receiver (webhook.go) and WorkflowService's RPCs (rpc.go),
// wired up here around the Chassis/Postgres/Blueprint scaffolding Phase 1 proved and
// the DAG scheduler Phase 3 built. See model.go for the bun-mapped row structs
// backing the workflows/runs/step_results tables and their conversion functions
// to/from the api/tooling/workflow/v1 generated types, and workflows.go for loading
// workflow YAML files into Postgres at startup.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"

	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"
	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/loggers/zerolog"
	"github.com/steady-bytes/draft/pkg/repositories/postgres/bun"

	"golang.org/x/net/http2"
)

func main() {
	validatePath := flag.String("validate", "", "load and validate a workflow YAML file, report valid/invalid, and exit (no execution, no service startup)")
	runPath := flag.String("run", "", "load, validate, and execute a workflow YAML file against real running services, print the resulting Run, and exit (no service startup); requires Postgres and Blueprint reachable per config.yaml")
	flag.Parse()
	if *validatePath != "" {
		os.Exit(runValidate(*validatePath))
	}
	if *runPath != "" {
		os.Exit(runRun(*runPath))
	}

	var (
		logger = zerolog.New()
		db     = bun.New("")
	)

	// chassis.New(...).WithRepository(db) opens the Postgres connection
	// synchronously (WithRepository wraps it in a chassis.Effect, and Effect's
	// setup runs immediately when called, not deferred to Start() — see
	// pkg/chassis/effect.go), so db.Client() is already usable by the time this
	// call returns. Everything below that depends on the database (schema
	// creation, loading workflows_dir) can run inline here, before the mux ever
	// starts serving requests — unlike Phase 1, where nothing yet depended on the
	// schema existing before Start().
	runtime := chassis.New(logger).WithRepository(db)

	ctx := context.Background()
	if err := createSchema(ctx, db); err != nil {
		logger.WithError(err).Fatal("failed to create bench schema")
	}

	cfg := chassis.GetConfig()
	workflowsDir := cfg.GetString("bench.workflows_dir")
	if workflowsDir == "" {
		workflowsDir = defaultWorkflowsDir
	}

	store := NewPostgresResultStore(db)
	seeded, err := loadWorkflowsDir(ctx, workflowsDir, store, logger)
	if err != nil {
		logger.WithError(err).WithField("workflows_dir", workflowsDir).Fatal("failed to load workflows_dir")
	}
	allWorkflows, err := store.ListWorkflows(ctx)
	if err != nil {
		logger.WithError(err).Fatal("failed to list workflows after seeding")
	}
	logger.WithField("seeded_this_run", seeded).WithField("workflow_count", len(allWorkflows)).Info("workflows ready")

	registriesSeeded, err := loadPluginRegistries(ctx, cfg, store, logger)
	if err != nil {
		logger.WithError(err).Fatal("failed to load bench.plugin_registries")
	}
	allRegistries, err := store.ListPluginRegistries(ctx)
	if err != nil {
		logger.WithError(err).Fatal("failed to list plugin registries after seeding")
	}
	logger.WithField("seeded_this_run", registriesSeeded).WithField("registry_count", len(allRegistries)).Info("plugin registries ready")

	// One Scheduler, one ResultStore, one ServiceResolver, constructed once here
	// and reused across every request — not built fresh per invocation the way the
	// -run CLI flag below does for a one-shot process. A fresh blueprintResolver
	// per request would mean re-querying Blueprint's full registry (Query ignores
	// its Filter, so every call is already a full snapshot) before every single
	// run instead of caching it once, and there'd be no reason to pay that cost on
	// every webhook delivery or TriggerRun call.
	httpClient := newH2CClient()
	resolver := NewBlueprintResolver(httpClient, cfg.Entrypoint())

	// Tied to chassis.Closer(), not ctx above (which is just context.Background()
	// and never canceled here anyway) -- this context has to outlive every event
	// this publisher will ever send, across the whole life of the process, the
	// same reasoning services/examples/producer/main.go documents for its own
	// long-lived Produce stream.
	eventsCtx, cancelEvents := context.WithCancel(context.Background())
	go func() {
		<-chassis.Closer()
		cancelEvents()
	}()
	events := NewCatalystPublisher(eventsCtx, httpClient, cfg, logger)

	scheduler := NewScheduler(store, resolver, httpClient, store, events)
	secrets := newBlueprintSecretResolver(httpClient, cfg.Entrypoint())

	rpcHandler := NewHandler(logger, store, scheduler)
	webhookH := newWebhookHandler(logger, store, scheduler, secrets)
	uiHandler := NewUIHandler(logger, store, scheduler, httpClient)
	settingsHandler := NewSettingsHandler(logger, store, httpClient)

	defer runtime.
		WithRPCHandler(rpcHandler).
		WithRPCHandler(webhookH).
		WithRPCHandler(uiHandler).
		WithRPCHandler(settingsHandler).
		Register(chassis.RegistrationOptions{
			Namespace: "tooling",
		}).
		Start()
}

// newH2CClient builds the same h2c-over-plaintext client construction chassis
// itself uses to talk to Blueprint (pkg/chassis/builder.go's newBlueprintClient) —
// Draft services speak HTTP/2 without TLS internally, so a plain http.Client can't
// reach them. Shared by the Blueprint service-discovery resolver and the Blueprint
// KV secret resolver, both of which talk to Blueprint directly.
func newH2CClient() *http.Client {
	return &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLS: func(network, addr string, _ *tls.Config) (net.Conn, error) {
				return net.Dial(network, addr)
			},
		},
	}
}

// runValidate is the -validate flag's implementation: load a workflow YAML file via
// loader.go, report valid/invalid with a specific reason, and return a process exit
// code. It never starts the service or executes anything — see loader.go for
// Phase 2 (Workflow loading) of the implementation plan.
func runValidate(path string) int {
	w, err := LoadWorkflowFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid: %v\n", err)
		return 1
	}
	fmt.Printf("valid: workflow %q, %d step(s)\n", w.GetName(), len(w.GetSteps()))
	return 0
}

// runRun is the -run flag's implementation: Phase 3's manual-trigger entry point,
// kept alongside the webhook (Phase 4) and TriggerRun RPC (Phase 5) as a one-shot
// CLI alternative to standing up the whole service — it calls scheduler.Run
// (synchronous, blocks until the workflow finishes) directly rather than StartRun,
// which is exactly what a one-shot CLI invocation wants. Loads and validates path
// the same way -validate does, then actually executes it against real running
// services (via Blueprint service discovery) and persists the resulting
// Run/StepResults to Postgres, printing a summary to stdout. Requires a reachable
// Postgres and Blueprint — unlike -validate, and unlike Phase 1/2, that's expected
// and fine for this phase.
func runRun(path string) int {
	w, err := LoadWorkflowFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid: %v\n", err)
		return 1
	}

	ctx := context.Background()
	cfg := chassis.GetConfig()

	db := bun.New("")
	if err := db.Open(ctx, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to postgres: %v\n", err)
		return 1
	}
	defer db.Close(ctx)

	if err := createSchema(ctx, db); err != nil {
		fmt.Fprintf(os.Stderr, "failed to prepare bench schema: %v\n", err)
		return 1
	}

	httpClient := newH2CClient()
	resolver := NewBlueprintResolver(httpClient, cfg.Entrypoint())
	store := NewPostgresResultStore(db)
	if _, err := loadPluginRegistries(ctx, cfg, store, zerolog.New()); err != nil {
		fmt.Fprintf(os.Stderr, "failed to load bench.plugin_registries: %v\n", err)
		return 1
	}
	events := NewCatalystPublisher(ctx, httpClient, cfg, zerolog.New())
	scheduler := NewScheduler(store, resolver, httpClient, store, events)

	run, err := scheduler.Run(ctx, w)
	if err != nil {
		fmt.Fprintf(os.Stderr, "run failed to execute: %v\n", err)
		return 1
	}

	printRun(run)
	if run.GetStatus() != workflowv1.RunStatus_RUN_STATUS_PASSED {
		return 1
	}
	return 0
}

func printRun(run *workflowv1.Run) {
	fmt.Printf("run %s: workflow=%q status=%s\n", run.GetRunId(), run.GetWorkflowName(), run.GetStatus())
	for _, sr := range run.GetSteps() {
		fmt.Printf("  - %-30s %s", sr.GetStepName(), sr.GetStatus())
		if sr.GetError() != "" {
			fmt.Printf("  (%s)", sr.GetError())
		}
		fmt.Println()
	}
}
