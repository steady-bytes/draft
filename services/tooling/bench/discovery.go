// This file resolves two different kinds of names to a live "host:port",
// both going through Blueprint's ServiceDiscoveryService the same way every
// other Draft service discovers its peers — see
// pkg/chassis/builder.go's newBlueprintClient/synchronize for the pattern
// this mirrors (verified by reading that file, not assumed):
//
//   - ServiceResolver.Resolve, used by the grpc-call executor, resolves a
//     `with.service` name (e.g. "golf-app.app.v1.CourseCreator") — the
//     fully-qualified RPC service a step wants to call.
//   - PluginResolver.ResolveByProcessName, used by the garage-plugin
//     executor (garage_plugin.go, Phase 6), resolves a plugin's own
//     `service.name` (e.g. "slack-notify") to that specific instance's
//     address. This can't reuse ServiceResolver.Resolve: every garage
//     plugin implements the exact same RPC service
//     (tooling.step_executor.v1.StepExecutor), so looking up by RPC service
//     name the way grpc-call does would match *some* plugin, not
//     necessarily the one `garage://<name>@<version>` actually asked for.
//     Process.name (distinct from the RPC-service-keyed Metadata below) is
//     what disambiguates: it's the process's own service.name config value,
//     set once at Initialize and not required to be unique cluster-wide in
//     general, but unique in practice for a given plugin's process.
//
// Both resolvers share one underlying mechanism:
//
//   - QueryRequest.Filter is ignored by the current Blueprint implementation
//     (services/core/blueprint/service_discovery/controller.go's Query lists every
//     registered process regardless of what's passed), so resolution works by
//     querying everything once and matching client-side.
//   - Every registered process advertises the RPC service names it serves as
//     Metadata{Key: <fully-qualified service name>, Value: <same>} — see
//     pkg/chassis/builder.go's synchronize, which populates Metadata from
//     rpcServiceNames (itself populated by AddHandler's `pattern` argument, i.e.
//     the exact fully-qualified name a Connect handler is mounted at).
//   - Process.IpAddress — despite the name — holds the full "host:port" strings
//     (services/core/blueprint/service_discovery/controller.go sets it from
//     ClientDetails.AdvertiseAddress, which pkg/chassis/builder.go builds as
//     "<host>:<port>").
package main

import (
	"context"
	"fmt"
	"sync"

	sdv1 "github.com/steady-bytes/draft/api/core/registry/service_discovery/v1"
	sdv1Connect "github.com/steady-bytes/draft/api/core/registry/service_discovery/v1/v1connect"

	"connectrpc.com/connect"
)

// ServiceResolver resolves a fully-qualified RPC service name (as named in a
// grpc-call step's `with.service`) to a "host:port" address to call.
type ServiceResolver interface {
	Resolve(ctx context.Context, service string) (string, error)
}

// PluginResolver resolves a garage plugin's own process name (its
// service.name config value, e.g. "slack-notify") to a "host:port" address —
// see the file comment for why this can't just be ServiceResolver.Resolve
// against tooling.step_executor.v1.StepExecutor.
type PluginResolver interface {
	ResolveByProcessName(ctx context.Context, name string) (string, error)
}

// Resolver is what NewBlueprintResolver actually returns: a single cached
// Blueprint registry snapshot queried once and answering both kinds of
// lookup from it, so a plugin-resolving step doesn't cost a second Query
// call beyond what grpc-call steps already pay for in the same run.
type Resolver interface {
	ServiceResolver
	PluginResolver
}

// blueprintResolver is the real Resolver, backed by Blueprint's
// ServiceDiscoveryService. It caches the full registry snapshot from its first
// Query call for the lifetime of the resolver instance — the brief is explicit
// that a run shouldn't re-query Blueprint (which returns the *entire* registry,
// filter or not) before every single step, so one blueprintResolver is constructed
// per Run and shared across all of that run's steps/goroutines.
type blueprintResolver struct {
	client sdv1Connect.ServiceDiscoveryServiceClient

	mu     sync.Mutex
	loaded bool
	// addresses maps a fully-qualified RPC service name -> "host:port".
	addresses map[string]string
	// byProcessName maps a running process's own service.name -> "host:port"
	// — see PluginResolver's doc comment. Only PROCESS_RUNNING processes are
	// included: unlike addresses above (unfiltered, an existing and
	// documented staleness gap — see the "Found, but did not fix" note in
	// bench-workflow-engine.md's Phase 8 checkbox), a garage plugin is far
	// more likely to have multiple stale entries sharing the same Name after
	// a few local restarts during development, so filtering here is worth
	// the small extra check.
	byProcessName map[string]string
}

// NewBlueprintResolver builds a Resolver against Blueprint's entrypoint,
// using the same h2c-over-plaintext client construction chassis itself uses for
// blueprint clients (pkg/chassis/builder.go's newBlueprintClient) — copied
// verbatim rather than reinvented, per the brief.
func NewBlueprintResolver(httpClient connect.HTTPClient, entrypoint string) Resolver {
	return &blueprintResolver{
		client:        sdv1Connect.NewServiceDiscoveryServiceClient(httpClient, entrypoint),
		addresses:     make(map[string]string),
		byProcessName: make(map[string]string),
	}
}

func (r *blueprintResolver) Resolve(ctx context.Context, service string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.loaded {
		if err := r.load(ctx); err != nil {
			return "", err
		}
	}

	addr, ok := r.addresses[service]
	if !ok {
		return "", fmt.Errorf("no running process advertises RPC service %q (checked blueprint's registered process metadata)", service)
	}
	return addr, nil
}

func (r *blueprintResolver) ResolveByProcessName(ctx context.Context, name string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.loaded {
		if err := r.load(ctx); err != nil {
			return "", err
		}
	}

	addr, ok := r.byProcessName[name]
	if !ok {
		return "", fmt.Errorf("no running process named %q is registered with blueprint", name)
	}
	return addr, nil
}

// load performs the single Query call this resolver's cache is built from. Must be
// called with r.mu held.
func (r *blueprintResolver) load(ctx context.Context) error {
	res, err := r.client.Query(ctx, connect.NewRequest(&sdv1.QueryRequest{}))
	if err != nil {
		return fmt.Errorf("failed to query blueprint service discovery: %w", err)
	}

	for _, process := range res.Msg.GetData() {
		addr := process.GetIpAddress()
		if addr == "" {
			continue
		}
		for _, md := range process.GetMetadata() {
			if md.GetKey() != "" {
				r.addresses[md.GetKey()] = addr
			}
		}
		if process.GetRunningState() == sdv1.ProcessRunningState_PROCESS_RUNNING && process.GetName() != "" {
			r.byProcessName[process.GetName()] = addr
		}
	}
	r.loaded = true
	return nil
}
