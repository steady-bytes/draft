---
weight: 4
title: 'Blueprint Web Client Components'
description: 'A reference guide to the reusable Dioxus components that make up the Blueprint web client UI.'
icon: 'dashboard'
draft: false
toc: true
---

The Blueprint web client is a Dioxus (Rust/WASM) single-page application that provides a control panel for Draft clusters. Its UI is built from a set of reusable components living in `services/core/blueprint/web-client/src/components/`. This page documents each component, its inputs, and its purpose.

---

## WaveLoader

**File:** `wave_loader.rs`

An animated sound-wave loading indicator made of seven thin white SVG bars. Each bar pulses at a slightly different speed and delay to produce a random, organic wave effect. The animation keyframes are embedded directly in the SVG so the component has no external CSS dependency.

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| `width` | `u32` | `80` | Width of the SVG in pixels. |
| `height` | `u32` | `30` | Height of the SVG in pixels. Bars scale to fill the full height. |

```rust
WaveLoader {}                          // 80 × 30 default
WaveLoader { width: 120, height: 40 } // larger variant
```

---

## CesqlBar

**File:** `cesql_bar.rs`

A single-row input bar for entering and running [CESQL](https://github.com/cloudevents/spec/blob/main/cesql/spec.md) filter expressions. Renders a text field, a clear (`✕`) button, and a **run** button joined into a single DaisyUI `join` group. Pressing Enter in the field also triggers `on_run`.

| Prop | Type | Description |
|------|------|-------------|
| `expression` | `Signal<String>` | Two-way binding for the current expression string. |
| `on_run` | `EventHandler<()>` | Called when the user clicks **run** or presses Enter. |
| `on_clear` | `EventHandler<()>` | Called when the user clicks the clear button. |

```rust
CesqlBar {
    expression,
    on_run:  move |_| run_query.call(()),
    on_clear: move |_| expression.set(String::new()),
}
```

---

## FilterChips

**File:** `filter_chips.rs`

A horizontal row of clickable badge chips, each representing a common preset CESQL expression (e.g. `type = 'order.created'`, `source LIKE '%shop%'`). Clicking a chip calls `on_select` with the full CESQL string, which the parent can append to the expression bar.

| Prop | Type | Description |
|------|------|-------------|
| `on_select` | `EventHandler<String>` | Called with the preset CESQL expression string when a chip is clicked. |

```rust
FilterChips {
    on_select: move |expr: String| expression.set(expr),
}
```

---

## TypeBadge

**File:** `type_badge.rs`

A small colored badge that displays a CloudEvent `type` string. The badge color is deterministically derived from the event type name via a hash, so the same type always renders in the same color across the UI.

| Prop | Type | Description |
|------|------|-------------|
| `event_type` | `String` | The CloudEvent type string to display (e.g. `"com.shop.order.created"`). |

```rust
TypeBadge { event_type: event.r#type.clone() }
```

---

## QueryBuilder

**File:** `query_builder.rs`

A collapsible form that guides the user through building a CESQL filter expression without writing it by hand. The user picks a field (`type`, `source`, `id`, `subject`, or `body.*`), an operator (`=` or `LIKE`), and a value. A live preview shows the resulting fragment before it is added. Also contains an ORDER BY toggle for controlling query sort direction.

When `has_expression` is true an AND/OR connector toggle appears so the new fragment can be joined to an existing expression.

| Prop | Type | Description |
|------|------|-------------|
| `has_expression` | `bool` | Whether an expression already exists. Controls whether the AND/OR connector is shown. |
| `on_add` | `EventHandler<String>` | Called with the generated CESQL fragment (prefixed with `"OR "` or `"AND "` when joining). |
| `sort_dir` | `SortDir` | Current sort direction (`SortDir::Asc` or `SortDir::Desc`). |
| `on_sort_change` | `EventHandler<SortDir>` | Called when the user changes the ORDER BY direction. |

```rust
QueryBuilder {
    has_expression: !expression.read().is_empty(),
    sort_dir: sort_dir(),
    on_sort_change: move |dir: SortDir| { sort_dir.set(dir); run_query.call(()); },
    on_add: move |fragment: String| {
        let current = expression.read().clone();
        expression.set(if current.trim().is_empty() { fragment }
                        else { format!("{current} {fragment}") });
    },
}
```

---

## MetricCard

**File:** `metric_card.rs`

A dashboard-style card that displays a single metric value with a label, optional unit, trend indicator, and a small sparkline chart. The trend arrow and color respond to `trend_up` and `trend_good` so the card can express both direction and health independently.

| Prop | Type | Description |
|------|------|-------------|
| `icon` | `MetricIcon` | Icon displayed in the card header. One of: `Messages`, `Clock`, `Warning`, `Users`, `Server`, `ArrowUp`. |
| `label` | `String` | Uppercase label shown above the value (e.g. `"MESSAGES / MIN"`). |
| `value` | `String` | The primary value displayed in large text. Pass `"—"` when data is unavailable. |
| `unit` | `Option<String>` | Optional unit suffix rendered smaller next to the value (e.g. `Some("ms")`). |
| `trend_up` | `bool` | `true` renders an upward arrow `↑`, `false` renders a downward arrow `↓`. |
| `trend_good` | `bool` | `true` colors the trend green, `false` colors it red. |
| `trend_delta` | `String` | Label shown next to the trend arrow (e.g. `"live"`, `"since start"`). |
| `sparkline_data` | `Vec<f32>` | Series of data points rendered as a polyline sparkline. Needs at least 2 points. |
| `sparkline_color` | `String` | CSS color string for the sparkline stroke (e.g. `"#4ade80"`). |

```rust
MetricCard {
    icon: MetricIcon::Messages,
    label: "MESSAGES / MIN".to_string(),
    value: m.msgs_per_min.to_string(),
    unit: None,
    trend_up: true,
    trend_good: true,
    trend_delta: "live".to_string(),
    sparkline_data: vec![280.0, 295.0, 310.0, 290.0, 305.0],
    sparkline_color: "#4ade80".to_string(),
}
```

---

## Hero

**File:** `hero.rs`

A full-screen DaisyUI hero section shown on the Blueprint landing page. Displays the application name and a link to the Draft documentation. Takes no props — the application name is read from a global signal.

```rust
Hero {}
```

---

## ArcSpine

**File:** `arc_spine.rs`

An SVG visualization that renders the event topology as a two-column arc diagram. Producers appear on the left, consumers on the right, and edges between them are drawn as cubic Bézier curves colored by event type. Nodes are interactive — clicking one highlights its connected edges and dims the rest. Bundled edges between the same producer/consumer pair are fanned out with a small offset.

| Prop | Type | Description |
|------|------|-------------|
| `data` | `TopologyData` | The full topology snapshot: producers, consumers, and edges with event types. |

See [Topology Data Types](#topology-data-types) below for the shape of `TopologyData`.

```rust
ArcSpine { data: topology_data }
```

---

## FullCircle

**File:** `full_circle.rs`

An SVG visualization that arranges all topology nodes (producers and consumers) evenly around a single circle. Edges are drawn as Bézier curves through the center, with stroke weight and opacity scaled by message volume. Clicking a node opens a side panel listing its event connections and volumes.

| Prop | Type | Description |
|------|------|-------------|
| `data` | `TopologyData` | The full topology snapshot: producers, consumers, and edges with volume counts. |

```rust
FullCircle { data: topology_data }
```

---

## Navbar Helpers

**File:** `navbar.rs`

Three zero-argument helper functions used to compose the application navbar. They are free functions rather than components — call them with `{navbar_icon()}` syntax inside RSX.

| Function | Description |
|----------|-------------|
| `navbar_icon()` | Renders the application name as a link back to the Key Value view. |
| `navbar_menu_button()` | Renders a hamburger icon button that opens the DaisyUI drawer sidebar. |
| `navbar_secondary_menu_button()` | Renders a three-dot overflow button for secondary navigation actions. |

---

## Topology Data Types

**File:** `topology.rs`

Shared data structures passed to `ArcSpine` and `FullCircle`, plus a color-mapping helper used across topology and event table views.

### `TopologyData`

| Field | Type | Description |
|-------|------|-------------|
| `producers` | `Vec<TopologyNode>` | Services that publish CloudEvents. |
| `consumers` | `Vec<TopologyNode>` | Services that subscribe to CloudEvents. |
| `edges` | `Vec<TopologyEdge>` | Directed flows from a producer to a consumer for a given event type. |

### `TopologyNode`

| Field | Type | Description |
|-------|------|-------------|
| `id` | `String` | Unique identifier for the node (matches the CloudEvent `source` field). |
| `name` | `String` | Human-readable display name. Falls back to `id` when empty. |

### `TopologyEdge`

| Field | Type | Description |
|-------|------|-------------|
| `producer_id` | `String` | `id` of the producing node. |
| `consumer_id` | `String` | `id` of the consuming node. |
| `event_type` | `String` | CloudEvent type string flowing along this edge. |
| `vol` | `u32` | Message volume count, used to scale visual weight in topology charts. |

### `event_color(event_type: &str) -> &'static str`

Maps well-known event type strings to hex color codes for consistent coloring across the UI. Falls back to `#6b7280` (gray) for unrecognized types.

| Event type | Color |
|------------|-------|
| `"created"` | `#3b82f6` (blue) |
| `"cancelled"` | `#ef4444` (red) |
| `"ok"` | `#22c55e` (green) |
| `"failed"` | `#f97316` (orange) |
| `"reserved"` | `#a855f7` (purple) |
| `"updated"` | `#14b8a6` (teal) |
| `"registered"` | `#eab308` (yellow) |
| _other_ | `#6b7280` (gray) |
