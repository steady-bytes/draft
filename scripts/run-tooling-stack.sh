#!/usr/bin/env bash
#
# Builds and runs a local Draft cluster: the three core services (Blueprint,
# Catalyst, Fuse) plus the E2E-testing tooling services (Bench, Garage,
# slack-notify, catalyst-consume, http-call, grpc-call, catalyst-produce),
# backed by a throwaway local Postgres.
#
# Only Garage has a UI today (services/tooling/garage's Phase 4 — see
# docs/website/content/docs/architecture/garage-plugin-repository.md). Bench's
# own UI is Phase 8 of its implementation plan and doesn't exist yet; Bench is
# still started here since it's a real, working service (webhook + RPCs), just
# not one you can browse to.
#
# Usage:
#   ./scripts/run-tooling-stack.sh          # build, start everything, stay in
#                                            # the foreground until Ctrl+C
#
# On exit (Ctrl+C, or any error after infra is up), every process this script
# started and the Postgres container are torn down.
#
# URLs once running:
#   Garage UI            http://localhost:9301/
#   Blueprint web client  http://localhost:2221/
#   Bench   (RPC/webhook only, no UI)  http://localhost:9300/
#   slack-notify (RPC only)            http://localhost:9302/
#   catalyst-consume (RPC only)        http://localhost:9303/
#   http-call (RPC only)               http://localhost:9304/
#   grpc-call (RPC only)               http://localhost:9305/
#   catalyst-produce (RPC only)        http://localhost:9306/
#
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUN_DIR="$REPO_ROOT/.local-stack"
BIN_DIR="$RUN_DIR/bin"
LOG_DIR="$RUN_DIR/logs"
POSTGRES_CONTAINER="draft-tooling-stack-postgres"
POSTGRES_PORT=5432

mkdir -p "$BIN_DIR" "$LOG_DIR"

PIDS=()
STARTED_POSTGRES=0

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
  if [ "$STARTED_POSTGRES" = "1" ]; then
    docker rm -f "$POSTGRES_CONTAINER" >/dev/null 2>&1 || true
  fi
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
for p in 2221 2220 18000 9300 9301 9302 9303 9304 9305 9306 "$POSTGRES_PORT"; do
  if port_in_use "$p"; then
    die "port $p is already in use — stop whatever's using it (or a previous run of this script that didn't get torn down) before retrying"
  fi
done

command -v docker >/dev/null 2>&1 || die "docker is required (for the Postgres container) but isn't on PATH"
docker info >/dev/null 2>&1 || die "docker daemon isn't running"

# ---------------------------------------------------------------------------
# Postgres — one instance, three databases: bench and garage (this script's
# own managed services) plus draft, matching every services/examples/*
# service's checked-in config.yaml (repositories.postgres.url) — e.g. crud,
# which isn't started by this script but connects to this same local
# Postgres instance whenever it's run by hand, and previously needed its
# role/database created manually after every fresh Postgres container.
# ---------------------------------------------------------------------------
log "Starting Postgres"
docker run -d --name "$POSTGRES_CONTAINER" \
  -p "$POSTGRES_PORT:5432" \
  -e POSTGRES_PASSWORD=postgres \
  postgres:16-alpine >/dev/null
STARTED_POSTGRES=1

log "Waiting for Postgres to accept connections"
for ((i = 0; i < 60; i++)); do
  docker exec "$POSTGRES_CONTAINER" pg_isready -U postgres >/dev/null 2>&1 && break
  sleep 0.5
done
docker exec "$POSTGRES_CONTAINER" pg_isready -U postgres >/dev/null 2>&1 \
  || die "postgres never became ready"

log "Creating bench/garage/draft roles and databases"
docker exec -i "$POSTGRES_CONTAINER" psql -U postgres -v ON_ERROR_STOP=1 <<'SQL' >/dev/null \
  || die "failed to create bench/garage/draft roles/databases"
CREATE ROLE bench LOGIN PASSWORD 'bench';
CREATE DATABASE bench OWNER bench;
CREATE ROLE garage LOGIN PASSWORD 'garage';
CREATE DATABASE garage OWNER garage;
CREATE ROLE draft LOGIN PASSWORD 'draft';
CREATE DATABASE draft OWNER draft;
SQL

# ---------------------------------------------------------------------------
# Build everything up front, so a compile error surfaces immediately instead
# of mid-startup. Each service is its own Go module (this monorepo's
# convention throughout), so each build runs in its own directory.
# ---------------------------------------------------------------------------
log "Building core services"
(cd "$REPO_ROOT/services/core/blueprint" && go build -o "$BIN_DIR/blueprint" .) || die "blueprint build failed"
(cd "$REPO_ROOT/services/core/catalyst" && go build -o "$BIN_DIR/catalyst" .) || die "catalyst build failed"
(cd "$REPO_ROOT/services/core/fuse" && go build -o "$BIN_DIR/fuse" .) || die "fuse build failed"

log "Building tooling services"
(cd "$REPO_ROOT/services/tooling/bench" && go build -o "$BIN_DIR/bench" .) || die "bench build failed"
(cd "$REPO_ROOT/services/tooling/garage" && go build -o "$BIN_DIR/garage" .) || die "garage build failed"
(cd "$REPO_ROOT/services/tooling/slack-notify" && go build -o "$BIN_DIR/slack-notify" .) || die "slack-notify build failed"
(cd "$REPO_ROOT/services/tooling/catalyst-consume" && go build -o "$BIN_DIR/catalyst-consume" .) || die "catalyst-consume build failed"
(cd "$REPO_ROOT/services/tooling/http-call" && go build -o "$BIN_DIR/http-call" .) || die "http-call build failed"
(cd "$REPO_ROOT/services/tooling/grpc-call" && go build -o "$BIN_DIR/grpc-call" .) || die "grpc-call build failed"
(cd "$REPO_ROOT/services/tooling/catalyst-produce" && go build -o "$BIN_DIR/catalyst-produce" .) || die "catalyst-produce build failed"

# ---------------------------------------------------------------------------
# Start order matters: Blueprint first (everything else registers with it),
# then anything else can come up in any order except slack-notify, which
# needs Garage's PluginCatalogService reachable before its own startup
# Effect (a fatal one — see pkg/chassis/effect.go) tries to publish to it.
#
# service.network.internal.host in bench's and garage's checked-in
# config.yaml is host.docker.internal (set for a container-networked
# deployment); overridden to localhost here via chassis's DRAFT_ env-var
# convention (pkg/chassis/config.go: SetEnvPrefix("DRAFT") +
# SetEnvKeyReplacer(".", "_")) since every process here is a plain native
# binary on the same host, not containerized.
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
Draft tooling stack is up.

  Garage UI (catalog)        http://localhost:9301/
  Blueprint web client       http://localhost:2221/
  Bench   (RPC + webhook, no UI yet)   localhost:9300
  slack-notify (RPC only, reference plugin)  localhost:9302
  catalyst-consume (RPC only, reference plugin)  localhost:9303
  http-call (RPC only, reference plugin)  localhost:9304
  grpc-call (RPC only, reference plugin)  localhost:9305
  catalyst-produce (RPC only, reference plugin)  localhost:9306

Logs: $LOG_DIR/<service>.log
Press Ctrl+C to stop everything (including the Postgres container).
--------------------------------------------------------------------
EOF

# Stay in the foreground so a Ctrl+C (or this script being killed) triggers
# the cleanup trap and actually tears things down, rather than leaving
# every child process orphaned.
wait
