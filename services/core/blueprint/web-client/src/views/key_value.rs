use crate::Route;
use dioxus::prelude::*;

// const KEY_VALUE_CSS: Asset = asset!("/assets/styling/key_value.css");

#[component]
pub fn KeyValue() -> Element {
    rsx! {
        // document::Link { rel: "stylesheet", href: KEY_VALUE_CSS }

        div {
            id: "key-value",

            // Content
            h1 { "Key-Value Store" }
            p { "This is the key-value store page. Here, we will show how to use the Dioxus store to manage state in our application." }
        }
    }
}