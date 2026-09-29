---
weight: 46
title: 'Design System — Implementation Plan'
description: 'Assessment of the new Draft mockups and a phased plan to move every Draft UI (Blueprint, Beacon, Lineman, Bench, Foundry) onto one shared design system: a `draft-ui` crate in tools/ with the SCSS/CSS, daisyUI theme configuration, Dioxus components, and a Go SSR kit.'
icon: 'palette'
draft: false
toc: true
---

{{< alert context="info" text="Implemented 2026-09-26. The plan below is kept as it was reviewed; what actually shipped, where it differs, and what was found along the way is in section 12. Every number in this document was measured against the repo on 2026-09-26; items marked VERIFY were resolved in Phase 0." />}}

## 1. Summary

`mockups/` contains a complete visual system (`draft.css`, 785 lines) and 17 page mockups covering Blueprint, Beacon, Lineman, Bench and Foundry. This plan moves all of Draft's UIs onto it by:

1. Creating **`tools/draft-ui`** — one directory, one source of truth, two thin language bindings:
   - **SCSS source → compiled CSS** (design tokens, daisyUI 5 theme configuration, and the `d-*` component classes).
   - **Rust crate** of Dioxus components used by the three WASM clients (Blueprint, Beacon, Lineman).
   - **Go module** that embeds the same CSS plus `html/template` partials for the two server-rendered services (Bench, Foundry).
2. Migrating each app in stages (theme swap → shell → views) so every app keeps working after every merge.
3. Deleting the duplicated code the shared kit replaces (≈3,500 LOC of app-local Dioxus components, two copies of the Go SSR scaffolding, ~1 MB of unreferenced vendored CSS/JS).

What you get at the end: one look across all services, no CDN dependency for styling, no Tailwind Play CDN, and each UI pattern (stat tile, query bar, drawer, table, waterfall…) implemented exactly once per renderer.

### Scope at a glance

| | Today | After |
|---|---|---|
| Route-level views | 36 (Blueprint 13, Beacon 5, Lineman 8, Bench 7, Foundry 3; counts include 404s, and Bench's new/edit form counts once) | every URL keeps working; 6 routes fold into another view (Blueprint's key/value, service and route detail pages become drawers, Blueprint's NewRoute becomes a modal, Lineman's TaskBoard merges into ObjectiveDetail, Bench's workflow list merges into the Overview); 1 new page (Bench Recent runs) |
| Mockup coverage | 17 pages covering 22 of the 36 views | all 17 implemented; the other 14 views (+ the new page) get the shared components inside each app's phase, with no bespoke design |
| Shells | 3 different Dioxus shells + 2 Go `base.html` copies | 1 `AppShell` (Rust) + 1 `shell` template (Go), same CSS |
| Styling | daisyUI 5 CDN + Tailwind browser JIT (Dioxus), daisyUI **4** + Tailwind Play CDN (Go) | `draft.css` only, self-hosted |
| Hard-coded colours in Rust | 96 source lines with hex literals (54 in `cluster.rs`) | 0 |

---

## 2. Assessment of the mockups

### 2.1 What the design is

"Circuit board, not dashboard": dark-first, monospace for anything that is data, 1 px rules instead of shadows, bracket-corner marks for focus, and colour only where it means something (healthy is the quietest state). Tokens: 8 surface/ink values, 4 component "kind" colours (Catalyst blue, Blueprint violet, Fuse amber, generic slate), warn/err, and a swappable **primary** (5 swatches). Two themes, `draft` (dark) and `draft-light`.

Every page uses the same frame: 232 px rail · 48 px topbar (breadcrumb, status pill, ⌘K, theme) · main · 28 px status bar. The rail is per-app sections plus a shared **Apps** block of cross-links (↗).

### 2.2 Page → current view map

| Mockup | Current implementation | Relationship |
|---|---|---|
| `blueprint-key-value` | `views/key_value.rs` (343) + `key_value_detail.rs` (155) | list + **drawer**; detail page folds in |
| `blueprint-service-registry` | `service_registry.rs` (170) + `service_detail.rs` (245) | **expandable rows** replace the per-service detail page |
| `blueprint-gateway` | `gateway.rs` (152) + `route_detail.rs` (471, also `NewRoute`) | list + drawer; form becomes modal |
| `blueprint-cluster` | `cluster.rs` (2,117) | restyle + groups, minimap, inspector |
| `blueprint-events-query` | `store.rs` (746) | restyle; facets, detail drawer |
| `blueprint-events-topology` | `topology.rs` (127) + `full_circle.rs` (657) | **new visual** (3-column flow board) |
| `blueprint-events-metrics` | `metrics.rs` (485) + `metric_card.rs` | restyle; shared chart |
| `beacon-wide-events` / `-logs` / `-traces` / `-metrics` | `wide_events.rs` / `stream.rs` / `traces.rs` / `metrics.rs` | restyle; traces switches flame graph → **waterfall** |
| `lineman-board` | `objective_detail.rs` (324) + `task_board.rs` (132) | two views **merge** into one (Board ⇄ List) |
| `bench-overview` | `dashboard.html` + `workflow_list.html` | two pages **merge**; adds history strips |
| `bench-run` | `run_detail.html` | restyle; adds progress strip, grouped steps, side panels |
| `bench-editor` | `workflow_form.html` | adds name/description fields, pipeline strip, gutter, problems panel |
| `foundry-catalog` / `foundry-plugin` | `catalog.html` + `card_grid.html` / `detail.html` | restyle; schema gains a description column |

**No mockup exists for** (they get the shared components, no bespoke design): Blueprint Settings, NewRoute form, 404s; Lineman Dashboard, Create Objective, Task Detail, Scheduler, Loops; Bench workflow detail, Recent runs (nav item exists, page does not), Settings; every loading / empty / error state; ⌘K palette (button is drawn, palette is not).

### 2.3 Findings that shape the plan

**F1 — The design system is self-contained; Tailwind is not needed.** The mockups use only `d-*` classes plus small page-local `<style>` blocks. daisyUI appears only as a *token mapping* (`--color-*` under `[data-theme="draft"]`) so legacy `btn`/`badge`/`table` markup adopts the palette during migration. Consequence: the Go services can drop Tailwind Play CDN + daisyUI 4 entirely, and the Dioxus clients can drop the browser JIT once no utility classes remain.

**F2 — Page-local CSS is copy-pasted across pages.** Comparing the 17 inline `<style>` blocks: 12 pages define their own list-plus-drawer grid (`.kv-layout`, `.gw-layout`, `.eq-layout`, `.lg-layout`, `.we-layout`, `.tr-layout`, `.mx-layout`, `.tp-layout`, `.ed-layout`, `.rn-layout`, `.pd-layout`, `.cl-wrap`); 15 of 17 pages re-declare the same `↗` rail rule; 8 define their own toolbar row; 8 their own drawer heading + section block; 5 their own JSON/code `pre`; 3 their own bar strip (four variants); 2 their own waterfall/duration lanes. These are hoisted into shared classes (table in §5.2). Anything used by only one page stays page-local.

**F3 — Accessibility defects in the mockup that must be fixed before we copy them into production** (measured, WCAG 2.x contrast):

| Issue | Measured | Fix |
|---|---|---|
| `--dimmer` used for readable text (ticket IDs like `T-104`, chart axis labels, step numbers, placeholders): `color: var(--dimmer)` appears 6× in `draft.css` and 27× across the pages; not all are text (separators and masks are decoration), but the text ones fail | dark 3.12 : 1 on `--bg`, 2.61 : 1 on `--panel-3`; light 2.38 : 1 | Keep the token, restrict it to borders/decoration/disabled (the file's own comment already says so); re-point every text use to `--dim` (dark 5.31 : 1 worst case). Placeholders → `--dim` |
| Light `--fs` `#b0620a` | 3.92 : 1 on `--panel-3` | `#9a5508` → 4.90 : 1 |
| Light `--ca` `#1f6fd1` | 4.24 : 1 on `--panel-3` | `#1a63bd` → 5.05 : 1 |
| Light `--err` `#c93636`, `--warn` `#946400` | 4.43 / 4.41 : 1 on `--panel-3` | `#bd2f2f` (4.99) / `#8a5c00` (4.99) |
| Light primary derived at 68 % toward black | worst swatch (`#3ddc97`, `#ffb454`) 4.06 : 1 on `--panel-3` | derive at **62 %** → all 5 swatches ≥ 4.99 : 1 |
| `.d-toggle` is a `<span role="switch">` with no `tabindex` (11 uses: 9 in pages, 2 in the design-system page) | not keyboard reachable | `<button role="switch" aria-checked>` |
| No responsive rules at all (`@media` appears once, for reduced-motion) | shell is fixed 232 px rail + `100vh` | add breakpoints (§5.1). Blueprint is a PWA today with a collapsing drawer; the mockup would regress mobile |
| `@import url(fonts.googleapis…)` at the top of the stylesheet | render-blocking, external, fails offline (PWA/desktop) | self-host Inter + JetBrains Mono woff2 under `dist/fonts/` (flag `$self-host-fonts`) |
| Theme/primary stored in per-origin `localStorage`; each app is its own subdomain | choice does not follow the user between `bench.` / `blueprint.` / `beacon.draft.localhost` | try a parent-domain cookie (VERIFY in Phase 0), fall back to per-origin |

These are enforced by a test in the crate (§3.7) so they cannot regress.

**F4 — The mockups assume data that does not exist yet.** §7 lists 30 items. After the Phase 0 audit: 17 need backend, proto, feature or bug-fix work, 5 were checked and are simply absent (omitted), 1 is available with no work, 5 are client-only, 1 is a design task, and 1 (theme sync) is verified working. None block the restyle: each ships as an omitted slot and lights up when its data lands. Highlights: Bench `Run` has no trigger/user/trace_id and `Trigger` only has `webhook` (no `schedule`, but the overview mockup shows "schedule · hourly"); Foundry has no category/usage/address data; Blueprint KV has no revision/updated_at; Beacon `MetricsService` has only `QueryMetrics` (no metric browser); Lineman `Task` has no due date or short id (`T-104`).

**F5 — Three mockups deliberately change behaviour, not just looks. Flagging so they are decisions, not surprises:**

1. **Query builders.** Blueprint (260 LOC), Beacon (357) and Beacon's wide-event builder (355) are structured field/operator/value forms. The mockups show only a query bar plus preset chips. **Decided:** keep the capability as **one grammar-driven predicate builder** opened from a trailing "Filter…" chip (972 → ~200 LOC), designed so query autocomplete can be added later without reworking the schema (§3.8). Small deviation from the mockup. PromQL is not a predicate grammar and stays bar + chips until it gets its own completer.
2. **Beacon traces** switches from flame graph to waterfall. The flame graph stays for the Wide-events "Flame graph" view.
3. **Lineman board** adds drag-and-drop ("Drag cards between columns"). **Decided:** include it, staged after the board itself (D11). `UpdateTaskState` and `ReorderTask` already exist, so no backend change.

**F6 — Existing code the shared kit makes redundant** (all measured):

| Duplication | Where | Resolution |
|---|---|---|
| `MetricCard` copied nearly verbatim (the two files differ by 15 lines of `diff` output, mostly a header comment and one reformatted expression) | Blueprint 185 LOC, Beacon 187 LOC. Beacon's header says it copied to avoid a cross-crate dependency | one `StatTile` + `Sparkline` |
| Three different shells | Blueprint (drawer + KV-config nav + Fuse-discovered "Services"), Beacon (top navbar), Lineman (static drawer) | one `AppShell`; Blueprint's `ui/navigation` KV config and Fuse `ListRoutes` discovery move into the shared crate so Beacon/Lineman get them free |
| `PageNotFound` ×3, `toast`, `priority_badge`, `agent_badge`, `severity_badge`, `TypeBadge`, `AuthBadge`, `ProtocolBadges`, `ValidationDot` | all three clients | `Tag` / `Status` / `Dot` + per-app mapping functions |
| `get_domain()` + `API_DOMAIN` + `GrpcConfig` provider + `main()` boilerplate | 3× | `draft_ui::util` |
| Unused components: `Hero`, `FilterChips`, `ArcSpine` | Blueprint | delete |
| Unused dependencies: `eframe`, `egui_graphs`, `petgraph`, `dotenv` (0 source uses; `cluster` view already replaced egui) | Blueprint `Cargo.toml` | delete |
| ~1.07 MB unreferenced `daisyui.css`, `daisyui-theme.css`, `tailwind.js` in `public/` — copied into the release build and therefore **into the Go binary** via `go:embed web-client/target/dx/…/public` | Blueprint | delete |
| `base.html` (6.4 KB and 7.2 KB, same theme block and layout), `static.go` (2), `templates.go` (2), `htmx.min.js` (2 × 48 KB), `.signal` CSS (2) | Bench, Foundry | `draftui` Go module |
| Bench renders run rows in 3 places (dashboard failures, dashboard recent, workflow history) | `dashboard.html`, `workflow_detail.html` | one `runs-table` partial |

---

## 3. Architecture: the `draft-ui` crate

### 3.1 Layout

```
github/draft/tools/draft-ui/
├── Cargo.toml            # package draft-ui · features: dioxus, discovery, cli
├── go.mod                # module github.com/steady-bytes/draft/tools/draft-ui
├── embed.go              # package draftui — //go:embed dist templates
├── README.md
├── scss/
│   ├── draft.scss                 # entry point
│   ├── abstracts/_tokens.scss     # $themes, $kinds, $tones, $swatches maps
│   ├── abstracts/_mixins.scss     # label(), mono(), tint(), focus-ring()
│   ├── base/_theme.scss           # emits :root/[data-theme] vars from the maps
│   ├── base/_daisyui.scss         # --color-* daisyUI 5 mapping
│   ├── base/_reset.scss  base/_type.scss
│   ├── layout/                    # _shell _split _page _toolbar _drawer _responsive
│   ├── components/                # _btn _field _seg _toggle _chip _tag _status _count _kbd
│   │                              # _table _stat _card _panel _kv _list _code _strip _meter
│   │                              # _modal _alert _toast _menu _empty _loading _details
│   ├── viz/                       # _chart _histogram _waterfall _wire _editor _trace _flow
│   └── docs/_bootstrap.scss       # Phase 6: maps tokens onto the Hugo/lotusdocs Bootstrap vars
├── dist/                          # GENERATED and committed — never hand-edit
│   ├── draft.css  draft.min.css
│   ├── draft-daisyui.theme.css    # Tailwind v4 `@plugin "daisyui/theme"` blocks (literal values)
│   ├── tokens.css                 # variables only (docs site, third parties)
│   ├── draft.js                   # SSR progressive enhancement (theme, swatches, toggles, copy, editor gutter)
│   ├── boot.js                    # ≤10 lines, inlined in <head>: restores theme/primary before first paint
│   ├── fonts/                     # Inter + JetBrains Mono woff2 (subset)
│   └── vendor/htmx.min.js         # SSR services only — never referenced by the Rust side
├── src/                           # Rust (Dioxus 0.7)
│   ├── lib.rs  assets.rs  kinds.rs  theme.rs
│   ├── shell/    (AppShell Rail Topbar Crumbs StatusBar AppLinks chrome)
│   ├── layout/   (PageHead Toolbar Split Drawer DrawerBlock SectionTitle)
│   ├── ui/       (Btn Seg Toggle Chip Tag Status Dot Glyph Count Kbd Field Input Select Textarea
│   │              Modal Alert Toast Menu Empty Loading)
│   ├── data/     (StatTile Kv CodeBlock Strip Meter Progress DurationCell List Board Card)
│   ├── query/    (QueryBar QueryChips QueryBuilder grammar completer)  # see §3.8
│   ├── viz/      (Sparkline Histogram TimeSeriesChart Waterfall Wire Via Pad)
│   ├── util/     (domain, format, secret-mask helpers)
│   └── bin/draft-ui-css.rs        # feature `cli`: scss → dist/  ·  `--check` fails on drift (CI)
├── templates/                     # Go html/template partials (shell, page-head, stat, tag, status, strip, kv, code, empty…)
├── preview/                       # design-system.html + component gallery, linked to ../dist/draft.css
└── tests/                         # contrast · class-contract · ssr-render · scss-compiles
```

Cargo and Go ignore each other's files; both embed the same `dist/` and `templates/`. This is the same local-`replace` arrangement every Go service in this repo already uses for `pkg/chassis` and `api`.

### 3.2 What each consumer gets

| Consumer | Stack | Consumes | How |
|---|---|---|---|
| Blueprint, Beacon, Lineman | Dioxus/WASM | `draft.css` + Rust components | `draft-ui = { path = "../../../../tools/draft-ui", features = ["dioxus"] }` (same shape as the existing `dioxus-grpc` path dep); CSS via `asset!()` so `dx` fingerprints it |
| Bench, Foundry | Go SSR (`html/template` + htmx) | `draft.css`, `draft.js`, fonts, htmx, `templates/` | `require github.com/steady-bytes/draft/tools/draft-ui` + `replace … => ../../../tools/draft-ui`; static assets served by `draftui.Static()` |
| Docs site | Hugo + Bootstrap SCSS | `scss/abstracts/_tokens.scss` | Hugo `module.mounts` (Phase 6) |
| Future apps (e.g. `fitness-application`) | Tailwind v4 compiled | `draft-daisyui.theme.css` | `@plugin "daisyui/theme"` blocks; no code copying |

### 3.3 SCSS and the build

* **Compiler: `grass`** (pure-Rust Sass). This machine has `cargo` but no `sass`/`just`, so this needs no new toolchain. **Spike result (2026-09-26): grass compiled the mockup's `draft.css` unchanged — 163/163 rule blocks, 13/13 `color-mix()` calls and 276/276 `var()` references preserved — and handled `@each`, mixins and interpolation.**
* `cargo run -p draft-ui --features cli --bin draft-ui-css` writes `dist/`. `--check` regenerates in memory and fails on any diff; CI runs it so the committed `dist/` (needed because Go's `go:embed` cannot build CSS) can never drift.
* Maps do the repetitive work the mockup writes out by hand:

```scss
// abstracts/_tokens.scss
$tones: (primary: var(--primary), ca: var(--ca), bp: var(--bp), fs: var(--fs),
         sv: var(--sv), err: var(--err), warn: var(--warn), quiet: var(--dimmer));

// components/_tag.scss — replaces 8 hand-written modifier rules
@each $name, $value in $tones {
  .d-tag--#{$name} { --c: #{$value}; }
}
```

### 3.4 daisyUI configuration (the requirement)

Three forms, all generated from the same `$themes` map so they cannot disagree:

1. **`draft.css` `[data-theme]` block** — daisyUI 5 `--color-*` mapping (`base-100/200/300`, `primary`, `secondary`, `accent`, `info/success/warning/error` + `-content`, radii, `--depth: 0`, `--noise: 0`). Works with daisyUI from a CDN *or* a compiled build. This is the block the SSR services use if they ever want a daisyUI component that has no `d-*` equivalent yet.
2. **`draft-daisyui.theme.css`** — literal-valued `@plugin "daisyui/theme" { name: "draft"; default: true; color-scheme: dark; … }` and `draft-light`, for Tailwind v4 compiled pipelines (the pattern in `.claude/skills/dioxus-ui/references/daisyui.md`). Lets any steady-bytes project adopt the theme without copying tokens.
3. **`tokens.css`** — variables only.

Policy: shipped views use `d-*` classes. A daisyUI component class may be used only if no `d-*` equivalent exists **and** a follow-up issue is filed to add one. The class-contract test (§3.7) reports remaining daisyUI/Tailwind usage per app so Phase 7 has a hard exit criterion.

### 3.5 Rust component library

Rules (from `.claude/skills/dioxus-ui`): one component per file; variant props are `Copy + PartialEq + Default` enums mapped to class strings inside the component; every prop type derives `PartialEq`; computed strings are bound to a `let` before `rsx!` (no expressions in format strings). The crate is **data-agnostic** — props in, events out. It never imports `draft-api` except behind the `discovery` feature.

| Component | Replaces | Notes |
|---|---|---|
| `AppShell { app, current, sections, children }` | Blueprint `dashboard_layout` + `navbar.rs`, Beacon navbar, Lineman drawer | rail, topbar, status bar, responsive rail (checkbox toggle, no JS); nav items are data (`NavItem { label, path, count, ext }`); a section with no items is not rendered, so adding one later is a data change, rendered as `Link { to: path }` so the crate does not depend on any app's `Route` enum |
| `use_page_chrome(Chrome { crumbs, status, statusbar })` | — | how a view sets its own breadcrumb / status pill / status bar from inside a route layout (context signal read by `AppShell`) |
| `AppLinks` + `use_app_links()` (feature `discovery`) | Blueprint's inline Fuse `ListRoutes` discovery in `main.rs` | builds the rail's **Apps** block; excludes the current app; Beacon and Lineman get it for free |
| `NavConfig` loader (feature `discovery`) | Blueprint's `ui/navigation` KV fetch | optional; Beacon/Lineman pass static sections |
| `PageHead`, `Toolbar`, `SectionTitle`, `Split`, `Drawer`, `DrawerBlock` | 12 per-page layouts, `LogDetailDrawer` chrome, `ConfigDrawer` chrome, `KeyValueDetail`/`RouteDetail`/`ServiceDetail` page chrome | `Split` takes an `Option<Element>` drawer; below 1100 px the drawer becomes a right overlay |
| `Btn`, `Seg<T>`, `Toggle`, `Chip`, `Count`, `Kbd` | `TimeRangePicker`, List/Flame toggles, daisyUI `btn`/`join`/`toggle` | one generic `Seg<T: SegValue>` serves time ranges, view switches, sort, domain filter, Board/List |
| `Tag`, `Status`, `Dot`, `Glyph` + `kinds.rs` (`AppKind`, `TagKind`, `StatusKind`) | `TypeBadge`, `AuthBadge`, `ProtocolBadges`, `ValidationDot`, `severity_badge`, `priority_badge`, `agent_badge`, `.signal` | domain mappings (protocol → tag, severity → tag, priority → tag) stay as 5-line functions in the owning app |
| `Field`, `Input`, `Select`, `Textarea`, `Check` | daisyUI `input-bordered`/`select-bordered`/`label` (54 + 31 + 15 uses) | |
| `Modal`, `Alert`, `Toast`, `Menu`, `Empty`, `Loading` | Blueprint "Add entry" modal, `alert` (15 uses), Lineman `toast`, cluster `CtxMenu` chrome, `WaveLoader` | not in the mockups — designed in the same language (§5.3) |
| `StatTile { label, value, unit, delta, spark }` | `MetricCard` ×2, daisyUI `stat` | |
| `Kv`, `CodeBlock { lang }`, `Strip`, `Meter`, `Progress`, `DurationCell`, `List`/`ListRow` | ad-hoc `<dl>`s, JSON `<pre>`s, `severity_stripe_color`, per-page bars | `CodeBlock` ships a small JSON/YAML/HTTP tokenizer (`tk-*` classes) so the KV drawer, CloudEvent drawer, workflow snippet and step request/response share one highlighter |
| `Board`, `BoardColumn`, `Card` | Lineman card markup | |
| `QueryBar { grammar }`, `QueryChips`, `QueryBuilder { grammar }` | `CesqlBar`, Beacon `QueryBar`, `FilterChips`, both `QueryBuilder`s, `WideEventQueryBuilder` | `grammar` is a `Grammar` value (§3.8), not an enum: CESQL, BeaconQL × 3 datasets, PromQL subset, plain search. Chips come from the grammar, so a chip can never emit a construct the backend rejects |
| `Sparkline`, `Histogram`, `TimeSeriesChart`, `Waterfall`, `Wire`/`Via`/`Pad` | `SeverityHistogram`, `WideEventHistogram`, Beacon `TimeSeriesChart`, Blueprint `metrics.rs` chart, `flame_graph.rs` (trace use), cluster `WirePads`/`ViaDots` | `Histogram` is stacked buckets (`[(label, [(kind, value)])]`); `Waterfall` serves Traces and the wide-event drawer |
| `ThemeProvider`, `use_theme()`, `ThemeToggle`, `PrimarySwatches` | — | persistence keys `draft.theme` / `draft.primary` (same as the mockup's `draft.js`) |
| `NotFound` | 3 copies | |
| `util::{resolve_domain, format_duration_ns, format_bytes, short_type_name, truncate}` | `get_domain()`/`API_DOMAIN` ×3, `format_duration_ns`, `short_kind_name`, `truncate_preview` | `option_env!` must expand in the app crate, so the app keeps `option_env!("API_DOMAIN")` and passes it in |

### 3.6 Go SSR kit (`package draftui`)

Same CSS contract, second renderer — so Bench and Foundry get the same shell and components without a Rust dependency.

```go
var app = draftui.App{Code: "Bn", Name: "bench", Kind: draftui.KindService, Rail: benchRail}

// one call replaces templates.go's per-page ParseFS lists:
var pages = draftui.MustPages(app, templatesFS, "templates/*.html")

type overviewPage struct {
    draftui.Page              // Title, Crumbs, Current, Status, StatusBar — replaces the ad-hoc `Nav string`
    Stats []draftui.Stat
    // …
}
```

* `draftui.Static()` → `http.Handler` for `/static/draft/…`: CSS, JS, fonts and htmx, with content-hash `ETag` + `Cache-Control`. Replaces both `static.go` files and both vendored `htmx.min.js`.
* `shell` template with named blocks `title`, `crumbs`, `status`, `content`, `statusbar`; partials for `d-page-head`, `d-stat`, `d-tag`, `d-status`, `d-strip`, `d-kv`, `d-code`, `d-empty`, `d-drawer`, called with a `dict` helper (html/template has no parameterised includes).
* Funcs: `dict`, `dur`, `ago`, `spark` (values → SVG path `d`), `strip` (statuses → cells), `count`.
* htmx is a **Go-SSR-only** asset, per the standing rule that Blueprint's UI never uses htmx. The Rust crate never references it.
* Per-page fragments used by htmx (`run-detail-fragment`, `plugin-search-results`, `card-grid`) keep their existing "parse the page file alone" pattern.
* App-to-app links: derived from the request host by the same convention Blueprint already uses (`<name>.draft.localhost` + current port) with a config override. Replaces the hard-coded `http://localhost:9301/` and `:9300/` in the two `base.html` files.

### 3.7 Tests that protect the system

| Test | Purpose |
|---|---|
| **contrast** (Rust, native) | For both themes × 5 primary swatches, asserts ≥ 4.5 : 1 for every text-token/surface pair the CSS uses, and ≥ 3 : 1 for non-text UI. Encodes the F3 fixes |
| **class-contract** | Scans `.rs` (`class: "…"`), `.html`/`.tmpl`, and Go template files for `d-*` names and fails if any is undefined in `dist/draft.css`; also **reports** daisyUI/Tailwind classes still used per app (Phase 7 gate) |
| **scss-compiles / drift** | `draft-ui-css --check` |
| **ssr-render** | `dioxus-ssr` renders `Tag`, `Status`, `StatTile`, `AppShell`… to strings and asserts classes/ARIA — markup regression tests with no browser |
| **Go render** | `httptest` per page; update `foundry/ui_test.go` (it asserts `hx-get="/partials/catalog-cards"`, which must survive) |

### 3.8 Query grammar and future autocomplete

**What the builder must cover** (read from the current code, not assumed):

| Grammar | Dataset and fields | Today's builder | Backend accepts |
|---|---|---|---|
| CESQL (Catalyst) | `type`, `source`, `id`, `subject`, `body.<path>` | `=`, `LIKE` only; value interpolated as `'{value}'` **without escaping** (a value containing `'` yields a broken query) | AND / OR / XOR / NOT, all comparisons, arithmetic |
| BeaconQL: logs | 8 fields; map fields `attributes[k]`, `resource_attributes[k]`; numeric `severity_number`; `severity` aliases `severity_text` | `= != < <= > >= LIKE NOT LIKE IN NOT IN` | same |
| BeaconQL: traces | `trace_id`, `service_name`, `span_name`, `status_code`, `start_time`, `duration_ns`, synthetic `duration_ms` (scaled to ns), `attributes[k]` | none | same parser, different column map |
| BeaconQL: wide events | 8 scalar fields + `attributes[k]`, `business_attributes[k]`, `runtime_attributes[k]` | separate 355-LOC builder | same parser, different column map |
| PromQL subset | metric name, `{label="v"}` / `{label!="v"}`, `rate()`, `sum/avg/max/min by (…)` | none (bar only) | own parser. **No `histogram_quantile` or `topk`**, which the mockup's chips show |

The four predicate grammars share one shape (field, operator, value, connector) and one builder covers them. PromQL is structured differently (metric → matchers → function → aggregation) and is deliberately **not** forced into the predicate model.

**Three parts, so autocomplete is an addition and not a rewrite:**

1. **`Grammar`** — the schema. Fields, kinds, operators per kind, literal syntax, the combine rule, and the chips.
2. **Compiler** — structured selection → fragment, plus `combine()`. Carries the Beacon rule that OR is the loosest binder, so appending `AND x` to an existing expression wraps it in parentheses first. The builder form uses this.
3. **`Completer`** — reads `(text, cursor)`, decides what is expected next, and suggests from the schema plus **value providers**. The builder form is the same thing with a structured cursor, so parts 1 and 2 are shared.

```rust
pub struct Grammar {
    pub id: GrammarId,                       // Cesql | BeaconLogs | BeaconTraces | BeaconWideEvents | PromQl
    pub fields: Vec<FieldSpec>,
    pub literal: LiteralStyle,               // SingleQuote | DoubleQuoteBackslash
    pub combine: CombineRule,                // OrLoosest (parenthesise before AND) | Flat
    pub chips: Vec<Chip>,                    // only constructs the backend supports
}
pub struct FieldSpec {
    pub name: &'static str,                  // "duration_ms"
    pub aliases: &'static [&'static str],    // "severity" → severity_text
    pub kind: FieldKind,                     // Text | Number | Duration{unit} | Timestamp | Enum | Bool | Map{value} | Path
    pub ops: Option<Vec<Op>>,               // None → defaults for the kind
    pub values: ValueSource,                 // None | Static(&[..]) | Remote(ValueQuery)
    pub doc: &'static str,
}
pub enum Expect { Field, MapKey(FieldRef), Operator(FieldRef), Value(FieldRef), ListItem(FieldRef), Connector, Nothing }
pub trait Completer { fn expect(&self, text: &str, cursor: usize) -> Expect;
                      async fn suggest(&self, e: &Expect, prefix: &str) -> Vec<Suggestion>; }
```

**Staging.**
* **Now (Phase 2):** `Grammar`, `FieldSpec`, the compiler and `combine()`, four predicate grammars as compiled-in tables, `ValueSource::Static`, and the `Completer` trait defined but not implemented. Unit tests port both existing builders' outputs before the old code is deleted. Fixes: CESQL value escaping; CESQL gains `!= < <= > >= IN` (its evaluator already supports them). ORDER BY is not part of any query language, so it moves out of the builder into a "Newest first / Oldest first" `Seg` in the results toolbar (the mockup's status bar already says "Newest first").
* **Later:** a tokenizer implementing `Completer`, a combobox dropdown on `QueryBar` (ARIA `combobox`/`listbox`, keyboard navigation), and the backend support below. Cursor-context detection starts **client-side** (instant); the schema and values come from the server.

**Backend support autocomplete needs** (tracked in §7): a `DescribeQuerySchema` RPC so the client loads field lists instead of hand-copying Go's `columnFor*` switches (today they can drift), value-lookup RPCs (distinct `service_name`, attribute keys, event types, metric and label names), and a position on `ParseError` (it carries only `Msg`, and lexer tokens carry no offsets, so inline error markers are impossible today).

**Honest limits.** The builder appends clauses; it cannot edit an existing nested expression. That is what autocomplete on the raw text is for, so the builder is the guided on-ramp and autocomplete the long-term editing path. CESQL functions, arithmetic and XOR stay free-text.

---

## 4. Decisions (defaults chosen; veto any)

| # | Decision | Default | Why / alternative |
|---|---|---|---|
| D1 | Name and location | `github/draft/tools/draft-ui`, crate `draft-ui` | You asked for `tools/`; trivially renamed |
| D2 | Go binding location — **confirmed** | Same directory as the crate (`go.mod` beside `Cargo.toml`) | `go:embed` cannot reach outside its module, so a sibling module could not embed `dist/`. Alternative: `dctl ui sync` copying files into each service — reintroduces the duplication we are removing |
| D3 | Sass toolchain | `grass` (Rust) | Verified by spike; no Node. Fallback: `sass` from npm |
| D4 | Commit `dist/` | Yes, with CI drift check | Go embed needs the file present without a Rust toolchain. Alternative: build in each Go service's Dockerfile (slower, couples toolchains) |
| D5 | daisyUI role | Theme configuration only; not a dependency of migrated views | Matches F1. Alternative: keep daisyUI components as a runtime dependency — the SSR services would have to keep a CDN or 780 KB vendored file |
| D6 | Detail routes | Keep every existing route as a deep link, render list + drawer | Bookmarks and cross-view links (`PENDING_KV_KIND`, `PENDING_TRACE_ID`) keep working |
| D7 | Data-less mockup elements | Omit until data exists; tracked in §7 | Avoids fabricating numbers on screen |
| D8 | `pkg/basic_authentication` login/register templates — **confirmed: restyle** | Restyle the three starter templates onto `d-*` (centred `d-panel` card, wordmark, fields, error `Alert`), neutral default title instead of "Golf Tracker", and add an `auth-card` partial to `draftui`. README gains a note that the consuming service must mount `draftui.Static()` | No Draft service imports the package. The golf app uses **its own copy** (`replace … => ../../../pkg/basic_authentication` resolves inside `golf-tracker-back-end`), and the README tells consumers to copy and modify the templates, so restyling Draft's copy has no blast radius. (An earlier draft of this plan wrongly said other repos consume it.) No mockup exists; the card is composed from existing components |
| D9 | `mockups/` | Stays as the frozen design reference; `mockups/draft.css` deleted and pages re-linked to `tools/draft-ui/dist/draft.css` | One stylesheet, no drift. Page-local inline styles stay (they are the mockups) |
| D10 | Query builders — **confirmed** | One grammar-driven predicate builder behind a "Filter…" chip; autocomplete later on the same schema, client-side cursor-context detection first; server supplies schema and values | See F5.1 and §3.8. Alternative considered: server-side completion via the real parsers (always accurate, but a round trip per keystroke) — revisit if client-side detection drifts |
| D11 | Lineman drag-and-drop — **confirmed** | Included, staged last in the Lineman phase: "Move to…" menu, then cross-column drag, then within-column reorder; not part of the pilot exit criterion | Cards with pending `needs_input` are not draggable; agent-owned cards can be moved with an Undo toast. See §6 Phase 4.1 |
| D12 | Bench "Webhook secrets" and Beacon "Saved queries" rail items — **confirmed** | Omitted; tickets filed (§7 items 6 and 15) | The mockups link both to `#`, so no page design exists. Secrets rotation is security-sensitive and deserves its own design and review. The shell already hides empty sections |

---

## 5. Design-system work beyond copying the mockup

### 5.1 Shell responsiveness (new)

* ≥ 1100 px: as mocked.
* 720–1099 px: rail collapses; `☰` in the topbar toggles it as an overlay; `.d-split` drawer becomes a right-side overlay (`width: min(420px, 100%)`).
* < 720 px: single column; tables scroll inside `.d-table-wrap`; page-head actions wrap.
* Implemented as a **CSS-only checkbox toggle** (`<input type="checkbox" id="d-rail-toggle" class="d-sr">` + `<label>`), identical markup in Rust and Go — no JS, works in SSR.

### 5.2 Page-local mockup CSS hoisted into shared classes

| Pattern | Mockup pages that repeat it | Shared class |
|---|---|---|
| External-app rail link (↗) | all 17 | `d-nav-item--ext` |
| Primary-coloured rail action ("+ New workflow") | 2 (inline `style`) | `d-nav-item--action` |
| List + fixed-width drawer, bleeding to the edges of `main` | kv gw eq lg we tr mx tp ed rn pd cl | `d-split` (+ `--wide` 420 px, `--narrow` 340 px) |
| Drawer heading / section block | kv gw eq lg we tr tp cl | `d-drawer-title`, `d-block` |
| Toolbar row | kv gw sr bn fc ob eq | `d-toolbar` |
| Inline page head (h1 + label + segmented controls) | lg we tr mx tp eq m | `d-page-head--inline` |
| Code/JSON block with token colours | kv eq fp ed rn | `d-code` + `tk-key/str/num/val/com/url/ph/err` |
| Bar strips | bench `.runs`, registry `.inst`, run `.hist` + `.rn-progress` | `d-strip` + `is-ok/err/run/none/skip/cur` |
| Coloured dot / service label | eq lg we tr | `d-dot`, `d-svc` |
| Duration cell with bar | we tr | `d-dur` |
| Waterfall rows and lanes | we (drawer) tr | `d-wf`, `d-wf-row`, `d-wf-lane`, `d-wf-bar` |
| Chart body, hover tooltip, legend | mx m | `d-chart`, `d-chart-tip` (legend reuses `d-legend`) |
| Simple list rows | cl fp tp ed mx | `d-list`, `d-list-row` |
| Facet chip with count | eq | `d-chip[aria-pressed]` + `d-count` |
| Meter / share bar | m tp | `d-meter` |
| Card grid, `uses:` code chip, dashed "+ Add" card | fc ob | `d-card-grid`, `d-uses`, `d-add` |
| Status in info colour (inline `--c`/`color` override, 5×) | bench | `d-status--info` |
| Verdict lamp line | bn | `d-verdict` |
| Vertical step trace / horizontal pipeline / failure card | rn / ed / rn | `d-trace`, `d-pipe`, `d-fail` |
| SVG wire / via / pad / event-flow | cl gw ed | `d-wire`, `d-via`, `d-pad`, `d-evt` |
| Expandable row, group header row, column sizing | sr / gw / many | `d-caret` + `is-child`, `is-group`, `is-fill`, `is-key` |
| Code editor (gutter, line highlight, squiggle) | ed | `d-editor` |

Stays page-local: cluster stage geometry (`.cl-*`) → Blueprint `assets/cluster.css` (replaces the `CLUSTER_CSS` string constant); gateway flow-diagram box styles; topology board columns.

### 5.3 Components the apps need that the mockups do not draw

`Modal`, `Alert`/error banner, `Toast`, context `Menu` (cluster right-click), `Textarea`, checkbox/radio, field label + hint + error state, `<details>` disclosure (`d-details`), `Empty` state, `Loading` (keeps the wave loader), focus and disabled states for every control. Each follows existing tokens (1 px rules, `--radius` 2 px, mono labels) — no new visual vocabulary. **Review these once in the gallery (Phase 1) before any app depends on them.**

---

## 6. Implementation plan

Effort: **S** ≈ ½ day, **M** ≈ 1–2 days, **L** ≈ 3+ days. Every phase merges independently and leaves every app working. Order is chosen so the riskiest unknowns (Phase 0) and the smallest real consumers (Foundry, then Lineman) prove the kit before Blueprint — the largest — depends on it.

### Phase 0 — Spikes and data audit (S)

Prove the four things the plan assumes; adjust the plan if any fail.

1. **`asset!()` from a library crate.** Scaffold `tools/draft-ui` with one component that renders `document::Stylesheet { href: asset!("/dist/draft.css") }`; depend on it from Lineman; `dx build --release` and confirm the CSS is fingerprinted into `…/public`. *Fallback:* `include_str!` + `document::Style`, or a copy step into `public/`.
2. **`Link` with string targets from a library crate**, and the dependency shape (`dioxus` vs `dioxus-lib`, `router` feature). Confirm `dx` 0.7.9 builds it for `wasm32-unknown-unknown` (target is installed).
3. **Theme cookie across subdomains** (`bench.draft.localhost` ↔ `blueprint.draft.localhost`). *Fallback:* per-origin storage.
4. **Data audit — confirm every VERIFY item in §7** (Service `group`/`process_kind` usable for the Core/Tooling/Plugin/Example filter; whether `ListRoutes` carries an Envoy config version; whether topology edges carry rates).

Exit: a written result for each spike appended to this document.

#### Phase 0 results (2026-09-26)

| Spike | Result | Consequence |
|---|---|---|
| 1. `asset!()` from a library crate | **Works.** A scratch app depending on `draft-ui` built with `dx build --release --platform web`; `dx` copied `tools/draft-ui/dist/draft.css`, fingerprinted it (`draft-dxh….css`) and minified it (24 KB → 17 KB). | `DraftStyles` uses `asset!` as planned; no fallback needed |
| 2. `Link` with string targets, dependency shape | **Works.** `Link { to: String }` and `dioxus::router::router().full_route_string()` compile natively and for `wasm32-unknown-unknown`. Dependency is `dioxus = { version = "0.7", default-features = false, features = ["lib", "router"] }` (`lib` = macro, html, signals, hooks, document, asset). The clients lock different patch versions (Blueprint 0.7.6; Beacon and Lineman 0.7.10), so the crate must not pin a patch. | Shell nav items are string paths; no app `Route` coupling. Re-verify against Blueprint's 0.7.6 lock when it adopts the crate |
| 3. Theme cookie across subdomains | **Works.** In Chrome, `document.cookie = "draft.theme=…; Domain=draft.localhost"` set on `a.draft.localhost` was visible on `b.draft.localhost`. | `draft.js` and `ThemeProvider` write a parent-domain cookie (host minus its first label, only when the host has ≥ 3 labels) and fall back to host-only + `localStorage` when the browser rejects it (public suffixes) |
| 4. Data audit | see items 10–14 and 20 in §7 | Below |

Data audit outcomes: **10** Raft term, commit and per-node role are exposed by no RPC (only the KV `leader` key exists) → topbar shows "Raft · N nodes · leader X" and omits term/commit. **11** `ProcessKind` is only SERVER/JOB and nothing in `pkg/chassis` sets `Service.group`, so Core/Tooling/Plugin/Example is derived client-side from a name table (a non-empty `group` overrides it). **12** `Route` carries no config version → "envoy config vN" omitted. **13** No delivery status exists on events → omitted. **14** `TopologyEdge.vol` exists (per producer/consumer/event-type volume) → rates are derivable; use `GetMetrics` for windowed rates. **20** `Task` has no link to a `Loop` (only `ScheduledTask.created_task_id` and `Loop.occurrence_count`) → the Lineman loop card is built from the `Loop` message (recurrence, next fire, "×N fired") and does not group task instances.

### Phase 1 — `draft-ui` foundations (M)

1. Scaffold `Cargo.toml`, `go.mod`, `embed.go`, README, `src/bin/draft-ui-css.rs`.
2. Split `mockups/draft.css` into the SCSS tree in §3.1, **byte-for-byte equivalent first** (diff `dist/draft.css` against the mockup CSS with whitespace normalised), then apply changes in separate commits so each is reviewable:
   * a. tokens into `$themes`/`$tones` maps, tag/status modifiers via `@each`;
   * b. F3 fixes (colour values, `--dimmer` text re-pointing, `<button role="switch">` styles, 62 % derivation);
   * c. hoist the §5.2 patterns; add §5.3 components;
   * d. responsive shell (§5.1);
   * e. self-hosted fonts behind `$self-host-fonts`;
   * f. emit `draft-daisyui.theme.css`, `tokens.css`, `draft.min.css`.
3. `dist/draft.js` from the mockup's `draft.js` plus copy button, `[data-confirm]`, editor gutter hooks; `dist/boot.js`; vendor htmx once.
4. Port `design-system.html` to `preview/` as a **component gallery** with every component in every state (default, hover, selected, disabled, focus, empty, loading, error) in both themes and all five swatches. This is the review artifact for §5.3.
5. Tests from §3.7 (contrast, drift, scss-compiles); CI job.
6. Re-link `mockups/pages/*.html` to the crate's `dist/draft.css`; delete `mockups/draft.css`.

Exit: gallery reviewed and signed off; `cargo test -p draft-ui` green; `draft-ui-css --check` green.

### Phase 2 — Rust component library (L)

Build bottom-up so each layer is testable before the next uses it. Each component gets an `ssr-render` test and a gallery entry.

1. **Primitives:** `Tag`, `Status`, `Dot`, `Glyph`, `Count`, `Kbd`, `Btn`, `Seg<T>`, `Toggle`, `Chip`, `Field`/`Input`/`Select`/`Textarea`/`Check`, `Alert`, `Empty`, `Loading`, `Modal`, `Toast`, `Menu`, `NotFound`.
2. **Layout + shell:** `PageHead`, `Toolbar`, `SectionTitle`, `Split`, `Drawer`, `DrawerBlock`, `Rail`, `Topbar`, `Crumbs`, `StatusBar`, `AppShell`, `use_page_chrome`, `ThemeProvider`/`ThemeToggle`/`PrimarySwatches`, `kinds.rs`.
3. **Data:** `StatTile`, `Sparkline`, `Kv`, `CodeBlock` (+ tokenizer with unit tests on JSON/YAML/HTTP samples), `Strip`, `Meter`, `Progress`, `DurationCell`, `List`, `Board`, `Card`.
4. **Query (see §3.8):** `Grammar`/`FieldSpec`, compiler + `combine()`, `QueryBar`, `QueryChips`, `QueryBuilder`. Four predicate grammars as compiled-in tables (CESQL, BeaconQL logs / traces / wide events) with unit tests that reproduce the fragments the old Blueprint and Beacon builders generate, written **before** the old code is deleted. Define the `Completer` trait and `ValueSource::{Static, Remote}` but do not implement completion. PromQL gets a `Grammar` for chips and placeholder only. Fix CESQL value escaping and add the operators its evaluator already supports.
5. **Viz:** `Histogram`, `TimeSeriesChart` (crosshair tooltip, toggleable series — port the mockup's `beacon-metrics.html` script), `Waterfall`, `Wire`/`Via`/`Pad`.
6. **Feature `discovery`:** `use_app_links()` (Fuse `ListRoutes`, ported from Blueprint's `main.rs`), `NavConfig` loader (`ui/navigation` KV).
7. **`util`:** domain resolution, duration/bytes formatting, `short_type_name`, `truncate`.

Exit: `cargo check --target wasm32-unknown-unknown -p draft-ui --features dioxus,discovery`; gallery renders every component; no `draft-api` import outside `discovery`.

### Phase 3 — Go SSR kit and Foundry (M) — first real consumer

1. Implement `draftui` (§3.6): `Static()`, `App`, `Page`, `MustPages`, `shell` + partials, funcs, and render tests.
2. **Foundry**, replacing `base.html`, `static.go`, `templates.go` and `htmx.min.js`:

| Page | Change | Data notes |
|---|---|---|
| Shell | rail: **Catalog** (All plugins + count, Recently published), **Maintainers** (derived from data, links to `?maintainer=`), **Publish** (links to docs), **Apps**; topbar status "N plugins · M versions"; status bar (catalog address, last publish) | last publish = max `published_at` |
| Catalog `/` | `d-query` (lang `SEARCH`) keeping the existing `hx-get="/partials/catalog-cards"`; sort `Seg` (A–Z, Newest); `d-card-grid` of plugin cards: glyph (a small name → code/kind table matching the mockup: `catalyst-*` → Ca, `lineman` → Lm, `slack-notify` → Sn, fallback `Pl`), name, version tags (all versions, latest in primary), description, `uses: foundry://name@version` chip, footer (maintainer, "N required") | "Most used" sort, category chips, "N workflows" → §7, omitted |
| Plugin `/plugins/{name}/{version}` | head meta (maintainer, published, source ↗); version `Seg` (links); config-schema `d-table` with **Description** and default columns and `required`/`optional` tags + "N required · M optional"; snippet panel with Copy; Versions list | extend `schemaFields` to read `description`/`default` from the JSON Schema already stored (no proto change). Address/Serving/Used-by → §7, omitted |
| 404 | `Empty` | |

3. Update `ui_test.go` expectations; add a render test per page.

Exit: `go test ./...` in `services/tooling/foundry`; screenshot parity with `foundry-catalog.html` and `foundry-plugin.html` at 1440 px in both themes; Play CDN and daisyUI 4 gone from Foundry.

### Phase 4 — Dioxus clients

**Stage A (per app, first commit, ~1 hour):** token swap — add `draft.css`, set `<html data-theme="draft">`, keep the daisyUI/Tailwind scripts. Every unmigrated view adopts the palette immediately (this is the swap the mockup's own comment describes). Ship it so the apps look coherent while views are migrated.

**Stage B (per app):** replace the shell with `AppShell`. **Stage C:** migrate views one PR each.

Common to every view PR: uses only `d-*` classes and shared components; no hex literals; both themes; loading, empty and error states present; keyboard reachable; existing route kept; screenshot parity with the mockup at 1440 px and 900 px; old components it replaced deleted **in the same PR**.

#### 4.1 Lineman (M) — pilot

Smallest client (1,444 LOC); one mockup; five unmocked views, so it exercises the "no mockup" path too.

| View | Change |
|---|---|
| Shell | `AppShell`, app `Lm`. Rail: **Overview** (Dashboard) · **Objectives** (live list from `ListObjectives`, count per objective, "+ New objective" as `d-nav-item--action`) · **Automation** (Scheduler, Loops, counts) · **Apps**. Topbar status "N agents online" from `ListAgents`. Status bar: objective id, last `Watch` event age, Catalyst connection state |
| `ObjectiveDetail` + `TaskBoard` → **one view** at `/objectives/:id` (`/objectives/:id/board` kept as alias) | Page head with `Seg` Board ⇄ List and "+ Add task" opening a `Modal` (replaces the inline Add Task card). `Progress` + stat row (Total / Done / In flight / Queued / High-priority open) derived from task states. Toolbar: name filter, Priority and Assignee `Select`s. `Board`: column head dot + count; card = priority `Tag`, short id, title, meta row, agent `Avatar`. Keeps the existing `Watch` stream and upsert logic unchanged. Card "Move to…" `Menu` (keyboard and touch accessible; reuses the `UpdateTaskState` call `TaskDetail` already makes). **Drag-and-drop, staged as the last PRs of this phase (D11):** ① "Move to…" ships with the board; ② cross-column drag via `UpdateTaskState`; ③ within-column reorder via `ReorderTask` (the board must first sort by `order`, which it does not today). "+ Add task" dashed card per column |
| Column colour rule | states are free-form per objective, so: first state → `--dim`, last → `--primary`, any state matching `flight\|progress\|running\|active` → `--ca`, others cycle neutral. Pure function with unit tests |
| Loop card (`d-card--group`) | Phase 0 found no task→loop link, so this is built from the `Loop` message (recurrence text, next fire, "×N fired") in the first state's column, from `ListLoops`; task instances are **not** grouped and Expand is omitted |
| `Dashboard` | (no mockup) `StatTile` row + `d-card-grid` of objectives with `Progress` and counts |
| `CreateObjective`, `Scheduler`, `Loops`, `TaskDetail` | (no mockup) `PageHead` + `Panel`/`Field`/`Btn`/`d-table`/`Status`/`Tag`; creation flows in `Modal`; `toast` → `Toast`; `TaskDetail` uses `Split` with the task facts in the drawer |
| Delete | `components/mod.rs` helpers (`priority_badge`, `agent_badge`, `toast`), `not_found.rs`; keep `parse_datetime_local`, `agent_kind_label` |

Exit: `dx build --release` for Lineman; screenshot parity with `lineman-board.html`; Lineman on `AppShell` only. **Drag-and-drop is not part of the pilot's exit criterion** — steps ②–③ land after it, so they cannot hold up the shared kit.

Drag-and-drop rules (D11): cards with a pending `needs_input` are **not draggable** (`UpdateTaskState` clears `needs_input` on any state change, which would silently discard the agent's question; these are answered from Task Detail). Moving a card owned by an agent is allowed as a human override (`UpdateTaskState` leaves `agent_id` alone) and shows a toast with **Undo** (a move back). Drops apply optimistically and roll back on error. A drop at a specific position in another column is two non-atomic calls (`UpdateTaskState`, then `ReorderTask`), so the UI applies both locally first and tolerates the intermediate `Watch` event. The "Move to…" menu is the WCAG 2.2 non-drag alternative and must work without a pointer.

#### 4.2 Beacon (M)

| View | Change |
|---|---|
| Shell | `AppShell`, app `Bc`. Rail: **Signals** (Wide events, Logs, Traces, Metrics) · **Apps**. **Saved queries** section is not built (§7) — omitted. Topbar status "ClickHouse · N events/min". Move the `drawer-slide-in` keyframes from `index.html` into the shared drawer overlay CSS |
| Wide events `/` (498 LOC) | Inline page head; `Seg` view (List / Flame graph; "Group by" → §7) + time-range `Seg`; `QueryBar(BeaconQl)` + `QueryChips` + Filter… builder; `Histogram` (volume by status, error cap min 3 px as in the mockup); table with `d-svc`, `DurationCell`, `Status`, truncated `attr-k` attributes; `Drawer`: status/time, title, spans as compact `Waterfall`, attributes `Kv`, logs `Kv` (severity-coloured keys), actions Open trace (`PENDING_TRACE_ID`) / Logs ±50 / Similar events (prefills query from the row's route or service) |
| Logs `/logs` (431 + `log_detail` 416) | Live-tail `Toggle`; `Histogram` by severity; table; `LogDetailDrawer` → `Drawer` with readable trace link pill (`TracePill` becomes a `Tag`-styled link) |
| Traces `/traces` (250) | Trace-list table (root span, service dots, span count, duration, status, started); **`Waterfall`** replaces `FlameGraph` here, retries drawn as separate bars on one timeline; span `Drawer` (attributes, events, related links to Logs / Bench run / Gateway route); "Errors only" `Toggle` |
| Metrics `/metrics` (184) | `QueryBar(PromQl)` + chips; four derived `StatTile`s (Total now, Peak, Series, Samples — all computed client-side); `TimeSeriesChart` with crosshair and toggleable series; series table (Last/Min/Avg/Max); Live `Toggle` (interval refresh). **Metric browser** drawer needs a metadata RPC (§7) — omitted until then |
| Delete | `metric_card.rs`, `query_builder.rs`, `wide_event_query_builder.rs`, `query_bar.rs`, `time_range.rs`, `severity_histogram.rs`, `wide_event_histogram.rs`, `time_series_chart.rs`, `severity_badge.rs` (mapping fn kept), `page_not_found.rs`. `flame_graph.rs` **stays** (Wide events flame view), restyled with tokens |

#### 4.3 Blueprint (L)

Delete first (independent PR, immediate build-time win): `eframe`, `egui_graphs`, `petgraph`, `dotenv` from `Cargo.toml`; unused `Hero`, `FilterChips`, `ArcSpine`; unreferenced `public/daisyui.css`, `public/daisyui-theme.css`, `public/tailwind.js` (~1.07 MB out of the embedded release build). Confirm with `cargo check` and a `dx build --release`.

Shell: `AppShell`, app `Bp`. Sections come from the existing `ui/navigation` `NavigationConfig` (proto unchanged); Apps block from `use_app_links()` (replaces the inline discovery and `service_url`/`service_label`). Rail counts (Key/Value, Registry, Gateway) via a small optional fetch. Topbar status per view via `use_page_chrome`.

| View | Change | Notes |
|---|---|---|
| Key / Value `/` + `/kv/:key` | One view; `KeyValueDetail` renders the list with the drawer open. Toolbar: kind `Select`, filter, "N entries · M secret masked". Table: key, value (JSON `Tag` + truncated preview, or masked `d-secret` with Reveal), type `Tag` (full type URL in `title`), hover-only Delete. Drawer: type, size, **`CodeBlock` JSON**, Edit / Copy / Delete. "Add entry" → `Modal` (replaces daisyUI modal). Secret rule (`-----BEGIN`, `*secret*`, `*token*`, `*_ca`) is a pure function with unit tests | Revision / updated / "replicated to 5/5" → §7 |
| Service registry `/service-registry` + `/:name` | Four `StatTile`s; domain `Seg` + "Problems only" `Toggle`; expandable rows with `Strip` instance bars and child rows per instance (this **replaces `ServiceDetail`'s one-card-per-pid**; `/:name` deep link renders with that row open); stale = `last_status_time` older than 15 s | Domain filter source, per-node Raft role → §7 (VERIFY `group`/`process_kind`) |
| Gateway `/gateway`, `/new`, `/:name` | Grouped table (UI host routes / RPC prefix routes, `is-group` rows); protocol `Tag`s and auth `Tag` replace `ProtocolBadges`/`AuthBadge`; `ValidationDot` → `Status` (conflict detection kept); filter chips. Drawer: `RouteFlow` diagram (client → Fuse → backends, `Wire`/`Via`), Match and Upstream `Kv`, Edit / View traffic in Beacon / Delete. Existing `RouteForm` opens in a `Modal` for Add and Edit | "View traffic in Beacon" needs Beacon to read a `?q=` param (§7); envoy config version → §7 |
| Cluster `/cluster` (2,117 LOC) | **First commit: mechanical split** into `cluster/{model,layout,routing,canvas,inspector,menu}.rs`, no behaviour change. Then: replace 54 hex literals with `var(--…)`; `NodeCard` → `d-panel` + `Glyph`/`Status`; add group frames and collapse Foundry plugins to chips; toolbar (legend, Snap/Flow `Toggle`s, counts); minimap + Fit/±zoom `Seg`; right **Inspector** replaces `ConfigDrawer` (endpoint config moves to a `Modal` opened from "Endpoint config…"); `CtxMenu` → shared `Menu`; `CLUSTER_CSS` string → `assets/cluster.css` using shared `d-wire/d-via/d-pad/d-evt`; status bar with pointer coordinates and shortcut hints | Existing persistence (`cluster/layout` KV + localStorage) untouched |
| Events query `/query` (746) | Facet chips per event type with counts (colour dot from the existing type-hash); `QueryBar(Cesql)` + `QueryChips` + Filter… builder; ORDER BY direction becomes a "Newest first / Oldest first" `Seg` in the toolbar; table; CloudEvent `Drawer` with `CodeBlock`. `TypeBadge` → `Tag`/`Dot`, `CesqlBar`, `QueryBuilder` deleted | Delivery status → §7 (VERIFY) |
| Topology `/topology` (127 + 657) | New `TopologyBoard` (~200 LOC, Blueprint-local): producers → event types → consumers with SVG connectors, hover-to-isolate, side panel with per-producer rates. Consumes the existing `TopologyData`/`WatchTopology` stream. **Delete `FullCircle`** (657) | Rates: VERIFY whether `TopologyEdge` carries counts |
| Events metrics `/metrics` (485) | `StatTile`s (replace `MetricCard` + `MetricIcon`), shared `TimeSeriesChart` with crosshair, `d-legend`, `Meter` share bars, per-type table with `Sparkline`. Remove the view's 14 hex literals | |
| Settings `/settings` (255) | (no mockup) `PageHead` + `Panel` + `Field` + `Btn` + `Alert` | |
| `PageNotFound` | shared `NotFound` | |

Exit: `dx build --release` for Blueprint; Go binary still embeds and serves it; screenshot parity for all 7 mocked pages; `grep` for `eframe|egui|petgraph` returns nothing.

### Phase 5 — Bench (L)

Replace `base.html`, `static.go`, `templates.go`, `htmx.min.js` with `draftui`; add `?window=` and `?filter=` handling; add the `/runs` page.

| Page | Change | Data notes |
|---|---|---|
| Shell | rail: **Workflows** (Overview, All workflows + count, "+ New workflow" as action item) · **Runs** (Recent runs + count, Failed · 24h + count) · **Settings** (Plugin registries + count) · **Apps**; topbar "N runs in flight". "Webhook secrets" rail item is **omitted** (no page exists) | |
| Overview `/` (+ `/workflows` becomes an alias; `workflow_list.html` deleted) | `d-verdict` sentence derived from each workflow's last run; window `Seg` (1h / 24h / 7d, `?window=`); four `StatTile`s (Pass rate + sparkline, Running now, Failing workflows, Median run + delta vs previous window); toolbar filter + chips (Failing, Webhook; `?filter=`); workflow `d-table`: name + description, trigger `Tag` (webhook / manual), step count, **12-run `Strip` oldest → newest** (never-run cells `is-none`), last-run `Status` with failing step, p50, Run button (existing `POST /workflows/{name}/run`; disabled while running) | New store query "last N runs per workflow"; p50 and delta computed in Go from runs in window; "Scheduled" chip and `schedule · hourly` tag → §7 |
| Recent runs `/runs` (new) | (no mockup) filterable run table (`?status=failed`) using the shared `runs-table` partial | Same partial replaces the three run lists that exist today |
| Workflow detail `/workflows/{name}` | (no mockup) head with Edit / Delete / Run; `Split`: YAML in `CodeBlock`, run history via `runs-table` | |
| Editor `/workflows/new`, `/{name}/edit` | Name + Description fields; **pipeline strip** (trigger → steps, current step highlighted) rendered server-side from the parsed YAML; `d-editor` textarea with line-number gutter (small JS in `draft.js`); **Problems** panel with `line:col`; topbar Cancel / Validate / Save; right drawer: plugin search (existing `pluginSearch` fragment, restyled as result cards with `required:` line and Insert → existing `appendStep`). Status bar: line count, cursor, ⌘S save, ⌘↵ save-and-run | New `POST /workflows/validate` returns pipeline + problems as htmx fragments; built on `yaml.v3` `Node` for positions plus required-field / unknown-`with:`-key checks against the plugin's stored `config_schema`. Name/Description inputs override `metadata.*` on save (parse → mutate → existing `renderWorkflowYAML`). **Stretch:** YAML syntax colouring via a `<pre>` overlay (~40 lines in `draft.js`); "Selected step" panel (cursor → step) |
| Run `/runs/{id}` | Head: status `Tag` (solid), meta (started, duration, "13 passed · 1 failed · 7 skipped"), actions (Definition, Re-run); `Strip` progress bar; **`d-trace`** step timeline (solid up to the failing step, dashed after — CSS variable set by a 4-line script in `draft.js`); consecutive passed steps collapse into one `<details>` group when > 3 (server-side `groupSteps`); `d-fail` card (error line, request and response side by side, status line highlighted); side panels: last-12-runs `Strip`, run `Kv`. Existing htmx 2 s polling on the fragment is preserved, including self-terminating polling | `uses:` per step joined from the current workflow definition by step name. "Open trace in Beacon", trigger/user, "Likely cause" → §7, omitted |
| Settings | (no mockup) `d-list` of registries + Add form | |
| 404 | `Empty` | |

Exit: `go test ./...` in `services/tooling/bench` (existing template tests updated); screenshot parity with the three Bench mockups; htmx run polling still stops on terminal status.

### Phase 6 — Unmocked surfaces (S–M)

1. **Docs site** (`docs/website`, Hugo + lotusdocs/Bootstrap): `scss/docs/_bootstrap.scss` maps Bootstrap variables (`$primary`, `$body-bg`, `$body-color`, navbar, code, link) onto Draft tokens via Hugo `module.mounts`; dark-first; chroma code colours from `tk-*`. Token-level only — no restructuring of the theme. (`draft.css`'s own header lists the docs site as a target surface.)
2. **Login / register (D8)**: add a `draftui` `auth-card` partial and restyle `pkg/basic_authentication/templates/{index,login,register}.html` onto it (centred `d-panel` card, `Field`s, `Alert` for "user already exists", neutral title); drop the daisyUI/Tailwind CDN tags from its `index.html` layout; README gains the `draftui.Static()` mount note. Cheap (77 lines) and safe: nothing in Draft imports it and the golf app uses its own copy. **Also file a ticket**: `chi.go` hard-codes `services/golf-app/app/templates/index.html` (lines 60 and 144), so the package is not usable outside that layout.
3. **Placeholders** (`catalyst`, `fuse`, `echo`, `crud`, `file_host` `web-client/package.json`): no UI exists; nothing to do.
4. **⌘K**: the button is drawn everywhere but no palette is designed. Render it inert (`aria-disabled`) until a palette is designed — do not ship a button that does nothing without saying so.

### Phase 7 — Cleanup and docs (S)

1. Remove the daisyUI and Tailwind CDN tags from each `index.html` once the class-contract report shows zero daisyUI/Tailwind classes for that app; keep the two-line boot snippet.
2. Delete leftovers: Foundry/Bench `templates/base.html`, `static.go`, `static/htmx.min.js`; `public/` vendored files; `input.css` + `tailwind.config.js` in Blueprint (unreferenced by the build).
3. Docs: update `blueprint-web-client-components.md` (component reference now lives in the crate README/gallery), `service-ui-subdomains.md`, `bench-workflow-engine.md`, `foundry-plugin-repository.md` (UI sections), root `CLAUDE.md` repo overview (add `tools/draft-ui`).
4. Update `.claude/skills/dioxus-ui/references/daisyui.md` and `components.md`: the shared crate and `draft-daisyui.theme.css` become the default; the CDN pipeline note is retired.
5. Add a project memory entry for the crate and the no-htmx-in-Blueprint rule.

---

## 7. Backend and data follow-ups (not blocking the restyle)

| # | Mockup element | Missing | Where | Status |
|---|---|---|---|---|
| 1 | Bench run: trigger source and user, "Open trace in Beacon", trace id | `Run` has only id/workflow/status/times/steps | `workflow/v1/service.proto` `Run` | confirmed absent |
| 2 | Bench "schedule · hourly" tag, "Scheduled" chip | `Trigger` has only `webhook` | proto + Bench scheduler | confirmed absent |
| 3 | Bench 12-run history, p50, median delta | store query for last N runs per workflow | `bench/store.go` | new query, no proto change |
| 4 | Bench editor problems with line:col, unknown-field / "did you mean" | validation endpoint | `bench` (yaml.v3 `Node`) | new endpoint |
| 5 | Bench "Likely cause" | correlation of a failed step's route with Blueprint heartbeat staleness | Bench ↔ Blueprint | design needed |
| 6 | Bench "Webhook secrets" page (rail item omitted, D12) | no page design. Bench only *reads* `BenchWebhookSecret` from Blueprint KV today (`blueprint://secrets/<key>`); a page needs KV write access for rotation, a confirm + one-time reveal, and a security review (rotation breaks senders; secrets shown in an SSR page) | Bench, Blueprint KV | not started; own design + review |
| 7 | Foundry categories/tags, "Most used", "N workflows", "Used by" | manifest tags; Bench usage join | proto + cross-service call | not started |
| 8 | Foundry Address / "Serving" / "Registered in Blueprint" | join plugin name to Blueprint service registry | Foundry → Blueprint | not started |
| 9 | Blueprint KV revision, updated, "replicated to N/M" | no such fields on KV values | `key_value` proto | confirmed absent |
| 10 | Blueprint topbar: Raft term / commit / leader; per-node role | no RPC exposes term, commit or role; only the KV `leader` key exists | Blueprint | **audited: absent.** Show "Raft · N nodes · leader X", omit the rest |
| 11 | Blueprint registry domain (Core/Tooling/Plugin/Example) | `ProcessKind` is only SERVER/JOB; nothing sets `Service.group` | discovery proto | **audited: absent.** Derive client-side from a name table; a non-empty `group` overrides |
| 12 | Gateway "envoy config vN applied" | `Route` has no version field | Fuse | **audited: absent.** Omit |
| 13 | Events query delivery status | no such field on events or the query response | Catalyst | **audited: absent.** Omit |
| 14 | Events topology per-producer rates | `TopologyEdge.vol` exists; `GetMetrics` takes a window | Catalyst | **audited: available.** Rates derivable, no backend work |
| 15 | Beacon saved queries (rail item omitted, D12) | no page design and no storage. Beacon's client reaches only Beacon's own RPCs and its server uses no KV/repository. Options: localStorage (client-only, per-browser stopgap) or a Beacon RPC with server-side storage (shareable). Count badges need one query per saved item, so drop them in v1 | Beacon | not started; the `Grammar` work (§3.8) makes "save this query" cheap later |
| 16 | Beacon "Group by" view | grouping in query/UI | Beacon | not started |
| 17 | Beacon metric browser (names, type, labels) | only `QueryMetrics` RPC exists | `metrics/v1` | confirmed absent |
| 18 | Cross-app deep links with `?q=` | Beacon reads `PENDING_TRACE_ID` global, not URL params | Beacon | small change |
| 19 | Lineman due date, short id (`T-104`) | `Task` has neither | `models.proto` | confirmed absent |
| 20 | Lineman loop-instance grouping | `Task` has no loop link | `models.proto` | **audited: absent.** Loop card built from `Loop`; no grouping |
| 21 | Lineman "N agents online", last-event age | derivable from `ListAgents` / `Watch` | client-side | no backend work |
| 22 | Rail counts (Blueprint) | 3 small fetches | client-side | no backend work |
| 23 | Cross-app theme sync | parent-domain cookie | Phase 0 spike | **verified working** in Chrome across `a.`/`b.draft.localhost` |
| 24 | ⌘K command palette | no design | design | not started |
| 25 | Query autocomplete (future): field/operator/attribute-key/value suggestions | `DescribeQuerySchema` RPC per grammar and dataset (Beacon logs / traces / wide events, Catalyst CESQL), replacing hand-copied Rust field lists | `beacon/query`, `catalyst/broker` | not started; Phase 2 ships compiled-in tables as the fallback |
| 26 | Query autocomplete values; also unlocks item 17 (metric browser) | value-lookup RPCs: distinct `service_name`, attribute / business / runtime attribute keys, event types, metric names and label names | Beacon (ClickHouse `DISTINCT` / `mapKeys`), Catalyst | not started |
| 27 | Inline query error markers | `ParseError` has only `Msg`; add a position, and record offsets on lexer tokens | `beacon/query/beaconql.go`, `promql.go` | confirmed absent |
| 28 | PromQL chips in the mockup (`histogram_quantile(0.95, …)`, `topk(5, …)`) | parser supports only `rate` and `sum/avg/max/min by`; chips must be limited to the supported subset until the backend grows | `beacon/query/promql.go` | client-only fix now |
| 29 | ORDER BY control | not a language feature; moves to a results-toolbar `Seg` | Blueprint events query | client-only |
| 30 | `pkg/basic_authentication` usable outside the golf-app layout | `chi.go` hard-codes `services/golf-app/app/templates/index.html` (lines 60, 144); template location should be a parameter | `pkg/basic_authentication` | confirmed defect; not UI work |

| 31 | Lineman restart: publish-on-start is not idempotent | Foundry `Publish` returns `AlreadyExists` for `lineman v1` on every restart (the shutdown retract does not clear it), and the effect failure is fatal and swallowed by the OTel logger, so Lineman exits 1 silently. The same hazard applies to every plugin that self-publishes | `services/tooling/lineman`, Foundry | confirmed, pre-existing |
| 32 | Beacon Logs live tail | opening the tail shows the oldest 200 backlog rows, then jumps to the live stream | `beacon` `StreamLogs` backlog | confirmed |
| 33 | Beacon trace list: span count, service dots | `TraceRoot` has neither | `beacon` trace proto | confirmed absent |
| 34 | Beacon wide events: a duration column | no `duration_ms` on wide events | wide-events proto | confirmed absent |
| 35 | Blueprint Metrics chart and trend | Catalyst exposes no event time series; the old page drew fixed placeholder sparklines and an always-empty error-rate tile, so neither was carried over | Catalyst | needs a time-series endpoint |
| 36 | Blueprint registry: per-node Raft role, version, domain source | see 10 and 11 | Blueprint | confirmed absent |
| 37 | Lineman cards are not keyboard-reachable (only the ⋯ menu is) | drag and drop has no keyboard path other than "Move to" | `lineman/web-client` | design needed |
| 38 | Bench run page: expanded groups collapse on each poll | the 2 s swap re-renders `<details>`; `hx-preserve` or a client-side open-state map | `bench` | small |
| 39 | Bench editor: "Selected step" panel and cursor → pipeline highlight | stretch items from the mockup, not built | `bench` + `draft.js` | not started |
| 40 | Blueprint `input.css`, `tailwind.config.js`; root `package.json` / `package-lock.json` | unreferenced leftovers. The two Blueprint files are named by `scripts/run-local-watch.sh` (`-w`), which cannot be edited under a running instance; remove all three (and the `-w` arguments) at the next stack restart | repo | cleanup |
| 41 | Docs landing page keeps its orange brand | its hero illustration is orange; restyling it means redrawing the illustration | `docs/website` | design decision |
| 42 | `dx` CLI 0.7.9 against crates locked at 0.7.10 | "dx and dioxus versions are incompatible", and `wasm-opt` aborts on DWARF, so bundles ship unoptimised (~3 MB vs ~2.4 MB) | tooling | pre-existing |

Each of these is a separate small ticket. The UI slot for each is omitted, not stubbed with fake values.

---

## 8. Consolidation ledger

Existing code **deleted or replaced** by shared crate components (measured LOC):

| Area | Removed | LOC |
|---|---|---|
| Blueprint | `metric_card` 185 · `query_builder` 260 · `arc_spine` 372 · `full_circle` 657 · `hero` 24 · `filter_chips` 35 · `cesql_bar` 34 · `type_badge` 27 · `validation_dot` 16 · `protocol_badges` 19 · `auth_badge` 21 · `navbar` 54 | 1,704 |
| Beacon | `metric_card` 187 · `query_builder` 357 · `wide_event_query_builder` 355 · `query_bar` 87 · `time_range` 171 · `severity_histogram` 176 · `wide_event_histogram` 161 · `time_series_chart` 179 · `severity_badge` 31 · `trace_pill` 45 | 1,749 |
| Not-found views | 3 copies | 34 |
| **Rust total** | of 12,895 LOC across the three clients | **≈ 3,500** |
| Go SSR | `base.html` ×2 (6.4 + 7.2 KB), `static.go` ×2, `templates.go` boilerplate ×2, `htmx.min.js` ×2 (96 KB → 48 KB), duplicated `.signal` CSS, `workflow_list.html`, two of three run-list blocks | — |
| Assets | ~1.07 MB unreferenced vendored CSS/JS in Blueprint `public/`; daisyUI 4 + Tailwind Play CDN from both Go services; daisyUI 5 + Tailwind browser JIT from the three clients (Phase 7) | — |
| Dependencies | `eframe`, `egui_graphs`, `petgraph`, `dotenv` | — |

New shared code is written once (est. 2,000–2,500 LOC Rust, ~600 LOC Go, ~2,200 lines SCSS). The saving is mostly in **not having three-to-five divergent copies** of each pattern to keep in step.

---

## 9. Risks and open questions

| Risk | Mitigation |
|---|---|
| `asset!()` / `Link` / dependency shape does not work from a library crate | Phase 0 spikes with named fallbacks |
| `grass` diverges from dart-sass on a future feature | Restrict SCSS to maps, `@each`, mixins, `@use`; drift check catches output changes; fallback is npm `sass` with no source changes |
| Committed `dist/` drifts from SCSS | CI `--check` |
| Blueprint `cluster.rs` restyle regresses behaviour (drag, ports, routing, persistence) | mechanical split first with no behaviour change; restyle in later commits; existing layout persistence keys untouched |
| Query-builder consolidation changes generated queries | unit tests port the old components' fragment outputs before the old code is deleted |
| Responsive/PWA regression from the mockup's fixed shell | §5.1 built and gallery-reviewed in Phase 1, before any app adopts the shell |
| Light theme is un-mocked and only partly exercised | every view PR is checked in both themes; contrast test |
| Theme choice does not sync across subdomains | Phase 0 cookie spike; acceptable fallback is per-app |
| Go 1.24 toolchain locally vs `go 1.25.3` in `go.mod` | `go test` will fetch the toolchain; note for CI |

**Questions for you — all five resolved in review on 2026-09-26** (decisions recorded as D2, D8, D10, D11, D12 in §4):

1. ~~D2 — `go.mod` beside `Cargo.toml` in `tools/draft-ui`~~ — **resolved: yes.**
2. ~~F5.1 — query builders~~ — **resolved: one grammar-driven predicate builder, autocomplete-ready (§3.8), client-side completion first.**
3. ~~Lineman drag-and-drop~~ — **resolved: include, staged last in the Lineman phase (D11).**
4. ~~Bench "Webhook secrets" and Beacon "Saved queries" rail items~~ — **resolved: omit both, file tickets (D12).**
5. ~~D8 — `pkg/basic_authentication` templates~~ — **resolved: restyle the starter templates and add an `auth-card` partial** (correcting an earlier wrong claim that other repos consume the package).

---

## 10. Verification and definition of done

**Per view:** uses only `d-*` classes and shared components · zero hex literals · both themes and a non-default primary swatch · loading / empty / error states · keyboard reachable with visible focus · deep link preserved · screenshot parity with the mockup at 1440 px and 900 px (served side by side and compared in Chrome) · replaced components deleted in the same PR.

**Per phase:**

| Phase | Commands |
|---|---|
| 0–2 | `cargo test -p draft-ui` · `cargo run -p draft-ui --features cli --bin draft-ui-css -- --check` · `cargo check --target wasm32-unknown-unknown -p draft-ui --features dioxus,discovery` |
| 3, 5 | `go test ./...` in `services/tooling/foundry` and `services/tooling/bench` · `curl` the running service for `/static/draft/draft.css` (a prior bug in these services was a static path that only failed at runtime) |
| 4 | `dx build --release` in each `web-client` · run through Fuse at `*.draft.localhost` · confirm the Go binary still embeds the new build |
| 7 | class-contract report shows 0 daisyUI/Tailwind classes; `grep -r "cdn.jsdelivr\|cdn.tailwindcss"` over `services/` returns nothing |

---

## 11. Milestone checklist

- [x] **P0** Spikes recorded; VERIFY items resolved
- [x] **P1** `draft-ui` scaffold · SCSS split byte-equivalent · F3 fixes · responsive shell · gallery signed off · CI drift + contrast tests
- [x] **P2** Primitives · layout/shell · data · query · viz · discovery · util
- [x] **P3** Go kit · Foundry migrated
- [x] **P4.1** Lineman (pilot) · **P4.2** Beacon · **P4.3** Blueprint (dependency cleanup first)
- [x] **P5** Bench
- [x] **P6** Docs-site theme · auth partial · ⌘K inert
- [x] **P7** CDN tags removed · leftovers deleted (two Blueprint files and a root `package.json` remain on purpose: ticket 40) · docs, skill and memory updated
- [x] §7 follow-up tickets written up (items 31–42 added); filing them in the tracker is the remaining step

---

## 12. As built (2026-09-26)

### What shipped

| Phase | Result |
|---|---|
| 0–2 | `tools/draft-ui`: SCSS compiled to `dist/` by `grass` (`draft.css`, `tokens.css`, `fonts.css`, the daisyUI 5 theme, `draft.js`/`boot.js`), the Rust component library behind the `dioxus` / `discovery` / `cli` features, tests for WCAG contrast, class contract and `dist/` drift |
| 3 | The Go kit (`draftui`): shell, partials, `Kit`, embedded static handler. Foundry migrated first; `dist/` is committed and un-ignored (the root `.gitignore` had excluded every `dist/`) |
| 4 | Lineman (board, list, drag and drop with Undo, "Move to", modals), Beacon (wide events, logs with live tail, traces with a flame graph, metrics) and Blueprint (Key/Value, registry, gateway, events query / topology / metrics, the cluster canvas rebuilt in Dioxus) all on the crate. Blueprint no longer depends on `eframe`, `egui_graphs` or `petgraph`, and its `public/` daisyUI and Tailwind copies are gone from the binary |
| 5 | Bench: Overview, Recent runs, Workflow detail, Run, editor with live validation, Settings; see [Bench's UI](/docs/architecture/bench-workflow-engine#the-ui) |
| 6 | The docs-site theme (`dist/docs/_draft.scss`, mounted into Hugo as the lotusdocs colour file), the `auth-card` partial and `bare` layout, restyled `basic_authentication` starter templates, ⌘K rendered inert everywhere |
| 7 | The `d-*` class contract passes over every consumer (Rust sources, Go templates, the auth starters), no `cdn.jsdelivr` or Tailwind reference remains under `services/`, docs and the `dioxus-ui` skill updated, CI extended |

### Where it differs from the plan

- **`RouteLink` replaces `NavConfig` in the crate.** The KV-stored rail configuration is Blueprint-specific and stays in Blueprint; the crate has `NavSection` / `NavItem`.
- **Blueprint's query filtering is client-side.** Catalyst's `Query` takes a typed expression a browser cannot build, so `cesql.rs` matches events in the client and the request only bounds the time range.
- **Bench**: `/workflows` is an alias that renders the Overview. The editor's Name and Description are bound to the YAML text by `draft.js` rather than overriding `metadata.*` on the server, so the text stays the single source of truth. Not built: the editor's "Selected step" panel and cursor-to-step highlight (stretch), the mockups' Webhook secrets page, trace and "likely cause" on the run page (tickets 1, 5, 6).
- **Docs**: the docs pages take the Draft palette, fonts and code colours; the landing page keeps its orange brand (ticket 41). The mapping is generated from the token maps rather than hand-copied, and imported as a Hugo module through a `go.mod` `replace`. `hugo.yaml`'s `module.replacements` cannot do this on its own: `go mod download` still runs against the requirement.
- **Additions beyond the plan**, each because a second place needed it: `d-cols` (main plus a side column), `d-status-stack`, the two-line `is-title` table cell, `d-fields`, `d-auth`, `d-check-row`, an `d-drawer--inline` and `d-hide-narrow` for narrow screens, a `topbar` block and `BarItem.ID` on the Go shell, and `draft.js` behaviours `data-insert-into`, `data-focus`, `data-yaml-field` and the editor's YAML colouring overlay.
- **Left in place on purpose** (ticket 40): Blueprint's `input.css` and `tailwind.config.js` and the root `package.json`.

### Found and fixed on the way

- **Theme preference did not persist on an IP host.** `boot.js` took an existing cookie as proof its parent-domain write had been accepted, so on `127.0.0.1` only the first toggle ever landed. Fixed, with `tests/boot.test.mjs`, and `tests/boot_sync.rs` now fails when a client's inlined copy of `boot.js` drifts from the source.
- **The shared `Modal` never took focus.** It now moves focus in, cycles Tab, and restores it on close.
- **Lineman's rank arithmetic disagreed with the server** (`midpoint` used different constants); aligned.
- **Beacon's waterfall let a tiny final span overflow its lane**; the bar is clamped.
- **Blueprint's cluster canvas zoomed about a stale origin** measured once on mount; it now reads the stage origin at gesture time.
- **Fabricated data was removed**: the old Blueprint Metrics page drew hard-coded placeholder sparklines and an error-rate tile that was always empty (ticket 35).
- **A plan-document shortcode crashed `hugo server`.** An unclosed `{{</* alert */>}}` shortcode in this document made the live docs server fail its rebuild and panic; Hugo 0.162 requires the self-closing `/>}}` form.

### Verification

Crate: Rust unit and integration tests (contrast for apps and docs, class contract over all consumers, drift, boot sync, SSR structure), Go kit tests, and 17 node tests for the shared JavaScript. Apps: Blueprint 57, Beacon 18, Lineman 16 native tests; Bench's UI tests against an in-memory store plus store tests against a real Postgres. Every page was driven in a browser in both themes and at 400 px against mock or real data (the real Bench, Blueprint, Foundry and Catalyst were only read), including the parts a screenshot cannot show: drag and drop, live validation against the real Foundry schemas, the run page's self-terminating poll, and out-of-band updates of the topbar.
