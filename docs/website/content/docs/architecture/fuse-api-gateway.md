---
weight: 34
title: "Fuse — API Gateway"
description: "Product brief and system design for expanding Fuse into a full dynamic API gateway: conflict-validated routing, subdomain matching, full protocol coverage, request tracing, automatic TLS/mTLS, and a Blueprint UI."
icon: "alt_route"
draft: false
toc: true
---

## Overview

Fuse is already Draft's control plane for [Envoy](https://www.envoyproxy.io/): a process registers a route, Fuse rebuilds an Envoy snapshot, and traffic flows. What it isn't yet is a *gateway* a team can operate — there's no way to see what's routed, no protection against two services silently claiming the same path, no subdomain story, no TLS, and no visibility into which request went where. This document is the system design for closing that gap, scoped against what Fuse's control plane (`services/core/fuse/control_plane`) actually does today, not a green-field redesign.

A companion visual design brief — wireframes for the two Blueprint UI views, a protocol-coverage matrix, and an architecture diagram — lives at [`assets/fuse-api-gateway-design-brief.html`](https://github.com/steady-bytes/draft/tree/main/assets/fuse-api-gateway-design-brief.html); open it directly in a browser to view.

## Current state — what Fuse already does

Read directly from `control_plane/rpc.go` and `control_plane/controller.go`:

- **`NetworkingService.AddRoute` / `ListRoutes`** (ConnectRPC, `api/core/control_plane/networking/v1/service.proto`) — a process registers a `Route` (name, `RouteMatch{prefix, host}`, `Endpoint{host, port}`, `enable_http2`, `RouteAuth`); Fuse validates that `name` and `match.prefix` are non-empty, persists the route in Blueprint's key/value store keyed by route name, and rebuilds the full Envoy snapshot.
- **Per-route auth is already real.** `RouteAuth` supports `BYPASS`, `AUTHENTICATED`, `GROUPS`, and `SCOPES`, compiled into per-route `ext_authz` overrides (`makePerRouteAuthConfig`) — this is further along than the other gaps below.
- **Persistence already happens** — `UpdateCacheWithNewRoute` always writes to Blueprint's KV store before rebuilding the snapshot; `core-services.md` currently describes this as "optional," which is stale relative to the code.
- **xDS is fully wired** — Fuse's `xDSRpc` (`rpc.go:157`) registers all six Aggregated Discovery Service endpoints against `go-control-plane`: Endpoint, Cluster, Route, Listener, **Secret**, and Runtime discovery. Notably, **Secret Discovery Service (SDS) is already registered but never populated** — `apply()`'s snapshot only ever sets `ClusterType`, `RouteType`, and `ListenerType` resources. This is the natural hook point for automatic TLS (see below) — it doesn't need to be built from scratch, just used.
- **WebSockets already work** — the `HttpConnectionManager` sets a global `UpgradeConfigs` entry for `"websocket"` (`controller.go:274-278`).
- **HTTP/2 is opt-in per route** — `Route.enable_http2` toggles `Http2ProtocolOptions` on the upstream cluster, which is what gRPC needs.

## Gaps, mapped to the requirements

| Requirement | State |
|---|---|
| Dynamic routing rules | **Partial.** `AddRoute`/`ListRoutes` work; `DeleteRoute` is a stub that returns `ErrNotImplemented` (`rpc.go:128`) despite the RPC and proto message already existing. |
| Route conflict validation | **Missing.** `AddRoute` checks only that `name` and `prefix` are non-empty — nothing detects two routes claiming the same `(host, prefix)`. |
| HTTP / HTTP2 / gRPC / gRPC-Web / WebSockets | **Partial.** HTTP, HTTP/2, gRPC, and WebSockets work today. **gRPC-Web does not** — Envoy's `envoy.filters.http.grpc_web` filter, which translates the grpc-web wire format browsers use, is not in Fuse's `httpFilters` chain (`controller.go:250-262`). This matters directly for Blueprint's own Dioxus/WASM client and any future browser client of Fuse-routed services. |
| Strict matching, deny by default | **Inverted from the ask.** Every route is compiled as an Envoy `Prefix` match (`controller.go:525`) — there's no exact-match option, so "strict by default" doesn't hold today. Deny-by-default itself is already true at the Envoy level (no catch-all route exists), so this is a matching-semantics gap, not a security gap. |
| Subdomains, including on localhost | **Partial.** `RouteMatch.host` already flows straight into an Envoy `VirtualHost.Domains` entry (`controller.go:545`), and Envoy natively supports a leading-wildcard domain like `*.draft.localhost` — Fuse just never constructs or documents one; today `host` is always used as a single literal domain. Note this needs zero extra host resolution work locally: any `*.localhost` name already resolves to `127.0.0.1` in modern browsers and OS resolvers with no `/etc/hosts` entry, so wildcard *routing* is the only real gap. |
| Request ID / trace ID propagation | **Missing.** No tracing provider is configured on the `HttpConnectionManager`, so there's no `trace_id` generation or forwarding today (Envoy's default `x-request-id` generation is present implicitly, but nothing surfaces or documents it, and there's no `traceparent` propagation). |
| Automatic TLS / mTLS on deploy | **Missing**, but the hard part — SDS registration — already exists unused (see above). The default listener (`controller.go:291-312`) is plain HTTP with no `DownstreamTlsContext` at all. |
| Tight Blueprint coupling: persistence + UI | **Since resolved — see below.** *(Snapshot at design time, kept for history:)* Persistence is real. Blueprint's web client already has a `Gateway` view (`views/gateway.rs`, routed at `/gateway`) — a single-page table with an add/edit/delete modal, wired directly to `NetworkingServiceClient`. But it's a rougher first pass than a finished feature: its **Delete** button (and **Edit**, which is implemented as delete-then-re-add) calls `DeleteRoute`, which is broken server-side today — so both are silently non-functional right now. It also hardcodes its own `FUSE_URL = "http://127.0.0.1:18000"` instead of reusing the `crate::API_DOMAIN` constant every other view in the client uses, which means it's dev-only and would need a code change, not a config change, to work in a deployed cluster. There's no protocol/auth-policy display, no validation feedback, and no list/detail split. All of this shipped in Phases 1 and 7 below — `FUSE_URL` went through two iterations (`API_DOMAIN`, then a dedicated `FUSE_DOMAIN` once per-service subdomains made `API_DOMAIN` ambiguous — see [Service UIs via Subdomains](/docs/architecture/service-ui-subdomains)). |
| Common path / domain / subdomain matching | **Partial.** Prefix and single-domain matching work; exact-path and wildcard-domain matching do not exist yet. |

## System design

### Route validation & conflict detection

Add a validation pass that runs inside `AddRoute` before `UpdateCacheWithNewRoute` is called, comparing the incoming route against the result of `ListRoutes`:

- **Identity conflict** — another route already has the same `(host, prefix, match_type)` tuple. Reject with the existing route's name in the error.
- **Shadowing warning** — an `EXACT` route's path is already fully covered by an existing `PREFIX` route on the same host (or vice versa isn't an error, since Envoy's route table is evaluated in the order Fuse emits it — see the sort-by-longest-prefix comment already in `makeRouterConfig`, `controller.go:558-566` — but it's worth surfacing to the person registering the route).

This needs two additions to the proto surface: `AddRouteResponse` currently carries only a `code`; add `string message` and `repeated string conflicting_routes` so the Blueprint UI can render *why* a route was rejected, not just that it was. A `ValidateRoute(Route) → ValidateRouteResponse` RPC (same conflict logic, no persistence) gives the UI a dry-run path to check before submit — this is what backs the "route validation" requirement as a UI-visible feature rather than only a server-side guard.

### Strict matching, deny by default

Extend `RouteMatch` with a `MatchType` enum (`EXACT`, `PREFIX`), defaulting to `EXACT` when unset — inverting today's implicit always-prefix behavior. `makeRouterConfig` picks `route.RouteMatch_Path` (exact) vs `route.RouteMatch_Prefix` based on the field instead of always using prefix. Deny-by-default requires no new code: Envoy already 404s any request that matches no virtual host or route, since Fuse never emits a catch-all wildcard.

### Protocol coverage

| Protocol | Envoy mechanism | Status |
|---|---|---|
| HTTP/1.1 | Default HCM codec | Done |
| HTTP/2 | `Http2ProtocolOptions` on the cluster, opt-in via `Route.enable_http2` | Done |
| gRPC | HTTP/2 + standard router | Done (rides on HTTP/2 support) |
| gRPC-Web | `envoy.filters.http.grpc_web` HTTP filter | **Add.** Insert into the filter chain before the router, after `ext_authz`. It's a no-op passthrough for non-grpc-web requests, so it's safe to enable unconditionally rather than per-route. |
| WebSockets | `UpgradeConfigs` on the HCM | Done |

### Subdomains — including on localhost

No new resolution mechanism is needed for local development — any `*.localhost` hostname already resolves to `127.0.0.1` in Chrome, Firefox, and Safari without touching `/etc/hosts`. What Fuse needs is the *routing* half: accept a leading-wildcard `host` (e.g. `*.draft.localhost`) and pass it through unmodified into `VirtualHost.Domains` — Envoy's own domain matcher already understands the wildcard syntax, so this is a validation change (allow a single leading `*.`) rather than new routing logic. The convention this enables: `orders.draft.localhost`, `payments.draft.localhost`, etc. all terminate at the same Fuse-managed listener and get virtual-host-routed by subdomain, exactly like a production `*.api.example.com` setup — dev and prod use the same mechanism, just different hostnames underneath a single wildcard route registered per service.

**Shipped, one-subdomain-per-service (not wildcard) form:** every service's own UI is now reachable through Fuse this way — `blueprint.draft.localhost`, `beacon.draft.localhost`, `bench.draft.localhost`, `garage.draft.localhost` — self-discovered in Blueprint's sidebar via the same `ListRoutes` call the Gateway view uses. See [Service UIs via Subdomains](/docs/architecture/service-ui-subdomains) for the convention and the real bugs (a startup-order deadlock, a silently-swallowed route conflict, browsers including the port in their Host header, and an HTTP/2 requirement from the grpc_web filter) found shipping it — several apply to *any* host-based route, not just these four.

### Request ID & trace ID propagation

Configure `HttpConnectionManager.Tracing` with an OpenTelemetry provider so Envoy generates a `trace_id` per request (not just the existing default `x-request-id`) and forwards `traceparent`/`x-request-id` headers to the upstream cluster — Envoy does this automatically once a tracing provider is set, no per-route config needed. This is deliberately decoupled from [Beacon](/docs/architecture/beacon-observability): Fuse can generate and propagate trace IDs on its own schedule, and the payoff — correlating a request's trace across Fuse, the backend service's logs, and any downstream spans — only fully lands once Beacon exists to ingest and query them. Until then, `request_id`/`trace_id` still land in Fuse's own access logs, which is enough for the "logging and auditing" half of the requirement on its own.

### Automatic TLS / mTLS

The SDS registration already sitting unused in `rpc.go` is the whole mechanism — this is populating a resource type, not building new xDS plumbing:

- **Local dev:** Fuse generates (or is handed, via `mkcert`) a locally-trusted CA and per-domain leaf certs, delivered to Envoy as `resource.SecretType` resources over the already-registered SDS endpoint. The listener's `FilterChainMatch` selects a cert per SNI, which is also what makes multi-subdomain TLS work on one listener.
- **Deployed clusters:** the same SDS path, sourced from an ACME-backed cert instead of a self-signed one — pluggable, not a hardcoded Let's Encrypt dependency, since Draft may run behind an operator-managed cert already.
- **mTLS:** an opt-in `Route`-level (or `RouteAuth`-level) flag adds a `CertificateValidationContext` to the listener's `DownstreamTlsContext`, requiring and validating a client cert for that route's domain before Envoy accepts the connection.

Because certs are delivered via SDS rather than baked into the listener config, rotation/renewal is a secret push, not a listener restart — the same "no restart on change" property Fuse already gets for routes via the RDS path.

### Blueprint coupling — persistence & UI

Persistence is already correct today (KV-backed, one route per key). The only proto surface gap is `DeleteRoute`, which is a two-line fix once looked at directly: `KeyValueService.Delete` already exists (`api/core/registry/key_value/v1/service.proto`) and is unused by Fuse — `DeleteRoute` just needs to call it and then re-run `apply()` to rebuild the snapshot without that route, mirroring what `UpdateCacheWithNewRoute` already does on add. This alone fixes two already-shipped-but-silently-broken buttons in Blueprint's `Gateway` view (see below).

UI is a refinement, not a from-scratch build: evolve the existing `Gateway` view (`services/core/blueprint/web-client/src/views/gateway.rs`) into the two views described below, described below.

## Blueprint UI — two views

Per the brief, both views live in Blueprint's own UI, not a separate Fuse client — consistent with how Fuse has never had its own UI and how Beacon is planned to serve its own client rather than bolt one onto Blueprint. Concretely, this splits and extends the existing single-page `Gateway` view rather than adding it new.

### Routing List

A table of every registered route: name, match (`host` + `prefix`/`EXACT` path), target `Endpoint`, protocol badges (HTTP/HTTP2/gRPC/gRPC-Web/WS — derived from `enable_http2` and whether the route matches a `grpc_match_options`), auth policy badge (`BYPASS`/`AUTHENTICATED`/`GROUPS`/`SCOPES`), and a validation-state indicator (clean vs. conflicting). Search/filter by host or target service. `Gateway` already renders a route table today (name, prefix, host, endpoint host/port, HTTP/2) — this extends it with protocol/auth/validation columns and turns rows into links instead of inline edit buttons.

### Route Detail

A new page (`/gateway/:name`) opened from a Routing List row, replacing today's edit-in-a-modal pattern. Shows the full `Route` — match configuration, target endpoint, protocol toggles, `RouteAuth` policy editor (group/scope requirements), and any conflicts flagged by `ValidateRoute`. A delete action wired to the now-implemented `DeleteRoute` (today's delete-then-re-add "edit" becomes a real update once `DeleteRoute` works, but a dedicated detail page removes the need for that workaround entirely). Once trace propagation lands, this is also the natural place to show "recent requests through this route" pulled from Beacon by `route_name`/`trace_id` — noted here as the intended integration point, not built in this phase.

A full step-by-step implementation plan — proto diffs, Go snippets, and the Blueprint view refactor — lives at [Fuse — API Gateway Implementation Plan](/docs/architecture/fuse-api-gateway-implementation-plan).

## Phased plan

1. ✅ **Route lifecycle correctness** — implement `DeleteRoute`; add `MatchType` (`EXACT` default / `PREFIX` opt-in) to `RouteMatch`; add conflict validation to `AddRoute` plus the new `ValidateRoute` RPC; extend `AddRouteResponse` with `message`/`conflicting_routes`. Shipped, plus a fix `chassis.WithRoute` needed once conflict detection existed: it now checks `AddRouteResponse.Code`, not just transport-level `err` — see [Service UIs via Subdomains](/docs/architecture/service-ui-subdomains#a-rejected-addroute-looked-like-success).
2. ✅ **Protocol completeness** — add the `grpc_web` HTTP filter to the chain. Shipped — and using it for real (a browser client, not just `curl`) surfaced that routes talking to a chassis backend need `EnableHttp2: true` too; see the same doc's [EnableHttp2](/docs/architecture/service-ui-subdomains#why-every-route-needs-enablehttp2) section.
3. ✅ **Subdomains** — shipped in the concrete "one subdomain per service" form (not the general wildcard-per-tenant form) for every service's own UI; the general leading-wildcard `*.draft.localhost` case (one route, many subdomains) is exercised in Bench's `fuse-proxy-e2e` workflow but not yet used by a real service. Also needed `StripAnyHostPort` on the `HttpConnectionManager`, since browsers include a non-default port in their Host header and routes are registered with a bare host — see [Service UIs via Subdomains](/docs/architecture/service-ui-subdomains#browsers-include-the-port-fuses-routes-didnt).
4. **Request tracing** — configure OpenTelemetry tracing on the `HttpConnectionManager`; confirm `trace_id`/`x-request-id` land in Fuse's access logs.
5. **Automatic TLS** — populate `resource.SecretType` snapshots over the existing SDS registration for local (mkcert-style) certs; SNI-based `FilterChainMatch` per domain.
6. **mTLS** — opt-in per-route client-cert validation, building on the TLS work above.
7. ✅ **Blueprint UI** — Routing List and Route Detail views, backed by `ListRoutes`/`ValidateRoute`/`DeleteRoute`. Shipped, plus self-service discovery: every service's own UI now shows up in Blueprint's sidebar automatically — see [Service UIs via Subdomains](/docs/architecture/service-ui-subdomains).

Each phase is independently shippable and does not block the next — in particular, the UI (phase 7) only needs `ListRoutes`, which already exists, so it could be pulled forward if the UI is the priority.

## Non-goals, v1

- **Non-Envoy proxy support.** `core-services.md` floats "considering supporting various proxies in the future" — this design stays entirely inside Envoy's existing xDS surface; swapping proxies is a separate, much larger effort.
- **A general-purpose WAF or rate limiting.** Envoy supports both, but neither is in the requirements above; adding them later is additive to this design, not blocked by it.
- **Multi-cluster / global load balancing.** Fuse remains a single-cluster gateway; cross-cluster routing is out of scope.
- **Route change audit history.** Request-level `request_id`/`trace_id` tracing (in scope) is different from a versioned history of *route configuration* changes over time — `controller.go`'s own `increment()` TODO already flags this as future work, and it stays future work here too.

## Landscape

Fuse's differentiator against the general-purpose options below is the same one Beacon has against SigNoz: it isn't a separate product bolted beside the cluster, it's already the cluster's control plane.

| | Caddy | Traefik | Raw Envoy | Fuse |
|---|---|---|---|---|
| Config source | Caddyfile / JSON API | Docker/K8s labels, file, Consul | Static config + xDS | Blueprint KV, via a typed RPC |
| Dynamic reload | Yes | Yes | Only if you build an xDS control plane | Already an xDS control plane |
| Auto TLS | Yes (built-in ACME) | Yes (built-in ACME) | No (bring your own SDS source) | Planned, via the already-registered SDS path |
| Service registration | External (labels/config) | External (labels/config) | External (your control plane) | Native — same `Register` path every Draft process already uses |
| Operator UI | None built-in | Dashboard (read-only-ish) | None built-in | Planned — full read/write in Blueprint |

Envoy references used while scoping this design: [Caddy reverse proxy docs](https://caddyserver.com/docs/quick-starts/reverse-proxy), [Traefik](https://traefik.io/traefik), [Envoy](https://www.envoyproxy.io/).
