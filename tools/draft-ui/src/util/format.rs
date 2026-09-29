/// `1.23s`, `4.56ms`, `7.89µs`, `12ns` — ported from Beacon's `flame_graph.rs`.
pub fn format_duration_ns(ns: u64) -> String {
    if ns >= 1_000_000_000 {
        format!("{:.2}s", ns as f64 / 1_000_000_000.0)
    } else if ns >= 1_000_000 {
        format!("{:.2}ms", ns as f64 / 1_000_000.0)
    } else if ns >= 1_000 {
        format!("{:.2}µs", ns as f64 / 1_000.0)
    } else {
        format!("{ns}ns")
    }
}

/// `612 B`, `3.1 KB`, `1.2 MB`.
pub fn format_bytes(n: u64) -> String {
    const UNITS: [&str; 4] = ["B", "KB", "MB", "GB"];
    if n < 1024 {
        return format!("{n} B");
    }
    let mut v = n as f64;
    let mut u = 0;
    while v >= 1024.0 && u < UNITS.len() - 1 {
        v /= 1024.0;
        u += 1;
    }
    format!("{v:.1} {}", UNITS[u])
}

/// `4,118` — thousands separators for counters in stat tiles and the status bar.
pub fn format_count(n: u64) -> String {
    let s = n.to_string();
    let mut out = String::with_capacity(s.len() + s.len() / 3);
    for (i, c) in s.chars().enumerate() {
        if i > 0 && (s.len() - i).is_multiple_of(3) {
            out.push(',');
        }
        out.push(c);
    }
    out
}

/// `just now`, `12s ago`, `4 min ago`, `2 h ago`, `3 d ago`.
pub fn relative_time(seconds_ago: i64) -> String {
    match seconds_ago {
        s if s < 5 => "just now".to_string(),
        s if s < 60 => format!("{s}s ago"),
        s if s < 3600 => format!("{} min ago", s / 60),
        s if s < 86_400 => format!("{} h ago", s / 3600),
        s => format!("{} d ago", s / 86_400),
    }
}

/// `type.googleapis.com/core.registry.key_value.v1.Value` → `Value`; falls back to everything
/// after the last `.` and then to the input. Ported from Blueprint's `short_kind_name`.
pub fn short_type_name(type_url: &str) -> &str {
    type_url
        .strip_prefix("type.googleapis.com/")
        .and_then(|s| s.rsplit('.').next())
        .unwrap_or(type_url)
}

/// Flattens whitespace and truncates to `max` characters with an ellipsis (a one-line preview of a
/// JSON value). Ported from Blueprint's `truncate_preview`.
pub fn truncate_preview(text: &str, max: usize) -> String {
    let flat: String = text.split_whitespace().collect::<Vec<_>>().join(" ");
    if flat.chars().count() <= max {
        flat
    } else {
        let cut: String = flat.chars().take(max).collect();
        format!("{cut}…")
    }
}

/// `5ca1ef52…d68c` — keeps both ends of an id.
pub fn truncate_middle(text: &str, head: usize, tail: usize) -> String {
    let n = text.chars().count();
    if n <= head + tail + 1 {
        return text.to_string();
    }
    let h: String = text.chars().take(head).collect();
    let t: String = text.chars().skip(n - tail).collect();
    format!("{h}…{t}")
}

/// `01:47` (UTC) for a unix timestamp — axis labels and tooltips.
pub fn format_clock(unix_seconds: i64) -> String {
    let s = unix_seconds.rem_euclid(86_400);
    format!("{:02}:{:02}", s / 3600, (s % 3600) / 60)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn durations() {
        assert_eq!(format_duration_ns(12), "12ns");
        assert_eq!(format_duration_ns(1_500), "1.50µs");
        assert_eq!(format_duration_ns(59_700_000), "59.70ms");
        assert_eq!(format_duration_ns(2_210_000_000), "2.21s");
    }

    #[test]
    fn bytes() {
        assert_eq!(format_bytes(612), "612 B");
        assert_eq!(format_bytes(3_174), "3.1 KB");
        assert_eq!(format_bytes(5 * 1024 * 1024), "5.0 MB");
    }

    #[test]
    fn counts() {
        assert_eq!(format_count(0), "0");
        assert_eq!(format_count(999), "999");
        assert_eq!(format_count(4118), "4,118");
        assert_eq!(format_count(1_234_567), "1,234,567");
    }

    #[test]
    fn relative() {
        assert_eq!(relative_time(2), "just now");
        assert_eq!(relative_time(42), "42s ago");
        assert_eq!(relative_time(240), "4 min ago");
        assert_eq!(relative_time(7300), "2 h ago");
        assert_eq!(relative_time(200_000), "2 d ago");
    }

    #[test]
    fn type_names() {
        assert_eq!(short_type_name("type.googleapis.com/core.registry.key_value.v1.Value"), "Value");
        assert_eq!(short_type_name("plain"), "plain");
    }

    #[test]
    fn previews() {
        assert_eq!(truncate_preview("{\n  \"a\": 1\n}", 40), "{ \"a\": 1 }");
        assert_eq!(truncate_preview("abcdefghij", 4), "abcd…");
    }

    #[test]
    fn middle() {
        assert_eq!(truncate_middle("5ca1ef522473b1592123dd68c270", 8, 4), "5ca1ef52…c270");
        assert_eq!(truncate_middle("short", 8, 4), "short");
    }

    #[test]
    fn clock_is_utc_hh_mm() {
        assert_eq!(format_clock(0), "00:00");
        assert_eq!(format_clock(3600 * 13 + 60 * 5 + 7), "13:05");
        assert_eq!(format_clock(-60), "23:59");
    }
}
