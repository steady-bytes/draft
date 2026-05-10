use dioxus::prelude::*;

const COLORS: [&str; 5] = [
    "badge-success",
    "badge-warning",
    "badge-error",
    "badge-info",
    "badge-accent",
];

fn badge_color(event_type: &str) -> &'static str {
    let hash = event_type
        .bytes()
        .fold(0usize, |acc, b| acc.wrapping_mul(31).wrapping_add(b as usize));
    COLORS[hash % COLORS.len()]
}

#[component]
pub fn TypeBadge(event_type: String) -> Element {
    let color = badge_color(&event_type);
    rsx! {
        span { class: "flex items-center gap-1.5",
            span { class: "badge badge-xs {color}" }
            span { class: "text-xs", "{event_type}" }
        }
    }
}
