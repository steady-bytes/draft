//! Chart geometry: pure functions from data to SVG coordinates, shared by every chart component
//! and unit-tested natively.

/// `M0 18 L12 16 …` for a sparkline drawn in a `width × height` box. Values are scaled to the
/// series' own min/max; a constant series draws a flat mid-height line.
pub fn spark_path(values: &[f64], width: f64, height: f64) -> String {
    if values.is_empty() {
        return String::new();
    }
    let (min, max) = min_max(values);
    let range = if (max - min).abs() < 0.01 { 1.0 } else { max - min };
    let pad = 2.0;
    let last = (values.len().max(2) - 1) as f64;
    let mut d = String::new();
    for (i, v) in values.iter().enumerate() {
        let x = i as f64 / last * width;
        let y = if (max - min).abs() < 0.01 {
            height / 2.0
        } else {
            height - pad - (v - min) / range * (height - 2.0 * pad)
        };
        d.push_str(&format!("{}{:.1} {:.1} ", if i == 0 { "M" } else { "L" }, x, y));
    }
    d.trim_end().to_string()
}

pub fn min_max(values: &[f64]) -> (f64, f64) {
    values.iter().fold((f64::INFINITY, f64::NEG_INFINITY), |(lo, hi), &v| (lo.min(v), hi.max(v)))
}

/// "Nice" axis ticks covering `[min, max]` with roughly `target` divisions (1/2/5 × 10ⁿ steps).
pub fn nice_ticks(min: f64, max: f64, target: usize) -> Vec<f64> {
    let (min, max) = if (max - min).abs() < f64::EPSILON { (min - 1.0, max + 1.0) } else { (min, max) };
    let raw = (max - min) / target.max(1) as f64;
    let mag = 10f64.powf(raw.log10().floor());
    let step = match raw / mag {
        r if r <= 1.0 => 1.0,
        r if r <= 2.0 => 2.0,
        r if r <= 5.0 => 5.0,
        _ => 10.0,
    } * mag;
    let start = (min / step).floor() * step;
    let mut ticks = vec![];
    let mut i = 0.0;
    loop {
        let t = start + i * step;
        ticks.push((t / step).round() * step);
        if t >= max - step * 1e-9 {
            break;
        }
        i += 1.0;
    }
    ticks
}

/// A compact number for an axis label: `4`, `1.5`, `12k`, `3.2M`.
pub fn compact(v: f64) -> String {
    let a = v.abs();
    if a >= 1_000_000.0 {
        format!("{:.1}M", v / 1_000_000.0).replace(".0M", "M")
    } else if a >= 10_000.0 {
        format!("{:.0}k", v / 1000.0)
    } else if a >= 1000.0 {
        format!("{:.1}k", v / 1000.0).replace(".0k", "k")
    } else if (v - v.round()).abs() < 1e-9 {
        format!("{}", v.round() as i64)
    } else {
        format!("{v:.2}").trim_end_matches('0').trim_end_matches('.').to_string()
    }
}

/// Maps `x` in `[x0, x1]` to `[a, b]` (clamped denominator so a single point does not divide by 0).
pub fn scale(x: f64, x0: f64, x1: f64, a: f64, b: f64) -> f64 {
    let d = if (x1 - x0).abs() < f64::EPSILON { 1.0 } else { x1 - x0 };
    a + (x - x0) / d * (b - a)
}

/// The index of the point in sorted `xs` nearest to `x` (for a crosshair).
pub fn nearest(xs: &[f64], x: f64) -> Option<usize> {
    if xs.is_empty() {
        return None;
    }
    let mut best = 0;
    let mut best_d = f64::INFINITY;
    for (i, &v) in xs.iter().enumerate() {
        let d = (v - x).abs();
        if d < best_d {
            best = i;
            best_d = d;
        }
    }
    Some(best)
}

/// Stacked bar heights in px for one histogram bucket. Each non-zero segment keeps at least
/// `min_px`, so a single failure stays visible as a cap on a tall bar.
pub fn stack_heights(values: &[f64], max_total: f64, plot_h: f64, min_px: f64) -> Vec<f64> {
    values
        .iter()
        .map(|&v| if v <= 0.0 { 0.0 } else { (v / max_total * plot_h).max(min_px) })
        .collect()
}

/// Groups timestamped items into `n_buckets` equal time buckets between `start` and `end`
/// (unix seconds), counting per class. `items` are `(seconds, class)`; classes outside
/// `0..n_classes` and times outside the range are ignored. Replaces the bucketing that was
/// duplicated in Beacon's `SeverityHistogram` and `WideEventHistogram`.
pub fn bucket_counts(items: &[(i64, usize)], n_classes: usize, start: i64, end: i64, n_buckets: usize) -> Vec<Vec<f64>> {
    let mut out = vec![vec![0.0; n_classes]; n_buckets];
    if n_buckets == 0 || end <= start {
        return out;
    }
    let width = (end - start) as f64 / n_buckets as f64;
    for &(t, class) in items {
        if t < start || t > end || class >= n_classes {
            continue;
        }
        let idx = (((t - start) as f64 / width) as usize).min(n_buckets - 1);
        out[idx][class] += 1.0;
    }
    out
}

/// `(fraction, label)` axis ticks for a span of `total_ns`, `n + 1` of them from 0 to the total.
pub fn duration_ticks(total_ns: u64, n: usize) -> Vec<(f64, String)> {
    (0..=n)
        .map(|i| {
            let f = i as f64 / n as f64;
            let label = if i == 0 { "0".to_string() } else { crate::util::format_duration_ns((total_ns as f64 * f) as u64) };
            (f, label)
        })
        .collect()
}

/// Left offset and width, as fractions of `total`, of a span starting `start` after the trace began.
pub fn span_fractions(start: u64, duration: u64, total: u64) -> (f64, f64) {
    let total = total.max(1) as f64;
    let left = (start as f64 / total).clamp(0.0, 1.0);
    // A span too short to see keeps a hairline so it is still visible.
    let width = (duration as f64 / total).clamp(0.004, 1.0 - left);
    (left, width)
}

/// `(last, min, avg, max)` of a series' values, or `None` when it is empty.
pub fn series_stats(values: &[f64]) -> Option<(f64, f64, f64, f64)> {
    let last = *values.last()?;
    let (min, max) = min_max(values);
    let avg = values.iter().sum::<f64>() / values.len() as f64;
    Some((last, min, avg, max))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn spark_path_starts_with_move_and_has_one_point_per_value() {
        let d = spark_path(&[1.0, 3.0, 2.0], 96.0, 24.0);
        assert!(d.starts_with("M0.0 "));
        assert_eq!(d.matches('L').count(), 2);
    }

    #[test]
    fn spark_path_flat_series_is_mid_height() {
        let d = spark_path(&[5.0, 5.0, 5.0], 96.0, 24.0);
        assert!(d.contains("12.0"), "{d}");
    }

    #[test]
    fn spark_path_empty() {
        assert_eq!(spark_path(&[], 96.0, 24.0), "");
    }

    #[test]
    fn ticks_are_nice_and_cover_the_range() {
        assert_eq!(nice_ticks(0.0, 4.0, 4), vec![0.0, 1.0, 2.0, 3.0, 4.0]);
        let t = nice_ticks(0.0, 4200.0, 5);
        assert!(t.first().copied().unwrap() <= 0.0 && t.last().copied().unwrap() >= 4200.0);
        assert!(nice_ticks(3.0, 3.0, 4).len() >= 2);
    }

    #[test]
    fn compact_numbers() {
        assert_eq!(compact(4.0), "4");
        assert_eq!(compact(1.5), "1.5");
        assert_eq!(compact(12_400.0), "12k");
        assert_eq!(compact(2_000_000.0), "2M");
    }

    #[test]
    fn scale_and_nearest() {
        assert_eq!(scale(5.0, 0.0, 10.0, 0.0, 100.0), 50.0);
        assert_eq!(scale(1.0, 1.0, 1.0, 0.0, 100.0), 0.0);
        assert_eq!(nearest(&[0.0, 10.0, 20.0], 13.0), Some(1));
        assert_eq!(nearest(&[], 1.0), None);
    }

    #[test]
    fn small_segments_keep_a_minimum_height() {
        let h = stack_heights(&[100.0, 1.0, 0.0], 180.0, 72.0, 3.0);
        assert!(h[1] >= 3.0);
        assert_eq!(h[2], 0.0);
    }

    #[test]
    fn buckets_count_per_class_and_ignore_outliers() {
        let items = [(0, 0), (5, 0), (5, 1), (9, 1), (10, 0), (99, 0), (5, 7)];
        let b = bucket_counts(&items, 2, 0, 10, 2);
        assert_eq!(b, vec![vec![1.0, 0.0], vec![2.0, 2.0]]);
        assert_eq!(bucket_counts(&items, 2, 5, 5, 3), vec![vec![0.0, 0.0]; 3]);
    }

    #[test]
    fn duration_ticks_span_zero_to_total() {
        let t = duration_ticks(2_200_000_000, 4);
        assert_eq!(t.len(), 5);
        assert_eq!(t[0].1, "0");
        assert_eq!(t[4].1, "2.20s");
    }

    #[test]
    fn span_fractions_keep_a_hairline_and_stay_inside() {
        let (l, w) = span_fractions(0, 1, 1_000_000);
        assert_eq!(l, 0.0);
        assert!(w >= 0.004);
        let (l, w) = span_fractions(900, 500, 1000);
        assert!(l + w <= 1.0 + 1e-9);
    }

    #[test]
    fn stats() {
        assert_eq!(series_stats(&[1.0, 3.0, 2.0]), Some((2.0, 1.0, 2.0, 3.0)));
        assert_eq!(series_stats(&[]), None);
    }
}
