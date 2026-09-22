package main

import (
	"context"
	"strings"
	"testing"

	settingsv1 "github.com/steady-bytes/draft/api/tooling/settings/v1"
)

// TestFindPluginInRegistries_FirstMatchWins proves Phase 11d's federated
// resolution directly: given multiple registries, the first one (in the
// order they're passed, matching store.go's ListPluginRegistries ordering
// by created_at ASC) that has (name, version) published wins, and
// registries before it that don't have it are silently skipped, not
// treated as errors.
func TestFindPluginInRegistries_FirstMatchWins(t *testing.T) {
	empty := newFakeRegistryServer(t, nil)
	hasIt := newFakeRegistryServer(t, map[string]bool{"slack-notify@v2": true})

	registries := []*settingsv1.PluginRegistry{
		{Name: "first-empty", Address: empty.URL},
		{Name: "second-has-it", Address: hasIt.URL},
	}

	got, err := findPluginInRegistries(context.Background(), h2cTestClient(), registries, "slack-notify", "v2")
	if err != nil {
		t.Fatalf("findPluginInRegistries returned unexpected error: %v", err)
	}
	if got != "second-has-it" {
		t.Errorf("matched registry = %q, want %q", got, "second-has-it")
	}
}

// TestFindPluginInRegistries_CollisionFirstAddedWins is the flip side: two
// registries both have it, and the earlier one in the list wins — the
// silent-collision behavior the design doc's Open questions section flags
// as deliberately unresolved, verified here to actually match what's
// documented rather than being an accident of iteration order.
func TestFindPluginInRegistries_CollisionFirstAddedWins(t *testing.T) {
	both := map[string]bool{"catalyst-consume@v1": true}
	first := newFakeRegistryServer(t, both)
	second := newFakeRegistryServer(t, both)

	registries := []*settingsv1.PluginRegistry{
		{Name: "added-first", Address: first.URL},
		{Name: "added-second", Address: second.URL},
	}

	got, err := findPluginInRegistries(context.Background(), h2cTestClient(), registries, "catalyst-consume", "v1")
	if err != nil {
		t.Fatalf("findPluginInRegistries returned unexpected error: %v", err)
	}
	if got != "added-first" {
		t.Errorf("matched registry = %q, want %q (the earlier-added one)", got, "added-first")
	}
}

func TestFindPluginInRegistries_NotPublishedAnywhere(t *testing.T) {
	a := newFakeRegistryServer(t, nil)
	b := newFakeRegistryServer(t, nil)

	registries := []*settingsv1.PluginRegistry{
		{Name: "reg-a", Address: a.URL},
		{Name: "reg-b", Address: b.URL},
	}

	_, err := findPluginInRegistries(context.Background(), h2cTestClient(), registries, "nonexistent-plugin", "v1")
	if err == nil {
		t.Fatal("findPluginInRegistries returned nil error for a plugin published nowhere, want an error")
	}
	if !strings.Contains(err.Error(), "reg-a") || !strings.Contains(err.Error(), "reg-b") {
		t.Errorf("error = %q, want it to name every registry checked", err.Error())
	}
}
