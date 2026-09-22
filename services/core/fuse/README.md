# Fuse
A programmable `control plane` for the `envoy` proxy.

# Setup
1. [Install Envoy Proxy](https://www.envoyproxy.io/docs/envoy/latest/start/install)
__NOTE:__ It's assumed that `envoy` will be available on your path.

2. Start `blueprint` and `fuse` with the configuration file found in the tests directory (`tests/fuse/config.yaml`)

3. Start `envoy` with the default dynamic `xds` configuration. Envoy will be configured to connect to port `:18000` to get it's routing information.
```sh
envoy -c envoy-xds-config.yaml --drain-time-s 1 -l debug
```

Dynamic routing is now setup.

# Proxy backend

`fuse.proxy_backend` (`config.yaml`) selects which `ProxyBackend` handles traffic: `envoy` (the setup above; currently the default) or `native` — Fuse terminating connections itself, with no separate `envoy` process. See [Fuse — Pluggable Proxy Backends](../../../docs/website/content/docs/architecture/fuse-native-proxy.md) for the design and current status.

# WideEvent logging (native backend only)

On the `native` backend, every proxied request can produce a [WideEvent](../../../docs/website/content/docs/architecture/wide-events.md) — a single correlated record (route, upstream, status, duration, and more) queryable in Beacon. Two flags control this together, and both need to be considered to get the behavior you expect:

- **`telemetry.wide_events.enabled`** (`config.yaml`, cluster-wide, default `false`) — the same flag every WideEvent producer in the framework uses. Off by default because WideEvent production is materially higher volume than tracing alone.
- **`Route.wide_events_disabled`** (per route, set by the registering service via `chassis.WithRoute()`, default `false` meaning "emit") — an opt-*out*, not opt-in. This is the inverse of the per-service default elsewhere in the framework: once `telemetry.wide_events.enabled` is on, Fuse emits a WideEvent for every route by default, since Fuse is the one process positioned to see every request in the cluster, not just its own inbound RPCs. Set it to `true` on a specific route to exclude it.

Neither flag does anything on the `envoy` backend today — see the design doc's [WideEvent parity for the Envoy backend](../../../docs/website/content/docs/architecture/fuse-native-proxy.md#wideevent-parity-for-the-envoy-backend) section for the (unbuilt) path to closing that gap.