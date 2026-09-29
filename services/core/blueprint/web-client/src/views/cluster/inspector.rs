#![allow(unused_imports)]
use super::*;
use draft_ui::data::{Kv, KvItem, List, ListRow};
use draft_ui::layout::DrawerBlock;
use draft_ui::ui::{Field, Select, Tag, TextInput};
use draft_ui::Tone;

/// The side panel for the selected node.
///
/// A **live** node is real cluster state, so the panel shows real facts: where it is, what it is
/// linked to, the gateway routes Fuse serves, the event types Catalyst carries. A **manual** node is
/// a hand-added annotation, so the editors for its (local, unsaved) routing rules, topics, endpoint
/// and lease TTL live here.
#[component]
pub(super) fn Inspector(
    node: ClNode,
    nodes: Vec<ClNode>,
    traces: Vec<ClTrace>,
    routes: Vec<GwRoute>,
    topology: TopologyData,
    on_close: EventHandler<()>,
    on_rules: EventHandler<Vec<RoutingRule>>,
    on_topics: EventHandler<Vec<Topic>>,
    on_endpoint: EventHandler<(String, u16, String)>,
    on_ttl: EventHandler<String>,
    on_rm_trace: EventHandler<String>,
) -> Element {
    let live = node.origin == NodeOrigin::Live;
    let tone = node.kind.tone();
    let (status_kind, status_text) = if node.online { (StatusKind::Ok, "Online") } else { (StatusKind::Err, "Offline") };
    let name_of = |id: &str| nodes.iter().find(|n| n.id == id).map(|n| n.name.clone()).unwrap_or_else(|| id.to_string());
    let manual_ids: Vec<&str> = nodes.iter().filter(|n| n.origin == NodeOrigin::Manual).map(|n| n.id.as_str()).collect();

    // Every wire and event flow touching this node, as (trace, peer's name, direction).
    let mut links: Vec<(ClTrace, String, &'static str)> = traces
        .iter()
        .filter(|t| t.from == node.id || t.to == node.id)
        .map(|t| {
            let (peer, dir) = if t.from == node.id { (name_of(&t.to), "→") } else { (name_of(&t.from), "←") };
            (t.clone(), peer, dir)
        })
        .collect();
    links.sort_by(|a, b| a.1.cmp(&b.1));

    let address = if node.host.is_empty() { "—".to_string() } else if node.port > 0 { format!("{}:{}", node.host, node.port) } else { node.host.clone() };
    let origin = if live { "Live · tracks the service registry" } else { "Manual · drawn on this canvas only" };
    let title = node.name.clone();
    let role = node.kind.role();

    rsx! {
        aside { class: "cv-inspector", aria_label: "Node inspector",
            div { class: "cv-inspector-head",
                Glyph { code: node.kind.sym().to_string(), tone, large: true }
                div { class: "cv-title",
                    div { class: "cv-name", title: "{title}", "{title}" }
                    span { class: "d-label", "{role}" }
                }
                span { class: "d-spacer" }
                Btn { variant: BtnVariant::Ghost, size: BtnSize::Sm, icon: true, aria_label: "Close inspector".to_string(), onclick: move |_| on_close.call(()), "✕" }
            }
            div { class: "cv-inspector-body",
                Status { kind: status_kind, "{status_text}" }
                Kv {
                    KvItem { label: "origin".to_string(), "{origin}" }
                    KvItem { label: "address".to_string(), "{address}" }
                    KvItem { label: "links".to_string(), "{node.wire_count(&traces)} wires · {node.event_count(&traces)} event flows" }
                }

                if !links.is_empty() {
                    DrawerBlock { title: "Links".to_string(),
                        List { flush: true,
                            for (t , peer , dir) in links {
                                {
                                    let removable = manual_ids.contains(&t.from.as_str()) || manual_ids.contains(&t.to.as_str());
                                    // An event type is shown by its last segment; the full name is the tooltip.
                                    let full = t.topic.clone().unwrap_or_else(|| "wire".to_string());
                                    let what = full.rsplit('.').next().unwrap_or(&full).to_string();
                                    let tid = t.id.clone();
                                    rsx! {
                                        ListRow { key: "{t.id}",
                                            span { class: "d-muted", "{dir}" }
                                            span { class: "d-trunc", "{peer}" }
                                            span { class: "d-spacer" }
                                            Tag { tone: if t.kind == TraceKind::Event { Tone::Ca } else { Tone::Quiet }, title: full.clone(), "{what}" }
                                            if removable {
                                                Btn { variant: BtnVariant::Ghost, size: BtnSize::Sm, icon: true, aria_label: format!("Remove link to {peer}"), onclick: move |_| on_rm_trace.call(tid.clone()), "×" }
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    }
                }

                match node.kind {
                    NodeKind::Fuse if live => rsx! { FuseRoutes { routes: routes.clone() } },
                    NodeKind::Catalyst if live => rsx! { CatalystTypes { topology: topology.clone() } },
                    NodeKind::Blueprint => rsx! { BlueprintRegistry { node: node.clone(), nodes: nodes.clone(), live, on_ttl } },
                    NodeKind::Fuse => rsx! { RuleEditor { node: node.clone(), nodes: nodes.clone(), on_rules } },
                    NodeKind::Catalyst => rsx! { TopicEditor { node: node.clone(), on_topics } },
                    NodeKind::Service if !live => rsx! { EndpointEditor { node: node.clone(), on_endpoint } },
                    _ => rsx! {},
                }
            }
        }
    }
}

/// The gateway routes Fuse is serving, from its route table.
#[component]
fn FuseRoutes(routes: Vec<GwRoute>) -> Element {
    rsx! {
        DrawerBlock { title: format!("Gateway routes · {}", routes.len()),
            if routes.is_empty() {
                span { class: "d-muted", "Fuse reports no routes." }
            } else {
                List { flush: true,
                    for r in routes {
                        {
                            let target = r.endpoint.as_ref().map(|e| format!("{}:{}", e.host, e.port)).unwrap_or_else(|| "—".to_string());
                            rsx! {
                                ListRow { key: "{r.name}",
                                    span { class: "d-trunc", "{r.name}" }
                                    span { class: "d-spacer" }
                                    small { "{target}" }
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}

/// The event types Catalyst carries, from the topology.
#[component]
fn CatalystTypes(topology: TopologyData) -> Element {
    let types: Vec<String> = topology.unique_event_types().into_iter().map(String::from).collect();
    rsx! {
        DrawerBlock { title: format!("Event types · {}", types.len()),
            if types.is_empty() {
                span { class: "d-muted", "No event flows yet." }
            } else {
                List { flush: true,
                    for t in types {
                        ListRow { key: "{t}", span { class: "d-trunc", "{t}" } }
                    }
                }
            }
        }
    }
}

/// The processes Blueprint is tracking, and (for a hand-added node) its lease TTL.
#[component]
fn BlueprintRegistry(node: ClNode, nodes: Vec<ClNode>, live: bool, on_ttl: EventHandler<String>) -> Element {
    let listed: Vec<ClNode> = nodes.iter().filter(|n| n.id != node.id).cloned().collect();
    rsx! {
        DrawerBlock { title: format!("Service registry · {}", listed.len()),
            if listed.is_empty() {
                span { class: "d-muted", "No other nodes on the canvas." }
            } else {
                List { flush: true,
                    for svc in listed {
                        {
                            let addr = if svc.host.is_empty() { "—".to_string() } else { svc.host.clone() };
                            let (kind, text) = if svc.online { (StatusKind::Ok, "Online") } else { (StatusKind::Err, "Offline") };
                            rsx! {
                                ListRow { key: "{svc.id}",
                                    Status { kind, "{text}" }
                                    span { class: "d-trunc", "{svc.name}" }
                                    span { class: "d-spacer" }
                                    small { "{addr}" }
                                }
                            }
                        }
                    }
                }
            }
        }
        if !live {
            DrawerBlock { title: "Health checks".to_string(),
                Field { label: "Lease TTL".to_string(), TextInput { value: node.ttl.clone(), oninput: move |v| on_ttl.call(v) } }
            }
        }
    }
}

/// Routing rules of a hand-added Fuse: first match wins.
#[component]
fn RuleEditor(node: ClNode, nodes: Vec<ClNode>, on_rules: EventHandler<Vec<RoutingRule>>) -> Element {
    let rules = node.rules.clone();
    let targets: Vec<String> = nodes.iter().filter(|n| n.id != node.id && n.kind != NodeKind::Fuse).map(|n| n.name.clone()).collect();
    let methods: Vec<(String, String)> = ["GET", "POST", "PUT", "DELETE", "*"].iter().map(|m| (m.to_string(), m.to_string())).collect();
    let first_target = targets.first().cloned().unwrap_or_else(|| "svc".to_string());
    rsx! {
        DrawerBlock { title: "Routing rules".to_string(),
            for (i , rule) in rules.iter().enumerate() {
                {
                    let mut target_options: Vec<(String, String)> = targets.iter().map(|t| (t.clone(), t.clone())).collect();
                    if !targets.contains(&rule.target) {
                        target_options.insert(0, (rule.target.clone(), rule.target.clone()));
                    }
                    let (a, b, c, d) = (rules.clone(), rules.clone(), rules.clone(), rules.clone());
                    rsx! {
                        div { key: "{i}", class: "cv-rule",
                            Select { value: rule.method.clone(), options: methods.clone(), aria_label: "Method".to_string(),
                                on_change: move |v: String| { let mut x = a.clone(); x[i].method = v; on_rules.call(x); } }
                            TextInput { value: rule.path.clone(), aria_label: "Path prefix".to_string(),
                                oninput: move |v: String| { let mut x = b.clone(); x[i].path = v; on_rules.call(x); } }
                            Select { value: rule.target.clone(), options: target_options, aria_label: "Target".to_string(),
                                on_change: move |v: String| { let mut x = c.clone(); x[i].target = v; on_rules.call(x); } }
                            Btn { variant: BtnVariant::Ghost, size: BtnSize::Sm, icon: true, aria_label: "Remove rule".to_string(),
                                onclick: move |_| { let mut x = d.clone(); x.remove(i); on_rules.call(x); }, "×" }
                        }
                    }
                }
            }
            Btn { variant: BtnVariant::Ghost, size: BtnSize::Sm,
                onclick: move |_| {
                    let mut x = rules.clone();
                    x.push(RoutingRule { method: "GET".into(), path: "/api/".into(), target: first_target.clone() });
                    on_rules.call(x);
                },
                "+ Add rule"
            }
            p { class: "d-hint", "Rules resolve targets through Blueprint. First match wins." }
        }
    }
}

/// Topics of a hand-added Catalyst.
#[component]
fn TopicEditor(node: ClNode, on_topics: EventHandler<Vec<Topic>>) -> Element {
    let topics = node.topics.clone();
    rsx! {
        DrawerBlock { title: "Topics".to_string(),
            for (i , tp) in topics.iter().enumerate() {
                {
                    let (a, b, c, d) = (topics.clone(), topics.clone(), topics.clone(), topics.clone());
                    rsx! {
                        div { key: "{i}", class: "cv-rule",
                            TextInput { value: tp.name.clone(), aria_label: "Topic name".to_string(),
                                oninput: move |v: String| { let mut x = a.clone(); x[i].name = v; on_topics.call(x); } }
                            TextInput { value: tp.retention.clone(), aria_label: "Retention".to_string(),
                                oninput: move |v: String| { let mut x = b.clone(); x[i].retention = v; on_topics.call(x); } }
                            TextInput { r#type: "number".to_string(), value: tp.partitions.to_string(), aria_label: "Partitions".to_string(),
                                oninput: move |v: String| { if let Ok(n) = v.parse() { let mut x = c.clone(); x[i].partitions = n; on_topics.call(x); } } }
                            Btn { variant: BtnVariant::Ghost, size: BtnSize::Sm, icon: true, aria_label: "Remove topic".to_string(),
                                onclick: move |_| { let mut x = d.clone(); x.remove(i); on_topics.call(x); }, "×" }
                        }
                    }
                }
            }
            Btn { variant: BtnVariant::Ghost, size: BtnSize::Sm,
                onclick: move |_| {
                    let mut x = topics.clone();
                    x.push(Topic { name: "new.topic".into(), retention: "24h".into(), partitions: 1 });
                    on_topics.call(x);
                },
                "+ Add topic"
            }
            p { class: "d-hint", "Topics persist as CloudEvents. Consumers track per-destination cursors." }
        }
    }
}

/// Endpoint of a hand-added service.
#[component]
fn EndpointEditor(node: ClNode, on_endpoint: EventHandler<(String, u16, String)>) -> Element {
    let (host, port, proto) = (node.host.clone(), node.port, node.protocol.clone());
    let protocols: Vec<(String, String)> = ["grpc", "grpc-web", "http"].iter().map(|p| (p.to_string(), p.to_string())).collect();
    let (h1, p1) = (host.clone(), port);
    let (h2, pr2) = (host.clone(), proto.clone());
    let pr1 = proto.clone();
    rsx! {
        DrawerBlock { title: "Endpoint".to_string(),
            Field { label: "Host".to_string(),
                TextInput { value: host.clone(), oninput: move |v: String| on_endpoint.call((v, port, proto.clone())) }
            }
            Field { label: "Port".to_string(),
                TextInput { r#type: "number".to_string(), value: port.to_string(),
                    oninput: move |v: String| { if let Ok(p) = v.parse() { on_endpoint.call((h2.clone(), p, pr2.clone())); } } }
            }
            Field { label: "Protocol".to_string(),
                Select { value: pr1.clone(), options: protocols, on_change: move |v: String| on_endpoint.call((h1.clone(), p1, v)) }
            }
            p { class: "d-hint", "On deploy, this service self-registers with Blueprint and becomes routable through Fuse." }
        }
    }
}
