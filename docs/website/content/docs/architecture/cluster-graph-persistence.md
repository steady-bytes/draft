---
weight: 5
title: Cluster Graph — Persistence Implementation Plan
description: How the Blueprint web client renders the cluster graph with egui and persists layout state to localStorage and Blueprint KV.
icon: account_tree
draft: false
toc: true
---

The cluster page of the Blueprint web client renders a live graph of Draft core services using [egui](https://github.com/emilk/egui) embedded inside a Dioxus component. This document describes how the graph is built, how node layout is persisted to the browser's `localStorage`, and how that same layout will later be synced to Blueprint's key-value store so it survives across browsers and sessions.

---

## Background — egui inside Dioxus

Dioxus manages the page as a standard WASM single-page application. egui/eframe is a completely separate immediate-mode GUI framework that renders into an HTML `<canvas>` element using WebGL (via `glow`). The two frameworks coexist by having Dioxus own the canvas DOM element and hand it off to eframe on mount.

### Canvas lifecycle

The `Cluster` Dioxus component renders a single `<canvas id="cluster-egui-canvas">`. A `use_effect` hook fires on every mount:

1. Destroys any previous `WebRunner` (the eframe instance handle) stored in a `thread_local`
2. Looks up the canvas element from the DOM by ID
3. Calls `WebRunner::start()` with the `HtmlCanvasElement`, initialising a fresh eframe app against the new canvas

This destroy-and-recreate pattern is required because Dioxus unmounts and remounts components on route navigation. Each mount produces a new canvas DOM element — eframe must be reinitialised against it, not the previous one.

```rust
thread_local! {
    static RUNNER: RefCell<Option<WebRunner>> = const { RefCell::new(None) };
}

#[component]
pub fn Cluster() -> Element {
    use_effect(move || {
        RUNNER.with(|r| {
            if let Some(old) = r.borrow_mut().take() {
                old.destroy();
            }
        });
        spawn(async move {
            let canvas = web_sys::window()
                .and_then(|w| w.document())
                .and_then(|d| d.get_element_by_id("cluster-egui-canvas"))
                .and_then(|e| e.dyn_into::<web_sys::HtmlCanvasElement>().ok());
            // ... start WebRunner
        });
    });
    rsx! {
        canvas { id: "cluster-egui-canvas", style: "width: 100%; height: 100%; display: block;" }
    }
}
```

### Graph rendering

The graph is rendered using [egui_graphs](https://github.com/blitzarx1/egui_graphs), which wraps a `petgraph::StableGraph` and provides an interactive `GraphView` widget. Nodes use a custom `SquareNodeShape` that implements the `DisplayNode` trait:

- **Shape**: `RectShape` with `CornerRadius::ZERO` and a semi-transparent fill (`alpha = 20`) to give a "ghost square" appearance
- **Border**: 0.75px stroke, thickening to 1.5px when selected
- **Label**: rendered above each node using egui's font system; always visible
- **Edges**: 0.75px lines via `SettingsStyle::with_edge_stroke_hook`
- **Navigation**: zoom and pan enabled; initial fit-to-screen disabled so saved positions are respected

Node positions are set at construction time using `add_node_with_label_and_location`, giving each core service (Blueprint, Catalyst, Fuse, Service A/B/C) a fixed starting position that the user can then drag.

---

## Persistence — Phase 1: localStorage

eframe includes a built-in persistence layer that reads and writes `localStorage` on the web. Implementing it requires three things: a serializable layout type, two helper functions, and two `App` lifecycle hooks.

### Step 1 — Define `ClusterLayout` as a Protocol Buffer

Rather than defining the layout type by hand in Rust, it is defined as a Protocol Buffer in the shared API layer. This ensures the Go server and Rust client share the same type definition from the start, with no translation required when Blueprint KV persistence is added later.

**New file**: `api/core/registry/key_value/v1/cluster_layout.proto`

```protobuf
syntax = "proto3";

package core.registry.key_value.v1;

option go_package = "github.com/steady-bytes/draft/api/core/registry/key_value/v1";

message NodePosition {
    float x = 1;
    float y = 2;
}

message ClusterLayout {
    map<string, NodePosition> positions = 1;  // node label → position
}
```

Positions are keyed by node label string (e.g. `"Blueprint"`, `"Catalyst"`) rather than by petgraph node index. This means:

- Adding or removing nodes in the future does not invalidate saved state for existing nodes
- The key maps directly to a Blueprint KV key (`cluster/layout`) without any transformation

After creating the file, run:

```shell
dctl api build
```

This generates:
- **Go**: a `ClusterLayout` struct in `github.com/steady-bytes/draft/api/core/registry/key_value/v1`
- **Rust**: a `ClusterLayout` struct via `prost`, available through the existing `draft-api` crate

No new Cargo dependencies are needed — the web client already has `prost` and `draft-api`.

Because `eframe::set_value` / `get_value` use serde JSON internally, the generated `ClusterLayout` and `NodePosition` types need `serde::Serialize` and `serde::Deserialize`. If `prost-build` is not configured to derive these, add a thin newtype wrapper or enable serde support in the build script.

### Step 2 — Add extract / apply helpers

Two pure functions in `cluster.rs` move data between the live graph and the proto layout type. These functions are the only place that knows about both representations, and they are shared by the localStorage and Blueprint KV paths.

```rust
fn extract_layout(graph: &ClusterGraph) -> ClusterLayout {
    // Iterate graph nodes, read SquareNodeShape.label_text and .pos,
    // populate ClusterLayout.positions map
}

fn apply_layout(graph: &mut ClusterGraph, layout: &ClusterLayout) {
    // Iterate graph nodes, look up label_text in layout.positions,
    // write pos if found — unknown labels are silently ignored
}
```

`apply_layout` ignoring unknown labels is the key invariant: it means a layout saved with six nodes will apply cleanly to a graph that now has seven, leaving the new node at its default position.

### Step 3 — Hook into eframe's save / load cycle

eframe calls `App::save` automatically on a ~30-second interval and whenever the app is about to close. No manual timer is needed.

**Saving:**

```rust
impl eframe::App for ClusterApp {
    fn save(&mut self, storage: &mut dyn eframe::Storage) {
        let layout = extract_layout(&self.graph);
        eframe::set_value(storage, "cluster_layout", &layout);
    }
}
```

**Loading:**

```rust
impl ClusterApp {
    fn new(cc: &eframe::CreationContext<'_>) -> Self {
        let mut app = Self { graph: build_graph() };
        if let Some(layout) = cc.storage
            .and_then(|s| eframe::get_value::<ClusterLayout>(s, "cluster_layout"))
        {
            apply_layout(&mut app.graph, &layout);
        }
        app
    }
}
```

`build_graph` constructs the graph with default node positions. The saved layout is then applied on top, overwriting positions for any node whose label appears in the saved data.

### Step 4 — Verify persistence in `WebOptions`

`WebOptions::default()` enables eframe's persistence layer on web. Confirm during implementation that `cc.storage` is `Some` on the first load after a save — if it is `None`, check that `WebOptions::persist_window` or equivalent is not disabled.

---

## Persistence — Phase 2: Blueprint KV (future)

Once localStorage persistence is in place, Blueprint KV is additive — the `ClusterLayout` proto type and the extract/apply helpers do not change.

### Save path

`App::save` gains a second branch after the localStorage write:

```rust
fn save(&mut self, storage: &mut dyn eframe::Storage) {
    let layout = extract_layout(&self.graph);
    eframe::set_value(storage, "cluster_layout", &layout);

    // Encode to proto bytes and fire a gRPC Set call to Blueprint KV
    // key: "cluster/layout", value: prost::Message::encode(layout)
    // (fire-and-forget via spawn — save must not block the render loop)
}
```

The gRPC call is fire-and-forget: `spawn` it so it does not block the eframe render loop.

### Load path

`ClusterApp::new` gains a remote-first fallback:

1. Attempt a gRPC `Get` on key `cluster/layout` from Blueprint KV
2. If successful, decode the `ClusterLayout` proto and call `apply_layout`
3. If the call fails or returns nothing, fall back to the value in `cc.storage`

Because `ClusterApp::new` is called inside an `async` block (within the `WebRunner::start` closure), the gRPC call can be awaited without needing an additional spawn.

### Why this ordering matters

localStorage is always written first. If the Blueprint KV call fails (Blueprint is down, network error), the last known layout is still preserved locally. The KV store is the source of truth for cross-browser/cross-session sharing; localStorage is the fast-path cache.

---

## Summary

| Step | File(s) changed | What it does |
|---|---|---|
| 1 | `api/core/registry/key_value/v1/cluster_layout.proto` | Defines the shared layout type for both Rust and Go |
| 2 | `services/core/blueprint/web-client/src/views/cluster.rs` | Adds `extract_layout` and `apply_layout` helpers |
| 3 | `services/core/blueprint/web-client/src/views/cluster.rs` | Implements `App::save` and loads layout in `ClusterApp::new` |
| 4 | — | Verify `WebOptions` wires `cc.storage` correctly |
| Future | `cluster.rs` + Blueprint KV gRPC client | Adds remote save/load with localStorage fallback |
