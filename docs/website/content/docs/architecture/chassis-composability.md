---
weight: 6
title: 'Chassis — Composability & the Effect Stack'
description: 'How Chassis composes plugins today, the gap in that model, and the effect-tracking primitive that closes it.'
icon: 'extension'
draft: false
toc: true
---

[Chassis](https://github.com/steady-bytes/draft/tree/main/pkg/chassis) is the Go module every Draft service embeds. It handles registration with [Blueprint](/docs/architecture/core-services#blueprint), config, graceful shutdown, logging, and — the subject of this document — composing a service out of plugins: repositories, brokers, secret stores, RPC handlers, and consensus. This document explains the composability model Chassis uses today, a gap in it that shows up as a real bug in production code, and the design (and staged implementation) of the primitive that closes it.

It's written for two audiences at once: if you've never touched Chassis internals, the first half teaches the underlying idea from scratch. If you're implementing this change, the second half is the design doc.

---

## The idea, in one sentence

**The code that performs a side effect is the only code with enough context to also write that effect's inverse — so make writing the inverse part of performing the effect, not a separate step someone does later from memory.**

Instead of a setup function that just does something:

```go
func setup() error {
    // do a thing
    return nil
}
```

write a setup function that does the thing and hands back its own undo:

```go
func setup() (dispose func(context.Context) error, err error) {
    // do a thing
    return func(ctx context.Context) error {
        // undo that thing
        return nil
    }, nil
}
```

That's the whole idea. Once every mutation follows this shape, a generic runtime can collect the `dispose` closures as they're produced and run them in reverse order during shutdown — no plugin author has to hand-write a `Close()`/`cleanup()` path that might be incomplete, forgotten, or written by someone other than the person who understands what needs undoing.

The "reverse order" part matters: if a later effect assumed an earlier one was already in place, the later effect's teardown must run *before* the earlier one's, or its inverse might reach for something already gone. Last-in-first-out is the only ordering that's correct without every effect needing to know about every other effect's teardown requirements — it's the same reason Go's own `defer` unwinds LIFO within a function. This model just extends that discipline across a whole process's lifetime instead of one stack frame.

{{< alert context="info" text="This pattern — pairing every effect with a tracked inverse, so removal is a structural guarantee instead of an authoring convention — is formalized as **revertible effects** in the paper 'A Programming Paradigm for Spatiotemporal Composability' (Shi, Zhang, Cui; Peking University / DeepSeek-AI), and implemented as the core primitive of Cordis, the plugin framework underlying DeepSeek Harness. This document adapts that idea to Chassis's constraints — Go's compiled, statically-linked model doesn't support the paper's other half (hot-loading arbitrary new code at runtime the way Node.js can); what follows is the part of the idea that transfers cleanly." />}}

## Where Chassis stands today

A service composes its Chassis `Runtime` imperatively, once, in `main()`:

```go
c := chassis.New(logger).
    WithRepository(keyValueModel).
    WithConsensus(chassis.Raft, keyValueController)

c.WithRPCHandler(keyValueRPC).
    WithRPCHandler(serviceDiscoveryRPC).
    WithClientApplication(files, "web-client/target/dx/blueprint-pwa/release/web/public").
    WithRunner(func() { /* ... */ })

defer c.Start()
```

*(from [`services/core/blueprint/main.go`](https://github.com/steady-bytes/draft/blob/main/services/core/blueprint/main.go))*

`WithRepository`, `WithBroker`, and `WithSecretStore` each append to their own typed slice on `Runtime` — `repositories []Repository`, `brokers []Broker`, `secretStores []SecretStore` (`pkg/chassis/runtime.go`). `Runtime.shutdown()` then hand-rolls teardown per kind:

```go
// shutdown repositories
for _, r := range c.repositories {
    r := r
    group.Go(func() error {
        e := r.Close(ctx)
        // ...
    })
}

// shutdown brokers
for _, b := range c.brokers {
    b := b
    group.Go(func() error {
        e := b.Close(false)
        // ...
    })
}
```

*(from [`pkg/chassis/builder.go`](https://github.com/steady-bytes/draft/blob/main/pkg/chassis/builder.go))*

This works, but composition happens at the granularity of **plugin kind**, not **effect**. Two consequences follow directly, and one of them is a live gap in the codebase today:

1. **`secretStores` is collected and never torn down.** `WithSecretStore` appends to `c.secretStores`; nothing in `shutdown()` ever reads that slice. This isn't currently a bug — `SecretStore` only declares `Open` and `Get`, no `Close` — but it's the shape of gap this document is about: the day a `SecretStore` implementation needs to release a lease or close a connection, someone has to remember to add `Close` to the interface *and* add a fourth hand-copied loop to `shutdown()`. Nothing today enforces that both halves land together.
2. **Ad hoc mutations outside the three plugin kinds have no teardown path at all.** The `auth` service ([`services/core/auth/main.go`](https://github.com/steady-bytes/draft/blob/main/services/core/auth/main.go)) writes its address into Blueprint's key/value store on startup so [Fuse](/docs/architecture/core-services#fuse) can discover it and wire up the `ext_authz` filter:

   ```go
   func writeAuthAddress(logger chassis.Logger) {
       // ...
       _, err = client.Set(context.Background(), connect.NewRequest(&kvv1.SetRequest{
           Key: authServiceBlueprintKey, Value: val,
       }))
       if err != nil {
           logger.WithError(err).Error("failed to write auth_service_address to blueprint")
           return // swallowed — nothing above this frame learns the write failed
       }
   }
   ```

   This write has no corresponding delete. If `auth` exits cleanly, the key stays in Blueprint indefinitely, and Fuse keeps routing `ext_authz` checks at a dead address until something else notices — and nothing does, because `auth_service_address` is a raw KV key, not a registered service, so Blueprint's `service_discovery` reaper (which handles unhealthy *registered processes*) doesn't cover it.

Both problems trace to the same root cause: teardown is attached to plugin *kinds*, and every new kind of mutation needs its own bespoke cleanup path that nothing forces into existence. The rest of this document describes the primitive that removes that requirement.

## The design: a generic effect stack

```go
// Effect pairs a name (for logging and debugging) with the closure that reverts
// whatever the effect did.
type Effect struct {
    Name    string
    Dispose func(context.Context) error
}

// Effect performs setup and, if it succeeds and returns a non-nil dispose, tracks
// that dispose on the runtime's effect stack so it runs — in LIFO order, alongside
// every other tracked effect — during shutdown.
//
// A nil dispose is valid and means "this mutation has nothing to revert."
func (c *Runtime) Effect(name string, setup func() (dispose func(context.Context) error, err error)) *Runtime {
    dispose, err := setup()
    if err != nil {
        c.logger.WithError(err).WithField("effect", name).Fatal("failed to apply effect")
    }
    if dispose != nil {
        c.effectsMu.Lock()
        c.effects = append(c.effects, Effect{Name: name, Dispose: dispose})
        c.effectsMu.Unlock()
    }
    return c
}
```

Four decisions here are worth stating explicitly rather than leaving implicit, since each is a place to push back if the tradeoff is wrong for a given use case:

- **`name` is required, not derived.** The current pattern derives a log field via `reflect.TypeOf(plugin).String()`. That's fine when there's one instance per Go type, but breaks the moment two effects share a concrete type — two `postgres.Client` repositories for different domains would both log as `*postgres.Client`, indistinguishable in a shutdown-failure log line. An explicit name costs the caller one string and buys back debuggability permanently.
- **Setup failure is `Fatal`**, matching `WithRepository`'s existing behavior exactly. A service that can't establish a declared dependency at boot has never been allowed to limp along in Draft; this document doesn't change that policy.
- **The stack is mutex-guarded.** Every current `WithX` call happens synchronously in `main()` before `Start()`, so nothing appends concurrently today — but `WithRunner` goroutines run *after* `Start()`, and a future plugin registering an effect from inside one (a reconnect loop, say) is plausible. Guarding the slice now avoids a data race that would otherwise surface confusingly, in an unrelated PR, the first time someone does that.
- **Composition is sequential LIFO, not concurrent** — a deliberate departure from today's `errgroup`-based concurrent close. A later effect may assume an earlier one is still live while it tears down; sequential reverse-order teardown is the only ordering that's correct without every effect knowing about every other effect's requirements. This trades shutdown speed for a correctness guarantee the current code doesn't have. If shutdown latency becomes a measured problem for services with many independently-slow-closing plugins, the fix is an additive, opt-in concurrent variant for effects verified to be independent — not silently parallelizing the default.

## Implementation

### `Runtime` and the three built-in plugin kinds

`Runtime` drops its three typed slices for one:

```go
type Runtime struct {
    config                    Config
    logger                    Logger
    effects                   []Effect
    effectsMu                 sync.Mutex
    isRPC                     bool
    noMux                     bool
    rpcReflectionServiceNames []string
    rpcServiceNames           []string
    mux                       *http.ServeMux
    consensusKind             ConsensusKind
    raftAdvertiseAddress      *net.TCPAddr
    RaftController            RaftController
    onStart                   []func()
    blueprintClient           sdv1Cnt.ServiceDiscoveryServiceClient
    blueprintCluster          *BlueprintCluster
}
```

This is safe to do without touching any service: `repositories`, `brokers`, and `secretStores` are unexported fields referenced nowhere outside `pkg/chassis/builder.go` itself.

`WithRepository`, `WithBroker`, and `WithSecretStore` keep their exact public signatures — no service's `main.go` needs to change for this part — and become thin wrappers over `Effect`:

```go
func (c *Runtime) WithRepository(plugin Repository) *Runtime {
    name := reflect.TypeOf(plugin).String()
    return c.Effect(name, func() (func(context.Context) error, error) {
        if err := plugin.Open(context.Background(), c.config); err != nil {
            return nil, err
        }
        c.logger.WithField("plugin", name).Info("successfully set up repository plugin")
        return plugin.Close, nil
    })
}

func (c *Runtime) WithBroker(plugin Broker) *Runtime {
    name := reflect.TypeOf(plugin).String()
    return c.Effect(name, func() (func(context.Context) error, error) {
        if err := plugin.Open(context.Background(), c.config); err != nil {
            return nil, err
        }
        c.logger.WithField("plugin", name).Info("successfully set up broker plugin")
        return func(ctx context.Context) error {
            if err := plugin.Close(false); err != nil {
                c.logger.WithField("plugin", name).Error("failed to gracefully close broker: forcing")
                return plugin.Close(true)
            }
            return nil
        }, nil
    })
}

func (c *Runtime) WithSecretStore(plugin SecretStore) *Runtime {
    name := reflect.TypeOf(plugin).String()
    return c.Effect(name, func() (func(context.Context) error, error) {
        if err := plugin.Open(context.Background(), c.config); err != nil {
            return nil, err
        }
        c.logger.WithField("plugin", name).Info("successfully set up secret store plugin")
        return nil, nil // SecretStore has no Close today. Explicit nil, not an
                         // omission — the day it gains one, this is a one-line change.
    })
}
```

`WithBroker`'s graceful-then-forced retry logic moves out of `shutdown()` and into the wrapper — the more honest location for it, since only a broker plugin's own author knows whether force-close is meaningful for that broker.

`shutdown()` collapses from two duplicated `errgroup` blocks into one sequential loop:

```go
func (c *Runtime) shutdown() {
    c.logger.Info("shutting down")
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    for i := len(c.effects) - 1; i >= 0; i-- {
        e := c.effects[i]
        if err := e.Dispose(ctx); err != nil {
            c.logger.WithError(err).WithField("effect", e.Name).Error("failed to revert effect during shutdown")
            // continue — one failed teardown shouldn't block the rest from being attempted
        }
    }
    c.logger.Info("shutdown complete")
}
```

### Worked example: giving `auth` a real inverse

This is the concrete bug this document exists to fix. `services/core/auth/main.go` changes from:

```go
func main() {
    logger := zerolog.New()
    cfg := loadAuthConfig()
    handler := newCheckHandler(cfg, logger)

    defer chassis.New(logger).
        Register(chassis.RegistrationOptions{Namespace: "core"}).
        WithRunner(func() {
            writeAuthAddress(logger)
            serveCheckEndpoint(handler, logger)
        }).
        Start()
}
```

to:

```go
func main() {
    logger := zerolog.New()
    cfg := loadAuthConfig()
    handler := newCheckHandler(cfg, logger)

    c := chassis.New(logger).
        Register(chassis.RegistrationOptions{Namespace: "core"})

    c.Effect(authServiceBlueprintKey, func() (func(context.Context) error, error) {
        if err := writeAuthAddress(logger); err != nil {
            return nil, err
        }
        return deleteAuthAddress, nil
    })

    defer c.WithRunner(func() {
        serveCheckEndpoint(handler, logger)
    }).Start()
}

// writeAuthAddress now returns its error instead of logging-and-swallowing it.
// This isn't incidental — Effect's setup signature is func() (dispose, error), so
// the caller needs the real failure to decide whether to Fatal. A setup function
// that eats its own error is exactly the kind of gap this document closes, so
// fixing it here is part of the change, not a drive-by cleanup.
func writeAuthAddress(logger chassis.Logger) error {
    config := chassis.GetConfig()
    addr := fmt.Sprintf("http://%s:%d",
        config.GetString("service.network.internal.host"),
        config.GetInt("service.network.internal.port"))

    val, err := anypb.New(&kvv1.Value{Data: addr})
    if err != nil {
        return fmt.Errorf("failed to marshal auth_service_address: %w", err)
    }

    client := kvv1Connect.NewKeyValueServiceClient(http.DefaultClient, config.Entrypoint())
    if _, err := client.Set(context.Background(), connect.NewRequest(&kvv1.SetRequest{
        Key: authServiceBlueprintKey, Value: val,
    })); err != nil {
        return fmt.Errorf("failed to write auth_service_address to blueprint: %w", err)
    }
    logger.WithField("address", addr).Info("registered auth_service_address with blueprint")
    return nil
}

func deleteAuthAddress(ctx context.Context) error {
    config := chassis.GetConfig()
    val, _ := anypb.New(&kvv1.Value{}) // DeleteRequest.Value only carries type info
    client := kvv1Connect.NewKeyValueServiceClient(http.DefaultClient, config.Entrypoint())
    _, err := client.Delete(ctx, connect.NewRequest(&kvv1.DeleteRequest{
        Key: authServiceBlueprintKey, Value: val,
    }))
    return err
}
```

`kvv1.DeleteRequest` and the `Delete` RPC already exist on `KeyValueService` (`api/core/registry/key_value/v1/service.proto`) — this needs a caller, not a new API.

## Scope: what this fixes, and what it deliberately doesn't

**This closes the gap for graceful shutdown only.** If `auth` panics, is OOM-killed, or its node disappears, `deleteAuthAddress` never runs — nothing runs after a process is killed rather than asked to stop. Blueprint's `service_discovery` reaper is what covers that case for *registered services*, and remains necessary. The two mechanisms are complementary: the effect stack handles the common case (clean restarts, deploys, planned teardown) instantly and precisely; the reaper is the slower, coarser safety net for the case the effect stack structurally cannot reach. Giving `auth_service_address` the same crash-safety guarantee as a registered service's presence — folding raw KV keys into the registration/reap path — is a separate, larger change and out of scope here.

## Testing

`pkg/chassis` has no test coverage today. This is also the point at which it starts:

- `setup` runs exactly once per `Effect` call; its `dispose` runs exactly once during shutdown (not zero, not twice).
- Multiple effects tear down in strict reverse-registration order.
- `setup` returning `(nil, nil)` — no dispose — doesn't panic on shutdown and isn't part of the LIFO teardown list.
- `setup` returning a non-nil error triggers `Fatal` and never registers a dispose (there's nothing to revert if setup never completed).
- A dispose that returns an error doesn't stop the remaining effects in the stack from also being attempted.

## Rollout

1. Land `Effect` and the internal migration of `WithRepository`/`WithBroker`/`WithSecretStore`, with the tests above. Zero external API change — every existing `main.go` keeps compiling and behaving identically.
2. Land the `auth` service change as its own PR — the first caller of `Effect` directly, and the fix for the concrete bug that motivated this document.
3. Document the pattern in `pkg/chassis/doc.go` so the next one-off `client.Set()` inside a `WithRunner` closure reaches for `c.Effect(...)` instead of repeating the gap described above.

---

This is one step in a broader composability roadmap for Chassis — [reactive dependency resolution between plugins](/docs/architecture/chassis-reactive-dependencies), a [declarative plugin registry](/docs/architecture/chassis-declarative-plugin-registry) driven by config instead of hand-wired `main.go` files, and (much further out, speculative) [sandboxed dynamic components](/docs/architecture/chassis-sandboxed-dynamic-components). Those are placeholders, captured so the ideas aren't lost, not committed designs — this document covers only the effect-tracking primitive above, which is the one piece of the roadmap actually specified.
