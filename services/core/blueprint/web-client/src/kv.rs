//! Key/Value display rules, kept pure so they can be tested: which values are masked, how a kind
//! is named, whether a value is JSON.

pub const VALUE_TYPE_URL: &str = "type.googleapis.com/core.registry.key_value.v1.Value";

/// `Value` / `NavigationConfig` for `type.googleapis.com/core.registry.key_value.v1.Value`: the
/// last dotted segment, or the whole thing when it is not a type URL.
pub fn short_kind_name(type_url: &str) -> &str {
    type_url
        .strip_prefix("type.googleapis.com/")
        .and_then(|s| s.rsplit('.').next())
        .unwrap_or(type_url)
}

/// Whether an entry's value is hidden until revealed: a PEM block (`-----BEGIN …`), or a key that
/// names a credential (`*secret*`, `*token*`, `*password*`, `*_ca`, `*_key` is deliberately not
/// included — plenty of ordinary keys end in `_key`).
pub fn is_secret(key: &str, value: &str) -> bool {
    let k = key.to_ascii_lowercase();
    value.trim_start().starts_with("-----BEGIN")
        || k.contains("secret")
        || k.contains("token")
        || k.contains("password")
        || k.ends_with("_ca")
}

/// What a masked value is, for the line next to the mask: `PEM · 3.1 KB`.
pub fn secret_summary(value: &str) -> String {
    let what = if value.trim_start().starts_with("-----BEGIN") { "PEM" } else { "Secret" };
    format!("{what} · {}", draft_ui::util::format_bytes(value.len() as u64))
}

/// A value as indented JSON when it parses as JSON. `None` means it is plain text.
pub fn pretty_json(data: &str) -> Option<String> {
    let parsed: serde_json::Value = serde_json::from_str(data).ok()?;
    // A bare scalar ("42", "true", "\"x\"") parses as JSON but is not worth a JSON badge.
    if !(parsed.is_object() || parsed.is_array()) {
        return None;
    }
    serde_json::to_string_pretty(&parsed).ok()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn kinds_read_short() {
        assert_eq!(short_kind_name(VALUE_TYPE_URL), "Value");
        assert_eq!(short_kind_name("type.googleapis.com/core.registry.key_value.v1.NavigationConfig"), "NavigationConfig");
        assert_eq!(short_kind_name("not a url"), "not a url");
    }

    #[test]
    fn credentials_are_masked_by_key_or_by_shape() {
        assert!(is_secret("fuse_local_ca", "anything"));
        assert!(is_secret("Slack_Token", "x"));
        assert!(is_secret("db/password", "x"));
        assert!(is_secret("webhook_secret", "x"));
        assert!(is_secret("any", "-----BEGIN CERTIFICATE-----\nabc"));
        assert!(is_secret("any", "  \n-----BEGIN PRIVATE KEY-----"));
    }

    #[test]
    fn ordinary_keys_are_not_masked() {
        assert!(!is_secret("auth_service_address", "http://localhost:9095"));
        assert!(!is_secret("cluster/layout", "{}"));
        assert!(!is_secret("otel_endpoint", "http://localhost:2222"));
        assert!(!is_secret("api_key_name", "x"));
        assert!(!is_secret("nav/sections", "[]"));
        // Ends in `ca` but not `_ca`: a word like "local_cache" is fine.
        assert!(!is_secret("cache", "x"));
    }

    #[test]
    fn a_secret_summary_says_what_it_is() {
        let pem = "-----BEGIN CERTIFICATE-----\nMIIB";
        assert_eq!(secret_summary(pem), format!("PEM · {} B", pem.len()));
        assert_eq!(secret_summary("hunter2"), "Secret · 7 B");
    }

    #[test]
    fn only_objects_and_arrays_count_as_json() {
        assert_eq!(pretty_json(r#"{"a":1}"#).as_deref(), Some("{\n  \"a\": 1\n}"));
        assert!(pretty_json("[1,2]").is_some());
        assert!(pretty_json("42").is_none());
        assert!(pretty_json("true").is_none());
        assert!(pretty_json("http://localhost:2221").is_none());
        assert!(pretty_json("").is_none());
    }
}
