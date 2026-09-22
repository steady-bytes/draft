#!/usr/bin/env bash
#
# Builds and runs a full local Draft cluster: every core service (Blueprint,
# Catalyst, Fuse, Beacon), every example service Bench's own workflows/
# actually exercise as a real backend (examples/crud for crud-e2e.yaml,
# examples/echo for fuse-proxy-e2e.yaml and fuse-proxy-e2e-10x.yaml), and
# every tooling service (Bench, Foundry, slack-notify, catalyst-consume,
# http-call, grpc-call, catalyst-produce), plus Draft's own documentation
# site (docs/website, a Hugo-modules site -- distinct from the
# steady-bytes.com marketing site living outside this repo), and every
# infra dependency the cluster needs to actually work end to end — Postgres
# (Bench/Foundry/crud) and ClickHouse (Catalyst's optional event store and
# Beacon's log/trace/metric store). Fuse itself is the reverse proxy
# (fuse.proxy_backend: native, services/core/fuse/config.yaml) -- no
# separate Envoy process, and no xDS control-plane server for one to pull
# config from. Every workflow under services/tooling/bench/workflows/
# (Bench's own seed directory) passes against this stack EXCEPT
# fuse-proxy-e2e.yaml and fuse-proxy-e2e-10x.yaml, which are deliberately
# Envoy-specific fixtures (they register a route with a literal
# host.docker.internal endpoint and exercise Envoy's own xDS-pushed data
# plane by design, per their own doc comments) -- run those against
# services/tooling/bench/workflows/fuse-proxy-e2e.yaml's own instructions
# with Envoy started separately, not against this script. The other
# examples/* services (auth, consumer, consumer2, producer, producer2,
# file_host) aren't referenced by any workflow there, so they're
# deliberately left out.
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
# Proxying through Fuse's native data plane (port 10000) to these processes
# works because every service's checked-in config.yaml sets route.host to
# "localhost" (this script, this repo's checked-in configs, and Fuse's own
# native backend are all plain host processes now -- no Docker-to-host
# hostname to bridge, unlike the old Envoy-in-Docker setup this replaced).
# Verified live under this script's hot-reloading sibling
# (run-local-watch.sh, identical services/configs): blueprint.draft.localhost,
# foundry.draft.localhost, bench.draft.localhost, and beacon.draft.localhost
# (plus Beacon's /core.observability. Connect-RPC prefix) all proxy through
# port 10000 successfully.
#
# URLs once running:
#   Foundry UI                          http://localhost:9301/
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
#   Fuse data plane (native reverse proxy; proxies blueprint/foundry/bench/beacon.draft.localhost + Beacon's RPC prefix) http://localhost:10000/
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
# 10000 is Fuse's own native-backend data-plane listener (fuse.listener.port,
# services/core/fuse/config.yaml) -- a plain host process, not a separate
# Envoy container, now that Fuse is the reverse proxy directly.
for p in 2221 2220 18000 2222 4317 9090 9091 9300 9301 9302 9303 9304 9305 9306 \
         10000 "$DOCS_PORT" \
         "$POSTGRES_PORT" "$CLICKHOUSE_NATIVE_PORT" "$CLICKHOUSE_HTTP_PORT"; do
  if port_in_use "$p"; then
    die "port $p is already in use — stop whatever's using it (or a previous run of this script, or run-tooling-stack.sh, that didn't get torn down) before retrying"
  fi
done

command -v docker >/dev/null 2>&1 || die "docker is required (for Postgres/ClickHouse) but isn't on PATH"
docker info >/dev/null 2>&1 || die "docker daemon isn't running"
command -v hugo >/dev/null 2>&1 || die "hugo is required (for the docs site — brew install hugo) but isn't on PATH"

# ---------------------------------------------------------------------------
# Postgres — one instance, two databases (bench, foundry), matching each
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

log "Creating bench/foundry/crud roles and databases"
docker exec -i "$POSTGRES_CONTAINER" psql -U postgres -v ON_ERROR_STOP=1 <<'SQL' >/dev/null \
  || die "failed to create bench/foundry/crud roles/databases"
CREATE ROLE bench LOGIN PASSWORD 'bench';
CREATE DATABASE bench OWNER bench;
CREATE ROLE foundry LOGIN PASSWORD 'foundry';
CREATE DATABASE foundry OWNER foundry;
-- examples/crud's checked-in config.yaml (repositories.postgres.url) points at
-- role/database "draft" specifically, not "crud" -- matching that exactly
-- rather than overriding it via DRAFT_ env vars like bench/foundry's
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
(cd "$REPO_ROOT/services/tooling/foundry" && go build -o "$BIN_DIR/foundry" .) || die "foundry build failed"
(cd "$REPO_ROOT/services/tooling/slack-notify" && go build -o "$BIN_DIR/slack-notify" .) || die "slack-notify build failed"
(cd "$REPO_ROOT/services/tooling/catalyst-consume" && go build -o "$BIN_DIR/catalyst-consume" .) || die "catalyst-consume build failed"
(cd "$REPO_ROOT/services/tooling/http-call" && go build -o "$BIN_DIR/http-call" .) || die "http-call build failed"
(cd "$REPO_ROOT/services/tooling/grpc-call" && go build -o "$BIN_DIR/grpc-call" .) || die "grpc-call build failed"
(cd "$REPO_ROOT/services/tooling/catalyst-produce" && go build -o "$BIN_DIR/catalyst-produce" .) || die "catalyst-produce build failed"

# ---------------------------------------------------------------------------
# Start order: Blueprint first (everything else registers with it), then
# Catalyst (now that ClickHouse is up for it to connect to), then Fuse (its
# own native backend terminates the data plane directly -- see this script's
# header -- so nothing further needs to come up before the tooling services
# can reach it), then the tooling services — slack-notify last since it
# needs Foundry's PluginCatalogService reachable before its own startup
# Effect (a fatal one — see pkg/chassis/effect.go) tries to publish to it.
#
# Every checked-in config.yaml in this repo (bench's, foundry's, and every
# other service's) already points route.host/internal.host at "localhost"
# -- these are plain host processes, and Fuse's own native backend is too,
# so there's no Docker-to-host hostname to bridge the way the old
# Envoy-in-Docker setup needed (host.docker.internal, resolvable only from
# inside a container). No DRAFT_ env-var override needed for this anymore.
# ---------------------------------------------------------------------------
log "Starting Blueprint"
start_bg blueprint "$REPO_ROOT/services/core/blueprint" "$BIN_DIR/blueprint"
wait_for_tcp localhost 2221 blueprint

log "Starting Catalyst"
start_bg catalyst "$REPO_ROOT/services/core/catalyst" "$BIN_DIR/catalyst"
wait_for_tcp localhost 2220 catalyst

log "Starting Fuse"
start_bg fuse "$REPO_ROOT/services/core/fuse" "$BIN_DIR/fuse"
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
# fuse-proxy-e2e-10x workflows -- though per this script's header, those two
# are Envoy-specific fixtures that won't pass against this script anymore;
# echo itself is still a normal example service other workflows may use.
log "Starting echo (examples)"
start_bg echo "$REPO_ROOT/services/examples/echo" "$BIN_DIR/echo"
wait_for_tcp localhost 9091 echo

log "Starting Foundry"
start_bg foundry "$REPO_ROOT/services/tooling/foundry" "$BIN_DIR/foundry"
wait_for_tcp localhost 9301 foundry

log "Starting Bench"
start_bg bench "$REPO_ROOT/services/tooling/bench" "$BIN_DIR/bench"
wait_for_tcp localhost 9300 bench

log "Starting slack-notify (publishes itself to Foundry's catalog on startup)"
start_bg slack-notify "$REPO_ROOT/services/tooling/slack-notify" "$BIN_DIR/slack-notify"
wait_for_tcp localhost 9302 slack-notify

log "Starting catalyst-consume (publishes itself to Foundry's catalog on startup)"
start_bg catalyst-consume "$REPO_ROOT/services/tooling/catalyst-consume" "$BIN_DIR/catalyst-consume"
wait_for_tcp localhost 9303 catalyst-consume

log "Starting http-call (publishes itself to Foundry's catalog on startup)"
start_bg http-call "$REPO_ROOT/services/tooling/http-call" "$BIN_DIR/http-call"
wait_for_tcp localhost 9304 http-call

log "Starting grpc-call (publishes itself to Foundry's catalog on startup)"
start_bg grpc-call "$REPO_ROOT/services/tooling/grpc-call" "$BIN_DIR/grpc-call"
wait_for_tcp localhost 9305 grpc-call

log "Starting catalyst-produce (publishes itself to Foundry's catalog on startup)"
start_bg catalyst-produce "$REPO_ROOT/services/tooling/catalyst-produce" "$BIN_DIR/catalyst-produce"
wait_for_tcp localhost 9306 catalyst-produce

cat <<EOF

--------------------------------------------------------------------
Full local Draft cluster is up.

  Foundry UI                    http://localhost:9301/
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
  Fuse data plane (native reverse proxy)  localhost:10000  (proxies blueprint/foundry/bench/beacon.draft.localhost + Beacon's RPC prefix — see this script's header)
  ClickHouse HTTP (catalyst events + beacon logs/traces/metrics) http://localhost:8123/

Logs: $LOG_DIR/<service>.log
Press Ctrl+C to stop everything (including every container started above).
--------------------------------------------------------------------
EOF

# Stay in the foreground so a Ctrl+C (or this script being killed) triggers
# the cleanup trap and actually tears things down, rather than leaving
# every child process/container orphaned.
wait
