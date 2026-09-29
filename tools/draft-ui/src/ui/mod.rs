//! Primitives: the small controls and indicators every view is built from.

mod alert;
mod btn;
mod chip;
mod empty;
mod field;
mod glyph;
mod link;
mod menu;
mod modal;
mod seg;
mod status;
mod tag;
mod toast;
mod toggle;

pub use alert::Alert;
pub use btn::{Btn, BtnSize, BtnVariant};
pub use chip::Chip;
pub use empty::{Empty, Loading, NotFound, Skeleton};
pub use field::{Check, Field, Select, TextInput, Textarea};
pub use glyph::{AppGlyph, Glyph};
pub use link::RouteLink;
pub use menu::{Menu, MenuItem};
pub use modal::Modal;
pub use seg::Seg;
pub use status::{Count, Dot, Kbd, Status};
pub use tag::{Svc, Tag, TraceLink};
pub use toast::{use_toast, Toast, ToastState};
pub use toggle::Toggle;

/// Joins class fragments, skipping empty ones.
pub(crate) fn cx(parts: &[&str]) -> String {
    parts.iter().filter(|p| !p.is_empty()).copied().collect::<Vec<_>>().join(" ")
}
