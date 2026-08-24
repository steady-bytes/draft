// This file is Bench's Phase 8 (The UI): the Dashboard, Workflow list, Workflow
// detail, and Run detail pages from docs/website/content/docs/architecture/
// bench-workflow-engine.md's "The UI" section, sharing services/tooling/garage's
// "workshop" DaisyUI theme and html/template + htmx stack (see garage's base.html
// for the CDN-loading rationale, duplicated verbatim in templates/base.html here).
//
// Phase 10 (see the doc's "Authoring workflows without a file") adds the
// New/Edit/Delete workflow handlers below: workflow_write.go's
// createWorkflow/updateWorkflow are the same functions rpc.go's
// CreateWorkflow/UpdateWorkflow RPCs call, so this UI and the RPC surface
// can't drift on what "create"/"update" mean — this file only adds the
// HTML-form transport around them.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"time"

	plugincatalogv1 "github.com/steady-bytes/draft/api/tooling/plugin_catalog/v1"
	plugincatalogv1connect "github.com/steady-bytes/draft/api/tooling/plugin_catalog/v1/v1connect"
	settingsv1 "github.com/steady-bytes/draft/api/tooling/settings/v1"
	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gopkg.in/yaml.v3"
)

const (
	// dashboardRunSample and workflowRunHistorySize both cap how many runs a
	// page pulls back to compute its view — reasonable for a run history
	// expected to stay small for a long time (the same scale assumption
	// services/tooling/garage's store.go documents for its own catalog),
	// not tuned against any real load. Revisit if either page's query ever
	// shows up as a real cost.
	dashboardRunSample     = 200
	workflowRunHistorySize = 25
)

type uiHandler struct {
	logger     chassis.Logger
	store      *pgResultStore
	scheduler  workflowRunner
	httpClient connect.HTTPClient
}

// NewUIHandler builds Bench's HTML page handler. store and scheduler are the
// same instances main.go already constructed for the RPC/webhook handlers —
// the UI reads/writes through them directly rather than looping back through
// its own Connect RPC surface over the network, the same choice
// services/tooling/garage/ui.go makes for its own store. httpClient is used
// by the Settings page's live registry-reachability check
// (registry_write.go) and the search sidebar's federated Garage queries
// (Phase 11).
func NewUIHandler(logger chassis.Logger, store *pgResultStore, scheduler workflowRunner, httpClient connect.HTTPClient) chassis.RPCRegistrar {
	return &uiHandler{logger: logger, store: store, scheduler: scheduler, httpClient: httpClient}
}

func (h *uiHandler) RegisterRPC(server chassis.Rpcer) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", h.dashboard)
	mux.HandleFunc("GET /workflows", h.workflowList)
	mux.HandleFunc("GET /workflows/new", h.workflowNew)
	mux.HandleFunc("POST /workflows", h.createWorkflow)
	mux.HandleFunc("GET /workflows/plugins/search", h.pluginSearch)
	mux.HandleFunc("GET /workflows/{name}", h.workflowDetail)
	mux.HandleFunc("GET /workflows/{name}/edit", h.workflowEdit)
	mux.HandleFunc("POST /workflows/{name}", h.updateWorkflow)
	mux.HandleFunc("POST /workflows/{name}/delete", h.deleteWorkflow)
	mux.HandleFunc("POST /workflows/{name}/run", h.triggerRun)
	mux.HandleFunc("GET /runs/{id}", h.runDetail)
	mux.HandleFunc("GET /partials/runs/{id}", h.runDetailFragment)
	mux.HandleFunc("GET /settings", h.settings)
	mux.HandleFunc("POST /settings", h.addRegistry)
	mux.HandleFunc("POST /settings/{name}/delete", h.deleteRegistry)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticFS)))

	// enableReflection is false for the same reason garage/ui.go's is: these
	// are plain HTML/static routes, not a Connect/gRPC service.
	server.AddHandler("/", mux, false)
}

// --- view models -------------------------------------------------------------

// runView is the shared per-run row shape used by the dashboard, workflow
// list, and workflow detail pages — everywhere a run shows up as one line in
// a list rather than the full run-detail page.
type runView struct {
	RunID        string
	WorkflowName string
	Status       string // human label, e.g. "passed"
	SignalClass  string
	StartedAt    string
	Duration     string
}

func newRunView(r *workflowv1.Run) runView {
	return runView{
		RunID:        r.GetRunId(),
		WorkflowName: r.GetWorkflowName(),
		Status:       statusLabel(r.GetStatus().String()),
		SignalClass:  statusSignalClass(r.GetStatus().String()),
		StartedAt:    formatTime(r.GetStartedAt()),
		Duration:     formatDuration(r.GetStartedAt(), r.GetFinishedAt()),
	}
}

func statusLabel(raw string) string {
	raw = strings.TrimPrefix(raw, "RUN_STATUS_")
	raw = strings.TrimPrefix(raw, "STEP_STATUS_")
	return strings.ToLower(raw)
}

func statusSignalClass(raw string) string {
	switch {
	case strings.HasSuffix(raw, "_PASSED"):
		return "signal-success"
	case strings.HasSuffix(raw, "_RUNNING"), strings.HasSuffix(raw, "_PENDING"):
		return "signal-warning"
	case strings.HasSuffix(raw, "_FAILED"):
		return "signal-error"
	default: // _SKIPPED, _UNSPECIFIED
		return "signal-idle"
	}
}

func isTerminal(status workflowv1.RunStatus) bool {
	return status == workflowv1.RunStatus_RUN_STATUS_PASSED || status == workflowv1.RunStatus_RUN_STATUS_FAILED
}

func formatTime(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return "—"
	}
	return ts.AsTime().Local().Format("Jan 2 15:04:05")
}

func formatDuration(start, finish *timestamppb.Timestamp) string {
	if start == nil {
		return "—"
	}
	if finish == nil {
		return "running…"
	}
	d := finish.AsTime().Sub(start.AsTime())
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	default:
		return d.Round(time.Second).String()
	}
}

// --- dashboard -----------------------------------------------------------

type dashboardPageData struct {
	Nav            string
	WorkflowCount  int
	TotalRuns      int
	PassRatePct    string
	RunningCount   int
	RecentFailures []runView
	RecentRuns     []runView
}

func (h *uiHandler) dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	workflows, err := h.store.ListWorkflows(ctx)
	if err != nil {
		h.logger.WithError(err).Error("failed to list workflows for dashboard")
		http.Error(w, "failed to load dashboard", http.StatusInternalServerError)
		return
	}

	runs, _, err := h.store.ListRuns(ctx, "", dashboardRunSample, "")
	if err != nil {
		h.logger.WithError(err).Error("failed to list runs for dashboard")
		http.Error(w, "failed to load dashboard", http.StatusInternalServerError)
		return
	}

	var (
		passed, failed, running int
		recentFailures          []runView
	)
	for _, run := range runs {
		switch run.GetStatus() {
		case workflowv1.RunStatus_RUN_STATUS_PASSED:
			passed++
		case workflowv1.RunStatus_RUN_STATUS_FAILED:
			failed++
			if len(recentFailures) < 5 {
				recentFailures = append(recentFailures, newRunView(run))
			}
		case workflowv1.RunStatus_RUN_STATUS_RUNNING, workflowv1.RunStatus_RUN_STATUS_PENDING:
			running++
		}
	}

	passRate := "—"
	if terminal := passed + failed; terminal > 0 {
		passRate = fmt.Sprintf("%.0f%%", float64(passed)/float64(terminal)*100)
	}

	recent := make([]runView, 0, 10)
	for i, run := range runs {
		if i >= 10 {
			break
		}
		recent = append(recent, newRunView(run))
	}

	h.render(w, dashboardTemplate, "base", dashboardPageData{
		Nav:            "dashboard",
		WorkflowCount:  len(workflows),
		TotalRuns:      len(runs),
		PassRatePct:    passRate,
		RunningCount:   running,
		RecentFailures: recentFailures,
		RecentRuns:     recent,
	})
}

// --- workflow list ---------------------------------------------------------

type workflowListItem struct {
	Name        string
	Description string
	WebhookSlug string // empty if the workflow has no webhook trigger
	StepCount   int
	LastRun     *runView // nil if the workflow has never run
}

type workflowListPageData struct {
	Nav       string
	Workflows []workflowListItem
}

func (h *uiHandler) workflowList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	workflows, err := h.store.ListWorkflows(ctx)
	if err != nil {
		h.logger.WithError(err).Error("failed to list workflows")
		http.Error(w, "failed to load workflows", http.StatusInternalServerError)
		return
	}
	sort.Slice(workflows, func(i, j int) bool { return workflows[i].GetName() < workflows[j].GetName() })

	items := make([]workflowListItem, 0, len(workflows))
	for _, wf := range workflows {
		item := workflowListItem{
			Name:        wf.GetName(),
			Description: wf.GetDescription(),
			WebhookSlug: wf.GetTrigger().GetWebhook().GetSlug(),
			StepCount:   len(wf.GetSteps()),
		}
		// One ListRuns call per workflow, matching services/tooling/garage's
		// listVersions-per-name pattern in its own list views — acceptable
		// for the same "stays small for a long time" reason, not something
		// worth a dedicated aggregate query yet.
		runs, _, err := h.store.ListRuns(ctx, wf.GetName(), 1, "")
		if err != nil {
			h.logger.WithError(err).WithField("workflow", wf.GetName()).Warn("failed to load last run for workflow list")
		} else if len(runs) > 0 {
			rv := newRunView(runs[0])
			item.LastRun = &rv
		}
		items = append(items, item)
	}

	h.render(w, workflowListTemplate, "base", workflowListPageData{Nav: "workflows", Workflows: items})
}

// --- new / edit workflow ---------------------------------------------------

// workflowFormPageData backs both the New and Edit workflow pages — one
// template, since the only real difference is whether an existing name is
// being replaced (IsEdit/Name) and where the form posts to (FormAction).
type workflowFormPageData struct {
	Nav        string
	IsEdit     bool
	Name       string // the workflow being edited; empty on the New page
	YAML       string // pre-filled content: blank (new), reconstructed (edit), or the just-submitted body that failed validation
	Error      string
	FormAction string
}

// defaultWorkflowTemplate pre-fills the New-workflow textarea so a first-time
// author has a real, structurally valid starting point to edit rather than a
// blank box — passes loader.go's Validate as-is (it only checks shape:
// required fields, a recognized uses: scheme, an acyclic depends_on graph),
// though running it will fail until service/method are pointed at something
// real, same as the placeholder text this replaces already implied.
const defaultWorkflowTemplate = `apiVersion: bench/v1
kind: Workflow
metadata:
  name: my-workflow
  description: ""
steps:
  - name: step-a
    uses: bench://grpc-call@v1
    with:
      service: my.service.v1.Thing
      method: DoSomething
    expect:
      status: OK
`

func (h *uiHandler) workflowNew(w http.ResponseWriter, r *http.Request) {
	h.render(w, workflowFormTemplate, "base", workflowFormPageData{
		Nav:        "workflows",
		YAML:       defaultWorkflowTemplate,
		FormAction: "/workflows",
	})
}

func (h *uiHandler) workflowEdit(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := r.Context()

	wf, err := h.store.GetWorkflow(ctx, name)
	if err != nil {
		h.renderNotFound(w, fmt.Sprintf("No workflow named %q is loaded.", name))
		return
	}

	yamlSrc, err := renderWorkflowYAML(wf)
	if err != nil {
		h.logger.WithError(err).WithField("workflow", name).Error("failed to render workflow yaml for edit form")
		yamlSrc = "# failed to render this workflow's definition"
	}

	h.render(w, workflowFormTemplate, "base", workflowFormPageData{
		Nav:        "workflows",
		IsEdit:     true,
		Name:       name,
		YAML:       yamlSrc,
		FormAction: "/workflows/" + name,
	})
}

// createWorkflow handles the New-workflow form POST. On a validation
// failure (bad YAML, or the name is already taken), the same form
// re-renders with the submitted text preserved and workflow_write.go's
// error shown, rather than losing what was typed — matching the design
// doc's "Authoring workflows without a file" section.
func (h *uiHandler) createWorkflow(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}
	yamlDoc := r.FormValue("yaml")

	wf, err := createWorkflow(r.Context(), h.store, yamlDoc)
	if err != nil {
		h.render(w, workflowFormTemplate, "base", workflowFormPageData{
			Nav:        "workflows",
			YAML:       yamlDoc,
			Error:      err.Error(),
			FormAction: "/workflows",
		})
		return
	}

	http.Redirect(w, r, "/workflows/"+wf.GetName(), http.StatusSeeOther)
}

// updateWorkflow handles the Edit-workflow form POST — see createWorkflow's
// comment; the same re-render-with-error behavior on a validation failure,
// keyed off the workflow being edited rather than a blank form.
func (h *uiHandler) updateWorkflow(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}
	yamlDoc := r.FormValue("yaml")

	wf, err := updateWorkflow(r.Context(), h.store, name, yamlDoc)
	if err != nil {
		if errors.Is(err, ErrWorkflowNotFound) {
			h.renderNotFound(w, fmt.Sprintf("No workflow named %q is loaded.", name))
			return
		}
		h.render(w, workflowFormTemplate, "base", workflowFormPageData{
			Nav:        "workflows",
			IsEdit:     true,
			Name:       name,
			YAML:       yamlDoc,
			Error:      err.Error(),
			FormAction: "/workflows/" + name,
		})
		return
	}

	http.Redirect(w, r, "/workflows/"+wf.GetName(), http.StatusSeeOther)
}

// deleteWorkflow handles the Delete button on the workflow detail page. Run
// history for name is untouched (store.go's DeleteWorkflow) — only the
// definition goes away.
func (h *uiHandler) deleteWorkflow(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if err := h.store.DeleteWorkflow(r.Context(), name); err != nil && !errors.Is(err, ErrWorkflowNotFound) {
		h.logger.WithError(err).WithField("workflow", name).Error("failed to delete workflow")
		http.Error(w, "failed to delete workflow", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/workflows", http.StatusSeeOther)
}

// --- workflow detail ---------------------------------------------------------

type workflowDetailPageData struct {
	Nav         string
	Name        string
	Description string
	WebhookSlug string
	YAML        string
	Runs        []runView
}

func (h *uiHandler) workflowDetail(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := r.Context()

	wf, err := h.store.GetWorkflow(ctx, name)
	if err != nil {
		h.renderNotFound(w, fmt.Sprintf("No workflow named %q is loaded.", name))
		return
	}

	yamlSrc, err := renderWorkflowYAML(wf)
	if err != nil {
		h.logger.WithError(err).WithField("workflow", name).Error("failed to render workflow yaml")
		yamlSrc = "# failed to render this workflow's definition"
	}

	runs, _, err := h.store.ListRuns(ctx, name, workflowRunHistorySize, "")
	if err != nil {
		h.logger.WithError(err).WithField("workflow", name).Error("failed to list runs for workflow detail")
		http.Error(w, "failed to load workflow", http.StatusInternalServerError)
		return
	}
	runViews := make([]runView, 0, len(runs))
	for _, run := range runs {
		runViews = append(runViews, newRunView(run))
	}

	h.render(w, workflowDetailTemplate, "base", workflowDetailPageData{
		Nav:         "workflows",
		Name:        wf.GetName(),
		Description: wf.GetDescription(),
		WebhookSlug: wf.GetTrigger().GetWebhook().GetSlug(),
		YAML:        yamlSrc,
		Runs:        runViews,
	})
}

// triggerRun is the UI's "run now" button — the same StartRun path
// TriggerRun (rpc.go) and the webhook (webhook.go) use, just invoked by a
// plain form POST instead of an RPC or a signed webhook call.
func (h *uiHandler) triggerRun(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := r.Context()

	wf, err := h.store.GetWorkflow(ctx, name)
	if err != nil {
		h.renderNotFound(w, fmt.Sprintf("No workflow named %q is loaded.", name))
		return
	}

	run, err := h.scheduler.StartRun(ctx, wf)
	if err != nil {
		h.logger.WithError(err).WithField("workflow", name).Error("failed to start run from ui")
		http.Error(w, "failed to start run", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/runs/"+run.GetRunId(), http.StatusSeeOther)
}

// renderWorkflowYAML reconstructs a readable YAML view of a persisted
// Workflow. This is not the literal file bytes Bench originally loaded --
// nothing stores those, only the parsed-and-reserialized definition (see
// model.go's workflowRow.Definition) -- so what's shown is a semantically
// equivalent re-serialization, not guaranteed byte-identical to the source
// file. structpb.Struct.AsMap() converts a step's with:/expect: back to
// plain Go values yaml.Marshal already knows how to encode.
func renderWorkflowYAML(wf *workflowv1.Workflow) (string, error) {
	type yamlTriggerWebhookSignature struct {
		Header    string `yaml:"header,omitempty"`
		SecretRef string `yaml:"secret_ref,omitempty"`
	}
	type yamlTriggerWebhook struct {
		Slug      string                       `yaml:"slug"`
		Signature *yamlTriggerWebhookSignature `yaml:"signature,omitempty"`
	}
	type yamlTrigger struct {
		Webhook *yamlTriggerWebhook `yaml:"webhook,omitempty"`
	}
	type yamlStep struct {
		Name      string                 `yaml:"name"`
		Uses      string                 `yaml:"uses"`
		With      map[string]interface{} `yaml:"with,omitempty"`
		Expect    map[string]interface{} `yaml:"expect,omitempty"`
		DependsOn []string               `yaml:"depends_on,omitempty"`
		OnFailure string                 `yaml:"on_failure,omitempty"`
	}
	type yamlDoc struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
		Metadata   struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description,omitempty"`
		} `yaml:"metadata"`
		Trigger *yamlTrigger `yaml:"trigger,omitempty"`
		Steps   []yamlStep   `yaml:"steps"`
	}

	doc := yamlDoc{APIVersion: "bench/v1", Kind: "Workflow"}
	doc.Metadata.Name = wf.GetName()
	doc.Metadata.Description = wf.GetDescription()

	if webhook := wf.GetTrigger().GetWebhook(); webhook != nil {
		yw := &yamlTriggerWebhook{Slug: webhook.GetSlug()}
		if webhook.GetSignatureHeader() != "" || webhook.GetSecretRef() != "" {
			yw.Signature = &yamlTriggerWebhookSignature{
				Header:    webhook.GetSignatureHeader(),
				SecretRef: webhook.GetSecretRef(),
			}
		}
		doc.Trigger = &yamlTrigger{Webhook: yw}
	}

	for _, s := range wf.GetSteps() {
		ys := yamlStep{
			Name:      s.GetName(),
			Uses:      s.GetUses(),
			DependsOn: s.GetDependsOn(),
		}
		if s.GetWith() != nil {
			ys.With = s.GetWith().AsMap()
		}
		if s.GetExpect() != nil {
			ys.Expect = s.GetExpect().AsMap()
		}
		if s.GetOnFailure() != workflowv1.FailurePolicy_FAILURE_POLICY_UNSPECIFIED {
			label := strings.ToLower(strings.TrimPrefix(s.GetOnFailure().String(), "FAILURE_POLICY_"))
			if label != "fail" { // fail is the default; omit it for a cleaner snippet
				ys.OnFailure = label
			}
		}
		doc.Steps = append(doc.Steps, ys)
	}

	out, err := yaml.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// --- run detail ---------------------------------------------------------

type stepView struct {
	Name        string
	Status      string
	SignalClass string
	Duration    string
	Error       string
	// RequestJSON/ResponseJSON/DetailJSON are pretty-printed JSON, empty
	// when the corresponding struct wasn't set (e.g. RequestJSON is empty
	// when templating failed before with: could be resolved). Rendered in
	// a collapsible section per step so troubleshooting a failed step
	// doesn't require leaving the run-detail page — see
	// templates/run_detail.html.
	RequestJSON  string
	ResponseJSON string
	DetailJSON   string
	// HasDiagnostics is true when there's anything to show in the
	// collapsible section at all, so the template can skip rendering an
	// empty, useless collapse control for steps that never got far enough
	// to produce any of the three (e.g. still pending).
	HasDiagnostics bool
}

// prettyJSON renders a *structpb.Struct as indented JSON for display, or ""
// if it's unset -- structpb.Struct.MarshalJSON produces compact JSON, so
// this goes through json.Indent rather than relying on that directly.
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
	return buf.String()
}

type runDetailPageData struct {
	Nav          string
	RunID        string
	WorkflowName string
	Status       string
	SignalClass  string
	StartedAt    string
	Duration     string
	Steps        []stepView
	// Polling is true while the run hasn't reached a terminal status --
	// the page wraps itself in an htmx auto-refresh so a run started from
	// the UI (or a webhook) visibly progresses without a manual reload.
	// This is a poll, not push: Catalyst event publishing (Phase 7) isn't
	// built yet, so there's no event stream to subscribe to instead.
	Polling bool
}

func newRunDetailPageData(run *workflowv1.Run) runDetailPageData {
	data := runDetailPageData{
		Nav:          "workflows",
		RunID:        run.GetRunId(),
		WorkflowName: run.GetWorkflowName(),
		Status:       statusLabel(run.GetStatus().String()),
		SignalClass:  statusSignalClass(run.GetStatus().String()),
		StartedAt:    formatTime(run.GetStartedAt()),
		Duration:     formatDuration(run.GetStartedAt(), run.GetFinishedAt()),
		Polling:      !isTerminal(run.GetStatus()),
	}
	for _, s := range run.GetSteps() {
		requestJSON := prettyJSON(s.GetRequest())
		responseJSON := prettyJSON(s.GetResult())
		detailJSON := prettyJSON(s.GetDetail())
		data.Steps = append(data.Steps, stepView{
			Name:           s.GetStepName(),
			Status:         statusLabel(s.GetStatus().String()),
			SignalClass:    statusSignalClass(s.GetStatus().String()),
			Duration:       formatDuration(s.GetStartedAt(), s.GetFinishedAt()),
			Error:          s.GetError(),
			RequestJSON:    requestJSON,
			ResponseJSON:   responseJSON,
			DetailJSON:     detailJSON,
			HasDiagnostics: requestJSON != "" || responseJSON != "" || detailJSON != "",
		})
	}
	return data
}

func (h *uiHandler) runDetail(w http.ResponseWriter, r *http.Request) {
	run, err := h.store.GetRun(r.Context(), r.PathValue("id"))
	if err != nil {
		h.renderNotFound(w, fmt.Sprintf("No run with id %q.", r.PathValue("id")))
		return
	}
	h.render(w, runDetailTemplate, "base", newRunDetailPageData(run))
}

// runDetailFragment serves the same run-detail content without the page
// chrome, for the htmx auto-refresh on runDetail's own page (see
// runDetailPageData.Polling and templates/run_detail.html's hx-get) to swap
// into itself every couple of seconds while a run is in flight.
func (h *uiHandler) runDetailFragment(w http.ResponseWriter, r *http.Request) {
	run, err := h.store.GetRun(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	h.render(w, runDetailFragmentTemplate, "run-detail-fragment", newRunDetailPageData(run))
}

// --- settings (Phase 11) ---------------------------------------------------

type settingsPageData struct {
	Nav         string
	Registries  []*settingsv1.PluginRegistry
	Error       string
	FormName    string // preserved on a failed Add, so the form doesn't lose what was typed
	FormAddress string
}

func (h *uiHandler) settings(w http.ResponseWriter, r *http.Request) {
	registries, err := h.store.ListPluginRegistries(r.Context())
	if err != nil {
		h.logger.WithError(err).Error("failed to list plugin registries")
		http.Error(w, "failed to load settings", http.StatusInternalServerError)
		return
	}
	h.render(w, settingsTemplate, "base", settingsPageData{Nav: "settings", Registries: registries})
}

// addRegistry handles the Settings page's Add-registry form POST. On a
// validation failure (missing fields, or address doesn't answer as a
// garage-compatible registry — see registry_write.go's checkRegistryReachable),
// the page re-renders with the submitted name/address preserved and the
// error shown, the same "server does the real work, re-render with an
// error" shape createWorkflow/updateWorkflow already established.
func (h *uiHandler) addRegistry(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	address := strings.TrimSpace(r.FormValue("address"))

	if _, err := addPluginRegistry(r.Context(), h.store, h.httpClient, name, address); err != nil {
		registries, listErr := h.store.ListPluginRegistries(r.Context())
		if listErr != nil {
			h.logger.WithError(listErr).Error("failed to list plugin registries while re-rendering settings form error")
		}
		h.render(w, settingsTemplate, "base", settingsPageData{
			Nav:         "settings",
			Registries:  registries,
			Error:       err.Error(),
			FormName:    name,
			FormAddress: address,
		})
		return
	}

	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (h *uiHandler) deleteRegistry(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.store.DeletePluginRegistry(r.Context(), name); err != nil && !errors.Is(err, ErrPluginRegistryNotFound) {
		h.logger.WithError(err).WithField("registry", name).Error("failed to delete plugin registry")
		http.Error(w, "failed to delete plugin registry", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

// --- plugin search sidebar (Phase 11) ---------------------------------------

// pluginResultView is one search result card: the plugin itself plus which
// registry it came from (see the design doc's "Federated search, first-match
// execution" — a human picking a result needs to see the source even though
// garage:// resolution picks silently) and a ready-to-paste step snippet.
type pluginResultView struct {
	Name         string
	Version      string
	Description  string
	RegistryName string
	Snippet      string
}

type pluginSearchPageData struct {
	Query        string
	Results      []pluginResultView
	NoRegistries bool
	Error        string
}

// pluginSearch is the htmx target behind the New/Edit workflow page's search
// sidebar: federates Search (or List, for an empty query) across every
// configured registry and merges the results — see the design doc for why
// this needs no new Bench-side RPC, it's just Bench calling Garage's already
// -real PluginCatalogService like garage_plugin.go already does for
// execution.
func (h *uiHandler) pluginSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := strings.TrimSpace(r.URL.Query().Get("q"))

	registries, err := h.store.ListPluginRegistries(ctx)
	if err != nil {
		h.logger.WithError(err).Error("failed to list plugin registries for search")
		h.render(w, pluginSearchTemplate, "plugin-search-results", pluginSearchPageData{Query: query, Error: "failed to load registries"})
		return
	}
	if len(registries) == 0 {
		h.render(w, pluginSearchTemplate, "plugin-search-results", pluginSearchPageData{Query: query, NoRegistries: true})
		return
	}

	var results []pluginResultView
	for _, reg := range registries {
		client := plugincatalogv1connect.NewPluginCatalogServiceClient(h.httpClient, reg.GetAddress(), connect.WithGRPC())
		var plugins []*plugincatalogv1.Plugin
		if query == "" {
			resp, err := client.List(ctx, connect.NewRequest(&plugincatalogv1.ListPluginsRequest{PageSize: 50}))
			if err != nil {
				h.logger.WithError(err).WithField("registry", reg.GetName()).Warn("failed to list plugins from registry for search sidebar")
				continue
			}
			plugins = resp.Msg.GetPlugins()
		} else {
			resp, err := client.Search(ctx, connect.NewRequest(&plugincatalogv1.SearchPluginsRequest{Query: query}))
			if err != nil {
				h.logger.WithError(err).WithField("registry", reg.GetName()).Warn("failed to search plugins from registry for search sidebar")
				continue
			}
			plugins = resp.Msg.GetPlugins()
		}
		for _, p := range plugins {
			results = append(results, pluginResultView{
				Name:         p.GetName(),
				Version:      p.GetVersion(),
				Description:  p.GetDescription(),
				RegistryName: reg.GetName(),
				Snippet:      pluginUsesSnippet(p),
			})
		}
	}

	h.render(w, pluginSearchTemplate, "plugin-search-results", pluginSearchPageData{Query: query, Results: results})
}

// pluginUsesSnippet builds a ready-to-paste step block from a plugin's
// published config_schema -- Bench's own small reimplementation of Garage's
// own private usesSnippet (services/tooling/garage/ui.go), not a
// cross-service call; see the design doc's "The snippet itself is a small,
// deliberate duplication" for why.
// pluginUsesSnippet builds a step block indented to match a `steps:` list
// item everywhere else in this codebase renders one: a 2-space-indented
// "- name:" line, its fields at 4 spaces, and (when present) with:'s own
// fields at 6 -- see renderWorkflowYAML's reconstructed YAML and
// defaultWorkflowTemplate's own steps: entry for the same convention. Not
// column 0: appendStep (workflow_form.html) appends this directly under an
// existing steps: list, so it has to nest as a sibling of whatever's
// already there, not a top-level block.
func pluginUsesSnippet(p *plugincatalogv1.Plugin) string {
	var b strings.Builder
	fmt.Fprintf(&b, "  - name: use-%s\n", p.GetName())
	fmt.Fprintf(&b, "    uses: garage://%s@%s\n", p.GetName(), p.GetVersion())

	schema := p.GetConfigSchema().AsMap()
	required, _ := schema["required"].([]interface{})
	properties, _ := schema["properties"].(map[string]interface{})

	if len(required) == 0 {
		b.WriteString("    with: {}\n")
		return b.String()
	}
	b.WriteString("    with:\n")
	for _, f := range required {
		field, ok := f.(string)
		if !ok {
			continue
		}
		fieldType := "string"
		if props, ok := properties[field].(map[string]interface{}); ok {
			if t, ok := props["type"].(string); ok {
				fieldType = t
			}
		}
		fmt.Fprintf(&b, "      %s: <%s>\n", field, fieldType)
	}
	return b.String()
}

// --- shared rendering -----------------------------------------------------

func (h *uiHandler) renderNotFound(w http.ResponseWriter, message string) {
	w.WriteHeader(http.StatusNotFound)
	// Nav intentionally left blank: base.html's nav highlighting only matches
	// "dashboard"/"workflows" exactly, so an empty value just means neither
	// link is highlighted, which is correct for a page that isn't really on
	// either. Every *PageData struct needs *some* value for base.html's
	// {{.Nav}} references to resolve — html/template errors on a missing
	// struct field (unlike a missing map key, which resolves to the zero
	// value silently) — a real bug caught here: without this field, this
	// exact page failed to render at all (a 404 status with next to nothing
	// in the body), caught by curling the running service, not go vet/build.
	h.render(w, notFoundTemplate, "base", notFoundPageData{Message: message})
}

type notFoundPageData struct {
	Nav     string
	Message string
}

// render mirrors services/tooling/garage/ui.go's render exactly, including
// its reasoning: html/template only partially writes on an execution error
// (headers are already sent by the time a template error can happen), so a
// failure here is logged rather than turned into an http.Error.
func (h *uiHandler) render(w http.ResponseWriter, tmpl *template.Template, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		h.logger.WithError(err).Error("failed to render template")
	}
}
