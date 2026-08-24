package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"

	"github.com/steady-bytes/draft/pkg/chassis"
)

// This file is Phase 4 (The UI): the Catalog and Plugin detail pages from
// docs/website/content/docs/architecture/garage-plugin-repository.md's "The
// UI" section, server-side rendered with html/template + htmx + DaisyUI —
// see that doc's stack rationale, shared with Bench's UI section.
//
// uiHandler is wired onto the *same* mux the RPC handler in rpc.go uses,
// rather than a second listener: it implements chassis.RPCRegistrar (like
// handler in rpc.go) and, in RegisterRPC, mounts a plain net/http.ServeMux
// of page routes at "/" via chassis.Rpcer.AddHandler. Go's net/http.ServeMux
// resolves the more specific "/tooling.plugin_catalog.v1.PluginCatalogService/"
// pattern (registered by rpc.go's handler) ahead of this catch-all "/", so
// RPC and UI traffic share one port with no path collision. main.go passes
// both handlers to chassis via two WithRPCHandler calls; see its comment for
// why a second listener (the services/core/auth fallback the brief offered)
// wasn't needed here.
//
// One known, deliberately-accepted quirk of reusing AddHandler for this:
// AddHandler always records its (trimmed) pattern in the service-name list
// chassis reports to Blueprint during sync, regardless of the
// enableReflection flag passed here (false) — see pkg/chassis/rpc.go's
// AddHandler. Registering "/" trims to an empty string, so Blueprint's sync
// metadata for garage will include one Metadata{Key: "", Value: ""} entry
// alongside the real PluginCatalogService one. It's inert (nothing reads
// that Blueprint metadata entry for routing today), but is worth knowing
// about if Blueprint's metadata list is ever surfaced/consumed directly.

const (
	// catalogListPageSize is large enough to fetch "every" published plugin
	// in one page for the catalog view. The design doc's Decided section
	// expects Garage's catalog to stay small for a long time (same
	// reasoning store.go's defaultListPageSize comment gives), so a single
	// generous page — rather than the UI itself paginating — is adequate for
	// Phase 4; true pagination in the catalog page is not in this phase's
	// scope.
	catalogListPageSize = 500
)

type uiHandler struct {
	logger chassis.Logger
	store  *store
}

// NewUIHandler builds the HTML page handler. Kept separate from Handler in
// rpc.go (which is untouched by this phase, per the brief) even though both
// implement chassis.RPCRegistrar and are registered the same way.
func NewUIHandler(logger chassis.Logger, store *store) chassis.RPCRegistrar {
	return &uiHandler{logger: logger, store: store}
}

func (h *uiHandler) RegisterRPC(server chassis.Rpcer) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", h.catalog)
	mux.HandleFunc("GET /partials/catalog-cards", h.catalogCards)
	mux.HandleFunc("GET /plugins/{name}/{version}", h.pluginDetail)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticFS)))

	// enableReflection is false: these are plain HTML/static routes, not a
	// Connect/gRPC service, and have no business in grpc reflection's
	// service list.
	server.AddHandler("/", mux, false)
}

// cardData is what card_grid.html renders per plugin *name* (its latest
// published version, not every version — see groupLatestByName).
type cardData struct {
	Name          string
	Description   string
	LatestVersion string
	Maintainer    string
}

type catalogPageData struct {
	Query string
	Cards []cardData
}

// catalog serves GET / — the full catalog page.
func (h *uiHandler) catalog(w http.ResponseWriter, r *http.Request) {
	rows, _, err := h.store.list(r.Context(), catalogListPageSize, "")
	if err != nil {
		h.logger.WithError(err).Error("failed to list plugins for catalog page")
		http.Error(w, "failed to load catalog", http.StatusInternalServerError)
		return
	}

	data := catalogPageData{Cards: groupLatestByName(rows)}
	h.render(w, catalogTemplate, "base", data)
}

// catalogCards serves GET /partials/catalog-cards?q=... — the htmx fragment
// the catalog page's search input swaps in on keyup/change (see
// catalog.html's hx-get/hx-target/hx-swap attributes). An empty/missing q
// behaves like the full catalog (store.list, not store.search), so clearing
// the search box restores every plugin rather than an empty result.
func (h *uiHandler) catalogCards(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))

	var (
		rows []*pluginRow
		err  error
	)
	if query == "" {
		rows, _, err = h.store.list(r.Context(), catalogListPageSize, "")
	} else {
		rows, err = h.store.search(r.Context(), query)
	}
	if err != nil {
		h.logger.WithError(err).Error("failed to search plugins for catalog fragment")
		http.Error(w, "failed to search catalog", http.StatusInternalServerError)
		return
	}

	data := catalogPageData{Query: query, Cards: groupLatestByName(rows)}
	h.render(w, cardGridTemplate, "card-grid", data)
}

// groupLatestByName collapses store rows (one per published *version*) down
// to one cardData per plugin *name*, keeping whichever row has the latest
// PublishedAt — the design doc's card grid shows "name, description, latest
// version, maintainer", not one card per version. Returned sorted by name
// for a stable, deterministic grid ordering regardless of publish order.
func groupLatestByName(rows []*pluginRow) []cardData {
	latest := make(map[string]*pluginRow, len(rows))
	for _, row := range rows {
		current, ok := latest[row.Name]
		if !ok || row.PublishedAt.After(current.PublishedAt) {
			latest[row.Name] = row
		}
	}

	cards := make([]cardData, 0, len(latest))
	for _, row := range latest {
		cards = append(cards, cardData{
			Name:          row.Name,
			Description:   row.Description,
			LatestVersion: row.Version,
			Maintainer:    row.Maintainer,
		})
	}
	sort.Slice(cards, func(i, j int) bool { return cards[i].Name < cards[j].Name })
	return cards
}

// schemaField is one row of the plugin detail page's config-schema table.
type schemaField struct {
	Name     string
	Type     string
	Required bool
}

type detailPageData struct {
	Name        string
	Version     string
	Versions    []string // every published version of Name, newest first, for the tabs strip
	Description string
	Maintainer  string
	Source      string

	SchemaFields []schemaField
	UsesSnippet  string
}

type notFoundPageData struct {
	Message string
}

// pluginDetail serves GET /plugins/{name}/{version}.
func (h *uiHandler) pluginDetail(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	version := r.PathValue("version")
	ctx := r.Context()

	row, err := h.store.get(ctx, name, version)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			h.renderNotFound(w, fmt.Sprintf("No published plugin matches %s@%s.", name, version))
			return
		}
		h.logger.WithError(err).Error("failed to get plugin for detail page")
		http.Error(w, "failed to load plugin", http.StatusInternalServerError)
		return
	}

	versionRows, err := h.store.listVersions(ctx, name)
	if err != nil {
		h.logger.WithError(err).Error("failed to list plugin versions for detail page")
		http.Error(w, "failed to load plugin", http.StatusInternalServerError)
		return
	}
	versions := make([]string, 0, len(versionRows))
	for _, vr := range versionRows {
		versions = append(versions, vr.Version)
	}

	fields, err := schemaFields(row.ConfigSchema)
	if err != nil {
		// A malformed config_schema shouldn't 500 the whole page — the rest
		// of the plugin's metadata is still valid and worth showing. Publish
		// already runs a (deliberately minimal) shape check, so this is not
		// expected in practice; render an empty table rather than fail.
		h.logger.WithError(err).Warn("failed to parse config_schema for detail page")
		fields = nil
	}

	data := detailPageData{
		Name:         row.Name,
		Version:      row.Version,
		Versions:     versions,
		Description:  row.Description,
		Maintainer:   row.Maintainer,
		Source:       row.Source,
		SchemaFields: fields,
		UsesSnippet:  usesSnippet(row.Name, row.Version, fields),
	}
	h.render(w, detailTemplate, "base", data)
}

func (h *uiHandler) renderNotFound(w http.ResponseWriter, message string) {
	w.WriteHeader(http.StatusNotFound)
	h.render(w, notFoundTemplate, "base", notFoundPageData{Message: message})
}

// schemaFields walks a plugin's config_schema (stored as raw JSON Schema —
// see model.go's pluginRow.ConfigSchema) into the rows the detail page's
// table renders: for each key under "properties", its declared "type" and
// whether it's named in the top-level "required" array. This only walks the
// shape the design doc's example manifest uses (a flat "object" schema with
// scalar-typed properties) — nested/composed schemas (oneOf, nested
// objects, $ref) render with a best-effort Type string rather than being
// fully modeled, since nothing published so far needs more than that.
func schemaFields(raw json.RawMessage) ([]schemaField, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config_schema: %w", err)
	}

	required := make(map[string]bool, len(schema.Required))
	for _, name := range schema.Required {
		required[name] = true
	}

	names := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		names = append(names, name)
	}
	sort.Strings(names)

	fields := make([]schemaField, 0, len(names))
	for _, name := range names {
		var prop struct {
			Type json.RawMessage `json:"type"`
		}
		fieldType := "any"
		if err := json.Unmarshal(schema.Properties[name], &prop); err == nil && len(prop.Type) > 0 {
			fieldType = propertyTypeString(prop.Type)
		}
		fields = append(fields, schemaField{
			Name:     name,
			Type:     fieldType,
			Required: required[name],
		})
	}
	return fields, nil
}

// propertyTypeString renders a JSON Schema "type" keyword (a string, or an
// array of strings for a union type) as display text.
func propertyTypeString(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return strings.Join(list, " | ")
	}
	return "any"
}

// usesSnippet builds the copy-pasteable YAML block the detail page's
// mockup-code panel shows, matching the exact `uses: garage://name@version`
// reference syntax from Bench's worked example
// (docs/website/content/docs/architecture/bench-workflow-engine.md's
// Primitives section: "uses: garage://slack-notify@v2"). The `with:` block
// is populated from the schema's required fields (placeholder values named
// after their type) so the snippet answers "what do I put in with: for this
// plugin, concretely" — the exact question the design doc's UI section
// calls out — directly from data, without a plugin author separately
// maintaining example docs that can drift from the schema.
func usesSnippet(name, version string, fields []schemaField) string {
	var b strings.Builder
	fmt.Fprintf(&b, "- name: use-%s\n", name)
	fmt.Fprintf(&b, "  uses: garage://%s@%s\n", name, version)

	var required []schemaField
	for _, f := range fields {
		if f.Required {
			required = append(required, f)
		}
	}
	if len(required) == 0 {
		b.WriteString("  with: {}\n")
	} else {
		b.WriteString("  with:\n")
		for _, f := range required {
			fmt.Fprintf(&b, "    %s: <%s>\n", f.Name, f.Type)
		}
	}
	return b.String()
}

// render executes tmpl's named entry point against data, writing straight
// to w. html/template only partially-writes on an execution error (headers
// already sent, since there's no way to know a template will fail before
// running it), so a failure here is logged rather than turned into an
// http.Error — the response is already in flight.
func (h *uiHandler) render(w http.ResponseWriter, tmpl *template.Template, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		h.logger.WithError(err).Error("failed to render template")
	}
}
