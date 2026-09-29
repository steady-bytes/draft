//! Render every component to a string and assert on its classes and ARIA. Markup regression tests
//! that need no browser: if a component stops emitting a `d-*` class the stylesheet defines, or
//! loses an accessibility attribute, these fail.

use dioxus::prelude::*;
use draft_ui::data::*;
use draft_ui::kinds::{AppKind, StatusKind, Tone};
use draft_ui::layout::*;
use draft_ui::query::{GrammarId, QueryBar, QueryFilters};
use draft_ui::shell::{AppShell, NavItem, NavSection};
use draft_ui::ui::*;
use draft_ui::viz::*;

fn render(app: fn() -> Element) -> String {
    let mut dom = VirtualDom::new(app);
    dom.rebuild_in_place();
    dioxus_ssr::render(&dom)
}

macro_rules! html {
    ($body:expr) => {{
        fn app() -> Element {
            $body
        }
        render(app)
    }};
}

fn has(html: &str, needle: &str) {
    assert!(html.contains(needle), "expected `{needle}` in:\n{html}");
}

fn lacks(html: &str, needle: &str) {
    assert!(!html.contains(needle), "did not expect `{needle}` in:\n{html}");
}

#[test]
fn tag_classes() {
    let h = html!(rsx! {
        Tag { tone: Tone::Err, solid: true, "Failed" }
        Tag { "plain" }
    });
    has(&h, r#"class="d-tag d-tag--err d-tag--solid""#);
    has(&h, r#"class="d-tag d-tag--quiet""#);
}

#[test]
fn status_kinds_and_live() {
    let h = html!(rsx! {
        Status { kind: StatusKind::Warn, "Degraded" }
        Status { live: true, "Online" }
        Status { kind: StatusKind::Info, live: true, "Running" }
    });
    has(&h, r#"class="d-status d-status--warn""#);
    has(&h, r#"class="d-status d-status--live""#);
    has(&h, r#"class="d-status d-status--info d-status--live""#);
}

#[test]
fn buttons_render_as_button_or_anchor() {
    let h = html!(rsx! {
        Btn { variant: BtnVariant::Primary, "Run" }
        Btn { variant: BtnVariant::Danger, size: BtnSize::Sm, "Delete" }
        Btn { href: "https://example.com".to_string(), external: true, "Docs" }
        Btn { disabled: true, "Off" }
    });
    has(&h, r#"<button class="d-btn d-btn--primary" type="button">"#);
    has(&h, "d-btn d-btn--danger d-btn--sm");
    has(&h, r#"target="_blank""#);
    has(&h, r#"rel="noopener noreferrer""#);
    has(&h, r#"disabled"#);
}

#[test]
fn seg_marks_the_selected_option() {
    let h = html!(rsx! {
        Seg::<u32> {
            options: vec![(15, "15m".to_string()), (60, "1h".to_string()), (180, "3h".to_string())],
            value: 60,
            on_change: |_| {},
            label: "Range".to_string(),
        }
    });
    has(&h, r#"class="d-seg""#);
    has(&h, r#"role="group""#);
    // exactly one pressed
    assert_eq!(h.matches(r#"aria-pressed="true""#).count(), 1, "{h}");
    assert_eq!(h.matches(r#"aria-pressed="false""#).count(), 2, "{h}");
}

#[test]
fn toggle_is_a_real_switch_button() {
    let h = html!(rsx! {
        Toggle { checked: true, on_change: |_| {}, "Stream" }
        Toggle { checked: false, on_change: |_| {}, "Snap" }
    });
    has(&h, r#"role="switch""#);
    has(&h, r#"aria-checked="true""#);
    has(&h, r#"aria-checked="false""#);
    has(&h, "<button");
}

#[test]
fn chip_toggle_count_and_dot() {
    let h = html!(rsx! {
        Chip { pressed: true, count: 42u32, dot: Tone::Ca, onclick: |_| {}, "order.created" }
    });
    has(&h, r#"class="d-chip""#);
    has(&h, r#"aria-pressed="true""#);
    has(&h, r#"class="d-count""#);
    has(&h, r#"class="d-dot""#);
}

#[test]
fn field_shows_error_over_hint() {
    let h = html!(rsx! {
        Field { label: "Name".to_string(), hint: "hint".to_string(), error: "Name is required".to_string(),
            TextInput { value: String::new(), oninput: |_| {}, invalid: true }
        }
    });
    has(&h, "Name is required");
    lacks(&h, r#"class="d-hint""#);
    has(&h, r#"aria-invalid="true""#);
}

#[test]
fn modal_is_absent_when_closed_and_a_dialog_when_open() {
    let closed = html!(rsx! { Modal { open: false, title: "Add".to_string(), on_close: |_| {}, "body" } });
    lacks(&closed, "d-modal");
    let open = html!(rsx! { Modal { open: true, title: "Add".to_string(), on_close: |_| {}, "body" } });
    has(&open, r#"role="dialog""#);
    has(&open, r#"aria-modal="true""#);
    has(&open, "d-scrim");
}

#[test]
fn alert_carries_role_and_tone() {
    let h = html!(rsx! { Alert { kind: StatusKind::Err, mono: true, "boom" } });
    has(&h, r#"class="d-alert d-alert--err d-alert--mono""#);
    has(&h, r#"role="alert""#);
}

#[test]
fn menu_marks_checked_and_danger_items() {
    let h = html!(rsx! {
        Menu {
            heading: "Move to".to_string(),
            items: vec![MenuItem::new("q", "Queued").checked(true), MenuItem::new("d", "Delete").danger().separated()],
            on_select: |_| {},
            on_close: |_| {},
        }
    });
    has(&h, r#"role="menu""#);
    has(&h, r#"aria-checked="true""#);
    has(&h, "d-menu-item--danger");
    has(&h, "d-menu-sep");
    has(&h, "d-menu--anchored");
}

#[test]
fn empty_loading_and_not_found() {
    let h = html!(rsx! {
        Empty { title: "Nothing here".to_string(), "Try again" }
        Loading {}
        NotFound { route: vec!["a".to_string(), "b".to_string()] }
    });
    has(&h, r#"class="d-empty""#);
    has(&h, r#"class="d-wave""#);
    has(&h, "No page at /a/b");
}

#[test]
fn stat_tile_delta_colour_follows_good_or_bad() {
    let h = html!(rsx! {
        StatTile { label: "Error rate".to_string(), value: "0.4".to_string(), unit: "%".to_string(),
            delta: Delta::good("▼ 0.2pt"), spark: vec![3.0, 2.0, 1.0] }
        StatTile { label: "P95".to_string(), value: "94".to_string(), delta: Delta::bad("▲ 12ms"), value_tone: Tone::Err }
    });
    has(&h, "d-delta is-good");
    has(&h, "d-delta is-bad");
    has(&h, r#"class="d-spark""#);
    has(&h, "color:var(--err)");
}

#[test]
fn kv_pairs() {
    let h = html!(rsx! {
        Kv { KvItem { label: "Type".to_string(), "Value" } KvItem { label: "Size".to_string(), tone: Tone::Err, "612 B" } }
    });
    has(&h, r#"<dl class="d-kv">"#);
    has(&h, "<dt");
    has(&h, "color:var(--err)");
}

#[test]
fn code_block_highlights() {
    let h = html!(rsx! { CodeBlock { text: "{\"a\": 1}".to_string(), lang: CodeLang::Json } });
    has(&h, r#"class="d-code""#);
    has(&h, r#"class="tk-key""#);
    has(&h, r#"class="tk-num""#);
}

#[test]
fn strip_states() {
    let h = html!(rsx! {
        Strip {
            cells: vec![StripCell::new(CellState::Ok), StripCell::new(CellState::Err).current(), StripCell::new(CellState::Run), StripCell::new(CellState::None)],
            label: "3 of 4".to_string(),
        }
    });
    has(&h, r#"class="d-strip""#);
    has(&h, "is-err is-cur");
    has(&h, "is-run");
    has(&h, "is-none");
}

#[test]
fn progress_uses_the_track_colour_for_quiet() {
    let h = html!(rsx! {
        Progress { segments: vec![ProgressSegment::new(Tone::Primary, 10.0), ProgressSegment::new(Tone::Quiet, 90.0)] }
    });
    has(&h, "--c:var(--primary)");
    has(&h, "--c:var(--rule-strong)");
}

#[test]
fn board_card_locks_and_drags() {
    let h = html!(rsx! {
        Board {
            BoardColumn { title: "Queued".to_string(), count: 2u32, drop_active: true,
                Card { draggable: true, "a" }
                Card { draggable: true, locked: true, "b" }
            }
        }
    });
    has(&h, r#"class="d-board""#);
    has(&h, "d-col-head is-drop");
    has(&h, r#"draggable="true""#);
    has(&h, r#"draggable="false""#); // the locked card
    has(&h, "is-locked");
}

#[test]
fn histogram_keeps_a_minimum_height_for_errors() {
    let h = html!(rsx! {
        Histogram {
            buckets: vec![HistoBucket { title: "t".to_string(), values: vec![100.0, 1.0] }],
            tones: vec![Tone::Primary, Tone::Err],
            axis: vec!["22:50".to_string(), "01:50".to_string()],
        }
    });
    has(&h, r#"class="d-histo""#);
    has(&h, "background:var(--err)");
    has(&h, "height:3.0px"); // 1/100 of 72px would be under 1px; the floor keeps it visible
    has(&h, r#"class="d-axis""#);
}

#[test]
fn chart_draws_series_or_an_empty_state() {
    let with = html!(rsx! {
        TimeSeriesChart {
            series: vec![ChartSeries::new("a", Tone::Bp, vec![(0.0, 1.0), (60.0, 3.0), (120.0, 2.0)])],
            unit: " req/s".to_string(),
        }
    });
    has(&with, r#"class="d-chart""#);
    has(&with, r#"class="grid""#);
    has(&with, "stroke=\"var(--bp)\"");
    let none = html!(rsx! { TimeSeriesChart { series: vec![] } });
    has(&none, "No data points in range");
}

#[test]
fn waterfall_selection_and_error_bars() {
    let h = html!(rsx! {
        Waterfall {
            rows: vec![WaterfallRow {
                id: "s1".to_string(), name: "ingress".to_string(), service: "fuse".to_string(), tone: Tone::Fs,
                depth: 1, left: 0.8, width: 0.19, label: Some("610 ms".to_string()), err: true,
            }],
            ticks: vec![(0.0, "0".to_string()), (1.0, "2.21s".to_string())],
            selected: "s1".to_string(),
            on_select: |_| {},
        }
    });
    has(&h, "d-wf-bar is-err is-right"); // ends past 75% → label goes left of the bar
    has(&h, r#"aria-selected="true""#);
    has(&h, "--d:1");
}

#[test]
fn page_head_variants() {
    let h = html!(rsx! {
        PageHead { title: "Key / Value".to_string(), eyebrow: "Control plane".to_string(), description: "desc".to_string(), actions: rsx! { "x" } }
        PageHead { title: "Logs".to_string(), inline: true }
    });
    has(&h, r#"class="d-page-head""#);
    has(&h, "d-page-head d-page-head--inline");
    has(&h, r#"class="d-actions""#);
}

#[test]
fn split_only_grows_a_drawer_column_when_given_one() {
    let h = html!(rsx! {
        Split { width: DrawerWidth::Wide, flush: true, drawer: rsx! { Drawer { label: "Entry".to_string(), title: "cluster/layout".to_string(), on_close: |_| {}, "body" } }, "list" }
    });
    has(&h, "d-split d-split--wide d-split--flush");
    has(&h, r#"<aside class="d-drawer" aria-label="Entry">"#);
    has(&h, r#"class="d-drawer-title""#);
    let bare = html!(rsx! { Split { "list" } });
    lacks(&bare, "d-drawer");
}

#[test]
fn query_bar_shows_the_grammar_label_and_error() {
    let h = html!(rsx! {
        QueryBar { grammar: GrammarId::BeaconLogs, value: use_signal(String::new), on_run: |_| {}, error: "unknown field \"x\"".to_string() }
    });
    has(&h, "BEACONQL");
    has(&h, r#"aria-invalid="true""#);
    has(&h, "unknown field");
}

#[test]
fn filters_show_grammar_chips_and_a_builder_chip_for_predicate_grammars() {
    let p = html!(rsx! { QueryFilters { grammar: GrammarId::Cesql, expression: use_signal(String::new), on_run: |_| {} } });
    has(&p, "Filter…");
    has(&p, "type LIKE");
    let prom = html!(rsx! { QueryFilters { grammar: GrammarId::PromQl, expression: use_signal(String::new), on_run: |_| {} } });
    lacks(&prom, "Filter…");
    has(&prom, "rate(fuse_requests_total[5m])");
}

// ── router-dependent ─────────────────────────────────────────────────────────────────────────────

#[derive(Routable, Clone, PartialEq)]
enum Route {
    #[layout(Layout)]
    #[route("/")]
    Home {},
    #[route("/gateway")]
    Gateway {},
}

#[component]
fn Layout() -> Element {
    let route = use_route::<Route>();
    let current = route.to_string();
    rsx! {
        AppShell {
            app: AppKind::Blueprint,
            current,
            sections: vec![
                NavSection::new("Control plane", vec![NavItem::new("Key / Value", "/").count(6), NavItem::new("Gateway", "/gateway")]),
                NavSection::new("Empty", vec![]),
            ],
            apps: vec![NavItem::app(AppKind::Beacon, "Beacon", "http://beacon.draft.localhost/")],
            Outlet::<Route> {}
        }
    }
}

#[component]
fn Home() -> Element {
    rsx! { "home page" }
}

#[component]
fn Gateway() -> Element {
    rsx! { "gateway page" }
}

fn router_app() -> Element {
    rsx! { Router::<Route> {} }
}

#[test]
fn shell_structure_rail_and_current_page() {
    let h = render(router_app);
    // the CSS contract for the responsive rail: checkbox first, then brand, topbar, rail, main, status bar
    has(&h, r#"class="d-app""#);
    let order = ["d-rail-toggle", "d-brand", "d-topbar", "d-rail", "d-main", "d-statusbar", "d-rail-scrim"];
    let mut last = 0;
    for name in order {
        let at = h.find(&format!("class=\"{name}\"")).unwrap_or_else(|| panic!("missing {name}:\n{h}"));
        assert!(at >= last, "{name} out of order");
        last = at;
    }
    has(&h, "home page");
    // Home is "/" → current; Gateway is not.
    has(&h, r#"aria-current="page""#);
    assert_eq!(h.matches(r#"aria-current="page""#).count(), 1, "{h}");
    // an empty section is not rendered
    lacks(&h, ">Empty<");
    // the Apps block links out, with the ↗ marker class and glyph
    has(&h, "d-nav-item d-nav-item--ext");
    has(&h, r#"target="_blank""#);
    has(&h, ">Bc<"); // Beacon's glyph in the Apps block
}

#[test]
fn shell_accessibility_hooks() {
    let h = render(router_app);
    has(&h, r#"aria-label="Toggle navigation""#);
    has(&h, r#"aria-label="Breadcrumb""#);
    has(&h, r#"aria-label="Navigation""#);
    // the command palette is drawn but inert until it is designed
    has(&h, r#"aria-disabled="true""#);
}
