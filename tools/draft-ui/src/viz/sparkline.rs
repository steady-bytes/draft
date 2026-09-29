use dioxus::prelude::*;

use super::geom::spark_path;
use crate::kinds::Tone;

/// A tiny trend line (`d-spark`), 96×24 by default.
#[component]
pub fn Sparkline(values: Vec<f64>, #[props(default = Tone::Primary)] tone: Tone) -> Element {
    let d = spark_path(&values, 96.0, 24.0);
    let style = format!("--c:{}", tone.css_var());
    rsx! {
        svg { class: "d-spark", view_box: "0 0 96 24", style: "{style}", "aria-hidden": "true",
            path { d: "{d}" }
        }
    }
}
