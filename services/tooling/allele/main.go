// Command allele is the AST-aware git server described in
// docs/website/content/docs/architecture/allele-implementation-plan.md.
//
// This is Phase 3 of that plan's Implementation Plan: git hosting, no diffing yet. It serves real
// git smart-HTTP traffic (clone/fetch/push) against bare repositories it manages on local disk,
// alongside AlleleService's own Connect RPCs (ListRepositories/CreateRepository/GetRepository real;
// everything else CodeUnimplemented until its own later phase -- see rpc.go). Phase 1 proved the
// Chassis/Postgres/Blueprint scaffolding; Phase 2 added the proto. The tree-sitter/structural-diff
// core (Phases 4-6), worktrees/merge queue (Phase 7), provenance/permission enforcement (Phase 8),
// Catalyst events (Phase 9), Bench-backed verification (Phase 10), the linkage manifest (Phase 11),
// and the Dioxus web client (Phase 12) all land in later phases.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/repositories/postgres/bun"
)

const (
	defaultReposDir    = "./.allele-repos"
	defaultBindPort    = 9311
	defaultFoundryAddr = "http://localhost:9301"
	defaultBenchAddr   = "http://localhost:9300"
	// preReceiveHookBinaryName/postReceiveHookBinaryName are built alongside this service's own
	// binary by scripts/run-local*.sh into the same $BIN_DIR -- resolveHookBinaryPath below looks
	// for each as a sibling of os.Executable(), the same directory convention, rather than
	// requiring a second/third configured path.
	preReceiveHookBinaryName  = "allele-pre-receive-hook"
	postReceiveHookBinaryName = "allele-post-receive-hook"
)

// resolveHookBinaryPath looks for name next to this process's own executable. Returns "" (not an
// error) when it isn't there -- see store.go's installPreReceiveHook/installPostReceiveHook on what
// that means for a newly created repository.
func resolveHookBinaryPath(name string) string {
	self, err := os.Executable()
	if err != nil {
		return ""
	}
	candidate := filepath.Join(filepath.Dir(self), name)
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		return candidate
	}
	return ""
}

func main() {
	logger := chassis.NewOTelLogger()
	chassis.NewMetricsReporter().Start()

	db := bun.New("")

	// WithRepository(db) opens the Postgres connection synchronously (it's a chassis.Effect, and
	// Effect's setup runs immediately when called, not deferred to Start()) -- see
	// services/tooling/bench/main.go's identical comment for the same builder shape. db.Client()
	// is already usable by the time this call returns, so createSchema can run inline below,
	// before the mux ever starts serving requests.
	runtime := chassis.New(logger).WithRepository(db)

	ctx := context.Background()
	if err := createSchema(ctx, db); err != nil {
		logger.WithError(err).Fatal("failed to create allele schema")
	}

	cfg := chassis.GetConfig()
	reposDir := cfg.GetString("allele.repos_dir")
	if reposDir == "" {
		reposDir = defaultReposDir
	}

	bindPort := cfg.GetInt("service.network.bind_port")
	if bindPort == 0 {
		bindPort = defaultBindPort
	}
	serverAddr := fmt.Sprintf("http://localhost:%d", bindPort)

	preReceiveHookPath := resolveHookBinaryPath(preReceiveHookBinaryName)
	if preReceiveHookPath == "" {
		// Disclosed, not silent: see store.go's installPreReceiveHook on what a repository created
		// without this binary available actually means (no R4.2 enforcement for it).
		logger.Warn("allele-pre-receive-hook binary not found next to this executable -- repositories created this run will have no push-permission enforcement")
	}
	postReceiveHookPath := resolveHookBinaryPath(postReceiveHookBinaryName)
	if postReceiveHookPath == "" {
		logger.Warn("allele-post-receive-hook binary not found next to this executable -- repositories created this run will not publish SymbolChanged telemetry")
	}

	// eventsCtx lives for the whole process, cancelled via a real chassis.Effect teardown below --
	// NOT via a second goroutine reading chassis.Closer() directly, the convention
	// services/examples/crud-event/main.go and services/tooling/lineman/main.go both otherwise use.
	// That convention has a real, reproducible race: Closer() returns the single package-level,
	// buffer-1 channel chassis's own Start() reads internally (signal.Notify(closer, ...) in
	// runtime.go) to trigger its shutdown() sequence -- a second goroutine racing to read the same
	// one buffered value can win and drain it first, leaving chassis's own internal <-closer wait
	// blocked forever, so shutdown() (and the graceful-shutdown logging/effect-teardown it runs)
	// never happens at all; the process then only dies on SIGKILL, not SIGTERM. Reproduced directly
	// while verifying this phase: a manually-run instance sent SIGTERM did not exit for 7+ seconds
	// (chassis's own shutdown has a 5-second total budget) until force-killed. A chassis.Effect
	// avoids this entirely -- its teardown is called directly by shutdown()'s own Dispose loop, not
	// by racing for the same signal. This is a pre-existing, shared pattern (crud-event/lineman/
	// relay/bench all read Closer() this way too), not something introduced by Allele; fixed here
	// for Allele's own code only -- see this phase's own completion note.
	eventsCtx, cancelEvents := context.WithCancel(context.Background())
	runtime.Effect("catalyst-publisher-shutdown", func() (func(context.Context) error, error) {
		return func(context.Context) error {
			cancelEvents()
			return nil
		}, nil
	})
	events := NewCatalystPublisher(eventsCtx, logger, cfg.GetString("catalyst.address"))

	foundryAddr := cfg.GetString("foundry.address")
	if foundryAddr == "" {
		foundryAddr = defaultFoundryAddr
	}
	foundryClient := NewFoundryClient(foundryAddr)
	manifest := BuildManifest()

	// bgCtx (Phase 10) outlives any single request -- background Bench-run pollers (EnqueueMerge's
	// own goroutine, and recoverPendingBenchRuns's at startup) need a context that isn't cancelled
	// the moment the RPC that launched them returns. Same chassis.Effect-not-Closer()-race pattern
	// as eventsCtx above, for the identical reason.
	bgCtx, cancelBg := context.WithCancel(context.Background())
	runtime.Effect("bench-poller-shutdown", func() (func(context.Context) error, error) {
		return func(context.Context) error {
			cancelBg()
			return nil
		}, nil
	})

	benchAddr := cfg.GetString("allele.bench.address")
	if benchAddr == "" {
		benchAddr = defaultBenchAddr
	}
	bench := newBenchClient(benchAddr)

	st, err := newStore(db.Client(), reposDir, preReceiveHookPath, postReceiveHookPath, serverAddr, events, bench, bgCtx)
	if err != nil {
		logger.WithError(err).Fatal("failed to initialize repository store")
	}
	if err := st.recoverPendingBenchRuns(ctx); err != nil {
		// Not fatal: a failure here means an in-flight verification from before this restart stays
		// stuck until the next restart tries again, not that this process can't serve anything else.
		logger.WithError(err).Error("failed to recover in-flight bench verifications")
	}

	gitH, err := newGitHandler(st)
	if err != nil {
		logger.WithError(err).Fatal("failed to build git http-backend handler")
	}

	rpcHandler := newHandler(st)

	runtime.
		WithRPCHandler(rpcHandler).
		WithRPCHandler(gitH).
		// One route for the whole mux (RPCs, git traffic, and -- once Phase 12 lands -- the web
		// client), the same single-route-per-service pattern every other tooling service here
		// uses (Bench/Lineman/Relay). allele.draft.localhost is this service's subdomain per the
		// established convention (docs/architecture/service-ui-subdomains.md).
		WithRoute(&ntv1.Route{
			Name: "tooling-allele",
			Match: &ntv1.RouteMatch{
				Host:   "allele.draft.localhost",
				Prefix: "/",
			},
			// See blueprint/main.go's identical field: Fuse's grpc_web filter bridges browser
			// grpc-web calls into plain (HTTP/2-only) gRPC, which breaks against an
			// HTTP/1.1-only upstream cluster. Harmless for the git traffic sharing this same
			// route -- git's own smart-HTTP protocol doesn't care which HTTP version carried it.
			EnableHttp2: true,
		}).
		// Publishing to Foundry's catalog is the "ad hoc effect with an inverse" shape
		// chassis.Effect exists for -- publish on startup, retract on graceful shutdown. Allele
		// does not implement StepExecutor; this manifest exists purely for discovery/distribution,
		// the same reasoning/precedent as Lineman/Relay's own decision.
		Effect("foundry-catalog-entry", FoundryCatalogEffectSetup(logger, foundryClient, manifest)).
		Register(chassis.RegistrationOptions{
			Namespace: "tooling",
		}).
		Start()
}
