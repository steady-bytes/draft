#![allow(unused_imports)]
use super::*;

// ── Geometry ──────────────────────────────────────────────────────────────────

pub(super) const NW: f64 = 178.0;
pub(super) const NH: f64 = 84.0;

pub(super) fn port_of(nx: f64, ny: f64, other_cx: f64) -> (f64, f64, i32) {
    if other_cx >= nx + NW / 2.0 {
        (nx + NW, ny + NH / 2.0, 1)
    } else {
        (nx, ny + NH / 2.0, -1)
    }
}

pub(super) fn route_pts(sx: f64, sy: f64, sd: i32, ex: f64, ey: f64, ed: i32) -> Vec<(f64, f64)> {
    const STUB: f64 = 14.0;
    let (p1x, p1y) = (sx + sd as f64 * STUB, sy);
    let (p4x, p4y) = (ex + ed as f64 * STUB, ey);
    let mut pts = vec![(sx, sy), (p1x, p1y)];
    let (dx, dy) = (p4x - p1x, p4y - p1y);
    let (adx, ady) = (dx.abs(), dy.abs());
    let sgx: f64 = if dx >= 0.0 { 1.0 } else { -1.0 };
    let sgy: f64 = if dy >= 0.0 { 1.0 } else { -1.0 };
    if ady < 0.5 {
    } else if sgx == sd as f64 && adx >= ady {
        pts.push((p1x + sgx * (adx - ady), p1y));
    } else if sgx == sd as f64 && ady > adx {
        pts.push((p1x + sgx * adx, p1y + sgy * adx));
    } else {
        let mid_y = p1y + sgy * (ady / 2.0).max(40.0);
        pts.push((p1x, mid_y));
        pts.push((p4x, mid_y));
    }
    pts.push((p4x, p4y));
    pts.push((ex, ey));
    let mut out: Vec<(f64, f64)> = Vec::new();
    for &p in &pts {
        if out.last().map_or(true, |&q: &(f64, f64)| {
            (p.0 - q.0).abs() > 0.25 || (p.1 - q.1).abs() > 0.25
        }) {
            out.push(p);
        }
    }
    out
}

pub(super) fn pts_path(pts: &[(f64, f64)]) -> String {
    pts.iter()
        .enumerate()
        .map(|(i, (x, y))| {
            if i == 0 {
                format!("M {x:.1} {y:.1}")
            } else {
                format!("L {x:.1} {y:.1}")
            }
        })
        .collect::<Vec<_>>()
        .join(" ")
}

pub(super) fn poly_len(pts: &[(f64, f64)]) -> f64 {
    pts.windows(2)
        .map(|w| {
            let ((x0, y0), (x1, y1)) = (w[0], w[1]);
            ((x1 - x0).powi(2) + (y1 - y0).powi(2)).sqrt()
        })
        .sum()
}

pub(super) fn point_at(pts: &[(f64, f64)], mut d: f64) -> (f64, f64, f64) {
    for w in pts.windows(2) {
        let ((x0, y0), (x1, y1)) = (w[0], w[1]);
        let seg = ((x1 - x0).powi(2) + (y1 - y0).powi(2)).sqrt();
        if d <= seg {
            let t = if seg > 0.0 { d / seg } else { 0.0 };
            return (
                x0 + (x1 - x0) * t,
                y0 + (y1 - y0) * t,
                (y1 - y0).atan2(x1 - x0),
            );
        }
        d -= seg;
    }
    let &(lx, ly) = pts.last().unwrap_or(&(0.0, 0.0));
    (lx, ly, 0.0)
}

/// `color` at `alpha` opacity, as a CSS colour: works for any colour token (`var(--bp)`) because the
/// mix is done by the browser, so it follows the theme.
pub(super) fn mix(color: &str, alpha: f64) -> String {
    format!("color-mix(in oklab, {color} {:.0}%, transparent)", (alpha * 100.0).clamp(0.0, 100.0))
}

pub(super) fn snap8(v: f64) -> f64 {
    (v / 8.0).round() * 8.0
}
