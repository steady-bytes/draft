//! Service registry: every process registered with Blueprint, grouped by service. Seeded by
//! `Query` and kept current by the `Watch` stream. A row expands to its instances (one per pid: a
//! restart or a replica each add one), so `/service-registry/:name` is this page with that
//! service's row open.

use std::collections::{HashMap, HashSet};

use chrono::Utc;
use dioxus::prelude::*;
use draft_api::hook::core_registry_service_discovery_v1::{filter, use_service_discovery_service_service, Filter, QueryRequest};
use draft_api::proto::core_registry_service_discovery_v1::{
    service_discovery_service_client::ServiceDiscoveryServiceClient, LeadershipStatus, Process, WatchRequest,
};
use draft_ui::data::{CellState, List, ListRow, Strip, StripCell, StripSize, StatTile, Delta};
use draft_ui::layout::{PageHead, Toolbar};
use draft_ui::shell::{use_page_chrome, BarItem};
use draft_ui::ui::{Empty, Glyph, Loading, Seg, Status, Tag, TextInput, Toggle};
use draft_ui::util::{relative_time, truncate_middle};
use draft_ui::{AppKind, StatusKind, Tone};
use gloo_timers::future::TimeoutFuture;
use tonic_web_wasm_client::Client as WasmClient;

use crate::raft::use_raft;
use crate::registry::{address_summary, group, heartbeat_age, instance_state, Domain, Group, InstanceState};

/// How often the heartbeat column recomputes "Ns ago".
const TICK_MS: u32 = 5_000;

/// `/service-registry`.
#[component]
pub fn ServiceRegistry() -> Element {
    rsx! { RegistryPage { open: None } }
}

/// `/service-registry/:name` — the registry with that service expanded.
#[component]
pub fn ServiceDetail(name: String) -> Element {
    rsx! { RegistryPage { open: Some(name) } }
}

fn state_kind(s: InstanceState) -> StatusKind {
    match s {
        InstanceState::Healthy => StatusKind::Ok,
        InstanceState::Starting => StatusKind::Info,
        InstanceState::Stale => StatusKind::Warn,
        InstanceState::Unhealthy | InstanceState::Disconnected => StatusKind::Err,
    }
}

fn cell_state(s: InstanceState) -> CellState {
    match s {
        InstanceState::Healthy => CellState::Ok,
        InstanceState::Starting => CellState::Run,
        InstanceState::Stale => CellState::Warn,
        InstanceState::Unhealthy | InstanceState::Disconnected => CellState::Err,
    }
}

/// The glyph beside a service name: the app's own code and colour when it is a Draft app, a
/// domain code otherwise.
fn glyph(name: &str, domain: Domain) -> (String, Tone) {
    match AppKind::from_name(name) {
        AppKind::Service => match domain {
            Domain::Plugin => ("Pl".to_string(), Tone::Quiet),
            Domain::Example => ("Ex".to_string(), Tone::Quiet),
            _ => ("Sv".to_string(), Tone::Sv),
        },
        app => (app.code().to_string(), app.tone()),
    }
}

/// The group's worst instance state, which is what its Health cell shows.
fn worst(g: &Group, now: i64) -> InstanceState {
    let rank = |s: InstanceState| match s {
        InstanceState::Disconnected => 4,
        InstanceState::Unhealthy => 3,
        InstanceState::Stale => 2,
        InstanceState::Starting => 1,
        InstanceState::Healthy => 0,
    };
    g.instances.iter().map(|p| instance_state(p, now)).max_by_key(|s| rank(*s)).unwrap_or(InstanceState::Healthy)
}

/// `Healthy`, `Stale · 42s`, `Unhealthy` — the health cell's text.
fn health_text(state: InstanceState, age: Option<i64>) -> String {
    match (state, age) {
        (InstanceState::Stale, Some(a)) => format!("Stale · {a}s"),
        _ => state.label().to_string(),
    }
}

#[component]
fn RegistryPage(open: Option<String>) -> Element {
    let raft = use_raft();
    let mut filter_text = use_signal(String::new);
    let mut domain: Signal<Option<Domain>> = use_signal(|| None);
    let mut problems_only = use_signal(|| false);
    let mut expanded: Signal<HashSet<String>> = use_signal(move || open.clone().into_iter().collect());
    let mut now = use_signal(|| Utc::now().timestamp());

    let query_request = use_signal(|| QueryRequest { filter: Some(Filter { attribute: Some(filter::Attribute::All(String::new())) }) });
    let service = use_service_discovery_service_service();
    let query_result = service.query(query_request);

    // The registry keyed by pid: seeded from Query, then upserted or removed as Watch reports.
    let mut processes: Signal<HashMap<String, Process>> = use_signal(HashMap::new);
    use_effect(move || {
        if let Some(Ok(ref resp)) = *query_result.read() {
            *processes.write() = resp.data.clone();
        }
    });
    use_coroutine(move |_rx: UnboundedReceiver<()>| async move {
        let mut client = ServiceDiscoveryServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
        let Ok(response) = client.watch(WatchRequest {}).await else {
            return;
        };
        let mut stream = response.into_inner();
        while let Ok(Some(msg)) = stream.message().await {
            let Some(process) = msg.process else { continue };
            if msg.removed {
                processes.write().remove(&process.pid);
            } else {
                processes.write().insert(process.pid.clone(), process);
            }
        }
    });
    // "Ns ago" and staleness are functions of the clock, so the clock ticks.
    use_future(move || async move {
        loop {
            TimeoutFuture::new(TICK_MS).await;
            now.set(Utc::now().timestamp());
        }
    });

    // Derived ---------------------------------------------------------------------------------
    let now_secs = now();
    let all_groups = group(processes.read().values().cloned());
    let total_instances: usize = all_groups.iter().map(|g| g.instances.len()).sum();
    let healthy_instances: usize = all_groups.iter().map(|g| g.healthy(now_secs)).sum();
    let problem_names: Vec<String> = all_groups.iter().filter(|g| g.has_problem(now_secs)).map(|g| g.name.clone()).collect();
    let by_domain: Vec<String> = [Domain::Core, Domain::Tooling, Domain::Plugin, Domain::Example, Domain::Other]
        .into_iter()
        .filter_map(|d| {
            let n = all_groups.iter().filter(|g| g.domain == d).count();
            (n > 0).then(|| format!("{n} {}", d.label().to_lowercase()))
        })
        .collect();
    let biggest = all_groups.iter().max_by_key(|g| g.instances.len()).filter(|g| g.instances.len() > 1);
    let newest_join = processes.read().values().filter_map(|p| p.joined_time.as_ref().map(|t| (t.seconds, p.name.clone()))).max();

    let needle = filter_text().to_lowercase();
    let shown: Vec<Group> = all_groups
        .iter()
        .filter(|g| needle.is_empty() || g.name.to_lowercase().contains(&needle))
        .filter(|g| domain().map_or(true, |d| g.domain == d))
        .filter(|g| !problems_only() || g.has_problem(now_secs))
        .cloned()
        .collect();
    let loading = query_result.read().is_none() && all_groups.is_empty();

    // The closure re-runs when what it reads changes, so it reads the live registry itself rather
    // than a count captured at render time.
    use_page_chrome(move || {
        let services = group(processes.read().values().cloned()).len();
        let mut chrome = raft.chrome(&["Blueprint", "Control plane", "Service registry"]);
        chrome.left = vec![BarItem::kv("Services", services.to_string())];
        chrome
    });

    let domain_options: Vec<(Option<Domain>, String)> = vec![
        (None, "All".to_string()),
        (Some(Domain::Core), "Core".to_string()),
        (Some(Domain::Tooling), "Tooling".to_string()),
        (Some(Domain::Plugin), "Plugins".to_string()),
        (Some(Domain::Example), "Examples".to_string()),
    ];
    let raft_nodes = raft.info().map(|i| i.nodes).unwrap_or_default();
    let show_raft = shown.iter().any(|g| g.name == "blueprint") && !raft_nodes.is_empty();

    rsx! {
        PageHead {
            title: "Service registry".to_string(),
            eyebrow: "Control plane".to_string(),
            description: "Every process that has registered with Blueprint, grouped by service. Instances report a heartbeat every 5 seconds and are marked stale after 15.".to_string(),
        }

        div { class: "d-stats",
            StatTile {
                label: "Services".to_string(),
                value: all_groups.len().to_string(),
                delta: if by_domain.is_empty() { None } else { Some(Delta::neutral(by_domain.join(" · "))) },
            }
            StatTile {
                label: "Instances".to_string(),
                value: total_instances.to_string(),
                delta: biggest.map(|g| Delta::neutral(format!("{} runs {}", g.name, g.instances.len()))),
            }
            StatTile {
                label: "Healthy".to_string(),
                value: healthy_instances.to_string(),
                unit: format!("/ {total_instances}"),
                delta: if problem_names.is_empty() { None } else { Some(Delta::bad(format!("{} with problems · {}", problem_names.len(), problem_names.join(", ")))) },
            }
            StatTile {
                label: "Last change".to_string(),
                value: newest_join.as_ref().map(|(t, _)| relative_time((now_secs - t).max(0))).unwrap_or_else(|| "—".to_string()),
                delta: newest_join.as_ref().map(|(_, n)| Delta::neutral(format!("{n} registered"))),
            }
        }

        div { style: "margin-top:20px" }
        Toolbar {
            TextInput { value: filter_text(), oninput: move |v| filter_text.set(v), placeholder: "Filter services…".to_string(), aria_label: "Filter services".to_string() }
            Seg::<Option<Domain>> { options: domain_options, value: domain(), on_change: move |d| domain.set(d), label: "Domain".to_string() }
            span { class: "d-spacer" }
            Toggle { checked: problems_only(), on_change: move |on| problems_only.set(on), "Problems only" }
        }

        if loading {
            Loading {}
        } else if shown.is_empty() {
            Empty { title: "No services".to_string(),
                if all_groups.is_empty() { "No processes have registered yet." } else { "No service matches the filters." }
            }
        } else {
            div { class: "d-panel d-table-wrap",
                table { class: "d-table sr-table",
                    thead {
                        tr {
                            th { "Service" }
                            th { "Domain" }
                            th { "Instances" }
                            th { "Health" }
                            th { "Address" }
                            th { class: "is-right", "Last heartbeat" }
                        }
                    }
                    tbody {
                        for g in shown {
                            {
                                let is_open = expanded().contains(&g.name);
                                let name_toggle = g.name.clone();
                                let name_key = g.name.clone();
                                let (code, tone) = glyph(&g.name, g.domain);
                                let state = worst(&g, now_secs);
                                let beat = g.last_heartbeat(now_secs);
                                let stale_age = g.instances.iter().filter(|p| instance_state(p, now_secs) == InstanceState::Stale).filter_map(|p| heartbeat_age(p, now_secs)).max();
                                let health = health_text(state, stale_age);
                                let cells: Vec<StripCell> = g.instances.iter().map(|p| {
                                    let s = instance_state(p, now_secs);
                                    StripCell::titled(cell_state(s), format!("{} · {}", truncate_middle(&p.pid, 8, 0), s.label()))
                                }).collect();
                                let healthy = g.healthy(now_secs);
                                let total = g.instances.len();
                                let address = address_summary(&g);
                                let beat_text = beat.map(relative_time).unwrap_or_else(|| "—".to_string());
                                let caret = if is_open { "▼" } else { "▶" };
                                let children = if is_open { g.instances.clone() } else { Vec::new() };
                                rsx! {
                                    Fragment { key: "{g.name}",
                                        tr {
                                            class: if is_open { "is-open" } else { "" },
                                            tabindex: "0",
                                            aria_expanded: "{is_open}",
                                            onclick: move |_| {
                                                let mut set = expanded.write();
                                                if !set.remove(&name_toggle) {
                                                    set.insert(name_toggle.clone());
                                                }
                                            },
                                            onkeydown: move |k| {
                                                if k.key() == Key::Enter || k.key() == Key::Character(" ".to_string()) {
                                                    k.prevent_default();
                                                    let mut set = expanded.write();
                                                    if !set.remove(&name_key) {
                                                        set.insert(name_key.clone());
                                                    }
                                                }
                                            },
                                            td { class: "name",
                                                span { class: "d-row",
                                                    span { class: "caret", aria_hidden: "true", "{caret}" }
                                                    Glyph { code, tone }
                                                    "{g.name}"
                                                }
                                            }
                                            td { Tag { tone: Tone::Quiet, "{g.domain.label()}" } }
                                            td {
                                                span { class: "inst", Strip { cells, size: StripSize::Md, label: format!("{healthy} of {total} instances healthy") } }
                                                "{healthy} / {total}"
                                            }
                                            td { Status { kind: state_kind(state), "{health}" } }
                                            td { class: "is-dim", "{address}" }
                                            td { class: "is-right beat", "{beat_text}" }
                                        }
                                        for p in children {
                                            {
                                                let s = instance_state(&p, now_secs);
                                                let age = heartbeat_age(&p, now_secs);
                                                let pid = truncate_middle(&p.pid, 8, 0);
                                                let running = draft_api::proto::core_registry_service_discovery_v1::ProcessRunningState::try_from(p.running_state)
                                                    .map(|r| r.as_str_name().trim_start_matches("PROCESS_").to_lowercase())
                                                    .unwrap_or_else(|_| "unknown".to_string());
                                                let text = health_text(s, age);
                                                let beat_child = age.map(relative_time).unwrap_or_else(|| "—".to_string());
                                                rsx! {
                                                    tr { key: "{p.pid}", class: "child",
                                                        td { title: "{p.pid}", "{pid}" }
                                                        td { class: "role", "{running}" }
                                                        td {}
                                                        td { Status { kind: state_kind(s), "{text}" } }
                                                        td { class: "is-dim", "{p.ip_address}" }
                                                        td { class: "is-right beat", "{beat_child}" }
                                                    }
                                                }
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }

        if show_raft {
            div { class: "d-panel", style: "margin-top:16px",
                div { class: "d-panel-head", span { class: "d-label", "Raft cluster · {raft_nodes.len()} nodes" } }
                List {
                    for node in raft_nodes {
                        {
                            let leader = node.leadership_status == LeadershipStatus::Leader as i32;
                            rsx! {
                                ListRow { key: "{node.id}",
                                    span { class: "d-trunc", "{node.id}" }
                                    span { class: "d-spacer" }
                                    small { "{node.address}" }
                                    if leader {
                                        Tag { tone: Tone::Primary, "Leader" }
                                    } else {
                                        Tag { tone: Tone::Quiet, "Follower" }
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}
