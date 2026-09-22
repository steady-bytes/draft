package control_plane

import (
	"strings"
	"testing"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
)

// fakeBackend is a minimal ProxyBackend for testing checkCapabilities
// without a real Envoy or native backend (both require I/O -- an xDS
// server or Blueprint KV -- that a unit test shouldn't need).
type fakeBackend struct {
	caps BackendCapabilities
	name string
}

func (f *fakeBackend) Apply(routes []*ntv1.Route) error  { return nil }
func (f *fakeBackend) Capabilities() BackendCapabilities { return f.caps }
func (f *fakeBackend) Name() string                      { return f.name }

func TestCheckCapabilities_MTLSRejectedWhenUnsupported(t *testing.T) {
	cp := &controlPlane{backend: &fakeBackend{name: "envoy", caps: BackendCapabilities{MTLS: false}}}
	route := &ntv1.Route{Name: "secure", Mtls: &ntv1.MTLSPolicy{Enabled: true}}

	msg := cp.checkCapabilities(route)
	if msg == "" {
		t.Fatal("expected a rejection message for an mTLS route on a backend that doesn't support it")
	}
	if !strings.Contains(msg, "envoy") {
		t.Errorf("expected the rejection message to name the active backend, got %q", msg)
	}
}

func TestCheckCapabilities_MTLSAllowedWhenSupported(t *testing.T) {
	cp := &controlPlane{backend: &fakeBackend{name: "native", caps: BackendCapabilities{MTLS: true}}}
	route := &ntv1.Route{Name: "secure", Mtls: &ntv1.MTLSPolicy{Enabled: true}}

	if msg := cp.checkCapabilities(route); msg != "" {
		t.Errorf("expected no rejection, got %q", msg)
	}
}

func TestCheckCapabilities_NoMTLSRequirementAlwaysPasses(t *testing.T) {
	cp := &controlPlane{backend: &fakeBackend{name: "envoy", caps: BackendCapabilities{}}}

	cases := []*ntv1.Route{
		{Name: "no-mtls-field"},
		{Name: "mtls-present-but-disabled", Mtls: &ntv1.MTLSPolicy{Enabled: false}},
	}
	for _, route := range cases {
		if msg := cp.checkCapabilities(route); msg != "" {
			t.Errorf("route %q: expected no rejection when mTLS isn't requested, got %q", route.GetName(), msg)
		}
	}
}

// TestCheckCapabilities_OnlyChecksMTLS documents the deliberate scope
// limit: WideEvents (an opt-out that degrades safely -- a backend that
// can't produce one just doesn't) and TLS (a listener-level setting, not
// a per-route one) have nothing to reject here, unlike mTLS, where
// silently not enforcing a client-certificate requirement would be a real
// security downgrade rather than a graceful no-op. A backend that supports
// neither TLS nor WideEvents still accepts a route with no mtls
// requirement.
func TestCheckCapabilities_OnlyChecksMTLS(t *testing.T) {
	cp := &controlPlane{backend: &fakeBackend{name: "envoy", caps: BackendCapabilities{TLS: false, WideEvents: false, ACME: false}}}
	route := &ntv1.Route{Name: "plain", WideEventsDisabled: false}

	if msg := cp.checkCapabilities(route); msg != "" {
		t.Errorf("expected no rejection for a plain route regardless of TLS/WideEvents/ACME support, got %q", msg)
	}
}
