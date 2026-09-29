//! Create an objective. Leaving states empty lets the server default to
//! `["Queued", "In Flight", "Done"]`.

use dioxus::prelude::*;
use draft_api::proto::tooling_lineman_v1::{lineman_service_client::LinemanServiceClient, CreateObjectiveRequest};
use draft_ui::layout::PageHead;
use draft_ui::shell::use_page_chrome;
use draft_ui::ui::{Alert, Btn, BtnVariant, Field, TextInput, Textarea};
use draft_ui::StatusKind;
use tonic_web_wasm_client::Client as WasmClient;

use crate::rail::use_rail;
use crate::Route;

#[component]
pub fn CreateObjective() -> Element {
    let navigator = use_navigator();
    let rail = use_rail();
    let mut name = use_signal(String::new);
    let mut description = use_signal(String::new);
    let mut states_csv = use_signal(String::new);
    let mut error: Signal<Option<String>> = use_signal(|| None);
    let mut submitting = use_signal(|| false);

    use_page_chrome(move || rail.chrome(&["Lineman", "Objectives", "New objective"]));

    let submit = use_callback(move |_: ()| {
        let name_v = name();
        if name_v.trim().is_empty() {
            error.set(Some("Name is required".to_string()));
            return;
        }
        let description_v = description();
        let states: Vec<String> =
            states_csv().split(',').map(|s| s.trim().to_string()).filter(|s| !s.is_empty()).collect();
        submitting.set(true);
        spawn(async move {
            let mut client = LinemanServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
            let created = client
                .create_objective(CreateObjectiveRequest { name: name_v, description: description_v, states })
                .await;
            match created {
                Ok(resp) => {
                    rail.refresh();
                    navigator.push(Route::ObjectiveDetail { id: resp.into_inner().id });
                }
                Err(err) => {
                    submitting.set(false);
                    error.set(Some(err.message().to_string()));
                }
            }
        });
    });

    rsx! {
        PageHead {
            title: "New objective".to_string(),
            eyebrow: "Lineman".to_string(),
            description: "An objective groups tasks that move through the same states.".to_string(),
        }

        div { class: "d-panel form-panel",
            if let Some(msg) = error() {
                Alert { kind: StatusKind::Err, "{msg}" }
            }
            Field { label: "Name".to_string(),
                TextInput {
                    value: name(),
                    oninput: move |v| name.set(v),
                    onkeydown: move |e: KeyboardEvent| {
                        if e.key() == Key::Enter {
                            submit.call(());
                        }
                    },
                }
            }
            Field { label: "Description".to_string(),
                Textarea { value: description(), oninput: move |v| description.set(v) }
            }
            Field {
                label: "States".to_string(),
                hint: "Comma-separated, optional. Empty defaults to Queued, In Flight, Done".to_string(),
                TextInput {
                    value: states_csv(),
                    oninput: move |v| states_csv.set(v),
                    placeholder: "Queued, In Flight, Verifying, Done".to_string(),
                }
            }
            div { class: "form-actions",
                Btn { variant: BtnVariant::Ghost, to: "/".to_string(), "Cancel" }
                Btn {
                    variant: BtnVariant::Primary,
                    disabled: submitting(),
                    onclick: move |_| submit.call(()),
                    "Create objective"
                }
            }
        }
    }
}
