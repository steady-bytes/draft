use dioxus::prelude::*;

/// A segmented control (`d-seg`): time ranges, List / Flame graph, sort, domain filter,
/// Board / List. One generic component replaces `TimeRangePicker` and the ad-hoc `join` toggles.
///
/// `options` pairs a value with its label; the option equal to `value` is pressed.
#[component]
pub fn Seg<T: Clone + PartialEq + 'static>(
    options: Vec<(T, String)>,
    value: T,
    on_change: EventHandler<T>,
    #[props(default)] label: String,
) -> Element {
    rsx! {
        span { class: "d-seg", role: "group", aria_label: "{label}",
            for (i , (opt , text)) in options.into_iter().enumerate() {
                {
                    let pressed = if opt == value { "true" } else { "false" };
                    rsx! {
                        button {
                            key: "{i}",
                            r#type: "button",
                            aria_pressed: "{pressed}",
                            onclick: move |_| on_change.call(opt.clone()),
                            "{text}"
                        }
                    }
                }
            }
        }
    }
}
