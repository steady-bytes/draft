---
weight: 9
title: 'Chassis — Sandboxed Dynamic Components'
description: 'Speculative idea: hot-loadable, sandboxed components at runtime. Not a committed roadmap item.'
icon: 'science'
draft: true
toc: true
---

{{< alert context="warning" text="**Speculative — further out than the rest of the roadmap.** This isn't a committed design or even a firm roadmap item, just an idea worth recording so it isn't reinvented from scratch later. It should not be started before the effect stack, reactive dependencies, or declarative plugin registry ideas, and probably shouldn't be started at all without a concrete driving use case." />}}

This is the third and most speculative of three follow-on ideas from [Chassis — Composability & the Effect Stack](/docs/architecture/chassis-composability). The other two ([reactive dependencies](/docs/architecture/chassis-reactive-dependencies), [declarative registry](/docs/architecture/chassis-declarative-plugin-registry)) are about composing plugins that already exist as Go code in the binary. This one is about something categorically different: a component that didn't exist when the binary was built.

## The motivating case

DeepSeek Harness — an AI agent harness built on [Cordis](https://github.com/cordiverse/cordis), the TypeScript plugin framework whose composability model this whole roadmap draws from — ships a self-referential toolset that lets a model define, run, stop, and remove a sandboxed plugin *inside its own running process*, at the model's own initiative. That's a working existence proof of a "self-evolving" component system, not a hypothetical.

## Why this doesn't translate directly to Go

Cordis's version of this depends on something Node.js has and Go does not: a module registry that can evict and garbage-collect a loaded module (`require.cache`). Go has no equivalent for statically-linked code — once `main.go` links a package, it's in the binary for the process's lifetime. Go's `plugin` package (`.so` loading) exists, but is Linux-only, requires the plugin to be built with the exact same toolchain and dependency versions as the host, and — the disqualifying part — **loaded plugins cannot be unloaded**. There's no `dlclose` equivalent. That's a hard violation of the "revert on removal" guarantee the rest of this roadmap is built around, not a rough edge to work through.

## The realistic path, if this is ever pursued

WebAssembly, not Go plugins. A WASM module compiled from arbitrary source can be instantiated and **cleanly torn down** by a pure-Go runtime like [wazero](https://wazero.io/) (no cgo). The sandbox boundary would be WASM linear memory plus an explicit host-function allow-list — structurally the same shape as Cordis's own `node:vm` sandbox for its dynamic toolset, including the same caveat Cordis states explicitly about it: the sandbox isolates globals, but it is not a security boundary on its own — whatever host functions get exposed to the sandboxed code *are* the real trust boundary, and have to be reasoned about as carefully as, say, deciding what a service account can reach.

## Why this is last, not first

Everything else on this roadmap makes an existing, static composition model in Chassis more capable. This one would introduce genuinely new capability — arbitrary code arriving and running inside a live process — with a security surface to match. It's recorded here because the paper this roadmap is based on names self-evolving agent harnesses as the compelling future-validation direction for this whole model, and it would be a mistake to lose that thread. It is not recorded here as something to build without a concrete, specific use case driving it — a self-modifying operator/controller component, for instance — that justifies taking on the sandbox's design and trust burden.
