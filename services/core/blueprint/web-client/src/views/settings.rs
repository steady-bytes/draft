//! Settings: the rail's navigation, stored in KV as `ui/navigation`. Edits work on a local copy and
//! take effect in the rail as soon as they are saved.

use dioxus::prelude::*;
use draft_api::hook::core_registry_key_value_v1::{key_value_service_client::KeyValueServiceClient, SetRequest};
use draft_api::proto::core_registry_key_value_v1::{NavigationConfig, NavigationItem, NavigationSection};
use draft_ui::layout::PageHead;
use draft_ui::shell::use_page_chrome;
use draft_ui::ui::{use_toast, Btn, BtnSize, BtnVariant, Select, TextInput, Toast};
use prost::Message as _;
use prost_types::Any;
use tonic_web_wasm_client::Client as WasmClient;

use crate::raft::use_raft;

#[component]
pub fn Settings() -> Element {
    let raft = use_raft();
    let toast = use_toast();
    // Provided by the layout: reading it reflects the live rail, writing it re-renders the rail
    // at once without a reload.
    let nav_config_ctx: Signal<Option<NavigationConfig>> = use_context();

    // The working copy starts as the compile-time default and is replaced once, when the layout's
    // KV fetch resolves.
    let mut local: Signal<NavigationConfig> = use_signal(crate::default_nav_config);
    let mut initialized = use_signal(|| false);
    let mut saving = use_signal(|| false);

    use_page_chrome(move || raft.chrome(&["Blueprint", "System", "Settings"]));

    use_effect(move || {
        if !initialized() {
            if let Some(cfg) = nav_config_ctx() {
                local.set(cfg);
                initialized.set(true);
            }
        }
    });

    // Edits go through one helper so every control reads the same way.
    let edit = use_callback(move |change: Box<dyn FnOnce(&mut NavigationConfig)>| {
        let mut cfg = local.peek().clone();
        change(&mut cfg);
        local.set(cfg);
    });

    let save = use_callback(move |_: ()| {
        let config = local.peek().clone();
        let mut ctx = nav_config_ctx;
        saving.set(true);
        spawn(async move {
            let any = Any { type_url: crate::NAV_CONFIG_TYPE_URL.to_string(), value: config.clone().encode_to_vec() };
            let result = KeyValueServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()))
                .set(SetRequest { key: crate::NAV_CONFIG_KV_KEY.to_string(), value: Some(any) })
                .await;
            saving.set(false);
            match result {
                Ok(_) => {
                    ctx.set(Some(config));
                    toast.show("Navigation saved", 3000);
                }
                Err(e) => toast.show(format!("Could not save: {}", e.message()), 6000),
            }
        });
    });

    let cfg = local();
    let sections = cfg.sections.len();
    let route_options: Vec<(String, String)> = crate::KNOWN_ROUTES.iter().map(|(p, n)| (p.to_string(), n.to_string())).collect();

    rsx! {
        PageHead {
            title: "Settings".to_string(),
            eyebrow: "System".to_string(),
            description: "Configure the sections and items shown in the navigation rail. Changes take effect as soon as they are saved.".to_string(),
            actions: rsx! {
                Btn {
                    onclick: move |_| local.set(crate::default_nav_config()),
                    "Restore defaults"
                }
                Btn { variant: BtnVariant::Primary, disabled: saving(), onclick: move |_| save.call(()), "Save" }
            },
        }

        div { class: "settings-stack",
            for (si , section) in cfg.sections.iter().enumerate() {
                {
                    let items = section.items.len();
                    rsx! {
                        div { key: "{si}", class: "d-panel",
                            div { class: "d-panel-head",
                                span { class: "d-label", "Section {si + 1}" }
                                span { class: "d-spacer" }
                                Btn { variant: BtnVariant::Ghost, size: BtnSize::Sm, icon: true, disabled: si == 0, aria_label: "Move section up".to_string(),
                                    onclick: move |_| edit.call(Box::new(move |c| c.sections.swap(si, si - 1))), "↑" }
                                Btn { variant: BtnVariant::Ghost, size: BtnSize::Sm, icon: true, disabled: si + 1 == sections, aria_label: "Move section down".to_string(),
                                    onclick: move |_| edit.call(Box::new(move |c| c.sections.swap(si, si + 1))), "↓" }
                                Btn { variant: BtnVariant::Danger, size: BtnSize::Sm, aria_label: "Remove section".to_string(),
                                    onclick: move |_| edit.call(Box::new(move |c| { c.sections.remove(si); })), "Remove" }
                            }
                            div { class: "d-panel-body settings-section",
                                TextInput {
                                    value: section.label.clone(),
                                    placeholder: "Section name".to_string(),
                                    aria_label: "Section name".to_string(),
                                    oninput: move |v: String| edit.call(Box::new(move |c| c.sections[si].label = v)),
                                }
                                for (ii , item) in section.items.iter().enumerate() {
                                    div { key: "{ii}", class: "settings-item",
                                        TextInput {
                                            value: item.label.clone(),
                                            placeholder: "Label".to_string(),
                                            aria_label: "Item label".to_string(),
                                            oninput: move |v: String| edit.call(Box::new(move |c| c.sections[si].items[ii].label = v)),
                                        }
                                        Select {
                                            value: item.path.clone(),
                                            options: route_options.clone(),
                                            aria_label: "Item page".to_string(),
                                            on_change: move |v: String| edit.call(Box::new(move |c| c.sections[si].items[ii].path = v)),
                                        }
                                        Btn { variant: BtnVariant::Ghost, size: BtnSize::Sm, icon: true, disabled: ii == 0, aria_label: "Move item up".to_string(),
                                            onclick: move |_| edit.call(Box::new(move |c| c.sections[si].items.swap(ii, ii - 1))), "↑" }
                                        Btn { variant: BtnVariant::Ghost, size: BtnSize::Sm, icon: true, disabled: ii + 1 == items, aria_label: "Move item down".to_string(),
                                            onclick: move |_| edit.call(Box::new(move |c| c.sections[si].items.swap(ii, ii + 1))), "↓" }
                                        Btn { variant: BtnVariant::Danger, size: BtnSize::Sm, aria_label: "Remove item".to_string(),
                                            onclick: move |_| edit.call(Box::new(move |c| { c.sections[si].items.remove(ii); })), "Remove" }
                                    }
                                }
                                div {
                                    Btn { variant: BtnVariant::Ghost, size: BtnSize::Sm,
                                        onclick: move |_| edit.call(Box::new(move |c| c.sections[si].items.push(NavigationItem { label: "New item".to_string(), path: "/".to_string() }))),
                                        "+ Add item"
                                    }
                                }
                            }
                        }
                    }
                }
            }
            div {
                Btn {
                    onclick: move |_| edit.call(Box::new(|c| c.sections.push(NavigationSection { label: "New section".to_string(), items: vec![] }))),
                    "+ Add section"
                }
            }
        }

        Toast { state: toast }
    }
}
