// The workflow pages: a workflow's detail (its definition and run history) and the editor for
// writing one (live validation, the pipeline strip and a plugin search).
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	plugincatalogv1 "github.com/steady-bytes/draft/api/tooling/plugin_catalog/v1"
	plugincatalogv1connect "github.com/steady-bytes/draft/api/tooling/plugin_catalog/v1/v1connect"
	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"
	draftui "github.com/steady-bytes/draft/tools/draft-ui"

	"connectrpc.com/connect"
	"gopkg.in/yaml.v3"
)

// validateTimeout bounds the registry calls one validation makes, so a slow registry cannot stall
// the editor's live feedback.
const validateTimeout = 4 * time.Second

// --- workflow detail ---------------------------------------------------------------------------

type workflowDetailPage struct {
	draftui.Page
	Name, Description, WebhookSlug string
	Steps                          int
	YAML                           string
	Runs                           runsTable
	History                        draftui.Strip
	HistoryPassed                  string
}

func (h *uiHandler) workflowDetail(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := r.Context()

	wf, err := h.store.GetWorkflow(ctx, name)
	if err != nil {
		h.notFound(w, r, fmt.Sprintf("No workflow named %q is loaded.", name))
		return
	}
	yamlSrc, err := renderWorkflowYAML(wf)
	if err != nil {
		h.logger.WithError(err).WithField("workflow", name).Error("failed to render workflow yaml")
		yamlSrc = "# failed to render this workflow's definition"
	}
	records, _, err := h.store.RunRecords(ctx, runFilter{Workflow: name}, workflowRunHistorySize, "")
	if err != nil {
		h.logger.WithError(err).WithField("workflow", name).Error("failed to list runs for workflow detail")
		http.Error(w, "failed to load workflow", http.StatusInternalServerError)
		return
	}

	data := workflowDetailPage{
		Page:        h.frame(r, name, "Bench", name).page,
		Name:        wf.GetName(),
		Description: wf.GetDescription(),
		WebhookSlug: wf.GetTrigger().GetWebhook().GetSlug(),
		Steps:       len(wf.GetSteps()),
		YAML:        yamlSrc,
		Runs:        h.buildRunsTable(ctx, records, false),
	}
	data.Runs.Empty = "No runs yet."
	data.History = historyStrip(records, h.now().UTC(), "")
	data.History.Size = "tall"
	passed := 0
	for i, rec := range records {
		if i < historyRuns && rec.Passed() {
			passed++
		}
	}
	data.HistoryPassed = fmt.Sprintf("%d / %d passed", passed, min(len(records), historyRuns))
	h.render(w, workflowDetailTemplate, data)
}

// workflowRunHistorySize caps how many runs a workflow's own page lists.
const workflowRunHistorySize = 25

// deleteWorkflow handles the Delete button on the workflow detail page. Run history for name is
// untouched (store.go's DeleteWorkflow) — only the definition goes away.
func (h *uiHandler) deleteWorkflow(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.store.DeleteWorkflow(r.Context(), name); err != nil && !errors.Is(err, ErrWorkflowNotFound) {
		h.logger.WithError(err).WithField("workflow", name).Error("failed to delete workflow")
		http.Error(w, "failed to delete workflow", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/workflows", http.StatusSeeOther)
}

// --- editor ------------------------------------------------------------------------------------

// editorFeedback is what validation draws around the text area: the pipeline, the problems and the
// topbar pill. It is rendered into the page once and then swapped out of band by /workflows/validate.
type editorFeedback struct {
	// OOB marks the markup for htmx's out-of-band swap.
	OOB      bool
	Pipe     []pipeNode
	Problems []problem
	// ErrorLines is "24,25" for the gutter's red line numbers.
	ErrorLines string
	Errors     int
	Warnings   int
	// Checked is "grpc-call@v1, http-call@v1": the plugin schemas `with:` was validated against.
	Checked string
	Note    string
	Status  draftui.Status
}

func newEditorFeedback(a analysis, oob bool) editorFeedback {
	f := editorFeedback{OOB: oob, Pipe: a.Pipe, Problems: a.Problems, Errors: a.Errors()}
	f.Warnings = len(a.Problems) - f.Errors
	lines := make([]string, 0, len(a.ErrorLines()))
	for _, l := range a.ErrorLines() {
		lines = append(lines, fmt.Sprintf("%d", l))
	}
	f.ErrorLines = strings.Join(lines, ",")
	f.Checked = strings.Join(a.Checked, ", ")
	if a.SchemaUnavailable {
		f.Note = "Plugin schemas are unavailable, so with: was not checked."
	}
	switch {
	case f.Errors > 0:
		f.Status = draftui.Status{Class: "d-status--err", Text: plural(f.Errors, "problem")}
	case f.Warnings > 0:
		f.Status = draftui.Status{Class: "d-status--warn", Text: plural(f.Warnings, "warning")}
	default:
		f.Status = draftui.Status{Text: "Valid"}
	}
	return f
}

// Subtitle is the panel head's right-hand label.
func (f editorFeedback) Subtitle() string {
	switch {
	case f.Note != "":
		return f.Note
	case f.Checked != "":
		return "Validated against " + f.Checked + " schema"
	}
	return "Structure checked"
}

type editorPage struct {
	draftui.Page
	IsEdit      bool
	Name        string
	Description string
	YAML        string
	FormAction  string
	CancelHref  string
	Feedback    editorFeedback
	Lines       int
}

// defaultWorkflowTemplate pre-fills the New-workflow editor so a first-time author has a real,
// structurally valid starting point to edit rather than a blank box — passes loader.go's Validate
// as-is (it only checks shape: required fields, a recognized uses: scheme, an acyclic depends_on
// graph), though running it will fail until service/method are pointed at something real.
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

func (h *uiHandler) editorPage(r *http.Request, isEdit bool, name, yamlSrc string, extraErr error) editorPage {
	crumbs := []string{"Bench", "New workflow"}
	title := "New workflow"
	action, cancel := "/workflows", "/workflows"
	if isEdit {
		crumbs = []string{"Bench", name, "Edit"}
		title = "Edit " + name
		action, cancel = "/workflows/"+name, "/workflows/"+name
	}
	fr := h.frame(r, title, crumbs...)
	fr.page.Status = nil // the editor's own pill (Feedback.Status) lives in the topbar block

	a := analyzeWorkflow(r.Context(), yamlSrc, nil)
	if extraErr != nil && a.Errors() == 0 {
		a.Problems = append(a.Problems, problem{Message: extraErr.Error()})
	}
	wfName, desc := workflowMeta(yamlSrc)
	fr.page.Left = []draftui.BarItem{draftui.KVID("wf-lines", "YAML", fmt.Sprintf("%d lines", a.Lines)), draftui.KVID("wf-cursor", "Cursor", "1:1")}
	fr.page.Right = []draftui.BarItem{draftui.Hint("⌘S", "save"), draftui.Hint("⌘↵", "save and run")}
	return editorPage{
		Page:        fr.page,
		IsEdit:      isEdit,
		Name:        firstNonEmpty(name, wfName),
		Description: desc,
		YAML:        yamlSrc,
		FormAction:  action,
		CancelHref:  cancel,
		Feedback:    newEditorFeedback(a, false),
		Lines:       a.Lines,
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func (h *uiHandler) workflowNew(w http.ResponseWriter, r *http.Request) {
	h.render(w, editorTemplate, h.editorPage(r, false, "", defaultWorkflowTemplate, nil))
}

func (h *uiHandler) workflowEdit(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	wf, err := h.store.GetWorkflow(r.Context(), name)
	if err != nil {
		h.notFound(w, r, fmt.Sprintf("No workflow named %q is loaded.", name))
		return
	}
	yamlSrc, err := renderWorkflowYAML(wf)
	if err != nil {
		h.logger.WithError(err).WithField("workflow", name).Error("failed to render workflow yaml for edit form")
		yamlSrc = "# failed to render this workflow's definition"
	}
	h.render(w, editorTemplate, h.editorPage(r, true, name, yamlSrc, nil))
}

// createWorkflow handles the New-workflow form POST. On a validation failure (bad YAML, or the name
// is already taken) the same editor re-renders with the submitted text preserved and the reason
// shown, rather than losing what was typed. With `run=1` (⌘↵) a successful save also starts a run.
func (h *uiHandler) createWorkflow(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}
	yamlDoc := r.FormValue("yaml")
	wf, err := createWorkflow(r.Context(), h.store, yamlDoc)
	if err != nil {
		h.render(w, editorTemplate, h.editorPage(r, false, "", yamlDoc, err))
		return
	}
	h.afterSave(w, r, wf, r.FormValue("run") == "1")
}

// updateWorkflow handles the Edit-workflow form POST — see createWorkflow; the same
// re-render-with-the-reason behavior on a validation failure, keyed off the workflow being edited.
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
			h.notFound(w, r, fmt.Sprintf("No workflow named %q is loaded.", name))
			return
		}
		h.render(w, editorTemplate, h.editorPage(r, true, name, yamlDoc, err))
		return
	}
	h.afterSave(w, r, wf, r.FormValue("run") == "1")
}

// afterSave sends the author on: to the new run when they asked to save and run, else to the workflow.
func (h *uiHandler) afterSave(w http.ResponseWriter, r *http.Request, wf *workflowv1.Workflow, run bool) {
	if run {
		started, err := h.scheduler.StartRun(r.Context(), wf)
		if err != nil {
			// The workflow is saved; only the run failed to start. Land on its page, where Run is one click.
			h.logger.WithError(err).WithField("workflow", wf.GetName()).Error("saved workflow but failed to start its run")
		} else {
			http.Redirect(w, r, "/runs/"+started.GetRunId(), http.StatusSeeOther)
			return
		}
	}
	http.Redirect(w, r, "/workflows/"+wf.GetName(), http.StatusSeeOther)
}

// validate is the editor's live feedback: it returns the pipeline, the problems list and the topbar
// pill as out-of-band swaps for whatever YAML is in the text area.
func (h *uiHandler) validate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), validateTimeout)
	defer cancel()
	a := analyzeWorkflow(ctx, r.FormValue("yaml"), h.schemaLookup(ctx))
	h.fragment(w, editorFeedbackTemplate, "editor-feedback", newEditorFeedback(a, true))
}

// schemaLookup resolves foundry://name@version against the configured registries, in order (the
// same first-match rule the foundry:// executor uses). It reads the registry list once per
// validation, so a registry added on the Settings page is used on the next keystroke.
func (h *uiHandler) schemaLookup(ctx context.Context) schemaLookup {
	registries, err := h.store.ListPluginRegistries(ctx)
	if err != nil || len(registries) == 0 {
		return func(context.Context, string, string) (*pluginSchema, error) {
			return nil, errors.New("no plugin registry available")
		}
	}
	return func(ctx context.Context, name, version string) (*pluginSchema, error) {
		unreachable := false
		for _, reg := range registries {
			client := plugincatalogv1connect.NewPluginCatalogServiceClient(h.httpClient, reg.GetAddress(), connect.WithGRPC())
			resp, err := client.Get(ctx, connect.NewRequest(&plugincatalogv1.GetPluginRequest{Name: name, Version: version}))
			if err != nil {
				if connect.CodeOf(err) != connect.CodeNotFound {
					unreachable = true
				}
				continue
			}
			return schemaOf(resp.Msg.GetConfigSchema().AsMap()), nil
		}
		if unreachable {
			return nil, errors.New("a registry could not be reached")
		}
		return nil, nil
	}
}

// schemaOf reads the parts of a JSON Schema the editor checks: `required` and the keys of `properties`.
func schemaOf(schema map[string]interface{}) *pluginSchema {
	s := &pluginSchema{Properties: map[string]bool{}}
	if req, ok := schema["required"].([]interface{}); ok {
		for _, r := range req {
			if name, ok := r.(string); ok {
				s.Required = append(s.Required, name)
			}
		}
	}
	if props, ok := schema["properties"].(map[string]interface{}); ok {
		for name := range props {
			s.Properties[name] = true
		}
	}
	return s
}

// --- plugin search -----------------------------------------------------------------------------

// pluginResult is one search result card: the plugin itself plus which registry it came from (a
// human picking a result needs to see the source even though foundry:// resolution picks silently)
// and a ready-to-paste step snippet.
type pluginResult struct {
	Name, Version, Description, Registry, Snippet string
	// RequiredLine is "required: address · service · method", or a note that nothing is required.
	RequiredLine string
}

type pluginSearchData struct {
	Query        string
	Results      []pluginResult
	NoRegistries bool
	Error        string
}

// pluginSearch is the htmx target behind the editor's plugin drawer: it federates Search (or List,
// for an empty query) across every configured registry and merges the results — Bench calling
// Foundry's already-real PluginCatalogService like foundry_plugin.go already does for execution.
func (h *uiHandler) pluginSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := strings.TrimSpace(r.URL.Query().Get("q"))

	registries, err := h.store.ListPluginRegistries(ctx)
	if err != nil {
		h.logger.WithError(err).Error("failed to list plugin registries for search")
		h.fragment(w, pluginResultsTemplate, "plugin-results", pluginSearchData{Query: query, Error: "Failed to load the plugin registries."})
		return
	}
	if len(registries) == 0 {
		h.fragment(w, pluginResultsTemplate, "plugin-results", pluginSearchData{Query: query, NoRegistries: true})
		return
	}

	var results []pluginResult
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
			results = append(results, pluginResult{
				Name:         p.GetName(),
				Version:      p.GetVersion(),
				Description:  p.GetDescription(),
				Registry:     reg.GetName(),
				Snippet:      pluginUsesSnippet(p),
				RequiredLine: requiredLine(schemaOf(p.GetConfigSchema().AsMap()).Required),
			})
		}
	}
	h.fragment(w, pluginResultsTemplate, "plugin-results", pluginSearchData{Query: query, Results: results})
}

func requiredLine(required []string) string {
	if len(required) == 0 {
		return "no required fields"
	}
	return "required: " + strings.Join(required, " · ")
}

// pluginUsesSnippet builds a ready-to-paste step block from a plugin's published config_schema —
// Bench's own small reimplementation of Foundry's own private usesSnippet
// (services/tooling/foundry/ui.go), not a cross-service call; see the design doc's "The snippet
// itself is a small, deliberate duplication" for why. It is indented to match a `steps:` list item
// everywhere else in this codebase renders one: a 2-space-indented "- name:" line, its fields at 4
// spaces, and (when present) with:'s own fields at 6 — see renderWorkflowYAML's reconstructed YAML
// and defaultWorkflowTemplate's own steps: entry for the same convention. Not column 0: the
// editor's insert appends this directly under an existing steps: list, so it has to nest as a
// sibling of whatever's already there, not a top-level block.
func pluginUsesSnippet(p *plugincatalogv1.Plugin) string {
	var b strings.Builder
	fmt.Fprintf(&b, "  - name: use-%s\n", p.GetName())
	fmt.Fprintf(&b, "    uses: foundry://%s@%s\n", p.GetName(), p.GetVersion())

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

// renderWorkflowYAML reconstructs a readable YAML view of a persisted Workflow. This is not the
// literal file bytes Bench originally loaded — nothing stores those, only the
// parsed-and-reserialized definition (see model.go's overviewRow.Definition) — so what's shown is a
// semantically equivalent re-serialization, not guaranteed byte-identical to the source file.
// structpb.Struct.AsMap() converts a step's with:/expect: back to plain Go values yaml.Marshal
// already knows how to encode.
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

	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return buf.String(), nil
}
