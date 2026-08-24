// This file implements Phase 6/11: resolving a `uses: garage://<name>@<version>`
// step reference to a live plugin instance and calling its StepExecutor RPC,
// per docs/website/content/docs/architecture/garage-plugin-repository.md's
// "Publishing and discovery" section and bench-workflow-engine.md's
// "Connecting a plugin registry" (Phase 11's federated resolution).
//
// Resolution is two steps, each hitting a different service for a different
// reason:
//  1. Confirm (name, version) is actually published somewhere, by checking
//     every currently-configured registry's PluginCatalogService.Get, in
//     insertion order, using the first one that has it (see "Federated
//     search, first-match execution" in the design doc) — a registry is
//     purely descriptive (manifests: config_schema, description, ...), it
//     never tracks where a plugin is actually running (see
//     api/tooling/plugin_catalog/v1/service.proto's own doc comment:
//     "Garage does not execute plugins itself").
//  2. Find a live instance's address via Blueprint, by the plugin's own
//     process name — see discovery.go's PluginResolver for why this can't
//     reuse the RPC-service-name-keyed lookup grpc-call uses. Unaffected by
//     which registry's catalog the plugin was found in: Blueprint discovery
//     is cluster-wide, not registry-scoped.
//
// Known simplifications, both deliberate, not oversights:
//   - Step 2 matches on name only, not version — nothing in this codebase's
//     registration path (pkg/chassis/builder.go's synchronize) advertises
//     which version of a plugin a running process is, so two different
//     published versions running simultaneously can't be disambiguated at
//     the network layer today. In practice this repo only ever runs one
//     version of a given plugin at a time (matches Garage's own
//     "exact-pinned version, no ranges" scope note), so this isn't hit.
//   - Step 1's first-match-wins means two registries publishing the same
//     (name, version) resolve silently to whichever was added first, with
//     no collision warning — flagged as an open question in the design doc,
//     not solved here.
package main

import (
	"context"
	"fmt"
	"strings"

	plugincatalogv1 "github.com/steady-bytes/draft/api/tooling/plugin_catalog/v1"
	plugincatalogv1connect "github.com/steady-bytes/draft/api/tooling/plugin_catalog/v1/v1connect"
	settingsv1 "github.com/steady-bytes/draft/api/tooling/settings/v1"
	stepexecutorv1 "github.com/steady-bytes/draft/api/tooling/step_executor/v1"
	stepexecutorv1connect "github.com/steady-bytes/draft/api/tooling/step_executor/v1/v1connect"
	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"
)

// registryLister is the read side of *pgResultStore the garage:// executor
// and the search sidebar (ui.go) both depend on — kept narrow so tests can
// fake it without a real Postgres instance.
type registryLister interface {
	ListPluginRegistries(ctx context.Context) ([]*settingsv1.PluginRegistry, error)
}

// garagePluginExecutor is the Executor behind every `garage://...` reference
// — one shared instance handles all of them (unlike bench://grpc-call@v1,
// which is registered once but is itself generic across every `with:`;
// garagePluginExecutor is the same shape: one instance, per-call resolution
// driven by step.GetUses() rather than by which map key found it).
//
// Deliberately holds no cached PluginCatalogServiceClient: registries is
// queried fresh on every Execute call (see NewGaragePluginExecutor), so a
// registry added, edited, or removed through the Settings page changes what
// a garage:// step can resolve against on the very next run, not after a
// restart — the same "dynamic, not cached at construction" fix Phase 10c
// already applied to webhook routing.
type garagePluginExecutor struct {
	registries registryLister
	plugins    PluginResolver
	httpClient connect.HTTPClient
}

// NewGaragePluginExecutor builds the garage:// executor against the current
// set of configured plugin registries (registries) and the same Resolver a
// run's grpc-call steps already share (plugins) — see discovery.go.
func NewGaragePluginExecutor(httpClient connect.HTTPClient, registries registryLister, plugins PluginResolver) Executor {
	return &garagePluginExecutor{
		registries: registries,
		plugins:    plugins,
		httpClient: httpClient,
	}
}

func (e *garagePluginExecutor) Execute(ctx context.Context, step *workflowv1.Step, with *structpb.Struct) (*ExecutionResult, error) {
	name, version, err := parseGarageUses(step.GetUses())
	if err != nil {
		return nil, err
	}

	registries, err := e.registries.ListPluginRegistries(ctx)
	if err != nil {
		return nil, fmt.Errorf("garage plugin %q@%q: failed to list configured plugin registries: %w", name, version, err)
	}
	if len(registries) == 0 {
		return nil, fmt.Errorf("garage plugin %q@%q: no plugin registries are configured (see the Settings page)", name, version)
	}

	registryName, err := findPluginInRegistries(ctx, e.httpClient, registries, name, version)
	if err != nil {
		return nil, err
	}

	addr, err := e.plugins.ResolveByProcessName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("garage plugin %q@%q: %w", name, version, err)
	}

	client := stepexecutorv1connect.NewStepExecutorClient(e.httpClient, "http://"+addr, connect.WithGRPC())

	// Context is left unset: this step's with: has already been fully
	// template-resolved by the time Execute is called (scheduler.go's
	// runStep calls resolveTemplates before Execute), which covers every
	// {{ steps.X.result }} reference this repo's plugins actually need.
	// StepRequest.context exists in the contract for a plugin that wants
	// the raw prior-results snapshot beyond what its own with: references —
	// not needed by anything built so far, and threading a live snapshot
	// through the Executor interface would touch every existing
	// implementation (including every fake Executor scheduler_test.go
	// defines) for a capability nothing uses yet.
	resp, err := client.Execute(ctx, connect.NewRequest(&stepexecutorv1.StepRequest{
		StepName: step.GetName(),
		Config:   with,
	}))
	if err != nil {
		return nil, fmt.Errorf("garage plugin %q@%q: calling %s: %w", name, version, addr, err)
	}

	detail, detailErr := structpb.NewStruct(map[string]interface{}{
		"plugin":   name,
		"version":  version,
		"address":  addr,
		"registry": registryName,
	})
	if detailErr != nil {
		detail = nil
	}

	return &ExecutionResult{
		Result:        resp.Msg.GetResult(),
		Passed:        resp.Msg.GetSuccess(),
		FailureReason: resp.Msg.GetError(),
		Detail:        detail,
	}, nil
}

// findPluginInRegistries checks registries in order (insertion order, the
// order ListPluginRegistries already returns them in — see store.go) and
// returns the name of the first one whose PluginCatalogService.Get confirms
// (name, version) is published there. See the file comment's "Known
// simplifications" for what happens when more than one registry has it.
func findPluginInRegistries(ctx context.Context, httpClient connect.HTTPClient, registries []*settingsv1.PluginRegistry, name, version string) (string, error) {
	var checked []string
	for _, reg := range registries {
		client := plugincatalogv1connect.NewPluginCatalogServiceClient(httpClient, reg.GetAddress(), connect.WithGRPC())
		if _, err := client.Get(ctx, connect.NewRequest(&plugincatalogv1.GetPluginRequest{
			Name:    name,
			Version: version,
		})); err == nil {
			return reg.GetName(), nil
		}
		checked = append(checked, reg.GetName())
	}
	return "", fmt.Errorf("garage plugin %q@%q: not published to any configured registry (checked: %s)", name, version, strings.Join(checked, ", "))
}

// parseGarageUses splits "garage://name@version" into its name and version.
func parseGarageUses(uses string) (name, version string, err error) {
	_, rest, ok := strings.Cut(uses, "://")
	if !ok {
		return "", "", fmt.Errorf("step uses %q: missing scheme", uses)
	}
	name, version, ok = strings.Cut(rest, "@")
	if !ok || name == "" || version == "" {
		return "", "", fmt.Errorf("step uses %q: expected garage://<name>@<version>", uses)
	}
	return name, version, nil
}
