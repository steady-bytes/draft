// This file is the arithmetic behind the overview page: which window the numbers cover, the pass
// rate and its trend, the median run and how it moved, the one-sentence verdict, and the history
// strip beside each workflow. Everything here is a pure function of the runs handed in, so it is
// tested without a database (ui_stats_test.go).
package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	draftui "github.com/steady-bytes/draft/tools/draft-ui"
)

// --- window ----------------------------------------------------------------------------------

// timeWindow is the span the overview's numbers cover (`?window=`).
type timeWindow struct {
	Key string
	Dur time.Duration
}

var overviewWindows = []timeWindow{
	{"1h", time.Hour},
	{"24h", 24 * time.Hour},
	{"7d", 7 * 24 * time.Hour},
}

// defaultWindow is 24h, the window the mockup opens on.
func defaultWindow() timeWindow { return overviewWindows[1] }

// parseWindow resolves `?window=`; anything unknown falls back to the default rather than erroring,
// since the value comes from a URL a person may have edited.
func parseWindow(key string) timeWindow {
	for _, w := range overviewWindows {
		if w.Key == key {
			return w
		}
	}
	return defaultWindow()
}

// --- statistics ------------------------------------------------------------------------------

// overviewStats are the four tiles and the verdict's inputs.
type overviewStats struct {
	Terminal, Passed int
	// PassSpark is the pass rate (0–100) per slice of the window, oldest first; nil with no runs.
	PassSpark []float64
	// InFlight are the runs that have not finished.
	InFlight []runRecord
	// FailingWorkflows are the workflows whose last run failed, most recently failed first.
	FailingWorkflows   []string
	Median, PrevMedian time.Duration
	HasMedian, HasPrev bool
}

// PassRate is the share of finished runs that passed, 0–100; ok is false with none finished.
func (s overviewStats) PassRate() (rate float64, ok bool) {
	if s.Terminal == 0 {
		return 0, false
	}
	return float64(s.Passed) / float64(s.Terminal) * 100, true
}

const passSparkBuckets = 12

// computeOverviewStats reads runs (everything started since now-2×window, any order) and lasts
// (each workflow's most recent run) into the overview's numbers. The second window is only there
// to say whether the median run got faster or slower.
func computeOverviewStats(now time.Time, w timeWindow, runs []runRecord, lasts map[string]runRecord) overviewStats {
	var s overviewStats
	start := now.Add(-w.Dur)
	prevStart := now.Add(-2 * w.Dur)

	var current []runRecord
	var durs, prevDurs []time.Duration
	seenInFlight := map[string]bool{}
	for _, r := range runs {
		if r.InFlight() && !seenInFlight[r.RunID] {
			seenInFlight[r.RunID] = true
			s.InFlight = append(s.InFlight, r)
		}
		switch {
		case r.Started.IsZero():
		case !r.Started.Before(start):
			current = append(current, r)
			if r.Passed() || r.Failed() {
				s.Terminal++
				if r.Passed() {
					s.Passed++
				}
				if d, ok := r.Duration(); ok {
					durs = append(durs, d)
				}
			}
		case !r.Started.Before(prevStart):
			if d, ok := r.Duration(); ok && (r.Passed() || r.Failed()) {
				prevDurs = append(prevDurs, d)
			}
		}
	}
	// A workflow's last run may be older than the loaded window and still be in flight.
	for _, r := range lasts {
		if r.InFlight() && !seenInFlight[r.RunID] {
			seenInFlight[r.RunID] = true
			s.InFlight = append(s.InFlight, r)
		}
	}
	sort.Slice(s.InFlight, func(i, j int) bool { return s.InFlight[i].Started.After(s.InFlight[j].Started) })

	s.PassSpark = passSpark(current, start, now, passSparkBuckets)
	s.Median, s.HasMedian = median(durs)
	s.PrevMedian, s.HasPrev = median(prevDurs)

	var failing []runRecord
	for _, r := range lasts {
		if r.Failed() {
			failing = append(failing, r)
		}
	}
	sort.Slice(failing, func(i, j int) bool { return failing[i].Started.After(failing[j].Started) })
	for _, r := range failing {
		s.FailingWorkflows = append(s.FailingWorkflows, r.Workflow)
	}
	return s
}

// median is the middle duration, or the mean of the two middle ones.
func median(ds []time.Duration) (time.Duration, bool) {
	if len(ds) == 0 {
		return 0, false
	}
	sorted := append([]time.Duration(nil), ds...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2], true
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2, true
}

// passSpark is the pass rate in each of `buckets` equal slices of [start, end). A slice with no
// finished runs repeats the previous rate (the overall rate for a leading gap), so a quiet hour is a
// flat stretch, not a plunge to zero.
func passSpark(runs []runRecord, start, end time.Time, buckets int) []float64 {
	if len(runs) == 0 || buckets <= 0 || !end.After(start) {
		return nil
	}
	passed := make([]int, buckets)
	total := make([]int, buckets)
	span := end.Sub(start)
	allPassed, all := 0, 0
	for _, r := range runs {
		if !(r.Passed() || r.Failed()) {
			continue
		}
		i := int(float64(r.Started.Sub(start)) / float64(span) * float64(buckets))
		if i < 0 {
			i = 0
		}
		if i >= buckets {
			i = buckets - 1
		}
		total[i]++
		all++
		if r.Passed() {
			passed[i]++
			allPassed++
		}
	}
	if all == 0 {
		return nil
	}
	carry := float64(allPassed) / float64(all) * 100
	out := make([]float64, buckets)
	for i := range out {
		if total[i] > 0 {
			carry = float64(passed[i]) / float64(total[i]) * 100
		}
		out[i] = carry
	}
	return out
}

// --- verdict ---------------------------------------------------------------------------------

// verdict is the sentence under the page title: "Mostly. 4 of 5 workflows passed their last run."
type verdict struct {
	// Class is the `d-verdict--*` modifier: "" (amber), d-verdict--ok, d-verdict--err, d-verdict--idle.
	Class string
	Lead  string
	Text  string
}

// failedLast describes a workflow whose last run failed, for the verdict's second sentence.
type failedLast struct {
	Workflow string
	// Step is the failing step's position ("step 14"), empty when unknown.
	Step string
	At   time.Time
}

// computeVerdict judges the bench from each workflow's last run. workflows is how many exist; lasts
// holds the last run of those that have run.
func computeVerdict(workflows int, lasts map[string]runRecord, failed []failedLast) verdict {
	if workflows == 0 {
		return verdict{Class: "d-verdict--idle", Lead: "Empty bench.", Text: "No workflows yet. Create one, or drop a YAML file into the configured workflows_dir."}
	}
	var passed, failing, inflight int
	for _, r := range lasts {
		switch {
		case r.Passed():
			passed++
		case r.Failed():
			failing++
		case r.InFlight():
			inflight++
		}
	}
	ran := passed + failing + inflight
	if ran == 0 {
		return verdict{Class: "d-verdict--idle", Lead: "Nothing has run yet.", Text: fmt.Sprintf("%s loaded, none run so far.", plural(workflows, "workflow"))}
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("%d of %s passed their last run.", passed, plural(workflows, "workflow")))
	if failing > 0 && len(failed) > 0 {
		f := failed[0]
		s := f.Workflow + " failed"
		if f.Step != "" {
			s += " at " + f.Step
		}
		if !f.At.IsZero() {
			s += " at " + f.At.UTC().Format("15:04") + " UTC"
		}
		parts = append(parts, s+".")
	}
	if never := workflows - ran; never > 0 {
		parts = append(parts, fmt.Sprintf("%d never run.", never))
	}

	v := verdict{Text: strings.Join(parts, " ")}
	switch {
	case failing == 0:
		v.Class, v.Lead = "d-verdict--ok", "All green."
	case passed == 0 && inflight == 0:
		v.Class, v.Lead = "d-verdict--err", "Red."
	default:
		v.Lead = "Mostly."
	}
	return v
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// --- history strip ---------------------------------------------------------------------------

const historyRuns = 12

// historyStrip draws a workflow's last runs oldest → newest, padded on the left with "never run"
// cells so every row is the same width. recent is newest first. current, when set, marks that run.
func historyStrip(recent []runRecord, now time.Time, current string) draftui.Strip {
	if len(recent) > historyRuns {
		recent = recent[:historyRuns]
	}
	cells := make([]draftui.Cell, 0, historyRuns)
	for i := len(recent); i < historyRuns; i++ {
		cells = append(cells, draftui.Cell{State: "none", Title: "never run"})
	}
	var passed, failed, running int
	for i := len(recent) - 1; i >= 0; i-- {
		r := recent[i]
		c := draftui.Cell{Current: current != "" && r.RunID == current}
		switch {
		case r.Passed():
			passed++
			c.Title = "passed"
		case r.Failed():
			failed++
			c.State, c.Title = "err", "failed"
		default:
			running++
			c.State, c.Title = "run", "running"
		}
		c.Title = fmt.Sprintf("%s · %s", c.Title, clock(r.Started, now))
		cells = append(cells, c)
	}
	var label []string
	if passed > 0 {
		label = append(label, fmt.Sprintf("%d passed", passed))
	}
	if failed > 0 {
		label = append(label, fmt.Sprintf("%d failed", failed))
	}
	if running > 0 {
		label = append(label, fmt.Sprintf("%d running", running))
	}
	if n := historyRuns - len(recent); n > 0 {
		label = append(label, fmt.Sprintf("%d never run", n))
	}
	return draftui.Strip{Cells: cells, Label: strings.Join(label, ", ")}
}

// --- formatting ------------------------------------------------------------------------------

// clock is when a run started: the time of day (UTC) if it was in the last day, else the date.
func clock(t, now time.Time) string {
	if t.IsZero() {
		return "—"
	}
	t = t.UTC()
	if now.Sub(t) < 24*time.Hour && now.Sub(t) >= -time.Minute {
		return t.Format("15:04") + " UTC"
	}
	return t.Format("Jan 2")
}

// clockSeconds is a full timestamp for the run page.
func clockSeconds(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.UTC().Format("Jan 2 15:04:05") + " UTC"
}

// dur formats a duration for a table: milliseconds under a second, then seconds, then minutes.
func dur(d time.Duration) string {
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	default:
		d = d.Round(time.Second)
		return fmt.Sprintf("%dm %02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
}

// deltaLabel says how the median moved against the previous window: "▼ 0.3s vs prev 24h". good is
// "good" when the run got faster (a smaller median is better), "bad" when slower, "" when flat.
func deltaLabel(cur, prev time.Duration, windowKey string) (label, good string) {
	diff := cur - prev
	if abs(diff) < 50*time.Millisecond {
		return "no change vs prev " + windowKey, ""
	}
	if diff < 0 {
		return fmt.Sprintf("▼ %s vs prev %s", dur(-diff), windowKey), "good"
	}
	return fmt.Sprintf("▲ %s vs prev %s", dur(diff), windowKey), "bad"
}

func abs(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
