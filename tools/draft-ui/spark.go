package draftui

import (
	"fmt"
	"math"
	"strings"
)

// SparkPath is the SVG `d` for a sparkline drawn in a width×height box. Values are scaled to the
// series' own min/max; a constant series draws a flat mid-height line. It mirrors the Rust
// `viz::geom::spark_path` so both renderers draw the same line.
func SparkPath(values []float64, width, height float64) string {
	if len(values) == 0 {
		return ""
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, v := range values {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	flat := math.Abs(hi-lo) < 0.01
	rng := hi - lo
	if flat {
		rng = 1
	}
	const pad = 2.0
	last := float64(len(values) - 1)
	if last < 1 {
		last = 1
	}
	var b strings.Builder
	for i, v := range values {
		x := float64(i) / last * width
		y := height / 2
		if !flat {
			y = height - pad - (v-lo)/rng*(height-2*pad)
		}
		op := "L"
		if i == 0 {
			op = "M"
		}
		fmt.Fprintf(&b, "%s%.1f %.1f ", op, x, y)
	}
	return strings.TrimSpace(b.String())
}
