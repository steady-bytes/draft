use dioxus::prelude::*;
use gloo_timers::future::TimeoutFuture;

use super::{Btn, BtnSize, BtnVariant};

/// Reactive state for a single toast. Create with [`use_toast`].
#[derive(Clone, Copy, PartialEq)]
pub struct ToastState {
    message: Signal<Option<String>>,
}

impl ToastState {
    /// Shows `message` and clears it after `ms` milliseconds.
    pub fn show(&self, message: impl Into<String>, ms: u32) {
        let mut slot = self.message;
        let message = message.into();
        slot.set(Some(message.clone()));
        spawn(async move {
            TimeoutFuture::new(ms).await;
            // Only clear our own message; a newer toast may have replaced it.
            if slot.peek().as_deref() == Some(message.as_str()) {
                slot.set(None);
            }
        });
    }

    pub fn dismiss(&self) {
        let mut slot = self.message;
        slot.set(None);
    }

    pub fn current(&self) -> Option<String> {
        self.message.read().clone()
    }
}

pub fn use_toast() -> ToastState {
    ToastState { message: use_signal(|| None) }
}

/// The toast itself (`d-toast`), bottom-left above the status bar. Pass `on_undo` to show Undo.
#[component]
pub fn Toast(state: ToastState, on_undo: Option<EventHandler<()>>) -> Element {
    let Some(message) = state.current() else {
        return rsx! {};
    };
    rsx! {
        div { class: "d-toast", role: "status",
            span { "{message}" }
            if let Some(undo) = on_undo {
                Btn {
                    variant: BtnVariant::Ghost,
                    size: BtnSize::Sm,
                    onclick: move |_| {
                        undo.call(());
                        state.dismiss();
                    },
                    "Undo"
                }
            }
        }
    }
}
