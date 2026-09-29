#![allow(unused_imports)]
use super::*;

// ── Layout persistence (localStorage + Blueprint KV) ────────────────────────
// Finishes what docs/architecture/cluster-graph-persistence.md started but
// never wired up against this (later, pure-Dioxus) renderer — see
// docs/architecture/cluster-live-topology-implementation-plan.md Phase 5.
// Reuses the existing generic `Value{data: string}` KV shape (JSON-encoded)
// rather than adding a new proto message.

#[derive(Clone, Debug, Default, serde::Serialize, serde::Deserialize)]
pub(super) struct LayoutBlob {
    pub(super) positions: HashMap<String, (f64, f64)>,
    pub(super) pan_x: f64,
    pub(super) pan_y: f64,
    pub(super) zoom: f64,
}

pub(super) fn local_storage() -> Option<web_sys::Storage> {
    web_sys::window()?.local_storage().ok()?
}

pub(super) fn load_layout_from_local_storage() -> Option<LayoutBlob> {
    let raw = local_storage()?.get_item(LAYOUT_LOCAL_STORAGE_KEY).ok()??;
    serde_json::from_str(&raw).ok()
}

pub(super) fn save_layout_to_local_storage(layout: &LayoutBlob) {
    if let (Some(storage), Ok(raw)) = (local_storage(), serde_json::to_string(layout)) {
        let _ = storage.set_item(LAYOUT_LOCAL_STORAGE_KEY, &raw);
    }
}

pub(super) async fn load_layout_from_kv() -> Option<LayoutBlob> {
    let mut client = KeyValueServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
    let resp = client
        .get(GetRequest {
            key: LAYOUT_KV_KEY.to_string(),
            value: Some(Any {
                type_url: LAYOUT_VALUE_TYPE_URL.to_string(),
                value: vec![],
            }),
        })
        .await
        .ok()?;
    let any = resp.into_inner().value?;
    let val = KvValue::decode(any.value.as_slice()).ok()?;
    serde_json::from_str(&val.data).ok()
}

pub(super) async fn save_layout_to_kv(layout: &LayoutBlob) {
    let Ok(raw) = serde_json::to_string(layout) else {
        return;
    };
    let mut client = KeyValueServiceClient::new(WasmClient::new(crate::API_DOMAIN.clone()));
    let any = Any {
        type_url: LAYOUT_VALUE_TYPE_URL.to_string(),
        value: KvValue { data: raw }.encode_to_vec(),
    };
    let _ = client
        .set(SetRequest {
            key: LAYOUT_KV_KEY.to_string(),
            value: Some(any),
        })
        .await;
}

/// localStorage is written first (fast, always available) then Blueprint KV
/// (source of truth across browsers/sessions) — if the KV call fails
/// (Blueprint down, network error), the localStorage write still landed.
pub(super) fn persist_layout(layout: LayoutBlob) {
    save_layout_to_local_storage(&layout);
    spawn(async move {
        save_layout_to_kv(&layout).await;
    });
}
