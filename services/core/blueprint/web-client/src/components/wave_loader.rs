use dioxus::prelude::*;

const BAR_DURATIONS: [f32; 7] = [0.9, 0.7, 1.1, 0.6, 1.3, 0.8, 1.0];
const BAR_DELAYS:    [f32; 7] = [0.0, 0.15, 0.3, 0.45, 0.2, 0.35, 0.1];

// Embedded so the component has no external CSS dependency.
const KEYFRAMES: &str = "@keyframes wave-bar {\
    0%,100%{transform:scaleY(0.06)}\
    50%{transform:scaleY(1)}\
}";

#[derive(Props, Clone, PartialEq)]
pub struct WaveLoaderProps {
    #[props(default = 80)]
    pub width: u32,
    #[props(default = 30)]
    pub height: u32,
}

#[component]
pub fn WaveLoader(props: WaveLoaderProps) -> Element {
    let w = props.width as f32;
    let h = props.height as f32;
    let n: usize = 7;
    let bar_w = 2.0_f32;
    let gap = (w - n as f32 * bar_w) / (n as f32 + 1.0);

    rsx! {
        svg {
            width: "{props.width}",
            height: "{props.height}",
            view_box: "0 0 {w:.0} {h:.0}",
            xmlns: "http://www.w3.org/2000/svg",
            style { "{KEYFRAMES}" }
            for i in 0..n {
                {
                    let x = gap + i as f32 * (bar_w + gap);
                    let dur = BAR_DURATIONS[i];
                    let delay = BAR_DELAYS[i];
                    rsx! {
                        rect {
                            key: "{i}",
                            x: "{x:.2}",
                            y: "0",
                            width: "{bar_w}",
                            height: "{h}",
                            fill: "white",
                            rx: "1",
                            style: "transform-box:fill-box;transform-origin:center;animation:wave-bar {dur}s ease-in-out {delay}s infinite;",
                        }
                    }
                }
            }
        }
    }
}
