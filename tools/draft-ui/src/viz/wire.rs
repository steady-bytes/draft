use dioxus::prelude::*;

#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum WireKind {
    /// A route: solid primary.
    #[default]
    Route,
    /// A quieter route.
    RouteDim,
    /// An event flow: dashed blue, animated.
    Event,
}

/// A wire in an SVG diagram (`d-wire` / `d-evt`). Use inside an `<svg>`; `d` is an SVG path.
#[component]
pub fn Wire(d: String, #[props(default)] kind: WireKind) -> Element {
    let class = match kind {
        WireKind::Route => "d-wire",
        WireKind::RouteDim => "d-wire d-wire--dim",
        WireKind::Event => "d-evt",
    };
    rsx! {
        path { class: "{class}", d: "{d}" }
    }
}

/// A junction where wires meet (`d-via`).
#[component]
pub fn Via(x: f64, y: f64, #[props(default)] event: bool) -> Element {
    let class = if event { "d-via d-via--e" } else { "d-via" };
    rsx! {
        circle { class: "{class}", cx: "{x}", cy: "{y}", r: "3" }
    }
}

/// A port at a node edge (`d-pad`).
#[component]
pub fn Pad(x: f64, y: f64, #[props(default)] event: bool) -> Element {
    let class = if event { "d-pad d-pad--e" } else { "d-pad" };
    rsx! {
        circle { class: "{class}", cx: "{x}", cy: "{y}", r: "2.5" }
    }
}
