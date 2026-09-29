---
weight: 4
title: 'Blueprint Web Client Components'
description: 'How the Blueprint web client is put together: its pages and modules, and where the components it used to define now live (the shared draft-ui crate).'
icon: 'dashboard'
draft: false
toc: true
---

The Blueprint web client is a Dioxus 0.7 (Rust/WASM) single-page application: the control panel for a Draft cluster. Since the [design-system migration](/docs/architecture/design-system-implementation-plan) it no longer defines its own components. Frame, controls, tables, charts and the query bar come from the shared **`draft-ui`** crate (`tools/draft-ui`), the same one Beacon and Lineman use, styled by the same compiled CSS. The component reference is that crate's [README](https://github.com/steady-bytes/draft/tree/main/tools/draft-ui) and its gallery (`tools/draft-ui/preview/index.html`); this page covers what stays in Blueprint.

---

## Pages

| Route | View | What it shows |
|---|---|---|
| `/`, `/kv/*key` | `key_value.rs` | The key/value store: a filterable list, typed values, and a detail drawer (the key is in the URL). Add, edit, copy and delete; secrets are masked |
| `/service-registry` | `service_registry.rs` | Services grouped by name with an instance strip per service, health, and the Raft cluster |
| `/gateway`, `/gateway/new`, `/gateway/:name` | `gateway.rs` | Fuse's routes grouped by kind, a flow diagram, and route create / edit |
| `/query` | `store.rs` | Catalyst's event store: a [CESQL](https://github.com/cloudevents/spec/blob/main/cesql/spec.md) query bar (typed predicates or raw), a time range, a type facet, a stream toggle and an event drawer |
| `/topology` | `topology.rs` (+ `../topology.rs`) | Producers, consumers and event types drawn as columns, with volumes; hovering isolates a node's flows |
| `/cluster` | `cluster/` | The cluster canvas: nodes, wires and event flows you can pan, zoom and rearrange, with an inspector and a context menu |
| `/metrics` | `metrics.rs` | Event volume and delivery numbers from Catalyst's topology |
| `/settings` | `settings.rs` | The navigation rail's sections and items, stored in the `ui/navigation` key |

An unknown path renders the shared `NotFound` inside the frame, so the rail is still there.

## Modules that stay in Blueprint

| Module | Role |
|---|---|
| `main.rs` | The router and the frame: `AppShell` with the configured rail sections, the Apps block from Fuse, and the rail counts |
| `kv.rs`, `raft.rs`, `registry.rs`, `routes.rs` | Thin clients over Blueprint's key/value, cluster and service-discovery RPCs, and Fuse's networking API, with the view models the pages need |
| `cesql.rs` | The query grammar and client-side matcher for `/query`. Catalyst's `Query` takes a typed expression a browser cannot build, so filtering runs in the client over the events the time range fetches |
| `events.rs` | Event decoding and the type / source helpers shared by `/query`, `/topology` and `/metrics` |
| `topology.rs` | Catalyst's `GetTopology` reshaped into nodes, edges and volumes |
| `views/cluster/` | The canvas: `model` (nodes and traces), `geometry` (wires, pads, viewport maths), `derive` (a live topology from Fuse, Blueprint and Catalyst), `persist` (positions in local storage and the `cluster/layout` key), `card`, `menu`, `inspector`, `traces` |

Manual nodes drawn on the canvas are local to that canvas and are not persisted; positions, pan and zoom are.

## Where the old components went

The previous version of this page documented components in `src/components/`. They were replaced, not moved:

| Old | Now |
|---|---|
| `WaveLoader` | `draft_ui::ui::Loading` |
| `CesqlBar`, `QueryBuilder` | `draft_ui::query::QueryBar` with the CESQL `Grammar` (typed predicates, autocomplete-ready) |
| `FilterChips` | `draft_ui::query::QueryChips` / `QueryFilters` |
| `TypeBadge` | `draft_ui::ui::Tag` |
| `MetricCard` | `draft_ui::data::StatTile` |
| `Hero` (landing) | removed: Key/Value is the home page |
| `ArcSpine`, `FullCircle` (the egui-drawn graph) | replaced by the Dioxus topology and cluster views; `eframe`, `egui_graphs` and `petgraph` are no longer dependencies |
| Navbar helpers, daisyUI drawer | `AppShell`, `NavSection` / `NavItem` (a CSS-only responsive rail) |
| `TopologyData`, `TopologyNode`, `TopologyEdge`, `event_color` | `topology.rs` and `events.rs` |

## Building and checking

```sh
cd services/core/blueprint/web-client
cargo test                                      # the view models and the query grammar, natively
cargo check --target wasm32-unknown-unknown     # the WASM build
dx build --release --platform web               # the bundle Blueprint's Go binary embeds
```

`dx serve` proxies Blueprint's key/value and service-discovery RPCs and Fuse's networking API (see `Dioxus.toml`). Dioxus 0.7's RSX has sharp edges the crate's components have already been shaped around: format strings take only simple identifiers and field access (compute anything else first), `key` only goes on the first node of a `for` body (wrap in a `Fragment`), and SVG attributes with dashes are string keys (`"aria-hidden"`).
