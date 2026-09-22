---
weight: 46
title: Fuse — Pluggable Proxy Backends Implementation Plan
description: Phased implementation plan for Fuse's ProxyBackend abstraction — extracting the existing Envoy control plane behind an interface, then adding a native reverse-proxy backend with TLS/mTLS and WideEvent logging alongside it.
icon: checklist
draft: false
toc: true
---

This document is the step-by-step implementation plan for [Fuse — Pluggable Proxy Backends](/docs/architecture/fuse-native-proxy), in the order work should be done. Each phase produces a runnable, testable artifact and does not block the next. File paths and existing code below are quoted directly from `services/core/fuse/{main.go,control_plane/controller.go,control_plane/rpc.go}`, `api/core/control_plane/networking/v1/service.proto`, and `pkg/chassis/{builder.go,otel_trace.go,wide_event.go}` as they exist today — Phases 1–4 of [Fuse — API Gateway](/docs/architecture/fuse-api-gateway) (route lifecycle correctness, gRPC-Web, subdomains, request tracing) are already shipped, so this plan builds on that code, not on the pre-Phase-1 state.

**Target end state (per [Fuse — Pluggable Proxy Backends](/docs/architecture/fuse-native-proxy)): `fuse.proxy_backend` defaults to `native`, and `envoy` requires explicit configuration.** This plan does not flip the literal default to `native` until **Phase 3** is done, not Phase 2 — routing parity (Phase 2) alone isn't enough, because `RouteAuth` enforcement (`AUTHENTICATED`/`GROUPS`/`SCOPES`) is a real capability existing Envoy-backed clusters depend on today, and flipping the default before the native backend can enforce it (Phase 3) would be a silent security regression for anyone who upgrades without setting `fuse.proxy_backend` explicitly. Concretely: **Phases 1 and 2 ship with the default still `envoy`**, and **Phase 3 is the phase that flips it to `native`**, once both routing parity (2.9) and auth parity (this phase) are proven. Every phase from Phase 3 onward should be read against a `native` default; Phases 1–2 should be read against `envoy`.

---

## Phase 1 — `ProxyBackend` Extraction

Pulls the Envoy-specific two-thirds of `controlPlane.apply()` out from behind route persistence, with no behavior change. This is the seam every later phase is built on.

### 1.1 Config key

Add to `services/core/fuse/config.yaml`'s `fuse:` stanza, and read it in `main.go`:

```yaml
fuse:
  address: http://localhost:18000    # control-plane / RPC port
  proxy_backend: envoy               # or "native"; defaults to "envoy" for now -- see 2.10
  listener:
    address: 0.0.0.0
    port: 10000                     # data-plane / proxy port -- must differ from address's port, see 1.7
```

```go
const ProxyBackendConfigKey = "fuse.proxy_backend"

// proxyBackendDefault is temporarily "envoy": there is no native backend to
// fall back to until Phase 2 ships and passes its parity suite (2.9). Phase
// 2.10 changes this literal to "native", which is the permanent, intended
// default -- this is not the final value, it's what Phase 1 alone can safely
// ship with.
const proxyBackendDefault = "envoy"

func proxyBackendName() string {
	if v := chassis.GetConfig().GetString(ProxyBackendConfigKey); v != "" {
		return v
	}
	return proxyBackendDefault
}
```

### 1.2 The `ProxyBackend` interface

New file `services/core/fuse/control_plane/backend.go`:

```go
package control_plane

import ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"

// ProxyBackend is implemented once per supported proxy. controlPlane's own
// route validation, conflict detection (FindConflicts), and Blueprint-KV
// persistence (UpdateCacheWithNewRoute, DeleteRoute, ListRoutes) are
// identical regardless of which backend is active -- Apply is the only
// backend-specific step, called with the full merged route table every
// time it changes.
type ProxyBackend interface {
	Apply(routes []*ntv1.Route) error
	Capabilities() BackendCapabilities
	Name() string
}

// BackendCapabilities reports what a backend can actually enforce, so a
// route/TLS/mTLS/WideEvent setting the active backend can't honor fails
// clearly (see Phase 8) instead of being silently accepted and ignored.
type BackendCapabilities struct {
	TLS        bool
	MTLS       bool
	WideEvents bool
	ACME       bool
}
```

### 1.3 Extract `envoyBackend` from `apply()`

Everything in today's `(cp *controlPlane) apply(ctx, client)` from `getAuthServiceAddress` through `cp.cache.SetSnapshot` (`controller.go:366-542`) is Envoy-specific and moves, close to verbatim, into a new type:

```go
// services/core/fuse/control_plane/envoy_backend.go
package control_plane

type envoyBackend struct {
	logger          chassis.Logger
	cache           cache.SnapshotCache
	xDSServer       server.Server
	listenerAddress string
	listenerPort    uint32
}

func NewEnvoyBackend(logger chassis.Logger) *envoyBackend { /* today's NewControlPlane body */ }

func (b *envoyBackend) Apply(routes []*ntv1.Route) error {
	ctx := context.Background()
	client := kvv1Connect.NewKeyValueServiceClient(http.DefaultClient, chassis.GetConfig().Entrypoint())
	// body is today's apply(), unchanged, starting from "authAddr :=
	// cp.getAuthServiceAddress(...)" -- it already only reads `routes` and
	// Blueprint KV (for auth-address/tracing lookups), never controlPlane
	// fields other than the ones moving here (cache, listenerAddress,
	// listenerPort, xDSServer's target cache).
}

func (b *envoyBackend) Capabilities() BackendCapabilities {
	return BackendCapabilities{TLS: false, MTLS: false, WideEvents: false, ACME: false}
	// flip TLS/MTLS to true once Fuse — API Gateway's Phases 5-6 ship;
	// flip WideEvents to true once the ALS path (see the design doc) ships.
}

func (b *envoyBackend) Name() string { return "envoy" }
```

`xDSServer` (the ADS gRPC server `XDSRpc.RegisterRPC` wires up) stays owned by `envoyBackend`, not `controlPlane` — `NewXDSRpc` now takes `*envoyBackend` instead of `*controlPlane` (see 1.5).

### 1.4 `controlPlane` shrinks to route persistence + one call to `Apply`

```go
type controlPlane struct {
	logger  chassis.Logger
	backend ProxyBackend
}

func NewControlPlane(logger chassis.Logger, backend ProxyBackend) *controlPlane {
	return &controlPlane{logger: logger, backend: backend}
}

func (cp *controlPlane) apply(ctx context.Context, client kvv1Connect.KeyValueServiceClient) error {
	raw, err := cp.listRawRoutes(ctx, client)
	if err != nil {
		return err
	}
	return cp.backend.Apply(mergeRoutes(raw))
}
```

`LoadCache`, `UpdateCacheWithNewRoute`, `DeleteRoute`, `ListRoutes`, `listRawRoutes`, `FindConflicts`, `mergeRoutes`, `storageKey`, `endpointKey`, `routeKey` are all untouched — none of them reference Envoy types today (confirmed by reading `controller.go` directly; this was true before this phase, which is what makes the extraction mechanical rather than a redesign).

### 1.5 `main.go`: construct the configured backend

```go
func main() {
	chassis.NewMetricsReporter().Start()
	var (
		logger  = chassis.NewOTelLogger()
		backend = newBackend(logger) // picks envoyBackend or nativeBackend per fuse.proxy_backend
		cp      = control_plane.NewControlPlane(logger, backend)
	)

	runtime := chassis.New(logger).
		Register(chassis.RegistrationOptions{Namespace: "core"}).
		WithRPCHandler(control_plane.NewRPC(logger, cp)).
		WithRunner(cp.LoadCache)

	if eb, ok := backend.(*control_plane.EnvoyBackend); ok {
		// xDS/ADS only makes sense when Envoy is the backend actually
		// consuming it -- registering it unconditionally would open a gRPC
		// surface that does nothing when the native backend is active.
		runtime = runtime.WithRPCHandler(control_plane.NewXDSRpc(logger, eb))
	}

	defer runtime.Start()
}

func newBackend(logger chassis.Logger) control_plane.ProxyBackend {
	switch chassis.GetConfig().GetString(control_plane.ProxyBackendConfigKey) {
	case "native":
		return control_plane.NewNativeBackend(logger) // Phase 2
	default:
		return control_plane.NewEnvoyBackend(logger)
	}
}
```

This is the same explicit-construction shape `chassis.Runtime.WithBroker(plugin Broker)`/`WithRepository(plugin Repository)` already use for message-broker and database plugins (`pkg/chassis/builder.go`) — a typed interface, picked and constructed by the caller — not a string-keyed registry. Adding a third backend later means one more `case` here plus its own constructor.

### 1.6 Regression check

No proto change, no wire change, no new dependency in this phase. Before moving to Phase 2: run `tests/fuse` and the bench `fuse-proxy-e2e.yaml`/`fuse-proxy-e2e-10x.yaml` workflows unmodified against the refactored code with `fuse.proxy_backend` unset (or explicitly `envoy`) and confirm identical behavior — this phase should be invisible to every existing test.

### 1.7 Port-separation validation

Add to `main.go`, before either backend is constructed. This matters starting with Phase 2's native backend, not this phase's `envoy`-only world, but the check is cheap and belongs next to the config key it validates:

```go
func validatePortSeparation() {
	controlPort := portOf(chassis.GetConfig().Entrypoint()) // fuse.address's port -- NetworkingService/RPC
	proxyPort := chassis.GetConfig().GetUint32(control_plane.LISTENER_PORT_CONFIG_KEY)
	if controlPort == proxyPort {
		panic(fmt.Sprintf(
			"fuse.listener.port (%d) must differ from fuse.address's port (%d) -- "+
				"the native backend opens both sockets in the same process and cannot bind one port twice",
			proxyPort, controlPort,
		))
	}
}
```

A panic (not a logged warning) is deliberate: on the `envoy` backend a collision here was merely confusing (two independent processes, each free to bind the same port number until one actually loses the race); on the `native` backend it's a guaranteed same-process double-bind failure moments later regardless, so failing immediately with a named cause beats the generic "address already in use" the OS would otherwise report.

---

## Phase 2 — Native Backend: Route Table & Reverse Proxy Core

**✅ Shipped** — `services/core/fuse/control_plane/native/{backend.go,matcher.go,h2c.go,matcher_test.go}`, selectable today via `fuse.proxy_backend: native`. Not the default yet — see Phase 3's closing note on why the flip waits for auth parity too. 2.9's parity suite (the full test matrix through both a real Blueprint/Fuse instance) is still outstanding — what shipped here is unit-tested at the matcher level (`matcher_test.go`) and verified by `go build`/`go vet`, not yet exercised end-to-end against a live Envoy comparison.

New package `services/core/fuse/control_plane/native/`, registered as the `native` `ProxyBackend`.

### 2.1 Route table

```go
type RouteTable struct {
	routes []*ntv1.Route // already merged (mergeRoutes), one entry per logical route
}

type nativeBackend struct {
	logger chassis.Logger
	table  atomic.Pointer[RouteTable]
	server *http.Server
}

func (b *nativeBackend) Apply(routes []*ntv1.Route) error {
	b.table.Store(&RouteTable{routes: routes})
	return nil
}
```

### 2.2 Matcher — same semantics as `makeRouterConfig`, consulted instead of compiled

**Shipped (`control_plane/native/matcher.go`), and corrected from the shape originally sketched here.** The flat single-pass version below looks reasonable but is wrong: it scores every route purely by path-prefix length, so a long prefix on the host-less "default" virtual host could beat a short prefix on a route with a specific host — the opposite of Envoy, which always selects the most specific matching *virtual host* by domain first, and only then matches routes by path within it. The two are independent axes; collapsing them into one score is the bug.

The shipped version is two-phase — `selectHostScope` (exact host beats wildcard beats the no-host default, mirroring `makeRouterConfig`'s `defaultVirtualHost`/per-host `VirtualHost` split) followed by `matchPath` (EXACT wins outright per `makeRouterConfig`'s `slices.SortFunc`, `controller.go:878-893`; otherwise longest PREFIX wins) — within the routes that phase one selected, not across the whole table. `matcher_test.go` locks in the case that would silently regress if this ever got flattened back to one pass: an exact-host route with a short prefix must beat a default (no-host) route with a much longer prefix.

For reference, the originally-sketched (incorrect) single-pass version:

```go
// INCORRECT -- kept here only to document why it was replaced. Scores
// every route by path length regardless of host specificity, so a
// long-prefix default-host route can incorrectly beat a short-prefix
// specific-host route.
func matchRoute(t *RouteTable, host, path string) (*ntv1.Route, bool) {
	var best *ntv1.Route
	bestLen := -1
	for _, r := range t.routes {
		if !hostMatches(r.Match.GetHost(), host) {
			continue
		}
		mt := r.Match.GetMatchType()
		if mt == ntv1.MatchType_MATCH_TYPE_EXACT {
			if r.Match.Prefix == path {
				return r, true
			}
			continue
		}
		if strings.HasPrefix(path, r.Match.Prefix) && len(r.Match.Prefix) > bestLen {
			best, bestLen = r, len(r.Match.Prefix)
		}
	}
	return best, best != nil
}
```

This matcher is a candidate to hoist into a package both backends import (e.g. `control_plane/matching`), so `envoyBackend`'s own conflict/ordering logic and this one don't drift independently — noted as a follow-up, not done as part of this phase.

### 2.3 Listener: HTTP/1.1 and h2c on the same port

Fuse's Envoy listener today accepts both without extra configuration (Envoy's `HttpConnectionManager` auto-detects the codec). The stdlib `http.Server` doesn't do this by default for cleartext HTTP/2 — wrap the handler with `golang.org/x/net/http2/h2c`:

```go
import "golang.org/x/net/http2/h2c"

h2s := &http2.Server{}
b.server = &http.Server{
	Addr:    fmt.Sprintf("%s:%d", listenerAddress, listenerPort),
	Handler: h2c.NewHandler(http.HandlerFunc(b.serveHTTP), h2s),
}
```

`golang.org/x/net` is already an indirect dependency of `services/core/fuse` today (pulled in transitively by `go-control-plane`); this phase makes it a direct one.

### 2.4 The proxy itself

```go
func (b *nativeBackend) serveHTTP(w http.ResponseWriter, r *http.Request) {
	t := b.table.Load()
	route, ok := matchRoute(t, stripPort(r.Host), r.URL.Path) // StripAnyHostPort equivalent -- see controller.go:492
	if !ok {
		http.NotFound(w, r) // deny-by-default, same as Envoy's "no matching virtual host"
		return
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(&url.URL{Scheme: "http", Host: endpointAddr(route)})
			// Envoy's default router does not rewrite the Host/authority header
			// (no HostRewriteSpecifier is set anywhere in controller.go) --
			// SetURL alone changes pr.Out.Host to the target's host, which
			// would silently diverge from that default. Preserve the
			// original inbound Host explicitly to match.
			pr.Out.Host = pr.In.Host
		},
	}
	if route.EnableHttp2 {
		proxy.Transport = h2cTransport // 2.5
	}
	proxy.ServeHTTP(w, r)
}
```

The `pr.Out.Host = pr.In.Host` line is the one real behavioral trap here, flagged the same way `service-ui-subdomains.md` flags the `StripAnyHostPort` gotcha — `httputil.ReverseProxy`'s default `Rewrite` (or the older `NewSingleHostReverseProxy`) overwrites the outbound Host to the target's, which is *not* what Envoy does today, and would break any upstream that relies on seeing the original inbound host (subdomain-per-service routing, in particular).

### 2.5 HTTP/2 to the upstream (`enable_http2`)

```go
var h2cTransport = &http2.Transport{
	AllowHTTP: true,
	DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
		return net.Dial(network, addr) // cleartext -- upstream is plain h2c, matches EnableHttp2's meaning today (no TLS between Fuse and the upstream cluster either way)
	},
}
```

Routes without `enable_http2` use `ReverseProxy`'s default `http.Transport` (HTTP/1.1), matching `makeCluster`'s own opt-in behavior (`controller.go:596`).

### 2.6 WebSockets

No code: `httputil.ReverseProxy` has proxied `Connection: Upgrade` requests via raw bidirectional byte copying since Go 1.12. Confirm with a manual test in Phase 9 rather than writing anything here.

### 2.7 gRPC-Web

No code, by design — see the design doc's [reverse proxy engine](/docs/architecture/fuse-native-proxy#the-reverse-proxy-engine) section. Confirm with a real Connect-Web client in Phase 9's test matrix rather than adding a filter.

### 2.8 Startup wiring

```go
func NewNativeBackend(logger chassis.Logger) *nativeBackend {
	b := &nativeBackend{logger: logger}
	b.table.Store(&RouteTable{})
	return b
}

// called from main.go's newBackend arm for "native", the same WithRunner
// shape LoadCache already uses:
runtime = runtime.WithRunner(func() {
	if nb, ok := backend.(*control_plane.NativeBackend); ok {
		if err := nb.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.WithError(err).Panic("native proxy backend failed")
		}
	}
})
```

### 2.9 Feature-parity test matrix (before Phase 3 starts)

Run the same route set through both backends and diff behavior: plain HTTP prefix match, EXACT match, `enable_http2` gRPC unary + streaming call, a WebSocket echo, a leading-wildcard host, and the no-host default virtual host. This is the gate for calling Phase 2 done — not a nice-to-have.

---

## Phase 3 — Auth Middleware

**✅ Shipped** — `services/core/fuse/control_plane/native/auth.go` (+ `auth_test.go`). Native-backend equivalent of Envoy's `ext_authz` filter, same `RouteAuth` policy table.

### 3.0 What a live run found, before writing this phase's real code

The plan originally called for a dependency check against Envoy's undocumented HTTP `ext_authz` wire shape. That check ran for real: Blueprint, Fuse (this repo's own `envoy` backend, unmodified), `services/core/auth` (in its default `bypass` mode), and the ready-made `services/examples/auth` (`Greet`/`Secret` RPCs) were brought up together, with the *real* Envoy already running as part of this environment's persistent local dev stack, and a request was sent through it. Three things came out of that which changed this phase's design from what was originally sketched:

1. **Envoy's ext_authz check request forwards the original method and path, but only the `Authorization` header** — not `Cookie`, not `User-Agent`, not any custom header. Confirmed by dumping the exact request `services/core/auth`'s `checkHandler` received: `headers=map[Authorization:[Bearer my-test-token] Content-Length:[0] Traceparent:[...] Tracestate:[] X-Envoy-Expected-Rq-Timeout-Ms:[250] X-Envoy-Internal:[true]]` — no `Cookie` despite the client request setting one. `makeExtAuthzFilter` never configures `allowed_headers`, so Envoy's default (restrictive) allowlist applies. This directly contradicts [Authentication](/docs/architecture/authentication)'s description of the check request carrying "the original request headers" generally — that description is aspirational relative to the actual Envoy config in this repo. **The native middleware below forwards only `Authorization`, matching real behavior, not the broader claim.**
2. **No `context_extensions`-derived signal reaches the check request at all**, and `services/core/auth`'s own `forwarder.go` never reads anything group/scope-related even when running in `authentik` mode — it forwards whatever headers it receives, unconditionally, to Authentik's outpost. This confirms `AUTH_POLICY_GROUPS`/`AUTH_POLICY_SCOPES` are **not currently enforced differently from `AUTH_POLICY_AUTHENTICATED`** anywhere in the real request path, on either backend, today. That's a pre-existing gap in `services/core/auth`, not something this phase should invent a fix for — the native middleware treats all three policies identically, same as reality, and says so in a comment rather than silently matching a design that doesn't exist yet.
3. **A 200 check response's headers (e.g. `X-Authentik-Username`) do not reach the upstream service today**, on the envoy backend, because `makeExtAuthzFilter` never sets `allowed_upstream_headers`. Confirmed live: with the check response explicitly setting `X-Authentik-Username: test-user`, `auth-example`'s `Secret` RPC still reported `"anonymous (auth bypass mode)"` — the header never arrived. This contradicts `authentication.md`'s "the outpost adds user identity headers... that are forwarded to the upstream service" claim, and `auth-example/main.go`'s own comment on `Secret`. Real gap, out of scope for this phase; the native middleware matches this behavior (forwards no check-response headers to the upstream) rather than "fixing" a promise Envoy itself doesn't keep yet.

**A fourth, unrelated bug was also found and fixed along the way**, without which none of the above could have been observed: `services/core/auth/main.go` never called `chassis.Runtime.DisableMux()`. Chassis's `Runtime.Start()` unconditionally binds `service.network.bind_port` with its own (empty, since nothing calls `WithRPCHandler`) `*http.ServeMux` unless told not to (`builder.go:351`, `if !c.noMux { go c.runMux(handler) }`) — and this service *also* runs its own manual `http.ListenAndServe` on that exact same port (`serveCheckEndpoint`). The two raced for the listener; in this environment chassis's own empty mux won every time, and Go's 1.22+ `ServeMux.findHandler`/`matchOrRedirect` panics (nil-pointer dereference) when the mux has zero registered patterns — meaning **every ext_authz check request was silently crashing the connection and Envoy was denying via `FailureModeAllow: false`**, regardless of `auth.mode`. Fixed with one line (`.DisableMux()` added to the builder chain in `main.go`), verified live (the check request started reaching `checkHandler` immediately after). This is a real, standing bug in `services/core/auth` independent of anything in this design — worth a heads-up to whoever owns that service, not just a footnote here.

### 3.1 The real `authMiddleware`

```go
// services/core/fuse/control_plane/native/auth.go
type authMiddleware struct {
	client *http.Client
}

func (m *authMiddleware) check(w http.ResponseWriter, r *http.Request, authAddr string) bool {
	checkReq, err := http.NewRequestWithContext(r.Context(), r.Method, authAddr+r.URL.Path, nil)
	if err != nil {
		http.Error(w, "failed to build auth check request", http.StatusInternalServerError)
		return false
	}
	// Only Authorization -- verified live, see 3.0 above.
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
		for key, vals := range resp.Header {
			for _, v := range vals {
				w.Header().Add(key, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body) //nolint:errcheck
		return false
	}
	// No allow-path header relay to the upstream -- see 3.0's point 3.
	return true
}

// authRequired mirrors makePerRouteAuthConfig's own enable/disable gate.
func authRequired(route *ntv1.Route) bool {
	auth := route.GetAuth()
	return auth != nil && auth.GetEnabled() && auth.GetPolicy() != ntv1.AuthPolicy_AUTH_POLICY_BYPASS
}
```

`authAddr` is resolved via `cp.GetAuthServiceAddress` — a small refactor alongside this phase pulled the KV lookup `envoyBackend`'s `Apply` already had into a shared, exported, package-level function in `backend.go` (rather than a private method), so both backends call the identical lookup. This is the "candidate to hoist into a shared package" the Phase 2.2 matcher note flagged, applied here instead since this lookup was simpler and had no reason to wait.

Wired into `serveHTTP` (2.4) before endpoint selection:

```go
if authRequired(route) {
	authAddr := authAddress(r.Context())
	if authAddr != "" && !b.authMW.check(w, r, authAddr) {
		return // check already wrote the denial response
	}
}
```

`envoyBackend.Capabilities()` and `native.Backend.Capabilities()` do **not** change in this phase — auth enforcement isn't in `BackendCapabilities` (both backends have always supported it; this phase brings the native one to parity, it doesn't add a new capability axis).

### 3.2 Tests

`auth_test.go` locks in the verified contract directly: `TestAuthMiddleware_OnlyForwardsAuthorization` spins up a real `httptest.Server` as the check endpoint and asserts `Cookie`/`User-Agent`/a custom header do **not** arrive while `Authorization` does (with a note on Go's `http.Client` stamping its own default `User-Agent`, which the test accounts for rather than misreading as a forwarding bug). `TestAuthMiddleware_DenyRelaysStatusAndBody` and `TestAuthMiddleware_UnreachableAuthServiceDenies` cover the two non-200 paths.

### 3.4 Flip the default to `native`

Once 2.9's routing-parity suite and this phase's own auth-parity testing (run the same `RouteAuth` scenarios — `BYPASS`/`AUTHENTICATED`/`GROUPS`/`SCOPES`, plus outpost-unreachable — through both backends and diff the result) both pass:

```go
const proxyBackendDefault = "native" // was "envoy" through Phase 1-3; see 1.1
```

This is the one-line change that makes [Fuse — Pluggable Proxy Backends](/docs/architecture/fuse-native-proxy)'s target default real. Everything downstream of this point in the plan (Phases 4–9) is written assuming `native` is what a cluster gets without an explicit `fuse.proxy_backend` setting — including in Fuse's own README and `dctl`'s scaffolding output, which should be updated in this same phase to tell a new cluster what it's getting by default rather than continuing to describe Envoy as the implicit choice.

---

## Phase 4 — WideEvent Integration

**✅ Shipped** — `services/core/fuse/control_plane/native/wide_event.go` (+ `wide_event_test.go`), the proto field, and the span wrap in `backend.go`'s `serveHTTP`. One real fix along the way, beyond what 4.2's sketch below shows: naively wrapping the `ResponseWriter` to capture the upstream status code would have silently broken the WebSocket/Upgrade proxying Phase 2 already established, since `httputil.ReverseProxy`'s upgrade handling depends on type-asserting the `ResponseWriter` to `http.Hijacker`. `statusRecorder` forwards `Hijack`/`Flush` to the underlying writer; `wide_event_test.go`'s `TestStatusRecorder_ForwardsHijack` is the regression test for exactly this. `native.Backend.Capabilities()` now reports `WideEvents: true`.

### 4.1 Proto: per-route opt-out

```protobuf
message Route {
    string name  = 1;
    RouteMatch match = 2;
    Endpoint endpoint = 3;
    bool enable_http2 = 4;
    RouteAuth auth = 5;
    repeated Endpoint endpoints = 6;
    // Opt out of per-request WideEvent production on the native backend.
    // Ignored entirely on the envoy backend (see Capabilities.WideEvents).
    // Unset (false) means "emit" -- the inverse default from every other
    // WideEvent producer in the framework; see the design doc's Decisions.
    bool wide_events_disabled = 7;
}
```

`dctl api build` regenerates bindings; wire-compatible (new field, default `false`), no other service needs to change.

### 4.2 Span wrap in `serveHTTP`

```go
func (b *nativeBackend) serveHTTP(w http.ResponseWriter, r *http.Request) {
	route, ok := matchRoute(b.table.Load(), stripPort(r.Host), r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	var span *chassis.Span
	emit := chassis.GetConfig().GetBool("telemetry.wide_events.enabled") && !route.WideEventsDisabled
	if emit {
		var ctx context.Context
		ctx, span = chassis.StartSpan(r.Context(), route.Name)
		r = r.WithContext(ctx)
	}

	rw := &statusRecorder{ResponseWriter: w} // captures status code; ReverseProxy doesn't expose it otherwise
	upstreamStart := time.Now()

	// ... auth middleware (Phase 3), then ReverseProxy.ServeHTTP(rw, r) ...

	if span != nil {
		span.SetAttribute("http.method", r.Method)
		span.SetAttribute("http.path", r.URL.Path)
		span.SetAttribute("http.status_code", strconv.Itoa(rw.status))
		span.SetAttribute("route.name", route.Name)
		span.SetAttribute("route.match_type", route.Match.GetMatchType().String())
		span.SetRuntimeAttribute("upstream.address", endpointAddr(route))
		span.SetRuntimeAttribute("upstream.duration_ms", strconv.FormatInt(time.Since(upstreamStart).Milliseconds(), 10))
		if r.TLS != nil {
			span.SetRuntimeAttribute("tls.version", tlsVersionString(r.TLS.Version))
			span.SetRuntimeAttribute("tls.cipher_suite", tls.CipherSuiteName(r.TLS.CipherSuite))
		}
		span.End(nil)
	}
}
```

`statusRecorder` is a small `http.ResponseWriter` wrapper (`WriteHeader` records `status` before delegating) — `httputil.ReverseProxy` never exposes the upstream's status code to its caller directly, so this is the standard way to observe it.

### 4.3 `envoyBackend.Capabilities().WideEvents` stays `false`

Nothing in this phase touches `envoyBackend` — an Envoy-backed cluster gets no WideEvents until the ALS path (design doc, [WideEvent parity for the Envoy backend](/docs/architecture/fuse-native-proxy#wideevent-parity-for-the-envoy-backend)) ships, which is explicitly out of scope here.

### 4.4 Documentation

Add a section to `services/core/fuse/README.md` (or wherever `fuse.proxy_backend` ends up documented per Phase 1) covering `telemetry.wide_events.enabled` + `Route.wide_events_disabled` together, since they're two flags a cluster operator needs to reason about jointly to get the behavior described in the design doc.

---

## Phase 5 — TLS

**✅ Shipped** — `services/core/fuse/control_plane/native/{tls.go,tls_selfsigned.go,tls_static.go}` (+ three test files, 12 tests including a real end-to-end TLS handshake). **Native backend only.** No change to `envoyBackend.Capabilities()` in this phase.

A few things resolved or corrected relative to the sketch below, while implementing for real:

- **CA persistence** (5.2.1) went in as designed: `kvv1.Value` has a single `string` field, so the cert and key PEM blocks are concatenated into one string and split back apart with a plain `pem.Decode` loop (PEM blocks are self-delimiting) — no need for a second KV key or a custom separator.
- **The CA's key is generated and stored as `crypto.Signer` via PKCS8** (`x509.MarshalPKCS8PrivateKey`/`ParsePKCS8PrivateKey`), not the sketch's unspecified `crypto.Signer` field left abstract — PKCS8 is algorithm-agnostic, so switching the CA off RSA later wouldn't touch the persistence code.
- **`tls.VersionName`/`tls.CipherSuiteName`-style stdlib helpers don't exist for "mint a leaf cert" the way Phase 4 found for TLS version strings** — this phase's crypto (`x509.CreateCertificate`, serial numbers via `crypto/rand`, not time-based, since two leaves can be minted in the same nanosecond under concurrent `GetCertificate` calls for different domains) is genuinely new code, verified by `TestMintLeaf_SignedByCA` actually calling `x509.Verify` against a pool containing the CA.
- **The operator-supplied provider (5.3) uses mtime-polling on `GetCertificate`, not `fsnotify`** — avoids a new dependency for what's a cheap `stat(2)` on a path that changes rarely; `TestStaticProvider_HotReloadsOnFileChange` forces the mtime forward with `os.Chtimes` to make the test filesystem-resolution-independent.
- **The `GetConfigForClient` wiring (5.4) was pulled into a standalone `tlsConfigForProvider` function**, not inlined into backend construction, specifically so `TestTLSHandshake_SelfSignedProviderEndToEnd` could exercise the *exact* code a real listener uses — a real `net.Listen` + `http.Server.ServeTLS` + a real `http.Client` with the CA in `RootCAs` and `ServerName` driving SNI, not just unit coverage of the certificate-generation helpers. A companion negative test (`TestTLSHandshake_UntrustedClientRejected`) confirms a client that doesn't trust the CA is rejected, the same way an un-provisioned browser would be. This is the one piece of Phase 5 that isn't replicating existing Envoy behavior (unlike Phases 2-4), so it got a live-handshake test rather than relying on crypto-helper unit coverage alone — done entirely with an in-process `net.Listen("tcp", "127.0.0.1:0")`, no interaction with the shared persistent Blueprint this plan's earlier phases had to avoid clobbering.
- **`Backend.Capabilities().TLS` is now `true`**; `MTLS` stays `false` until Phase 6.

### 5.1 `CertificateProvider` interface

```go
// services/core/fuse/control_plane/native/tls.go
type CertificateProvider interface {
	GetCertificate(domain string) (*tls.Certificate, error)
	Changed() <-chan string
}
```

### 5.2 Self-signed local CA

```go
type selfSignedProvider struct {
	caCert *x509.Certificate
	caKey  crypto.Signer
	mu     sync.Mutex
	leaves map[string]*tls.Certificate
	changed chan string
}

func NewSelfSignedProvider(logger chassis.Logger) (*selfSignedProvider, error) {
	ca, key, err := loadOrGenerateCA(logger) // 5.2.1
	if err != nil {
		return nil, err
	}
	return &selfSignedProvider{caCert: ca, caKey: key, leaves: map[string]*tls.Certificate{}, changed: make(chan string, 8)}, nil
}

func (p *selfSignedProvider) GetCertificate(domain string) (*tls.Certificate, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.leaves[domain]; ok && !expiringSoon(c) {
		return c, nil
	}
	leaf, err := mintLeaf(p.caCert, p.caKey, domain) // crypto/x509.CreateCertificate, short NotAfter (eg. 7 days)
	if err != nil {
		return nil, err
	}
	p.leaves[domain] = leaf
	select {
	case p.changed <- domain:
	default:
	}
	return leaf, nil
}

func (p *selfSignedProvider) Changed() <-chan string { return p.changed }
```

`GetCertificate` minting lazily (rather than up front for every route) means the "silently re-minted on the next handshake past expiry" renewal behavior from the design doc falls out of `expiringSoon` naturally — no separate renewal loop needed for this provider.

### 5.2.1 CA persistence — resolves the design doc's open question

Store the CA cert/key in Blueprint KV under a well-known key (e.g. `fuse_local_ca`), matching the design doc's stated lean: it moves with the cluster, and a `dctl infra` reset (which resets Blueprint too) regenerating a new CA is the expected/consistent behavior rather than a surprise. On first boot with no existing key, generate and store; on every subsequent boot, load. Print the one-time trust instruction (path to a PEM export of the cert, plus the OS-specific trust command) whenever a *new* CA is generated, not on every boot.

### 5.3 Operator-supplied provider

```go
type staticProvider struct {
	mu    sync.RWMutex
	certs map[string]*tls.Certificate
	changed chan string
}
// Loaded from Blueprint KV or a mounted file pair at startup; a KV-backed
// instance additionally watches for changes (Blueprint KV doesn't have a
// push/watch primitive today -- poll on an interval, matching the cadence
// getAuthServiceAddress's callers already accept for other KV-sourced
// config) and pushes to `changed` on update.
```

### 5.4 Wiring into the listener

```go
b.server.TLSConfig = &tls.Config{
	GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		cert, err := b.certProvider.GetCertificate(hello.ServerName)
		if err != nil {
			return nil, err
		}
		cfg := &tls.Config{Certificates: []tls.Certificate{*cert}, NextProtos: []string{"h2", "http/1.1"}}
		if route := routeForHost(b.table.Load(), hello.ServerName); route.GetMtls().GetEnabled() {
			cfg.ClientAuth = tls.RequireAndVerifyClientCert
			cfg.ClientCAs = b.clientCAPool(route) // Phase 6
		}
		return cfg, nil
	},
}
```

A background goroutine reads `certProvider.Changed()` purely for logging/metrics in this phase — no listener restart is needed at all, since `GetConfigForClient` is invoked fresh on every handshake and always calls `GetCertificate` again, so a provider-side update is picked up on the very next connection with no explicit propagation step required.

### 5.5 Config

```yaml
fuse:
  tls:
    mode: off   # off | self-signed | operator | acme (acme is Phase 7)
```

---

## Phase 6 — mTLS

**✅ Shipped** — proto (`MTLSPolicy`, `Route.mtls`), `routeForMTLS` in `matcher.go`, `GetCACertPool` on both providers, and the `GetConfigForClient` wiring in `backend.go` (+ 6 new tests: 3 for `routeForMTLS`, 3 sub-tests for a real end-to-end mTLS handshake). `Backend.Capabilities().MTLS` is now `true`.

**A real, unavoidable limitation surfaced while implementing, not before:** mTLS enforcement can only be host-scoped, not per-route. A TLS `ClientHello` (and therefore the `ClientAuth`/`ClientCAs` decision) is processed once per connection, before any HTTP path is known -- SNI gives a host, never a path. So `routeForMTLS` reuses `selectHostScope` (the same exact/wildcard/default virtual-host selection `matchRoute` already does) and enforces mTLS for *every* route sharing a host with an `mtls.enabled` one, even a sibling route on that host that never asked for a client cert. `TestRouteForMTLS_IsHostScopedNotPathScoped` documents this directly. This is inherent to TLS-layer enforcement, not a shortcut this implementation took -- no backend (Envoy included) can do better than host-level mTLS without a second listener per policy.

**A real bug surfaced by the test suite itself, not by inspection:** the first draft of `TestTLSHandshake_MTLSRequiresClientCert`'s "accepted" case used `mintLeaf` (Fuse's own *server*-cert minter, `ExtKeyUsageServerAuth` only) to build the *client* certificate, and Go correctly rejected it (`certificate specifies an incompatible key usage`). This was a test bug, not a product one -- `mintLeaf` has no reason to support client-auth usage, since Fuse only ever validates client certs (`GetCACertPool`), never issues them. Fixed by adding a test-only `mintTestClientCert` helper (`ExtKeyUsageClientAuth`) to `tls_handshake_test.go`, simulating what an external tool (openssl, a real client-cert workflow) would produce when issuing a cert from Fuse's exported `fuse-local-ca.pem`.

**`selfSignedProvider.GetCACertPool`** trusts client certs signed by the same local dev CA already used for server certs, regardless of the `name` argument -- one CA, nothing to select between yet. **`staticProvider`** gained an optional third constructor argument, `clientCAFile` (`fuse.tls.operator.client_ca_file` in config), loaded once at construction (not hot-reloaded like the server cert/key -- a trusted-client-CA bundle changes far less often in practice, so mtime-polling it too was judged not worth the same mechanism copy-pasted for comparatively little benefit; revisit if that assumption is wrong).

Resolves the design doc's open question in favor of a **new `Route` field**, not a `RouteAuth` extension — mTLS is decided during the TLS handshake, before any HTTP request (and therefore any `RouteAuth` policy check) exists, so it belongs on the transport-facing side of the config, not the request-auth side:

```protobuf
message Route {
    // ... fields 1-7 unchanged ...
    MTLSPolicy mtls = 8; // optional; enforced only when the active backend's Capabilities().MTLS is true
}

message MTLSPolicy {
    bool enabled = 1;
    // Name of a trusted-CA secret sourced from the same CertificateProvider
    // as this route's own server certificate (eg. an operator-supplied
    // bundle, or a second self-signed CA reserved for client certs).
    string trusted_ca_secret_name = 2;
}
```

`b.clientCAPool(route)` (referenced in 5.4) loads `trusted_ca_secret_name` via the same `CertificateProvider` the listener's own cert comes from — providers gain one more method:

```go
type CertificateProvider interface {
	GetCertificate(domain string) (*tls.Certificate, error)
	GetCACertPool(name string) (*x509.CertPool, error) // new
	Changed() <-chan string
}
```

Once verified, the client certificate's `Subject.CommonName` is available in `serveHTTP` (Phase 4) via `r.TLS.PeerCertificates[0]`, for `span.SetRuntimeAttribute("tls.client_cert_subject", ...)`.

---

## Phase 7 — ACME Provider

**✅ Shipped** — `services/core/fuse/control_plane/native/tls_acme.go` (+ 4 tests). `Backend.Capabilities()` is now all-`true` (`TLS`, `MTLS`, `WideEvents`, `ACME`).

**The plan's own flagged open question — "rebuild the domain allowlist on every `Apply()`, or accept a static config list" — is resolved with a third option neither alternative considered**, not by picking one of the two: `HostPolicy` isn't a static list (`autocert.HostWhitelist`) at all. It's a function closed over the same `*atomic.Pointer[RouteTable]` the backend already maintains, calling the exact `selectHostScope` helper `matchRoute`/`routeForMTLS` already use to decide "is there currently a route for this host." This is always exactly in sync with reality — no rebuild step to remember, no snapshot to go stale — and it means only a host with a *real, currently-registered* route can ever trigger a real ACME request against your directory's rate limits, which is the actual security property `HostPolicy` exists for in the first place. `TestACMEProvider_HostPolicyTracksLiveRouteTable` proves this directly: a route added after construction is authorized immediately, and one removed loses authorization immediately, with no code in between to make that true other than the closure reading the live pointer.

**`GetCACertPool` declines outright** rather than guessing at a trust source for a hypothetical ACME+mTLS combination nothing in this codebase asks for — ACME provisions server certs from a public CA, which has no bearing on validating client certificates.

**`Changed()` returning a `nil` channel is correct, not an oversight**: `autocert`'s own `DirCache` handles renewal entirely internally, so there's nothing to signal, and ranging over a nil channel (`Backend.logCertificateChanges`) blocks forever rather than panicking or busy-looping — exactly the right behavior for "this provider will never report anything."

**A real, unplanned dependency-version lesson, not a code bug:** the first attempt to add `golang.org/x/crypto/acme/autocert` via `go get golang.org/x/crypto/acme/autocert` (no version pin) resolved to the latest release (`v0.57.0`), which silently bumped the module's `go` directive from `1.23.2` to `1.26.0` and dragged four unrelated transitive dependencies (`x/net`, `x/sync`, `x/sys`, `x/text`) up with it — far more disruptive than "add one dependency." Reverted and re-added pinned to `v0.32.0` (the version already implied by the existing dependency graph), which added only `golang.org/x/crypto` itself with zero other changes. Worth remembering for any future `go get` in this module: pin the version rather than trusting `@latest`.

```go
type acmeProvider struct {
	m *autocert.Manager
}

func NewACMEProvider(directoryURL string, domains []string, cacheDir string) *acmeProvider {
	return &acmeProvider{m: &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(domains...),
		Cache:      autocert.DirCache(cacheDir),
		Client:     &acme.Client{DirectoryURL: directoryURL}, // non-default directoryURL supports any RFC 8555 CA, not only Let's Encrypt
	}}
}

func (p *acmeProvider) GetCertificate(domain string) (*tls.Certificate, error) {
	return p.m.GetCertificate(&tls.ClientHelloInfo{ServerName: domain})
}

func (p *acmeProvider) Changed() <-chan string { return nil } // DirCache handles renewal transparently; nothing to signal
```

`autocert.HostWhitelist` needs the full domain list up front, which cuts against Fuse's otherwise-dynamic route registration — resolve at implementation time whether this rebuilds on every `Apply()` call (simplest) or accepts a static config list (safer against ACME rate limits from a route that's added and removed repeatedly). Not resolved in the design doc; flagging here rather than picking silently.

```yaml
fuse:
  tls:
    mode: acme
    acme:
      directory_url: https://acme-v02.api.letsencrypt.org/directory
      cache_dir: /var/lib/fuse/acme-cache
```

---

## Phase 8 — Capability Negotiation

**✅ Shipped** — `checkCapabilities` in `controller.go` (+ 4 tests in the new `control_plane` package test file), wired into both `AddRoute` and `ValidateRoute` in `rpc.go`.

**Simpler than the plan anticipated, once actually written:** the plan's own sketch assumed `Capabilities()`/`Name()` needed "a small addition" to make them reachable from `rpc.go`'s `h.controlPlane`. They didn't -- `rpc.go` and `controller.go` are both `package control_plane`, so `h.controlPlane.backend` (an unexported field) was already accessible directly, same-package visibility being what it is. `checkCapabilities` was added as a method on `controlPlane` itself, next to `FindConflicts`, with no interface or struct changes needed at all.

**Scoped to mTLS only, deliberately** -- not WideEvents or TLS, even though the design doc's Decisions section talks about all three. Reasoned through while writing the check itself: `wide_events_disabled` is an opt-*out* that already degrades safely when unsupported (a backend that can't produce a WideEvent just doesn't, same as choosing not to instrument something) -- there's nothing to reject. TLS has no per-route field at all to check; it's `fuse.tls.mode`, a listener-level setting no individual `Route` opts into. mTLS is categorically different: silently not enforcing a client-certificate requirement is a real security downgrade, not a graceful no-op, which is exactly the class of problem this phase exists to close. `TestCheckCapabilities_OnlyChecksMTLS` documents this scope choice directly rather than leaving it implicit.

**A real, known gap this phase does not close:** capability validation only runs at `AddRoute`/`ValidateRoute` time. A route with `mtls.enabled` registered while `native` was active, still sitting in Blueprint KV, then loaded via `LoadCache` after an operator switches `fuse.proxy_backend` back to `envoy`, is never re-validated -- `envoyBackend.Apply` doesn't read `Route.mtls` at all, so it's silently ignored on that path, same as before this phase. Closing that would mean re-validating every persisted route on every backend switch, which is real additional scope this phase doesn't attempt; flagged here rather than left to be rediscovered.

### 8.1 Honest `Capabilities()` on both backends

By this phase: `envoyBackend.Capabilities()` returns `{TLS: false, MTLS: false, WideEvents: false, ACME: false}` unless [Fuse — API Gateway](/docs/architecture/fuse-api-gateway)'s own Phases 5–6 have separately shipped by then, in which case update it to match reality — this phase's job is making the method tell the truth, not changing what's true. `nativeBackend.Capabilities()` returns `{TLS: true, MTLS: true, WideEvents: true, ACME: true}` once Phases 5–7 above are done.

### 8.2 Config-time validation

Wire into `AddRoute`/`ValidateRoute` (`rpc.go`), after the existing conflict check:

```go
caps := h.controlPlane.backend.Capabilities()
if r := msg.GetRoute(); r.GetMtls().GetEnabled() && !caps.MTLS {
	return connect.NewResponse(&ntv1.AddRouteResponse{
		Code:    ntv1.AddRouteResponseCode_INVALID_REQUEST,
		Message: fmt.Sprintf("route requires mTLS, but the active proxy backend (%s) doesn't support it", h.controlPlane.backend.Name()),
	}), nil
}
```

This needs `ProxyBackend` (or at least `Capabilities()`+`Name()`) reachable from `rpc.go`'s `h.controlPlane` — a small addition to the `controlPlane` type from Phase 1, not a new dependency.

### 8.3 Blueprint UI

No new view: the existing `AddRouteResponse.message` surfaces this exactly the way it already surfaces a conflict message (per [Fuse — API Gateway](/docs/architecture/fuse-api-gateway)'s Route Detail view) — a capability-mismatch rejection reads the same as any other `INVALID_REQUEST`, so [Fuse — API Gateway](/docs/architecture/fuse-api-gateway)'s existing conflict-banner rendering already displays it with no new UI code.

---

## Phase 9 — Integration & Testing

**🟡 Partially done, live, against a real cluster — not the full matrix, and the default flip is explicitly not taken.** Rather than a throwaway local stack, this ran against the author's actual persistent Draft dev cluster (a 5-node Blueprint, the real `envoy`-backed Fuse, Catalyst, and Beacon, all already running) — with a real, considered risk mitigation, because every Fuse process unconditionally writes its own address to Blueprint's shared `fuse_address` key on startup (`rpc.go`'s `RegisterRPC`), and a second Fuse registering against a live cluster overwrites the key the real one depends on. The approach taken: a temporary native-backend Fuse ran on alternate ports (`18001`/`10001`) against the *same* live Blueprint/Catalyst/Beacon, its test routes and the `fuse_address` key were explicitly cleaned up and restored immediately after each round of testing, and the real Fuse's health and full route table were reconfirmed afterward. Total exposure window: a few minutes, twice.

**A real, previously-undocumented architectural finding surfaced immediately on the first live request:** every route already registered in this cluster targets `host.docker.internal` (the convention every service's `config.yaml` uses so Envoy, running in Docker, can reach a service running natively on the host). A native-backend Fuse is *not* in a container — it's just another native host process, the same as the services it proxies to — and `host.docker.internal` **does not resolve from the host itself** (confirmed directly: `nslookup host.docker.internal` returns `NXDOMAIN` outside a container). Concretely: **every route in this cluster, as currently configured, is unreachable from the native backend**, not because of a bug in the native backend, but because the `host.docker.internal` convention is load-bearing specifically for Envoy-in-Docker. Flipping the default to `native` in a real deployment isn't just a Fuse-side config change — it requires revisiting `service.network.internal.host` (or `route.host`) across every service's own config, likely back to `localhost`/the real host address, once Envoy is no longer the one needing the Docker-specific hostname. This is a real migration cost the design doc doesn't currently mention and should.

**9.1 — Backend-parity suite: partially done.** A real request was proxied through the native backend to an already-running service (`examples-echo`), registered via a fresh test route (avoiding the Docker-hostname problem above by pointing directly at `127.0.0.1`), and its response was diffed byte-for-byte against the same request sent directly to the upstream, bypassing Fuse entirely — including an incidental negative case (a wrong RPC method name) that 404'd identically through both paths, confirming transparent passthrough of status, headers, and body rather than just the happy path. **Not done**: the full enumerated matrix (WebSocket echo, `enable_http2` gRPC streaming, a leading-wildcard host resolved live) and a real side-by-side run of `tests/fuse` / the bench `fuse-proxy-e2e*.yaml` workflows against both backends. Wildcard-host and EXACT/PREFIX precedence are covered thoroughly at the unit level (`matcher_test.go`) but not exercised live end-to-end.

**9.2 — TLS/mTLS round-trip: done, and further than the plan's own outline specified.** Self-signed CA generation, persistence (verified by reading `fuse_local_ca` back out of the *real* Blueprint KV, not a mock), and the trust-instruction file write were all confirmed live. For the mTLS "valid cert is accepted" case, the plan didn't specify how to obtain a valid client cert — this pass extracted the CA's actual private key from Blueprint KV (the same `fuse_local_ca` value the provider persisted) and used real `openssl` commands to mint a genuine CA-signed client certificate, then confirmed: no client cert → handshake fails (`curl -v` shows the server's `Request CERT` followed immediately by a broken pipe when none arrives); a cert from that real CA → accepted, request proxied, correct response. This is a stronger check than the in-process Go tests alone, which mint their own in-memory client certs — this one round-tripped through the actual persisted-and-reloaded CA material.

**9.3 — WideEvent round-trip: done, against real Beacon.** Queried `WideEventsService.SearchWideEvents` on the actual running Beacon (not a stub) after proxying requests through the native backend, and got back real rows with `service_name: "fuse"`, `attributes["route.name"]` matching the route, `attributes["http.status_code"]` correctly reflecting both a 200 and an incidental 404, and `runtime_attributes["upstream.address"]` matching the endpoint exactly. A second route with `wide_events_disabled: true` produced zero rows for the same query, confirming the opt-out.

**9.4 — Capability rejection: done, with no new process at all.** `AddRoute` with `mtls.enabled: true` sent directly to the live, already-running `envoy`-backed Fuse returned exactly `{"code":"INVALID_REQUEST", "message":"route requires mTLS, but the active proxy backend (envoy) doesn't support it"}` — this needed zero new infrastructure since it's just an RPC call against the backend that was already running.

**9.5 — Default-safety check: not attempted, deliberately.** This check's own premise — "post-Phase-3, so the default is `native`" — presupposes the Phase 3.4 default flip already happened. It hasn't: `proxyBackendDefault` is still `ProxyBackendEnvoy` in `backend.go`. Flipping it was explicitly gated (Phase 3.4) on the *full* 2.9/9.1 parity suite passing, which this pass did not complete (see 9.1 above) — and, independent of test coverage, is a product decision about what every cluster gets by default, not something to change as an incidental side effect of writing an integration test. Left for an explicit decision, not silently taken either way.

---

## Milestone Summary

| Phase | Deliverable | Unblocks |
|---|---|---|
| 1 | `ProxyBackend` interface; `envoyBackend` extracted with zero behavior change; `fuse.proxy_backend` config (temporarily still defaulting to `envoy`); port-separation startup check | Every later phase |
| 2 | Native backend: route table, HTTP/1.1 + h2c reverse proxy, WebSockets, gRPC-Web-for-free | Phase 3 |
| 3 | Native-backend auth middleware, parity with Envoy's `ext_authz` policy table; **default flips to `native`** | Native backend safe to default to; every phase after this one assumes `native` is the default |
| 4 | WideEvent production on the native backend, per-route opt-out | Native-backend request-level observability in Beacon |
| 5 | Self-signed local CA + operator-supplied `CertificateProvider`, SNI-based termination, hot-reload | Phase 6, Phase 7 |
| 6 | Per-route mTLS (`MTLSPolicy`), building on Phase 5 | — |
| 7 | ACME `CertificateProvider` | Public-TLS-capable native backend |
| 8 | `Capabilities()` on both backends; config-time rejection of unsupported route settings | Safe to recommend either backend without silent gaps |
| 9 | Backend-parity suite, TLS/mTLS/WideEvent round-trips, default-safety check passing | Ship |
