// This file is Bench's server-rendered UI: the routes, the page frame every page shares, the
// Settings page and the 404. The pages themselves live beside it — ui_overview.go (Overview and
// Recent runs), ui_workflow.go (workflow detail and the editor) and ui_run.go (a run) — and all of
// them render through the shared design system (tools/draft-ui) rather than a template of Bench's own.
//
// Phase 10 (docs/website/content/docs/architecture/bench-workflow-engine.md's "Authoring
// workflows without a file"): workflow_write.go's createWorkflow/updateWorkflow are the same
// functions rpc.go's CreateWorkflow/UpdateWorkflow RPCs call, so this UI and the RPC surface can't
// drift on what "create" and "update" mean — the editor only adds the HTML-form transport.
package main

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	settingsv1 "github.com/steady-bytes/draft/api/tooling/settings/v1"
	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"
	"github.com/steady-bytes/draft/pkg/chassis"
	draftui "github.com/steady-bytes/draft/tools/draft-ui"

	"connectrpc.com/connect"
)

// uiStore is what the UI reads and writes. *pgResultStore satisfies it; the interface exists so
// the handlers can be tested against an in-memory fake (ui_test.go).
type uiStore interface {
	workflowWriteStore
	pluginRegistryStore
	registryLister

	ListWorkflows(ctx context.Context) ([]*workflowv1.Workflow, error)
	DeleteWorkflow(ctx context.Context, name string) error
	GetRun(ctx context.Context, runID string) (*workflowv1.Run, error)
	RunRecords(ctx context.Context, f runFilter, pageSize int, pageToken string) ([]runRecord, string, error)
	LastRunRecords(ctx context.Context, n int) (map[string][]runRecord, error)
	RunCounts(ctx context.Context, since time.Time) (runCounts, error)
	FailedSteps(ctx context.Context, runIDs []string) (map[string]string, error)
	DeletePluginRegistry(ctx context.Context, name string) error
}

type uiHandler struct {
	logger     chassis.Logger
	store      uiStore
	scheduler  workflowRunner
	httpClient connect.HTTPClient
	// now is the clock; tests fix it.
	now func() time.Time
}

// NewUIHandler builds Bench's HTML page handler. store and scheduler are the same instances
// main.go already constructed for the RPC/webhook handlers — the UI reads/writes through them
// directly rather than looping back through its own Connect RPC surface over the network, the same
// choice services/tooling/foundry/ui.go makes for its own store. httpClient is used by the Settings
// page's live registry-reachability check (registry_write.go) and the editor's plugin search and
// schema checks.
func NewUIHandler(logger chassis.Logger, store *pgResultStore, scheduler workflowRunner, httpClient connect.HTTPClient) chassis.RPCRegistrar {
	return newUIHandler(logger, store, scheduler, httpClient)
}

func newUIHandler(logger chassis.Logger, store uiStore, scheduler workflowRunner, httpClient connect.HTTPClient) *uiHandler {
	return &uiHandler{logger: logger, store: store, scheduler: scheduler, httpClient: httpClient, now: time.Now}
}

func (h *uiHandler) RegisterRPC(server chassis.Rpcer) {
	// enableReflection is false for the same reason foundry/ui.go's is: these are plain
	// HTML/static routes, not a Connect/gRPC service.
	server.AddHandler("/", h.mux(), false)
}

func (h *uiHandler) mux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", h.overview)
	mux.HandleFunc("GET /workflows", h.overview) // an alias: the rail's "All workflows"
	mux.HandleFunc("GET /partials/workflows", h.workflowTable)
	mux.HandleFunc("GET /workflows/new", h.workflowNew)
	mux.HandleFunc("POST /workflows", h.createWorkflow)
	mux.HandleFunc("POST /workflows/validate", h.validate)
	mux.HandleFunc("GET /workflows/plugins/search", h.pluginSearch)
	mux.HandleFunc("GET /workflows/{name}", h.workflowDetail)
	mux.HandleFunc("GET /workflows/{name}/edit", h.workflowEdit)
	mux.HandleFunc("POST /workflows/{name}", h.updateWorkflow)
	mux.HandleFunc("POST /workflows/{name}/delete", h.deleteWorkflow)
	mux.HandleFunc("POST /workflows/{name}/run", h.triggerRun)
	mux.HandleFunc("GET /runs", h.runs)
	mux.HandleFunc("GET /runs/{id}", h.runDetail)
	mux.HandleFunc("GET /partials/runs/{id}", h.runDetailFragment)
	mux.HandleFunc("GET /settings", h.settings)
	mux.HandleFunc("POST /settings", h.addRegistry)
	mux.HandleFunc("POST /settings/{name}/delete", h.deleteRegistry)
	// The design system's stylesheet, fonts, scripts and the vendored htmx, embedded in the module
	// (tools/draft-ui) rather than duplicated per service.
	mux.Handle("GET "+draftui.StaticPrefix, draftui.Static())
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		h.notFound(w, r, fmt.Sprintf("Nothing lives at %s.", r.URL.Path))
	})
	return mux
}

// --- the page frame ----------------------------------------------------------------------------

// countsWindow is how far back the rail's "Recent runs" and "Failed" counts look.
const countsWindow = 24 * time.Hour

// frame is what every page needs beyond its own content: the rail with its counts, the topbar
// status and the status bar. Counts that cannot be loaded are left blank rather than failing the
// page; the page's own content is what matters.
type frame struct {
	page draftui.Page
	// inFlight is how many runs have not finished (also the topbar status).
	inFlight int
	// registry describes the first plugin registry for the status bar.
	registry string
}

func (h *uiHandler) frame(r *http.Request, title string, crumbs ...string) frame {
	ctx := r.Context()
	p := kit.NewPage(r, title, crumbs...)
	fixRailCurrent(&p, r)
	f := frame{page: p, registry: "none"}

	if workflows, err := h.store.ListWorkflows(ctx); err != nil {
		h.logger.WithError(err).Warn("failed to count workflows for the rail")
	} else {
		f.page.SetCount("/workflows", len(workflows))
	}
	if counts, err := h.store.RunCounts(ctx, h.now().Add(-countsWindow)); err != nil {
		h.logger.WithError(err).Warn("failed to count runs for the rail")
	} else {
		f.page.SetCount("/runs", counts.Total)
		f.page.SetErrCount("/runs?status=failed", counts.Failed)
		f.inFlight = counts.InFlight
	}
	if registries, err := h.store.ListPluginRegistries(ctx); err != nil {
		h.logger.WithError(err).Warn("failed to count plugin registries for the rail")
	} else {
		f.page.SetCount("/settings", len(registries))
		if len(registries) > 0 {
			f.registry = registries[0].GetName() + " · " + hostOf(registries[0].GetAddress())
		}
	}

	f.page.Status = inFlightStatus(f.inFlight)
	f.page.Left = []draftui.BarItem{draftui.KV("Registry", f.registry)}
	f.page.Right = []draftui.BarItem{draftui.KV("Webhooks", "POST /webhooks/:slug")}
	return f
}

// inFlightStatus is the topbar pill: "1 run in flight" (live) or a quiet "Idle".
func inFlightStatus(n int) *draftui.Status {
	if n == 0 {
		return &draftui.Status{Class: "d-status--idle", Text: "Idle"}
	}
	return &draftui.Status{Class: "d-status--info", Live: true, Text: plural(n, "run") + " in flight"}
}

// fixRailCurrent settles the rail items that share a path: "Recent runs" and "Failed" are both
// /runs (told apart by ?status=), and "+ New workflow" sits under "All workflows".
func fixRailCurrent(p *draftui.Page, r *http.Request) {
	failed := r.URL.Path == "/runs" && r.URL.Query().Get("status") == "failed"
	for i := range p.Rail {
		for j := range p.Rail[i].Items {
			it := &p.Rail[i].Items[j]
			switch it.Path {
			case "/runs?status=failed":
				it.Current = failed
			case "/runs":
				it.Current = r.URL.Path == "/runs" && !failed
			case "/workflows":
				if r.URL.Path == "/workflows/new" {
					it.Current = false
				}
			}
		}
	}
}

// hostOf trims the scheme from an address for display ("http://localhost:9301" → "localhost:9301").
func hostOf(address string) string {
	if _, rest, ok := strings.Cut(address, "://"); ok {
		return strings.TrimSuffix(rest, "/")
	}
	return address
}

// --- settings ----------------------------------------------------------------------------------

type settingsPage struct {
	draftui.Page
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
	h.render(w, settingsTemplate, settingsPage{
		Page:       h.frame(r, "Settings", "Bench", "Plugin registries").page,
		Registries: registries,
	})
}

// addRegistry handles the Settings page's Add-registry form POST. On a validation failure (missing
// fields, or address doesn't answer as a foundry-compatible registry — see registry_write.go's
// checkRegistryReachable), the page re-renders with the submitted name/address preserved and the
// error shown, the same "server does the real work, re-render with an error" shape
// createWorkflow/updateWorkflow already established.
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
		h.render(w, settingsTemplate, settingsPage{
			Page:        h.frame(r, "Settings", "Bench", "Plugin registries").page,
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

// --- not found ---------------------------------------------------------------------------------

type notFoundPage struct {
	draftui.Page
	Message string
}

func (h *uiHandler) notFound(w http.ResponseWriter, r *http.Request, message string) {
	data := notFoundPage{Page: h.frame(r, "Not found", "Bench", "Not found").page, Message: message}
	w.WriteHeader(http.StatusNotFound)
	h.render(w, notFoundTemplate, data)
}

// render executes a full page template. draftui.Kit.Render buffers the output first, so a template
// error becomes a clean 500 instead of a half-written page.
func (h *uiHandler) render(w http.ResponseWriter, tmpl *template.Template, data any) {
	if err := kit.Render(w, tmpl, "shell", data); err != nil {
		h.logger.WithError(err).Error("failed to render page")
	}
}

// fragment executes one named block of a fragment template (an htmx swap target).
func (h *uiHandler) fragment(w http.ResponseWriter, tmpl *template.Template, name string, data any) {
	if err := kit.Render(w, tmpl, name, data); err != nil {
		h.logger.WithError(err).WithField("fragment", name).Error("failed to render fragment")
	}
}
