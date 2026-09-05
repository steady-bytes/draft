#!/usr/bin/env bash
#
# Builds and runs a full local Draft cluster: every core service (Blueprint,
# Catalyst, Fuse, Beacon), every example service Bench's own workflows/
# actually exercise as a real backend (examples/crud for crud-e2e.yaml,
# examples/echo for fuse-proxy-e2e.yaml and fuse-proxy-e2e-10x.yaml), and
# every tooling service (Bench, Garage, slack-notify, catalyst-consume,
# http-call, grpc-call, catalyst-produce), plus Draft's own documentation
# site (docs/website, a Hugo-modules site -- distinct from the
# steady-bytes.com marketing site living outside this repo), and every
# infra dependency the cluster needs to actually work end to end — Postgres
# (Bench/Garage/crud), ClickHouse (Catalyst's optional event store and
# Beacon's log/trace/metric store), and Envoy (Fuse's data-plane proxy,
# which Fuse itself never starts — it only runs an xDS control-plane server
# that a separately-running Envoy connects to). Every workflow under
# services/tooling/bench/workflows/ (Bench's own seed directory) passes
# against this stack — verified live. The other examples/* services
# (auth, consumer, consumer2, producer, producer2, file_host) aren't
# referenced by any workflow there, so they're deliberately left out.
#
# This is a broader sibling of scripts/run-tooling-stack.sh, not a
# replacement for it: that script stays as the lighter tooling-only stack
# (no Envoy, no ClickHouse) for when you don't need the full picture.
#
# Usage:
#   ./scripts/run-local.sh          # build, start everything, stay in the
#                                    # foreground until Ctrl+C
#
# On exit (Ctrl+C, or any error after infra is up), every process this
# script started and every container it started are torn down.
#
# Proxying through Envoy's data plane (port 10000) to these native host
# processes does work, despite Envoy running in Docker: each service's
# checked-in config.yaml sets a route.host of "host.docker.internal"
# specifically for Fuse's route-endpoint auto-fill (distinct from
# internal.host, "localhost", used for every native-process-to-native-process
# call), and Docker Desktop for macOS resolves that back to the host from
# inside the Envoy container. Verified live: blueprint.draft.localhost,
# garage.draft.localhost, bench.draft.localhost, and beacon.draft.localhost
# (plus Beacon's /core.observability. Connect-RPC prefix) all proxy through
# port 10000 successfully with real x-envoy-upstream-service-time headers,
# not something Envoy served itself.
#
# URLs once running:
#   Garage UI                          http://localhost:9301/
#   Bench UI                           http://localhost:9300/
#   Blueprint web client                http://localhost:2221/
#   Beacon web client                   http://localhost:2222/
#   slack-notify (RPC only)             http://localhost:9302/
#   catalyst-consume (RPC only)         http://localhost:9303/
#   http-call (RPC only)                http://localhost:9304/
#   grpc-call (RPC only)                http://localhost:9305/
#   catalyst-produce (RPC only)         http://localhost:9306/
#   crud (examples, RPC only)           http://localhost:9090/
#   echo (examples, RPC only)           http://localhost:9091/
#   Documentation site (Hugo)           http://localhost:1313/
#   Envoy admin                         http://localhost:19000/
#   Envoy data plane (proxies blueprint/garage/bench/beacon.draft.localhost + Beacon's RPC prefix) http://localhost:10000/
#   ClickHouse HTTP (catalyst events + beacon logs/traces/metrics) http://localhost:8123/
#
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUN_DIR="$REPO_ROOT/.local-stack"
BIN_DIR="$RUN_DIR/bin"
LOG_DIR="$RUN_DIR/logs"

POSTGRES_CONTAINER="draft-local-postgres"
POSTGRES_PORT=5432
CLICKHOUSE_CONTAINER="draft-local-clickhouse"
CLICKHOUSE_NATIVE_PORT=9000
CLICKHOUSE_HTTP_PORT=8123
ENVOY_CONTAINER="draft-local-envoy"
ENVOY_IMAGE="envoyproxy/envoy:v1.31.2"
ENVOY_DATA_PLANE_PORT=10000
ENVOY_ADMIN_PORT=19000
ENVOY_ALS_PORT=18090
ENVOY_BOOTSTRAP="$RUN_DIR/envoy-bootstrap.yaml"
DOCS_PORT=1313

mkdir -p "$BIN_DIR" "$LOG_DIR"

PIDS=()
CONTAINERS=()

log()  { printf '\033[1;34m==>\033[0m %s\n' "$1"; }
warn() { printf '\033[1;33m!!\033[0m %s\n' "$1" >&2; }
die()  { printf '\033[1;31mERROR:\033[0m %s\n' "$1" >&2; exit 1; }

cleanup() {
  local exit_code=$?
  trap - EXIT INT TERM
  echo
  log "Shutting down..."
  for pid in "${PIDS[@]:-}"; do
    [ -n "${pid:-}" ] && kill "$pid" 2>/dev/null || true
  done
  # Give each process a moment to run its own graceful-shutdown effect chain
  # (KV/catalog retraction, etc.) before anything gets force-killed.
  sleep 1
  for pid in "${PIDS[@]:-}"; do
    [ -n "${pid:-}" ] && kill -9 "$pid" 2>/dev/null || true
  done
  for container in "${CONTAINERS[@]:-}"; do
    [ -n "${container:-}" ] && docker rm -f "$container" >/dev/null 2>&1 || true
  done
  log "Stopped. Logs kept at $LOG_DIR"
  exit "$exit_code"
}
trap cleanup EXIT INT TERM

port_in_use() {
  lsof -i ":$1" -sTCP:LISTEN >/dev/null 2>&1
}

wait_for_tcp() {
  local host="$1" port="$2" name="$3" tries=60
  for ((i = 0; i < tries; i++)); do
    if (exec 3<>"/dev/tcp/$host/$port") 2>/dev/null; then
      exec 3>&- 3<&-
      return 0
    fi
    sleep 0.5
  done
  die "$name did not start listening on $host:$port within $((tries / 2))s — check $LOG_DIR/$name.log"
}

start_bg() {
  # start_bg <name> <working-dir> <binary> [env=val ...]
  local name="$1" dir="$2" bin="$3"
  shift 3
  (
    cd "$dir" || exit 1
    exec env "$@" "$bin"
  ) >"$LOG_DIR/$name.log" 2>&1 &
  local pid=$!
  PIDS+=("$pid")
  log "$name started (pid $pid), logs: $LOG_DIR/$name.log"
}

# ---------------------------------------------------------------------------
# Preflight: fail fast and clearly if a port we need is already taken by
# something else, rather than producing a confusing bind error buried in a
# log file.
# ---------------------------------------------------------------------------
for p in 2221 2220 18000 2222 4317 9090 9091 9300 9301 9302 9303 9304 9305 9306 \
         "$DOCS_PORT" \
         "$POSTGRES_PORT" "$CLICKHOUSE_NATIVE_PORT" "$CLICKHOUSE_HTTP_PORT" \
         "$ENVOY_DATA_PLANE_PORT" "$ENVOY_ADMIN_PORT" "$ENVOY_ALS_PORT"; do
  if port_in_use "$p"; then
    die "port $p is already in use — stop whatever's using it (or a previous run of this script, or run-tooling-stack.sh, that didn't get torn down) before retrying"
  fi
done

command -v docker >/dev/null 2>&1 || die "docker is required (for Postgres/ClickHouse/Envoy) but isn't on PATH"
docker info >/dev/null 2>&1 || die "docker daemon isn't running"
command -v hugo >/dev/null 2>&1 || die "hugo is required (for the docs site — brew install hugo) but isn't on PATH"

# ---------------------------------------------------------------------------
# Postgres — one instance, two databases (bench, garage), matching each
# tooling service's checked-in config.yaml (repositories.postgres.url).
# Same setup as run-tooling-stack.sh.
# ---------------------------------------------------------------------------
log "Starting Postgres"
docker run -d --name "$POSTGRES_CONTAINER" \
  -p "$POSTGRES_PORT:5432" \
  -e POSTGRES_PASSWORD=postgres \
  postgres:16-alpine >/dev/null
CONTAINERS+=("$POSTGRES_CONTAINER")

log "Waiting for Postgres to accept connections"
for ((i = 0; i < 60; i++)); do
  docker exec "$POSTGRES_CONTAINER" pg_isready -U postgres >/dev/null 2>&1 && break
  sleep 0.5
done
docker exec "$POSTGRES_CONTAINER" pg_isready -U postgres >/dev/null 2>&1 \
  || die "postgres never became ready"

log "Creating bench/garage/crud roles and databases"
docker exec -i "$POSTGRES_CONTAINER" psql -U postgres -v ON_ERROR_STOP=1 <<'SQL' >/dev/null \
  || die "failed to create bench/garage/crud roles/databases"
CREATE ROLE bench LOGIN PASSWORD 'bench';
CREATE DATABASE bench OWNER bench;
CREATE ROLE garage LOGIN PASSWORD 'garage';
CREATE DATABASE garage OWNER garage;
-- examples/crud's checked-in config.yaml (repositories.postgres.url) points at
-- role/database "draft" specifically, not "crud" -- matching that exactly
-- rather than overriding it via DRAFT_ env vars like bench/garage's
-- internal.host, since nothing else on this stack needs that name.
CREATE ROLE draft LOGIN PASSWORD 'draft';
CREATE DATABASE draft OWNER draft;
SQL

# ---------------------------------------------------------------------------
# ClickHouse — Catalyst's optional event store (services/core/catalyst/
# config.yaml has clickhouse.enabled: true, address localhost:9000, database
# "catalyst"; Catalyst falls back to a no-op store if this isn't reachable,
# but this script wires up the real thing since that's what the user asked
# for). Image/env/ports match services/core/catalyst/docker-compose.yaml
# exactly — that's the authoritative local ClickHouse setup for this
# service, not something to reinvent here. CLICKHOUSE_DB=catalyst matters:
# Catalyst's own migrate() only does CREATE TABLE IF NOT EXISTS for its
# events table, it never creates the database itself. Beacon shares this
# same server on its own "beacon" database (services/core/beacon/
# config.yaml) — unlike Catalyst, Beacon's own store bootstraps that
# database itself (CREATE DATABASE IF NOT EXISTS) on startup, so nothing
# extra is needed here for it.
# ---------------------------------------------------------------------------
log "Starting ClickHouse"
docker run -d --name "$CLICKHOUSE_CONTAINER" \
  -p "$CLICKHOUSE_NATIVE_PORT:9000" \
  -p "$CLICKHOUSE_HTTP_PORT:8123" \
  -e CLICKHOUSE_DB=catalyst \
  -e CLICKHOUSE_USER=default \
  -e CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT=1 \
  -e CLICKHOUSE_PASSWORD= \
  clickhouse/clickhouse-server:24.8 >/dev/null
CONTAINERS+=("$CLICKHOUSE_CONTAINER")

log "Waiting for ClickHouse to accept connections"
for ((i = 0; i < 60; i++)); do
  docker exec "$CLICKHOUSE_CONTAINER" clickhouse-client --query "SELECT 1" >/dev/null 2>&1 && break
  sleep 0.5
done
docker exec "$CLICKHOUSE_CONTAINER" clickhouse-client --query "SELECT 1" >/dev/null 2>&1 \
  || die "clickhouse never became ready"

# ---------------------------------------------------------------------------
# Documentation site (docs/website) -- Draft's own architecture docs, a
# separate Hugo-modules site from the steady-bytes.com marketing site
# (../../../website, outside this repo). Entirely independent of the rest
# of the cluster (no Blueprint/Fuse registration, no ordering dependency),
# so it's started here rather than woven into the service startup order
# below. `hugo server` has its own built-in live-reload on content changes,
# so unlike the Go services this needs no rebuild/restart wrapper.
# ---------------------------------------------------------------------------
log "Starting Documentation site (Hugo)"
(
  cd "$REPO_ROOT/docs/website" || exit 1
  exec hugo server --port "$DOCS_PORT" --bind 127.0.0.1
) >"$LOG_DIR/docs.log" 2>&1 &
DOCS_PID=$!
PIDS+=("$DOCS_PID")
log "docs started (pid $DOCS_PID), logs: $LOG_DIR/docs.log"
wait_for_tcp localhost "$DOCS_PORT" docs

# ---------------------------------------------------------------------------
# Build everything up front, so a compile error surfaces immediately instead
# of mid-startup. Each service is its own Go module (this monorepo's
# convention throughout), so each build runs in its own directory.
# ---------------------------------------------------------------------------
log "Building core services"
(cd "$REPO_ROOT/services/core/blueprint" && go build -o "$BIN_DIR/blueprint" .) || die "blueprint build failed"
(cd "$REPO_ROOT/services/core/catalyst" && go build -o "$BIN_DIR/catalyst" .) || die "catalyst build failed"
(cd "$REPO_ROOT/services/core/fuse" && go build -o "$BIN_DIR/fuse" .) || die "fuse build failed"
(cd "$REPO_ROOT/services/core/beacon" && go build -o "$BIN_DIR/beacon" .) || die "beacon build failed"

log "Building example services"
(cd "$REPO_ROOT/services/examples/crud" && go build -o "$BIN_DIR/crud" .) || die "crud build failed"
(cd "$REPO_ROOT/services/examples/echo" && go build -o "$BIN_DIR/echo" .) || die "echo build failed"

log "Building tooling services"
(cd "$REPO_ROOT/services/tooling/bench" && go build -o "$BIN_DIR/bench" .) || die "bench build failed"
(cd "$REPO_ROOT/services/tooling/garage" && go build -o "$BIN_DIR/garage" .) || die "garage build failed"
(cd "$REPO_ROOT/services/tooling/slack-notify" && go build -o "$BIN_DIR/slack-notify" .) || die "slack-notify build failed"
(cd "$REPO_ROOT/services/tooling/catalyst-consume" && go build -o "$BIN_DIR/catalyst-consume" .) || die "catalyst-consume build failed"
(cd "$REPO_ROOT/services/tooling/http-call" && go build -o "$BIN_DIR/http-call" .) || die "http-call build failed"
(cd "$REPO_ROOT/services/tooling/grpc-call" && go build -o "$BIN_DIR/grpc-call" .) || die "grpc-call build failed"
(cd "$REPO_ROOT/services/tooling/catalyst-produce" && go build -o "$BIN_DIR/catalyst-produce" .) || die "catalyst-produce build failed"

# ---------------------------------------------------------------------------
# Start order: Blueprint first (everything else registers with it), then
# Catalyst (now that ClickHouse is up for it to connect to), then Fuse, then
# Envoy (needs Fuse's xDS server already listening on 18000 before it can
# pull config), then the tooling services — slack-notify last since it needs
# Garage's PluginCatalogService reachable before its own startup Effect (a
# fatal one — see pkg/chassis/effect.go) tries to publish to it.
#
# service.network.internal.host in bench's and garage's checked-in
# config.yaml is host.docker.internal (set for a container-networked
# deployment); overridden to localhost here via chassis's DRAFT_ env-var
# convention (pkg/chassis/config.go: SetEnvPrefix("DRAFT") +
# SetEnvKeyReplacer(".", "_")) since these are plain native binaries on the
# host, not containerized.
# ---------------------------------------------------------------------------
log "Starting Blueprint"
start_bg blueprint "$REPO_ROOT/services/core/blueprint" "$BIN_DIR/blueprint"
wait_for_tcp localhost 2221 blueprint

log "Starting Catalyst"
start_bg catalyst "$REPO_ROOT/services/core/catalyst" "$BIN_DIR/catalyst"
wait_for_tcp localhost 2220 catalyst

log "Starting Fuse"
# fuse.listener.address overridden to 0.0.0.0 for the same reason
# deployments/compose/fuse.yaml sets it that way and services/core/fuse's own
# checked-in config.yaml (listener.address: localhost) doesn't: this is the
# bind address Fuse hands Envoy in the Listener resource it pushes over xDS,
# and Envoy's socket_address rejects a hostname there ("malformed IP
# address: localhost") — confirmed live, every listener push was rejected
# and Envoy stayed at 0 listeners until this was overridden.
start_bg fuse "$REPO_ROOT/services/core/fuse" "$BIN_DIR/fuse" \
  DRAFT_FUSE_LISTENER_ADDRESS=0.0.0.0
wait_for_tcp localhost 18000 fuse

# Beacon registers its UI and Connect-RPC routes with Fuse synchronously,
# before it starts serving (same pattern as Catalyst/Fuse, unlike Blueprint's
# own registerBlueprintUIRoute — see pkg/chassis/builder.go's WithRoute), so
# it must start after Fuse is already listening on 18000, not before.
log "Starting Beacon"
start_bg beacon "$REPO_ROOT/services/core/beacon" "$BIN_DIR/beacon"
wait_for_tcp localhost 2222 beacon

# crud's own WithRoute call is synchronous too (same reasoning as Beacon's
# above), so it likewise needs Fuse already up. It also needs the "draft"
# Postgres role/database created earlier in this script — its checked-in
# config.yaml (repositories.postgres.url) points there directly, no
# DRAFT_ env override needed.
log "Starting crud (examples)"
start_bg crud "$REPO_ROOT/services/examples/crud" "$BIN_DIR/crud"
wait_for_tcp localhost 9090 crud

# echo's own WithRoute call is synchronous too, same reasoning as crud/beacon
# above -- needs Fuse already up. Required for Bench's fuse-proxy-e2e and
# fuse-proxy-e2e-10x workflows, which call it through Envoy's real data
# plane to prove Fuse's dynamic routing end to end.
log "Starting echo (examples)"
start_bg echo "$REPO_ROOT/services/examples/echo" "$BIN_DIR/echo"
wait_for_tcp localhost 9091 echo

# ---------------------------------------------------------------------------
# Envoy — Fuse never starts this itself, it only runs the xDS control-plane
# server Envoy pulls config from (services/core/fuse/main.go). The bootstrap
# below is structurally deployments/compose/envoy.yaml's (the proven,
# already-working Docker-Compose version — STRICT_DNS'd, unlike
# services/core/fuse/envoy-xds-config.yaml's own copy, which targets a bare
# 0.0.0.0 and isn't meant to be used standalone), written out directly
# (rather than sed-patched from that file) so dns_lookup_family: V4_ONLY can
# be baked in from the start: Docker Desktop's host.docker.internal resolves
# to *both* an A and a AAAA record, and Envoy's STRICT_DNS cluster picking
# the IPv6 one first fails to connect (confirmed live — every xds_cluster
# connection attempt failed until this was pinned to V4_ONLY), since Fuse
# only listens on 0.0.0.0 (IPv4). `address: host.docker.internal` (in place
# of Compose's service-name DNS, since Envoy and Fuse are sibling containers
# there but Fuse here is a native process on the host, not a container on
# the same network) is otherwise the only change from that file.
# Linux Docker Engine users: host.docker.internal isn't automatic there —
# add `--add-host=host.docker.internal:host-gateway` to the `docker run`
# below if this doesn't resolve.
# ---------------------------------------------------------------------------
log "Generating Envoy bootstrap config ($ENVOY_BOOTSTRAP)"
cat > "$ENVOY_BOOTSTRAP" <<EOF
node:
  cluster: fuse-proxy
  id: fuse-proxy-1

admin:
  access_log_path: /dev/null
  address:
    socket_address:
      address: 0.0.0.0
      port_value: $ENVOY_ADMIN_PORT

dynamic_resources:
  cds_config:
    resource_api_version: V3
    api_config_source:
      api_type: GRPC
      transport_api_version: V3
      grpc_services:
        - envoy_grpc:
            cluster_name: xds_cluster
      set_node_on_first_message_only: true
  lds_config:
    resource_api_version: V3
    api_config_source:
      api_type: GRPC
      transport_api_version: V3
      grpc_services:
        - envoy_grpc:
            cluster_name: xds_cluster
      set_node_on_first_message_only: true

static_resources:
  clusters:
    - name: xds_cluster
      connect_timeout: 1s
      type: STRICT_DNS
      dns_lookup_family: V4_ONLY
      load_assignment:
        cluster_name: xds_cluster
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address:
                      address: host.docker.internal
                      port_value: 18000
      http2_protocol_options: {}
    - name: als_cluster
      connect_timeout: 1s
      load_assignment:
        cluster_name: als_cluster
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address:
                      address: 0.0.0.0
                      port_value: $ENVOY_ALS_PORT
      http2_protocol_options: {}

layered_runtime:
  layers:
    - name: runtime-0
      rtds_layer:
        rtds_config:
          resource_api_version: V3
          api_config_source:
            transport_api_version: V3
            api_type: GRPC
            grpc_services:
              envoy_grpc:
                cluster_name: xds_cluster
        name: runtime-0
EOF

log "Starting Envoy"
docker run -d --name "$ENVOY_CONTAINER" \
  -p "$ENVOY_DATA_PLANE_PORT:10000" \
  -p "$ENVOY_ADMIN_PORT:19000" \
  -p "$ENVOY_ALS_PORT:18090" \
  -v "$ENVOY_BOOTSTRAP:/etc/envoy/envoy.yaml" \
  "$ENVOY_IMAGE" >/dev/null
CONTAINERS+=("$ENVOY_CONTAINER")
wait_for_tcp localhost "$ENVOY_ADMIN_PORT" envoy

log "Starting Garage"
start_bg garage "$REPO_ROOT/services/tooling/garage" "$BIN_DIR/garage" \
  DRAFT_SERVICE_NETWORK_INTERNAL_HOST=localhost
wait_for_tcp localhost 9301 garage

log "Starting Bench"
start_bg bench "$REPO_ROOT/services/tooling/bench" "$BIN_DIR/bench" \
  DRAFT_SERVICE_NETWORK_INTERNAL_HOST=localhost
wait_for_tcp localhost 9300 bench

log "Starting slack-notify (publishes itself to Garage's catalog on startup)"
start_bg slack-notify "$REPO_ROOT/services/tooling/slack-notify" "$BIN_DIR/slack-notify"
wait_for_tcp localhost 9302 slack-notify

log "Starting catalyst-consume (publishes itself to Garage's catalog on startup)"
start_bg catalyst-consume "$REPO_ROOT/services/tooling/catalyst-consume" "$BIN_DIR/catalyst-consume"
wait_for_tcp localhost 9303 catalyst-consume

log "Starting http-call (publishes itself to Garage's catalog on startup)"
start_bg http-call "$REPO_ROOT/services/tooling/http-call" "$BIN_DIR/http-call"
wait_for_tcp localhost 9304 http-call

log "Starting grpc-call (publishes itself to Garage's catalog on startup)"
start_bg grpc-call "$REPO_ROOT/services/tooling/grpc-call" "$BIN_DIR/grpc-call"
wait_for_tcp localhost 9305 grpc-call

log "Starting catalyst-produce (publishes itself to Garage's catalog on startup)"
start_bg catalyst-produce "$REPO_ROOT/services/tooling/catalyst-produce" "$BIN_DIR/catalyst-produce"
wait_for_tcp localhost 9306 catalyst-produce

cat <<EOF

--------------------------------------------------------------------
Full local Draft cluster is up.

  Garage UI                    http://localhost:9301/
  Bench UI                     http://localhost:9300/
  Blueprint web client          http://localhost:2221/
  Beacon web client             http://localhost:2222/
  slack-notify (RPC only)       localhost:9302
  catalyst-consume (RPC only)   localhost:9303
  http-call (RPC only)          localhost:9304
  grpc-call (RPC only)          localhost:9305
  catalyst-produce (RPC only)   localhost:9306
  crud (examples, RPC only)     localhost:9090
  echo (examples, RPC only)     localhost:9091
  Documentation site (Hugo)     http://localhost:1313/
  Envoy admin                   http://localhost:19000/
  Envoy data plane               localhost:10000  (proxies blueprint/garage/bench/beacon.draft.localhost + Beacon's RPC prefix — see this script's header)
  ClickHouse HTTP (catalyst events + beacon logs/traces/metrics) http://localhost:8123/

Logs: $LOG_DIR/<service>.log
Press Ctrl+C to stop everything (including every container started above).
--------------------------------------------------------------------
EOF

# Stay in the foreground so a Ctrl+C (or this script being killed) triggers
# the cleanup trap and actually tears things down, rather than leaving
# every child process/container orphaned.
wait
