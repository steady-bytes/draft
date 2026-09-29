// The Overview page ("Is it green?") and the Recent runs list.
package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"
	draftui "github.com/steady-bytes/draft/tools/draft-ui"
)

const (
	// overviewRunCap bounds how many runs the overview loads to compute its window (and the one
	// before it). A run history is expected to stay small for a long time — the same scale
	// assumption services/tooling/foundry's store.go documents for its catalog — so this is a
	// guard, not a tuned figure. Revisit if that query ever shows up as a real cost.
	overviewRunCap = 5000
	runsPageSize   = 50
)

// --- overview ----------------------------------------------------------------------------------

// overviewQuery is what an overview request carries.
type overviewQuery struct {
	Window timeWindow
	// Filter is "", "failing" or "webhook" (`?filter=`); Text is the free-text search (`?q=`).
	Filter, Text string
}

func parseOverviewQuery(r *http.Request) overviewQuery {
	q := r.URL.Query()
	oq := overviewQuery{Window: parseWindow(q.Get("window")), Text: strings.TrimSpace(q.Get("q"))}
	if f := q.Get("filter"); f == "failing" || f == "webhook" {
		oq.Filter = f
	}
	return oq
}

// href is the overview URL with these settings, skipping the defaults.
func (q overviewQuery) href(base string, mutate func(*overviewQuery)) string {
	if mutate != nil {
		mutate(&q)
	}
	v := url.Values{}
	if q.Window != defaultWindow() {
		v.Set("window", q.Window.Key)
	}
	if q.Filter != "" {
		v.Set("filter", q.Filter)
	}
	if q.Text != "" {
		v.Set("q", q.Text)
	}
	if len(v) == 0 {
		return base
	}
	return base + "?" + v.Encode()
}

type chipLink struct {
	Label, Href string
	Active      bool
}

type overviewRow struct {
	Name, Description string
	Trigger           draftui.Tag
	Webhook           bool
	Steps             int
	History           draftui.Strip
	// Last is nil for a workflow that has never run.
	Last    *lastRunView
	P50     string
	Running bool
	Failing bool
}

// lastRunView is the "Last run" cell: a status and a line under it.
type lastRunView struct {
	Status draftui.StatusDot
	Note   string
	RunID  string
}

// workflowTable is the table and its toolbar state; the overview page and the htmx fragment both
// render it.
type workflowTable struct {
	Rows []overviewRow
	// Empty is the message when there are no rows.
	Empty string
}

type overviewPage struct {
	draftui.Page
	Base     string
	Windows  []chipLink
	Verdict  verdict
	Stats    []draftui.Stat
	Query    overviewQuery
	Chips    []chipLink
	Table    workflowTable
	Workflow int // how many workflows exist, unfiltered
}

func (h *uiHandler) overview(w http.ResponseWriter, r *http.Request) {
	oq := parseOverviewQuery(r)
	fr := h.frame(r, "Overview", "Bench", "Overview")
	data, err := h.loadOverview(r.Context(), oq)
	if err != nil {
		h.logger.WithError(err).Error("failed to load the overview")
		http.Error(w, "failed to load the overview", http.StatusInternalServerError)
		return
	}

	base := "/"
	if r.URL.Path == "/workflows" {
		base = "/workflows"
		fr.page.Crumbs = []string{"Bench", "Workflows"}
	}
	data.Page = fr.page
	data.Base = base
	data.Query = oq
	for _, win := range overviewWindows {
		win := win
		data.Windows = append(data.Windows, chipLink{
			Label:  win.Key,
			Href:   oq.href(base, func(q *overviewQuery) { q.Window = win }),
			Active: win == oq.Window,
		})
	}
	for _, c := range []struct{ key, label string }{{"failing", "Failing"}, {"webhook", "Webhook"}} {
		c := c
		active := oq.Filter == c.key
		data.Chips = append(data.Chips, chipLink{
			Label:  c.label,
			Active: active,
			Href: oq.href(base, func(q *overviewQuery) {
				if active {
					q.Filter = ""
				} else {
					q.Filter = c.key
				}
			}),
		})
	}
	h.render(w, overviewTemplate, data)
}

// workflowTable serves the table alone, for the filter box's htmx swap.
func (h *uiHandler) workflowTable(w http.ResponseWriter, r *http.Request) {
	data, err := h.loadOverview(r.Context(), parseOverviewQuery(r))
	if err != nil {
		h.logger.WithError(err).Error("failed to load workflows")
		http.Error(w, "failed to load workflows", http.StatusInternalServerError)
		return
	}
	h.fragment(w, workflowTableTemplate, "wf-table", data.Table)
}

// loadOverview gathers everything the overview shows. Page frame fields are filled by the caller.
func (h *uiHandler) loadOverview(ctx context.Context, oq overviewQuery) (overviewPage, error) {
	now := h.now().UTC()
	workflows, err := h.store.ListWorkflows(ctx)
	if err != nil {
		return overviewPage{}, err
	}
	lastN, err := h.store.LastRunRecords(ctx, historyRuns)
	if err != nil {
		return overviewPage{}, err
	}
	runs, _, err := h.store.RunRecords(ctx, runFilter{Since: now.Add(-2 * oq.Window.Dur)}, overviewRunCap, "")
	if err != nil {
		return overviewPage{}, err
	}

	lasts := make(map[string]runRecord, len(lastN))
	for name, recent := range lastN {
		if len(recent) > 0 {
			lasts[name] = recent[0]
		}
	}
	var failedIDs []string
	for _, r := range lasts {
		if r.Failed() {
			failedIDs = append(failedIDs, r.RunID)
		}
	}
	failedSteps, err := h.store.FailedSteps(ctx, failedIDs)
	if err != nil {
		// Only the "failed at step N" wording is lost; the page is still right.
		h.logger.WithError(err).Warn("failed to look up failing steps")
		failedSteps = nil
	}

	byName := make(map[string]*workflowv1.Workflow, len(workflows))
	for _, wf := range workflows {
		byName[wf.GetName()] = wf
	}

	stats := computeOverviewStats(now, oq.Window, runs, lasts)
	var failed []failedLast
	for _, name := range stats.FailingWorkflows {
		last := lasts[name]
		failed = append(failed, failedLast{
			Workflow: name,
			Step:     stepPosition(byName[name], failedSteps[last.RunID]),
			At:       last.Started,
		})
	}

	windowRuns := make(map[string][]runRecord)
	start := now.Add(-oq.Window.Dur)
	for _, r := range runs {
		if !r.Started.Before(start) {
			windowRuns[r.Workflow] = append(windowRuns[r.Workflow], r)
		}
	}

	rows := buildOverviewRows(now, workflows, lastN, windowRuns, failedSteps)
	table := workflowTable{Rows: filterOverviewRows(rows, oq)}
	switch {
	case len(workflows) == 0:
		table.Empty = "No workflows yet."
	case len(table.Rows) == 0:
		table.Empty = "No workflow matches."
	}

	return overviewPage{
		Verdict:  computeVerdict(len(workflows), lasts, failed),
		Stats:    overviewTiles(stats, oq.Window, lasts),
		Table:    table,
		Workflow: len(workflows),
	}, nil
}

// stepPosition is "step 14" for the named step of wf, or "" when it cannot be placed.
func stepPosition(wf *workflowv1.Workflow, step string) string {
	if wf == nil || step == "" {
		return ""
	}
	for i, s := range wf.GetSteps() {
		if s.GetName() == step {
			return fmt.Sprintf("step %d", i+1)
		}
	}
	return ""
}

func overviewTiles(s overviewStats, win timeWindow, lasts map[string]runRecord) []draftui.Stat {
	pass := draftui.Stat{Label: "Pass rate · " + win.Key, Value: "—", Delta: "no finished runs"}
	if rate, ok := s.PassRate(); ok {
		pass.Value, pass.Unit = fmt.Sprintf("%.1f", rate), "%"
		pass.Delta = fmt.Sprintf("%d of %s", s.Passed, plural(s.Terminal, "run"))
		pass.Spark = s.PassSpark
		if rate < 100 {
			pass.DeltaGood = ""
		}
	}

	running := draftui.Stat{Label: "Running now", Value: fmt.Sprintf("%d", len(s.InFlight)), Delta: "nothing in flight"}
	if len(s.InFlight) > 0 {
		running.ValueTone = "ca"
		first := s.InFlight[0]
		running.Delta = first.Workflow
		if len(s.InFlight) > 1 {
			running.Delta += fmt.Sprintf(" +%d more", len(s.InFlight)-1)
		}
	}

	failing := draftui.Stat{Label: "Failing workflows", Value: fmt.Sprintf("%d", len(s.FailingWorkflows)), Delta: "none failing", DeltaGood: "good"}
	if len(s.FailingWorkflows) > 0 {
		failing.ValueTone = "err"
		failing.Delta = s.FailingWorkflows[0]
		if len(s.FailingWorkflows) > 1 {
			failing.Delta += fmt.Sprintf(" +%d more", len(s.FailingWorkflows)-1)
		}
		failing.DeltaGood = "bad"
	}

	median := draftui.Stat{Label: "Median run", Value: "—", Delta: "no finished runs"}
	if s.HasMedian {
		median.Value, median.Unit = durParts(s.Median)
		median.Delta = "no earlier runs to compare"
		if s.HasPrev {
			median.Delta, median.DeltaGood = deltaLabel(s.Median, s.PrevMedian, win.Key)
		}
	}
	return []draftui.Stat{pass, running, failing, median}
}

// durParts splits a duration into the tile's big number and its small unit ("1.4", "s").
func durParts(d time.Duration) (value, unit string) {
	switch {
	case d < time.Second:
		return fmt.Sprintf("%d", d.Milliseconds()), "ms"
	case d < time.Minute:
		return fmt.Sprintf("%.1f", d.Seconds()), "s"
	default:
		return dur(d), ""
	}
}

// buildOverviewRows makes one row per workflow, sorted by name. lastN holds each workflow's recent
// runs newest first; windowRuns its runs inside the selected window (for the p50).
func buildOverviewRows(now time.Time, workflows []*workflowv1.Workflow, lastN, windowRuns map[string][]runRecord, failedSteps map[string]string) []overviewRow {
	sorted := append([]*workflowv1.Workflow(nil), workflows...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].GetName() < sorted[j].GetName() })

	rows := make([]overviewRow, 0, len(sorted))
	for _, wf := range sorted {
		name := wf.GetName()
		row := overviewRow{
			Name:        name,
			Description: wf.GetDescription(),
			Steps:       len(wf.GetSteps()),
			History:     historyStrip(lastN[name], now, ""),
			P50:         "—",
		}
		if slug := wf.GetTrigger().GetWebhook().GetSlug(); slug != "" {
			row.Webhook = true
			row.Trigger = draftui.Tag{Tone: "ca", Text: "webhook", Title: "POST /webhooks/" + slug}
		} else {
			row.Trigger = draftui.Tag{Tone: "quiet", Text: "manual"}
		}
		var durs []time.Duration
		for _, r := range windowRuns[name] {
			if d, ok := r.Duration(); ok && (r.Passed() || r.Failed()) {
				durs = append(durs, d)
			}
		}
		if m, ok := median(durs); ok {
			row.P50 = dur(m)
		}
		if recent := lastN[name]; len(recent) > 0 {
			row.Last = lastRunCell(recent[0], wf, failedSteps, now)
			row.Running = recent[0].InFlight()
			row.Failing = recent[0].Failed()
		}
		rows = append(rows, row)
	}
	// The ones that need looking at come first — failing, then running — the rest by name, as the
	// mockup lists them.
	rank := func(r overviewRow) int {
		switch {
		case r.Failing:
			return 0
		case r.Running:
			return 1
		}
		return 2
	}
	sort.SliceStable(rows, func(i, j int) bool { return rank(rows[i]) < rank(rows[j]) })
	return rows
}

// lastRunCell is a workflow's "Last run": its status with the failing step, and when and how long.
func lastRunCell(r runRecord, wf *workflowv1.Workflow, failedSteps map[string]string, now time.Time) *lastRunView {
	v := &lastRunView{RunID: r.RunID}
	switch {
	case r.Passed():
		v.Status = draftui.StatusDot{Text: "Passed"}
	case r.Failed():
		text := "Failed"
		if pos := stepPosition(wf, failedSteps[r.RunID]); pos != "" {
			text += " · " + pos
		}
		v.Status = draftui.StatusDot{Kind: "err", Text: text}
	default:
		text := "Running"
		if !r.Started.IsZero() {
			text += " · " + dur(now.Sub(r.Started))
		}
		v.Status = draftui.StatusDot{Kind: "info", Live: true, Text: text}
	}
	note := clock(r.Started, now)
	if d, ok := r.Duration(); ok {
		note += " · " + dur(d)
	}
	v.Note = note
	return v
}

func filterOverviewRows(rows []overviewRow, oq overviewQuery) []overviewRow {
	needle := strings.ToLower(oq.Text)
	out := rows[:0:0]
	for _, row := range rows {
		if oq.Filter == "failing" && !row.Failing {
			continue
		}
		if oq.Filter == "webhook" && !row.Webhook {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(row.Name+" "+row.Description), needle) {
			continue
		}
		out = append(out, row)
	}
	return out
}

// --- recent runs -------------------------------------------------------------------------------

// runLine is one line of the runs table.
type runLine struct {
	RunID, Workflow, Started, StartedTitle, Duration string
	Status                                           draftui.StatusDot
}

// runsTable is the shared runs table (runs.html and the workflow detail page).
type runsTable struct {
	Rows []runLine
	// ShowWorkflow adds the Workflow column (off on a single workflow's own history).
	ShowWorkflow bool
	Empty        string
}

type runsPage struct {
	draftui.Page
	Filters  []chipLink
	Workflow string
	Table    runsTable
	// Older links to the next page; empty on the last one.
	Older string
}

func (h *uiHandler) runs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := q.Get("status")
	if status != "failed" && status != "passed" && status != "running" {
		status = ""
	}
	workflow := strings.TrimSpace(q.Get("workflow"))
	token := q.Get("page")

	records, next, err := h.store.RunRecords(r.Context(), runFilter{Workflow: workflow, Status: status}, runsPageSize, token)
	if err != nil {
		h.logger.WithError(err).Error("failed to list runs")
		http.Error(w, "failed to load runs", http.StatusInternalServerError)
		return
	}

	title := "Recent runs"
	if status == "failed" {
		title = "Failed runs"
	}
	fr := h.frame(r, title, "Bench", "Runs")
	data := runsPage{Page: fr.page, Workflow: workflow}
	data.Table = h.buildRunsTable(r.Context(), records, true)
	if workflow != "" {
		data.Table.Empty = fmt.Sprintf("No runs of %s match.", workflow)
	} else if status != "" {
		data.Table.Empty = "No " + status + " runs."
	} else {
		data.Table.Empty = "No runs yet. Run a workflow from the overview, or wait for its webhook to fire."
	}

	link := func(s string) string {
		v := url.Values{}
		if s != "" {
			v.Set("status", s)
		}
		if workflow != "" {
			v.Set("workflow", workflow)
		}
		if len(v) == 0 {
			return "/runs"
		}
		return "/runs?" + v.Encode()
	}
	for _, f := range []struct{ key, label string }{{"", "All"}, {"passed", "Passed"}, {"failed", "Failed"}, {"running", "Running"}} {
		data.Filters = append(data.Filters, chipLink{Label: f.label, Href: link(f.key), Active: f.key == status})
	}
	if next != "" {
		data.Older = link(status)
		sep := "?"
		if strings.Contains(data.Older, "?") {
			sep = "&"
		}
		data.Older += sep + "page=" + url.QueryEscape(next)
	}
	h.render(w, runsTemplate, data)
}

// buildRunsTable builds the shared table's rows, naming the failing step of failed runs.
func (h *uiHandler) buildRunsTable(ctx context.Context, records []runRecord, showWorkflow bool) runsTable {
	var failedIDs []string
	for _, r := range records {
		if r.Failed() {
			failedIDs = append(failedIDs, r.RunID)
		}
	}
	failedSteps, err := h.store.FailedSteps(ctx, failedIDs)
	if err != nil {
		h.logger.WithError(err).Warn("failed to look up failing steps")
		failedSteps = nil
	}
	now := h.now().UTC()
	t := runsTable{ShowWorkflow: showWorkflow}
	for _, r := range records {
		t.Rows = append(t.Rows, newRunRow(r, failedSteps[r.RunID], now))
	}
	return t
}

// Short is the run id trimmed for a table cell (ids are UUIDs); the full id is the link's title.
func (r runLine) Short() string {
	if len(r.RunID) > 13 {
		return r.RunID[:8] + "…"
	}
	return r.RunID
}

func newRunRow(r runRecord, failedStep string, now time.Time) runLine {
	row := runLine{RunID: r.RunID, Workflow: r.Workflow, Started: clock(r.Started, now), Duration: "—"}
	if !r.Started.IsZero() {
		row.StartedTitle = r.Started.UTC().Format(time.RFC3339)
	}
	if d, ok := r.Duration(); ok {
		row.Duration = dur(d)
	}
	switch {
	case r.Passed():
		row.Status = draftui.StatusDot{Text: "Passed"}
	case r.Failed():
		text := "Failed"
		if failedStep != "" {
			text += " · " + failedStep
		}
		row.Status = draftui.StatusDot{Kind: "err", Text: text}
	default:
		row.Status = draftui.StatusDot{Kind: "info", Live: true, Text: "Running"}
		if !r.Started.IsZero() {
			row.Duration = dur(now.Sub(r.Started))
		}
	}
	return row
}
