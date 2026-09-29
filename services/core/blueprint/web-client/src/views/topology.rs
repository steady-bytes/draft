//! Topology: which services produce which event types, and which services consume them. A snapshot
//! from `GetTopology`, refetched whenever `WatchTopology` reports a change. Hover a node to trace
//! its flows; click one to pin it and open its detail.

use std::collections::BTreeSet;

use dioxus::prelude::*;
use draft_api::proto::core_message_broker_actors_v1::{topology_client::TopologyClient, GetTopologyRequest, GetTopologyResponse, WatchTopologyRequest};
use draft_ui::data::{Kv, KvItem, StatTile};
use draft_ui::layout::{Drawer, DrawerBlock, PageHead, Split};
use draft_ui::query::string_literal;
use draft_ui::query::LiteralStyle;
use draft_ui::shell::{use_page_chrome, BarItem, Chrome, ChromeStatus};
use draft_ui::ui::{Alert, Btn, BtnVariant, Empty, Loading, Toggle};
use draft_ui::util::format_count;
use draft_ui::{StatusKind, Tone};
use tonic_web_wasm_client::Client as WasmClient;

use crate::topology::{active_flows, board, display_name, is_wide_event, links, lit_nodes, stroke_width, type_label, Board, Col, Flow, Focus};
use crate::Route as AppRoute;

/// `1 producer`, `5 producers`.
fn plural(n: usize, noun: &str) -> String {
    if n == 1 {
        format!("1 {noun}")
    } else {
        format!("{n} {noun}s")
    }
}

fn flows_from(resp: GetTopologyResponse) -> Vec<Flow> {
    resp.edges
        .into_iter()
        .map(|e| Flow { producer: e.producer_source, consumer: e.consumer_source, event_type: e.event_type, vol: e.vol })
        .collect()
}

fn fetch() -> impl std::future::Future<Output = Result<Vec<Flow>, String>> {
    async {
        TopologyClient::new(WasmClient::new(crate::CATALYST_DOMAIN.clone()))
            .get_topology(GetTopologyRequest {})
            .await
            .map(|r| flows_from(r.into_inner()))
            .map_err(|e| e.message().to_string())
    }
}

#[component]
pub fn Topology() -> Element {
    let navigator = use_navigator();
    let mut flows: Signal<Vec<Flow>> = use_signal(Vec::new);
    let mut loading = use_signal(|| true);
    let mut error: Signal<Option<String>> = use_signal(|| None);
    let mut hover: Signal<Option<Focus>> = use_signal(|| None);
    let mut pinned: Signal<Option<Focus>> = use_signal(|| None);
    // Wide events are Beacon's own telemetry riding the broker and are usually the loudest type,
    // but on a quiet cluster they are the only one — so they are shown by default.
    let mut wide = use_signal(|| true);

    use_effect(move || {
        // A snapshot now, then a fresh snapshot on every change Catalyst reports.
        spawn(async move {
            match fetch().await {
                Ok(list) => {
                    flows.set(list);
                    error.set(None);
                }
                Err(e) => error.set(Some(e)),
            }
            loading.set(false);
        });
        spawn(async move {
            let Ok(resp) = TopologyClient::new(WasmClient::new(crate::CATALYST_DOMAIN.clone())).watch_topology(WatchTopologyRequest {}).await else {
                return;
            };
            let mut stream = resp.into_inner();
            while let Ok(Some(_)) = stream.message().await {
                if let Ok(list) = fetch().await {
                    flows.set(list);
                }
            }
        });
    });

    let visible: Vec<Flow> = flows().into_iter().filter(|f| wide() || !is_wide_event(&f.event_type)).collect();
    let layout = board(&visible);
    let focus = hover().or_else(|| pinned());
    let active = active_flows(&visible, focus.as_ref());
    let active_links = links(&layout, &active);
    let all_links = links(&layout, &visible.iter().collect::<Vec<_>>());
    let lit = lit_nodes(&visible, focus.as_ref());
    let max_vol = all_links.iter().map(|l| l.vol).max().unwrap_or(0);
    let summary = format!(
        "{} · {} · {}",
        plural(layout.producers.len(), "producer"),
        plural(layout.types.len(), "event type"),
        plural(layout.consumers.len(), "consumer")
    );

    use_page_chrome(move || Chrome {
        crumbs: vec!["Blueprint".into(), "Events".into(), "Topology".into()],
        status: Some(ChromeStatus::live("Catalyst · watching")),
        left: vec![BarItem::kv("Source", "catalyst")],
        right: vec![],
    });

    let drawer = pinned().map(|f| {
        let detail = NodeDetail::of(&visible, &f);
        let for_query = f.clone();
        rsx! {
            NodeDrawer {
                key: "{f.id}",
                focus: f,
                detail,
                on_close: move |_| pinned.set(None),
                on_query: move |_| {
                    *crate::PENDING_EVENT_QUERY.write() = Some(format!("type = {}", string_literal(LiteralStyle::SingleQuote, &for_query.id)));
                    navigator.push(AppRoute::Store {});
                },
            }
        }
    });

    rsx! {
        Split { flush: true, drawer,
            PageHead { inline: true, title: "Topology".to_string(),
                span { class: "d-label", "{summary}" }
                span { class: "d-spacer" }
                Toggle { checked: wide(), on_change: move |on| wide.set(on), "Wide events" }
            }

            if let Some(err) = error() {
                Alert { kind: StatusKind::Err, "Could not load the topology from Catalyst: {err}" }
            } else if loading() {
                Loading {}
            } else if visible.is_empty() {
                Empty { title: "No flows".to_string(), "No producer has published an event that a consumer receives." }
            } else {
                div { class: "d-panel tp-board",
                    TopologyBoard {
                        board: layout.clone(),
                        links: all_links.clone(),
                        active: active_links.iter().map(|l| (l.from.clone(), l.to.clone())).collect::<Vec<_>>(),
                        lit: lit.iter().cloned().collect::<Vec<_>>(),
                        focused: focus.is_some(),
                        pinned: pinned(),
                        max_vol,
                        on_hover: move |f: Option<Focus>| hover.set(f),
                        on_pin: move |f: Focus| {
                            if pinned.peek().as_ref() == Some(&f) {
                                pinned.set(None);
                            } else {
                                pinned.set(Some(f));
                            }
                        },
                    }
                }
                div { class: "tp-legend",
                    span { svg { view_box: "0 0 28 10", path { d: "M0 5 H28", stroke_width: "1" } } "quiet" }
                    span { svg { view_box: "0 0 28 10", path { d: "M0 5 H28", stroke_width: "3" } } "busier" }
                    span { svg { view_box: "0 0 28 10", path { d: "M0 5 H28", stroke_width: "5" } } "busiest" }
                    span { "Line colour is the event type. Hover a node to trace its flow; click to pin it." }
                }
            }
        }
    }
}

const W: f64 = 1000.0;
const NODE_W: f64 = 230.0;
const NODE_H: f64 = 30.0;
const STEP: f64 = 46.0;
const TOP: f64 = 40.0;

fn col_x(col: Col) -> f64 {
    match col {
        Col::Producer => 0.0,
        Col::Type => (W - NODE_W) / 2.0,
        Col::Consumer => W - NODE_W,
    }
}

fn node_y(idx: usize) -> f64 {
    TOP + idx as f64 * STEP
}

fn node_tone(col: Col, id: &str) -> Tone {
    match col {
        Col::Type => Tone::for_name(id),
        _ => Tone::for_name(display_name(id)),
    }
}

#[component]
fn TopologyBoard(
    board: Board,
    links: Vec<crate::topology::Link>,
    /// The links a focus keeps lit, as (from, to).
    active: Vec<((Col, usize), (Col, usize))>,
    lit: Vec<(Col, String)>,
    focused: bool,
    pinned: Option<Focus>,
    max_vol: u32,
    on_hover: EventHandler<Option<Focus>>,
    on_pin: EventHandler<Focus>,
) -> Element {
    let rows = board.producers.len().max(board.types.len()).max(board.consumers.len()).max(1);
    let height = TOP + rows as f64 * STEP;
    let lit: BTreeSet<(Col, String)> = lit.into_iter().collect();
    let columns = [(Col::Producer, "Producers", &board.producers), (Col::Type, "Event types", &board.types), (Col::Consumer, "Consumers", &board.consumers)];

    rsx! {
        svg { class: "tp-svg", view_box: "0 0 {W} {height}", role: "group", "aria-label": "Event flow from producing services through event types to consuming services",
            for (col , title , _) in columns.iter() {
                text { class: "tp-col", x: "{col_x(*col)}", y: "18", "{title}" }
            }
            g { class: "tp-links",
                for (i , l) in links.iter().enumerate() {
                    {
                        let (fx, fy) = (col_x(l.from.0) + NODE_W, node_y(l.from.1) + NODE_H / 2.0);
                        let (tx, ty) = (col_x(l.to.0), node_y(l.to.1) + NODE_H / 2.0);
                        let mid = (fx + tx) / 2.0;
                        let tone = Tone::for_name(&board.types[l.type_idx]);
                        let dim = focused && !active.contains(&(l.from.clone(), l.to.clone()));
                        let class = if dim { "tp-link is-dim" } else { "tp-link" };
                        let width = stroke_width(l.vol, max_vol);
                        rsx! {
                            path { key: "{i}", class: "{class}", d: "M{fx} {fy} C{mid} {fy} {mid} {ty} {tx} {ty}", style: "--c:{tone.css_var()}", stroke_width: "{width}" }
                        }
                    }
                }
            }
            for (col , _ , ids) in columns.iter() {
                for (i , id) in ids.iter().enumerate() {
                    {
                        let col = *col;
                        let focus = Focus::new(col, id.clone());
                        let is_pinned = pinned.as_ref() == Some(&focus);
                        let dim = focused && !lit.contains(&(col, id.clone()));
                        let class = format!("tp-node{}{}", if dim { " is-dim" } else { "" }, if is_pinned { " is-pinned" } else { "" });
                        let tone = node_tone(col, id);
                        let label = if col == Col::Type { type_label(id).to_string() } else { display_name(id).to_string() };
                        let x = col_x(col);
                        let y = node_y(i);
                        let (f_enter, f_leave, f_click, f_key) = (focus.clone(), focus.clone(), focus.clone(), focus.clone());
                        rsx! {
                            g {
                                key: "{id}",
                                class: "{class}",
                                style: "--c:{tone.css_var()}",
                                role: "button",
                                tabindex: "0",
                                "aria-pressed": "{is_pinned}",
                                "aria-label": "{label}",
                                onmouseenter: move |_| on_hover.call(Some(f_enter.clone())),
                                onmouseleave: move |_| on_hover.call(None),
                                onfocus: move |_| on_hover.call(Some(f_leave.clone())),
                                onblur: move |_| on_hover.call(None),
                                onclick: move |_| on_pin.call(f_click.clone()),
                                onkeydown: move |k| {
                                    if k.key() == Key::Enter || k.key() == Key::Character(" ".to_string()) {
                                        k.prevent_default();
                                        on_pin.call(f_key.clone());
                                    }
                                },
                                title { "{id}" }
                                rect { x: "{x}", y: "{y}", width: "{NODE_W}", height: "{NODE_H}", rx: "2" }
                                circle { cx: "{x + 14.0}", cy: "{y + NODE_H / 2.0}", r: "3.5" }
                                text { x: "{x + 26.0}", y: "{y + NODE_H / 2.0 + 4.0}", "{label}" }
                            }
                        }
                    }
                }
            }
        }
    }
}

/// What the drawer shows about a picked node.
#[derive(Clone, Debug, PartialEq)]
struct NodeDetail {
    volume: u32,
    producers: Vec<String>,
    types: Vec<String>,
    consumers: Vec<String>,
}

impl NodeDetail {
    fn of(flows: &[Flow], focus: &Focus) -> Self {
        let active = active_flows(flows, Some(focus));
        let distinct = |f: fn(&Flow) -> &String| -> Vec<String> {
            let set: BTreeSet<&String> = active.iter().map(|x| f(x)).collect();
            set.into_iter().cloned().collect()
        };
        Self {
            volume: active.iter().map(|f| f.vol).sum(),
            producers: distinct(|f| &f.producer),
            types: distinct(|f| &f.event_type),
            consumers: distinct(|f| &f.consumer),
        }
    }
}

#[component]
fn NodeDrawer(focus: Focus, detail: NodeDetail, on_close: EventHandler<()>, on_query: EventHandler<()>) -> Element {
    let kind = match focus.col {
        Col::Producer => "Producer",
        Col::Type => "Event type",
        Col::Consumer => "Consumer",
    };
    let title = if focus.col == Col::Type { focus.id.clone() } else { display_name(&focus.id).to_string() };
    let subtitle = if focus.col == Col::Type { String::new() } else { focus.id.clone() };
    let volume = if detail.volume == 0 { "not measured".to_string() } else { format_count(u64::from(detail.volume)) };
    let names = |list: &[String], short: bool| list.iter().map(|x| if short { display_name(x).to_string() } else { type_label(x).to_string() }).collect::<Vec<_>>().join(", ");
    let producers = names(&detail.producers, true);
    let consumers = names(&detail.consumers, true);
    let types = names(&detail.types, false);
    let lead = rsx! {
        span { class: "d-label", "{kind}" }
    };
    rsx! {
        Drawer { label: "Topology detail".to_string(), on_close: move |_| on_close.call(()), lead, title, subtitle,
            div { class: "d-stats", style: "grid-template-columns:1fr 1fr; margin-top:16px",
                StatTile { label: "Volume".to_string(), value: volume }
                StatTile { label: "Event types".to_string(), value: detail.types.len().to_string() }
            }
            DrawerBlock { title: "Flow".to_string(),
                Kv {
                    if focus.col != Col::Producer {
                        KvItem { label: "producers".to_string(), "{producers}" }
                    }
                    if focus.col != Col::Type {
                        KvItem { label: "event types".to_string(), "{types}" }
                    }
                    if focus.col != Col::Consumer {
                        KvItem { label: "consumers".to_string(), "{consumers}" }
                    }
                }
            }
            if focus.col == Col::Type {
                div { class: "tp-actions",
                    Btn { variant: BtnVariant::Primary, onclick: move |_| on_query.call(()), "Query these events" }
                }
            }
        }
    }
}
