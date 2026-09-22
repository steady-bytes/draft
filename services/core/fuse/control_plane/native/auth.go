package native

import (
	"context"
	"io"
	"net/http"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
	kvv1Connect "github.com/steady-bytes/draft/api/core/registry/key_value/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"
	cp "github.com/steady-bytes/draft/services/core/fuse/control_plane"
)

// authMiddleware is the native backend's ext_authz-equivalent: for a route
// with RouteAuth enabled, it forwards a check request to services/core/auth
// (whose address it discovers the same way EnvoyBackend does, via
// cp.GetAuthServiceAddress) and only proxies to the upstream on a 200
// response.
//
// The check request's shape below is not a guess -- it was verified against
// a real Envoy + services/core/auth run (see
// docs/website/content/docs/architecture/fuse-native-proxy-implementation-plan.md#phase-3--auth-middleware):
// Envoy's ext_authz HTTP filter, as configured by envoy_backend.go's
// makeExtAuthzFilter (no allowed_headers override), forwards the original
// request's method and path but only the Authorization header -- not
// Cookie, User-Agent, or any custom header. That live run is also what
// surfaced a real, separate bug (now fixed): services/core/auth's main.go
// was missing chassis.Runtime.DisableMux(), so chassis's own default empty
// RPC mux raced the service's manual check-server for the same port and
// consistently won, meaning every ext_authz check silently 403'd via
// FailureModeAllow: false before that fix, regardless of anything in Fuse.
type authMiddleware struct {
	client *http.Client
}

func newAuthMiddleware() *authMiddleware {
	return &authMiddleware{client: &http.Client{}}
}

// check returns true (allow) or false (deny, already written to w). Callers
// should skip calling check entirely for a route with no RouteAuth, or
// RouteAuth.Enabled == false, or Policy == AUTH_POLICY_BYPASS -- the same
// enable/disable gate makeExtAuthzFilter's per-route
// ExtAuthzPerRoute.Disabled already encodes.
//
// AUTH_POLICY_GROUPS and AUTH_POLICY_SCOPES are intentionally treated
// identically to AUTH_POLICY_AUTHENTICATED here: services/core/auth's own
// checkHandler/forwarder never reads Envoy's context_extensions today (read
// directly -- forwarder.go copies only r.Header, nothing derived from the
// policy), so there is no group/scope enforcement to replicate yet. This
// mirrors real behavior; it isn't a shortcut this middleware introduces.
func (m *authMiddleware) check(w http.ResponseWriter, r *http.Request, authAddr string) bool {
	checkReq, err := http.NewRequestWithContext(r.Context(), r.Method, authAddr+r.URL.Path, nil)
	if err != nil {
		http.Error(w, "failed to build auth check request", http.StatusInternalServerError)
		return false
	}
	// Only Authorization -- verified live, see the type comment above.
	if v := r.Header.Get("Authorization"); v != "" {
		checkReq.Header.Set("Authorization", v)
	}

	resp, err := m.client.Do(checkReq)
	if err != nil {
		// FailureModeAllow: false, matching makeExtAuthzFilter -- deny on an
		// unreachable check service, don't fail open.
		http.Error(w, "auth service unreachable", http.StatusServiceUnavailable)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Relay the denial verbatim, matching Envoy's own ext_authz
		// passthrough behavior ("Envoy returns the denial response to the
		// caller without contacting the upstream" -- authentication.md).
		for key, vals := range resp.Header {
			for _, v := range vals {
				w.Header().Add(key, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body) //nolint:errcheck
		return false
	}

	// Known parity gap, not fixed here: makeExtAuthzFilter never configures
	// allowed_upstream_headers, so identity headers a check response sets
	// (eg. X-Authentik-Username) do not reach the upstream on the envoy
	// backend today either -- verified live (auth-example's Secret RPC
	// reported "anonymous" even with a check response setting that header).
	// This middleware matches that real behavior rather than "fixing" it by
	// forwarding response headers Envoy itself doesn't.
	return true
}

// authAddress resolves the current auth service address via the same
// Blueprint KV lookup EnvoyBackend's Apply already used before
// cp.GetAuthServiceAddress was shared. Looked up fresh on every check
// rather than cached, matching this phase's simplicity; Phase 4+ may want
// to cache it (refreshed on Apply, the same cadence the envoy backend
// re-resolves it at) if the extra KV round trip per authenticated request
// turns out to matter.
func authAddress(ctx context.Context) string {
	client := kvv1Connect.NewKeyValueServiceClient(http.DefaultClient, chassis.GetConfig().Entrypoint())
	return cp.GetAuthServiceAddress(ctx, client)
}

// authRequired reports whether route's RouteAuth calls for a check at all --
// the same enable/disable gate makePerRouteAuthConfig already encodes for
// the envoy backend.
func authRequired(route *ntv1.Route) bool {
	auth := route.GetAuth()
	return auth != nil && auth.GetEnabled() && auth.GetPolicy() != ntv1.AuthPolicy_AUTH_POLICY_BYPASS
}
