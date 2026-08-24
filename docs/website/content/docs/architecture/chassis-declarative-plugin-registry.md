---
weight: 8
title: 'Chassis — Declarative Plugin Registry'
description: 'Roadmap idea: config-driven plugin composition instead of hand-wired main.go files. Not yet designed.'
icon: 'list_alt'
draft: true
toc: true
---

{{< alert context="warning" text="**Placeholder.** This page captures an idea from the Chassis composability roadmap so it isn't lost, not a committed design. There is no implementation plan here yet — see the open questions at the bottom for what would need to be resolved before there is one." />}}

This is one of three follow-on ideas from [Chassis — Composability & the Effect Stack](/docs/architecture/chassis-composability). That document covers tracking and reverting a mutation; this page is about a different question entirely: *which* plugins a service runs, and whether that has to be a compile-time decision.

## The problem this would solve

Every service's composition is fixed in Go source. [`services/core/blueprint/main.go`](https://github.com/steady-bytes/draft/blob/main/services/core/blueprint/main.go) is representative:

```go
c := chassis.New(logger).
    WithRepository(keyValueModel).
    WithConsensus(chassis.Raft, keyValueController)

c.WithRPCHandler(keyValueRPC).
    WithRPCHandler(serviceDiscoveryRPC).
    WithClientApplication(files, "web-client/target/dx/blueprint-pwa/release/web/public")
```

Changing what a service is composed of — swapping a repository backend, disabling a plugin for one deployment but not another — means editing this function and recompiling. There's no way to express "run this service with plugins A and C in staging, A and B in production" without two different binaries or a pile of build tags.

## The idea, sketched

A Go analog of a named-constructor registry (the pattern `database/sql` drivers already use), paired with a `plugins:` block in the service's existing `config.yaml`:

```go
func RegisterRepository(name string, ctor func(Config) (Repository, error))
```

```yaml
plugins:
  repositories:
    - name: sqlite
      enabled: true
      config: {}
```

`chassis.Boot(logger, config)` would resolve the plugin list from config instead of `main.go` doing it by hand. Combined with [the effect stack](/docs/architecture/chassis-composability), flipping `enabled: false` and reloading would tear that plugin's effects down cleanly; flipping it back on would re-instantiate it.

## Open questions

- **What triggers a reload?** SIGHUP, or a watched Blueprint key (which itself depends on [reactive plugin dependencies](/docs/architecture/chassis-reactive-dependencies) or an equivalent mechanism at the Blueprint KV layer — neither exists yet).
- **Build size / linking**: every registered plugin is still compiled into the binary whether `enabled` or not — this doesn't reduce binary size, only changes what's active at runtime. `database/sql` has the same property with its drivers; worth deciding explicitly whether that's acceptable here rather than discovering it as a surprise later.
- **Is this worth the migration cost?** Every service's `main.go` would need to change, unlike the effect stack (additive, zero call-site changes). That's a much bigger ask, and should wait for a stated requirement — a team actually needing to swap plugin backends per deployment without a rebuild — rather than being built because it's a natural next step on paper.
- **Relationship to existing `config.yaml`**: does `plugins:` live alongside existing service config, or does this want its own file? Not decided.
