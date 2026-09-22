use dioxus::prelude::*;
use draft_api::proto::tooling_lineman_v1::{
    lineman_service_client::LinemanServiceClient, CreateObjectiveRequest,
};
use tonic_web_wasm_client::Client as WasmClient;

use crate::Route;

/// Page 3 -- Create Objective. Leaving states empty lets the server default
/// to ["Queued", "In Flight", "Done"] (see the implementation plan's Phase
/// 3), matching the design brief's pre-filled default.
#[component]
pub fn CreateObjective() -> Element {
    let navigator = use_navigator();
    let mut name = use_signal(String::new);
    let mut description = use_signal(String::new);
    let mut states_csv = use_signal(String::new);
    let mut error = use_signal(|| Option::<String>::None);
    let mut submitting = use_signal(|| false);

    let submit = move |_| {
        let name_v = name();
        if name_v.trim().is_empty() {
            error.set(Some("Name is required".to_string()));
            return;
        }
        let description_v = description();
        let states: Vec<String> = states_csv()
            .split(',')
            .map(|s| s.trim().to_string())
            .filter(|s| !s.is_empty())
            .collect();
        submitting.set(true);
        spawn(async move {
            let mut client = LinemanServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
            match client
                .create_objective(CreateObjectiveRequest {
                    name: name_v,
                    description: description_v,
                    states,
                })
                .await
            {
                Ok(resp) => {
                    let id = resp.into_inner().id;
                    navigator.push(Route::ObjectiveDetail { id });
                }
                Err(err) => {
                    submitting.set(false);
                    error.set(Some(err.to_string()));
                }
            }
        });
    };

    rsx! {
        div { class: "max-w-lg flex flex-col gap-4",
            h1 { class: "text-2xl font-bold", "Create Objective" }

            if let Some(msg) = error() {
                div { class: "alert alert-error text-sm", "{msg}" }
            }

            div { class: "form-control",
                label { class: "label", span { class: "label-text", "Name" } }
                input {
                    class: "input input-bordered w-full",
                    value: "{name}",
                    oninput: move |e| name.set(e.value()),
                }
            }
            div { class: "form-control",
                label { class: "label", span { class: "label-text", "Description" } }
                textarea {
                    class: "textarea textarea-bordered w-full",
                    value: "{description}",
                    oninput: move |e| description.set(e.value()),
                }
            }
            div { class: "form-control",
                label { class: "label",
                    span { class: "label-text", "States (comma-separated, optional)" }
                }
                input {
                    class: "input input-bordered w-full",
                    placeholder: "Queued, In Flight, Verifying, Done",
                    value: "{states_csv}",
                    oninput: move |e| states_csv.set(e.value()),
                }
                label { class: "label",
                    span { class: "label-text-alt text-base-content/50",
                        "Leave empty to default to Queued → In Flight → Done"
                    }
                }
            }

            div { class: "flex gap-2 justify-end",
                Link { to: Route::Dashboard {}, class: "btn btn-ghost btn-sm", "Cancel" }
                button {
                    class: "btn btn-primary btn-sm",
                    disabled: submitting(),
                    onclick: submit,
                    "Create"
                }
            }
        }
    }
}
