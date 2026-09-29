package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	draftui "github.com/steady-bytes/draft/tools/draft-ui"
)

// This file tests the Phase 4 UI in two layers: schemaFields/usesSnippet
// (pure functions, no HTTP/template involved) and the actual rendered HTML
// (asserting expected strings appear in ExecuteTemplate's output for known
// fixture data), per the brief's suggestion this is worthwhile and not much
// effort. It does not spin up Postgres or a real server — that's what the
// live `curl` smoke test against a running instance is for.

func TestSchemaFields(t *testing.T) {
	raw := json.RawMessage(`{
		"type": "object",
		"required": ["channel", "message"],
		"properties": {
			"channel": {"type": "string", "pattern": "^#"},
			"message": {"type": "string"},
			"retries": {"type": "integer"}
		}
	}`)

	fields, err := schemaFields(raw)
	if err != nil {
		t.Fatalf("schemaFields returned error: %v", err)
	}
	if len(fields) != 3 {
		t.Fatalf("expected 3 fields, got %d: %+v", len(fields), fields)
	}

	byName := make(map[string]schemaField, len(fields))
	for _, f := range fields {
		byName[f.Name] = f
	}

	if f := byName["channel"]; f.Type != "string" || !f.Required {
		t.Errorf("channel: got %+v, want type=string required=true", f)
	}
	if f := byName["message"]; f.Type != "string" || !f.Required {
		t.Errorf("message: got %+v, want type=string required=true", f)
	}
	if f := byName["retries"]; f.Type != "integer" || f.Required {
		t.Errorf("retries: got %+v, want type=integer required=false", f)
	}
}

func TestSchemaFieldsDescriptionAndDefault(t *testing.T) {
	raw := json.RawMessage(`{
		"required": ["address"],
		"properties": {
			"address": {"type": "string", "description": "Target server as host:port."},
			"timeout": {"type": "string", "description": "Per-call deadline.", "default": "5s"},
			"tls":     {"type": "boolean", "default": false},
			"retries": {"type": ["integer", "null"], "default": 3}
		}
	}`)
	fields, err := schemaFields(raw)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]schemaField{}
	for _, f := range fields {
		by[f.Name] = f
	}
	if by["address"].Description != "Target server as host:port." || by["address"].Default != "" {
		t.Errorf("address = %+v", by["address"])
	}
	if by["timeout"].Default != "5s" {
		t.Errorf("a string default is shown unquoted, got %q", by["timeout"].Default)
	}
	if by["tls"].Default != "false" || by["retries"].Default != "3" {
		t.Errorf("non-string defaults are shown as JSON: tls=%q retries=%q", by["tls"].Default, by["retries"].Default)
	}
	if by["retries"].Type != "integer | null" {
		t.Errorf("union type = %q", by["retries"].Type)
	}
}

func TestSchemaFieldsListRequiredFirst(t *testing.T) {
	raw := json.RawMessage(`{"required": ["method", "address"], "properties": {
		"address": {"type": "string"}, "expect": {"type": "object"}, "method": {"type": "string"}, "tls": {"type": "boolean"}}}`)
	fields, _ := schemaFields(raw)
	var names []string
	for _, f := range fields {
		names = append(names, f.Name)
	}
	if got := strings.Join(names, ","); got != "address,method,expect,tls" {
		t.Errorf("order = %s, want required first then optional, each alphabetical", got)
	}
}

func TestSchemaFieldsNil(t *testing.T) {
	for _, raw := range []json.RawMessage{nil, json.RawMessage("null"), json.RawMessage("")} {
		fields, err := schemaFields(raw)
		if err != nil {
			t.Fatalf("schemaFields(%q) returned error: %v", raw, err)
		}
		if fields != nil {
			t.Errorf("schemaFields(%q) = %+v, want nil", raw, fields)
		}
	}
}

func TestUsesSnippet(t *testing.T) {
	fields := []schemaField{
		{Name: "channel", Type: "string", Required: true},
		{Name: "message", Type: "string", Required: true},
		{Name: "retries", Type: "integer", Required: false},
	}

	got := usesSnippet("slack-notify", "v2", fields)

	if !strings.Contains(got, "uses: foundry://slack-notify@v2") {
		t.Errorf("snippet missing exact uses: reference syntax, got:\n%s", got)
	}
	if !strings.Contains(got, "channel: <string>") {
		t.Errorf("snippet missing required field channel, got:\n%s", got)
	}
	if strings.Contains(got, "retries") {
		t.Errorf("snippet should only include required fields, got:\n%s", got)
	}
}

func TestUsesSnippetNoRequiredFields(t *testing.T) {
	got := usesSnippet("noop", "v1", nil)
	if !strings.Contains(got, "uses: foundry://noop@v1") {
		t.Errorf("snippet missing exact uses: reference syntax, got:\n%s", got)
	}
	if !strings.Contains(got, "with: {}") {
		t.Errorf("snippet should render an empty with: block when there are no required fields, got:\n%s", got)
	}
}

func TestGroupLatestByName(t *testing.T) {
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	schema := json.RawMessage(`{"required": ["channel", "message"], "properties": {"channel": {"type": "string"}, "message": {"type": "string"}}}`)

	rows := []*pluginRow{
		{Name: "slack-notify", Version: "v1", PublishedAt: older, Description: "old"},
		{Name: "slack-notify", Version: "v2", PublishedAt: newer, Description: "new", Maintainer: "platform-team", ConfigSchema: schema},
		{Name: "fixture-seed", Version: "v1", PublishedAt: older, Description: "seeds fixtures", Maintainer: "qa-team"},
	}

	cards := groupLatestByName(rows)
	if len(cards) != 2 {
		t.Fatalf("expected 2 cards (deduped by name), got %d: %+v", len(cards), cards)
	}

	// sorted by name: fixture-seed, slack-notify
	if cards[0].Name != "fixture-seed" || cards[0].Maintainer != "qa-team" {
		t.Errorf("cards[0] = %+v, want fixture-seed/qa-team", cards[0])
	}
	if cards[1].Name != "slack-notify" || cards[1].LatestVersion != "v2" || cards[1].Description != "new" {
		t.Errorf("cards[1] = %+v, want slack-notify@v2 (\"new\"), the newer of the two versions", cards[1])
	}

	slack := cards[1]
	if strings.Join(slack.Versions, ",") != "v1,v2" {
		t.Errorf("versions = %v, want oldest first", slack.Versions)
	}
	// With more than one version the latest is the primary-coloured tag, the others quiet.
	if slack.VersionTags[0].Tone != "quiet" || slack.VersionTags[1].Tone != "primary" {
		t.Errorf("version tags = %+v", slack.VersionTags)
	}
	if slack.Required != 2 {
		t.Errorf("required = %d, want 2", slack.Required)
	}
	if only := cards[0].VersionTags; len(only) != 1 || only[0].Tone != "quiet" {
		t.Errorf("a single version is one quiet tag, got %+v", only)
	}
	if slack.Glyph != "Sn" || cards[0].Glyph != "Pl" {
		t.Errorf("glyphs = %q, %q", slack.Glyph, cards[0].Glyph)
	}
}

func TestGlyphFor(t *testing.T) {
	for name, want := range map[string]string{"catalyst-consume": "Ca", "catalyst-produce": "Ca", "lineman": "Lm", "slack-notify": "Sn", "http-call": "Pl"} {
		if got, _ := glyphFor(name); got != want {
			t.Errorf("glyphFor(%q) = %q, want %q", name, got, want)
		}
	}
}

func req(target string) *http.Request {
	r := httptest.NewRequest("GET", "http://foundry.draft.localhost:10000"+target, nil)
	r.Host = "foundry.draft.localhost:10000"
	return r
}

func TestCatalogViewParsingAndLinks(t *testing.T) {
	v := parseCatalogView(req("/?sort=newest&maintainer=platform-team&q=%20call%20"))
	if v.sort != sortNewest || v.maintainer != "platform-team" || v.query != "call" {
		t.Fatalf("view = %+v", v)
	}
	if got := parseCatalogView(req("/?sort=bogus")).sort; got != sortName {
		t.Errorf("an unknown sort falls back to name, got %q", got)
	}
	// Sort links keep the maintainer filter; the default sort needs no parameter.
	links := v.sortLinks()
	if links[0].Href != "/?maintainer=platform-team" || links[1].Href != "/?maintainer=platform-team&sort=newest" || !links[1].Active || links[0].Active {
		t.Errorf("links = %+v", links)
	}
	if got := (catalogView{sort: sortName}).href(sortName); got != "/" {
		t.Errorf("plain catalog href = %q", got)
	}
}

func TestCatalogViewFiltersAndSorts(t *testing.T) {
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := older.AddDate(0, 5, 0)
	cards := []cardData{
		{Name: "a", Maintainer: "x", PublishedAt: older},
		{Name: "b", Maintainer: "y", PublishedAt: newer},
		{Name: "c", Maintainer: "x", PublishedAt: newer.AddDate(0, 1, 0)},
	}
	names := func(cs []cardData) string {
		var n []string
		for _, c := range cs {
			n = append(n, c.Name)
		}
		return strings.Join(n, ",")
	}
	if got := names(catalogView{sort: sortName}.apply(cards)); got != "a,b,c" {
		t.Errorf("by name = %s", got)
	}
	if got := names(catalogView{sort: sortNewest}.apply(cards)); got != "c,b,a" {
		t.Errorf("newest first = %s", got)
	}
	if got := names(catalogView{sort: sortName, maintainer: "x"}.apply(cards)); got != "a,c" {
		t.Errorf("maintainer x = %s", got)
	}
}

func TestSummaryDecoratesTheShell(t *testing.T) {
	published := time.Date(2026, 9, 26, 1, 40, 0, 0, time.UTC)
	s := catalogSummary{
		cards:       []cardData{{Name: "a", Maintainer: "platform-team"}, {Name: "b", Maintainer: "platform-team"}, {Name: "c", Maintainer: "steady-bytes"}},
		versions:    4,
		last:        &pluginRow{Name: "lineman", Version: "v1", PublishedAt: published},
		maintainers: []maintainerCount{{"platform-team", 2}, {"steady-bytes", 1}},
	}
	r := req("/?maintainer=platform-team")
	page := kit.NewPage(r, "Plugin catalog", "Foundry", "Catalog")
	s.decorate(&page, r, parseCatalogView(r))

	if page.Status == nil || page.Status.Text != "3 plugins · 4 versions" || !page.Status.Live {
		t.Errorf("status = %+v", page.Status)
	}
	if len(page.Left) != 2 || page.Left[1].Value != "lineman@v1 · 01:40 UTC" {
		t.Errorf("status bar = %+v", page.Left)
	}
	var maint *draftui.NavSection
	for i := range page.Rail {
		if page.Rail[i].Label == "Maintainers" {
			maint = &page.Rail[i]
		}
	}
	if maint == nil || len(maint.Items) != 2 || maint.Items[0].Count != "2" || !maint.Items[0].Current || maint.Items[1].Current {
		t.Fatalf("maintainers section = %+v", maint)
	}
	// Filtering by a maintainer means "All plugins" is not the current item.
	for _, it := range page.Rail[0].Items {
		if it.Current {
			t.Errorf("%q should not be current while filtered", it.Label)
		}
	}
	if page.Rail[0].Items[0].Count != "3" {
		t.Errorf("all plugins count = %q", page.Rail[0].Items[0].Count)
	}
}

func catalogPage(t *testing.T, data catalogPageData) string {
	t.Helper()
	r := req("/")
	data.Page = kit.NewPage(r, "Plugin catalog", "Foundry", "Catalog")
	rec := httptest.NewRecorder()
	if err := catalogTemplate.ExecuteTemplate(rec, "shell", data); err != nil {
		t.Fatalf("failed to render catalog page: %v", err)
	}
	return rec.Body.String()
}

// TestRenderCatalogPage asserts the full catalog page (shell+content+card-grid
// templates combined) renders real plugin data: names, versions,
// maintainers, and the htmx-wired search input all present in the output.
func TestRenderCatalogPage(t *testing.T) {
	body := catalogPage(t, catalogPageData{
		Sort: sortName,
		Cards: []cardData{
			{Name: "slack-notify", Description: "Posts a message to a Slack channel.", LatestVersion: "v2", Maintainer: "platform-team", Required: 2, Glyph: "Sn", GlyphClass: "d-glyph--fs",
				VersionTags: []draftui.Tag{{Tone: "quiet", Text: "v1"}, {Tone: "primary", Text: "v2"}}},
			{Name: "fixture-seed", Description: "Seeds fixture rows before a run.", LatestVersion: "v1", Maintainer: "qa-team", Glyph: "Pl", GlyphClass: "d-glyph--primary",
				VersionTags: []draftui.Tag{{Tone: "quiet", Text: "v1"}}},
		},
		SortLinks: []sortLink{{Label: "A–Z", Href: "/", Active: true}, {Label: "Newest", Href: "/?sort=newest"}},
	})

	for _, want := range []string{
		"slack-notify", "v2", "platform-team",
		"fixture-seed", "v1", "qa-team",
		`hx-get="/partials/catalog-cards"`,
		`id="card-grid"`,
		`hx-include="[name='sort'],[name='maintainer']"`, // sort and maintainer ride along with a search
		`data-shortcut="/"`,
		`<span class="d-query-lang">SEARCH</span>`,
		`class="d-tag d-tag--primary">v2<`,
		`<b>foundry://slack-notify@v2</b>`,
		"2 required",
		`aria-current="true">A–Z<`,
		`class="d-glyph d-glyph--fs"`,
		"/static/draft/draft.css?v=",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered catalog page missing %q; body:\n%s", want, body)
		}
	}
	// The design system replaced daisyUI 4 and the Tailwind Play CDN.
	for _, banned := range []string{"cdn.tailwindcss.com", "daisyui", "cdn.jsdelivr.net", "data-theme=\"workshop\""} {
		if strings.Contains(body, banned) {
			t.Errorf("rendered catalog page still references %q", banned)
		}
	}
}

// TestRenderCatalogCardsEmpty asserts the no-results state renders sensibly
// rather than an empty/broken fragment when a search matches nothing.
func TestRenderCatalogCardsEmpty(t *testing.T) {
	data := catalogPageData{Query: "does-not-exist"}

	rec := httptest.NewRecorder()
	if err := cardGridTemplate.ExecuteTemplate(rec, "card-grid", data); err != nil {
		t.Fatalf("failed to render card grid fragment: %v", err)
	}
	body := rec.Body.String()

	if !strings.Contains(body, "does-not-exist") {
		t.Errorf("rendered empty-state fragment missing the query, body:\n%s", body)
	}
	if !strings.Contains(body, `class="d-empty"`) {
		t.Errorf("empty state should use the shared d-empty partial, body:\n%s", body)
	}
	// The fragment is swapped into an existing page: it must not carry a shell.
	if strings.Contains(body, "<html") || strings.Contains(body, "d-app") {
		t.Errorf("fragment rendered a full page:\n%s", body)
	}
}

func TestRenderCatalogEscapesTheQuery(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := cardGridTemplate.ExecuteTemplate(rec, "card-grid", catalogPageData{Query: `<script>alert(1)</script>`}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rec.Body.String(), "<script>alert") {
		t.Errorf("query was not escaped:\n%s", rec.Body.String())
	}
}

// TestRenderDetailPage asserts the plugin detail page renders the config
// schema table and the exact copy-pasteable uses: snippet for real data.
func TestRenderDetailPage(t *testing.T) {
	fields := []schemaField{
		{Name: "channel", Type: "string", Required: true, Description: "Channel to post to."},
		{Name: "message", Type: "string", Required: true},
		{Name: "timeout", Type: "string", Default: "5s"},
	}
	data := detailPageData{
		Page:          kit.NewPage(req("/plugins/slack-notify/v2"), "slack-notify", "Foundry", "Catalog", "slack-notify"),
		Name:          "slack-notify",
		Version:       "v2",
		Versions:      []versionRow{{Version: "v2", Published: "Sep 21, 2026", Current: true}, {Version: "v1", Published: "Sep 1, 2026"}},
		Description:   "Posts a message to a Slack channel via an incoming webhook.",
		Maintainer:    "platform-team",
		Source:        "https://github.com/steady-bytes/draft-plugins/tree/main/slack-notify",
		Published:     "Sep 21, 2026",
		SchemaFields:  fields,
		RequiredCount: 2,
		OptionalCount: 1,
		UsesSnippet:   usesSnippet("slack-notify", "v2", fields),
	}

	rec := httptest.NewRecorder()
	if err := detailTemplate.ExecuteTemplate(rec, "shell", data); err != nil {
		t.Fatalf("failed to render detail page: %v", err)
	}
	body := rec.Body.String()

	for _, want := range []string{
		"slack-notify",
		"platform-team",
		"channel", "message",
		"Channel to post to.",
		"Default <code>5s</code>.",
		"2 required · 1 optional",
		`class="d-tag d-tag--warn">required<`, // required tag
		`class="d-tag d-tag--quiet">optional<`,
		"foundry://slack-notify@v2", // in the highlighted snippet
		`id="snippet"`,
		`data-copy="#snippet"`,
		`<span class="tk-url">foundry://slack-notify@v2</span>`,
		`href="/plugins/slack-notify/v1"`,
		`aria-current="true">v2<`, // version segmented control
		"Published <b>Sep 21, 2026</b>",
		"Source ↗",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered detail page missing %q; body:\n%s", want, body)
		}
	}
}

func TestRenderDetailPageWithoutSchema(t *testing.T) {
	data := detailPageData{
		Page:    kit.NewPage(req("/plugins/noop/v1"), "noop", "Foundry", "Catalog", "noop"),
		Name:    "noop",
		Version: "v1",
	}
	rec := httptest.NewRecorder()
	if err := detailTemplate.ExecuteTemplate(rec, "shell", data); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rec.Body.String(), "didn&#39;t publish a config schema") {
		t.Errorf("missing no-schema empty state:\n%s", rec.Body.String())
	}
}

func TestRenderNotFoundPage(t *testing.T) {
	rec := httptest.NewRecorder()
	data := notFoundPageData{Page: kit.NewPage(req("/plugins/nope/v9"), "Not found", "Foundry", "Not found"), Message: "No published plugin matches nope@v9."}
	if err := notFoundTemplate.ExecuteTemplate(rec, "shell", data); err != nil {
		t.Fatalf("failed to render not-found page: %v", err)
	}
	if body := rec.Body.String(); !strings.Contains(body, "No published plugin matches nope@v9.") {
		t.Errorf("rendered not-found page missing message, body:\n%s", body)
	}
}
