---
weight: 36
title: Service UIs via Subdomains
description: Every service's own UI (Blueprint, Beacon, Bench, Garage) reachable through Fuse on a dedicated subdomain, self-discovered in Blueprint's sidebar — plus the real bugs found building it.
icon: 'hub'
draft: false
toc: true
---

Every Draft service that ships a UI used to be reachable only by hitting its own bind port directly — `localhost:2221` for Blueprint, `:2222` for Beacon, `:9300` for Bench, `:9301` for Garage. None of them were routed through Fuse. This is the design and the real bugs found wiring that up, following [Fuse — API Gateway](/docs/architecture/fuse-api-gateway)'s subdomain-per-service plan (its Phase 3).

## The convention

One route per service, registered the same way every RPC route already is (`chassis.WithRoute`), with two fields that make it a *UI* route rather than an RPC-only one:

```go
WithRoute(&ntv1.Route{
    Name: "core-beacon-ui",
    Match: &ntv1.RouteMatch{
        Host:   "beacon.draft.localhost",
        Prefix: "/",
    },
    EnableHttp2: true,
})
```

- **`Host: "<service>.draft.localhost"`, `Prefix: "/"`** — a dedicated subdomain, catching everything. Since `chassis.WithClientApplication` already mounts a service's own UI at `/` on its own internal mux (dispatching to more specific RPC handler patterns first via normal `http.ServeMux` precedence), one route per service is enough to expose *both* its UI and its RPC API — no path rewriting, no second listener. `*.localhost` — any depth of subdomain, not just one label — already resolves to `127.0.0.1` in modern browsers (RFC 6761) with zero `/etc/hosts` config.
- **An explicit `Name`.** `chassis.WithRoute` auto-derives an unset `Route.Name` from `"<domain>-<service>"` — the same value every *other* unnamed `WithRoute` call from that same service would also derive. A service that already registers one RPC-only route (Beacon, Garage) needs its UI route explicitly named, or the second call silently overwrites the first instead of adding a second route.
- **`EnableHttp2: true`.** See [Why every route needs it](#why-every-route-needs-enablehttp2) below — this one is easy to forget and fails in a confusing way.

Four services carry this today: `blueprint.draft.localhost`, `beacon.draft.localhost`, `bench.draft.localhost`, `garage.draft.localhost` (`core-blueprint-ui`, `core-beacon-ui`, `tooling-bench-ui`, `tooling-garage-ui` in Fuse's route table).

## Self-discovery in Blueprint's sidebar

No new proto field, no separate registry. Blueprint's sidebar calls `ListRoutes`, and treats **any route with a non-empty `host` and `prefix == "/"`** as a UI worth linking to — exactly the shape every call above already has. A label is derived from the host (`beacon.draft.localhost` → "Beacon") and the link points at `{current protocol}//{host}:{current port}/`, reusing the viewing page's own scheme/port so the same code works whether Fuse listens on `:10000` locally or `:443` in a real deployment (`service_url`/`service_label`, `services/core/blueprint/web-client/src/main.rs`). Add a fifth service the same way the first four were added, and it shows up in the sidebar with no Blueprint-side change at all.

## Real bugs found building this

Every one of these was invisible until actual browser traffic went through Fuse for real — curl-based checks with a hand-set `Host` header masked two of them outright.

### Blueprint can't `WithRoute` on itself

Every other service's `WithRoute` call runs synchronously, before `Start()`, against an already-running Blueprint. Blueprint calling `WithRoute` on *itself* the same way panics: `WithRoute` queries Blueprint's own KV store for Fuse's address, and Blueprint hasn't started serving that KV store yet at that point in its own `main()`. Fixed by moving Blueprint's UI-route registration into a `WithRunner` goroutine that polls plain TCP reachability against its own entrypoint before calling `WithRoute` once — see `registerBlueprintUIRoute` in `services/core/blueprint/main.go`. (Retrying `WithRoute` itself directly, catching its panic, also would have worked, but `WithRoute` logs at Panic level on every failed attempt before panicking — a TCP probe first keeps that log line to the one attempt that's expected to succeed.)

### A rejected `AddRoute` looked like success

`chassis.withRoute` (`pkg/chassis/networking.go`) checked only the RPC call's transport-level `err`, never `AddRouteResponse.GetCode()`. A route Fuse actually rejected — say, for conflicting with an existing one — came back as an ordinary, error-free response, so the registering service logged "successfully added route" and moved on, with nothing actually persisted. This sat undetected until Garage's real `PluginCatalogService` route silently lost to a **stale, orphaned KV entry** named `tooling-bench` (auto-derived from `<domain>-<service>`, matching what today's Bench would derive too, but written by some earlier version of Bench's code that no longer exists) claiming the identical `(host="", prefix)` pair. `ListRoutes` and `ListRoutes` via Blueprint's own KV directly both agreed: Garage's route was never there. Deleting the orphan unblocked it immediately. Fixed at the source: `withRoute` now checks `resp.Msg.GetCode()` and turns a non-`OK` response into a real Go `error`, so a future conflict fails loudly (panics, per every other `WithRoute` failure) instead of silently no-opping.

### Browsers include the port; Fuse's routes didn't

A browser sends `Host: blueprint.draft.localhost:10000` for anything on a non-default port — but a route's `Match.Host` is registered as the bare `"blueprint.draft.localhost"`, no port. Envoy matches `VirtualHost.Domains` against the request's Host header *as sent* unless told otherwise, so the request fell through to the catch-all `"*"` virtual host — serving whatever else claims `/` there (`examples-file_host`'s "Hello World") instead of Blueprint. `curl -H "Host: blueprint.draft.localhost"` (no port) doesn't reproduce this, which is exactly why it wasn't caught until an actual browser hit the real listener port. Fixed once, cluster-wide, in Fuse's `HttpConnectionManager`:

```go
StripPortMode: &hcm.HttpConnectionManager_StripAnyHostPort{
    StripAnyHostPort: true,
},
```

### Why every route needs `EnableHttp2`

Fuse's [`grpc_web` filter](/docs/architecture/fuse-api-gateway#phase-2--grpc-web) bridges a browser's grpc-web request into plain gRPC before forwarding it upstream — and plain gRPC is HTTP/2-only. Every chassis-based backend already speaks HTTP/2 cleartext regardless (`pkg/chassis/builder.go`'s `Start` always wraps its handler in `h2c.NewHandler`), but Envoy's *cluster* config for a route defaults to HTTP/1.1 unless `Route.EnableHttp2` says otherwise. Without it, Envoy has a plain-gRPC request to send and only an HTTP/1.1-capable upstream cluster to send it over — the browser's own RPC calls came back as an opaque "malformed response," with no hint that the fix was a boolean on the *route*, not anything in the client. Symptom only showed up once a real `tonic_web_wasm_client` (Blueprint's own web client) call went through Fuse; plain Connect-JSON traffic (`curl`, `garage://http-call@v1`) never exercises the grpc-web path at all, so it looked fine.

### `API_DOMAIN` stopped meaning "Fuse" once subdomains were real

Blueprint's `NetworkingServiceClient` calls (`gateway.rs`, `route_detail.rs`, and the sidebar's own discovery fetch) used to resolve `API_DOMAIN`, which defaults to the current page's own origin. That was a reasonable default when Fuse was the only thing anything could plausibly be served through — but with each service on its own subdomain, a page served from `blueprint.draft.localhost` is same-origin with *Blueprint's* backend, not Fuse's. Added `FUSE_DOMAIN` (`main.rs`), the same shape as the pre-existing `CATALYST_DOMAIN`: a build-time-overridable constant defaulting to Fuse's own direct address (`http://localhost:18000`), used anywhere Blueprint's client needs Fuse's control-plane API specifically, regardless of which subdomain happens to be currently loaded.

## Non-goals here

- **Path-based routing** (`draft.localhost/beacon/`) — rejected in the original brief; would need Envoy-side prefix rewriting and every UI to become prefix-aware. Subdomains need neither.
- **A dedicated "this route is a UI" proto field.** The `(host != "", prefix == "/")` convention costs nothing to adopt and nothing to maintain; a real flag is easy to add later if the convention ever stops being sufficient.
- **Automatic TLS for these subdomains.** Still tracked as its own phase in [Fuse — API Gateway](/docs/architecture/fuse-api-gateway#phase-5--automatic-tls) — SNI-based cert selection composes directly with per-subdomain routing once it lands.
