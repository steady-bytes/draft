use dioxus::prelude::*;
use tonic_web_wasm_client::Client as WasmClient;
use draft_api::proto::core_message_broker_actors_v1::{
    query_client::QueryClient,
    topology_client::TopologyClient,
    CloudEvent, GetTopologyResponse, GetTopologyRequest,
    OrderDirection, QueryRequest, WatchTopologyRequest,
};
use crate::components::{FullCircle, TopologyData, TopologyNode, TopologyEdge};

fn topology_from_response(resp: GetTopologyResponse) -> TopologyData {
    let producers = resp.producers.into_iter()
        .map(|n| TopologyNode {
            id: n.id.clone(),
            name: if n.name.is_empty() { n.id } else { n.name },
        })
        .collect();
    let consumers = resp.consumers.into_iter()
        .map(|n| TopologyNode {
            id: n.id.clone(),
            name: if n.name.is_empty() { n.id } else { n.name },
        })
        .collect();
    let edges = resp.edges.into_iter()
        .map(|e| TopologyEdge {
            producer_id: e.producer_source,
            consumer_id: e.consumer_source,
            event_type:  e.event_type,
            vol:         e.vol,
        })
        .collect();
    TopologyData { producers, consumers, edges }
}

// Fallback: derive producers from raw event source fields when GetTopology is unavailable.
fn topology_from_events(events: &[CloudEvent]) -> TopologyData {
    let mut producers: Vec<TopologyNode> = Vec::new();
    for ev in events {
        if !ev.source.is_empty() && !producers.iter().any(|p| p.id == ev.source) {
            producers.push(TopologyNode { id: ev.source.clone(), name: ev.source.clone() });
        }
    }
    producers.sort_by(|a, b| a.id.cmp(&b.id));
    TopologyData { producers, consumers: vec![], edges: vec![] }
}

#[component]
pub fn Topology() -> Element {
    let mut data: Signal<TopologyData> = use_signal(TopologyData::default);

    use_effect(move || {
        // Task 1: GetTopology snapshot — producers + consumers + edges with server-computed vol.
        // Falls back to QueryClient history if the Topology RPC is unavailable.
        let host = crate::CATALYST_DOMAIN.clone();
        spawn(async move {
            let mut client = TopologyClient::new(WasmClient::new(host.clone()));
            match client.get_topology(GetTopologyRequest {}).await {
                Ok(resp) => data.set(topology_from_response(resp.into_inner())),
                Err(_) => {
                    let mut qclient = QueryClient::new(WasmClient::new(host));
                    if let Ok(resp) = qclient.query(QueryRequest {
                        expression: None,
                        limit: 0,
                        after: String::new(),
                        order_by: OrderDirection::Desc as i32,
                    }).await {
                        data.set(topology_from_events(&resp.into_inner().events));
                    }
                }
            }
        });

        // Task 2: WatchTopology — re-fetch the full snapshot on any topology change.
        let host = crate::CATALYST_DOMAIN.clone();
        spawn(async move {
            let mut client = TopologyClient::new(WasmClient::new(host.clone()));
            let Ok(resp) = client.watch_topology(WatchTopologyRequest {}).await else { return; };
            let mut stream = resp.into_inner();
            loop {
                match stream.message().await {
                    Ok(Some(_)) => {
                        let mut refresh = TopologyClient::new(WasmClient::new(host.clone()));
                        if let Ok(r) = refresh.get_topology(GetTopologyRequest {}).await {
                            data.set(topology_from_response(r.into_inner()));
                        }
                    }
                    Ok(None) | Err(_) => break,
                }
            }
        });
    });

    rsx! {
        div {
            style: "width:100%;height:calc(100dvh - 4rem);overflow:hidden;background:#080808;",
            FullCircle { data: data() }
        }
    }
}
