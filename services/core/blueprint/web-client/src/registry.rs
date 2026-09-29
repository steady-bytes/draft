//! How the service registry reads a process: which domain it belongs to, whether it is healthy,
//! stale or gone. Pure, so it is tested without a browser.

use std::collections::BTreeMap;

use draft_api::proto::core_registry_service_discovery_v1::{Process, ProcessHealthState, ProcessRunningState};

/// Instances report a heartbeat every 5 s; one silent for longer than this is stale.
pub const STALE_AFTER_SECS: i64 = 15;

#[derive(Clone, Copy, Debug, PartialEq, Eq, PartialOrd, Ord, Hash)]
pub enum Domain {
    Core,
    Tooling,
    Plugin,
    Example,
    Other,
}

impl Domain {
    pub const fn label(self) -> &'static str {
        match self {
            Domain::Core => "Core",
            Domain::Tooling => "Tooling",
            Domain::Plugin => "Plugin",
            Domain::Example => "Example",
            Domain::Other => "Other",
        }
    }
}

/// A process's domain, read from the RPC services it advertises: the registry has no field for it
/// (`group` is empty in practice). A `tooling.step_executor.*` service marks a Foundry plugin.
pub fn domain_of(p: &Process) -> Domain {
    let services: Vec<&str> = p.metadata.iter().map(|m| m.key.as_str()).collect();
    if services.iter().any(|s| s.starts_with("tooling.step_executor.")) {
        return Domain::Plugin;
    }
    for (prefix, domain) in [("core.", Domain::Core), ("tooling.", Domain::Tooling), ("examples.", Domain::Example)] {
        if services.iter().any(|s| s.starts_with(prefix)) {
            return domain;
        }
    }
    Domain::Other
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum InstanceState {
    Healthy,
    Starting,
    /// Running but silent for longer than [`STALE_AFTER_SECS`].
    Stale,
    Unhealthy,
    Disconnected,
}

impl InstanceState {
    pub const fn is_problem(self) -> bool {
        matches!(self, InstanceState::Stale | InstanceState::Unhealthy | InstanceState::Disconnected)
    }

    pub const fn label(self) -> &'static str {
        match self {
            InstanceState::Healthy => "Healthy",
            InstanceState::Starting => "Starting",
            InstanceState::Stale => "Stale",
            InstanceState::Unhealthy => "Unhealthy",
            InstanceState::Disconnected => "Disconnected",
        }
    }
}

/// Seconds since the process last reported, or `None` if it never has.
pub fn heartbeat_age(p: &Process, now_secs: i64) -> Option<i64> {
    p.last_status_time.as_ref().map(|t| (now_secs - t.seconds).max(0))
}

pub fn instance_state(p: &Process, now_secs: i64) -> InstanceState {
    if p.running_state == ProcessRunningState::ProcessDiconnected as i32 {
        return InstanceState::Disconnected;
    }
    if p.health_state == ProcessHealthState::ProcessUnhealthy as i32 {
        return InstanceState::Unhealthy;
    }
    if heartbeat_age(p, now_secs).is_some_and(|age| age > STALE_AFTER_SECS) {
        return InstanceState::Stale;
    }
    if p.running_state == ProcessRunningState::ProcessStarting as i32 || p.running_state == ProcessRunningState::ProcessTesting as i32 {
        return InstanceState::Starting;
    }
    InstanceState::Healthy
}

/// One logical service: every registered process sharing a name (a restart or a replica each add
/// a row to the registry, so grouping by name is what reads as one service).
#[derive(Clone, Debug, PartialEq)]
pub struct Group {
    pub name: String,
    pub domain: Domain,
    /// Newest heartbeat first.
    pub instances: Vec<Process>,
}

impl Group {
    pub fn healthy(&self, now: i64) -> usize {
        self.instances.iter().filter(|p| instance_state(p, now) == InstanceState::Healthy).count()
    }

    pub fn has_problem(&self, now: i64) -> bool {
        self.instances.iter().any(|p| instance_state(p, now).is_problem())
    }

    /// The freshest heartbeat in the group, in seconds ago.
    pub fn last_heartbeat(&self, now: i64) -> Option<i64> {
        self.instances.iter().filter_map(|p| heartbeat_age(p, now)).min()
    }
}

/// Groups processes by name, in name order.
pub fn group(processes: impl IntoIterator<Item = Process>) -> Vec<Group> {
    let mut by_name: BTreeMap<String, Vec<Process>> = BTreeMap::new();
    for p in processes {
        by_name.entry(p.name.clone()).or_default().push(p);
    }
    by_name
        .into_iter()
        .map(|(name, mut instances)| {
            instances.sort_by(|a, b| {
                let key = |p: &Process| p.last_status_time.as_ref().map(|t| (t.seconds, t.nanos));
                key(b).cmp(&key(a))
            });
            let domain = instances.first().map(domain_of).unwrap_or(Domain::Other);
            Group { name, domain, instances }
        })
        .collect()
}

/// The address column: the first instance's, with `+N` for the rest.
pub fn address_summary(g: &Group) -> String {
    match g.instances.split_first() {
        Some((first, rest)) if !rest.is_empty() => format!("{} +{}", first.ip_address, rest.len()),
        Some((first, _)) => first.ip_address.clone(),
        None => String::new(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use draft_api::proto::core_registry_service_discovery_v1::Metadata;
    use prost_types::Timestamp;

    fn process(name: &str, services: &[&str], beat_secs_ago: Option<i64>, now: i64) -> Process {
        Process {
            pid: format!("{name}-pid"),
            name: name.into(),
            ip_address: "localhost:1".into(),
            metadata: services.iter().map(|s| Metadata { pid: String::new(), key: (*s).into(), value: (*s).into() }).collect(),
            running_state: ProcessRunningState::ProcessRunning as i32,
            health_state: ProcessHealthState::ProcessHealthy as i32,
            last_status_time: beat_secs_ago.map(|a| Timestamp { seconds: now - a, nanos: 0 }),
            ..Default::default()
        }
    }

    const NOW: i64 = 1_800_000_000;

    #[test]
    fn a_domain_is_read_from_the_services_a_process_advertises() {
        assert_eq!(domain_of(&process("blueprint", &["core.consensus.raft.v1.RaftService"], Some(1), NOW)), Domain::Core);
        assert_eq!(domain_of(&process("bench", &["tooling.workflow.v1.WorkflowService", "webhooks"], Some(1), NOW)), Domain::Tooling);
        assert_eq!(domain_of(&process("http-call", &["tooling.step_executor.v1.StepExecutor"], Some(1), NOW)), Domain::Plugin);
        assert_eq!(domain_of(&process("crud", &["examples.crud.v1.CrudService"], Some(1), NOW)), Domain::Example);
        assert_eq!(domain_of(&process("mystery", &[], Some(1), NOW)), Domain::Other);
    }

    #[test]
    fn a_silent_process_is_stale_only_past_fifteen_seconds() {
        assert_eq!(instance_state(&process("a", &[], Some(15), NOW), NOW), InstanceState::Healthy);
        assert_eq!(instance_state(&process("a", &[], Some(16), NOW), NOW), InstanceState::Stale);
        assert!(instance_state(&process("a", &[], Some(60), NOW), NOW).is_problem());
    }

    #[test]
    fn a_process_that_never_reported_is_not_stale() {
        assert_eq!(instance_state(&process("a", &[], None, NOW), NOW), InstanceState::Healthy);
    }

    #[test]
    fn disconnected_and_unhealthy_win_over_freshness() {
        let mut p = process("a", &[], Some(1), NOW);
        p.health_state = ProcessHealthState::ProcessUnhealthy as i32;
        assert_eq!(instance_state(&p, NOW), InstanceState::Unhealthy);
        p.running_state = ProcessRunningState::ProcessDiconnected as i32;
        assert_eq!(instance_state(&p, NOW), InstanceState::Disconnected);
    }

    #[test]
    fn starting_is_not_a_problem() {
        let mut p = process("a", &[], Some(1), NOW);
        p.running_state = ProcessRunningState::ProcessStarting as i32;
        assert_eq!(instance_state(&p, NOW), InstanceState::Starting);
        assert!(!InstanceState::Starting.is_problem());
    }

    #[test]
    fn restarts_of_one_service_group_into_one_row_newest_first() {
        let groups = group([
            process("fuse", &["core.control_plane.networking.v1.NetworkingService"], Some(30), NOW),
            process("bench", &["tooling.workflow.v1.WorkflowService"], Some(2), NOW),
            process("fuse", &["core.control_plane.networking.v1.NetworkingService"], Some(1), NOW),
        ]);
        let names: Vec<&str> = groups.iter().map(|g| g.name.as_str()).collect();
        assert_eq!(names, ["bench", "fuse"]);
        let fuse = &groups[1];
        assert_eq!(fuse.instances.len(), 2);
        assert_eq!(fuse.last_heartbeat(NOW), Some(1));
        // The newest heartbeat is first; the stale twin is the problem.
        assert_eq!(heartbeat_age(&fuse.instances[0], NOW), Some(1));
        assert!(fuse.has_problem(NOW));
        assert_eq!(fuse.healthy(NOW), 1);
    }

    #[test]
    fn the_address_shows_the_first_instance_and_how_many_more() {
        let g = group([process("blueprint", &[], Some(1), NOW), process("blueprint", &[], Some(2), NOW)]);
        assert_eq!(address_summary(&g[0]), "localhost:1 +1");
    }
}
