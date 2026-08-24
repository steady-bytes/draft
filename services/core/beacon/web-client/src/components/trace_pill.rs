use dioxus::prelude::*;

use crate::Route;

/// TracePill renders a shortened `trace_id` as a small clickable pill (Phase
/// 14). Clicking it hands off to the Traces view via `PENDING_TRACE_ID` and
/// navigates there — no backend change needed: `trace_id` has been on every
/// `LogRecord` since Phase 1, and `SearchTraces`/`GetTrace` already accept a
/// `trace_id`-scoped BeaconQL filter (Phase 5). Renders nothing for an empty
/// `trace_id` (most rows outside an active RPC call won't have one).
#[component]
pub fn TracePill(trace_id: String) -> Element {
    if trace_id.is_empty() {
        return rsx! {};
    }

    let short = short_trace_id(&trace_id);
    let nav = use_navigator();

    rsx! {
        button {
            class: "font-mono text-xs text-info/70 hover:text-info hover:underline",
            title: "View trace {trace_id}",
            onclick: move |e| {
                e.stop_propagation();
                *crate::PENDING_TRACE_ID.write() = Some(trace_id.clone());
                nav.push(Route::Traces {});
            },
            "{short}"
        }
    }
}

/// short_trace_id renders the first and last few hex characters of a trace
/// id, e.g. "a91d4f…9c206c" — matches the wireframe's Trace column shape.
fn short_trace_id(id: &str) -> String {
    const HEAD: usize = 6;
    const TAIL: usize = 6;
    if id.chars().count() <= HEAD + TAIL {
        return id.to_string();
    }
    let head: String = id.chars().take(HEAD).collect();
    let tail: String = id.chars().rev().take(TAIL).collect::<Vec<_>>().into_iter().rev().collect();
    format!("{head}…{tail}")
}
