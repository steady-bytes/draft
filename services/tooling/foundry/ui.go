package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/steady-bytes/draft/pkg/chassis"
	draftui "github.com/steady-bytes/draft/tools/draft-ui"
)

// This file is Phase 4 (The UI): the Catalog and Plugin detail pages from
// docs/website/content/docs/architecture/foundry-plugin-repository.md's "The
// UI" section, server-side rendered with html/template + htmx on the shared Draft design system
// (tools/draft-ui: shell, partials and the embedded stylesheet) — see
// docs/website/content/docs/architecture/design-system-implementation-plan.md.
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
// metadata for foundry will include one Metadata{Key: "", Value: ""} entry
// alongside the real PluginCatalogService one. It's inert (nothing reads
// that Blueprint metadata entry for routing today), but is worth knowing
// about if Blueprint's metadata list is ever surfaced/consumed directly.

const (
	// catalogListPageSize is large enough to fetch "every" published plugin
	// in one page for the catalog view. The design doc's Decided section
	// expects Foundry's catalog to stay small for a long time (same
	// reasoning store.go's defaultListPageSize comment gives), so a single
	// generous page — rather than the UI itself paginating — is adequate for
	// Phase 4; true pagination in the catalog page is not in this phase's
	// scope.
	catalogListPageSize = 500

	sortName   = "name"
	sortNewest = "newest"
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
	// The design system's stylesheet, fonts, scripts and the vendored htmx, embedded in the
	// module (tools/draft-ui) rather than duplicated per service.
	mux.Handle("GET "+draftui.StaticPrefix, draftui.Static())

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
	PublishedAt   time.Time
	// Versions is every published version, oldest first.
	Versions []string
	// VersionTags renders Versions as tags: the latest in the primary colour when there is more
	// than one, the rest quiet.
	VersionTags []draftui.Tag
	// Required is how many fields the config schema requires.
	Required   int
	Glyph      string
	GlyphClass string
}

type sortLink struct {
	Label, Href string
	Active      bool
}

type catalogPageData struct {
	draftui.Page
	Query      string
	Sort       string
	Maintainer string
	Cards      []cardData
	SortLinks  []sortLink
}

// catalogView is the query a catalog request carries: the search text, the sort order and an
// optional maintainer filter.
type catalogView struct {
	query, sort, maintainer string
}

func parseCatalogView(r *http.Request) catalogView {
	q := r.URL.Query()
	v := catalogView{
		query:      strings.TrimSpace(q.Get("q")),
		sort:       sortName,
		maintainer: strings.TrimSpace(q.Get("maintainer")),
	}
	if q.Get("sort") == sortNewest {
		v.sort = sortNewest
	}
	return v
}

// href builds a catalog URL for this view with overrides.
func (v catalogView) href(sortOrder string) string {
	params := url.Values{}
	if sortOrder != sortName {
		params.Set("sort", sortOrder)
	}
	if v.maintainer != "" {
		params.Set("maintainer", v.maintainer)
	}
	if len(params) == 0 {
		return "/"
	}
	return "/?" + params.Encode()
}

// catalogSummary is what the shell shows about the whole catalog on every page: counts, the
// maintainers for the rail, and the last publish.
type catalogSummary struct {
	cards       []cardData
	versions    int
	last        *pluginRow
	maintainers []maintainerCount
}

type maintainerCount struct {
	Name  string
	Count int
}

func (h *uiHandler) summary(ctx context.Context) (catalogSummary, []*pluginRow, error) {
	rows, _, err := h.store.list(ctx, catalogListPageSize, "")
	if err != nil {
		return catalogSummary{}, nil, err
	}
	s := catalogSummary{cards: groupLatestByName(rows), versions: len(rows)}
	counts := map[string]int{}
	for _, c := range s.cards {
		if c.Maintainer != "" {
			counts[c.Maintainer]++
		}
	}
	for name, n := range counts {
		s.maintainers = append(s.maintainers, maintainerCount{Name: name, Count: n})
	}
	sort.Slice(s.maintainers, func(i, j int) bool { return s.maintainers[i].Name < s.maintainers[j].Name })
	for _, r := range rows {
		if s.last == nil || r.PublishedAt.After(s.last.PublishedAt) {
			s.last = r
		}
	}
	return s, rows, nil
}

// decorate fills the shell around a page: the topbar status pill, the status bar and the rail's
// counts, maintainers and current item.
func (s catalogSummary) decorate(p *draftui.Page, r *http.Request, v catalogView) {
	p.Status = &draftui.Status{Live: true, Text: fmt.Sprintf("%d plugins · %d versions", len(s.cards), s.versions)}
	p.Left = []draftui.BarItem{draftui.KV("Catalog", r.Host)}
	if s.last != nil {
		p.Left = append(p.Left, draftui.KV("Last publish", fmt.Sprintf("%s@%s · %s UTC", s.last.Name, s.last.Version, s.last.PublishedAt.UTC().Format("15:04"))))
	}
	p.Right = []draftui.BarItem{draftui.Hint("/", "search")}

	// The Catalog items differ only by query string, which the shell's path matching ignores, so
	// their current state is set here.
	for i := range p.Rail {
		for j := range p.Rail[i].Items {
			it := &p.Rail[i].Items[j]
			switch it.Path {
			case "/":
				it.Count = draftui.FormatCount(len(s.cards))
				it.Current = v.sort == sortName && v.maintainer == ""
			case "/?sort=newest":
				it.Current = v.sort == sortNewest && v.maintainer == ""
			}
		}
	}
	if len(s.maintainers) > 0 {
		section := draftui.NavSection{Label: "Maintainers"}
		for _, m := range s.maintainers {
			section.Items = append(section.Items, draftui.NavItem{
				Label:   m.Name,
				Path:    "/?maintainer=" + url.QueryEscape(m.Name),
				Count:   draftui.FormatCount(m.Count),
				Current: v.maintainer == m.Name,
			})
		}
		p.Rail = append(p.Rail, section)
	}
}

// view applies a catalog view (maintainer filter, then sort order) to the grouped cards.
func (v catalogView) apply(cards []cardData) []cardData {
	out := make([]cardData, 0, len(cards))
	for _, c := range cards {
		if v.maintainer == "" || c.Maintainer == v.maintainer {
			out = append(out, c)
		}
	}
	if v.sort == sortNewest {
		sort.SliceStable(out, func(i, j int) bool { return out[i].PublishedAt.After(out[j].PublishedAt) })
	}
	return out
}

func (v catalogView) sortLinks() []sortLink {
	return []sortLink{
		{Label: "A–Z", Href: v.href(sortName), Active: v.sort == sortName},
		{Label: "Newest", Href: v.href(sortNewest), Active: v.sort == sortNewest},
	}
}

// catalog serves GET / — the full catalog page.
func (h *uiHandler) catalog(w http.ResponseWriter, r *http.Request) {
	s, _, err := h.summary(r.Context())
	if err != nil {
		h.logger.WithError(err).Error("failed to list plugins for catalog page")
		http.Error(w, "failed to load catalog", http.StatusInternalServerError)
		return
	}

	v := parseCatalogView(r)
	data := catalogPageData{
		Page:       kit.NewPage(r, "Plugin catalog", "Foundry", "Catalog"),
		Sort:       v.sort,
		Maintainer: v.maintainer,
		Cards:      v.apply(s.cards),
		SortLinks:  v.sortLinks(),
	}
	s.decorate(&data.Page, r, v)
	h.render(w, catalogTemplate, "shell", data)
}

// catalogCards serves GET /partials/catalog-cards?q=... — the htmx fragment
// the catalog page's search input swaps in on keyup/change (see
// catalog.html's hx-get/hx-target/hx-swap attributes). An empty/missing q
// behaves like the full catalog (store.list, not store.search), so clearing
// the search box restores every plugin rather than an empty result. The
// sort order and maintainer filter ride along as hidden inputs.
func (h *uiHandler) catalogCards(w http.ResponseWriter, r *http.Request) {
	v := parseCatalogView(r)

	var (
		rows []*pluginRow
		err  error
	)
	if v.query == "" {
		rows, _, err = h.store.list(r.Context(), catalogListPageSize, "")
	} else {
		rows, err = h.store.search(r.Context(), v.query)
	}
	if err != nil {
		h.logger.WithError(err).Error("failed to search plugins for catalog fragment")
		http.Error(w, "failed to search catalog", http.StatusInternalServerError)
		return
	}

	data := catalogPageData{Query: v.query, Sort: v.sort, Maintainer: v.maintainer, Cards: v.apply(groupLatestByName(rows))}
	h.render(w, cardGridTemplate, "card-grid", data)
}

// glyphFor picks the two-letter chip a plugin's card and header carry. The mockups colour the
// plugins that are Draft services themselves; everything else is a generic plugin.
func glyphFor(name string) (code, class string) {
	switch {
	case strings.HasPrefix(name, "catalyst-"):
		return "Ca", "d-glyph--ca"
	case name == "lineman":
		return "Lm", "d-glyph--bp"
	case name == "slack-notify":
		return "Sn", "d-glyph--fs"
	default:
		return "Pl", "d-glyph--primary"
	}
}

// groupLatestByName collapses store rows (one per published *version*) down
// to one cardData per plugin *name*, keeping whichever row has the latest
// PublishedAt — the design doc's card grid shows "name, description, latest
// version, maintainer", not one card per version — plus every version's tag.
// Returned sorted by name for a stable, deterministic grid ordering
// regardless of publish order.
func groupLatestByName(rows []*pluginRow) []cardData {
	type group struct {
		latest *pluginRow
		rows   []*pluginRow
	}
	groups := make(map[string]*group, len(rows))
	for _, row := range rows {
		g, ok := groups[row.Name]
		if !ok {
			g = &group{}
			groups[row.Name] = g
		}
		g.rows = append(g.rows, row)
		if g.latest == nil || row.PublishedAt.After(g.latest.PublishedAt) {
			g.latest = row
		}
	}

	cards := make([]cardData, 0, len(groups))
	for _, g := range groups {
		sort.SliceStable(g.rows, func(i, j int) bool {
			if !g.rows[i].PublishedAt.Equal(g.rows[j].PublishedAt) {
				return g.rows[i].PublishedAt.Before(g.rows[j].PublishedAt)
			}
			return g.rows[i].Version < g.rows[j].Version
		})
		versions := make([]string, 0, len(g.rows))
		tags := make([]draftui.Tag, 0, len(g.rows))
		for i, r := range g.rows {
			versions = append(versions, r.Version)
			tone := "quiet"
			if len(g.rows) > 1 && i == len(g.rows)-1 {
				tone = "primary"
			}
			tags = append(tags, draftui.Tag{Tone: tone, Text: r.Version})
		}
		glyph, class := glyphFor(g.latest.Name)
		cards = append(cards, cardData{
			Name:          g.latest.Name,
			Description:   g.latest.Description,
			LatestVersion: g.latest.Version,
			Maintainer:    g.latest.Maintainer,
			PublishedAt:   g.latest.PublishedAt,
			Versions:      versions,
			VersionTags:   tags,
			Required:      requiredCount(g.latest.ConfigSchema),
			Glyph:         glyph,
			GlyphClass:    class,
		})
	}
	sort.Slice(cards, func(i, j int) bool { return cards[i].Name < cards[j].Name })
	return cards
}

// requiredCount is how many fields a config_schema's top-level "required" array names.
func requiredCount(raw json.RawMessage) int {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		return 0
	}
	return len(schema.Required)
}

// schemaField is one row of the plugin detail page's config-schema table.
type schemaField struct {
	Name        string
	Type        string
	Required    bool
	Description string
	// Default is the schema's "default", rendered as text ("" when there is none).
	Default string
}

type versionRow struct {
	Version   string
	Published string
	Current   bool
}

type detailPageData struct {
	draftui.Page
	Name        string
	Version     string
	Versions    []versionRow // every published version of Name, newest first
	Description string
	Maintainer  string
	Source      string
	Published   string

	SchemaFields  []schemaField
	RequiredCount int
	OptionalCount int
	UsesSnippet   string
}

type notFoundPageData struct {
	draftui.Page
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
			h.renderNotFound(w, r, fmt.Sprintf("No published plugin matches %s@%s.", name, version))
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
	versions := make([]versionRow, 0, len(versionRows))
	for _, vr := range versionRows {
		versions = append(versions, versionRow{Version: vr.Version, Published: formatDate(vr.PublishedAt), Current: vr.Version == row.Version})
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
	required := 0
	for _, f := range fields {
		if f.Required {
			required++
		}
	}

	data := detailPageData{
		Page:          kit.NewPage(r, name, "Foundry", "Catalog", name),
		Name:          row.Name,
		Version:       row.Version,
		Versions:      versions,
		Description:   row.Description,
		Maintainer:    row.Maintainer,
		Source:        row.Source,
		Published:     formatDate(row.PublishedAt),
		SchemaFields:  fields,
		RequiredCount: required,
		OptionalCount: len(fields) - required,
		UsesSnippet:   usesSnippet(row.Name, row.Version, fields),
	}
	// Counts for the shell are best-effort: a failure here must not take the page down.
	if s, _, err := h.summary(ctx); err == nil {
		s.decorate(&data.Page, r, catalogView{sort: sortName})
		// On a plugin page no Catalog item is current.
		for i := range data.Rail {
			for j := range data.Rail[i].Items {
				data.Rail[i].Items[j].Current = false
			}
		}
	}
	data.Left = append([]draftui.BarItem{draftui.KV("Plugin", fmt.Sprintf("%s@%s", row.Name, row.Version))}, data.Left...)
	h.render(w, detailTemplate, "shell", data)
}

func formatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("Jan 2, 2006")
}

func (h *uiHandler) renderNotFound(w http.ResponseWriter, r *http.Request, message string) {
	w.WriteHeader(http.StatusNotFound)
	h.render(w, notFoundTemplate, "shell", notFoundPageData{
		Page:    kit.NewPage(r, "Not found", "Foundry", "Not found"),
		Message: message,
	})
}

// schemaFields walks a plugin's config_schema (stored as raw JSON Schema —
// see model.go's pluginRow.ConfigSchema) into the rows the detail page's
// table renders: for each key under "properties", its declared "type",
// whether it's named in the top-level "required" array, and its "description"
// and "default". This only walks the shape the design doc's example manifest
// uses (a flat "object" schema with scalar-typed properties) — nested/composed
// schemas (oneOf, nested objects, $ref) render with a best-effort Type string
// rather than being fully modeled, since nothing published so far needs more
// than that.
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

	// Required fields first (they are what a workflow author must fill in), then the optional
	// ones; each group alphabetical so the order is stable.
	names := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if required[names[i]] != required[names[j]] {
			return required[names[i]]
		}
		return names[i] < names[j]
	})

	fields := make([]schemaField, 0, len(names))
	for _, name := range names {
		var prop struct {
			Type        json.RawMessage `json:"type"`
			Description string          `json:"description"`
			Default     json.RawMessage `json:"default"`
		}
		fieldType := "any"
		if err := json.Unmarshal(schema.Properties[name], &prop); err == nil && len(prop.Type) > 0 {
			fieldType = propertyTypeString(prop.Type)
		}
		fields = append(fields, schemaField{
			Name:        name,
			Type:        fieldType,
			Required:    required[name],
			Description: prop.Description,
			Default:     defaultString(prop.Default),
		})
	}
	return fields, nil
}

// defaultString renders a JSON Schema "default" as display text: strings unquoted, everything else
// as compact JSON. Empty when the schema declares none.
func defaultString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return strings.TrimSpace(string(raw))
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
// snippet panel shows, matching the exact `uses: foundry://name@version`
// reference syntax from Bench's worked example
// (docs/website/content/docs/architecture/bench-workflow-engine.md's
// Primitives section: "uses: foundry://slack-notify@v2"). The `with:` block
// is populated from the schema's required fields (placeholder values named
// after their type) so the snippet answers "what do I put in with: for this
// plugin, concretely" — the exact question the design doc's UI section
// calls out — directly from data, without a plugin author separately
// maintaining example docs that can drift from the schema.
func usesSnippet(name, version string, fields []schemaField) string {
	var b strings.Builder
	fmt.Fprintf(&b, "- name: use-%s\n", name)
	fmt.Fprintf(&b, "  uses: foundry://%s@%s\n", name, version)

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

// render executes tmpl's named entry point against data into a buffer first,
// so a template error becomes a clean 500 rather than a half-written page
// (html/template would otherwise have sent headers before failing).
func (h *uiHandler) render(w http.ResponseWriter, tmpl *template.Template, name string, data any) {
	if err := kit.Render(w, tmpl, name, data); err != nil {
		h.logger.WithError(err).Error("failed to render template")
	}
}
