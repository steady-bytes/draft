/// Topology types mirror Catalyst API data shapes:
///   producers — distinct CloudEvent.source values from the Produce stream
///   consumers — services registered via the Consume stream RPC
///   edges     — (source × event_type × consumer) flows derived from event history

#[derive(Clone, PartialEq)]
pub struct TopologyNode {
    pub id:   String,
    pub name: String,
}

#[derive(Clone, PartialEq)]
pub struct TopologyEdge {
    pub producer_id: String,
    pub consumer_id: String,
    pub event_type:  String,
    pub vol:         u32,
}

#[derive(Clone, PartialEq, Default)]
pub struct TopologyData {
    pub producers: Vec<TopologyNode>,
    pub consumers: Vec<TopologyNode>,
    pub edges:     Vec<TopologyEdge>,
}

impl TopologyData {
    pub fn unique_event_types(&self) -> Vec<&str> {
        let mut out: Vec<&str> = Vec::new();
        for e in &self.edges {
            if !out.contains(&e.event_type.as_str()) {
                out.push(e.event_type.as_str());
            }
        }
        out.sort_unstable();
        out
    }
}

pub fn event_color(event_type: &str) -> &'static str {
    match event_type {
        "created"    => "#3b82f6",
        "cancelled"  => "#ef4444",
        "ok"         => "#22c55e",
        "failed"     => "#f97316",
        "reserved"   => "#a855f7",
        "updated"    => "#14b8a6",
        "registered" => "#eab308",
        _            => "#6b7280",
    }
}

