//! The data the shell shows on every page — objectives with task counts, pending scheduled tasks,
//! active loops, online agents — loaded once in the layout and shared through context.
//!
//! One `ListTasks` call with no filter returns every task, so the rail's counts and the dashboard
//! cost a fixed handful of requests rather than one per objective. The `Watch` stream nudges a
//! refresh (coalesced) whenever anything changes.

use dioxus::prelude::*;
use draft_api::proto::tooling_lineman_v1::{
    lineman_service_client::LinemanServiceClient, ListAgentsRequest, ListLoopsRequest,
    ListObjectivesRequest, ListScheduledTasksRequest, ListTasksRequest, LoopStatus,
    ScheduledTaskStatus, WatchRequest,
};
use draft_ui::shell::{Chrome, ChromeStatus};
use gloo_timers::future::TimeoutFuture;
use tonic_web_wasm_client::Client as WasmClient;

use crate::state::{agents_online, summarize, Summary};

/// How long after a `Watch` event the shell waits before refetching, so a burst of events costs
/// one refresh.
const REFRESH_COALESCE_MS: u32 = 1200;

#[derive(Clone, Debug, Default, PartialEq)]
pub struct RailSnapshot {
    pub objectives: Vec<Summary>,
    pub scheduled_pending: u32,
    pub loops_active: u32,
    pub agents: Vec<draft_api::proto::tooling_lineman_v1::Agent>,
    pub agents_online: u32,
}

/// Handle to the shared data. `Copy`, so views take it by value from context.
#[derive(Clone, Copy)]
pub struct Rail {
    snapshot: Resource<Option<RailSnapshot>>,
    version: Signal<u32>,
}

impl Rail {
    /// The latest snapshot, or `None` while loading or if the fetch failed.
    pub fn get(&self) -> Option<RailSnapshot> {
        self.snapshot.read().clone().flatten()
    }

    /// Refetch now (after creating an objective or a task).
    pub fn refresh(&self) {
        let mut v = self.version;
        v += 1;
    }
}

impl Rail {
    /// A page's breadcrumb and the "N agents online" pill. Reads the snapshot, so a caller inside
    /// `use_page_chrome` re-runs when the counts change.
    pub fn chrome(&self, crumbs: &[&str]) -> Chrome {
        let online = self.get().map(|s| s.agents_online);
        Chrome {
            crumbs: crumbs.iter().map(|c| c.to_string()).collect(),
            status: online.map(|n| ChromeStatus::live(format!("{n} agent{} online", if n == 1 { "" } else { "s" }))),
            ..Chrome::default()
        }
    }
}

pub fn use_rail() -> Rail {
    use_context::<Rail>()
}

/// Loads the shared data and keeps it fresh. Call once, in the layout.
pub fn use_rail_provider() -> Rail {
    let version = use_signal(|| 0u32);

    let snapshot = use_resource(move || async move {
        let _ = version();
        load().await
    });

    // Refresh whenever the backend reports a change.
    let mut pending = use_signal(|| false);
    use_coroutine(move |_rx: UnboundedReceiver<()>| async move {
        let mut client = LinemanServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
        let Ok(response) = client.watch(WatchRequest {}).await else {
            return;
        };
        let mut stream = response.into_inner();
        while let Ok(Some(_)) = stream.message().await {
            if !*pending.peek() {
                pending.set(true);
                let mut v = version;
                spawn(async move {
                    TimeoutFuture::new(REFRESH_COALESCE_MS).await;
                    v += 1;
                    pending.set(false);
                });
            }
        }
    });

    use_context_provider(|| Rail { snapshot, version })
}

async fn load() -> Option<RailSnapshot> {
    let mut client = LinemanServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
    let objectives = client.list_objectives(ListObjectivesRequest::default()).await.ok()?.into_inner().objectives;
    let tasks = client.list_tasks(ListTasksRequest::default()).await.ok()?.into_inner().tasks;
    // The rest are decoration: a failure shows zeros rather than hiding the objectives.
    let scheduled = client
        .list_scheduled_tasks(ListScheduledTasksRequest::default())
        .await
        .map(|r| r.into_inner().scheduled_tasks)
        .unwrap_or_default();
    let loops = client.list_loops(ListLoopsRequest::default()).await.map(|r| r.into_inner().loops).unwrap_or_default();
    let agents = client.list_agents(ListAgentsRequest::default()).await.map(|r| r.into_inner().agents).unwrap_or_default();

    let now = chrono::Utc::now().timestamp();
    Some(RailSnapshot {
        objectives: objectives.iter().map(|o| summarize(o, &tasks)).collect(),
        scheduled_pending: scheduled.iter().filter(|s| s.status == ScheduledTaskStatus::Pending as i32).count() as u32,
        loops_active: loops.iter().filter(|l| l.status == LoopStatus::Active as i32).count() as u32,
        agents_online: agents_online(&agents, now),
        agents,
    })
}
