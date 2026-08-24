---
weight: 7
title: 'Chassis — Reactive Plugin Dependencies'
description: 'Roadmap idea: reactive dependency resolution between Chassis plugins. Not yet designed.'
icon: 'device_hub'
draft: true
toc: true
---

{{< alert context="warning" text="**Placeholder.** This page captures an idea from the Chassis composability roadmap so it isn't lost, not a committed design. There is no implementation plan here yet — see the open questions at the bottom for what would need to be resolved before there is one." />}}

This is one of three follow-on ideas from [Chassis — Composability & the Effect Stack](/docs/architecture/chassis-composability), which covers only effect *tracking* (undoing a mutation on shutdown). This page is about the other half: plugins reacting to *each other*, not just to shutdown.

## The problem this would solve

Today, dependencies between Chassis plugins are expressed as comments, not code. For example, [`services/core/blueprint/main.go`](https://github.com/steady-bytes/draft/blob/main/services/core/blueprint/main.go):

```go
// initialize service discovery components here since the controller requires
// the RaftController from the chassis
var (
    serviceDiscoveryController = sd.NewController(keyValueController, c.RaftController)
    serviceDiscoveryRPC        = sd.NewRPC(logger, serviceDiscoveryController)
)
```

`c.RaftController` must exist before this line runs, or it's nil. Nothing enforces that — a future refactor that reorders these calls compiles fine and fails at runtime, if it fails loudly at all. That's a real dependency, expressed only as prose.

## The idea, sketched

Blueprint's `service_discovery.Broadcaster` already does something like this at the *service* layer — pub/sub on process add/remove (`services/core/blueprint/service_discovery/broadcaster.go`). The idea is to bring the same shape down into Chassis itself, generic over any plugin-provided value, so a plugin can declare "I need X" and get notified when X becomes available (or goes away) instead of relying on call order in `main()`:

```go
type Key[T any] struct{ name string }

func NewKey[T any](name string) Key[T] { return Key[T]{name} }

func Provide[T any](c *Runtime, key Key[T], value T) (dispose func(context.Context) error)

func Watch[T any](c *Runtime, key Key[T], onChange func(value T, ok bool)) (unwatch func())
```

The comment above would become something enforced by the runtime instead of hoped for by the reader.

## Open questions

- **Sequencing**: this almost certainly wants [the effect stack](/docs/architecture/chassis-composability) landed first — `Provide`'s dispose is itself an effect, and there's no reason to design two separate teardown mechanisms.
- **Is this actually needed yet?** One documented case (the snippet above) isn't enough to justify a new generic primitive. The working rule from the broader research notes on this: wait for a second or third real ordering-dependency to show up before building this — watch for it, don't build ahead of it.
- **Ergonomics of Go generics** for this shape haven't been validated against the rest of the codebase's style — `Key[T]`/`Provide[T]`/`Watch[T]` reads fine in isolation but needs a real second or third use site to know if it's pleasant to use or just clever.
- **Where does this live?** As a `Runtime` extension (shown above), or as a step toward something more like Cordis's unified `Context` — a bigger structural question that shouldn't be answered by this one feature.
