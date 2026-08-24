package main

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	plugincatalogv1 "github.com/steady-bytes/draft/api/tooling/plugin_catalog/v1"
	plugincatalogv1connect "github.com/steady-bytes/draft/api/tooling/plugin_catalog/v1/v1connect"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// fakePluginCatalog is a minimal plugincatalogv1connect.PluginCatalogServiceHandler
// standing in for a real garage-compatible registry in tests -- List always
// succeeds (so it can double as the "reachable" check every add/update goes
// through), Get succeeds only for names in plugins.
type fakePluginCatalog struct {
	plugincatalogv1connect.UnimplementedPluginCatalogServiceHandler
	plugins map[string]bool // "name@version" -> published
}

func (f *fakePluginCatalog) List(ctx context.Context, req *connect.Request[plugincatalogv1.ListPluginsRequest]) (*connect.Response[plugincatalogv1.ListPluginsResponse], error) {
	return connect.NewResponse(&plugincatalogv1.ListPluginsResponse{}), nil
}

func (f *fakePluginCatalog) Get(ctx context.Context, req *connect.Request[plugincatalogv1.GetPluginRequest]) (*connect.Response[plugincatalogv1.Plugin], error) {
	key := req.Msg.GetName() + "@" + req.Msg.GetVersion()
	if !f.plugins[key] {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("not published"))
	}
	return connect.NewResponse(&plugincatalogv1.Plugin{Name: req.Msg.GetName(), Version: req.Msg.GetVersion()}), nil
}

// newFakeRegistryServer starts an httptest.Server behaving like a
// garage-compatible registry, h2c'd the same way grpc_call_test.go's
// newConnectJSONServer is for the same underlying reason (Connect over gRPC
// needs HTTP/2).
func newFakeRegistryServer(t *testing.T, plugins map[string]bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	pattern, handler := plugincatalogv1connect.NewPluginCatalogServiceHandler(&fakePluginCatalog{plugins: plugins})
	mux.Handle(pattern, handler)
	srv := httptest.NewServer(h2c.NewHandler(mux, &http2.Server{}))
	t.Cleanup(srv.Close)
	return srv
}

// unreachableAddr is a real listening TCP port immediately closed, so
// connections to it fail fast (connection refused) instead of timing out --
// used to test the "address doesn't answer" path of checkRegistryReachable.
func unreachableAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate an address: %v", err)
	}
	addr := "http://" + l.Addr().String()
	l.Close()
	return addr
}

func h2cTestClient() *http.Client {
	return &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLS: func(network, addr string, _ *tls.Config) (net.Conn, error) {
				return net.Dial(network, addr)
			},
		},
	}
}

func TestAddPluginRegistry_Success(t *testing.T) {
	store := newTestStore(t)
	srv := newFakeRegistryServer(t, nil)

	r, err := addPluginRegistry(context.Background(), store, h2cTestClient(), "registry-write-test-add", srv.URL)
	if err != nil {
		t.Fatalf("addPluginRegistry returned unexpected error: %v", err)
	}
	if r.GetAddress() != srv.URL {
		t.Errorf("address = %q, want %q", r.GetAddress(), srv.URL)
	}

	if _, err := store.GetPluginRegistry(context.Background(), "registry-write-test-add"); err != nil {
		t.Errorf("GetPluginRegistry after addPluginRegistry returned unexpected error: %v", err)
	}
}

func TestAddPluginRegistry_AlreadyExists(t *testing.T) {
	store := newTestStore(t)
	srv := newFakeRegistryServer(t, nil)
	ctx := context.Background()

	if _, err := addPluginRegistry(ctx, store, h2cTestClient(), "registry-write-test-dup", srv.URL); err != nil {
		t.Fatalf("first addPluginRegistry returned unexpected error: %v", err)
	}
	_, err := addPluginRegistry(ctx, store, h2cTestClient(), "registry-write-test-dup", srv.URL)
	if !errors.Is(err, ErrPluginRegistryAlreadyExists) {
		t.Errorf("second addPluginRegistry error = %v, want ErrPluginRegistryAlreadyExists", err)
	}
}

func TestAddPluginRegistry_UnreachableAddressRejected(t *testing.T) {
	store := newTestStore(t)
	_, err := addPluginRegistry(context.Background(), store, h2cTestClient(), "registry-write-test-unreachable", unreachableAddr(t))
	if err == nil {
		t.Fatal("addPluginRegistry with an unreachable address returned nil error, want an error")
	}
	if _, getErr := store.GetPluginRegistry(context.Background(), "registry-write-test-unreachable"); !errors.Is(getErr, ErrPluginRegistryNotFound) {
		t.Errorf("registry was persisted despite a failed reachability check (GetPluginRegistry err=%v)", getErr)
	}
}

func TestUpdatePluginRegistry_Success(t *testing.T) {
	store := newTestStore(t)
	srv1 := newFakeRegistryServer(t, nil)
	srv2 := newFakeRegistryServer(t, nil)
	ctx := context.Background()

	name := "registry-write-test-update"
	if _, err := addPluginRegistry(ctx, store, h2cTestClient(), name, srv1.URL); err != nil {
		t.Fatalf("addPluginRegistry returned unexpected error: %v", err)
	}

	r, err := updatePluginRegistry(ctx, store, h2cTestClient(), name, srv2.URL)
	if err != nil {
		t.Fatalf("updatePluginRegistry returned unexpected error: %v", err)
	}
	if r.GetAddress() != srv2.URL {
		t.Errorf("address = %q, want %q", r.GetAddress(), srv2.URL)
	}
}

func TestUpdatePluginRegistry_NotFound(t *testing.T) {
	store := newTestStore(t)
	srv := newFakeRegistryServer(t, nil)

	_, err := updatePluginRegistry(context.Background(), store, h2cTestClient(), "registry-write-test-missing", srv.URL)
	if !errors.Is(err, ErrPluginRegistryNotFound) {
		t.Errorf("error = %v, want ErrPluginRegistryNotFound", err)
	}
}
