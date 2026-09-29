# draft-ui

The shared Draft design system: one source of truth, four consumers.

```
scss/  ──(draft-ui-css)──▶  dist/  ─┬─▶ Dioxus clients   (Rust crate, `dioxus` feature)
                                    ├─▶ Go SSR services  (Go module `draftui`, embeds dist/ + templates/)
                                    ├─▶ Docs site       (dist/docs/_draft.scss, mounted into Hugo)
                                    └─▶ Other projects  (tokens.css, draft-daisyui.theme.css)
```

Design reference: [`../../mockups/`](../../mockups). Plan and decisions:
`docs/website/content/docs/architecture/design-system-implementation-plan.md`.

## Layout

| Path | What |
|---|---|
| `scss/` | Source. `abstracts/_tokens.scss` holds the token maps; everything else is generated from them |
| `dist/` | **Generated and committed.** `draft.css`, `draft.min.css`, `fonts.css` (+ `fonts/`), `tokens.css`, `draft-daisyui.theme.css`, `docs/_draft.scss`, `draft.js`, `boot.js`, `vendor/htmx.min.js`. Never hand-edit |
| `js/` | Sources for `dist/boot.js` and `dist/draft.js` (theme boot; editor, copy, insert, YAML fields and other progressive enhancement) |
| `src/` | Rust: Dioxus components (`dioxus` feature), app discovery (`discovery` feature), the SCSS build tool (`cli` feature) |
| `templates/`, `*.go` | Go module: `html/template` partials and an `http.Handler` for the embedded assets |
| `preview/` | Component gallery (`index.html`) and a full app shell (`shell.html`). Serve this directory and open them |
| `tests/` | WCAG contrast (apps and docs), class contract, `dist/` drift, SSR/DOM structure; `*.test.mjs` are the JS tests |

## Build

```sh
cargo run --features cli --bin draft-ui-css            # scss/ → dist/
cargo run --features cli --bin draft-ui-css -- --check # CI: fail if dist/ is stale
cargo test --features cli,dioxus
node --test tests/*.test.mjs                           # boot.js preferences, the editor's YAML fields and colouring
go test ./...                                          # the Go kit (shell, partials, auth card, static handler)
```

To also check the classes an app uses, point the class contract at its sources (an app's own
`assets/*.css` beside a scanned `src/` counts as defined):
`DRAFT_UI_SCAN=../../services/tooling/bench/templates:../../services/core/beacon/web-client/src cargo test --test contract`.

No Node and no `sass` binary: the compiler is [`grass`](https://crates.io/crates/grass).

## Themes and colour

* `data-theme="draft"` (dark, default) and `data-theme="draft-light"`.
* Primary is swatchable: set `--primary-base` on `:root`. Light mode derives a darker shade.
* **Text is `--ink`, `--dim`, a kind colour or a status colour.** `--dimmer` is for rules and
  decoration only (3.1 : 1 dark, 2.4 : 1 light). `tests/contrast.rs` enforces ≥ 4.5 : 1 for every
  text token on every surface, in both themes, for all five swatches — and lists the only places
  `--dimmer` may colour text.

## daisyUI

Shipped views use `d-*` classes. daisyUI 5 is supported as a *theme target*, not a dependency:

1. `draft.css` maps `--color-*` etc. onto the Draft tokens under `[data-theme]`, so daisyUI
   components (from a CDN or compiled) pick up the palette.
2. `draft-daisyui.theme.css` is the same theme as `@plugin "daisyui/theme"` blocks for Tailwind v4
   compiled builds (`@plugin "daisyui" { themes: false }`).
3. `tokens.css` is the variables alone.

Use a daisyUI class only when no `d-*` equivalent exists, and file a follow-up to add one.

## Using it

**Dioxus** (`Cargo.toml`): `draft-ui = { path = "../../../../tools/draft-ui", features = ["dioxus"] }`,
render `DraftStyles {}` once at the root and put the boot snippet (`dist/boot.js`) in `index.html`.

**Go SSR**: `require github.com/steady-bytes/draft/tools/draft-ui` plus
`replace … => ../../../tools/draft-ui`; mount `draftui.Static()` at `/static/draft/` and link
`fonts.css`, `draft.css`, `draft.js` from the layout.

**Anything else**: link `dist/fonts.css` and `dist/draft.css` and set `<html data-theme="draft">`.

htmx is vendored for the Go SSR services only. It is never used by the Dioxus side.

## Components

Both renderers emit the same `d-*` markup. Rust components live under `src/`; the Go side is
`html/template` partials (`templates/`) and small view models (`*.go`). Open `preview/index.html`
for every component rendered.

| Area | Rust (Dioxus) | Go (`draftui`) |
|---|---|---|
| Frame | `AppShell`, `use_page_chrome` (status pill, status bar), `use_app_links`, `NavSection` / `NavItem` | `Kit`, `Page`, the `shell` and `bare` templates (`topbar` and `head` blocks), `NavItem`, `AppLinks` |
| Layout | `PageHead`, `Split`, `Drawer`, `Toolbar`, `SectionTitle` | classes `d-page-head`, `d-split`, `d-drawer`, `d-cols`, `d-toolbar` |
| Controls | `Btn`, `Chip`, `Seg`, `Toggle`, `Menu`, `Modal` (focus managed), `Toast` / `use_toast`, `Field`, `TextInput`, `Select`, `Textarea`, `Check`, `Alert` | classes; `data-*` behaviours in `draft.js` (theme toggle, copy, confirm, `data-insert-into`, `data-yaml-field`, editor) |
| Status | `Status`, `Dot`, `Tag`, `Svc`, `TraceLink`, `Count`, `Kbd`, `Glyph` | partials `d-status`, `d-tag`; `Tag`, `StatusDot` |
| Data | `StatTile` / `Delta`, `Strip`, `Kv`, `List`, `CodeBlock` (highlighted), `Progress`, `Meter`, `DurationCell`, `Board` | partials `d-stat`, `d-strip`, `d-code`, `d-empty`; `Stat`, `Strip`, `Code` |
| Query | `QueryBar`, `QueryChips`, `QueryFilters`, `Grammar`s for CESQL, Beacon logs, traces and wide events, PromQL; `Completer` | (htmx search inputs: `d-query`) |
| Charts | `Histogram`, `TimeSeriesChart`, `SeriesTable`, `Waterfall`, `CompactWaterfall`, `Sparkline`, `Wire` | `SparkPath`; classes `d-trace`, `d-step`, `d-fail`, `d-pipe`, `d-editor` for Bench |
| Standalone pages | | `bare` layout and `auth-card` (`LoginCard`, `RegisterCard`) |

Rust-only pieces (the cluster canvas, drag and drop) stay in the app that uses them. Add a component
here when a second place needs it, in both renderers if both need it, and add its classes to
`scss/` so the class contract covers it.

## The docs site

`scss/docs.scss` compiles to `dist/docs/_draft.scss`, a colour file for the Hugo *lotusdocs* theme
(`params.docs.themeColor: draft`). `docs/website/hugo.yaml` imports this directory as a Hugo module
(`go.mod` `replace` → `../../tools/draft-ui`) and mounts `dist/docs` at
`assets/docs/scss/custom/colors` and `dist/fonts` at `static/draft/fonts`. The theme's own CSS
variables are overridden, nothing is copied. `tests/docs_theme.rs` checks its contrast and that its
surfaces are the apps' surfaces. The landing page keeps its own orange brand.

## Class contract

`d-*` components, `d-*--*` modifiers, `is-*` state, `tk-*` code tokens. Both renderers emit the same
classes; `tests/contract.rs` fails if any `d-*` class in Rust, Go templates or the preview is not
defined in `dist/draft.css`.
