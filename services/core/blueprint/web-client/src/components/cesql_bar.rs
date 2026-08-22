use dioxus::prelude::*;

#[component]
pub fn CesqlBar(expression: Signal<String>, on_run: EventHandler<()>, on_clear: EventHandler<()>) -> Element {
    rsx! {
        div { class: "join w-full",
            input {
                class: "input input-bordered input-sm join-item flex-1 font-mono bg-base-200",
                placeholder: "e.g.  type = 'com.shop.order.created'  or  source LIKE '%shop%'",
                value: "{expression}",
                oninput: move |e| expression.set(e.value()),
                onkeydown: move |e| {
                    if e.key() == Key::Enter {
                        on_run.call(());
                    }
                },
            }
            button {
                class: "btn btn-sm btn-ghost join-item text-base-content/50 hover:text-base-content",
                onclick: move |_| on_clear.call(()),
                "✕"
            }
            button {
                class: "btn btn-sm btn-neutral join-item",
                onclick: move |_| on_run.call(()),
                "run"
            }
        }
    }
}
