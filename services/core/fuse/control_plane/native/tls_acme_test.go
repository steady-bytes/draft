package native

import (
	"context"
	"sync/atomic"
	"testing"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
)

// TestACMEProvider_HostPolicyTracksLiveRouteTable is the real test for
// Phase 7's actual design decision: HostPolicy is not a static domain
// list (autocert.HostWhitelist) that has to be kept in sync with route
// changes -- it's a function that consults the live table on every call,
// via the exact same selectHostScope matchRoute already uses. This test
// exercises that decision directly, without touching a real ACME
// directory (which NewACMEProvider never contacts until a certificate is
// actually requested).
func TestACMEProvider_HostPolicyTracksLiveRouteTable(t *testing.T) {
	var table atomic.Pointer[RouteTable]
	table.Store(&RouteTable{routes: []*ntv1.Route{
		{Name: "orders", Match: &ntv1.RouteMatch{Host: "orders.example.com", Prefix: "/"}},
	}})

	p := NewACMEProvider(acmeDefaultDirectoryURL, t.TempDir(), &table)

	if err := p.m.HostPolicy(context.Background(), "orders.example.com"); err != nil {
		t.Errorf("expected a routed host to be authorized, got error: %v", err)
	}
	if err := p.m.HostPolicy(context.Background(), "not-routed.example.com"); err == nil {
		t.Error("expected an unrouted host to be refused")
	}

	// The whole point of tying HostPolicy to the live table rather than a
	// snapshot taken at construction: a route added after NewACMEProvider
	// is authorized immediately, with no rebuild step.
	table.Store(&RouteTable{routes: []*ntv1.Route{
		{Name: "orders", Match: &ntv1.RouteMatch{Host: "orders.example.com", Prefix: "/"}},
		{Name: "payments", Match: &ntv1.RouteMatch{Host: "payments.example.com", Prefix: "/"}},
	}})
	if err := p.m.HostPolicy(context.Background(), "payments.example.com"); err != nil {
		t.Errorf("expected a route added after construction to be authorized without a rebuild step, got error: %v", err)
	}

	// And a route that's since been removed loses authorization just as
	// immediately -- HostPolicy never caches a "yes" from an earlier call.
	table.Store(&RouteTable{routes: []*ntv1.Route{
		{Name: "payments", Match: &ntv1.RouteMatch{Host: "payments.example.com", Prefix: "/"}},
	}})
	if err := p.m.HostPolicy(context.Background(), "orders.example.com"); err == nil {
		t.Error("expected a removed route's host to lose ACME authorization immediately")
	}
}

func TestACMEProvider_HostPolicyRefusesWithoutTable(t *testing.T) {
	p := NewACMEProvider(acmeDefaultDirectoryURL, t.TempDir(), nil)
	if err := p.m.HostPolicy(context.Background(), "anything.example.com"); err == nil {
		t.Error("expected HostPolicy to refuse when constructed without a route table")
	}
}

func TestACMEProvider_GetCACertPoolUnsupported(t *testing.T) {
	p := NewACMEProvider(acmeDefaultDirectoryURL, t.TempDir(), nil)
	if _, err := p.GetCACertPool("anything"); err == nil {
		t.Error("expected the ACME provider to decline GetCACertPool, not guess at a trust source")
	}
}

func TestACMEProvider_ChangedIsNilChannel(t *testing.T) {
	p := NewACMEProvider(acmeDefaultDirectoryURL, t.TempDir(), nil)
	select {
	case <-p.Changed():
		t.Error("did not expect a value from Changed()")
	default:
		// Correct: ranging/receiving over this nil channel blocks forever
		// rather than panicking or ever firing -- see the Changed() doc
		// comment for why that's the intended behavior, not a bug.
	}
}
