//! One objective: its progress, and its tasks as a board or a list.
//!
//! This is the merge of the old Objective Detail and Task Board pages — `/objectives/:id` and
//! `/objectives/:id/board` both land here (the board is the default; List is a segmented toggle).
//! The board keeps its live `Watch` upserts and adds:
//!
//! * a "Move to…" menu on every card — the keyboard- and touch-accessible way to change a task's
//!   state (WCAG 2.2: dragging needs a non-drag alternative);
//! * drag and drop between columns (`UpdateTaskState`) and within a column (`ReorderTask`);
//! * an Undo toast after a move.
//!
//! Cards waiting on a human answer (`needs_input`) are locked: `UpdateTaskState` clears
//! `needs_input` on any state change, which would silently discard the agent's question, so those
//! are answered from Task Detail instead.

use std::collections::HashMap;

use dioxus::prelude::*;
use draft_api::proto::tooling_lineman_v1::{
    lineman_service_client::LinemanServiceClient, Agent, AgentKind, watch_response, CreateTaskRequest, GetObjectiveRequest,
    ListLoopsRequest, ListScheduledTasksRequest, ListTasksRequest, Loop, LoopStatus, Objective, Priority,
    ReorderTaskRequest, ScheduledTask, ScheduledTaskStatus, Task, UpdateTaskStateRequest, WatchRequest,
};
use draft_ui::data::{Avatar, Board, BoardColumn, Card, CardMeta, CardTitle, List, ListRow, Progress, ProgressSegment};
use draft_ui::layout::PageHead;
use draft_ui::shell::{use_page_chrome, BarItem};
use draft_ui::ui::{
    use_toast, Alert, Btn, BtnSize, BtnVariant, Empty, Field, Loading, Menu, MenuItem, Modal, RouteLink, Select, Seg,
    Status, Tag, TextInput, Textarea, Toast,
};
use draft_ui::util::truncate_middle;
use draft_ui::{StatusKind, Tone};
use tonic_web_wasm_client::Client as WasmClient;

use crate::components::{agent_kind_label, format_date, format_when, priority_options, recurrence_label, PriorityTag};
use crate::rail::use_rail;
use crate::state::{classify, column_order, midpoint, short_id, summarize, StateKind};
use crate::Route;

fn client() -> LinemanServiceClient<WasmClient> {
    LinemanServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()))
}

/// `/objectives/:id`. Keyed by id so navigating between objectives remounts the view and its
/// data (hooks otherwise keep the previous objective's resources).
#[component]
pub fn ObjectiveDetail(id: String) -> Element {
    rsx! { ObjectiveView { key: "{id}", id: id.clone() } }
}

/// `/objectives/:id/board` — the same view; the URL is kept as a deep link.
#[component]
pub fn TaskBoard(id: String) -> Element {
    rsx! { ObjectiveView { key: "{id}", id: id.clone() } }
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum ViewMode {
    Board,
    List,
}

/// A request to put a task in a state at a position: after `before` and ahead of `after` (either
/// may be absent for the start or end of the column).
#[derive(Clone, Debug, PartialEq)]
struct MoveRequest {
    task_id: String,
    to_state: String,
    before: Option<String>,
    after: Option<String>,
}

/// Who a task is assigned to, as a card shows them: humans get a round avatar, scripts and AI
/// agents a square one with their name beside it.
#[derive(Clone, Debug, PartialEq)]
struct AgentInfo {
    name: String,
    human: bool,
}

impl From<&Agent> for AgentInfo {
    fn from(a: &Agent) -> Self {
        Self { name: a.display_name.clone(), human: a.kind == AgentKind::Human as i32 }
    }
}

/// The assignee of a task, or `None` when unassigned. An id Lineman has no agent record for is
/// shown as-is, as an agent.
fn assignee_of(agents: &HashMap<String, AgentInfo>, agent_id: &str) -> Option<AgentInfo> {
    if agent_id.is_empty() {
        return None;
    }
    Some(agents.get(agent_id).cloned().unwrap_or_else(|| AgentInfo { name: agent_id.to_string(), human: false }))
}

fn initials_of(name: &str) -> String {
    name.chars().take(2).collect::<String>().to_uppercase()
}

/// `daily at 09:00`, or nothing when the loop has no recurrence.
fn loop_recurrence(l: &Loop) -> String {
    l.recurrence.as_ref().map(|r| recurrence_label(&r.kind, r.interval_days, &r.at)).unwrap_or_default()
}

/// A scheduled task's status as a tag: pending is blue, anything else quiet.
fn scheduled_status(status: i32) -> (Tone, String) {
    let parsed = ScheduledTaskStatus::try_from(status).unwrap_or(ScheduledTaskStatus::Unspecified);
    let label = parsed.as_str_name().trim_start_matches("SCHEDULED_TASK_STATUS_").to_lowercase();
    let tone = if parsed == ScheduledTaskStatus::Pending { Tone::Ca } else { Tone::Quiet };
    (tone, label)
}

/// The last task in `state` other than `except` — where a card goes when it is appended to a
/// column.
fn last_in_state(tasks: &[Task], state: &str, except: &str) -> Option<String> {
    let mut col: Vec<&Task> = tasks.iter().filter(|t| t.state == state && t.id != except).collect();
    col.sort_by(|a, b| column_order(a, b));
    col.last().map(|t| t.id.clone())
}

#[component]
fn ObjectiveView(id: String) -> Element {
    let rail = use_rail();
    let toast = use_toast();

    let mut mode = use_signal(|| ViewMode::Board);
    let mut filter = use_signal(String::new);
    let mut priority_filter = use_signal(|| "any".to_string());
    let mut assignee_filter = use_signal(|| "any".to_string());
    let mut undo: Signal<Option<(String, String)>> = use_signal(|| None);

    // Add-task modal.
    let mut show_add = use_signal(|| false);
    let mut add_name = use_signal(String::new);
    let mut add_details = use_signal(String::new);
    let mut add_priority = use_signal(|| "PRIORITY_MEDIUM".to_string());
    let mut add_error: Signal<Option<String>> = use_signal(|| None);

    // Data ------------------------------------------------------------------------------------------
    let id_obj = id.clone();
    let objective = use_resource(move || {
        let id = id_obj.clone();
        async move { client().get_objective(GetObjectiveRequest { id }).await.map(|r| r.into_inner()).map_err(|e| e.message().to_string()) }
    });

    let id_tasks = id.clone();
    let mut tasks_result = use_resource(move || {
        let objective_id = id_tasks.clone();
        async move {
            client()
                .list_tasks(ListTasksRequest { objective_id, ..Default::default() })
                .await
                .map(|r| r.into_inner().tasks)
                .map_err(|e| e.message().to_string())
        }
    });
    let mut tasks: Signal<Vec<Task>> = use_signal(Vec::new);
    use_effect(move || {
        if let Some(Ok(list)) = &*tasks_result.read() {
            tasks.set(list.clone());
        }
    });

    // Live updates: upsert (or remove) this objective's tasks as the Watch stream reports them.
    let id_watch = id.clone();
    use_coroutine(move |_rx: UnboundedReceiver<()>| {
        let objective_id = id_watch.clone();
        async move {
            let Ok(response) = client().watch(WatchRequest {}).await else {
                return;
            };
            let mut stream = response.into_inner();
            while let Ok(Some(msg)) = stream.message().await {
                let removed = msg.removed;
                if let Some(watch_response::Item::Task(t)) = msg.item {
                    if t.objective_id != objective_id {
                        continue;
                    }
                    let mut list = tasks.write();
                    if removed {
                        list.retain(|x| x.id != t.id);
                    } else if let Some(existing) = list.iter_mut().find(|x| x.id == t.id) {
                        *existing = t;
                    } else {
                        list.push(t);
                    }
                }
            }
        }
    });

    let id_loops = id.clone();
    let loops_result = use_resource(move || {
        let objective_id = id_loops.clone();
        async move { client().list_loops(ListLoopsRequest { objective_id }).await.map(|r| r.into_inner().loops) }
    });
    let id_sched = id.clone();
    let sched_result = use_resource(move || {
        let objective_id = id_sched.clone();
        async move { client().list_scheduled_tasks(ListScheduledTasksRequest { objective_id }).await.map(|r| r.into_inner().scheduled_tasks) }
    });

    // The page's own chrome: read the resource *inside* the closure so it stays current.
    use_page_chrome(move || {
        let (name, short) = match &*objective.read() {
            Some(Ok(o)) => (o.name.clone(), truncate_middle(&o.id, 8, 0)),
            _ => ("…".to_string(), String::new()),
        };
        let mut chrome = rail.chrome(&["Lineman", "Objectives", &name]);
        chrome.left = vec![BarItem::kv("Objective", short)];
        chrome.right = vec![BarItem::Text("Live updates on".to_string())];
        chrome
    });

    // Derived ---------------------------------------------------------------------------------------
    let obj: Option<Objective> = match &*objective.read() {
        Some(Ok(o)) => Some(o.clone()),
        _ => None,
    };
    let all_tasks = tasks();
    let states = obj.as_ref().map(|o| o.states.clone()).unwrap_or_default();
    let summary = obj.as_ref().map(|o| summarize(o, &all_tasks)).unwrap_or_default();

    let agents = rail.get().map(|s| s.agents).unwrap_or_default();
    let agent_infos: HashMap<String, AgentInfo> = agents.iter().map(|a| (a.id.clone(), AgentInfo::from(a))).collect();

    // Filters.
    let query = filter().to_lowercase();
    let prio = priority_filter();
    let assignee = assignee_filter();
    let visible: Vec<Task> = all_tasks
        .iter()
        .filter(|t| query.is_empty() || t.name.to_lowercase().contains(&query))
        .filter(|t| match prio.as_str() {
            "high" => t.priority == Priority::High as i32,
            "medium" => t.priority == Priority::Medium as i32,
            "low" => t.priority == Priority::Low as i32,
            _ => true,
        })
        .filter(|t| match assignee.as_str() {
            "any" => true,
            "none" => t.agent_id.is_empty(),
            id => t.agent_id == id,
        })
        .cloned()
        .collect();
    let mut assignee_ids: Vec<String> = all_tasks.iter().filter(|t| !t.agent_id.is_empty()).map(|t| t.agent_id.clone()).collect();
    assignee_ids.sort();
    assignee_ids.dedup();
    let mut assignee_options = vec![("any".to_string(), "Assignee: any".to_string()), ("none".to_string(), "Unassigned".to_string())];
    for a in &assignee_ids {
        assignee_options.push((a.clone(), assignee_of(&agent_infos, a).map(|i| i.name).unwrap_or_default()));
    }
    let priority_filters = vec![
        ("any".to_string(), "Priority: any".to_string()),
        ("high".to_string(), "High".to_string()),
        ("medium".to_string(), "Medium".to_string()),
        ("low".to_string(), "Low".to_string()),
    ];

    // Actions ---------------------------------------------------------------------------------------
    let id_submit = id.clone();
    let submit = use_callback(move |_: ()| {
        let name = add_name();
        if name.trim().is_empty() {
            add_error.set(Some("Name is required".to_string()));
            return;
        }
        let details = add_details();
        let priority = Priority::from_str_name(&add_priority()).unwrap_or(Priority::Medium);
        let objective_id = id_submit.clone();
        spawn(async move {
            let created = client()
                .create_task(CreateTaskRequest { objective_id, name, details, priority: priority as i32, state: String::new() })
                .await;
            match created {
                Ok(_) => {
                    add_name.set(String::new());
                    add_details.set(String::new());
                    add_error.set(None);
                    show_add.set(false);
                    tasks_result.restart();
                    rail.refresh();
                }
                Err(err) => add_error.set(Some(err.message().to_string())),
            }
        });
    });

    // Moves are optimistic: the card jumps at once and is rolled back if the server refuses.
    let on_move = use_callback(move |mut req: MoveRequest| {
        let Some(prev) = tasks.peek().iter().find(|t| t.id == req.task_id).cloned() else {
            return;
        };
        let from_state = prev.state.clone();
        // No neighbours means "append to the target column".
        if req.before.is_none() && req.after.is_none() {
            req.before = last_in_state(&tasks.peek(), &req.to_state, &req.task_id);
        }
        {
            let mut list = tasks.write();
            let orders: HashMap<String, i64> = list.iter().map(|t| (t.id.clone(), t.order)).collect();
            if let Some(t) = list.iter_mut().find(|t| t.id == req.task_id) {
                t.state = req.to_state.clone();
                if req.before.is_some() || req.after.is_some() {
                    let b = req.before.as_ref().and_then(|id| orders.get(id).copied());
                    let a = req.after.as_ref().and_then(|id| orders.get(id).copied());
                    t.order = midpoint(b, a);
                }
            }
        }
        spawn(async move {
            let mut c = client();
            let mut failure: Option<String> = None;
            if from_state != req.to_state {
                let moved = c
                    .update_task_state(UpdateTaskStateRequest { task_id: req.task_id.clone(), new_state: req.to_state.clone() })
                    .await;
                if let Err(e) = moved {
                    failure = Some(e.message().to_string());
                }
            }
            // Two non-atomic calls when a drop also picks a position; the Watch stream reconciles.
            if failure.is_none() && (req.before.is_some() || req.after.is_some()) {
                let placed = c
                    .reorder_task(ReorderTaskRequest {
                        task_id: req.task_id.clone(),
                        before_task_id: req.before.clone().unwrap_or_default(),
                        after_task_id: req.after.clone().unwrap_or_default(),
                    })
                    .await;
                if let Err(e) = placed {
                    failure = Some(e.message().to_string());
                }
            }
            match failure {
                None if from_state != req.to_state => {
                    undo.set(Some((req.task_id.clone(), from_state)));
                    toast.show(format!("Moved to {}", req.to_state), 6000);
                    rail.refresh();
                }
                None => {}
                Some(err) => {
                    let mut list = tasks.write();
                    if let Some(t) = list.iter_mut().find(|t| t.id == req.task_id) {
                        *t = prev;
                    }
                    undo.set(None);
                    toast.show(format!("Could not move task: {err}"), 6000);
                }
            }
        });
    });

    let undo_handler = EventHandler::new(move |_: ()| {
        let pending = undo.peek().clone();
        if let Some((task_id, state)) = pending {
            on_move.call(MoveRequest { task_id, to_state: state, before: None, after: None });
            undo.set(None);
        }
    });

    // Render ----------------------------------------------------------------------------------------
    if let Some(Err(err)) = &*objective.read() {
        return rsx! {
            Alert { kind: StatusKind::Err, "Failed to load objective: {err}" }
        };
    }
    let Some(obj) = obj else {
        return rsx! { Loading {} };
    };

    let (done, in_flight, queued) = summary.percents();
    let segments = vec![
        ProgressSegment::new(Tone::Primary, done),
        ProgressSegment::new(Tone::Ca, in_flight),
        ProgressSegment::new(Tone::Quiet, queued),
    ];
    let progress_label = format!("{} done, {} in flight, {} queued", summary.done, summary.in_flight, summary.queued);
    let eyebrow = format!("Objective · created {}", format_date(summary.created_at));
    let is_board = mode() == ViewMode::Board;
    let loops: Vec<Loop> = match &*loops_result.read() {
        Some(Ok(l)) => l.clone(),
        _ => Vec::new(),
    };
    let scheduled: Vec<ScheduledTask> = match &*sched_result.read() {
        Some(Ok(s)) => s.clone(),
        _ => Vec::new(),
    };
    let assigned_agents: Vec<_> = agents.iter().filter(|a| assignee_ids.contains(&a.id)).cloned().collect();

    rsx! {
        PageHead {
            title: obj.name.clone(),
            eyebrow: eyebrow,
            description: obj.description.clone(),
            actions: rsx! {
                Seg::<ViewMode> {
                    options: vec![(ViewMode::Board, "Board".to_string()), (ViewMode::List, "List".to_string())],
                    value: mode(),
                    on_change: move |m| mode.set(m),
                    label: "View".to_string(),
                }
                Btn { variant: BtnVariant::Primary, onclick: move |_| show_add.set(true), "+ Add task" }
            },
        }

        div { class: "ob-progress",
            Progress { segments, large: true, label: progress_label }
            div { class: "ob-stats",
                div { span { class: "d-label", "Total" } b { "{summary.total}" } }
                div { span { class: "d-label", style: "color:var(--primary)", "Done" } b { "{summary.done}" } }
                div { span { class: "d-label", style: "color:var(--ca)", "In flight" } b { "{summary.in_flight}" } }
                div { span { class: "d-label", "Queued" } b { "{summary.queued}" } }
                div { span { class: "d-label", "High priority open" } b { style: "color:var(--err)", "{summary.high_open}" } }
            }
        }

        div { class: "ob-bar",
            input {
                class: "d-input",
                r#type: "search",
                placeholder: "Filter tasks…",
                aria_label: "Filter tasks",
                value: "{filter}",
                oninput: move |e| filter.set(e.value()),
            }
            Select { value: prio.clone(), options: priority_filters, on_change: move |v| priority_filter.set(v), aria_label: "Priority".to_string() }
            Select { value: assignee.clone(), options: assignee_options, on_change: move |v| assignee_filter.set(v), aria_label: "Assignee".to_string() }
            span { class: "d-spacer" }
            if is_board {
                span { class: "d-label", "Drag cards between columns" }
            }
        }

        if is_board {
            BoardView {
                objective_id: id.clone(),
                states: states.clone(),
                tasks: visible.clone(),
                loops: loops.clone(),
                agents: agent_infos.clone(),
                on_move: move |r| on_move.call(r),
                on_add: move |_| show_add.set(true),
            }
        } else {
            ListView { objective_id: id.clone(), states: states.clone(), tasks: visible.clone(), agents: agent_infos.clone() }
        }

        if !assigned_agents.is_empty() || !scheduled.is_empty() || !loops.is_empty() {
            div { class: "ob-auto",
                if !assigned_agents.is_empty() {
                    div { class: "d-panel",
                        div { class: "d-panel-head", span { class: "d-label", "Active agents · {assigned_agents.len()}" } }
                        List {
                            for a in assigned_agents {
                                {
                                    let initials = initials_of(&a.display_name);
                                    let kind_label = agent_kind_label(a.kind);
                                    rsx! {
                                        ListRow { key: "{a.id}",
                                            Avatar { text: initials, agent: a.kind != AgentKind::Human as i32 }
                                            "{a.display_name}"
                                            span { class: "d-spacer" }
                                            small { "{kind_label}" }
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
                if !scheduled.is_empty() {
                    div { class: "d-panel",
                        div { class: "d-panel-head", span { class: "d-label", "Scheduled · {scheduled.len()}" } }
                        List {
                            for s in scheduled {
                                {
                                    let when = format_when(s.fire_at.as_ref().map(|t| t.seconds));
                                    let (tone, label) = scheduled_status(s.status);
                                    rsx! {
                                        ListRow { key: "{s.id}", to: "/scheduler".to_string(),
                                            span { class: "d-trunc", "{s.description}" }
                                            span { class: "d-spacer" }
                                            small { "{when}" }
                                            Tag { tone, "{label}" }
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
                if !loops.is_empty() {
                    div { class: "d-panel",
                        div { class: "d-panel-head", span { class: "d-label", "Loops · {loops.len()}" } }
                        List {
                            for l in loops {
                                {
                                    let recurrence = loop_recurrence(&l);
                                    rsx! {
                                        ListRow { key: "{l.id}", to: "/loops".to_string(),
                                            span { class: "d-trunc", "{l.description}" }
                                            span { class: "d-spacer" }
                                            small { "{recurrence}" }
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }

        Modal {
            open: show_add(),
            title: "Add task".to_string(),
            on_close: move |_| show_add.set(false),
            footer: rsx! {
                Btn { variant: BtnVariant::Ghost, onclick: move |_| show_add.set(false), "Cancel" }
                Btn { variant: BtnVariant::Primary, onclick: move |_| submit.call(()), "Add task" }
            },
            if let Some(msg) = add_error() {
                Alert { kind: StatusKind::Err, "{msg}" }
            }
            Field { label: "Name".to_string(),
                TextInput {
                    value: add_name(),
                    oninput: move |v| add_name.set(v),
                    placeholder: "What needs doing".to_string(),
                    onkeydown: move |e: KeyboardEvent| {
                        if e.key() == Key::Enter {
                            submit.call(());
                        }
                    },
                }
            }
            Field { label: "Details".to_string(), hint: "Everything needed to complete this task (optional)".to_string(),
                Textarea { value: add_details(), oninput: move |v| add_details.set(v) }
            }
            Field { label: "Priority".to_string(),
                Select {
                    value: add_priority(),
                    on_change: move |v| add_priority.set(v),
                    options: priority_options(),
                }
            }
        }

        Toast { state: toast, on_undo: if undo().is_some() { Some(undo_handler) } else { None } }
    }
}

// Board ---------------------------------------------------------------------------------------------

#[component]
fn BoardView(
    objective_id: String,
    states: Vec<String>,
    tasks: Vec<Task>,
    loops: Vec<Loop>,
    agents: HashMap<String, AgentInfo>,
    on_move: EventHandler<MoveRequest>,
    on_add: EventHandler<()>,
) -> Element {
    let dragging: Signal<Option<String>> = use_signal(|| None);
    let over_col: Signal<Option<String>> = use_signal(|| None);
    let over_card: Signal<Option<String>> = use_signal(|| None);
    let menu_for: Signal<Option<String>> = use_signal(|| None);

    if states.is_empty() {
        return rsx! {
            Empty { title: "No states".to_string(), "This objective has no task states defined." }
        };
    }

    rsx! {
        Board {
            for (i , state) in states.clone().into_iter().enumerate() {
                {
                    let mut column: Vec<Task> = tasks.iter().filter(|t| t.state == state).cloned().collect();
                    column.sort_by(column_order);
                    rsx! {
                        Column {
                            key: "{state}",
                            state: state.clone(),
                            states: states.clone(),
                            objective_id: objective_id.clone(),
                            tasks: column,
                            // Loops queue new work, so they sit in the first column.
                            loops: if i == 0 { loops.clone() } else { Vec::new() },
                            agents: agents.clone(),
                            dragging,
                            over_col,
                            over_card,
                            menu_for,
                            on_move,
                            on_add,
                            first: i == 0,
                        }
                    }
                }
            }
        }
    }
}

#[component]
fn Column(
    state: String,
    states: Vec<String>,
    objective_id: String,
    tasks: Vec<Task>,
    loops: Vec<Loop>,
    agents: HashMap<String, AgentInfo>,
    dragging: Signal<Option<String>>,
    over_col: Signal<Option<String>>,
    over_card: Signal<Option<String>>,
    menu_for: Signal<Option<String>>,
    on_move: EventHandler<MoveRequest>,
    on_add: EventHandler<()>,
    first: bool,
) -> Element {
    let kind = classify(&states, &state);
    let is_over = over_col().as_deref() == Some(state.as_str()) && dragging().is_some();
    let count = tasks.len() as u32;

    let state_for_over = state.clone();
    let state_for_leave = state.clone();
    let state_for_drop = state.clone();
    let tasks_for_drop = tasks.clone();

    rsx! {
        BoardColumn {
            title: state.clone(),
            tone: kind.tone(),
            count,
            drop_active: is_over,
            on_drag_over: move |_| over_col.set(Some(state_for_over.clone())),
            on_drag_leave: move |_| {
                if over_col.peek().as_deref() == Some(state_for_leave.as_str()) {
                    over_col.set(None);
                }
            },
            // A drop on the column itself (not on a card) appends to it.
            on_drop: move |_| {
                let mut dragging = dragging;
                if let Some(task_id) = dragging.peek().clone() {
                    let before = tasks_for_drop.iter().rev().find(|t| t.id != task_id).map(|t| t.id.clone());
                    on_move.call(MoveRequest { task_id, to_state: state_for_drop.clone(), before, after: None });
                }
                dragging.set(None);
                over_col.set(None);
                over_card.set(None);
            },
            for l in loops {
                LoopCard { key: "{l.id}", l: l.clone() }
            }
            for t in tasks.clone() {
                TaskCard {
                    key: "{t.id}",
                    task: t.clone(),
                    column: tasks.clone(),
                    state: state.clone(),
                    states: states.clone(),
                    objective_id: objective_id.clone(),
                    agent: assignee_of(&agents, &t.agent_id),
                    dragging,
                    over_col,
                    over_card,
                    menu_for,
                    on_move,
                }
            }
            if first {
                button { class: "d-add", r#type: "button", onclick: move |_| on_add.call(()), "+ Add task" }
            }
        }
    }
}

#[component]
fn TaskCard(
    task: Task,
    /// The column this card is in, in display order (for computing neighbours on a drop).
    column: Vec<Task>,
    state: String,
    states: Vec<String>,
    objective_id: String,
    agent: Option<AgentInfo>,
    dragging: Signal<Option<String>>,
    over_col: Signal<Option<String>>,
    over_card: Signal<Option<String>>,
    menu_for: Signal<Option<String>>,
    on_move: EventHandler<MoveRequest>,
) -> Element {
    let nav = use_navigator();
    let locked = task.needs_input.is_some();
    let has_agent = !task.agent_id.is_empty();
    let is_dragging = dragging().as_deref() == Some(task.id.as_str());
    let drop_before = dragging().is_some() && !is_dragging && over_card().as_deref() == Some(task.id.as_str());
    let menu_open = menu_for().as_deref() == Some(task.id.as_str());
    let short = short_id(&task.id);
    // The card's second row: what it is waiting on, what it is doing, or that nobody has it.
    let doing = if task.current_action.is_empty() { "Running".to_string() } else { task.current_action.clone() };
    let kind = classify(&states, &state);
    let in_flight = kind == StateKind::InFlight;
    let done = kind == StateKind::Done;
    let finished = format!("Done {}", format_when(task.updated_at.as_ref().map(|t| t.seconds)));

    let task_id = task.id.clone();
    let id_nav = task.id.clone();
    let obj_nav = objective_id.clone();
    let id_menu = task.id.clone();
    let id_over = task.id.clone();
    let id_drop = task.id.clone();
    let state_drop = state.clone();
    let column_drop = column.clone();
    let id_move = task.id.clone();
    let id_select = task.id.clone();

    let items: Vec<MenuItem> = states.iter().map(|s| MenuItem::new(s.clone(), s.clone()).checked(*s == state)).collect();

    // Corner brackets mark work an agent is doing right now, not everything that has an assignee.
    let bracket = if in_flight && has_agent && !locked { Some(Tone::Ca) } else { None };

    rsx! {
        Card {
            draggable: true,
            locked,
            dragging: is_dragging,
            dim: done,
            drop_before,
            bracket,
            drag_id: task_id,
            on_drag_start: move |_| {
                let mut d = dragging;
                d.set(Some(id_menu.clone()));
            },
            on_drag_end: move |_| {
                let (mut d, mut c, mut k) = (dragging, over_col, over_card);
                d.set(None);
                c.set(None);
                k.set(None);
            },
            on_drag_over: move |_| {
                let (mut k, mut c) = (over_card, over_col);
                k.set(Some(id_over.clone()));
                c.set(Some(state.clone()));
            },
            // Dropped on a card: land just above it.
            on_drop: move |_| {
                let (mut d, mut c, mut k) = (dragging, over_col, over_card);
                if let Some(dragged) = d.peek().clone() {
                    if dragged != id_drop {
                        let idx = column_drop.iter().position(|t| t.id == id_drop).unwrap_or(0);
                        let before = column_drop[..idx].iter().rev().find(|t| t.id != dragged).map(|t| t.id.clone());
                        on_move.call(MoveRequest { task_id: dragged, to_state: state_drop.clone(), before, after: Some(id_drop.clone()) });
                    }
                }
                d.set(None);
                c.set(None);
                k.set(None);
            },
            onclick: move |_| {
                nav.push(Route::TaskDetail { id: obj_nav.clone(), task_id: id_nav.clone() });
            },
            CardMeta {
                PriorityTag { priority: task.priority }
                if locked {
                    Tag { tone: Tone::Warn, "Needs input" }
                }
                span { class: "d-spacer" }
                if !locked {
                    // Clicks in the menu must not open the card.
                    div { class: "ob-card-menu", onclick: move |e| e.stop_propagation(),
                        Btn {
                            variant: BtnVariant::Ghost,
                            size: BtnSize::Sm,
                            icon: true,
                            aria_label: "Move to…".to_string(),
                            onclick: move |e: MouseEvent| {
                                e.stop_propagation();
                                let mut m = menu_for;
                                let open = m.peek().as_deref() == Some(id_move.as_str());
                                m.set(if open { None } else { Some(id_move.clone()) });
                            },
                            "⋯"
                        }
                        if menu_open {
                            Menu {
                                heading: "Move to".to_string(),
                                items,
                                // Appended to the target column.
                                on_select: move |to: String| {
                                    on_move.call(MoveRequest { task_id: id_select.clone(), to_state: to, before: None, after: None });
                                },
                                on_close: move |_| {
                                    let mut m = menu_for;
                                    m.set(None);
                                },
                            }
                        }
                    }
                }
                span { class: "id", "{short}" }
            }
            CardTitle { "{task.name}" }
            CardMeta {
                if locked {
                    Status { kind: StatusKind::Warn, "Waiting on you" }
                } else if done {
                    span { "{finished}" }
                } else if in_flight && has_agent {
                    Status { kind: StatusKind::Info, live: true, "{doing}" }
                } else if has_agent {
                    span { "Assigned" }
                } else {
                    span { "Unassigned" }
                }
                span { class: "d-spacer" }
                if let Some(a) = agent {
                    Avatar { text: initials_of(&a.name), agent: !a.human, title: a.name.clone() }
                    if !a.human {
                        span { "{a.name}" }
                    }
                }
            }
        }
    }
}

#[component]
fn LoopCard(l: Loop) -> Element {
    let status = LoopStatus::try_from(l.status).unwrap_or(LoopStatus::Unspecified);
    let recurrence = loop_recurrence(&l);
    let next = format_when(l.next_fire_at.as_ref().map(|t| t.seconds));
    rsx! {
        Card { group: true,
            CardMeta {
                PriorityTag { priority: l.priority }
                Tag { tone: Tone::Bp, "Loop" }
                if status == LoopStatus::Paused {
                    Tag { "Paused" }
                }
                span { class: "d-spacer" }
                span { class: "id", "×{l.occurrence_count}" }
            }
            CardTitle {
                b { "{l.description}" }
                " · {recurrence}"
            }
            CardMeta {
                span { "next {next}" }
                span { class: "d-spacer" }
                RouteLink { to: "/loops".to_string(), class: "".to_string(), "Manage" }
            }
        }
    }
}

// List ----------------------------------------------------------------------------------------------

#[component]
fn ListView(objective_id: String, states: Vec<String>, tasks: Vec<Task>, agents: HashMap<String, AgentInfo>) -> Element {
    if tasks.is_empty() {
        return rsx! {
            Empty { title: "No tasks match".to_string(), "Clear the filters or add a task." }
        };
    }
    let mut rows = tasks.clone();
    rows.sort_by(|a, b| {
        let ai = states.iter().position(|s| *s == a.state).unwrap_or(usize::MAX);
        let bi = states.iter().position(|s| *s == b.state).unwrap_or(usize::MAX);
        ai.cmp(&bi).then_with(|| column_order(a, b))
    });
    rsx! {
        div { class: "d-panel d-table-wrap",
            table { class: "d-table",
                thead {
                    tr {
                        th { "State" }
                        th { "Priority" }
                        th { "Task" }
                        th { "Assignee" }
                        th { class: "is-right", "Updated" }
                    }
                }
                tbody {
                    for t in rows {
                        {
                            let kind = classify(&states, &t.state);
                            let to = format!("/objectives/{}/tasks/{}", objective_id, t.id);
                            let agent = assignee_of(&agents, &t.agent_id).map(|a| a.name).unwrap_or_else(|| "—".to_string());
                            let updated = format_when(t.updated_at.as_ref().map(|x| x.seconds));
                            rsx! {
                                tr { key: "{t.id}",
                                    td { class: "is-shrink", Tag { tone: kind.tone(), "{t.state}" } }
                                    td { class: "is-shrink", PriorityTag { priority: t.priority } }
                                    td { class: "is-fill",
                                        RouteLink { to, class: "d-trunc".to_string(), "{t.name}" }
                                    }
                                    td { class: "is-dim", "{agent}" }
                                    td { class: "is-right is-dim", "{updated}" }
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}
