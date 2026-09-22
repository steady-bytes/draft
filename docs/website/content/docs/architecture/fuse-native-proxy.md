---
weight: 45
title: "Fuse — Pluggable Proxy Backends: Native Reverse Proxy & TLS"
description: "System design for a ProxyBackend abstraction in Fuse: Envoy stays fully supported, a native Go reverse proxy becomes a second, configurable option with built-in TLS/mTLS and first-class WideEvent logging, and the interface is shaped to admit further backends later."
icon: "security"
draft: false
toc: true
---

## Overview

Fuse is currently a control plane with no data plane of its own: it validates a route, writes it to Blueprint's key/value store, and rebuilds a snapshot it pushes to a separate Envoy process over xDS. Envoy does the actual work — terminating connections, matching routes, running `ext_authz`, and (once [Fuse — API Gateway](/docs/architecture/fuse-api-gateway) ships TLS) presenting certificates.

This document proposes a **`ProxyBackend` abstraction**: Fuse's route validation, conflict detection, and persistence stay exactly as they are today, but what happens after a route is accepted — how it becomes live traffic handling — is delegated to a swappable backend, selected by config. **`native` — Fuse terminating connections itself with a native Go `net/http` server — is the default backend**; Envoy remains fully supported, unchanged, as an explicit opt-in for clusters that need it. Three changes are bundled into this design because they only make sense once the native backend exists:

1. **A native proxy backend, as the default — with Envoy fully preserved as an opt-in.** No external binary required for a new cluster's default path, no xDS control-plane/data-plane split — but the Envoy backend keeps working exactly as it does today for anyone who sets `fuse.proxy_backend: envoy`. See [Migration note](#migration-note-for-existing-clusters) for what this means for a cluster upgrading from before this change.
2. **TLS, mTLS, and certificate renewal, built into the native backend.** Native termination, SNI-based certificate selection, a pluggable certificate source (local self-signed CA for dev, operator-supplied, or ACME), and rotation with no listener restart. This is specific to the native backend for now; the Envoy backend's own TLS story (Phases 5–6 of [Fuse — API Gateway](/docs/architecture/fuse-api-gateway), via Envoy's Secret Discovery Service) is untouched by this document and remains available to whoever runs that backend.
3. **WideEvent logging as a first-class Fuse feature, on the native backend today, with a forward path for Envoy.** Every request the native backend proxies becomes a `chassis`-instrumented span, which — because [WideEvent — Correlated Observability](/docs/architecture/wide-events) and its [implementation](/docs/architecture/wide-events-implementation-plan) already ship end-to-end in Beacon and chassis — means the native backend gets per-request wide-event logging for free. See [WideEvent parity for the Envoy backend](#wideevent-parity-for-the-envoy-backend) for how the same capability could reach Envoy-backed clusters too, without running Go code inside Envoy.

The interface itself is deliberately shaped so a third or fourth backend (Traefik, Caddy, NGINX, ...) can be added later without touching Fuse's core — see [Extensibility: more than two backends](#extensibility-more-than-two-backends).

A companion visual design brief — architecture diagram, backend comparison, and the certificate-provider model — lives at [`assets/fuse-native-proxy-design-brief.html`](https://github.com/steady-bytes/draft/tree/main/assets/fuse-native-proxy-design-brief.html); open it directly in a browser to view.

**This is a design for review, not an implementation plan.** Nothing here has been built yet. A full step-by-step implementation plan — proto diffs, Go snippets for both backends, and the test matrix — lives at [Fuse — Pluggable Proxy Backends Implementation Plan](/docs/architecture/fuse-native-proxy-implementation-plan), written to be ready to execute once this shape is approved.

### Relationship to the existing Fuse design

[Fuse — API Gateway](/docs/architecture/fuse-api-gateway) is the current accepted design for Fuse's routing, protocol, and auth surface, and it explicitly lists **"Non-Envoy proxy support"** as a v1 non-goal, framed as a swap: *"this design stays entirely inside Envoy's existing xDS surface; swapping proxies is a separate, much larger effort."* This document takes on that effort, but not as a swap — Envoy isn't going anywhere. Two things follow from that:

- **Phases 5 and 6 of that doc (Automatic TLS, mTLS) are unaffected, not superseded.** Reading its phased plan, only Phases 1, 2, 3, and 7 carry a ✅ — TLS and mTLS are still unbuilt on the Envoy path, and this document doesn't change that: they remain the plan for whoever runs the Envoy backend. This document's own TLS/mTLS sections below describe a parallel, native-backend-only mechanism, not a replacement for Envoy's SDS-based one.
- **Everything else in that doc is unaffected.** Route validation, conflict detection, `MatchType` (EXACT/PREFIX), subdomain matching, and the Blueprint `Gateway`/Route Detail UI are all specified against Fuse's `NetworkingService` proto and Blueprint UI — not against Envoy internals, and not against any particular backend. Both backends sit behind this same surface (see [What stays the same](#what-stays-the-same) below).

Where this document narrows the API Gateway doc's "Non-Envoy proxy support" non-goal, a small cross-reference note has been added at that point in the original doc — "narrows," not "supersedes," since Envoy support isn't being removed.

---

## What stays the same

None of this changes for a caller of Fuse:

- **`NetworkingService` proto** — `AddRoute`, `ListRoutes`, `DeleteRoute`, `ValidateRoute`, and the `Route`/`RouteMatch`/`RouteAuth`/`Endpoint` messages (as specced in the API Gateway doc) keep their shape. Nothing here requires a breaking proto change.
- **`chassis.WithRoute()`** — services keep registering routes exactly as they do today.
- **Persistence** — routes are still written to Blueprint's KV store, one route per key, loaded on boot via `LoadCache`.
- **Per-route auth semantics** — `BYPASS` / `AUTHENTICATED` / `GROUPS` / `SCOPES` mean exactly what they mean today (see [Authentication](/docs/architecture/authentication)). What changes is *who* enforces them (see [Per-route auth](#per-route-auth) below), not what they mean.
- **Blueprint's `Gateway` UI** — reads `ListRoutes` the same way; the Routing List / Route Detail views specced in the API Gateway doc are unaffected by this change.
- **The Envoy backend, entirely.** `github.com/envoyproxy/go-control-plane`, `xDSRpc`, CDS/RDS/LDS/EDS/SDS/RTDS registration, `envoy-xds-config.yaml`, `deployments/compose/envoy.yaml`, and the README's Envoy install step all stay. Nothing is removed by this document. A cluster that never touches the new config flag below sees no behavior change at all.

---

## Backend selection

A new config value, `fuse.proxy_backend`, picks which backend handles traffic — `native` (this document's new engine, and the **default**) or `envoy` (today's behavior, now an explicit opt-in). This lives in the same `fuse:` stanza `services/core/fuse/config.yaml` already has:

```yaml
fuse:
  address: http://localhost:18000    # control-plane / RPC port -- NetworkingService (AddRoute, ListRoutes, ...)
                                      # and, on the envoy backend, the xDS/ADS surface Envoy connects back to.
  proxy_backend: native              # or "envoy" -- defaults to "native" if omitted
  listener:
    address: 0.0.0.0
    port: 10000                      # data-plane / proxy port -- where actual client traffic lands.
```

`fuse.address`'s port (`18000`) and `fuse.listener.port` (`10000`) are deliberately two different config keys serving two different purposes, and **must stay different ports.** On the `envoy` backend this separation is free — Fuse's own control-plane RPC server and Envoy's data-plane listener are two different OS processes, so a collision would just be two unrelated servers that happen to share a number, caught immediately as a bind error on whichever starts second. On the `native` backend it stops being free: **the same Fuse process now opens both sockets**, so Phase 1 of the [implementation plan](/docs/architecture/fuse-native-proxy-implementation-plan#17-port-separation-validation) adds an explicit startup check that refuses to boot if they're equal, with a clear error naming both config keys — rather than surfacing as a generic "address already in use" from the OS the way a same-process double-`Listen` would otherwise fail.

### Migration note, for existing clusters

Because `native` is the default rather than `envoy`, a cluster that upgrades Fuse past this change **without** setting `fuse.proxy_backend` explicitly moves onto the native backend, not onto an unchanged Envoy path — the opposite of a typical "opt-in additive feature" upgrade. A cluster that wants to keep today's exact behavior through this upgrade needs to set `fuse.proxy_backend: envoy` explicitly before upgrading. This is called out here because it's a real, deliberate exception to the "no behavior change on upgrade" property most of the rest of this design otherwise preserves — see [Decisions](#decisions).

**A second, larger migration cost, found live rather than anticipated here:** every route in a real Draft cluster today is registered with `host.docker.internal` as its endpoint host — the convention every service's `config.yaml` uses specifically so Envoy, running in Docker, can reach a service running natively on the host (see `CLAUDE.md`'s own description of this pattern). The native backend is not in a container; it's just another native host process, the same as the services it proxies to — and `host.docker.internal` **does not resolve from the host itself**, only from inside a Docker container. Confirmed directly against a real cluster (see the [implementation plan](/docs/architecture/fuse-native-proxy-implementation-plan#phase-9--integration--testing)'s Phase 9 notes): every already-registered route in that cluster was unreachable from a native-backend Fuse, not from any bug, but because the hostname convention itself assumes an Envoy-in-Docker topology. **Flipping to `native` in a real deployment is not just a Fuse-side config change — it requires revisiting `service.network.internal.host` (or `route.host`) across every service in the cluster**, back to `localhost` or the real host address, once Envoy is no longer the process that specifically needs the Docker-facing hostname. This is a real, cluster-wide migration cost this design had not previously called out, not merely a Fuse implementation detail.

## Architecture: a `ProxyBackend` abstraction

Today, a route's path to live traffic is: `AddRoute` → validate → persist to Blueprint KV → rebuild Envoy snapshot → push over xDS → Envoy terminates the connection and proxies it. Route validation and persistence are backend-agnostic already — nothing about them mentions Envoy. What's Envoy-specific is only the last step: turning a validated route table into live traffic handling. This document pulls that step behind an interface:

```go
// ProxyBackend is implemented once per supported proxy. Fuse's own route
// validation, conflict detection, and Blueprint-KV persistence run
// identically regardless of which backend is active -- only what happens
// after AddRoute/DeleteRoute differs. Typed entirely on Fuse's own proto
// types (ntv1.Route, etc.), never on Envoy or net/http types, so a future
// backend implementation doesn't need to know either exists.
type ProxyBackend interface {
    // Apply is called with the full current route table whenever it
    // changes. The Envoy backend rebuilds and pushes an xDS snapshot; the
    // native backend swaps an in-memory table (see below).
    Apply(routes []*ntv1.Route) error

    // Capabilities reports what this backend can enforce, so route/TLS
    // config that requires something the active backend can't do fails
    // validation with a clear error instead of silently no-op'ing. See
    // Extensibility below.
    Capabilities() BackendCapabilities

    Name() string
}

type BackendCapabilities struct {
    TLS         bool
    MTLS        bool
    WideEvents  bool
    ACME        bool
}
```

The Envoy backend is today's `controller.go`/`rpc.go` logic, refactored behind this interface with no behavioral change — its `Apply` rebuilds and pushes an xDS snapshot exactly as `UpdateCacheWithNewRoute`'s callers do today. Its `Capabilities()` reflects what's actually shipped: `WideEvents: false` until [the ALS path](#wideevent-parity-for-the-envoy-backend) lands, `TLS`/`MTLS: false` until [Fuse — API Gateway](/docs/architecture/fuse-api-gateway)'s Phases 5–6 ship.

The native backend is new code, described below:

```
Backend process --AddRoute--> Fuse { validate, persist to Blueprint KV } --Apply()--> active ProxyBackend
                                                                                              |
                                                                            (native backend terminates connections itself)
                                                                                              v
                                                                TLS/mTLS (SNI-keyed) -> auth middleware -> route match -> reverse proxy -> upstream service
                                                                                              |
                                                                                              v
                                                                           chassis.StartSpan / span.End -> WideEvent -> Beacon (via Catalyst)
```

Its in-memory route table is an `atomic.Pointer[RouteTable]`, rebuilt and swapped on every `Apply` call — the same "no restart on change" property xDS gives the Envoy backend today, without a second process to push it to.

### Extensibility: more than two backends

Keeping the door open for a third backend later shapes two choices above, on purpose:

- **The interface is typed on Fuse's own proto messages**, not Envoy's config model or Go's `net/http`. A Traefik or Caddy backend (or an in-house one purpose-built for some future need) implements `Apply([]*ntv1.Route) error` however fits that proxy's own config/API — the interface doesn't assume xDS-style snapshots or in-process request handling either.
- **Backend selection follows the same shape as chassis's existing plugin pattern** — not a string-keyed registry, but typed interfaces constructed and wired explicitly by the caller, the way `chassis.Runtime.WithBroker(plugin Broker)` and `WithRepository(plugin Repository)` already work for message-broker and database plugins (`pkg/chassis/builder.go`). Fuse's own `main.go` reads `fuse.proxy_backend`, constructs the matching `ProxyBackend` implementation, and passes it into `NewControlPlane` — adding a backend later is adding one more `if`/`case` arm in `main.go` plus its own constructor, not a registry to maintain.
- **`Capabilities()` exists specifically so partial backends are safe, not silently broken.** A hypothetical minimal third backend that only does path-based HTTP routing (no TLS, no WideEvents) is still a legitimate `ProxyBackend` — it just reports that honestly. Fuse's config validation (see [Phased plan](#phased-plan), Phase 8) rejects a route's `mtls.enabled` at config time when the active backend can't enforce it — the one setting on `Route` where silently not enforcing it is a real security downgrade rather than a safe no-op; see the [implementation plan](/docs/architecture/fuse-native-proxy-implementation-plan#phase-8--capability-negotiation) for why WideEvents and TLS have nothing to check at the route level.

This document only builds two backends (`envoy`, `native`). The point of this section is that a third doesn't require revisiting this interface — see [Non-goals](#non-goals-v1) for what's explicitly *not* being built now.

### The reverse proxy engine

This section, and everything below it up to [Phased plan](#phased-plan), describes the **`native` backend specifically** — none of it applies when `fuse.proxy_backend: envoy` is selected, which keeps behaving exactly as it does today.

Built on `net/http/httputil.ReverseProxy` using its `Rewrite` hook (the current, non-deprecated API — not `Director`), with a small `http.Handler` in front of it doing route matching and auth:

- **Route matching** reuses the exact match semantics already specced in the API Gateway doc — `MatchType` (`EXACT` default / `PREFIX` opt-in) on path, single-domain and leading-wildcard (`*.draft.localhost`) on host — just consulted against the in-memory table instead of compiled into an Envoy `RouteConfiguration`. This is one implementation of one matcher, not two: whichever of these designs lands first should own the matcher, and the other reuses it.
- **HTTP/1.1** proxies via the default `http.Transport`.
- **HTTP/2 / gRPC** — routes with `enable_http2` set (same field, same opt-in as today) proxy through an `http2.Transport{AllowHTTP: true}` (cleartext h2c) to the upstream, matching the existing per-route `Http2ProtocolOptions` behavior.
- **WebSockets** — `httputil.ReverseProxy` has handled `Connection: Upgrade` requests (switching to raw bidirectional byte copying) since Go 1.12; no separate upgrade configuration is needed, unlike the HCM `UpgradeConfigs` entry the Envoy backend manages.
- **gRPC-Web needs no translation filter.** This is the one place the native backend is a simplification, not a gap to fill, relative to Envoy: every Draft backend already speaks [Connect](https://connectrpc.com/) (`connectrpc.com/connect`), which serves gRPC, gRPC-Web, and Connect's own protocol from the *same* HTTP handler with no server-side translation step. Envoy's `envoy.filters.http.grpc_web` filter exists to translate gRPC-Web into gRPC for backends that only understand gRPC — Draft's backends were never in that position. A byte-forwarding proxy in front of a Connect server needs to do nothing at all for gRPC-Web to work.

### Per-route auth

**Shipped, and verified against a real run** (Blueprint + the unmodified `envoy` backend + `services/core/auth` + Envoy, actually brought up together — see the [implementation plan](/docs/architecture/fuse-native-proxy-implementation-plan#30-what-a-live-run-found-before-writing-this-phases-real-code) for the full account). On the native backend, `RouteAuth` enforcement is Go middleware inside Fuse's own request handler, calling `services/core/auth`'s check endpoint (the same one Envoy's `ext_authz` filter calls) — the native backend becomes its own `ext_authz` caller, the same role Envoy already plays for the `envoy` backend.

That live run corrected two things this section originally assumed:

- **The check request forwards only the `Authorization` header**, not "the original request headers" generally — `Cookie`, `User-Agent`, and custom headers were all confirmed absent from the real check request Envoy sends today, since `makeExtAuthzFilter` never configures `allowed_headers`.
- **`AUTH_POLICY_GROUPS` and `AUTH_POLICY_SCOPES` behave identically to `AUTH_POLICY_AUTHENTICATED` today**, on both backends — `services/core/auth`'s check handler never reads anything derived from Envoy's `context_extensions`, so there is no group/scope distinction to replicate yet. The table below reflects that reality rather than the aspiration.

| `RouteAuth` state | Native backend middleware behavior |
|---|---|
| `auth` absent or `enabled = false` | Skip the check call entirely; proxy the request. |
| `AUTH_POLICY_BYPASS` | Skip the check call entirely; proxy the request. |
| `AUTH_POLICY_AUTHENTICATED` / `AUTH_POLICY_GROUPS` / `AUTH_POLICY_SCOPES` | Forward a check request (original method + path, `Authorization` header only) to the auth service; proxy only on `200`. All three policies are handled identically — see above. |

Because this is the same check-service contract, `services/core/auth`-side configuration doesn't change regardless of which backend is active — only which process is the HTTP client of it. A related, independent bug was found and fixed live in `services/core/auth` itself (a missing `chassis.Runtime.DisableMux()` was letting chassis's own default RPC mux race the service's manual check-server for the same port, and win) — see the implementation plan for detail; it isn't specific to either Fuse backend.

---

## TLS, mTLS, and certificates

**Native backend only, for now.** The Envoy backend's TLS/mTLS story remains [Fuse — API Gateway](/docs/architecture/fuse-api-gateway)'s Phases 5–6, populating Envoy's already-registered Secret Discovery Service — unbuilt today, and unaffected by this document. What follows is a second, independent TLS mechanism that exists only when `fuse.proxy_backend: native` is selected.

The native backend terminates TLS natively via `crypto/tls`, using `Config.GetConfigForClient` to select a certificate — and, where mTLS applies, a client-CA pool and `ClientAuth` level — keyed by the incoming SNI. This is the same one-listener-many-domains shape the Envoy-side SDS design relies on, achieved without Envoy: one `*.draft.localhost`-style listener can present a different leaf cert per subdomain, and a route that needs mTLS gets its own `RequireAndVerifyClientCert` branch without affecting any other route on the same port.

### Certificate providers

Certificate sourcing sits behind one small interface so the three cases below share a call site:

```go
type CertificateProvider interface {
    GetCertificate(domain string) (*tls.Certificate, error)
    // Changed fires when a certificate for a previously-served domain has
    // been renewed/replaced, so the TLS layer can pick it up without a
    // restart — same "no restart on change" property SDS gave Envoy.
    Changed() <-chan string
}
```

1. **Local dev — self-signed CA, generated by Fuse itself.** On first boot, the native backend generates a root CA (`crypto/x509`) and persists it (see [Open Questions](#open-questions) for where), then mints short-lived leaf certificates per requested SNI on demand — including a wildcard leaf for `*.draft.localhost`. This deliberately does *not* depend on the `mkcert` binary: one whole point of the native backend is giving a developer a path with *fewer* external binaries than `envoy`, not a different one — adding an `mkcert` dependency would be a wash. Fuse prints a one-time instruction (path to the generated root CA, and the OS/browser command to trust it) the first time it mints a cert — the same UX `mkcert -install` gives a developer today, without the binary.
2. **Operator-supplied.** A cert/key pair delivered via Blueprint KV or a mounted file pair, for clusters that already have one (an internal PKI, an existing wildcard cert, a cert an upstream load balancer already manages). This is the "bring your own" path the original design already called for.
3. **ACME.** `golang.org/x/crypto/acme/autocert` wired against an operator-chosen ACME directory (Let's Encrypt or any RFC 8555-compatible CA). Included in this design as the third `CertificateProvider` implementation, but scoped to v1 as "the interface supports it, `autocert` is wired in Phase 7" — see [Phased plan](#phased-plan) — not deep production-hardening (external-secret-manager integration, multi-CA failover, etc.), consistent with the original design's own stance: *"pluggable, not hardcoded... since Draft may run behind an operator-managed cert already."*

### Renewal

- **ACME**: `autocert`'s own `DirCache` handles renewal automatically; nothing Fuse-specific to build.
- **Self-signed dev certs**: minted with a short validity window and silently re-minted on the next handshake past expiry — no operator action, no restart.
- **Operator-supplied**: hot-reloaded on file/KV change, picked up via the same `GetConfigForClient` indirection.

All three report through `Changed()` so the TLS layer never needs a listener restart to pick up a new certificate — the same property the original SDS-based design promised, delivered without Envoy or xDS.

### mTLS

**Shipped.** An opt-in `Route.mtls` (`MTLSPolicy{enabled, trusted_ca_secret_name}`) field sets `ClientAuth: tls.RequireAndVerifyClientCert` and a `ClientCAs` pool inside `GetConfigForClient` — see the [implementation plan](/docs/architecture/fuse-native-proxy-implementation-plan#phase-6--mtls) for the real code.

**A real, unavoidable limitation, not a shortcut:** enforcement is necessarily host-scoped, not per-route. A TLS `ClientHello` — and therefore the `ClientAuth` decision — is processed once per connection, before any HTTP path is known; SNI gives a host, never a path. So a route sharing a host with an `mtls.enabled` route is covered by that requirement too, even if it never asked for a client cert itself. This is inherent to TLS-layer enforcement on any backend, Envoy included — not something a different implementation choice here would have avoided.

The client certificate's subject is available to attach to that request's WideEvent via `r.TLS.PeerCertificates[0].Subject`, once verified (see [Attribute mapping](#attribute-mapping) above).

---

## WideEvent logging as a first-class Fuse feature

**Native backend today; a path exists for Envoy too — see [below](#wideevent-parity-for-the-envoy-backend).** The important thing to understand about the native side: **the hard part is already built.** [WideEvent — Correlated Observability](/docs/architecture/wide-events) is fully specced, and its [implementation plan](/docs/architecture/wide-events-implementation-plan) is fully shipped — `WideEventsService` ingest and query exist in Beacon today (`services/core/beacon/ingest/wide_events.go`, `services/core/beacon/query/wide_events.go`), and chassis already produces a `WideEvent` automatically for any span, gated by the existing `telemetry.wide_events.enabled` config flag (`pkg/chassis/wide_event.go`). Making this "first class" for the native backend is wiring, not new infrastructure:

```go
ctx, span := chassis.StartSpan(ctx, route.Name)
defer span.End(err)

span.SetAttribute("http.method", r.Method)
span.SetAttribute("http.path", r.URL.Path)
span.SetAttribute("http.status_code", strconv.Itoa(status))
span.SetRuntimeAttribute("upstream.address", route.Endpoint.String())
span.SetRuntimeAttribute("upstream.duration_ms", strconv.FormatInt(upstreamDur.Milliseconds(), 10))
span.SetRuntimeAttribute("tls.version", tlsVersionString(r.TLS))
span.SetRuntimeAttribute("tls.cipher_suite", tlsCipherString(r.TLS))
// only set when the route requires mTLS and a client cert was presented:
span.SetRuntimeAttribute("tls.client_cert_subject", clientCertSubject)
```

This is the same three-call shape chassis's own RPC interceptor (`otelInterceptor.reportSpan` in `otel_trace.go`) already uses for every instrumented RPC handler in the framework — Fuse doesn't get a bespoke logging system, it becomes one more caller of the pattern every other Draft service already has available.

### Attribute mapping

| WideEvent field | Fuse populates it with |
|---|---|
| `attributes` | `http.method`, `http.path`, `http.status_code`, `route.name`, `route.match_type` |
| `business_attributes` | Resolved auth subject/groups/scopes, once the [per-route auth](#per-route-auth) check returns them |
| `runtime_attributes` | `upstream.address`, `upstream.duration_ms`, `tls.version`, `tls.cipher_suite`, `tls.client_cert_subject` (mTLS routes only) |

### Default: on for Fuse, opt-out per route

`telemetry.wide_events.enabled` keeps its existing cluster-wide default (**off**) — this design doesn't change that gate or its rationale (WideEvent production is meaningfully higher volume than tracing alone, per the implementation plan's own Decision). What this design adds: **once that flag is on, Fuse emits a WideEvent for every proxied request by default**, with a per-route opt-out — the inverse of the per-service default elsewhere in the framework. The reasoning: an individual service can already instrument itself and get its own WideEvents; Fuse's marginal value is specifically the cross-cutting, ingress-level view (which route, which upstream, what TLS/auth context) that no individual service's own instrumentation can produce. Defaulting Fuse's routes to *off* would mean most clusters get none of the "show me everything that happened during this failed request" value [WideEvent's own motivating example](/docs/architecture/wide-events#overview) describes, at exactly the boundary — ingress — where it's cheapest to capture once and most valuable to have.

This also closes a gap the API Gateway doc flagged explicitly: its [Request ID & trace ID propagation](/docs/architecture/fuse-api-gateway#request-id--trace-id-propagation) section noted the tracing payoff "only fully lands once Beacon exists to ingest and query them." Beacon exists now — this is that loop closing, for the native backend.

### WideEvent parity for the Envoy backend

Choosing the Envoy backend today means giving up per-request WideEvents, since Envoy is a separate process and nothing here runs Go code inside it. There's a concrete, already-half-wired path to close that gap without touching Envoy itself: `services/core/fuse/envoy-xds-config.yaml` already points a static `als_cluster` at `127.0.0.1:18090` — Envoy's [Access Log Service](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/access_loggers/grpc/v3/als.proto) (ALS), a gRPC stream Envoy pushes structured per-request log entries to. Nothing listens on `18090` today; this is the same "registered but unpopulated" shape [Fuse — API Gateway](/docs/architecture/fuse-api-gateway) already documented for SDS. If Fuse implements `envoy.service.accesslog.v3.AccessLogService` at that port, the Envoy backend can report `WideEvents: true` in its `Capabilities()` by translating each ALS entry into the same `chassis.StartSpan`/`WideEvent` production path the native backend uses — giving both backends the same observability story without either running Go code inside Envoy or forcing an Envoy-backed cluster onto the native backend just to get it.

This is deliberately **not** part of this design's v1 scope (see [Non-goals](#non-goals-v1)) — it's recorded here because it directly follows from the `ProxyBackend`/`Capabilities()` shape above, and because leaving it undocumented would make the native backend's WideEvent support look like a reason to migrate off Envoy, which cuts against this design's whole premise of Envoy staying a first-class, permanent option.

---

## Decisions

Recorded here so the reasoning doesn't get re-litigated later, matching the convention [WideEvent's own spec](/docs/architecture/wide-events#decisions) uses.

**Envoy remains a fully supported backend — the native backend does not replace it.** The compose stack, README setup flow, and bench `fuse-proxy-e2e*.yaml` workflows can all still run on Envoy, unchanged, by setting `fuse.proxy_backend: envoy`. Nothing about `ProxyBackend` removes Envoy support; it only changes which backend a cluster gets without an explicit choice.

**`native` is the default, and `envoy` requires explicit configuration.** New clusters get the native backend's benefits (no external proxy binary, built-in TLS/mTLS, WideEvent logging) without an opt-in step. The real consequence of this choice is on the upgrade path, not the greenfield one: a cluster that upgrades Fuse past this change without setting `fuse.proxy_backend` moves onto `native`, not an unchanged `envoy` path — see [Migration note](#migration-note-for-existing-clusters). This is accepted deliberately rather than defaulted around; a cluster that needs to preserve exact current behavior across the upgrade sets `fuse.proxy_backend: envoy` explicitly.

**`ProxyBackend` is typed on Fuse's own proto messages, never on Envoy's config model or `net/http` types.** This is what makes a third backend additive later rather than a rewrite of the interface — see [Extensibility](#extensibility-more-than-two-backends).

**No feature-parity guarantee across backends — `Capabilities()` makes the gap explicit instead of silent.** The native backend gets TLS/mTLS/WideEvents in this design; the Envoy backend doesn't, until its own Phases 5–6 ([Fuse — API Gateway](/docs/architecture/fuse-api-gateway)) and the [ALS path](#wideevent-parity-for-the-envoy-backend) above land, respectively. Rather than pretend both backends are equivalent, `Capabilities()` lets Fuse reject a route's `mtls.enabled` at config time when the active backend can't honor it, with a clear error — the one per-route setting where silence would be a security gap, not just a missing feature.

**No `mkcert` (or any new external binary) dependency for local dev TLS on the native backend.** It generates its own local CA. The Envoy backend keeps its own external-binary requirement (`envoy` itself) exactly as it is today — this decision is scoped to the new backend only.

**WideEvent emission on the native backend defaults to on (opt-out per route), once the existing global `telemetry.wide_events.enabled` flag is enabled.** See [Default: on for Fuse, opt-out per route](#default-on-for-fuse-opt-out-per-route) above. This does not change that flag's own cluster-wide default (still off), and has no effect at all when the Envoy backend is active.

**ACME shipped as Phase 7**, sequenced after local self-signed dev and operator-supplied certs (Phase 5) precisely because nothing about the `CertificateProvider` interface forced it to happen earlier, and Draft's deployment story (`docker-compose`, no in-repo Kubernetes ingress/cert-manager config) still doesn't have a cluster that strictly needs it yet — but it's built, not just designed-for. See the [implementation plan](/docs/architecture/fuse-native-proxy-implementation-plan#phase-7--acme-provider) for how `HostPolicy` ties ACME issuance to the live route table instead of a static domain list.

**The native backend validates at startup that its proxy listener port (`fuse.listener.port`) differs from Fuse's own control-plane RPC port (`fuse.address`'s port).** These were always two different config keys, but collapsing the control plane and data plane into one process (this document's whole premise) means a config mistake that sets them equal now fails as a same-process double-bind instead of two independent processes racing for a port — worth a clear, named error rather than whatever generic message the OS returns. See [Backend selection](#backend-selection).

---

## Phased plan

1. **`ProxyBackend` extraction.** Introduce the interface and `fuse.proxy_backend` config flag (default `native`); refactor today's xDS-snapshot logic in `controller.go`/`rpc.go` behind it as the `envoy` backend, unchanged when explicitly selected. Add the port-separation startup validation from [Backend selection](#backend-selection). Validated against the existing `tests/fuse` and bench `fuse-proxy-e2e*.yaml` workflows, run with `fuse.proxy_backend: envoy` explicitly set, before anything else starts.
2. **Native backend — HTTP/1.1 + HTTP/2 (h2c) reverse proxy.** `httputil.ReverseProxy` reading an in-memory route table fed by `Apply()`; WebSocket upgrade; a feature-parity pass against the Envoy backend's routing behavior (prefix/exact match, per-route `enable_http2`). This is what a cluster gets by default from Phase 1 onward whenever it doesn't set `fuse.proxy_backend: envoy`.
3. **Auth middleware.** Go-native `ext_authz`-equivalent for the native backend, same Authentik outpost protocol, same `RouteAuth` policy table the Envoy backend already enforces.
4. **WideEvent integration.** Wrap proxied requests in `chassis` spans on the native backend; attribute mapping above; per-route opt-out flag; document `telemetry.wide_events.enabled` and the backend choice together as part of Fuse's setup story.
5. **TLS.** Self-signed local-CA provider and operator-supplied provider on the native backend; SNI-based `GetConfigForClient`; hot-reload via `Changed()`.
6. **mTLS.** Per-route client-cert requirement on the native backend, building on Phase 5's `GetConfigForClient` branching.
7. **ACME provider.** `autocert`-backed third `CertificateProvider` implementation for the native backend.
8. **Capability negotiation.** Implement `Capabilities()` on both backends honestly; add config-time validation that rejects a route's `mtls.enabled` against a backend that can't fulfill it, instead of silently ignoring the setting — the one per-route setting where that silence would be a security gap (see the implementation plan for why TLS/WideEvents have nothing analogous to check).

Each phase is independently shippable and gated on the previous one working, not on the whole design landing at once. There is no "decommission Envoy" phase — see [Non-goals](#non-goals-v1).

---

## Non-goals, v1

- **Removing or deprecating the Envoy backend.** It stays fully supported as an explicit opt-in (`fuse.proxy_backend: envoy`); this design is additive only, even though it isn't the default.
- **Feature parity between backends in v1.** The native backend gets TLS/mTLS/WideEvents; the Envoy backend doesn't, until its own separately-tracked work lands (Phases 5–6 of the API Gateway doc; the [ALS path](#wideevent-parity-for-the-envoy-backend) above). `Capabilities()` (Phase 8) makes this gap visible instead of silently accepting settings a backend can't honor.
- **Building a third backend (Traefik, Caddy, NGINX, ...) now.** The interface is shaped to admit one later (see [Extensibility](#extensibility-more-than-two-backends)); none is built as part of this design.
- **General-purpose WAF or rate limiting** — carried over from the API Gateway doc's own non-goals; still out of scope here.
- **Multi-cluster / global load balancing** — Fuse remains a single-cluster proxy regardless of backend.
- **Route change audit history** — unchanged from the API Gateway doc's stance.
- **Feature parity with Envoy's admin surface** (`/stats`, `/config_dump`, etc.) on the native backend — Beacon's WideEvent/metrics path is the intended replacement for operational visibility, not a re-implementation of Envoy's admin API.
- **Production-grade ACME hardening** (external secret-manager-issued certs, multi-CA failover, HTTP-01 vs TLS-ALPN-01 selection beyond `autocert`'s defaults) — the `CertificateProvider` interface supports building this later; this design doesn't commit to it now.

## Open Questions

- ~~**mTLS's exact proto shape**~~ — **Resolved in the [implementation plan](/docs/architecture/fuse-native-proxy-implementation-plan#phase-6--mtls), Phase 6:** a new `Route.mtls` (`MTLSPolicy`) field, not an extension of `RouteAuth` — mTLS is a transport-handshake-time decision, before any HTTP request (and therefore any `RouteAuth` check) exists.
- ~~**Where the local self-signed root CA persists**~~ — **Resolved: Blueprint KV.** Survives with the cluster; a `dctl infra` reset regenerating a new CA is accepted as consistent with everything else Blueprint owns resetting together. See the [implementation plan](/docs/architecture/fuse-native-proxy-implementation-plan#521-ca-persistence--resolves-the-design-docs-open-question), 5.2.1.
- **Does `enable_http2` stay an explicit per-route opt-in**, as it is today, or does the native backend negotiate h2c automatically from registration metadata? Kept as today's explicit opt-in for v1; revisit only if it proves to be a footgun in practice.
- **Whether the ALS-based parity path for the Envoy backend (see [above](#wideevent-parity-for-the-envoy-backend)) is worth building at all**, versus simply telling an Envoy-backed cluster that wants WideEvents to switch to the native backend. Recorded as a real option, not a commitment either way.

## Landscape

Updated from the API Gateway doc's version now that Fuse can be either its own data plane or Envoy's control plane, depending on `fuse.proxy_backend`:

| | Caddy | Traefik | Raw Envoy | Fuse (`envoy` backend) | Fuse (`native` backend) |
|---|---|---|---|---|---|
| Config source | Caddyfile / JSON API | Docker/K8s labels, file, Consul | Static config + xDS | Blueprint KV, via a typed RPC | Blueprint KV, via a typed RPC |
| Dynamic reload | Yes | Yes | Only if you build an xDS control plane | Already an xDS control plane | Native — in-process route table swap |
| Auto TLS | Yes (built-in ACME) | Yes (built-in ACME) | No (bring your own SDS) | Planned (Phases 5–6, API Gateway doc) | Local self-signed + operator-supplied now; ACME provider included |
| mTLS | Yes | Yes | Bring your own SDS | Planned (same phases) | Per-route, SNI-scoped |
| Service registration | External (labels/config) | External (labels/config) | External (your control plane) | Native — same `Register` path every Draft process already uses | Same |
| Request-level observability | Access logs | Access logs, optional tracing | Access logs, optional tracing | Access logs today; WideEvent parity possible via ALS (see above) | WideEvent per request, pre-joined with route/TLS/auth context, queryable in Beacon |
| Operator UI | None built-in | Dashboard (read-only-ish) | None built-in | Full read/write in Blueprint (see API Gateway doc) | Same |

Fuse's differentiator is unchanged from the original doc's framing: it isn't a separate product bolted beside the cluster, it's already the cluster's control plane — this design gives it the *option* to be the data plane too, without giving up the control plane it's always been for Envoy.
