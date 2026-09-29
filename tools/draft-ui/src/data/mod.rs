//! Data display: stat tiles, key/value lists, code blocks, strips, meters, lists, boards.

mod highlight;
pub use highlight::{highlight, CodeLang, Span};

#[cfg(feature = "dioxus")]
mod board;
#[cfg(feature = "dioxus")]
mod code_block;
#[cfg(feature = "dioxus")]
mod kv;
#[cfg(feature = "dioxus")]
mod list;
#[cfg(feature = "dioxus")]
mod meter;
#[cfg(feature = "dioxus")]
mod stat_tile;
#[cfg(feature = "dioxus")]
mod strip;

#[cfg(feature = "dioxus")]
pub use board::{Avatar, Board, BoardColumn, Card, CardMeta, CardTitle};
#[cfg(feature = "dioxus")]
pub use code_block::CodeBlock;
#[cfg(feature = "dioxus")]
pub use kv::{Kv, KvItem};
#[cfg(feature = "dioxus")]
pub use list::{List, ListRow};
#[cfg(feature = "dioxus")]
pub use meter::{DurationCell, Meter, Progress, ProgressSegment};
#[cfg(feature = "dioxus")]
pub use stat_tile::{Delta, StatTile};
#[cfg(feature = "dioxus")]
pub use strip::{CellState, Strip, StripCell, StripSize};
