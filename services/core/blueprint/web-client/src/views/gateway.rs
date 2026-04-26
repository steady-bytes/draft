use dioxus::prelude::*;
use draft_api::proto::core_control_plane_networking_v1::{
    networking_service_client::NetworkingServiceClient,
    AddRouteRequest, DeleteRouteRequest, Endpoint, ListRoutesRequest, Route, RouteMatch,
};
use tonic_web_wasm_client::Client;

const FUSE_URL: &str = "http://127.0.0.1:18000";

#[component]
pub fn Gateway() -> Element {
    let mut list_result = use_resource(|| async {
        let mut client = NetworkingServiceClient::new(Client::new(FUSE_URL.to_string()));
        client.list_routes(ListRoutesRequest {}).await.map(|r| r.into_inner())
    });

    let mut show_modal = use_signal(|| false);
    let mut editing_name: Signal<Option<String>> = use_signal(|| None);

    let mut form_name = use_signal(String::new);
    let mut form_prefix = use_signal(String::new);
    let mut form_host = use_signal(String::new);
    let mut form_ep_host = use_signal(String::new);
    let mut form_ep_port = use_signal(|| String::from("0"));
    let mut form_http2 = use_signal(|| false);
    let mut status: Signal<Option<String>> = use_signal(|| None);

    let submit = move |_| {
        let name = form_name();
        let prefix = form_prefix();
        let host = form_host();
        let ep_host = form_ep_host();
        let port: u32 = form_ep_port().parse().unwrap_or(0);
        let http2 = form_http2();
        let original = editing_name();

        spawn(async move {
            let mut client = NetworkingServiceClient::new(Client::new(FUSE_URL.to_string()));

            if let Some(old_name) = original {
                if let Err(e) = client.delete_route(DeleteRouteRequest { name: old_name }).await {
                    status.set(Some(format!("Error removing old route: {e}")));
                    return;
                }
            }

            let route = Route {
                name,
                r#match: Some(RouteMatch {
                    prefix,
                    host,
                    headers: None,
                    grpc_match_options: None,
                    dynamic_metadata: None,
                }),
                endpoint: Some(Endpoint { host: ep_host, port }),
                enable_http2: http2,
            };

            match client.add_route(AddRouteRequest { route: Some(route) }).await {
                Ok(_) => {
                    show_modal.set(false);
                    status.set(Some("Route saved.".to_string()));
                    list_result.restart();
                }
                Err(e) => status.set(Some(format!("Error: {e}"))),
            }
        });
    };

    rsx! {
        div { class: "p-4",
            div { class: "flex items-center justify-between mb-4",
                h1 { class: "text-2xl font-bold", "Gateway" }
                button {
                    class: "btn btn-primary btn-sm",
                    onclick: move |_| {
                        form_name.set(String::new());
                        form_prefix.set(String::new());
                        form_host.set(String::new());
                        form_ep_host.set(String::new());
                        form_ep_port.set("0".to_string());
                        form_http2.set(false);
                        editing_name.set(None);
                        show_modal.set(true);
                    },
                    "+ Add Route"
                }
            }

            if let Some(msg) = status() {
                div { class: "alert alert-info mb-4",
                    span { "{msg}" }
                }
            }

            div { class: "overflow-x-auto",
                table { class: "table table-xs",
                    thead {
                        tr {
                            th { "Name" }
                            th { "Match Prefix" }
                            th { "Match Host" }
                            th { "Endpoint Host" }
                            th { "Port" }
                            th { "HTTP2" }
                            th { "Actions" }
                        }
                    }
                    tbody {
                        match &*list_result.read() {
                            Some(Ok(response)) => {
                                rsx! {
                                    for route in response.routes.clone() {
                                        {
                                            let key = route.name.clone();
                                            let col_name = route.name.clone();
                                            let col_prefix = route.r#match.as_ref().map(|m| m.prefix.clone()).unwrap_or_default();
                                            let col_host = route.r#match.as_ref().map(|m| m.host.clone()).unwrap_or_default();
                                            let col_ep_host = route.endpoint.as_ref().map(|e| e.host.clone()).unwrap_or_default();
                                            let col_port = route.endpoint.as_ref().map(|e| e.port).unwrap_or(0);
                                            let col_http2 = route.enable_http2;

                                            let edit_name = route.name.clone();
                                            let edit_prefix = col_prefix.clone();
                                            let edit_host = col_host.clone();
                                            let edit_ep_host = col_ep_host.clone();
                                            let edit_port = col_port;
                                            let edit_http2 = col_http2;

                                            let del_name = route.name.clone();

                                            rsx! {
                                                tr { key: "{key}", class: "hover:bg-base-300",
                                                    td { "{col_name}" }
                                                    td { "{col_prefix}" }
                                                    td { "{col_host}" }
                                                    td { "{col_ep_host}" }
                                                    td { "{col_port}" }
                                                    td { { if col_http2 { "Yes" } else { "No" } } }
                                                    td {
                                                        div { class: "flex gap-2",
                                                            button {
                                                                class: "btn btn-xs btn-ghost",
                                                                onclick: move |_| {
                                                                    form_name.set(edit_name.clone());
                                                                    form_prefix.set(edit_prefix.clone());
                                                                    form_host.set(edit_host.clone());
                                                                    form_ep_host.set(edit_ep_host.clone());
                                                                    form_ep_port.set(edit_port.to_string());
                                                                    form_http2.set(edit_http2);
                                                                    editing_name.set(Some(edit_name.clone()));
                                                                    show_modal.set(true);
                                                                },
                                                                "Edit"
                                                            }
                                                            button {
                                                                class: "btn btn-xs btn-ghost btn-error",
                                                                onclick: move |_| {
                                                                    let name = del_name.clone();
                                                                    spawn(async move {
                                                                        let mut client = NetworkingServiceClient::new(
                                                                            Client::new(FUSE_URL.to_string()),
                                                                        );
                                                                        match client.delete_route(DeleteRouteRequest { name }).await {
                                                                            Ok(_) => {
                                                                                status.set(Some("Route deleted.".to_string()));
                                                                                list_result.restart();
                                                                            }
                                                                            Err(e) => status.set(Some(format!("Error: {e}"))),
                                                                        }
                                                                    });
                                                                },
                                                                "Delete"
                                                            }
                                                        }
                                                    }
                                                }
                                            }
                                        }
                                    }
                                }
                            }
                            Some(Err(e)) => rsx! {
                                tr {
                                    td { colspan: "7", class: "text-center text-error", "Error: {e}" }
                                }
                            },
                            None => rsx! {
                                tr {
                                    td { colspan: "7", class: "text-center", "Loading..." }
                                }
                            },
                        }
                    }
                }
            }

            if show_modal() {
                div { class: "modal modal-open",
                    div { class: "modal-box",
                        h3 { class: "font-bold text-lg mb-4",
                            { if editing_name().is_some() { "Edit Route" } else { "Add Route" } }
                        }
                        div { class: "form-control mb-2",
                            label { class: "label", span { class: "label-text", "Name" } }
                            input {
                                class: "input input-bordered input-sm w-full",
                                value: "{form_name}",
                                oninput: move |e| form_name.set(e.value()),
                            }
                        }
                        div { class: "form-control mb-2",
                            label { class: "label", span { class: "label-text", "Match Prefix" } }
                            input {
                                class: "input input-bordered input-sm w-full",
                                value: "{form_prefix}",
                                oninput: move |e| form_prefix.set(e.value()),
                            }
                        }
                        div { class: "form-control mb-2",
                            label { class: "label", span { class: "label-text", "Match Host" } }
                            input {
                                class: "input input-bordered input-sm w-full",
                                value: "{form_host}",
                                oninput: move |e| form_host.set(e.value()),
                            }
                        }
                        div { class: "form-control mb-2",
                            label { class: "label", span { class: "label-text", "Endpoint Host" } }
                            input {
                                class: "input input-bordered input-sm w-full",
                                value: "{form_ep_host}",
                                oninput: move |e| form_ep_host.set(e.value()),
                            }
                        }
                        div { class: "form-control mb-2",
                            label { class: "label", span { class: "label-text", "Endpoint Port" } }
                            input {
                                r#type: "number",
                                class: "input input-bordered input-sm w-full",
                                value: "{form_ep_port}",
                                oninput: move |e| form_ep_port.set(e.value()),
                            }
                        }
                        div { class: "form-control mb-4",
                            label { class: "label cursor-pointer",
                                span { class: "label-text", "Enable HTTP2" }
                                input {
                                    r#type: "checkbox",
                                    class: "toggle",
                                    checked: form_http2(),
                                    onchange: move |e| form_http2.set(e.checked()),
                                }
                            }
                        }
                        div { class: "modal-action",
                            button {
                                class: "btn btn-ghost btn-sm",
                                onclick: move |_| show_modal.set(false),
                                "Cancel"
                            }
                            button {
                                class: "btn btn-primary btn-sm",
                                onclick: submit,
                                "Save"
                            }
                        }
                    }
                }
            }
        }
    }
}
