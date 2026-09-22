#!/usr/bin/env bash
#
# PROTOTYPE — hot-reloading sibling of scripts/run-local.sh. Same cluster,
# same infra (Postgres, ClickHouse), same service set and startup order, but
# every native Go process runs under `watchexec -r` instead of a
# one-shot `go build && run`: editing a service's source rebuilds and
# restarts just that service, in place, without touching anything else.
# Blueprint and Beacon additionally rebuild their Dioxus web-client
# (`dx build --release`) before rebuilding the Go binary that embeds it,
# since editing Rust/web-client source alone wouldn't otherwise be noticed.
#
# Also runs Draft's own documentation site (docs/website, Hugo) alongside
# everything else -- independent of the cluster, with its own built-in
# live-reload, so it needs no watchexec wrapper.
#
# Requires `watchexec` (`brew install watchexec`), `hugo` (`brew install
# hugo`), and, for Blueprint/Beacon, `dx` (dioxus-cli) on PATH.
#
# Usage:
#   ./scripts/run-local-watch.sh    # build, start everything, watch for
#                                    # changes, stay in the foreground
#                                    # until Ctrl+C
#
# Known rough edges (this is a prototype, not a replacement for
# run-local.sh yet):
#   - A build failure doesn't stop the script the way run-local.sh's
#     upfront build phase does -- wait_for_tcp will just time out after 30s
#     and die pointing at that service's log, which will have the compiler
#     error in it.
#   - dx build --release takes ~10s even with no source changes (Dioxus's
#     wasm release optimization pass), so a Blueprint/Beacon UI edit has a
#     slower feedback loop than a pure Go service does.
#   - Editing a config.yaml also triggers a rebuild+restart (not just a
#     restart) since watchexec doesn't distinguish -- harmless (go build is
#     a fast no-op when .go files haven't changed) but not instant either.
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

# Extra Blueprint raft nodes -- node_1 (the existing single instance) bootstraps the cluster on
# its usual port 2221 / raft port 1111; these four join it as raft voters (not separate
# bootstrap clusters of their own), for exercising real multi-node consensus locally rather than
# always running Blueprint as a cluster of one. See the "Extra Blueprint nodes" section below for
# the join sequencing and docs/architecture for the raft join RPC this relies on
# (RaftService.Join, must be called against whichever node is currently leader).
BLUEPRINT_NODE_IDS=(node_2 node_3 node_4 node_5)
BLUEPRINT_NODE_HTTP_PORTS=(2231 2232 2233 2234)
BLUEPRINT_NODE_RAFT_PORTS=(1112 1113 1114 1115)

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

# free_process_port kills whatever's listening on a plain host process port
# so a stale previous run of this script (or run-local.sh) doesn't block a
# fresh start. Only used for the native Go/hugo ports this script owns --
# never for a docker-published port (see free_docker_container below):
# on Docker Desktop, the process lsof reports for a published port is
# usually Docker's own shared networking helper, not something specific to
# one container, and killing it breaks Docker's networking wholesale rather
# than just freeing this one port.
free_process_port() {
  local port="$1"
  local pids
  pids="$(lsof -ti ":$port" -sTCP:LISTEN 2>/dev/null)"
  [ -z "$pids" ] && return 0

  warn "port $port is already in use -- killing it to free the port:"
  # shellcheck disable=SC2086
  ps -p $pids -o pid=,command= 2>/dev/null | sed 's/^/    /' >&2
  # shellcheck disable=SC2086
  kill $pids 2>/dev/null
  sleep 1

  pids="$(lsof -ti ":$port" -sTCP:LISTEN 2>/dev/null)"
  if [ -n "$pids" ]; then
    # shellcheck disable=SC2086
    kill -9 $pids 2>/dev/null
    sleep 1
  fi

  port_in_use "$port" && die "port $port is still in use after attempting to kill its owner -- free it manually and retry"
}

# free_docker_container removes a stale container left over from a previous
# run of this script that didn't get torn down cleanly (see cleanup()'s
# trap) -- the safe way to free a docker-published port. Deliberately never
# falls back to killing whatever process lsof reports for the port itself;
# see free_process_port's comment for why that's unsafe here.
free_docker_container() {
  local name="$1"
  docker ps -aq -f "name=^${name}\$" 2>/dev/null | grep -q . || return 0
  warn "found a stale '$name' container from a previous run -- removing it"
  docker rm -f "$name" >/dev/null 2>&1 || true
}

# write_blueprint_node_config generates a standalone config.yaml for an extra (non-bootstrap)
# Blueprint raft node -- everything node_1's own config.yaml has, but with its own bind port,
# raft identity, and a badger/raft data directory under $RUN_DIR (absolute, so it can't collide
# with node_1's tmp/ regardless of working directory). bootstrap is deliberately false: this node
# stays a lone, un-clustered raft server until join_blueprint_node calls RaftService.Join against
# the real leader, the same join path a production multi-node deployment would use, not a second
# bootstrap of its own (which would create a second, separate single-node cluster instead of
# joining the first).
write_blueprint_node_config() {
  local path="$1" bind_port="$2" node_id="$3" raft_port="$4" data_dir="$5"
  mkdir -p "$data_dir"
  cat > "$path" <<EOF
service:
  name: blueprint
  domain: core
  entrypoint: http://localhost:$bind_port

  logging:
    level: debug

  network:
    bind_port: $bind_port
    internal:
      host: localhost
      port: $bind_port
    # route.host: localhost, not host.docker.internal -- Fuse's native proxy backend runs on
    # the host, not in Docker, and can't resolve host.docker.internal at all (confirmed live,
    # see docs/architecture/fuse-native-proxy-implementation-plan.md's Phase 9 notes). Only the
    # envoy backend (a separate Docker container) needed that hostname; if a cluster runs Fuse
    # with fuse.proxy_backend: envoy, this needs to go back to host.docker.internal.
    route:
      host: localhost
      port: $bind_port

telemetry:
  enabled: true
  level: debug

badger:
  path: $data_dir/

raft:
  node-id: $node_id
  address: localhost
  port: $raft_port
  bootstrap: false
EOF
}

# join_blueprint_node calls the bootstrap node's RaftService.Join (over plain JSON, same as any
# other Connect-RPC call in this repo) to add node_id/raft_addr as a voter. Must be called against
# whichever node is currently leader -- node_1 always is at this point, since it just bootstrapped
# alone and nothing has triggered an election since. Retries briefly since the target node's own
# raft transport (not just its HTTP port, already confirmed listening by wait_for_tcp before this
# runs) needs a moment to be ready to accept the leader's connection.
join_blueprint_node() {
  local node_id="$1" raft_addr="$2" tries=10
  for ((i = 0; i < tries; i++)); do
    if curl -sf -X POST http://localhost:2221/core.consensus.raft.v1.RaftService/Join \
         -H "Content-Type: application/json" \
         -d "{\"nodeId\":\"$node_id\",\"raftAddress\":\"$raft_addr\"}" >/dev/null 2>&1; then
      log "$node_id joined the raft cluster (raft address $raft_addr)"
      return 0
    fi
    sleep 1
  done
  warn "$node_id failed to join the raft cluster after retrying -- check $LOG_DIR/blueprint.log and $LOG_DIR/$node_id.log"
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

# start_watched <name> <dir> <build_and_run_cmd> <-w path>... [-- env=val ...]
#
# Runs <build_and_run_cmd> (a shell command that should end with `exec` into
# the long-running binary, so watchexec's restart signal reaches the real
# process rather than an intermediate build step or shell) under
# `watchexec -r`. Every path to watch must be passed explicitly via -w --
# there is no default "watch the whole service dir": a recursive OS-level
# watch registers descriptors for every file under the watched root *before*
# --ignore filters anything out, and this repo's web-client/target dirs
# (multiple GB, 15-20k files each, from Dioxus/cargo release builds) are
# large enough to destabilize the native watcher outright if ever included
# -- confirmed live: the whole cluster restart-looped every ~1-2s against a
# constant stream of "Native fs watcher error" once web-client/target was
# anywhere in the recursively-watched tree, even with an --ignore rule
# targeting it. Same reasoning for Blueprint's tmp/ (badger) and node_1/
# (raft log) dirs -- smaller, but rewritten continuously at runtime, so
# still a live-loop risk, not just noise. Env assignments come after a
# literal `--`.
start_watched() {
  local name="$1" dir="$2" cmd="$3"
  shift 3
  local watches=()
  while [ "${1:-}" != "--" ] && [ $# -gt 0 ]; do
    if [ "$1" = "-w" ]; then
      shift
      # watchexec 2.7.0 fails outright ("No such file or directory") on any
      # relative -w path except a bare "." -- confirmed live: `-w key_value`
      # and even `-w ./key_value` both errored while `-w .` and an absolute
      # path both worked. Resolving to absolute here keeps call sites
      # readable (relative to each service's own dir) without hitting that.
      if [ "$1" = "." ]; then
        watches+=("-w" "$dir")
      else
        watches+=("-w" "$dir/$1")
      fi
      shift
    else
      watches+=("$1")
      shift
    fi
  done
  [ "${1:-}" = "--" ] && shift
  (
    cd "$dir" || exit 1
    # watchexec joins its COMMAND args into one string and runs it through
    # $SHELL -c itself (see --shell in `watchexec --help`) -- wrapping in an
    # explicit `sh -c` here would double-wrap and break on this machine's
    # $SHELL (zsh) choking on the outer, still-unparsed parens/&&. Passing
    # $cmd as a single already-complete shell command string is exactly what
    # watchexec's default wrapping expects.
    exec env "$@" watchexec -r --stop-signal SIGTERM "${watches[@]}" -- "$cmd"
  ) >"$LOG_DIR/$name.log" 2>&1 &
  local pid=$!
  PIDS+=("$pid")
  log "$name watching for changes (pid $pid), logs: $LOG_DIR/$name.log"
}

command -v watchexec >/dev/null 2>&1 || die "watchexec is required (brew install watchexec) but isn't on PATH"

# ---------------------------------------------------------------------------
# Preflight
# ---------------------------------------------------------------------------
for name in "$POSTGRES_CONTAINER" "$CLICKHOUSE_CONTAINER"; do
  free_docker_container "$name"
done

# 10000 is Fuse's own native-backend data-plane listener (fuse.listener.port,
# services/core/fuse/config.yaml) -- a plain host process now, not a Docker
# container, since Fuse is the reverse proxy directly and there's no Envoy
# sitting in front of it to own that port. Freed the same way every other
# service's port is (kill whatever's stale there), not the "die if anything's
# on it" treatment below, which is reserved for ports owned by a container
# this script itself doesn't manage the lifecycle of.
for p in 2221 2220 18000 2222 4317 9090 9091 9300 9301 9302 9303 9304 9305 9306 9307 \
         10000 1111 "${BLUEPRINT_NODE_HTTP_PORTS[@]}" "${BLUEPRINT_NODE_RAFT_PORTS[@]}" \
         "$DOCS_PORT"; do
  free_process_port "$p"
done

for p in "$POSTGRES_PORT" "$CLICKHOUSE_NATIVE_PORT" "$CLICKHOUSE_HTTP_PORT"; do
  if port_in_use "$p"; then
    die "port $p is already in use by something other than this script's own containers — stop it before retrying"
  fi
done

command -v docker >/dev/null 2>&1 || die "docker is required (for Postgres/ClickHouse) but isn't on PATH"
docker info >/dev/null 2>&1 || die "docker daemon isn't running"
command -v hugo >/dev/null 2>&1 || die "hugo is required (for the docs site — brew install hugo) but isn't on PATH"

# ---------------------------------------------------------------------------
# Postgres / ClickHouse — identical to run-local.sh, see that script's
# comments for the full rationale.
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
CREATE ROLE draft LOGIN PASSWORD 'draft';
CREATE DATABASE draft OWNER draft;
SQL

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
# Documentation site (docs/website) -- independent of the rest of the
# cluster, and hugo server already live-reloads on content changes, so
# unlike the Go services this needs no watchexec wrapper. See run-local.sh
# for the full rationale.
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
# Core services. Blueprint and Beacon each rebuild their web-client first
# (dx build --release, matching how main.go's //go:embed expects
# web-client/target/dx/<name>-pwa/release/web/public to already exist) --
# `tmp/**` (Blueprint's own badger data, rewritten continuously at runtime)
# and `web-client/target/**` (dx build's own output) are excluded from the
# watch, or each service would restart-loop on its own writes.
# ---------------------------------------------------------------------------
log "Starting Blueprint (watched: source + web-client)"
start_watched blueprint "$REPO_ROOT/services/core/blueprint" \
  "(cd web-client && dx build --release) && go build -o $BIN_DIR/blueprint . && exec $BIN_DIR/blueprint" \
  -w main.go -w go.mod -w go.sum -w config.yaml -w key_value -w service_discovery \
  -w web-client/src -w web-client/public -w web-client/index.html \
  -w web-client/Dioxus.toml -w web-client/Cargo.toml -w web-client/Cargo.lock \
  -w web-client/input.css -w web-client/tailwind.config.js
wait_for_tcp localhost 2221 blueprint

# ---------------------------------------------------------------------------
# Extra Blueprint raft nodes -- node_1 above already bootstrapped and is the
# cluster's sole (and, at this point, only possible) leader. Each of these
# starts as its own lone, un-clustered raft server (bootstrap: false, so it
# never self-forms a second cluster) and then explicitly joins node_1's via
# RaftService.Join, the same way a real multi-node deployment would add a
# voter. Watched like any other Go service for hot-reload, but skips the
# dx build step entirely -- they embed whatever web-client/target already has
# on disk (shared with node_1's own build; only node_1 needs to watch those
# paths to regenerate it) and only need their own main.go/key_value/
# service_discovery source watched to pick up a Go-level change.
#
# Placed immediately after node_1, before Catalyst/Fuse -- not after Fuse,
# where this used to live. node_1 now self-registers with its own
# ServiceDiscoveryService under the shared "blueprint" name (main.go's
# WithRunner calling c.Register()), and that registration writes to the KV
# store, which requires this node to actually be raft leader
# (key_value/controller.go's Set: "if c.raft.State() != raft.Leader"). A lone
# node_1, with no peers reachable yet, can never win an election by itself --
# its persisted raft configuration already lists all 5 voters from every
# prior run, so it needs an actual quorum (3 of 5), not just itself. Left
# after Fuse, node_1's own Register() call (chassis's shared, fixed 5x5s=25s
# retry budget -- not something this script controls) always exhausted and
# panicked before any of these nodes got a chance to join and grant it a
# leader. Confirmed live: this exact failure, cascading into Catalyst's own
# Register() call failing right after ("failed to connect to blueprint")
# once node_1 had already crashed.
#
# This does reintroduce the *other* race this block's old placement was
# designed to avoid -- Fuse now starts only after this whole block finishes,
# later than node_1's own "core-blueprint-ui" self-registration
# (registerBlueprintUIRoute). That call's retry budget was bumped from 10s to
# 60s specifically to absorb this, so it comfortably outlasts however long
# joining four more nodes and electing a leader actually takes.
#
# With Fuse's route storage now merging every same-name registration into
# one load-balanced Envoy cluster (see the Fuse load-balancing work) rather
# than the last one clobbering the rest, all five nodes' "core-blueprint-ui"
# registrations landing here are the intended, expected outcome -- not the
# rough edge this comment used to warn about.
for i in "${!BLUEPRINT_NODE_IDS[@]}"; do
  node_id="${BLUEPRINT_NODE_IDS[$i]}"
  http_port="${BLUEPRINT_NODE_HTTP_PORTS[$i]}"
  raft_port="${BLUEPRINT_NODE_RAFT_PORTS[$i]}"
  node_config="$RUN_DIR/blueprint-$node_id.yaml"
  node_data_dir="$RUN_DIR/blueprint-$node_id-data"

  write_blueprint_node_config "$node_config" "$http_port" "$node_id" "$raft_port" "$node_data_dir"

  log "Starting Blueprint $node_id (watched, joins node_1's raft cluster)"
  start_watched "$node_id" "$REPO_ROOT/services/core/blueprint" \
    "go build -o $BIN_DIR/blueprint-$node_id . && exec $BIN_DIR/blueprint-$node_id" \
    -w main.go -w go.mod -w go.sum -w key_value -w service_discovery \
    -- DRAFT_CONFIG="$node_config"
  wait_for_tcp localhost "$http_port" "$node_id"
  wait_for_tcp localhost "$raft_port" "$node_id"
done

for i in "${!BLUEPRINT_NODE_IDS[@]}"; do
  join_blueprint_node "${BLUEPRINT_NODE_IDS[$i]}" "localhost:${BLUEPRINT_NODE_RAFT_PORTS[$i]}"
done

log "Starting Catalyst (watched)"
start_watched catalyst "$REPO_ROOT/services/core/catalyst" \
  "go build -o $BIN_DIR/catalyst . && exec $BIN_DIR/catalyst" \
  -w .
wait_for_tcp localhost 2220 catalyst

log "Starting Fuse (watched)"
start_watched fuse "$REPO_ROOT/services/core/fuse" \
  "go build -o $BIN_DIR/fuse . && exec $BIN_DIR/fuse" \
  -w .
wait_for_tcp localhost 18000 fuse

log "Starting Beacon (watched: source + web-client)"
start_watched beacon "$REPO_ROOT/services/core/beacon" \
  "(cd web-client && dx build --release) && go build -o $BIN_DIR/beacon . && exec $BIN_DIR/beacon" \
  -w main.go -w go.mod -w go.sum -w config.yaml -w ingest -w query -w store \
  -w web-client/src -w web-client/public -w web-client/index.html \
  -w web-client/Dioxus.toml -w web-client/Cargo.toml -w web-client/Cargo.lock
wait_for_tcp localhost 2222 beacon

log "Starting crud (examples, watched)"
start_watched crud "$REPO_ROOT/services/examples/crud" \
  "go build -o $BIN_DIR/crud . && exec $BIN_DIR/crud" \
  -w .
wait_for_tcp localhost 9090 crud

log "Starting echo (examples, watched)"
start_watched echo "$REPO_ROOT/services/examples/echo" \
  "go build -o $BIN_DIR/echo . && exec $BIN_DIR/echo" \
  -w .
wait_for_tcp localhost 9091 echo

log "Starting Foundry (watched)"
start_watched foundry "$REPO_ROOT/services/tooling/foundry" \
  "go build -o $BIN_DIR/foundry . && exec $BIN_DIR/foundry" \
  -w . \
  -- DRAFT_SERVICE_NETWORK_INTERNAL_HOST=localhost
wait_for_tcp localhost 9301 foundry

log "Starting Bench (watched)"
start_watched bench "$REPO_ROOT/services/tooling/bench" \
  "go build -o $BIN_DIR/bench . && exec $BIN_DIR/bench" \
  -w . \
  -- DRAFT_SERVICE_NETWORK_INTERNAL_HOST=localhost
wait_for_tcp localhost 9300 bench

log "Starting slack-notify (watched, publishes itself to Foundry's catalog on startup)"
start_watched slack-notify "$REPO_ROOT/services/tooling/slack-notify" \
  "go build -o $BIN_DIR/slack-notify . && exec $BIN_DIR/slack-notify" \
  -w .
wait_for_tcp localhost 9302 slack-notify

log "Starting catalyst-consume (watched, publishes itself to Foundry's catalog on startup)"
start_watched catalyst-consume "$REPO_ROOT/services/tooling/catalyst-consume" \
  "go build -o $BIN_DIR/catalyst-consume . && exec $BIN_DIR/catalyst-consume" \
  -w .
wait_for_tcp localhost 9303 catalyst-consume

log "Starting http-call (watched, publishes itself to Foundry's catalog on startup)"
start_watched http-call "$REPO_ROOT/services/tooling/http-call" \
  "go build -o $BIN_DIR/http-call . && exec $BIN_DIR/http-call" \
  -w .
wait_for_tcp localhost 9304 http-call

log "Starting grpc-call (watched, publishes itself to Foundry's catalog on startup)"
start_watched grpc-call "$REPO_ROOT/services/tooling/grpc-call" \
  "go build -o $BIN_DIR/grpc-call . && exec $BIN_DIR/grpc-call" \
  -w .
wait_for_tcp localhost 9305 grpc-call

log "Starting catalyst-produce (watched, publishes itself to Foundry's catalog on startup)"
start_watched catalyst-produce "$REPO_ROOT/services/tooling/catalyst-produce" \
  "go build -o $BIN_DIR/catalyst-produce . && exec $BIN_DIR/catalyst-produce" \
  -w .
wait_for_tcp localhost 9306 catalyst-produce

log "Starting Lineman (watched: source + web-client, publishes itself to Foundry's catalog on startup)"
start_watched lineman "$REPO_ROOT/services/tooling/lineman" \
  "(cd web-client && dx build --release) && go build -o $BIN_DIR/lineman . && exec $BIN_DIR/lineman" \
  -w main.go -w go.mod -w go.sum -w config.yaml \
  -w agent.go -w catalyst.go -w controller.go -w foundry.go -w loop.go \
  -w objective.go -w rpc.go -w scheduler.go -w store.go -w task.go -w watch.go \
  -w web-client/src -w web-client/public -w web-client/index.html \
  -w web-client/Dioxus.toml -w web-client/Cargo.toml -w web-client/Cargo.lock
wait_for_tcp localhost 9307 lineman

cat <<EOF

--------------------------------------------------------------------
Full local Draft cluster is up, with hot rebuild+restart on source changes.

  Foundry UI                    http://localhost:9301/
  Bench UI                     http://localhost:9300/
  Blueprint web client          http://localhost:2221/ (node_1, raft leader/bootstrap)
  Blueprint raft nodes          node_2..node_5 on :2231-:2234 (RPC only, joined node_1's cluster)
  Beacon web client             http://localhost:2222/
  slack-notify (RPC only)       localhost:9302
  catalyst-consume (RPC only)   localhost:9303
  http-call (RPC only)          localhost:9304
  grpc-call (RPC only)          localhost:9305
  catalyst-produce (RPC only)   localhost:9306
  Lineman UI                    http://localhost:9307/
  crud (examples, RPC only)     localhost:9090
  echo (examples, RPC only)     localhost:9091
  Documentation site (Hugo)     http://localhost:1313/
  Fuse data plane (native reverse proxy)  localhost:10000
  ClickHouse HTTP (catalyst events + beacon logs/traces/metrics) http://localhost:8123/

Edit any service's source (or Blueprint's/Beacon's web-client) and it will
rebuild and restart on its own -- tail its log at $LOG_DIR/<service>.log to
watch it happen.

Logs: $LOG_DIR/<service>.log
Press Ctrl+C to stop everything (including every container started above).
--------------------------------------------------------------------
EOF

wait
