//! Key / Value: the cluster's Raft-replicated configuration. Values are protobuf `Any` messages;
//! long ones are previewed in the table and opened in full in the drawer. `/kv/:key` is this same
//! page with that key's drawer open, so a key is deep-linkable.

use std::collections::{HashMap, HashSet};

use dioxus::prelude::*;
use draft_api::hook::core_registry_key_value_v1::{
    key_value_service_client::KeyValueServiceClient, DecodeValuesRequest, DeleteRequest, ListKindsRequest,
    ListRequest, SetRequest, Value,
};
use draft_ui::data::{CodeBlock, CodeLang, Kv, KvItem};
use draft_ui::layout::{Drawer, DrawerBlock, PageHead, Split, Toolbar};
use draft_ui::shell::{use_page_chrome, BarItem};
use draft_ui::ui::{use_toast, Alert, Btn, BtnSize, BtnVariant, Empty, Field, Loading, Modal, Select, Tag, TextInput, Textarea, Toast};
use draft_ui::util::{format_bytes, truncate_preview};
use draft_ui::{StatusKind, Tone};
use prost::Message as _;
use prost_types::Any;
use tonic_web_wasm_client::Client as WasmClient;

use crate::raft::use_raft;
use crate::kv::{is_secret, pretty_json, secret_summary, short_kind_name, VALUE_TYPE_URL};
use crate::Route as AppRoute;

/// How many characters of a value the table previews before the drawer takes over.
const PREVIEW_CHARS: usize = 80;

fn client() -> KeyValueServiceClient<WasmClient> {
    KeyValueServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()))
}

/// `/` — the list.
#[component]
pub fn KeyValueView() -> Element {
    rsx! { KvPage { selected: None } }
}

/// `/kv/:..key` — the list with one key's drawer open. The key is a catch-all (rejoined with `/`)
/// because real keys contain slashes (`cluster/layout`, `ui/navigation`).
#[component]
pub fn KeyValueDetail(kv_key_parts: Vec<String>) -> Element {
    rsx! { KvPage { selected: Some(kv_key_parts.join("/")) } }
}

/// One entry, ready to show.
#[derive(Clone, Debug, PartialEq)]
struct Row {
    /// The key as the list shows it (the kind's prefix stripped).
    key: String,
    kind: String,
    /// The decoded value: text for `Value`, JSON for a registered kind, `None` when no descriptor
    /// is registered for the kind (only its byte count is known).
    data: Option<String>,
    raw_len: usize,
    secret: bool,
}

impl Row {
    fn is_value_kind(&self) -> bool {
        self.kind == VALUE_TYPE_URL
    }

    /// The one-line preview for the table.
    fn preview(&self) -> String {
        match &self.data {
            Some(d) => truncate_preview(d, PREVIEW_CHARS),
            None => format!("{} — {} bytes", short_kind_name(&self.kind), self.raw_len),
        }
    }

    fn size(&self) -> String {
        format_bytes(self.data.as_ref().map(|d| d.len()).unwrap_or(self.raw_len) as u64)
    }
}

#[component]
fn KvPage(selected: Option<String>) -> Element {
    let navigator = use_navigator();
    let toast = use_toast();

    let mut kind = use_signal(|| VALUE_TYPE_URL.to_string());
    let mut filter = use_signal(String::new);
    let mut revealed: Signal<HashSet<String>> = use_signal(HashSet::new);

    // The list → detail hand-off: a row click leaves its kind here so this page (remounted by the
    // route change) keeps showing the same kind. The read-and-clear happens in an effect, not the
    // body: reading in the body subscribes the render to the signal, and clearing it would then
    // re-run this same render forever.
    use_effect(move || {
        let Some(pending) = crate::PENDING_KV_KIND.read().clone() else {
            return;
        };
        *crate::PENDING_KV_KIND.write() = None;
        kind.set(pending);
    });

    let kinds_result = use_resource(|| async { client().list_kinds(ListKindsRequest {}).await.map(|r| r.into_inner().kinds) });
    let mut list_result = use_resource(move || {
        let kind = kind();
        async move {
            client().list(ListRequest { value: Some(Any { type_url: kind, value: vec![] }) }).await.map(|r| r.into_inner())
        }
    });

    // Registered kinds decode through Blueprint's type registry. `Value` is decoded directly below:
    // decoding some other kind's bytes as `Value` can "succeed" with garbage, because protobuf
    // strings and embedded messages share a wire type. Reading `list_result` here subscribes this
    // resource to it, so a kind change or a restart after a write re-runs it too.
    let decoded_result = use_resource(move || {
        let kind = kind();
        async move {
            if kind == VALUE_TYPE_URL {
                return HashMap::new();
            }
            let values: HashMap<String, Vec<u8>> = match &*list_result.read() {
                Some(Ok(resp)) => resp.values.iter().map(|(k, any)| (k.clone(), any.value.clone())).collect(),
                _ => return HashMap::new(),
            };
            if values.is_empty() {
                return HashMap::new();
            }
            client().decode_values(DecodeValuesRequest { type_url: kind, values }).await.map(|r| r.into_inner().json).unwrap_or_default()
        }
    });

    // Add / edit dialog, delete confirmation.
    let mut form_open = use_signal(|| false);
    let mut form_editing = use_signal(|| false);
    let mut form_key = use_signal(String::new);
    let mut form_value = use_signal(String::new);
    let mut form_error: Signal<Option<String>> = use_signal(|| None);
    let mut confirm_delete: Signal<Option<Row>> = use_signal(|| None);

    let close_drawer = use_callback(move |_: ()| {
        *crate::PENDING_KV_KIND.write() = Some(kind.peek().clone());
        navigator.push(AppRoute::KeyValueView {});
    });

    let save = use_callback(move |_: ()| {
        let key = form_key();
        if key.trim().is_empty() {
            form_error.set(Some("Key is required".to_string()));
            return;
        }
        let value = form_value();
        let editing = form_editing();
        spawn(async move {
            let any = Any { type_url: VALUE_TYPE_URL.to_string(), value: Value { data: value }.encode_to_vec() };
            match client().set(SetRequest { key, value: Some(any) }).await {
                Ok(_) => {
                    form_open.set(false);
                    form_error.set(None);
                    list_result.restart();
                    toast.show(if editing { "Entry updated" } else { "Entry saved" }, 3000);
                }
                Err(e) => form_error.set(Some(e.message().to_string())),
            }
        });
    });

    // The open drawer's key. A route change remounts this page, so it is fixed for this instance.
    let open_key = selected.clone();
    let delete = use_callback(move |row: Row| {
        let open_key = open_key.clone();
        spawn(async move {
            let key = row.key.clone();
            let request = DeleteRequest { key: key.clone(), value: Some(Any { type_url: row.kind.clone(), value: vec![] }) };
            match client().delete(request).await {
                Ok(_) => {
                    confirm_delete.set(None);
                    list_result.restart();
                    toast.show(format!("Deleted {key}"), 3000);
                    if open_key.as_ref() == Some(&key) {
                        close_drawer.call(());
                    }
                }
                Err(e) => {
                    confirm_delete.set(None);
                    toast.show(format!("Could not delete: {}", e.message()), 6000);
                }
            }
        });
    });

    // Rows -------------------------------------------------------------------------------------
    let current_kind = kind();
    let rows: Vec<Row> = match &*list_result.read() {
        Some(Ok(resp)) => {
            let decoded = decoded_result.read().clone().unwrap_or_default();
            let prefix = format!("{current_kind}-");
            let mut rows: Vec<Row> = resp
                .values
                .iter()
                .map(|(stored_key, any)| {
                    let data = if any.type_url == VALUE_TYPE_URL {
                        Some(Value::decode(any.value.as_slice()).map(|v| v.data).unwrap_or_else(|_| "(unreadable)".to_string()))
                    } else {
                        decoded.get(stored_key).cloned()
                    };
                    let key = stored_key.strip_prefix(prefix.as_str()).unwrap_or(stored_key).to_string();
                    let secret = is_secret(&key, data.as_deref().unwrap_or(""));
                    Row { key, kind: any.type_url.clone(), data, raw_len: any.value.len(), secret }
                })
                .collect();
            rows.sort_by(|a, b| a.key.cmp(&b.key));
            rows
        }
        _ => Vec::new(),
    };
    let loading = list_result.read().is_none();
    let load_error = match &*list_result.read() {
        Some(Err(e)) => Some(e.message().to_string()),
        _ => None,
    };

    let needle = filter().to_lowercase();
    let shown: Vec<Row> = rows.iter().filter(|r| needle.is_empty() || r.key.to_lowercase().contains(&needle)).cloned().collect();
    let masked = rows.iter().filter(|r| r.secret).count();

    let kind_options: Vec<(String, String)> = match &*kinds_result.read() {
        Some(Ok(kinds)) if !kinds.is_empty() => {
            kinds.iter().map(|k| (k.type_url.clone(), format!("{} · {}", short_kind_name(&k.type_url), k.count))).collect()
        }
        _ => vec![(VALUE_TYPE_URL.to_string(), "Value".to_string())],
    };
    let summary = if masked > 0 {
        format!("{} entries · {} secret{} masked", rows.len(), masked, if masked == 1 { "" } else { "s" })
    } else {
        format!("{} entries", rows.len())
    };

    let raft = use_raft();
    use_page_chrome(move || {
        let entries = match &*list_result.read() {
            Some(Ok(resp)) => resp.values.len(),
            _ => 0,
        };
        let mut chrome = raft.chrome(&["Blueprint", "Control plane", "Key · Value"]);
        chrome.left = vec![BarItem::kv("Kind", short_kind_name(&kind()).to_string()), BarItem::kv("Entries", entries.to_string())];
        chrome
    });

    // Drawer -----------------------------------------------------------------------------------
    let drawer = selected.as_ref().map(|key| match rows.iter().find(|r| &r.key == key) {
        Some(row) => {
            let row_edit = row.clone();
            let row_delete = row.clone();
            let row_copy = row.clone();
            let is_revealed = revealed().contains(&row.key);
            let key_reveal = row.key.clone();
            rsx! {
                EntryDrawer {
                    key: "{row.key}",
                    row: row.clone(),
                    revealed: is_revealed,
                    on_close: move |_| close_drawer.call(()),
                    on_toggle_reveal: move |_| {
                        let mut set = revealed.write();
                        if !set.remove(&key_reveal) {
                            set.insert(key_reveal.clone());
                        }
                    },
                    on_edit: move |_| {
                        form_editing.set(true);
                        form_key.set(row_edit.key.clone());
                        form_value.set(row_edit.data.clone().unwrap_or_default());
                        form_error.set(None);
                        form_open.set(true);
                    },
                    on_delete: move |_| confirm_delete.set(Some(row_delete.clone())),
                    on_copy: move |_| {
                        let text = row_copy.data.clone().unwrap_or_default();
                        let quoted = serde_json::to_string(&text).unwrap_or_else(|_| "\"\"".to_string());
                        document::eval(&format!("navigator.clipboard.writeText({quoted})"));
                        toast.show("Copied to clipboard", 2500);
                    },
                }
            }
        }
        None => rsx! {
            Drawer { label: "Entry detail".to_string(), on_close: move |_| close_drawer.call(()), title: key.clone(),
                if loading {
                    Loading {}
                } else {
                    Alert { kind: StatusKind::Warn, "No entry found for this key. It may have just been deleted, or it belongs to a different kind." }
                }
            }
        },
    });

    rsx! {
        Split { flush: true, drawer,
            PageHead {
                title: "Key / Value".to_string(),
                eyebrow: "Control plane".to_string(),
                description: "Raft-replicated configuration. Values are protobuf Any messages; long values are previewed here and opened in full on the right.".to_string(),
                actions: rsx! {
                    Btn {
                        variant: BtnVariant::Primary,
                        onclick: move |_| {
                            form_editing.set(false);
                            form_key.set(String::new());
                            form_value.set(String::new());
                            form_error.set(None);
                            form_open.set(true);
                        },
                        "+ Add entry"
                    }
                },
            }

            Toolbar {
                Select {
                    value: current_kind.clone(),
                    options: kind_options,
                    aria_label: "Kind".to_string(),
                    on_change: move |k: String| {
                        kind.set(k);
                        filter.set(String::new());
                    },
                }
                TextInput { value: filter(), oninput: move |v| filter.set(v), placeholder: "Filter keys…".to_string(), aria_label: "Filter keys".to_string() }
                span { class: "d-spacer" }
                span { class: "d-label", "{summary}" }
            }

            if let Some(err) = load_error {
                Alert { kind: StatusKind::Err, "Could not load entries: {err}" }
            } else if loading && rows.is_empty() {
                Loading {}
            } else if shown.is_empty() {
                Empty { title: "No entries".to_string(),
                    if rows.is_empty() { "There are no entries of this kind." } else { "No key matches the filter." }
                }
            } else {
                div { class: "d-panel d-table-wrap",
                    table { class: "d-table kv-table",
                        thead {
                            tr {
                                th { "Key" }
                                th { "Value" }
                                th { "Type" }
                                th {}
                            }
                        }
                        tbody {
                            for row in shown {
                                {
                                    let is_selected = selected.as_deref() == Some(row.key.as_str());
                                    let is_revealed = revealed().contains(&row.key);
                                    let row_for_click = row.clone();
                                    let row_for_key = row.clone();
                                    let row_for_delete = row.clone();
                                    let kind_for_click = current_kind.clone();
                                    let kind_for_key = current_kind.clone();
                                    let go = move |r: &Row, k: &str| {
                                        *crate::PENDING_KV_KIND.write() = Some(k.to_string());
                                        navigator.push(AppRoute::KeyValueDetail { kv_key_parts: r.key.split('/').map(String::from).collect() });
                                    };
                                    rsx! {
                                        tr {
                                            key: "{row.key}",
                                            aria_selected: "{is_selected}",
                                            tabindex: "0",
                                            onclick: move |_| go(&row_for_click, &kind_for_click),
                                            onkeydown: move |k| {
                                                if k.key() == Key::Enter {
                                                    go(&row_for_key, &kind_for_key);
                                                }
                                            },
                                            td { class: "kv-key", "{row.key}" }
                                            td { class: "kv-val", ValueCell { row: row.clone(), revealed: is_revealed } }
                                            td { class: "kv-type",
                                                Tag { tone: Tone::Quiet, title: row.kind.clone(), "{short_kind_name(&row.kind)}" }
                                            }
                                            td { class: "is-act",
                                                Btn {
                                                    variant: BtnVariant::Danger,
                                                    size: BtnSize::Sm,
                                                    aria_label: format!("Delete {}", row.key),
                                                    onclick: move |e: MouseEvent| {
                                                        e.stop_propagation();
                                                        confirm_delete.set(Some(row_for_delete.clone()));
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
                p { class: "d-hint", style: "margin-top:12px",
                    "Values starting with -----BEGIN, or keys containing secret, token or password or ending _ca, are masked."
                }
            }
        }

        Modal {
            open: form_open(),
            title: if form_editing() { "Edit entry".to_string() } else { "Add entry".to_string() },
            on_close: move |_| form_open.set(false),
            footer: rsx! {
                Btn { variant: BtnVariant::Ghost, onclick: move |_| form_open.set(false), "Cancel" }
                Btn { variant: BtnVariant::Primary, onclick: move |_| save.call(()), "Save" }
            },
            if let Some(msg) = form_error() {
                Alert { kind: StatusKind::Err, "{msg}" }
            }
            Field { label: "Key".to_string(), hint: if form_editing() { Some("A key cannot be renamed".to_string()) } else { None },
                TextInput { value: form_key(), disabled: form_editing(), oninput: move |v| form_key.set(v), placeholder: "e.g. fuse_address".to_string() }
            }
            Field { label: "Value".to_string(), hint: "Stored as a string; JSON is fine".to_string(),
                Textarea { value: form_value(), oninput: move |v| form_value.set(v) }
            }
        }

        Modal {
            open: confirm_delete().is_some(),
            title: "Delete entry?".to_string(),
            on_close: move |_| confirm_delete.set(None),
            footer: rsx! {
                Btn { variant: BtnVariant::Ghost, onclick: move |_| confirm_delete.set(None), "Cancel" }
                Btn {
                    variant: BtnVariant::Danger,
                    onclick: move |_| {
                        if let Some(row) = confirm_delete.peek().clone() {
                            delete.call(row);
                        }
                    },
                    "Delete"
                }
            },
            if let Some(row) = confirm_delete() {
                p { style: "margin:0", "This removes " b { "{row.key}" } " from every node in the cluster. It cannot be undone." }
            }
        }

        Toast { state: toast }
    }
}

/// The value column: a masked secret with Reveal, or a JSON / text preview.
#[component]
fn ValueCell(row: Row, revealed: bool) -> Element {
    if row.secret && !revealed {
        let summary = secret_summary(row.data.as_deref().unwrap_or(""));
        return rsx! {
            span { class: "d-secret",
                Tag { tone: Tone::Warn, "Secret" }
                span { class: "d-secret-mask", aria_hidden: "true", "••••••••••••" }
                span { "{summary}" }
            }
        };
    }
    let is_json = row.data.as_deref().and_then(pretty_json).is_some();
    let preview = row.preview();
    rsx! {
        span { class: "d-row", style: "gap:8px",
            if is_json {
                Tag { tone: Tone::Ca, "JSON" }
            }
            span { class: "d-trunc", "{preview}" }
        }
    }
}

/// The drawer: type, size and the value in full.
#[component]
fn EntryDrawer(
    row: Row,
    revealed: bool,
    on_close: EventHandler<()>,
    on_toggle_reveal: EventHandler<()>,
    on_edit: EventHandler<()>,
    on_delete: EventHandler<()>,
    on_copy: EventHandler<()>,
) -> Element {
    let masked = row.secret && !revealed;
    let json = row.data.as_deref().and_then(pretty_json);
    let (text, lang) = match (&json, &row.data) {
        (Some(j), _) => (j.clone(), CodeLang::Json),
        (None, Some(d)) => (d.clone(), CodeLang::Plain),
        (None, None) => (String::new(), CodeLang::Plain),
    };
    let size = row.size();
    let short = short_kind_name(&row.kind).to_string();
    let no_preview = row.data.is_none();
    let editable = row.is_value_kind();

    rsx! {
        Drawer { label: "Entry detail".to_string(), on_close: move |_| on_close.call(()), title: row.key.clone(),
            Kv {
                KvItem { label: "Type".to_string(), "{short}" }
                KvItem { label: "Type URL".to_string(), "{row.kind}" }
                KvItem { label: "Size".to_string(), "{size}" }
            }
            DrawerBlock { title: "Value".to_string(),
                if no_preview {
                    Alert { kind: StatusKind::Warn, "No preview for this kind yet: {row.raw_len} raw bytes are stored. Register a descriptor with Blueprint's type registry to see it decoded." }
                } else if masked {
                    div { class: "d-secret",
                        Tag { tone: Tone::Warn, "Secret" }
                        span { class: "d-secret-mask", aria_hidden: "true", "••••••••••••" }
                        span { "{secret_summary(row.data.as_deref().unwrap_or(\"\"))}" }
                    }
                } else {
                    CodeBlock { text, lang, wrap: true }
                }
            }
            div { class: "kv-actions",
                if row.secret {
                    Btn { onclick: move |_| on_toggle_reveal.call(()), if revealed { "Hide" } else { "Reveal" } }
                }
                if editable {
                    Btn { variant: BtnVariant::Primary, onclick: move |_| on_edit.call(()), "Edit" }
                }
                if !no_preview && !masked {
                    Btn { onclick: move |_| on_copy.call(()), "Copy" }
                }
                span { class: "d-spacer" }
                Btn { variant: BtnVariant::Danger, onclick: move |_| on_delete.call(()), "Delete" }
            }
        }
    }
}
