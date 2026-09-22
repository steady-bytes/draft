package main

import (
	"context"
	"errors"
	"testing"

	plugincatalogv1 "github.com/steady-bytes/draft/api/tooling/plugin_catalog/v1"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
)

// noopLogger is a minimal chassis.Logger for tests: it satisfies the
// interface without writing anything anywhere, so test output stays limited
// to what `go test` itself reports (no need for a real logger's Start(config)
// to have been called, unlike zerolog.New()'s implementation).
type noopLogger struct{}

func (noopLogger) Start(chassis.Config)                         {}
func (noopLogger) SetLevel(chassis.LogLevel)                    {}
func (noopLogger) GetLevel() chassis.LogLevel                   { return chassis.InfoLevel }
func (noopLogger) Wrap(err error) error                         { return err }
func (l noopLogger) WithError(error) chassis.Logger             { return l }
func (l noopLogger) WithContext(context.Context) chassis.Logger { return l }
func (l noopLogger) WithField(string, any) chassis.Logger       { return l }
func (l noopLogger) WithFields(chassis.Fields) chassis.Logger   { return l }
func (l noopLogger) WithCallDepth(int) chassis.Logger           { return l }
func (noopLogger) Trace(string)                                 {}
func (noopLogger) Debug(string)                                 {}
func (noopLogger) Debugf(string, ...any)                        {}
func (noopLogger) Info(string)                                  {}
func (noopLogger) Infof(string, ...any)                         {}
func (noopLogger) Warn(string)                                  {}
func (noopLogger) Warnf(string, ...any)                         {}
func (noopLogger) Error(string)                                 {}
func (noopLogger) Errorf(string, ...any)                        {}
func (noopLogger) WrappedError(error, string)                   {}
func (noopLogger) Fatal(string)                                 {}
func (noopLogger) Panic(string)                                 {}

var _ chassis.Logger = noopLogger{}

func newTestHandler(t *testing.T) Handler {
	t.Helper()
	return NewHandler(noopLogger{}, newTestStore(t))
}

func connectCode(t *testing.T, err error) connect.Code {
	t.Helper()
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected a *connect.Error, got %T: %v", err, err)
	}
	return connectErr.Code()
}

func TestPublish_Success(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()

	resp, err := h.Publish(ctx, connect.NewRequest(&plugincatalogv1.PublishRequest{
		Name:        "slack-notify-publish-success",
		Version:     "v1",
		Description: "Posts a message to a Slack channel via an incoming webhook.",
		Maintainer:  "platform-team",
		Source:      "https://github.com/steady-bytes/draft-plugins/tree/main/slack-notify",
	}))
	if err != nil {
		t.Fatalf("Publish returned unexpected error: %v", err)
	}

	got := resp.Msg.GetPlugin()
	if got.GetName() != "slack-notify-publish-success" {
		t.Errorf("Name = %q, want %q", got.GetName(), "slack-notify-publish-success")
	}
	if got.GetVersion() != "v1" {
		t.Errorf("Version = %q, want %q", got.GetVersion(), "v1")
	}
	if got.GetPublishedAt() == nil {
		t.Error("PublishedAt is nil, want a timestamp")
	}
}

func TestPublish_MissingFields(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()

	tests := []struct {
		name string
		req  *plugincatalogv1.PublishRequest
	}{
		{"missing name", &plugincatalogv1.PublishRequest{Version: "v1"}},
		{"missing version", &plugincatalogv1.PublishRequest{Name: "no-version"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := h.Publish(ctx, connect.NewRequest(tt.req))
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if code := connectCode(t, err); code != connect.CodeInvalidArgument {
				t.Errorf("code = %v, want %v", code, connect.CodeInvalidArgument)
			}
		})
	}
}

func TestPublish_DuplicateNameVersionRejected(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()

	req := connect.NewRequest(&plugincatalogv1.PublishRequest{
		Name:    "slack-notify-publish-duplicate",
		Version: "v1",
	})

	if _, err := h.Publish(ctx, req); err != nil {
		t.Fatalf("first Publish returned unexpected error: %v", err)
	}

	_, err := h.Publish(ctx, req)
	if err == nil {
		t.Fatal("expected an error publishing a duplicate (name, version), got nil")
	}
	if code := connectCode(t, err); code != connect.CodeAlreadyExists {
		t.Errorf("code = %v, want %v", code, connect.CodeAlreadyExists)
	}
}

func TestGet_Found(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()

	if _, err := h.Publish(ctx, connect.NewRequest(&plugincatalogv1.PublishRequest{
		Name:        "slack-notify-get-found",
		Version:     "v2",
		Description: "test",
	})); err != nil {
		t.Fatalf("Publish returned unexpected error: %v", err)
	}

	resp, err := h.Get(ctx, connect.NewRequest(&plugincatalogv1.GetPluginRequest{
		Name:    "slack-notify-get-found",
		Version: "v2",
	}))
	if err != nil {
		t.Fatalf("Get returned unexpected error: %v", err)
	}
	if resp.Msg.GetName() != "slack-notify-get-found" || resp.Msg.GetVersion() != "v2" {
		t.Errorf("got %s@%s, want slack-notify-get-found@v2", resp.Msg.GetName(), resp.Msg.GetVersion())
	}
}

func TestGet_NotFound(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()

	_, err := h.Get(ctx, connect.NewRequest(&plugincatalogv1.GetPluginRequest{
		Name:    "does-not-exist",
		Version: "v1",
	}))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if code := connectCode(t, err); code != connect.CodeNotFound {
		t.Errorf("code = %v, want %v", code, connect.CodeNotFound)
	}
}

func TestRetract_NeverPublishedIsNoOp(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()

	_, err := h.Retract(ctx, connect.NewRequest(&plugincatalogv1.RetractRequest{
		Name:    "never-published",
		Version: "v1",
	}))
	if err != nil {
		t.Fatalf("Retract of a never-published (name, version) returned an error, want no-op: %v", err)
	}
}

func TestRetract_RemovesPublishedVersion(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()

	if _, err := h.Publish(ctx, connect.NewRequest(&plugincatalogv1.PublishRequest{
		Name:    "slack-notify-retract",
		Version: "v1",
	})); err != nil {
		t.Fatalf("Publish returned unexpected error: %v", err)
	}

	if _, err := h.Retract(ctx, connect.NewRequest(&plugincatalogv1.RetractRequest{
		Name:    "slack-notify-retract",
		Version: "v1",
	})); err != nil {
		t.Fatalf("Retract returned unexpected error: %v", err)
	}

	_, err := h.Get(ctx, connect.NewRequest(&plugincatalogv1.GetPluginRequest{
		Name:    "slack-notify-retract",
		Version: "v1",
	}))
	if code := connectCode(t, err); code != connect.CodeNotFound {
		t.Errorf("Get after Retract: code = %v, want %v", code, connect.CodeNotFound)
	}

	// Retracting the same (name, version) again is still a no-op, not an error.
	if _, err := h.Retract(ctx, connect.NewRequest(&plugincatalogv1.RetractRequest{
		Name:    "slack-notify-retract",
		Version: "v1",
	})); err != nil {
		t.Fatalf("second Retract returned an error, want no-op: %v", err)
	}
}

func TestSearch_MatchesNameAndDescription(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()

	if _, err := h.Publish(ctx, connect.NewRequest(&plugincatalogv1.PublishRequest{
		Name:        "search-target-plugin",
		Version:     "v1",
		Description: "a distinctive marker sentence for search matching",
	})); err != nil {
		t.Fatalf("Publish returned unexpected error: %v", err)
	}

	byName, err := h.Search(ctx, connect.NewRequest(&plugincatalogv1.SearchPluginsRequest{Query: "search-target"}))
	if err != nil {
		t.Fatalf("Search by name returned unexpected error: %v", err)
	}
	if !containsPlugin(byName.Msg.GetPlugins(), "search-target-plugin", "v1") {
		t.Errorf("Search by name substring did not return search-target-plugin@v1: %v", byName.Msg.GetPlugins())
	}

	byDescription, err := h.Search(ctx, connect.NewRequest(&plugincatalogv1.SearchPluginsRequest{Query: "distinctive marker"}))
	if err != nil {
		t.Fatalf("Search by description returned unexpected error: %v", err)
	}
	if !containsPlugin(byDescription.Msg.GetPlugins(), "search-target-plugin", "v1") {
		t.Errorf("Search by description substring did not return search-target-plugin@v1: %v", byDescription.Msg.GetPlugins())
	}
}

func TestList_KeysetPagination(t *testing.T) {
	h := newTestHandler(t)
	ctx := context.Background()

	want := map[string]bool{
		"list-pagination-test-1@v1": false,
		"list-pagination-test-2@v1": false,
		"list-pagination-test-3@v1": false,
	}
	names := []string{"list-pagination-test-1", "list-pagination-test-2", "list-pagination-test-3"}
	for _, name := range names {
		if _, err := h.Publish(ctx, connect.NewRequest(&plugincatalogv1.PublishRequest{
			Name:    name,
			Version: "v1",
		})); err != nil {
			t.Fatalf("Publish(%s) returned unexpected error: %v", name, err)
		}
	}

	// Page through one row at a time (the table has rows from other tests too,
	// sharing the same database, so this walks every page until exhausted
	// rather than assuming a fixed total) and confirm every fixture published
	// above turns up exactly once, in the process proving the page_token
	// cursor actually advances instead of looping or skipping.
	seen := make(map[string]int)
	pageToken := ""
	for i := 0; i < 10000; i++ {
		resp, err := h.List(ctx, connect.NewRequest(&plugincatalogv1.ListPluginsRequest{
			PageSize:  1,
			PageToken: pageToken,
		}))
		if err != nil {
			t.Fatalf("List returned unexpected error: %v", err)
		}
		if len(resp.Msg.GetPlugins()) > 1 {
			t.Fatalf("List with page_size=1 returned %d plugins", len(resp.Msg.GetPlugins()))
		}
		for _, p := range resp.Msg.GetPlugins() {
			seen[p.GetName()+"@"+p.GetVersion()]++
		}
		pageToken = resp.Msg.GetNextPageToken()
		if pageToken == "" {
			break
		}
	}

	for key := range want {
		if seen[key] != 1 {
			t.Errorf("plugin %s seen %d times across pages, want exactly 1", key, seen[key])
		}
	}
}

func containsPlugin(plugins []*plugincatalogv1.Plugin, name, version string) bool {
	for _, p := range plugins {
		if p.GetName() == name && p.GetVersion() == version {
			return true
		}
	}
	return false
}
