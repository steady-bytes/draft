//! The Raft cluster's shape, for the topbar status ("Raft · 5 nodes · leader node_1") every view
//! shows. Loaded once by the layout and shared through context.

use dioxus::prelude::*;
use draft_api::proto::core_registry_service_discovery_v1::{
    service_discovery_service_client::ServiceDiscoveryServiceClient, GetClusterDetailsRequest, LeadershipStatus, Node,
};
use draft_ui::shell::{Chrome, ChromeStatus};
use draft_ui::StatusKind;
use tonic_web_wasm_client::Client as WasmClient;

#[derive(Clone, Debug, Default, PartialEq)]
pub struct RaftInfo {
    pub nodes: Vec<Node>,
}

impl RaftInfo {
    pub fn leader(&self) -> Option<&Node> {
        self.nodes.iter().find(|n| n.leadership_status == LeadershipStatus::Leader as i32)
    }

    /// `Raft · 5 nodes · leader node_1`, or `Raft · 1 node · no leader` while an election runs.
    pub fn summary(&self) -> String {
        let n = self.nodes.len();
        let nodes = if n == 1 { "1 node".to_string() } else { format!("{n} nodes") };
        match self.leader() {
            Some(l) => format!("Raft · {nodes} · leader {}", l.id),
            None => format!("Raft · {nodes} · no leader"),
        }
    }
}

/// Handle to the shared cluster info. `Copy`, so views take it from context.
#[derive(Clone, Copy)]
pub struct Raft {
    resource: Resource<Option<RaftInfo>>,
}

impl Raft {
    pub fn info(&self) -> Option<RaftInfo> {
        self.resource.read().clone().flatten()
    }

    /// A page's breadcrumb and the Raft pill. Reads the info, so a caller inside `use_page_chrome`
    /// re-runs when the cluster changes.
    pub fn chrome(&self, crumbs: &[&str]) -> Chrome {
        let status = match self.info() {
            Some(info) if info.leader().is_some() => ChromeStatus::live(info.summary()),
            Some(info) => ChromeStatus::new(StatusKind::Warn, info.summary()),
            None => ChromeStatus::new(StatusKind::Idle, "Raft · unknown"),
        };
        Chrome { crumbs: crumbs.iter().map(|c| c.to_string()).collect(), status: Some(status), ..Chrome::default() }
    }
}

pub fn use_raft() -> Raft {
    use_context::<Raft>()
}

/// Loads the cluster details and provides them. Call once, in the layout.
pub fn use_raft_provider() -> Raft {
    let resource = use_resource(|| async {
        let mut client = ServiceDiscoveryServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
        client.get_cluster_details(GetClusterDetailsRequest {}).await.ok().map(|r| RaftInfo { nodes: r.into_inner().nodes })
    });
    use_context_provider(|| Raft { resource })
}

#[cfg(test)]
mod tests {
    use super::*;

    fn node(id: &str, leader: bool) -> Node {
        Node {
            id: id.into(),
            address: "localhost:1111".into(),
            leadership_status: if leader { LeadershipStatus::Leader as i32 } else { LeadershipStatus::Follower as i32 },
        }
    }

    #[test]
    fn the_summary_names_the_leader() {
        let info = RaftInfo { nodes: vec![node("node_2", false), node("node_1", true)] };
        assert_eq!(info.summary(), "Raft · 2 nodes · leader node_1");
    }

    #[test]
    fn a_single_node_and_an_election_read_naturally() {
        assert_eq!(RaftInfo { nodes: vec![node("node_1", true)] }.summary(), "Raft · 1 node · leader node_1");
        assert_eq!(RaftInfo { nodes: vec![node("a", false), node("b", false)] }.summary(), "Raft · 2 nodes · no leader");
    }
}
