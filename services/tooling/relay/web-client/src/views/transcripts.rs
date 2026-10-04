use dioxus::prelude::*;
use draft_ui::layout::PageHead;
use draft_ui::ui::Empty;

/// Placeholder while Library and Record are built out first.
#[component]
pub fn Transcripts() -> Element {
    rsx! {
        PageHead {
            title: "Transcripts".to_string(),
            eyebrow: "Capture · archive".to_string(),
        }
        Empty { title: "Not built yet".to_string(), "The archive + search view is last." }
    }
}
