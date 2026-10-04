//! Small, view-agnostic helpers. No reusable *UI* components of Allele's own live here yet --
//! every view so far is built entirely from draft-ui's own component set plus plain `d-*` markup,
//! the same way services/tooling/relay/web-client/src/components/mod.rs stays mostly formatting
//! helpers rather than bespoke widgets.

use prost_types::Timestamp;

/// `prost_types::Timestamp` -> draft-ui's own `relative_time` ("2 min ago") -- every mockup page
/// shows timestamps this way (allele-repos.html's "Last change" column, allele-change.html's
/// "Opened 18 min ago"), never an absolute date.
pub fn relative_time(ts: Option<&Timestamp>) -> String {
    let Some(ts) = ts else { return "—".to_string() };
    let now = chrono::Utc::now().timestamp();
    draft_ui::util::relative_time((now - ts.seconds).max(0))
}
