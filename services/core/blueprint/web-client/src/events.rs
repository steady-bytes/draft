//! CloudEvent helpers for the events views, kept pure: reading attributes, formatting times,
//! counting types, and exposing an event to the CESQL filter.

use std::collections::HashMap;

use chrono::{DateTime, Utc};
use draft_api::proto::core_message_broker_actors_v1::{
    cloud_event::{cloud_event_attribute_value::Attr, CloudEventAttributeValue, Data},
    CloudEvent,
};
use prost_types::Timestamp;

use crate::cesql::Fields;

/// An attribute's value as text, whatever its CloudEvents type.
pub fn attr_text(v: &CloudEventAttributeValue) -> Option<String> {
    Some(match v.attr.as_ref()? {
        Attr::CeBoolean(b) => b.to_string(),
        Attr::CeInteger(i) => i.to_string(),
        Attr::CeString(s) | Attr::CeUri(s) | Attr::CeUriRef(s) => s.clone(),
        Attr::CeBytes(b) => format!("<{} bytes>", b.len()),
        Attr::CeTimestamp(ts) => DateTime::from_timestamp(ts.seconds, ts.nanos as u32)?.format("%Y-%m-%d %H:%M:%S").to_string(),
    })
}

pub fn attr(event: &CloudEvent, name: &str) -> Option<String> {
    event.attributes.get(name).and_then(attr_text)
}

fn attr_timestamp(event: &CloudEvent, name: &str) -> Option<Timestamp> {
    match event.attributes.get(name)?.attr.as_ref()? {
        Attr::CeTimestamp(ts) => Some(ts.clone()),
        Attr::CeString(s) => {
            let dt = DateTime::parse_from_rfc3339(s).ok()?;
            Some(Timestamp { seconds: dt.timestamp(), nanos: dt.timestamp_subsec_nanos() as i32 })
        }
        _ => None,
    }
}

/// When the event happened (`time`).
pub fn event_time(event: &CloudEvent) -> Option<Timestamp> {
    attr_timestamp(event, "time")
}

/// Milliseconds between the event's `time` and Catalyst forwarding it (`forwarded_at`).
pub fn forward_delay_ms(event: &CloudEvent) -> Option<i64> {
    let (t, f) = (event_time(event)?, attr_timestamp(event, "forwarded_at")?);
    Some((f.seconds - t.seconds) * 1000 + i64::from(f.nanos - t.nanos) / 1_000_000)
}

/// A row's time: the clock time (millisecond precision) for today, with the date otherwise.
pub fn row_time(ts: &Option<Timestamp>, now: DateTime<Utc>) -> String {
    match ts.as_ref().and_then(|t| DateTime::from_timestamp(t.seconds, t.nanos as u32)) {
        Some(dt) if dt.date_naive() == now.date_naive() => dt.format("%H:%M:%S%.3f").to_string(),
        Some(dt) => dt.format("%b %-d %H:%M:%S").to_string(),
        None => "—".to_string(),
    }
}

pub fn full_time(ts: &Option<Timestamp>) -> String {
    ts.as_ref()
        .and_then(|t| DateTime::from_timestamp(t.seconds, t.nanos as u32))
        .map(|dt| dt.format("%Y-%m-%d %H:%M:%S%.3f").to_string())
        .unwrap_or_else(|| "—".to_string())
}

pub fn text_data(event: &CloudEvent) -> &str {
    match &event.data {
        Some(Data::TextData(s)) => s.as_str(),
        _ => "",
    }
}

/// A JSON leaf as the tree and the filter fragment show it.
pub fn json_leaf(v: &serde_json::Value) -> String {
    match v {
        serde_json::Value::String(s) => s.clone(),
        serde_json::Value::Null => "null".to_string(),
        other => other.to_string(),
    }
}

/// Walks a dot path into a JSON object (`user.id`). Array indexing is not addressable.
pub fn json_path<'a>(value: &'a serde_json::Value, path: &str) -> Option<&'a serde_json::Value> {
    path.split('.').try_fold(value, |v, key| v.get(key))
}

impl Fields for CloudEvent {
    fn field(&self, name: &str) -> Option<String> {
        match name.to_ascii_lowercase().as_str() {
            "type" => Some(self.r#type.clone()),
            "source" => Some(self.source.clone()),
            "id" => Some(self.id.clone()),
            "specversion" | "spec_version" => Some(self.spec_version.clone()),
            lower => match lower.strip_prefix("body.") {
                // Keep the path's own casing: JSON keys are case-sensitive.
                Some(_) => {
                    let path = &name[5..];
                    let body: serde_json::Value = serde_json::from_str(text_data(self)).ok()?;
                    json_path(&body, path).filter(|v| !v.is_null()).map(json_leaf)
                }
                None => attr(self, name),
            },
        }
    }

    fn haystack(&self) -> String {
        format!("{} {} {} {} {}", self.r#type, self.source, self.id, attr(self, "subject").unwrap_or_default(), text_data(self))
    }
}

/// The number of events of each type, most common first.
pub fn type_facets<'a>(events: impl IntoIterator<Item = &'a CloudEvent>) -> Vec<(String, usize)> {
    let mut counts: HashMap<&str, usize> = HashMap::new();
    for e in events {
        *counts.entry(e.r#type.as_str()).or_default() += 1;
    }
    let mut list: Vec<(String, usize)> = counts.into_iter().map(|(t, n)| (t.to_string(), n)).collect();
    list.sort_by(|a, b| b.1.cmp(&a.1).then_with(|| a.0.cmp(&b.0)));
    list
}

#[cfg(test)]
mod tests {
    use super::*;

    fn text_attr(v: &str) -> CloudEventAttributeValue {
        CloudEventAttributeValue { attr: Some(Attr::CeString(v.to_string())) }
    }

    fn ts_attr(seconds: i64, nanos: i32) -> CloudEventAttributeValue {
        CloudEventAttributeValue { attr: Some(Attr::CeTimestamp(Timestamp { seconds, nanos })) }
    }

    fn event(kind: &str, body: &str) -> CloudEvent {
        let mut e = CloudEvent { id: "e1".into(), source: "/services/shop".into(), r#type: kind.into(), spec_version: "1.0".into(), ..Default::default() };
        e.attributes.insert("subject".into(), text_attr("o-17"));
        e.data = Some(Data::TextData(body.into()));
        e
    }

    #[test]
    fn an_event_exposes_its_context_attributes_and_body_paths_to_a_filter() {
        let e = event("order.created", r#"{"status":"failed","user":{"id":42},"tags":["a"]}"#);
        assert_eq!(e.field("type").as_deref(), Some("order.created"));
        assert_eq!(e.field("SOURCE").as_deref(), Some("/services/shop"));
        assert_eq!(e.field("subject").as_deref(), Some("o-17"));
        assert_eq!(e.field("specversion").as_deref(), Some("1.0"));
        assert_eq!(e.field("body.status").as_deref(), Some("failed"));
        assert_eq!(e.field("body.user.id").as_deref(), Some("42"));
        assert_eq!(e.field("body.missing"), None);
        assert_eq!(e.field("traceparent"), None);
    }

    #[test]
    fn a_body_path_keeps_its_case() {
        let e = event("x", r#"{"businessAttributes":{"callerService":"crud"}}"#);
        assert_eq!(e.field("body.businessAttributes.callerService").as_deref(), Some("crud"));
        assert_eq!(e.field("body.businessattributes.callerservice"), None);
    }

    #[test]
    fn a_body_that_is_not_json_has_no_paths() {
        assert_eq!(event("x", "plain text").field("body.a"), None);
    }

    #[test]
    fn the_cesql_filter_runs_against_a_real_event() {
        use crate::cesql::Filter;
        let e = event("order.created", r#"{"status":"failed"}"#);
        assert!(Filter::parse("type LIKE '%order%' AND body.status = 'failed'").unwrap().matches(&e));
        assert!(!Filter::parse("body.status = 'ok'").unwrap().matches(&e));
        assert!(Filter::parse("EXISTS subject").unwrap().matches(&e));
        assert!(Filter::parse("shop").unwrap().matches(&e), "a plain word searches the whole event");
    }

    #[test]
    fn the_forward_delay_is_in_milliseconds() {
        let mut e = event("x", "");
        e.attributes.insert("time".into(), ts_attr(1_800_000_000, 200_000_000));
        e.attributes.insert("forwarded_at".into(), ts_attr(1_800_000_000, 203_000_000));
        assert_eq!(forward_delay_ms(&e), Some(3));
        e.attributes.insert("forwarded_at".into(), ts_attr(1_800_000_001, 100_000_000));
        assert_eq!(forward_delay_ms(&e), Some(900));
        assert_eq!(forward_delay_ms(&event("y", "")), None);
    }

    #[test]
    fn time_may_arrive_as_an_rfc3339_string() {
        let mut e = event("x", "");
        e.attributes.insert("time".into(), text_attr("2026-09-26T01:45:18.204Z"));
        assert_eq!(event_time(&e).map(|t| t.nanos), Some(204_000_000));
    }

    #[test]
    fn facets_count_types_most_common_first() {
        let events = [event("a", ""), event("b", ""), event("a", ""), event("c", ""), event("b", ""), event("a", "")];
        assert_eq!(type_facets(events.iter()), [("a".to_string(), 3), ("b".to_string(), 2), ("c".to_string(), 1)]);
    }

    #[test]
    fn row_times_drop_the_date_only_for_today() {
        let now = DateTime::parse_from_rfc3339("2026-09-26T12:00:00Z").unwrap().with_timezone(&Utc);
        let today = Some(Timestamp { seconds: DateTime::parse_from_rfc3339("2026-09-26T01:45:18Z").unwrap().timestamp(), nanos: 204_000_000 });
        let earlier = Some(Timestamp { seconds: DateTime::parse_from_rfc3339("2026-09-24T23:05:00Z").unwrap().timestamp(), nanos: 0 });
        assert_eq!(row_time(&today, now), "01:45:18.204");
        assert_eq!(row_time(&earlier, now), "Sep 24 23:05:00");
        assert_eq!(row_time(&None, now), "—");
    }
}
