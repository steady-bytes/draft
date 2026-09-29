use dioxus::prelude::*;

/// What one cell of a [`Strip`] shows.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum CellState {
    Ok,
    Err,
    Warn,
    /// In flight (blinks).
    Run,
    /// Never ran / offline.
    None,
    Skip,
}

impl CellState {
    const fn class(self) -> &'static str {
        match self {
            CellState::Ok => "",
            CellState::Err => "is-err",
            CellState::Warn => "is-warn",
            CellState::Run => "is-run",
            CellState::None => "is-none",
            CellState::Skip => "is-skip",
        }
    }
}

#[derive(Clone, Debug, PartialEq)]
pub struct StripCell {
    pub state: CellState,
    pub title: Option<String>,
    /// The item currently being looked at (outlined).
    pub current: bool,
}

impl StripCell {
    pub fn new(state: CellState) -> Self {
        Self { state, title: None, current: false }
    }

    pub fn titled(state: CellState, title: impl Into<String>) -> Self {
        Self { state, title: Some(title.into()), current: false }
    }

    pub fn current(mut self) -> Self {
        self.current = true;
        self
    }
}

#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum StripSize {
    /// 8×16: run history.
    #[default]
    Md,
    /// 6×12: instance bars in a table row.
    Xs,
    /// Full width, 6 px tall: step results.
    Stretch,
    /// Full width, 22 px tall: last-N-runs.
    Tall,
}

/// A row of small cells, one per item (`d-strip`). Replaces four hand-written variants (run
/// history, instance bars, step progress, last-N-runs).
#[component]
pub fn Strip(cells: Vec<StripCell>, #[props(default)] size: StripSize, label: Option<String>) -> Element {
    let class = match size {
        StripSize::Md => "d-strip",
        StripSize::Xs => "d-strip d-strip--xs",
        StripSize::Stretch => "d-strip d-strip--stretch",
        StripSize::Tall => "d-strip d-strip--tall",
    };
    rsx! {
        span { class: "{class}", role: "img", aria_label: label,
            for (i , c) in cells.into_iter().enumerate() {
                {
                    let mut class = c.state.class().to_string();
                    if c.current {
                        if !class.is_empty() {
                            class.push(' ');
                        }
                        class.push_str("is-cur");
                    }
                    rsx! { i { key: "{i}", class: "{class}", title: c.title } }
                }
            }
        }
    }
}
