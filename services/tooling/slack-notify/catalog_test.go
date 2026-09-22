package main

import (
	"context"
	"errors"
	"os"
	"testing"

	plugincatalogv1 "github.com/steady-bytes/draft/api/tooling/plugin_catalog/v1"
	plugincatalogv1connect "github.com/steady-bytes/draft/api/tooling/plugin_catalog/v1/v1connect"

	"connectrpc.com/connect"
)

// fakeCatalogClient is a plugincatalogv1connect.PluginCatalogServiceClient
// test double that records Publish/Retract calls without needing a real
// Foundry instance — used to prove foundryCatalogEffectSetup's setup/dispose
// wiring is correct in isolation. The real Publish/Retract flow against a
// live Foundry is proved separately (manually, and by
// TestFoundryCatalogEffect_Integration below, gated behind an env var since
// it needs Foundry actually running).
type fakeCatalogClient struct {
	publishReq  *plugincatalogv1.PublishRequest
	publishErr  error
	retractReq  *plugincatalogv1.RetractRequest
	retractErr  error
	publishCall int
	retractCall int
}

func (f *fakeCatalogClient) Publish(_ context.Context, req *connect.Request[plugincatalogv1.PublishRequest]) (*connect.Response[plugincatalogv1.PublishResponse], error) {
	f.publishCall++
	f.publishReq = req.Msg
	if f.publishErr != nil {
		return nil, f.publishErr
	}
	return connect.NewResponse(&plugincatalogv1.PublishResponse{
		Plugin: &plugincatalogv1.Plugin{Name: req.Msg.GetName(), Version: req.Msg.GetVersion()},
	}), nil
}

func (f *fakeCatalogClient) Retract(_ context.Context, req *connect.Request[plugincatalogv1.RetractRequest]) (*connect.Response[plugincatalogv1.RetractResponse], error) {
	f.retractCall++
	f.retractReq = req.Msg
	if f.retractErr != nil {
		return nil, f.retractErr
	}
	return connect.NewResponse(&plugincatalogv1.RetractResponse{}), nil
}

func (f *fakeCatalogClient) Get(context.Context, *connect.Request[plugincatalogv1.GetPluginRequest]) (*connect.Response[plugincatalogv1.Plugin], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not implemented in fake"))
}

func (f *fakeCatalogClient) List(context.Context, *connect.Request[plugincatalogv1.ListPluginsRequest]) (*connect.Response[plugincatalogv1.ListPluginsResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not implemented in fake"))
}

func (f *fakeCatalogClient) Search(context.Context, *connect.Request[plugincatalogv1.SearchPluginsRequest]) (*connect.Response[plugincatalogv1.SearchPluginsResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not implemented in fake"))
}

var _ plugincatalogv1connect.PluginCatalogServiceClient = (*fakeCatalogClient)(nil)

func TestFoundryCatalogEffectSetup_PublishesThenDisposeRetracts(t *testing.T) {
	fake := &fakeCatalogClient{}
	manifest, err := buildManifest()
	if err != nil {
		t.Fatalf("buildManifest returned unexpected error: %v", err)
	}

	setup := foundryCatalogEffectSetup(noopLogger{}, fake, manifest)

	dispose, err := setup()
	if err != nil {
		t.Fatalf("setup returned unexpected error: %v", err)
	}
	if fake.publishCall != 1 {
		t.Fatalf("Publish called %d times, want 1", fake.publishCall)
	}
	if fake.publishReq.GetName() != pluginName || fake.publishReq.GetVersion() != pluginVersion {
		t.Errorf("Publish called with %s@%s, want %s@%s", fake.publishReq.GetName(), fake.publishReq.GetVersion(), pluginName, pluginVersion)
	}
	if dispose == nil {
		t.Fatal("setup returned a nil dispose, want a Retract inverse")
	}

	if err := dispose(context.Background()); err != nil {
		t.Fatalf("dispose returned unexpected error: %v", err)
	}
	if fake.retractCall != 1 {
		t.Fatalf("Retract called %d times, want 1", fake.retractCall)
	}
	if fake.retractReq.GetName() != pluginName || fake.retractReq.GetVersion() != pluginVersion {
		t.Errorf("Retract called with %s@%s, want %s@%s", fake.retractReq.GetName(), fake.retractReq.GetVersion(), pluginName, pluginVersion)
	}
}

func TestFoundryCatalogEffectSetup_PublishFailurePropagates(t *testing.T) {
	fake := &fakeCatalogClient{publishErr: errors.New("boom")}
	manifest, err := buildManifest()
	if err != nil {
		t.Fatalf("buildManifest returned unexpected error: %v", err)
	}

	setup := foundryCatalogEffectSetup(noopLogger{}, fake, manifest)

	_, err = setup()
	if err == nil {
		t.Fatal("expected setup to propagate the Publish error, got nil")
	}
}

// TestFoundryCatalogEffect_Integration proves the publish/retract flow for
// real against a live Foundry instance (see the Phase 3 brief's testing
// section). It's skipped unless SLACK_NOTIFY_FOUNDRY_INTEGRATION=1 is set,
// since it requires Foundry (and its Postgres) actually running at
// foundry.address — go test ./... stays green without any infra by default.
//
// Manual run instructions: start Postgres + a real `foundry` binary per
// services/tooling/foundry's own Phase 1/2 notes, then:
//
//	SLACK_NOTIFY_FOUNDRY_INTEGRATION=1 FOUNDRY_ADDRESS=http://localhost:9301 \
//	    go test ./... -run TestFoundryCatalogEffect_Integration -v
func TestFoundryCatalogEffect_Integration(t *testing.T) {
	if os.Getenv("SLACK_NOTIFY_FOUNDRY_INTEGRATION") != "1" {
		t.Skip("set SLACK_NOTIFY_FOUNDRY_INTEGRATION=1 to run against a real, running Foundry instance")
	}

	addr := os.Getenv("FOUNDRY_ADDRESS")
	if addr == "" {
		addr = "http://localhost:9301"
	}

	client := newFoundryClient(addr)
	manifest, err := buildManifest()
	if err != nil {
		t.Fatalf("buildManifest returned unexpected error: %v", err)
	}
	// Use a throwaway version so a run against a real Foundry doesn't collide
	// with (or get confused for) the actual slack-notify@v2 entry a running
	// instance of this service would itself publish.
	manifest.Version = "integration-test"

	ctx := context.Background()

	// Retract first in case a previous failed run left this behind.
	_, _ = client.Retract(ctx, connect.NewRequest(&plugincatalogv1.RetractRequest{
		Name: manifest.GetName(), Version: manifest.GetVersion(),
	}))

	setup := foundryCatalogEffectSetup(noopLogger{}, client, manifest)
	dispose, err := setup()
	if err != nil {
		t.Fatalf("setup (Publish) returned unexpected error: %v", err)
	}

	got, err := client.Get(ctx, connect.NewRequest(&plugincatalogv1.GetPluginRequest{
		Name: manifest.GetName(), Version: manifest.GetVersion(),
	}))
	if err != nil {
		t.Fatalf("Get after publish returned unexpected error: %v", err)
	}
	if got.Msg.GetName() != manifest.GetName() {
		t.Errorf("Get returned Name = %q, want %q", got.Msg.GetName(), manifest.GetName())
	}

	if err := dispose(ctx); err != nil {
		t.Fatalf("dispose (Retract) returned unexpected error: %v", err)
	}

	_, err = client.Get(ctx, connect.NewRequest(&plugincatalogv1.GetPluginRequest{
		Name: manifest.GetName(), Version: manifest.GetVersion(),
	}))
	if err == nil {
		t.Fatal("Get after retract succeeded, want a not-found error")
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) || connectErr.Code() != connect.CodeNotFound {
		t.Errorf("Get after retract returned %v, want CodeNotFound", err)
	}
}

// TestNewFoundryClient proves newFoundryClient builds a usable client (doesn't
// panic on a garbage address, etc.) without needing anything running.
func TestNewFoundryClient(t *testing.T) {
	client := newFoundryClient("http://localhost:9301")
	if client == nil {
		t.Fatal("newFoundryClient returned nil")
	}
}
