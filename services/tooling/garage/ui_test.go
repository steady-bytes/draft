package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

	if !strings.Contains(got, "uses: garage://slack-notify@v2") {
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
	if !strings.Contains(got, "uses: garage://noop@v1") {
		t.Errorf("snippet missing exact uses: reference syntax, got:\n%s", got)
	}
	if !strings.Contains(got, "with: {}") {
		t.Errorf("snippet should render an empty with: block when there are no required fields, got:\n%s", got)
	}
}

func TestGroupLatestByName(t *testing.T) {
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	rows := []*pluginRow{
		{Name: "slack-notify", Version: "v1", PublishedAt: older, Description: "old"},
		{Name: "slack-notify", Version: "v2", PublishedAt: newer, Description: "new", Maintainer: "platform-team"},
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
}

// TestRenderCatalogPage asserts the full catalog page (base+content+card-grid
// templates combined) renders real plugin data: names, versions,
// maintainers, and the htmx-wired search input all present in the output.
func TestRenderCatalogPage(t *testing.T) {
	data := catalogPageData{
		Cards: []cardData{
			{Name: "slack-notify", Description: "Posts a message to a Slack channel.", LatestVersion: "v2", Maintainer: "platform-team"},
			{Name: "fixture-seed", Description: "Seeds fixture rows before a run.", LatestVersion: "v1", Maintainer: "qa-team"},
		},
	}

	rec := httptest.NewRecorder()
	if err := catalogTemplate.ExecuteTemplate(rec, "base", data); err != nil {
		t.Fatalf("failed to render catalog page: %v", err)
	}
	body := rec.Body.String()

	for _, want := range []string{
		"slack-notify", "v2", "platform-team",
		"fixture-seed", "v1", "qa-team",
		`hx-get="/partials/catalog-cards"`,
		`id="card-grid"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered catalog page missing %q; body:\n%s", want, body)
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
}

// TestRenderDetailPage asserts the plugin detail page renders the config
// schema table and the exact copy-pasteable uses: snippet for real data.
func TestRenderDetailPage(t *testing.T) {
	fields := []schemaField{
		{Name: "channel", Type: "string", Required: true},
		{Name: "message", Type: "string", Required: true},
	}
	data := detailPageData{
		Name:         "slack-notify",
		Version:      "v2",
		Versions:     []string{"v2", "v1"},
		Description:  "Posts a message to a Slack channel via an incoming webhook.",
		Maintainer:   "platform-team",
		Source:       "https://github.com/steady-bytes/draft-plugins/tree/main/slack-notify",
		SchemaFields: fields,
		UsesSnippet:  usesSnippet("slack-notify", "v2", fields),
	}

	rec := httptest.NewRecorder()
	if err := detailTemplate.ExecuteTemplate(rec, "base", data); err != nil {
		t.Fatalf("failed to render detail page: %v", err)
	}
	body := rec.Body.String()

	for _, want := range []string{
		"slack-notify",
		"platform-team",
		"channel", "message",
		"badge-warning", // "required" badge
		"uses: garage://slack-notify@v2",
		"tab-active",
		`href="/plugins/slack-notify/v1"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered detail page missing %q; body:\n%s", want, body)
		}
	}
}

func TestRenderNotFoundPage(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := notFoundTemplate.ExecuteTemplate(rec, "base", notFoundPageData{Message: "No published plugin matches nope@v9."}); err != nil {
		t.Fatalf("failed to render not-found page: %v", err)
	}
	if body := rec.Body.String(); !strings.Contains(body, "No published plugin matches nope@v9.") {
		t.Errorf("rendered not-found page missing message, body:\n%s", body)
	}
}
