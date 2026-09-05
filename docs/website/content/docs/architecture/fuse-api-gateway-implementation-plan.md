---
weight: 35
title: Fuse — API Gateway Implementation Plan
description: Phased implementation plan for expanding Fuse into a full dynamic API gateway — proto diffs, Go control-plane changes, and the Blueprint UI refactor
icon: checklist
draft: false
toc: true
---

This document is the step-by-step implementation plan for [Fuse — API Gateway](/docs/architecture/fuse-api-gateway), in the order work should be done. Each phase produces a runnable, testable artifact and does not block the next — phases can be reordered or pulled forward independently. File paths and existing code below are quoted directly from `services/core/fuse/control_plane/{rpc.go,controller.go}`, `api/core/control_plane/networking/v1/service.proto`, and `services/core/blueprint/web-client/src/`.

---

## Phase 1 — Route Lifecycle Correctness

Fixes the two features Blueprint's `Gateway` view *already calls* but which are silently broken today, and adds conflict validation before any new protocol or TLS work builds on top of the route table.

### 1.1 Extend `networking/v1/service.proto`

```protobuf
// RouteMatch gets a match_type. Unset (0) is treated as PREFIX — matching
// today's implicit runtime behavior — so every route already registered by
// an existing service keeps working unchanged. New callers should set EXACT
// explicitly; MATCH_TYPE_UNSPECIFIED defaulting to PREFIX is a deliberate
// backward-compatibility shim, not the long-term default.
enum MatchType {
  MATCH_TYPE_UNSPECIFIED = 0;
  MATCH_TYPE_EXACT       = 1;
  MATCH_TYPE_PREFIX      = 2;
}

message RouteMatch {
  string prefix = 1;
  optional HeaderMatchOptions headers = 2;
  optional GrpcMatchOptions grpc_match_options = 3;
  optional DynamicMetadata dynamic_metadata = 4;
  string host = 5;
  MatchType match_type = 6; // new
}

// AddRouteResponse gets detail fields so the UI can show *why* a route
// was rejected, not just that it was.
message AddRouteResponse {
  AddRouteResponseCode code = 1;
  string message = 2;                    // new
  repeated string conflicting_routes = 3; // new
}
```

Add a dry-run validation RPC so the Blueprint UI can check for conflicts before submitting:

```protobuf
service NetworkingService {
  rpc AddRoute(AddRouteRequest) returns (AddRouteResponse) {}
  rpc ListRoutes(ListRoutesRequest) returns (ListRoutesResponse) {}
  rpc DeleteRoute(DeleteRouteRequest) returns (DeleteRouteResponse) {}
  rpc ValidateRoute(ValidateRouteRequest) returns (ValidateRouteResponse) {} // new
}

message ValidateRouteRequest {
  Route route = 1;
}

message ValidateRouteResponse {
  bool valid = 1;
  repeated string conflicting_routes = 2;
  string message = 3;
}
```

Run `dctl api build` (or `buf generate`) to regenerate the Go, Rust, and TypeScript bindings. Verify `services/core/fuse` and `services/core/blueprint/web-client` both still compile before proceeding — this is a wire-compatible change (new fields, new enum, new RPC) so no other service breaks.

### 1.2 Implement `DeleteRoute`

Today's stub in `control_plane/rpc.go`:

```go
func (h *rpc) DeleteRoute(context.Context, *connect.Request[ntv1.DeleteRouteRequest]) (*connect.Response[ntv1.DeleteRouteResponse], error) {
	return nil, ErrNotImplemented
}
```

`KeyValueService.Delete` already exists (`api/core/registry/key_value/v1/service.proto`) and is unused by Fuse — this is a matter of calling it and re-running `apply()`, mirroring what `UpdateCacheWithNewRoute` already does on add. Add to `controller.go`:

```go
func (cp *controlPlane) DeleteRoute(ctx context.Context, name string) error {
	client := kvv1Connect.NewKeyValueServiceClient(http.DefaultClient, chassis.GetConfig().Entrypoint())

	routeModel, err := anypb.New(&ntv1.Route{})
	if err != nil {
		return ErrFailedRouteMarshal
	}

	_, err = client.Delete(ctx, connect.NewRequest(&kvv1.DeleteRequest{
		Key:   name,
		Value: routeModel,
	}))
	if err != nil {
		return err
	}

	return cp.apply(ctx, client)
}
```

Add `DeleteRoute(ctx context.Context, name string) error` to the `ControlPlane` interface, and update `rpc.go`:

```go
func (h *rpc) DeleteRoute(ctx context.Context, req *connect.Request[ntv1.DeleteRouteRequest]) (*connect.Response[ntv1.DeleteRouteResponse], error) {
	name := req.Msg.GetName()
	if name == "" {
		return nil, ErrInvalidRouteName
	}
	if err := h.controlPlane.DeleteRoute(ctx, name); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&ntv1.DeleteRouteResponse{
		Code: ntv1.DeleteRouteCode_DELETE_ROUTE_OK,
	}), nil
}
```

This alone fixes Blueprint's existing `Gateway` view: its Delete button, and its Edit button (implemented as delete-then-re-add), both call `DeleteRoute` today and both currently fail silently against the `ErrNotImplemented` stub.

### 1.3 Conflict validation

Add a shared conflict-detection function to `controller.go`, used by both `AddRoute` and the new `ValidateRoute`:

```go
// FindConflicts returns the names of any existing routes that share the same
// (host, match_type, prefix) tuple as candidate. Excludes candidate.Name itself
// so re-registering an unchanged route (or updating one) doesn't flag against itself.
func (cp *controlPlane) FindConflicts(ctx context.Context, candidate *ntv1.Route) ([]string, error) {
	existing, err := cp.ListRoutes(ctx)
	if err != nil {
		return nil, err
	}

	key := routeKey(candidate.GetMatch())
	var conflicts []string
	for _, r := range existing {
		if r.GetName() == candidate.GetName() {
			continue
		}
		if r.GetMatch().GetHost() == candidate.GetMatch().GetHost() && routeKey(r.GetMatch()) == key {
			conflicts = append(conflicts, r.GetName())
		}
	}
	return conflicts, nil
}

// routeKey normalizes match_type (UNSPECIFIED behaves as PREFIX, see the proto
// comment) so two routes that would compile to the same Envoy route conflict
// even if one left match_type unset.
func routeKey(m *ntv1.RouteMatch) string {
	mt := m.GetMatchType()
	if mt == ntv1.MatchType_MATCH_TYPE_UNSPECIFIED {
		mt = ntv1.MatchType_MATCH_TYPE_PREFIX
	}
	return fmt.Sprintf("%d:%s", mt, m.GetPrefix())
}
```

Wire it into `AddRoute` in `rpc.go`, before the existing call to `UpdateCacheWithNewRoute`:

```go
conflicts, err := h.controlPlane.FindConflicts(ctx, msg.GetRoute())
if err != nil {
	return nil, connect.NewError(connect.CodeInternal, err)
}
if len(conflicts) > 0 {
	return connect.NewResponse(&ntv1.AddRouteResponse{
		Code:              ntv1.AddRouteResponseCode_INVALID_REQUEST,
		Message:           fmt.Sprintf("conflicts with existing route(s): %s", strings.Join(conflicts, ", ")),
		ConflictingRoutes: conflicts,
	}), nil
}
```

`ValidateRoute` calls the same `FindConflicts` without persisting:

```go
func (h *rpc) ValidateRoute(ctx context.Context, req *connect.Request[ntv1.ValidateRouteRequest]) (*connect.Response[ntv1.ValidateRouteResponse], error) {
	conflicts, err := h.controlPlane.FindConflicts(ctx, req.Msg.GetRoute())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	resp := &ntv1.ValidateRouteResponse{Valid: len(conflicts) == 0, ConflictingRoutes: conflicts}
	if len(conflicts) > 0 {
		resp.Message = fmt.Sprintf("conflicts with existing route(s): %s", strings.Join(conflicts, ", "))
	}
	return connect.NewResponse(resp), nil
}
```

### 1.4 Respect `match_type` when compiling Envoy routes

`makeRouterConfig` in `controller.go` currently always builds a `Prefix` match:

```go
Match: &route.RouteMatch{
	PathSpecifier: &route.RouteMatch_Prefix{
		Prefix: r.Match.Prefix,
	},
},
```

Change to branch on `MatchType`, defaulting unset to prefix (same backward-compatibility rule as `routeKey`):

```go
var pathSpecifier route.RouteMatch_PathSpecifier
if r.Match.GetMatchType() == ntv1.MatchType_MATCH_TYPE_EXACT {
	pathSpecifier = &route.RouteMatch_Path{Path: r.Match.Prefix}
} else {
	pathSpecifier = &route.RouteMatch_Prefix{Prefix: r.Match.Prefix}
}
envoyRoute := &route.Route{
	Match: &route.RouteMatch{PathSpecifier: pathSpecifier},
	// ... unchanged
}
```

Deny-by-default needs no code change — Envoy already 404s any request matching no virtual host or route, since Fuse never emits a catch-all wildcard route.

---

## Phase 2 — gRPC-Web

Add Envoy's `envoy.filters.http.grpc_web` filter to the HTTP filter chain in `controller.go`'s `apply()`. It's a no-op passthrough for non-grpc-web requests, so it's safe to add unconditionally rather than gated per-route, right after `ext_authz` and before the router:

```go
import (
	grpcwebv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/grpc_web/v3"
)

grpcWebAny, err := anypb.New(&grpcwebv3.GrpcWeb{})
if err != nil {
	cp.logger.Error(err.Error())
	return err
}
httpFilters = append(httpFilters, &hcm.HttpFilter{
	Name:       "envoy.filters.http.grpc_web",
	ConfigType: &hcm.HttpFilter_TypedConfig{TypedConfig: grpcWebAny},
})
// existing router filter append stays last
```

`envoy.extensions.filters.http.grpc_web.v3.GrpcWeb` is already vendored in the pinned `go-control-plane v0.12.0` (confirmed present at `envoy/extensions/filters/http/grpc_web/v3`) — no dependency changes needed for this phase.

---

## Phase 3 — Subdomains

No new resolution mechanism — `*.localhost` already resolves to `127.0.0.1` in modern browsers without touching `/etc/hosts`. The only code change is validation: today `AddRoute` never inspects `Match.Host` beyond requiring `Match.Prefix` to be non-empty. Add a check that allows a plain hostname or a single leading wildcard label:

```go
var hostPattern = regexp.MustCompile(`^(\*\.)?[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)*$`)

if host := msg.GetRoute().GetMatch().GetHost(); host != "" && !hostPattern.MatchString(host) {
	return nil, ErrInvalidRouteHost
}
```

No change is needed in `makeRouterConfig` — `r.Match.Host` already flows straight into `VirtualHost.Domains` (`controller.go:545`), and Envoy's own domain matcher already understands a leading `*.` wildcard. Document the convention (`*.draft.localhost` for local dev, a real wildcard domain in production) in the Fuse README and in `dctl`'s service-scaffolding output.

---

## Phase 4 — Request ID & Trace ID Propagation

**Dependency check before writing code:** the OpenTelemetry tracer's Go bindings (`envoy.extensions.tracers.opentelemetry.v3.OpenTelemetryConfig`) are **not present** in `go-control-plane` — confirmed absent in the currently pinned `v0.12.0` and still absent in `v0.13.4` and `v0.14.0`. `go-control-plane` only generates bindings for a curated subset of Envoy extensions, and this tracer isn't in it yet. Two options, in order of preference:

1. **Vendor the single proto file.** Pull `envoy/extensions/tracers/opentelemetry/v3/opentelemetry.proto` (and its `resource_detectors`/`samplers` dependencies) from the upstream `envoyproxy/envoy` API repo into a small local package (e.g. `services/core/fuse/internal/envoytrace/`), and generate Go bindings for just that file with the same `buf`/`protoc` toolchain Draft already uses for its own `api/` directory. This keeps full type safety and is a few hours of one-time work.
2. **Fall back to Zipkin.** `go-control-plane` does ship `envoy.config.trace.v3.ZipkinConfig` bindings. If OTLP-native tracing isn't needed immediately, configure the Zipkin tracer pointed at an OTLP-compatible collector's Zipkin-ingest port (both `docker-otel-lgtm` and the eventual Beacon can expose one) as an interim step, and swap to native OTLP once option 1 is done.

Once a typed config is available, configure it on the `HttpConnectionManager` in `controller.go` — the field already exists (`HttpConnectionManager.Tracing`, confirmed in the pinned version):

```go
provider := &trace.Tracing_Http{
	Name:       "envoy.tracers.opentelemetry",
	ConfigType: &trace.Tracing_Http_TypedConfig{TypedConfig: otelConfigAny},
}
manager.Tracing = &hcm.HttpConnectionManager_Tracing{Provider: provider}
```

Gate this the same way auth is already gated (`getAuthServiceAddress`, `controller.go:333`): read a `fuse.tracing.otlp_endpoint` config key, and skip setting `Tracing` entirely if it's empty. This keeps trace propagation optional and avoids a hard dependency on Beacon (or any collector) existing. Once enabled, Envoy generates and forwards `x-request-id`/`traceparent` automatically — no per-route configuration needed. Correlating traces against service logs is out of scope until [Beacon](/docs/architecture/beacon-observability) exists to ingest them; until then, IDs still land in Fuse's own access logs, which covers the logging/auditing half of the requirement on its own.

---

## Phase 5 — Automatic TLS (SDS)

Fuse's xDS server already registers the Secret Discovery Service (`rpc.go:178-179`, `secretservice.RegisterSecretDiscoveryServiceServer`) — this phase populates it, it doesn't build new xDS plumbing.

### 5.1 Certificate source

Add a `fuse.tls.mode` config key: `off` (default, current behavior) or `dev-self-signed`. For `dev-self-signed`, generate (or load a cached) CA and per-domain leaf certificates using Go's standard `crypto/x509`/`crypto/tls` on startup — equivalent to what `mkcert` does, but self-contained so `dctl infra init` doesn't need an external binary.

### 5.2 Populate the SDS snapshot

Extend `apply()`'s `cache.NewSnapshot` call to include `resource.SecretType`:

```go
import tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"

var secrets []types.Resource
if tlsEnabled {
	for _, domain := range domainsInUse(routes.Msg.GetValues()) {
		cert, key := cp.certFor(domain) // from 5.1
		secrets = append(secrets, &tlsv3.Secret{
			Name: domain,
			Type: &tlsv3.Secret_TlsCertificate{
				TlsCertificate: &tlsv3.TlsCertificate{
					CertificateChain: &core.DataSource{Specifier: &core.DataSource_InlineBytes{InlineBytes: cert}},
					PrivateKey:       &core.DataSource{Specifier: &core.DataSource_InlineBytes{InlineBytes: key}},
				},
			},
		})
	}
}

snapshot, _ = cache.NewSnapshot(cp.increment(),
	map[resource.Type][]types.Resource{
		resource.ClusterType:  clusters,
		resource.RouteType:    systemRoutes,
		resource.ListenerType: {listener},
		resource.SecretType:   secrets, // new
	},
)
```

### 5.3 Terminate TLS per domain via SNI

The listener (`controller.go:291-312`) currently has one `FilterChain` with no `FilterChainMatch` and no `TransportSocket`. When TLS is enabled, build one `FilterChain` per domain with `FilterChainMatch.ServerNames = []string{domain}` and a `TransportSocket` referencing that domain's secret by name via `SdsSecretConfig` — this is also what makes multiple TLS-terminated subdomains work on a single listener (Phase 3's wildcard routing and this phase's per-domain SNI cert selection compose directly). Because certs are delivered over SDS rather than baked into the listener, rotation is a secret push, not a listener restart — the same "no restart on change" property routes already get via RDS.

Because rotation/renewal always over SDS (independent of ACME vs self-signed), a deployed-cluster cert source (ACME-backed) is additive to this phase, not a redesign — swap `cp.certFor` for an ACME-backed implementation later.

---

## Phase 6 — mTLS

Builds directly on Phase 5. Add an opt-in flag — either on `Route` or folded into `RouteAuth` — and, when set, add a `CertificateValidationContext` (referencing a trusted-CA secret, delivered over the same SDS path) to that domain's `DownstreamTlsContext`, with `RequireClientCertificate: true`. Scope this to the domain's `FilterChain`, not globally, since mTLS is a per-route opt-in per the original requirements, not a cluster-wide mode.

---

## Phase 7 — Blueprint UI

Refactors the existing `services/core/blueprint/web-client/src/views/gateway.rs` — a working single-page list + add/edit/delete modal, already wired to `NetworkingServiceClient` — into the two views from the design brief. This is evolution, not a new build: the list-rendering and RPC-calling code already exists and gets extended, not replaced.

### 7.1 Fix the address, first

Before anything else: `gateway.rs` hardcodes `const FUSE_URL: &str = "http://127.0.0.1:18000";`, while every other view (`key_value.rs`, `service_registry.rs`) resolves its target through `crate::API_DOMAIN`, a `Lazy<String>` in `main.rs` that reads the `API_DOMAIN` env var at build time (`main.rs:130`). Replace `FUSE_URL` with `crate::API_DOMAIN.clone()` — otherwise every following change is being tested against a URL that only works on one developer's machine and never works in a deployed cluster.

### 7.2 Add the detail route

`main.rs`'s `Route` enum gains a dynamic segment inside the existing `dashboard_layout`:

```rust
#[route("/gateway")]
Gateway{},
#[route("/gateway/:name")]
RouteDetail { name: String },
```

Add the corresponding entry to `views/mod.rs` (`mod route_detail; pub use route_detail::RouteDetail;`) and to the `use views::{...}` import list. `path_to_route` (used for nav highlighting, `main.rs:75`) does not need a new arm — it's a non-exhaustive lookup used only for top-level nav items, and the detail page isn't one.

### 7.3 Split `gateway.rs` into list + detail

- `views/gateway.rs` keeps the `use_resource` call to `list_routes` and the table rendering, but drops the modal entirely — each row's name becomes a `Link { to: Route::RouteDetail { name: route.name.clone() }, ... }`, and the "+ Add Route" button links to `RouteDetail { name: String::new() }` (empty name signals "new" to the detail view, the same convention `editing_name: Option<String>` already used).
- New `views/route_detail.rs` owns the form fields currently in `gateway.rs`'s modal (`form_name`, `form_prefix`, `form_host`, `form_ep_host`, `form_ep_port`, `form_http2`), plus new fields for `match_type`, the `RouteAuth` policy, and protocol toggles. On submit, it calls `ValidateRoute` first; if `valid` is false, it renders the conflict banner (`conflicting_routes`) instead of submitting. Delete calls `DeleteRoute` directly — no more delete-then-re-add, since a dedicated detail page can just call `AddRoute` again with the same name when editing (or, once available, a future `UpdateRoute`).

### 7.4 New shared components

Add to `components/mod.rs`, following the existing pattern of small focused components (`metric_card.rs`, `type_badge.rs`):

- `protocol_badge.rs` — renders HTTP/H2/gRPC/gRPC-Web/WS pills from a `Route`, derived from `enable_http2` and (once Phase 2 ships) whether gRPC-Web is globally available.
- `auth_badge.rs` — renders a pill for `RouteAuth.policy` (`BYPASS`/`AUTHENTICATED`/`GROUPS`/`SCOPES`), reusing `type_badge.rs`'s existing color-by-variant pattern if it fits.
- `validation_dot.rs` — small colored dot + label, used by both the Routing List row and the Route Detail conflict banner.

---

## Phase 8 — Integration & Testing

### 8.1 Conflict validation test

1. `AddRoute` a route `(host: "a.localhost", prefix: "/", match_type: EXACT)`.
2. `AddRoute` a second route with the identical tuple, different name.
3. Assert the response `code` is `INVALID_REQUEST` and `conflicting_routes` names the first route.
4. `ValidateRoute` the same conflicting route and assert `valid: false` without it having been persisted (`ListRoutes` still returns only the first two).

### 8.2 Delete round-trip test

1. `AddRoute`, then `ListRoutes` and assert it's present.
2. `DeleteRoute` by name, assert `code: DELETE_ROUTE_OK`.
3. `ListRoutes` again and assert it's gone, and that the Envoy snapshot (`cache.SnapshotCache`) no longer contains a cluster for that route's name.

### 8.3 Wildcard subdomain test

1. `AddRoute` with `host: "*.draft.localhost"`.
2. Inspect the generated `RouteConfiguration` and assert a `VirtualHost` exists with `Domains: ["*.draft.localhost"]`.
3. (Manual/E2E) Confirm requests to `orders.draft.localhost` and `payments.draft.localhost` both resolve through the same route when no more specific route exists.

### 8.4 Blueprint UI smoke test

Using the `run` pattern already established for this repo: start Blueprint → Fuse (with a couple of example routes registered) → the web client, navigate to `/gateway`, confirm the list renders with protocol/auth badges, open a row, trigger a deliberate conflict (same host+prefix as another route) and confirm the banner renders, then delete a route and confirm it disappears from the list without a page reload.

---

## Milestone Summary

| Phase | Deliverable | Unblocks |
|---|---|---|
| 1 | `DeleteRoute` implemented, `MatchType`, conflict validation, `ValidateRoute` | Phases 7, 8; fixes Blueprint's already-shipped Delete/Edit buttons |
| 2 | gRPC-Web filter in the chain | Browser clients of gRPC/Connect services routed through Fuse |
| 3 | Wildcard `host` validation | Phase 7's subdomain examples; documented `*.draft.localhost` convention |
| 4 | Request/trace ID propagation (pending OTel binding decision) | Future correlation with Beacon |
| 5 | SDS-backed TLS | Phase 6 |
| 6 | Per-route mTLS | — |
| 7 | Blueprint `Gateway` view split into Routing List + Route Detail | Operator-visible route management |
| 8 | Tests passing, manual UI smoke test | Ship |
