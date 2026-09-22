package native

import (
	"testing"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
)

func route(name, host, prefix string, matchType ntv1.MatchType) *ntv1.Route {
	return &ntv1.Route{
		Name: name,
		Match: &ntv1.RouteMatch{
			Host:      host,
			Prefix:    prefix,
			MatchType: matchType,
		},
	}
}

// TestMatchRoute_HostScopePrecedence confirms exact host beats wildcard
// host beats the no-host "default" virtual host, even when the default
// scope's own route would otherwise win on path length alone -- this is
// exactly what a flat single-pass, score-by-prefix-length matcher (the
// implementation plan's illustrative pseudocode) would get wrong, since it
// never considers host specificity before scoring paths.
func TestMatchRoute_HostScopePrecedence(t *testing.T) {
	table := &RouteTable{routes: []*ntv1.Route{
		route("default-long", "", "/orders/v1/very/specific/path", ntv1.MatchType_MATCH_TYPE_PREFIX),
		route("exact-host-short", "orders.draft.localhost", "/", ntv1.MatchType_MATCH_TYPE_PREFIX),
		route("wildcard-host", "*.draft.localhost", "/", ntv1.MatchType_MATCH_TYPE_PREFIX),
	}}

	got, ok := matchRoute(table, "orders.draft.localhost", "/orders/v1/very/specific/path")
	if !ok {
		t.Fatal("expected a match")
	}
	if got.GetName() != "exact-host-short" {
		t.Errorf("expected exact-host route to win over a longer-prefix default-host route, got %q", got.GetName())
	}
}

func TestMatchRoute_WildcardBeatsDefault(t *testing.T) {
	table := &RouteTable{routes: []*ntv1.Route{
		route("default", "", "/", ntv1.MatchType_MATCH_TYPE_PREFIX),
		route("wildcard", "*.draft.localhost", "/", ntv1.MatchType_MATCH_TYPE_PREFIX),
	}}

	got, ok := matchRoute(table, "payments.draft.localhost", "/anything")
	if !ok || got.GetName() != "wildcard" {
		t.Errorf("expected wildcard route to win, got %q (ok=%v)", got.GetName(), ok)
	}
}

func TestMatchRoute_WildcardDoesNotMatchBareDomain(t *testing.T) {
	table := &RouteTable{routes: []*ntv1.Route{
		route("wildcard", "*.draft.localhost", "/", ntv1.MatchType_MATCH_TYPE_PREFIX),
	}}

	if _, ok := matchRoute(table, "draft.localhost", "/"); ok {
		t.Error("*.draft.localhost should not match the bare parent domain draft.localhost")
	}
}

func TestMatchRoute_ExactPathBeatsLongerPrefix(t *testing.T) {
	table := &RouteTable{routes: []*ntv1.Route{
		route("prefix-root", "api.draft.localhost", "/", ntv1.MatchType_MATCH_TYPE_PREFIX),
		route("prefix-long", "api.draft.localhost", "/orders", ntv1.MatchType_MATCH_TYPE_PREFIX),
		route("exact", "api.draft.localhost", "/orders", ntv1.MatchType_MATCH_TYPE_EXACT),
	}}

	got, ok := matchRoute(table, "api.draft.localhost", "/orders")
	if !ok || got.GetName() != "exact" {
		t.Errorf("expected EXACT route to win over PREFIX routes, got %q (ok=%v)", got.GetName(), ok)
	}
}

func TestMatchRoute_LongestPrefixWins(t *testing.T) {
	table := &RouteTable{routes: []*ntv1.Route{
		route("root", "api.draft.localhost", "/", ntv1.MatchType_MATCH_TYPE_PREFIX),
		route("orders", "api.draft.localhost", "/orders", ntv1.MatchType_MATCH_TYPE_PREFIX),
		route("orders-v1", "api.draft.localhost", "/orders/v1", ntv1.MatchType_MATCH_TYPE_PREFIX),
	}}

	got, ok := matchRoute(table, "api.draft.localhost", "/orders/v1/123")
	if !ok || got.GetName() != "orders-v1" {
		t.Errorf("expected longest-prefix route to win, got %q (ok=%v)", got.GetName(), ok)
	}
}

func TestMatchRoute_ExactPathMissDoesNotBlockOtherRoutes(t *testing.T) {
	table := &RouteTable{routes: []*ntv1.Route{
		route("exact-other", "api.draft.localhost", "/health", ntv1.MatchType_MATCH_TYPE_EXACT),
		route("prefix-root", "api.draft.localhost", "/", ntv1.MatchType_MATCH_TYPE_PREFIX),
	}}

	got, ok := matchRoute(table, "api.draft.localhost", "/orders")
	if !ok || got.GetName() != "prefix-root" {
		t.Errorf("expected fallback to prefix-root when the EXACT route doesn't match, got %q (ok=%v)", got.GetName(), ok)
	}
}

func TestMatchRoute_NoMatchIsDenyByDefault(t *testing.T) {
	table := &RouteTable{routes: []*ntv1.Route{
		route("only", "api.draft.localhost", "/", ntv1.MatchType_MATCH_TYPE_PREFIX),
	}}

	if _, ok := matchRoute(table, "unrelated.example.com", "/"); ok {
		t.Error("expected no match for a host with no matching virtual host and no default")
	}
}

func routeWithMTLS(name, host string, enabled bool, caName string) *ntv1.Route {
	r := route(name, host, "/", ntv1.MatchType_MATCH_TYPE_PREFIX)
	r.Mtls = &ntv1.MTLSPolicy{Enabled: enabled, TrustedCaSecretName: caName}
	return r
}

func TestRouteForMTLS_FindsEnabledRouteInHostScope(t *testing.T) {
	table := &RouteTable{routes: []*ntv1.Route{
		routeWithMTLS("plain", "secure.draft.localhost", false, ""),
		routeWithMTLS("protected", "secure.draft.localhost", true, "client-ca"),
	}}

	got, ok := routeForMTLS(table, "secure.draft.localhost")
	if !ok {
		t.Fatal("expected to find an mTLS-enabled route in this host's scope")
	}
	if got.GetName() != "protected" {
		t.Errorf("got route %q, want %q", got.GetName(), "protected")
	}
}

func TestRouteForMTLS_NoneEnabledReturnsFalse(t *testing.T) {
	table := &RouteTable{routes: []*ntv1.Route{
		route("plain", "open.draft.localhost", "/", ntv1.MatchType_MATCH_TYPE_PREFIX),
	}}

	if _, ok := routeForMTLS(table, "open.draft.localhost"); ok {
		t.Error("expected no mTLS requirement for a host with no mtls.enabled route")
	}
}

// TestRouteForMTLS_IsHostScopedNotPathScoped documents the real,
// unavoidable limitation routeForMTLS's own doc comment calls out: mTLS
// enforcement happens once per TLS connection (SNI is known before any
// HTTP path is), so a route on the same host as an mtls.enabled route is
// covered by that requirement even though its own RouteAuth/match never
// asked for one.
func TestRouteForMTLS_IsHostScopedNotPathScoped(t *testing.T) {
	table := &RouteTable{routes: []*ntv1.Route{
		routeWithMTLS("protected-api", "shared.draft.localhost", true, "client-ca"),
	}}

	// A different path on the same host still resolves to the same
	// mTLS-requiring scope, because selectHostScope groups by host only.
	got, ok := routeForMTLS(table, "shared.draft.localhost")
	if !ok || got.GetName() != "protected-api" {
		t.Fatalf("expected the host-wide mTLS requirement to apply, got %q (ok=%v)", got.GetName(), ok)
	}
}
