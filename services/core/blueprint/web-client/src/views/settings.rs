use dioxus::prelude::*;
use gloo_timers::future::TimeoutFuture;
use draft_api::proto::core_registry_key_value_v1::{NavigationConfig, NavigationSection, NavigationItem};
use draft_api::hook::core_registry_key_value_v1::{
    key_value_service_client::KeyValueServiceClient,
    SetRequest,
};
use prost::Message as _;
use prost_types::Any;
use tonic_web_wasm_client::Client as WasmClient;

#[component]
pub fn Settings() -> Element {
    // Shared signal provided by dashboard_layout — reading it reflects the live drawer config,
    // writing it causes the drawer to re-render immediately without a page reload.
    let nav_config_ctx: Signal<Option<NavigationConfig>> = use_context();

    // Local working copy. Starts with the compile-time default; synced from context once the
    // KV fetch completes (handled by the use_effect below).
    let mut local_config: Signal<NavigationConfig> = use_signal(crate::default_nav_config);
    let mut initialized = use_signal(|| false);
    let mut status: Signal<Option<String>> = use_signal(|| None);

    // Sync from context exactly once — when the KV fetch in dashboard_layout resolves.
    use_effect(move || {
        if !initialized() {
            if let Some(cfg) = nav_config_ctx() {
                local_config.set(cfg);
                initialized.set(true);
            }
        }
    });

    let save = move |_| {
        let config = local_config();
        let mut nav_config_ctx = nav_config_ctx;
        spawn(async move {
            let mut client = KeyValueServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
            let any = Any {
                type_url: crate::NAV_CONFIG_TYPE_URL.to_string(),
                value: config.clone().encode_to_vec(),
            };
            match client.set(SetRequest {
                key: crate::NAV_CONFIG_KV_KEY.to_string(),
                value: Some(any),
            }).await {
                Ok(_) => {
                    // Update the shared signal so the drawer reflects the new config immediately.
                    nav_config_ctx.set(Some(config));
                    status.set(Some("Navigation saved.".to_string()));
                    spawn(async move {
                        TimeoutFuture::new(3_000).await;
                        status.set(None);
                    });
                }
                Err(e) => status.set(Some(format!("Error: {e}"))),
            }
        });
    };

    let section_count = local_config().sections.len();

    rsx! {
        div { class: "p-4 max-w-2xl",
            h1 { class: "text-2xl font-bold mb-6", "Settings" }

            if let Some(msg) = status() {
                div { class: "toast toast-end toast-bottom z-50",
                    div { class: "alert alert-success",
                        span { "{msg}" }
                    }
                }
            }

            // ── Navigation sub-section ────────────────────────────────────────
            div { class: "card bg-base-200 mb-4",
                div { class: "card-body",
                    h2 { class: "card-title text-lg mb-1", "Navigation" }
                    p { class: "text-sm text-base-content/70 mb-4",
                        "Configure the sections and items shown in the navigation drawer. \
                         Changes take effect immediately after saving."
                    }

                    div { class: "space-y-3",
                        for (si, section) in local_config().sections.into_iter().enumerate() {
                            {
                                let section_label = section.label.clone();
                                let item_count = section.items.len();
                                rsx! {
                                    div { key: "{si}", class: "border border-base-300 rounded-lg p-3",

                                        // Section header row
                                        div { class: "flex items-center gap-2 mb-3",
                                            input {
                                                class: "input input-bordered input-sm flex-1",
                                                value: "{section_label}",
                                                placeholder: "Section name",
                                                oninput: move |e| {
                                                    let mut cfg = local_config();
                                                    cfg.sections[si].label = e.value();
                                                    local_config.set(cfg);
                                                },
                                            }
                                            button {
                                                class: "btn btn-xs btn-ghost",
                                                disabled: si == 0,
                                                title: "Move up",
                                                onclick: move |_| {
                                                    let mut cfg = local_config();
                                                    cfg.sections.swap(si, si - 1);
                                                    local_config.set(cfg);
                                                },
                                                "↑"
                                            }
                                            button {
                                                class: "btn btn-xs btn-ghost",
                                                disabled: si == section_count - 1,
                                                title: "Move down",
                                                onclick: move |_| {
                                                    let mut cfg = local_config();
                                                    cfg.sections.swap(si, si + 1);
                                                    local_config.set(cfg);
                                                },
                                                "↓"
                                            }
                                            button {
                                                class: "btn btn-xs btn-error btn-outline",
                                                title: "Remove section",
                                                onclick: move |_| {
                                                    let mut cfg = local_config();
                                                    cfg.sections.remove(si);
                                                    local_config.set(cfg);
                                                },
                                                "×"
                                            }
                                        }

                                        // Items list
                                        div { class: "ml-3 space-y-1",
                                            for (ii, item) in section.items.into_iter().enumerate() {
                                                {
                                                    let item_label = item.label.clone();
                                                    let item_path = item.path.clone();
                                                    rsx! {
                                                        div { key: "{ii}", class: "flex items-center gap-2",
                                                            input {
                                                                class: "input input-bordered input-xs w-28",
                                                                value: "{item_label}",
                                                                placeholder: "Label",
                                                                oninput: move |e| {
                                                                    let mut cfg = local_config();
                                                                    cfg.sections[si].items[ii].label = e.value();
                                                                    local_config.set(cfg);
                                                                },
                                                            }
                                                            select {
                                                                class: "select select-bordered select-xs flex-1",
                                                                onchange: move |e| {
                                                                    let mut cfg = local_config();
                                                                    cfg.sections[si].items[ii].path = e.value();
                                                                    local_config.set(cfg);
                                                                },
                                                                for &(path, name) in crate::KNOWN_ROUTES {
                                                                    option {
                                                                        value: "{path}",
                                                                        selected: item_path.as_str() == path,
                                                                        "{name}"
                                                                    }
                                                                }
                                                            }
                                                            button {
                                                                class: "btn btn-xs btn-ghost",
                                                                disabled: ii == 0,
                                                                title: "Move up",
                                                                onclick: move |_| {
                                                                    let mut cfg = local_config();
                                                                    cfg.sections[si].items.swap(ii, ii - 1);
                                                                    local_config.set(cfg);
                                                                },
                                                                "↑"
                                                            }
                                                            button {
                                                                class: "btn btn-xs btn-ghost",
                                                                disabled: ii == item_count - 1,
                                                                title: "Move down",
                                                                onclick: move |_| {
                                                                    let mut cfg = local_config();
                                                                    cfg.sections[si].items.swap(ii, ii + 1);
                                                                    local_config.set(cfg);
                                                                },
                                                                "↓"
                                                            }
                                                            button {
                                                                class: "btn btn-xs btn-error btn-outline",
                                                                title: "Remove item",
                                                                onclick: move |_| {
                                                                    let mut cfg = local_config();
                                                                    cfg.sections[si].items.remove(ii);
                                                                    local_config.set(cfg);
                                                                },
                                                                "×"
                                                            }
                                                        }
                                                    }
                                                }
                                            }

                                            button {
                                                class: "btn btn-xs btn-ghost mt-1",
                                                onclick: move |_| {
                                                    let mut cfg = local_config();
                                                    cfg.sections[si].items.push(NavigationItem {
                                                        label: "New Item".to_string(),
                                                        path: "/".to_string(),
                                                    });
                                                    local_config.set(cfg);
                                                },
                                                "+ Add Item"
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    }

                    button {
                        class: "btn btn-sm btn-ghost mt-3",
                        onclick: move |_| {
                            let mut cfg = local_config();
                            cfg.sections.push(NavigationSection {
                                label: "New Section".to_string(),
                                items: vec![],
                            });
                            local_config.set(cfg);
                        },
                        "+ Add Section"
                    }

                    div { class: "card-actions justify-end mt-4",
                        button {
                            class: "btn btn-primary btn-sm",
                            onclick: save,
                            "Save"
                        }
                    }
                }
            }
        }
    }
}
