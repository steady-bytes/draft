// The run page: one run's step timeline, where it failed and what was sent and received there.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"
	draftui "github.com/steady-bytes/draft/tools/draft-ui"

	"google.golang.org/protobuf/types/known/structpb"
)

// groupThreshold is how many consecutive steps in the same quiet state (passed, skipped) it takes
// before the timeline folds them into one line.
const groupThreshold = 3

// stepRow is one step of the timeline.
type stepRow struct {
	No       int
	Name     string
	Uses     string
	State    string // ok, err, skip, run, pending
	Duration string
	// Elapsed is the same duration as a number (for a group's total); zero when unknown.
	Elapsed time.Duration
	Error   string
	// Request, Response and Detail are pretty-printed JSON, empty when the struct was not set (Request
	// is empty when templating failed before with: could be resolved).
	Request, Response, Detail string
}

func (s stepRow) Failed() bool { return s.State == "err" }

// HasIO is true when there is a request or a response to show beside a failure.
func (s stepRow) HasIO() bool { return s.Request != "" || s.Response != "" }

// HasDiagnostics is true when there is anything to fold out under a step that did not fail.
func (s stepRow) HasDiagnostics() bool { return s.Request != "" || s.Response != "" || s.Detail != "" }

// StateClass is the `d-step` modifier.
func (s stepRow) StateClass() string {
	switch s.State {
	case "err":
		return " is-fail"
	case "skip", "pending":
		return " is-skip"
	case "run":
		return " is-run"
	}
	return ""
}

// timelineItem is a step, or a folded run of steps.
type timelineItem struct {
	Group bool
	Step  stepRow
	// The fields below are set for a group.
	Steps    []stepRow
	Range    string // "03–12"
	Names    string // "wait-1 … call-6"
	Summary  string // "10 steps passed"
	Duration string
	Skipped  bool
}

// groupSteps folds runs of more than groupThreshold consecutive passed (or skipped) steps into one
// line each, so a 21-step run reads as the handful of steps that matter.
func groupSteps(steps []stepRow) []timelineItem {
	var out []timelineItem
	for i := 0; i < len(steps); {
		state := steps[i].State
		j := i
		for j < len(steps) && steps[j].State == state {
			j++
		}
		quiet := state == "ok" || state == "skip"
		if quiet && j-i > groupThreshold {
			out = append(out, newGroup(steps[i:j], state))
		} else {
			for _, s := range steps[i:j] {
				out = append(out, timelineItem{Step: s})
			}
		}
		i = j
	}
	return out
}

func newGroup(steps []stepRow, state string) timelineItem {
	first, last := steps[0], steps[len(steps)-1]
	verb := "passed"
	if state == "skip" {
		verb = "skipped"
	}
	g := timelineItem{
		Group:   true,
		Steps:   steps,
		Range:   fmt.Sprintf("%02d–%02d", first.No, last.No),
		Names:   first.Name + " … " + last.Name,
		Summary: fmt.Sprintf("%d steps %s", len(steps), verb),
		Skipped: state == "skip",
	}
	var total time.Duration
	for _, s := range steps {
		total += s.Elapsed
	}
	g.Duration = "—"
	if total > 0 {
		g.Duration = dur(total)
	}
	return g
}

func stepState(s workflowv1.StepStatus) string {
	switch s {
	case workflowv1.StepStatus_STEP_STATUS_PASSED:
		return "ok"
	case workflowv1.StepStatus_STEP_STATUS_FAILED:
		return "err"
	case workflowv1.StepStatus_STEP_STATUS_SKIPPED:
		return "skip"
	case workflowv1.StepStatus_STEP_STATUS_RUNNING:
		return "run"
	}
	return "pending"
}

// buildSteps lists the steps in the workflow's order, joining each with its result by name. A step
// with no result yet is pending; a result for a step the workflow no longer has follows the rest.
func buildSteps(run *workflowv1.Run, wf *workflowv1.Workflow, now time.Time) []stepRow {
	results := make(map[string]*workflowv1.StepResult, len(run.GetSteps()))
	for _, r := range run.GetSteps() {
		results[r.GetStepName()] = r
	}
	var rows []stepRow
	add := func(name, uses string) {
		row := stepRow{No: len(rows) + 1, Name: name, Uses: uses, State: "pending", Duration: "—"}
		if r, ok := results[name]; ok {
			row.State = stepState(r.GetStatus())
			row.Error = r.GetError()
			row.Request, row.Response, row.Detail = prettyJSON(r.GetRequest()), prettyJSON(r.GetResult()), prettyJSON(r.GetDetail())
			if r.GetStartedAt() != nil {
				if r.GetFinishedAt() != nil {
					row.Elapsed = r.GetFinishedAt().AsTime().Sub(r.GetStartedAt().AsTime())
					row.Duration = dur(row.Elapsed)
				} else if row.State == "run" {
					row.Duration = dur(now.Sub(r.GetStartedAt().AsTime()))
				}
			}
			delete(results, name)
		}
		rows = append(rows, row)
	}
	for _, s := range wf.GetSteps() {
		add(s.GetName(), s.GetUses())
	}
	// Whatever is left ran under a definition that has since changed; keep it, in its own order.
	for _, r := range run.GetSteps() {
		if _, left := results[r.GetStepName()]; left {
			add(r.GetStepName(), "")
		}
	}
	return rows
}

// prettyJSON renders a *structpb.Struct as indented JSON for display, or "" if it's unset —
// structpb.Struct.MarshalJSON produces compact JSON, so this goes through json.Indent.
func prettyJSON(s *structpb.Struct) string {
	if s == nil {
		return ""
	}
	compact, err := s.MarshalJSON()
	if err != nil {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, compact, "", "  "); err != nil {
		return ""
	}
	if buf.String() == "{}" {
		return ""
	}
	return buf.String()
}

func isTerminal(status workflowv1.RunStatus) bool {
	return status == workflowv1.RunStatus_RUN_STATUS_PASSED || status == workflowv1.RunStatus_RUN_STATUS_FAILED
}

type kvPair struct{ Key, Value, Href string }

// runFragment is the part of the run page htmx swaps while the run is in flight.
type runFragment struct {
	RunID, Workflow string
	Label           string
	Status          draftui.Tag
	Started         string
	Duration        string
	StepSummary     string
	Progress        draftui.Strip
	Timeline        []timelineItem
	History         draftui.Strip
	HistoryPassed   string
	LastPass        *kvPair
	Run             []kvPair
	// HasWorkflow is false when the workflow's definition has been deleted (no Definition or Re-run).
	HasWorkflow bool
	// Polling is true while the run has not reached a terminal status: the fragment carries an
	// hx-get that re-fetches it every couple of seconds. It stops on its own — once a response
	// reflects a terminal status, the swapped-in fragment carries no hx-trigger, so htmx has
	// nothing left to re-fire. This is a poll, not push: Catalyst event publishing (Phase 7) isn't
	// built yet, so there's no event stream to subscribe to instead.
	Polling  bool
	Finished string
	// Status pill for the out-of-band swap that keeps the topbar in step with the run.
	Pill *draftui.Status
}

type runPage struct {
	draftui.Page
	Fragment runFragment
}

func (h *uiHandler) loadRun(r *http.Request) (*runFragment, error) {
	ctx := r.Context()
	run, err := h.store.GetRun(ctx, r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	now := h.now().UTC()
	wf, wfErr := h.store.GetWorkflow(ctx, run.GetWorkflowName())
	if wfErr != nil && !errors.Is(wfErr, ErrWorkflowNotFound) {
		h.logger.WithError(wfErr).WithField("workflow", run.GetWorkflowName()).Warn("failed to load the workflow for a run page")
	}

	steps := buildSteps(run, wf, now)
	var ok, bad, skipped, pending int
	progress := draftui.Strip{Size: "stretch"}
	for _, s := range steps {
		cell := draftui.Cell{Title: fmt.Sprintf("%02d %s", s.No, s.Name)}
		switch s.State {
		case "ok":
			ok++
		case "err":
			bad++
			cell.State = "err"
		case "skip":
			skipped++
			cell.State = "skip"
		case "run":
			pending++
			cell.State = "run"
		default:
			pending++
			cell.State = "skip"
		}
		progress.Cells = append(progress.Cells, cell)
	}
	var summary []string
	for _, p := range []struct {
		n     int
		label string
	}{{ok, "passed"}, {bad, "failed"}, {skipped, "skipped"}, {pending, "pending"}} {
		if p.n > 0 {
			summary = append(summary, fmt.Sprintf("%d %s", p.n, p.label))
		}
	}
	progress.Label = "Step results: " + strings.Join(summary, ", ")

	started, finished := run.GetStartedAt().AsTime().UTC(), run.GetFinishedAt().AsTime().UTC()
	frag := &runFragment{
		RunID:       run.GetRunId(),
		Workflow:    run.GetWorkflowName(),
		Label:       "Run " + run.GetRunId(),
		Started:     clockSeconds(started),
		Duration:    "running…",
		StepSummary: strings.Join(summary, " · "),
		Progress:    progress,
		Timeline:    groupSteps(steps),
		HasWorkflow: wfErr == nil,
		Polling:     !isTerminal(run.GetStatus()),
		Finished:    "—",
	}
	if run.GetStartedAt() == nil {
		frag.Started = "—"
	}
	if run.GetFinishedAt() != nil {
		frag.Duration = dur(finished.Sub(started))
		frag.Finished = clockSeconds(finished)
	}
	switch run.GetStatus() {
	case workflowv1.RunStatus_RUN_STATUS_PASSED:
		frag.Status = draftui.Tag{Tone: "primary", Text: "Passed", Solid: true}
	case workflowv1.RunStatus_RUN_STATUS_FAILED:
		frag.Status = draftui.Tag{Tone: "err", Text: "Failed", Solid: true}
	default:
		frag.Status = draftui.Tag{Tone: "ca", Text: "Running", Solid: true}
	}

	recent, _, err := h.store.RunRecords(ctx, runFilter{Workflow: run.GetWorkflowName()}, historyRuns, "")
	if err != nil {
		h.logger.WithError(err).Warn("failed to load the workflow's recent runs for a run page")
	} else {
		frag.History = historyStrip(recent, now, run.GetRunId())
		frag.History.Size = "tall"
		passed := 0
		for _, rec := range recent {
			if rec.Passed() {
				passed++
			}
			if frag.LastPass == nil && rec.Passed() && rec.RunID != run.GetRunId() {
				text := rec.RunID
				if d, ok := rec.Duration(); ok {
					text += " took " + dur(d)
				}
				frag.LastPass = &kvPair{Key: "Last passing run", Value: text, Href: "/runs/" + rec.RunID}
			}
		}
		frag.HistoryPassed = fmt.Sprintf("%d / %d passed", passed, len(recent))
	}

	frag.Run = []kvPair{
		{Key: "run id", Value: run.GetRunId()},
		{Key: "workflow", Value: run.GetWorkflowName(), Href: "/workflows/" + run.GetWorkflowName()},
		{Key: "started", Value: frag.Started},
		{Key: "finished", Value: frag.Finished},
	}
	if !frag.HasWorkflow {
		frag.Run[1].Href = ""
	}

	if counts, err := h.store.RunCounts(ctx, now.Add(-countsWindow)); err == nil {
		frag.Pill = inFlightStatus(counts.InFlight)
	}
	return frag, nil
}

func (h *uiHandler) runDetail(w http.ResponseWriter, r *http.Request) {
	frag, err := h.loadRun(r)
	if err != nil {
		if errors.Is(err, ErrRunNotFound) {
			h.notFound(w, r, fmt.Sprintf("No run with id %q.", r.PathValue("id")))
			return
		}
		h.logger.WithError(err).Error("failed to load a run")
		http.Error(w, "failed to load run", http.StatusInternalServerError)
		return
	}
	fr := h.frame(r, frag.Workflow+" · "+frag.RunID, "Bench", frag.Workflow, frag.RunID)
	polling := "off · run finished"
	if frag.Polling {
		polling = "on · every 2s"
	}
	fr.page.Left = []draftui.BarItem{draftui.KV("Run", frag.RunID), draftui.KVID("run-finished", "Finished", frag.Finished)}
	fr.page.Right = []draftui.BarItem{draftui.KVID("run-polling", "Polling", polling)}
	h.render(w, runTemplate, runPage{Page: fr.page, Fragment: *frag})
}

// runDetailFragment serves the same content without the page frame, for the htmx auto-refresh on
// runDetail's own page (see runFragment.Polling) to swap into itself every couple of seconds.
func (h *uiHandler) runDetailFragment(w http.ResponseWriter, r *http.Request) {
	frag, err := h.loadRun(r)
	if err != nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	h.fragment(w, runFragmentTemplate, "run-response", frag)
}

// triggerRun is the UI's "run now" button — the same StartRun path TriggerRun (rpc.go) and the
// webhook (webhook.go) use, just invoked by a plain form POST instead of an RPC or a signed webhook
// call.
func (h *uiHandler) triggerRun(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	wf, err := h.store.GetWorkflow(r.Context(), name)
	if err != nil {
		h.notFound(w, r, fmt.Sprintf("No workflow named %q is loaded.", name))
		return
	}
	run, err := h.scheduler.StartRun(r.Context(), wf)
	if err != nil {
		h.logger.WithError(err).WithField("workflow", name).Error("failed to start run from ui")
		http.Error(w, "failed to start run", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/runs/"+run.GetRunId(), http.StatusSeeOther)
}
