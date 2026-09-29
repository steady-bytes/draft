//! The event topology as a three-column board — producers, event types, consumers — kept pure so
//! the layout and the flow tracing are tested.
//!
//! Catalyst reports one edge per (producer, event type, consumer) flow. A link on the board is the
//! sum of the flows through it, so hovering a node can show exactly the flows that pass through it
//! and nothing else.

use std::collections::{BTreeMap, BTreeSet};

/// The topology as the cluster canvas draws its event bus: producers, consumers and the edges
/// between them (Catalyst's `GetTopology` shape, unchanged).
#[derive(Clone, PartialEq)]
pub struct TopologyNode {
    pub id: String,
    pub name: String,
}

#[derive(Clone, PartialEq)]
pub struct TopologyEdge {
    pub producer_id: String,
    pub consumer_id: String,
    pub event_type: String,
    pub vol: u32,
}

#[derive(Clone, PartialEq, Default)]
pub struct TopologyData {
    pub producers: Vec<TopologyNode>,
    pub consumers: Vec<TopologyNode>,
    pub edges: Vec<TopologyEdge>,
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

/// One flow: `producer` publishes `event_type`, `consumer` receives it. `vol` is the volume
/// Catalyst computed for it (0 when it has not been measured).
#[derive(Clone, Debug, PartialEq)]
pub struct Flow {
    pub producer: String,
    pub consumer: String,
    pub event_type: String,
    pub vol: u32,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord)]
pub enum Col {
    Producer,
    Type,
    Consumer,
}

/// A node picked out on the board.
#[derive(Clone, Debug, PartialEq, Eq, Hash)]
pub struct Focus {
    pub col: Col,
    pub id: String,
}

impl Focus {
    pub fn new(col: Col, id: impl Into<String>) -> Self {
        Self { col, id: id.into() }
    }

    fn holds(&self, f: &Flow) -> bool {
        match self.col {
            Col::Producer => f.producer == self.id,
            Col::Type => f.event_type == self.id,
            Col::Consumer => f.consumer == self.id,
        }
    }
}

/// `/services/beacon` → `beacon`.
pub fn display_name(id: &str) -> &str {
    id.rsplit('/').next().filter(|s| !s.is_empty()).unwrap_or(id)
}

/// `core.observability.wide_events.v1.WideEvent` → `WideEvent`.
pub fn type_label(event_type: &str) -> &str {
    event_type.rsplit('.').next().filter(|s| !s.is_empty()).unwrap_or(event_type)
}

/// Wide events are Beacon's own telemetry riding the broker: usually the loudest type by far.
pub fn is_wide_event(event_type: &str) -> bool {
    event_type.to_ascii_lowercase().contains("wide_event") || event_type.ends_with(".WideEvent")
}

/// The flows a focus passes through, or all of them with none.
pub fn active_flows<'a>(flows: &'a [Flow], focus: Option<&Focus>) -> Vec<&'a Flow> {
    flows.iter().filter(|f| focus.map_or(true, |x| x.holds(f))).collect()
}

#[derive(Clone, Debug, PartialEq)]
pub struct Link {
    pub from: (Col, usize),
    pub to: (Col, usize),
    pub type_idx: usize,
    pub vol: u32,
}

#[derive(Clone, Debug, Default, PartialEq)]
pub struct Board {
    pub producers: Vec<String>,
    pub types: Vec<String>,
    pub consumers: Vec<String>,
    pub links: Vec<Link>,
}

/// Every node in a column, biggest first (by total volume, then name), so the busiest are on top.
fn ordered(flows: &[Flow], key: impl Fn(&Flow) -> &String) -> Vec<String> {
    let mut vol: BTreeMap<&String, u64> = BTreeMap::new();
    for f in flows {
        *vol.entry(key(f)).or_default() += u64::from(f.vol);
    }
    let mut list: Vec<(&String, u64)> = vol.into_iter().collect();
    list.sort_by(|a, b| b.1.cmp(&a.1).then_with(|| a.0.cmp(b.0)));
    list.into_iter().map(|(k, _)| k.clone()).collect()
}

/// The board for a set of flows. Node order and indices come from *all* the flows, so nodes keep
/// their place when a focus dims some of them; `active` selects which links are drawn as lit.
pub fn board(flows: &[Flow]) -> Board {
    Board {
        producers: ordered(flows, |f| &f.producer),
        types: ordered(flows, |f| &f.event_type),
        consumers: ordered(flows, |f| &f.consumer),
        links: Vec::new(),
    }
}

/// The links a set of flows makes on `board`: producer → type and type → consumer, each summed
/// over the flows that pass through it.
pub fn links(board: &Board, flows: &[&Flow]) -> Vec<Link> {
    let index = |list: &[String], id: &str| list.iter().position(|x| x == id);
    let mut sums: BTreeMap<((Col, usize), (Col, usize), usize), u32> = BTreeMap::new();
    for f in flows {
        let (Some(p), Some(t), Some(c)) = (index(&board.producers, &f.producer), index(&board.types, &f.event_type), index(&board.consumers, &f.consumer)) else {
            continue;
        };
        *sums.entry(((Col::Producer, p), (Col::Type, t), t)).or_default() += f.vol;
        *sums.entry(((Col::Type, t), (Col::Consumer, c), t)).or_default() += f.vol;
    }
    sums.into_iter().map(|((from, to, type_idx), vol)| Link { from, to, type_idx, vol }).collect()
}

/// The nodes a focus lights up, by column: the focus itself and everything it flows to and from.
pub fn lit_nodes(flows: &[Flow], focus: Option<&Focus>) -> BTreeSet<(Col, String)> {
    let mut lit = BTreeSet::new();
    for f in active_flows(flows, focus) {
        lit.insert((Col::Producer, f.producer.clone()));
        lit.insert((Col::Type, f.event_type.clone()));
        lit.insert((Col::Consumer, f.consumer.clone()));
    }
    lit
}

/// A link's stroke width in px: 1 for the quietest, 5 for the busiest; unmeasured links get a
/// visible default.
pub fn stroke_width(vol: u32, max_vol: u32) -> f64 {
    if max_vol == 0 || vol == 0 {
        return 1.5;
    }
    1.0 + 4.0 * (f64::from(vol) / f64::from(max_vol))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn flow(p: &str, t: &str, c: &str, vol: u32) -> Flow {
        Flow { producer: format!("/services/{p}"), event_type: t.into(), consumer: format!("/services/{c}"), vol }
    }

    fn sample() -> Vec<Flow> {
        vec![
            flow("lineman", "task.created", "bench", 8),
            flow("lineman", "task.created", "beacon", 8),
            flow("fuse", "route.changed", "beacon", 3),
            flow("bench", "run.passed", "beacon", 20),
        ]
    }

    #[test]
    fn names_are_shortened_for_display() {
        assert_eq!(display_name("/services/beacon"), "beacon");
        assert_eq!(display_name("plain"), "plain");
        assert_eq!(type_label("core.observability.wide_events.v1.WideEvent"), "WideEvent");
        assert_eq!(type_label("route.changed"), "changed");
    }

    #[test]
    fn wide_events_are_recognised() {
        assert!(is_wide_event("core.observability.wide_events.v1.WideEvent"));
        assert!(!is_wide_event("lineman.task.created"));
    }

    #[test]
    fn columns_list_the_busiest_first() {
        let b = board(&sample());
        assert_eq!(b.producers, ["/services/bench", "/services/lineman", "/services/fuse"]);
        assert_eq!(b.types, ["run.passed", "task.created", "route.changed"]);
        assert_eq!(b.consumers, ["/services/beacon", "/services/bench"]);
    }

    #[test]
    fn a_link_sums_the_flows_through_it() {
        let flows = sample();
        let b = board(&flows);
        let all: Vec<&Flow> = flows.iter().collect();
        let l = links(&b, &all);
        // lineman → task.created carries both of its flows.
        let lineman = b.producers.iter().position(|p| p.ends_with("lineman")).unwrap();
        let created = b.types.iter().position(|t| t == "task.created").unwrap();
        let into_type = l.iter().find(|x| x.from == (Col::Producer, lineman) && x.to == (Col::Type, created)).unwrap();
        assert_eq!(into_type.vol, 16);
        // task.created → beacon carries only the flow that ends at beacon.
        let beacon = b.consumers.iter().position(|c| c.ends_with("beacon")).unwrap();
        let to_beacon = l.iter().find(|x| x.from == (Col::Type, created) && x.to == (Col::Consumer, beacon)).unwrap();
        assert_eq!(to_beacon.vol, 8);
    }

    #[test]
    fn a_focus_keeps_only_the_flows_through_it() {
        let flows = sample();
        let focus = Focus::new(Col::Producer, "/services/lineman");
        let active = active_flows(&flows, Some(&focus));
        assert_eq!(active.len(), 2);
        assert!(active.iter().all(|f| f.producer == "/services/lineman"));
        assert_eq!(active_flows(&flows, None).len(), 4);
    }

    #[test]
    fn a_consumer_lights_every_producer_and_type_that_reaches_it() {
        let flows = sample();
        let lit = lit_nodes(&flows, Some(&Focus::new(Col::Consumer, "/services/bench")));
        let expect: BTreeSet<(Col, String)> = [
            (Col::Producer, "/services/lineman".to_string()),
            (Col::Type, "task.created".to_string()),
            (Col::Consumer, "/services/bench".to_string()),
        ]
        .into_iter()
        .collect();
        assert_eq!(lit, expect);
    }

    #[test]
    fn a_focused_board_keeps_the_same_node_order() {
        let flows = sample();
        let full = board(&flows);
        // Links are computed against the full board even when only some flows are active.
        let focus = Focus::new(Col::Type, "route.changed");
        let active = active_flows(&flows, Some(&focus));
        let l = links(&full, &active);
        assert_eq!(l.len(), 2);
        assert!(l.iter().all(|x| x.type_idx == full.types.iter().position(|t| t == "route.changed").unwrap()));
    }

    #[test]
    fn stroke_widths_scale_with_volume() {
        assert_eq!(stroke_width(0, 100), 1.5);
        assert_eq!(stroke_width(5, 0), 1.5);
        assert_eq!(stroke_width(100, 100), 5.0);
        assert!(stroke_width(10, 100) < stroke_width(50, 100));
    }

    #[test]
    fn no_flows_make_an_empty_board() {
        let b = board(&[]);
        assert!(b.producers.is_empty() && b.types.is_empty() && b.consumers.is_empty());
        assert!(links(&b, &[]).is_empty());
    }
}
