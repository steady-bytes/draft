//! What an objective's free-form states mean, and the numbers derived from its tasks.
//!
//! An objective's states are its own ordered list (`["Queued", "In Flight", "Done"]` by default,
//! but `["Backlog", "Design", "Verifying", "Shipped"]` is equally valid), so the board cannot
//! colour columns by a fixed enum. [`classify`] reads a state's meaning from its name and, failing
//! that, its position: first is *queued*, last is *done*, anything between is *in flight*.

use draft_api::proto::tooling_lineman_v1::{Agent, Objective, Priority, Task};
use draft_ui::Tone;

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum StateKind {
    Queued,
    InFlight,
    Done,
}

impl StateKind {
    /// The column and tag colour: queued is quiet, in flight is blue, done is primary.
    pub const fn tone(self) -> Tone {
        match self {
            StateKind::Queued => Tone::Quiet,
            StateKind::InFlight => Tone::Ca,
            StateKind::Done => Tone::Primary,
        }
    }
}

const IN_FLIGHT: [&str; 6] = ["flight", "progress", "running", "active", "working", "doing"];
const DONE: [&str; 6] = ["done", "complete", "closed", "shipped", "finished", "resolved"];
const QUEUED: [&str; 5] = ["queue", "backlog", "todo", "to do", "new"];

/// The meaning of `state` within an objective whose ordered states are `states`.
pub fn classify(states: &[String], state: &str) -> StateKind {
    let name = state.to_lowercase();
    let has = |words: &[&str]| words.iter().any(|w| name.contains(w));
    if has(&IN_FLIGHT) {
        return StateKind::InFlight;
    }
    if has(&DONE) {
        return StateKind::Done;
    }
    if has(&QUEUED) {
        return StateKind::Queued;
    }
    match states.iter().position(|s| s == state) {
        Some(0) | None => StateKind::Queued,
        Some(i) if i + 1 == states.len() => StateKind::Done,
        Some(_) => StateKind::InFlight,
    }
}

/// An objective and how its tasks split across [`StateKind`]s.
#[derive(Clone, Debug, Default, PartialEq)]
pub struct Summary {
    pub id: String,
    pub name: String,
    pub description: String,
    pub states: Vec<String>,
    /// Unix seconds.
    pub created_at: Option<i64>,
    pub total: u32,
    pub queued: u32,
    pub in_flight: u32,
    pub done: u32,
    /// High-priority tasks that are not done.
    pub high_open: u32,
}

impl Summary {
    /// Percentages for a segmented progress bar: (done, in flight, queued).
    pub fn percents(&self) -> (f64, f64, f64) {
        if self.total == 0 {
            return (0.0, 0.0, 100.0);
        }
        let t = f64::from(self.total);
        (f64::from(self.done) / t * 100.0, f64::from(self.in_flight) / t * 100.0, f64::from(self.queued) / t * 100.0)
    }
}

pub fn summarize(objective: &Objective, tasks: &[Task]) -> Summary {
    let mut s = Summary {
        id: objective.id.clone(),
        name: objective.name.clone(),
        description: objective.description.clone(),
        states: objective.states.clone(),
        created_at: objective.created_at.as_ref().map(|t| t.seconds),
        ..Summary::default()
    };
    for t in tasks.iter().filter(|t| t.objective_id == objective.id) {
        s.total += 1;
        let kind = classify(&objective.states, &t.state);
        match kind {
            StateKind::Queued => s.queued += 1,
            StateKind::InFlight => s.in_flight += 1,
            StateKind::Done => s.done += 1,
        }
        if t.priority == Priority::High as i32 && kind != StateKind::Done {
            s.high_open += 1;
        }
    }
    s
}

/// An agent is online if it has been seen recently. Lineman has no explicit presence: agents
/// heartbeat, and silence means gone.
pub const AGENT_ONLINE_SECS: i64 = 90;

pub fn agents_online(agents: &[Agent], now_secs: i64) -> u32 {
    agents
        .iter()
        .filter(|a| a.last_seen_at.as_ref().is_some_and(|t| now_secs - t.seconds <= AGENT_ONLINE_SECS))
        .count() as u32
}

/// The short id shown on a card (`#3fa9c1`); tasks have no sequence number, only a UUID.
pub fn short_id(id: &str) -> String {
    format!("#{}", id.chars().filter(|c| c.is_ascii_alphanumeric()).take(6).collect::<String>())
}

/// Priority as a tag colour and label.
pub fn priority_tag(priority: i32) -> (Tone, &'static str) {
    match Priority::try_from(priority).unwrap_or(Priority::Unspecified) {
        Priority::High => (Tone::Err, "High"),
        Priority::Medium => (Tone::Warn, "Medium"),
        Priority::Low => (Tone::Quiet, "Low"),
        Priority::Unspecified => (Tone::Quiet, "—"),
    }
}

/// Sort key that keeps a column in the server's order (`order`, then creation as a tiebreak).
pub fn column_order(a: &Task, b: &Task) -> std::cmp::Ordering {
    a.order.cmp(&b.order).then_with(|| {
        let at = a.created_at.as_ref().map(|t| t.seconds).unwrap_or(0);
        let bt = b.created_at.as_ref().map(|t| t.seconds).unwrap_or(0);
        at.cmp(&bt)
    })
}

/// The server's gap between neighbouring orders (`orderGap` in Lineman's `task.go`).
const ORDER_GAP: i64 = 1000;

/// The `order` a card dropped between `before` and `after` gets — the same formula as the server's
/// `computeReorder`, so the optimistic position matches the one the `Watch` update confirms.
/// Either neighbour may be absent (start or end of the column). The server stays the authority
/// (it also resequences a column that has run out of room); this only places the card until then.
pub fn midpoint(before: Option<i64>, after: Option<i64>) -> i64 {
    match (before, after) {
        (Some(b), Some(a)) => b + (a - b) / 2,
        (Some(b), None) => b + ORDER_GAP,
        (None, Some(a)) => a / 2,
        (None, None) => ORDER_GAP,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn states(s: &[&str]) -> Vec<String> {
        s.iter().map(|x| x.to_string()).collect()
    }

    #[test]
    fn default_states_classify_by_name() {
        let s = states(&["Queued", "In Flight", "Done"]);
        assert_eq!(classify(&s, "Queued"), StateKind::Queued);
        assert_eq!(classify(&s, "In Flight"), StateKind::InFlight);
        assert_eq!(classify(&s, "Done"), StateKind::Done);
    }

    #[test]
    fn free_form_states_fall_back_to_position() {
        let s = states(&["Backlog", "Design", "Verifying", "Shipped"]);
        assert_eq!(classify(&s, "Backlog"), StateKind::Queued);
        assert_eq!(classify(&s, "Design"), StateKind::InFlight);
        assert_eq!(classify(&s, "Verifying"), StateKind::InFlight);
        assert_eq!(classify(&s, "Shipped"), StateKind::Done);
    }

    #[test]
    fn position_only_when_the_name_says_nothing() {
        let s = states(&["Alpha", "Beta", "Gamma"]);
        assert_eq!(classify(&s, "Alpha"), StateKind::Queued);
        assert_eq!(classify(&s, "Beta"), StateKind::InFlight);
        assert_eq!(classify(&s, "Gamma"), StateKind::Done);
    }

    #[test]
    fn a_name_beats_position() {
        // "In progress" as the last state is still in flight.
        let s = states(&["Open", "Review", "In progress"]);
        assert_eq!(classify(&s, "In progress"), StateKind::InFlight);
    }

    #[test]
    fn a_single_state_and_unknown_states_are_queued() {
        assert_eq!(classify(&states(&["Only"]), "Only"), StateKind::Queued);
        assert_eq!(classify(&states(&["A", "B"]), "Nope"), StateKind::Queued);
    }

    #[test]
    fn tones_follow_the_kind() {
        assert_eq!(StateKind::Queued.tone(), Tone::Quiet);
        assert_eq!(StateKind::InFlight.tone(), Tone::Ca);
        assert_eq!(StateKind::Done.tone(), Tone::Primary);
    }

    fn task(objective: &str, state: &str, priority: Priority) -> Task {
        Task { objective_id: objective.into(), state: state.into(), priority: priority as i32, ..Task::default() }
    }

    #[test]
    fn summary_splits_tasks_and_counts_open_high_priority() {
        let obj = Objective { id: "o1".into(), name: "Roll out".into(), states: states(&["Queued", "In Flight", "Done"]), ..Objective::default() };
        let tasks = vec![
            task("o1", "Queued", Priority::High),
            task("o1", "Queued", Priority::Low),
            task("o1", "In Flight", Priority::High),
            task("o1", "Done", Priority::High), // done: not "open"
            task("other", "Queued", Priority::High),
        ];
        let s = summarize(&obj, &tasks);
        assert_eq!((s.total, s.queued, s.in_flight, s.done, s.high_open), (4, 2, 1, 1, 2));
        let (done, flight, queued) = s.percents();
        assert_eq!((done, flight, queued), (25.0, 25.0, 50.0));
    }

    #[test]
    fn an_empty_objective_reads_as_all_queued() {
        let s = summarize(&Objective { id: "o".into(), ..Objective::default() }, &[]);
        assert_eq!(s.percents(), (0.0, 0.0, 100.0));
    }

    #[test]
    fn agents_are_online_when_recently_seen() {
        let seen = |secs| Agent { last_seen_at: Some(prost_types::Timestamp { seconds: secs, nanos: 0 }), ..Agent::default() };
        let agents = [seen(1000), seen(950), seen(100), Agent::default()];
        assert_eq!(agents_online(&agents, 1010), 2);
    }

    #[test]
    fn short_ids_are_six_alphanumerics() {
        assert_eq!(short_id("3fa9c1d2-aaaa-bbbb"), "#3fa9c1");
        assert_eq!(short_id("ab"), "#ab");
    }

    #[test]
    fn priority_tags() {
        assert_eq!(priority_tag(Priority::High as i32), (Tone::Err, "High"));
        assert_eq!(priority_tag(Priority::Medium as i32), (Tone::Warn, "Medium"));
        assert_eq!(priority_tag(Priority::Low as i32), (Tone::Quiet, "Low"));
        assert_eq!(priority_tag(99), (Tone::Quiet, "—"));
    }

    #[test]
    fn columns_keep_server_order() {
        let mk = |order, created| Task { order, created_at: Some(prost_types::Timestamp { seconds: created, nanos: 0 }), ..Task::default() };
        let mut v = vec![mk(20, 1), mk(10, 5), mk(10, 2)];
        v.sort_by(column_order);
        assert_eq!(v.iter().map(|t| (t.order, t.created_at.as_ref().unwrap().seconds)).collect::<Vec<_>>(), vec![(10, 2), (10, 5), (20, 1)]);
    }

    #[test]
    fn midpoint_between_neighbours_or_at_the_ends() {
        assert_eq!(midpoint(Some(10), Some(20)), 15);
        assert_eq!(midpoint(Some(2000), None), 3000);
        assert_eq!(midpoint(None, Some(2000)), 1000);
        assert_eq!(midpoint(None, None), 1000);
    }
}
