package main

import (
	"testing"
	"time"

	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"
)

func rec(id, wf string, status workflowv1.RunStatus, ago, took time.Duration) runRecord {
	r := runRecord{RunID: id, Workflow: wf, Status: status, Started: testNow.Add(-ago)}
	if took > 0 {
		r.Finished = r.Started.Add(took)
	}
	return r
}

var (
	passed  = workflowv1.RunStatus_RUN_STATUS_PASSED
	failed  = workflowv1.RunStatus_RUN_STATUS_FAILED
	running = workflowv1.RunStatus_RUN_STATUS_RUNNING
)

func TestParseWindowFallsBackToTheDefault(t *testing.T) {
	for key, want := range map[string]string{"1h": "1h", "24h": "24h", "7d": "7d", "": "24h", "1y": "24h", "24H": "24h"} {
		if got := parseWindow(key).Key; got != want {
			t.Errorf("parseWindow(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestMedian(t *testing.T) {
	if _, ok := median(nil); ok {
		t.Error("an empty set has no median")
	}
	if m, _ := median([]time.Duration{5 * time.Second, time.Second, 3 * time.Second}); m != 3*time.Second {
		t.Errorf("odd count: %v", m)
	}
	if m, _ := median([]time.Duration{time.Second, 3 * time.Second}); m != 2*time.Second {
		t.Errorf("even count is the mean of the middle two: %v", m)
	}
}

func TestOverviewStatsCountOnlyTheWindowAndCompareThePreviousOne(t *testing.T) {
	w := timeWindow{"24h", 24 * time.Hour}
	runs := []runRecord{
		rec("a", "x", passed, time.Hour, time.Second),
		rec("b", "x", passed, 2*time.Hour, 3*time.Second),
		rec("c", "x", failed, 3*time.Hour, 2*time.Second),
		rec("d", "y", running, time.Second, 0),
		rec("prev", "x", passed, 30*time.Hour, 10*time.Second), // the window before
		rec("old", "x", failed, 60*time.Hour, time.Second),     // older than both: ignored
	}
	lasts := map[string]runRecord{"x": runs[2], "y": runs[3]}
	s := computeOverviewStats(testNow, w, runs, lasts)

	if s.Terminal != 3 || s.Passed != 2 {
		t.Errorf("terminal/passed = %d/%d, want 3/2", s.Terminal, s.Passed)
	}
	if rate, ok := s.PassRate(); !ok || rate < 66.6 || rate > 66.7 {
		t.Errorf("pass rate %.2f", rate)
	}
	if len(s.InFlight) != 1 || s.InFlight[0].RunID != "d" {
		t.Errorf("in flight: %+v", s.InFlight)
	}
	if len(s.FailingWorkflows) != 1 || s.FailingWorkflows[0] != "x" {
		t.Errorf("failing: %v", s.FailingWorkflows)
	}
	if !s.HasMedian || s.Median != 2*time.Second {
		t.Errorf("median %v", s.Median)
	}
	if !s.HasPrev || s.PrevMedian != 10*time.Second {
		t.Errorf("previous median %v", s.PrevMedian)
	}
	if len(s.PassSpark) != passSparkBuckets {
		t.Errorf("spark has %d points", len(s.PassSpark))
	}
}

func TestOverviewStatsWithNoRuns(t *testing.T) {
	s := computeOverviewStats(testNow, defaultWindow(), nil, nil)
	if _, ok := s.PassRate(); ok || s.HasMedian || s.PassSpark != nil {
		t.Errorf("nothing ran, yet: %+v", s)
	}
}

func TestPassSparkCarriesQuietSlices(t *testing.T) {
	start := testNow.Add(-12 * time.Hour)
	runs := []runRecord{
		rec("1", "x", passed, 11*time.Hour+30*time.Minute, time.Second), // slice 0
		rec("2", "x", failed, 5*time.Hour+30*time.Minute, time.Second),  // slice 6
	}
	spark := passSpark(runs, start, testNow, 12)
	if spark[0] != 100 || spark[3] != 100 { // quiet slices repeat the last known rate
		t.Errorf("early slices: %v", spark)
	}
	if spark[6] != 0 || spark[11] != 0 {
		t.Errorf("after the failure: %v", spark)
	}
}

func TestVerdict(t *testing.T) {
	mk := func(rs ...runRecord) map[string]runRecord {
		m := map[string]runRecord{}
		for _, r := range rs {
			m[r.Workflow] = r
		}
		return m
	}
	cases := []struct {
		name      string
		workflows int
		lasts     map[string]runRecord
		failed    []failedLast
		class     string
		lead      string
		text      string
	}{
		{"empty", 0, nil, nil, "d-verdict--idle", "Empty bench.", ""},
		{"never run", 2, nil, nil, "d-verdict--idle", "Nothing has run yet.", "2 workflows loaded, none run so far."},
		{"green", 2, mk(rec("1", "a", passed, time.Hour, time.Second), rec("2", "b", passed, time.Hour, time.Second)), nil, "d-verdict--ok", "All green.", "2 of 2 workflows passed their last run."},
		{"mostly", 2, mk(rec("1", "a", passed, time.Hour, time.Second), rec("2", "b", failed, time.Hour, time.Second)),
			[]failedLast{{Workflow: "b", Step: "step 3", At: testNow.Add(-time.Hour)}}, "", "Mostly.", "1 of 2 workflows passed their last run. b failed at step 3 at 01:00 UTC."},
		{"red", 1, mk(rec("1", "a", failed, time.Hour, time.Second)), []failedLast{{Workflow: "a"}}, "d-verdict--err", "Red.", "0 of 1 workflow passed their last run. a failed."},
		{"some never run", 3, mk(rec("1", "a", passed, time.Hour, time.Second)), nil, "d-verdict--ok", "All green.", "1 of 3 workflows passed their last run. 2 never run."},
	}
	for _, c := range cases {
		v := computeVerdict(c.workflows, c.lasts, c.failed)
		if v.Class != c.class || v.Lead != c.lead || (c.text != "" && v.Text != c.text) {
			t.Errorf("%s: got %+v", c.name, v)
		}
	}
}

func TestHistoryStripPadsOldestFirst(t *testing.T) {
	recent := []runRecord{ // newest first, as the store returns them
		rec("new", "x", failed, time.Hour, time.Second),
		rec("mid", "x", running, 2*time.Hour, 0),
		rec("old", "x", passed, 3*time.Hour, time.Second),
	}
	strip := historyStrip(recent, testNow, "mid")
	if len(strip.Cells) != historyRuns {
		t.Fatalf("%d cells", len(strip.Cells))
	}
	states := []string{}
	for _, c := range strip.Cells[historyRuns-3:] {
		states = append(states, c.Class())
	}
	if states[0] != "" || states[1] != "is-run is-cur" || states[2] != "is-err" {
		t.Errorf("newest last, current marked: %v", states)
	}
	if strip.Cells[0].State != "none" || strip.Cells[8].State != "none" || strip.Cells[9].State == "none" {
		t.Errorf("nine leading never-run cells expected: %+v", strip.Cells)
	}
	if want := "1 passed, 1 failed, 1 running, 9 never run"; strip.Label != want {
		t.Errorf("label %q, want %q", strip.Label, want)
	}
}

func TestFormatting(t *testing.T) {
	for d, want := range map[time.Duration]string{
		0: "0ms", 812 * time.Millisecond: "812ms", 1400 * time.Millisecond: "1.4s", 38200 * time.Millisecond: "38.2s", 123 * time.Second: "2m 03s",
	} {
		if got := dur(d); got != want {
			t.Errorf("dur(%v) = %q, want %q", d, got, want)
		}
	}
	if v, u := durParts(1400 * time.Millisecond); v != "1.4" || u != "s" {
		t.Errorf("durParts = %q %q", v, u)
	}
	if got := clock(testNow.Add(-90*time.Minute), testNow); got != "00:30 UTC" {
		t.Errorf("recent run: %q", got)
	}
	if got := clock(testNow.Add(-50*time.Hour), testNow); got != "Sep 24" {
		t.Errorf("older run: %q", got)
	}
	if got := clock(time.Time{}, testNow); got != "—" {
		t.Errorf("never started: %q", got)
	}
	label, good := deltaLabel(1100*time.Millisecond, 1400*time.Millisecond, "24h")
	if label != "▼ 300ms vs prev 24h" || good != "good" {
		t.Errorf("faster: %q %q", label, good)
	}
	if label, good = deltaLabel(2*time.Second, time.Second, "1h"); label != "▲ 1.0s vs prev 1h" || good != "bad" {
		t.Errorf("slower: %q %q", label, good)
	}
	if label, good = deltaLabel(time.Second, time.Second, "7d"); good != "" || label != "no change vs prev 7d" {
		t.Errorf("flat: %q %q", label, good)
	}
}
