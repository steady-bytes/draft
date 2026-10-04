//! Small formatting helpers shared across views -- see
//! services/tooling/lineman/web-client/src/components/mod.rs for the sibling pattern this mirrors.

/// `4:12` from a duration in milliseconds. Relay's own tracks/recordings run well under an hour in
/// every real test so far; this deliberately doesn't handle an hours place -- add one if a real
/// recording ever needs it.
pub fn format_duration_ms(ms: i64) -> String {
    let total_seconds = ms.max(0) / 1000;
    format!("{}:{:02}", total_seconds / 60, total_seconds % 60)
}

/// `24.6 MB` from a byte count.
pub fn format_bytes(bytes: i64) -> String {
    let bytes = bytes.max(0) as f64;
    const KB: f64 = 1024.0;
    const MB: f64 = KB * 1024.0;
    const GB: f64 = MB * 1024.0;
    if bytes >= GB {
        format!("{:.1} GB", bytes / GB)
    } else if bytes >= MB {
        format!("{:.1} MB", bytes / MB)
    } else if bytes >= KB {
        format!("{:.1} KB", bytes / KB)
    } else {
        format!("{bytes:.0} B")
    }
}

// format_when (Sep 28 · 14:30 from a prost_types::Timestamp) isn't used yet -- Transcripts is the
// view that will need it, not built this pass. Add it back when that view lands rather than
// carrying dead code now.

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn durations_are_minutes_and_seconds() {
        assert_eq!(format_duration_ms(252_000), "4:12");
        assert_eq!(format_duration_ms(0), "0:00");
        assert_eq!(format_duration_ms(-500), "0:00");
    }

    #[test]
    fn byte_counts_pick_the_right_unit() {
        assert_eq!(format_bytes(512), "512 B");
        assert_eq!(format_bytes(24_600_000), "23.5 MB");
        assert_eq!(format_bytes(1_900_000_000), "1.8 GB");
    }
}
