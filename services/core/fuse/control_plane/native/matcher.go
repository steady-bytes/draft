package native

import (
	"strings"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
)

// matchRoute reproduces Envoy's own two-phase routing algorithm --
// selecting the most specific matching virtual host by domain first, then
// the best-matching route within it by path -- so a request resolves
// identically regardless of which ProxyBackend is active. Doing this in
// one flat pass over every route (scoring purely by path-prefix length)
// would be wrong: it could let a long prefix on the host-less "default"
// virtual host beat a short prefix on a route with a specific host, which
// is the opposite of what Envoy does (virtual host selection by domain
// always happens before route selection by path).
func matchRoute(t *RouteTable, host, path string) (*ntv1.Route, bool) {
	scope := selectHostScope(t.routes, host)
	if scope == nil {
		return nil, false
	}
	return matchPath(scope, path)
}

// selectHostScope picks which group of routes to search, mirroring the
// three VirtualHost.Domains shapes makeRouterConfig ever emits: an exact
// per-host virtual host, a leading-wildcard virtual host, or the shared
// no-host "default" virtual host (Domains: ["*"]) used as a fallback.
// Exact beats wildcard beats default, matching Envoy's own
// most-specific-domain-wins selection.
func selectHostScope(routes []*ntv1.Route, host string) []*ntv1.Route {
	var exact, none []*ntv1.Route
	var bestWildcardHost string
	bestWildcardLen := -1

	for _, r := range routes {
		h := r.GetMatch().GetHost()
		switch {
		case h == "":
			none = append(none, r)
		case h == host:
			exact = append(exact, r)
		case isWildcardMatch(h, host) && len(h) > bestWildcardLen:
			bestWildcardLen = len(h)
			bestWildcardHost = h
		}
	}
	if len(exact) > 0 {
		return exact
	}
	if bestWildcardHost != "" {
		var wildcard []*ntv1.Route
		for _, r := range routes {
			if r.GetMatch().GetHost() == bestWildcardHost {
				wildcard = append(wildcard, r)
			}
		}
		return wildcard
	}
	if len(none) > 0 {
		return none
	}
	return nil
}

// isWildcardMatch matches a "*.draft.localhost"-style route host against a
// request host, the same leading-wildcard convention rpc.go's hostPattern
// validates on registration. Requires at least one label before the
// suffix -- "draft.localhost" itself does not match "*.draft.localhost",
// only a strict subdomain of it.
func isWildcardMatch(routeHost, requestHost string) bool {
	suffix, ok := strings.CutPrefix(routeHost, "*.")
	if !ok {
		return false
	}
	wildcardSuffix := "." + suffix
	return strings.HasSuffix(requestHost, wildcardSuffix) && len(requestHost) > len(wildcardSuffix)
}

// routeForMTLS reports whether any route sharing host's virtual-host scope
// (the same exact/wildcard/default selection matchRoute uses) requires
// mTLS, and if so, returns it. Deliberately connection-level, not
// per-path: a TLS ClientHello is processed once per connection, before any
// HTTP path is known, so "does this host need a client cert" can only ever
// be answered per host-scope, not per route within it. If two routes in
// the same scope both enable mTLS with different trusted_ca_secret_name
// values, the first one found wins -- a real, known limitation of
// connection-level enforcement, not a silent arbitrary choice: a cluster
// that needs two different client-CA trust bundles on the same host
// cannot express that with TLS-layer enforcement alone, on any backend.
func routeForMTLS(t *RouteTable, host string) (*ntv1.Route, bool) {
	for _, r := range selectHostScope(t.routes, host) {
		if r.GetMtls().GetEnabled() {
			return r, true
		}
	}
	return nil, false
}

// matchPath picks the best route within an already-host-selected scope:
// an EXACT match wins outright the moment its path matches (identical
// (host, EXACT, path) tuples are already rejected as a conflict at
// registration, so at most one can match here); otherwise the longest
// matching PREFIX wins, the same ordering makeRouterConfig's
// slices.SortFunc compiles into Envoy's own route list.
func matchPath(routes []*ntv1.Route, path string) (*ntv1.Route, bool) {
	var best *ntv1.Route
	bestLen := -1
	for _, r := range routes {
		m := r.GetMatch()
		if m.GetMatchType() == ntv1.MatchType_MATCH_TYPE_EXACT {
			if m.GetPrefix() == path {
				return r, true
			}
			continue
		}
		if prefix := m.GetPrefix(); strings.HasPrefix(path, prefix) && len(prefix) > bestLen {
			best, bestLen = r, len(prefix)
		}
	}
	return best, best != nil
}
