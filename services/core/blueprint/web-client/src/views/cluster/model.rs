#![allow(unused_imports)]
use super::*;

// ── ID generator ─────────────────────────────────────────────────────────────

pub(super) static UID: AtomicU64 = AtomicU64::new(1);
pub(super) fn next_id() -> String {
    format!("n{}", UID.fetch_add(1, Ordering::Relaxed))
}
pub(super) fn uid_val() -> u64 {
    UID.load(Ordering::Relaxed)
}

// ── Node kind ─────────────────────────────────────────────────────────────────

#[derive(Clone, Debug, PartialEq)]
pub(super) enum NodeKind {
    Catalyst,
    Blueprint,
    Fuse,
    Service,
}

impl NodeKind {
    pub(super) fn sym(&self) -> &'static str {
        match self {
            Self::Catalyst => "Ca",
            Self::Blueprint => "Bp",
            Self::Fuse => "Fs",
            Self::Service => "Sv",
        }
    }
    pub(super) fn el(&self) -> &'static str {
        match self {
            Self::Catalyst => "CATALYST",
            Self::Blueprint => "BLUEPRINT",
            Self::Fuse => "FUSE",
            Self::Service => "SERVICE",
        }
    }
    pub(super) fn num(&self) -> &'static str {
        match self {
            Self::Catalyst => "01",
            Self::Blueprint => "02",
            Self::Fuse => "03",
            Self::Service => "··",
        }
    }
    pub(super) fn role(&self) -> &'static str {
        match self {
            Self::Catalyst => "EVENT BUS",
            Self::Blueprint => "SERVICE REGISTRY",
            Self::Fuse => "API GATEWAY",
            Self::Service => "WORKLOAD",
        }
    }
    /// The design-system tone this kind of node is drawn in.
    pub(super) fn tone(&self) -> draft_ui::Tone {
        match self {
            Self::Catalyst => draft_ui::Tone::Ca,
            Self::Blueprint => draft_ui::Tone::Bp,
            Self::Fuse => draft_ui::Tone::Fs,
            Self::Service => draft_ui::Tone::Sv,
        }
    }
    pub(super) fn color(&self) -> &'static str {
        match self {
            Self::Catalyst => "var(--ca)",
            Self::Blueprint => "var(--bp)",
            Self::Fuse => "var(--fs)",
            Self::Service => "var(--sv)",
        }
    }
}

// ── Data model ────────────────────────────────────────────────────────────────

#[derive(Clone, Debug, PartialEq)]
pub(super) struct RoutingRule {
    pub(super) method: String,
    pub(super) path: String,
    pub(super) target: String,
}

#[derive(Clone, Debug, PartialEq)]
pub(super) struct Topic {
    pub(super) name: String,
    pub(super) retention: String,
    pub(super) partitions: u32,
}

/// Live nodes are derived from real cluster data (Registry/Gateway/Topology)
/// on every refresh and can't be deleted/toggled from the canvas — deleting
/// the row on screen doesn't stop the real process. Manual nodes are the
/// original hand-authored kind: fully editable, never touched by a live
/// refresh. See docs/architecture/cluster-live-topology-implementation-plan.md.
#[derive(Clone, Debug, PartialEq)]
pub(super) enum NodeOrigin {
    Live,
    Manual,
}

#[derive(Clone, Debug, PartialEq)]
pub(super) struct ClNode {
    pub(super) id: String,
    pub(super) kind: NodeKind,
    pub(super) name: String,
    pub(super) x: f64,
    pub(super) y: f64,
    pub(super) online: bool,
    pub(super) rules: Vec<RoutingRule>,
    pub(super) topics: Vec<Topic>,
    pub(super) host: String,
    pub(super) port: u16,
    pub(super) protocol: String,
    pub(super) ttl: String,
    pub(super) origin: NodeOrigin,
}

/// live_node_id is a stable, content-addressed id for a live node — always
/// `live:<name>`, the same on every re-derivation, so drag positions and
/// wires attached to it survive a live-data refresh instead of being
/// recreated (and re-randomized) every time. Never collides with a manual
/// node's `next_id()`-generated `n<N>` id.
pub(super) fn live_node_id(name: &str) -> String {
    format!("live:{name}")
}

impl ClNode {
    pub(super) fn new(kind: NodeKind, name: &str, x: f64, y: f64) -> Self {
        let (rules, topics, host, port, ttl) = match &kind {
            NodeKind::Fuse => (
                vec![
                    RoutingRule {
                        method: "GET".into(),
                        path: "/api/auth/*".into(),
                        target: "auth-svc".into(),
                    },
                    RoutingRule {
                        method: "POST".into(),
                        path: "/api/billing/*".into(),
                        target: "billing-svc".into(),
                    },
                ],
                vec![],
                String::new(),
                8080u16,
                String::new(),
            ),
            NodeKind::Catalyst => (
                vec![],
                vec![
                    Topic {
                        name: "agent.events".into(),
                        retention: "72h".into(),
                        partitions: 6,
                    },
                    Topic {
                        name: "billing.tx".into(),
                        retention: "30d".into(),
                        partitions: 3,
                    },
                ],
                String::new(),
                8080,
                String::new(),
            ),
            NodeKind::Blueprint => (vec![], vec![], String::new(), 8080, "15s".into()),
            NodeKind::Service => (
                vec![],
                vec![],
                format!("{name}.cluster.local"),
                8080,
                String::new(),
            ),
        };
        Self {
            id: next_id(),
            kind,
            name: name.into(),
            x,
            y,
            online: true,
            rules,
            topics,
            host,
            port,
            protocol: "grpc".into(),
            ttl,
            origin: NodeOrigin::Manual,
        }
    }
    /// A node derived from real cluster data — see `derive_nodes`.
    pub(super) fn new_live(kind: NodeKind, name: &str, x: f64, y: f64) -> Self {
        Self {
            id: live_node_id(name),
            kind,
            name: name.into(),
            x,
            y,
            online: false,
            rules: vec![],
            topics: vec![],
            host: String::new(),
            port: 0,
            protocol: "grpc".into(),
            ttl: String::new(),
            origin: NodeOrigin::Live,
        }
    }
    pub(super) fn cx(&self) -> f64 {
        self.x + NW / 2.0
    }
    pub(super) fn wire_count(&self, ts: &[ClTrace]) -> usize {
        ts.iter()
            .filter(|t| t.kind == TraceKind::Wire && (t.from == self.id || t.to == self.id))
            .count()
    }
    pub(super) fn event_count(&self, ts: &[ClTrace]) -> usize {
        ts.iter()
            .filter(|t| t.kind == TraceKind::Event && (t.from == self.id || t.to == self.id))
            .count()
    }
}

#[derive(Clone, Debug, PartialEq)]
pub(super) enum TraceKind {
    Wire,
    Event,
}

#[derive(Clone, Debug, PartialEq)]
pub(super) struct ClTrace {
    pub(super) id: String,
    pub(super) from: String,
    pub(super) to: String,
    pub(super) kind: TraceKind,
    pub(super) topic: Option<String>,
}

impl ClTrace {
    pub(super) fn wire(a: &str, b: &str) -> Self {
        Self {
            id: next_id(),
            from: a.into(),
            to: b.into(),
            kind: TraceKind::Wire,
            topic: None,
        }
    }
    pub(super) fn event(f: &str, t: &str, tp: &str) -> Self {
        Self {
            id: next_id(),
            from: f.into(),
            to: t.into(),
            kind: TraceKind::Event,
            topic: Some(tp.into()),
        }
    }
}

// ── Interaction state ──────────────────────────────────────────────────────────

#[derive(Clone, Debug, Default, PartialEq)]
pub(super) enum Drag {
    #[default]
    None,
    Pan {
        sx: f64,
        sy: f64,
        px: f64,
        py: f64,
    },
    Node {
        id: String,
        sx: f64,
        sy: f64,
        nx: f64,
        ny: f64,
    },
    Pad {
        from_id: String,
        fx: f64,
        fy: f64,
        fd: i32,
        tx: f64,
        ty: f64,
    },
}

#[derive(Clone, Debug, PartialEq)]
pub(super) enum MenuFor {
    Canvas { wx: f64, wy: f64 },
    Node(String),
}

#[derive(Clone, Debug, PartialEq)]
pub(super) struct Menu {
    pub(super) x: f64,
    pub(super) y: f64,
    pub(super) for_: MenuFor,
}
