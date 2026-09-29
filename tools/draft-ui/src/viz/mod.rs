//! Charts and diagrams.

pub mod geom;

#[cfg(feature = "dioxus")]
mod histogram;
#[cfg(feature = "dioxus")]
mod sparkline;
#[cfg(feature = "dioxus")]
mod time_series;
#[cfg(feature = "dioxus")]
mod waterfall;
#[cfg(feature = "dioxus")]
mod wire;

#[cfg(feature = "dioxus")]
pub use histogram::{HistoBucket, Histogram, Legend};
#[cfg(feature = "dioxus")]
pub use sparkline::Sparkline;
#[cfg(feature = "dioxus")]
pub use time_series::{ChartSeries, SeriesTable, TimeSeriesChart};
#[cfg(feature = "dioxus")]
pub use waterfall::{CompactRow, CompactWaterfall, Waterfall, WaterfallRow};
#[cfg(feature = "dioxus")]
pub use wire::{Pad, Via, Wire, WireKind};
