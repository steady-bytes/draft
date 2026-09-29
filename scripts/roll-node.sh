#!/usr/bin/env bash
#
# Restart Blueprint's extra raft nodes (node_2 … node_5) one at a time, under the hot-reload stack
# started by scripts/run-local-watch.sh, so each rebuilds and picks up the current web-client bundle.
#
# Why this exists: only node_1's watcher runs `dx build` and watches web-client/. Nodes 2-5 embed
# whatever web-client/target held when *they* were last built, and only rebuild when main.go,
# key_value/ or service_discovery/ change. After a web-client change node_1 serves the new UI while
# the other four still serve the old one — and Fuse load-balances `blueprint.draft.localhost` across
# all five, so most page loads land on a stale node. Touching a shared source file would rebuild all
# four at once and drop raft quorum (3 of 5) for the length of a build; this rolls them one by one.
#
# Usage:
#   scripts/roll-node.sh node_2 [node_4 …]   restart just these, in the order given
#   scripts/roll-node.sh all                 node_2 node_3 node_4 node_5
#   scripts/roll-node.sh -n all              dry run: check and print, change nothing
#
# For each node it: stops that node's watchexec (SIGTERM, which stops the node with it), waits for
# the port to close, starts the watcher again exactly as run-local-watch.sh's `start_watched` does
# (same watch paths, same DRAFT_CONFIG, appending to .local-stack/logs/<node>.log), waits for the
# rebuilt node to listen, lets raft settle, and checks all five HTTP ports before the next node.
# It stops at the first problem instead of pressing on, so a failure never takes out a second node.
#
# The node table below mirrors BLUEPRINT_NODE_* in run-local-watch.sh; keep them in step.
#
# Environment: SETTLE (seconds to let raft settle after a node returns, default 10),
#              BUILD_TIMEOUT (seconds to wait for a rebuild and restart, default 180).

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUN_DIR="$REPO_ROOT/.local-stack"
BIN_DIR="$RUN_DIR/bin"
LOG_DIR="$RUN_DIR/logs"
SVC="$REPO_ROOT/services/core/blueprint"
BUNDLE="$SVC/web-client/target/dx/blueprint-pwa/release/web/public"

NODE_IDS=(node_2 node_3 node_4 node_5)
NODE_HTTP_PORTS=(2231 2232 2233 2234)
NODE1_HTTP_PORT=2221

SETTLE="${SETTLE:-10}"
BUILD_TIMEOUT="${BUILD_TIMEOUT:-180}"
DRY_RUN=0

log()  { printf '\033[1;34m==>\033[0m %s\n' "$1"; }
warn() { printf '\033[1;33mwarn:\033[0m %s\n' "$1" >&2; }
die()  { printf '\033[1;31mERROR:\033[0m %s\n' "$1" >&2; exit 1; }

usage() { sed -n '2,/^set -uo/p' "${BASH_SOURCE[0]}" | sed '$d' | sed 's/^# \{0,1\}//' ; exit "${1:-0}"; }

listening() { lsof -nP -iTCP:"$1" -sTCP:LISTEN >/dev/null 2>&1; }

http_port_of() {
  local i
  for i in "${!NODE_IDS[@]}"; do
    [ "${NODE_IDS[$i]}" = "$1" ] && { echo "${NODE_HTTP_PORTS[$i]}"; return 0; }
  done
  return 1
}

# The pid of a node's watchexec: the one whose command builds and execs its binary. Exactly one
# must exist, or something is off and we do nothing.
watcher_pid() {
  local pids
  pids="$(ps -axo pid,command | grep '[w]atchexec' | grep "blueprint-$1 \\." | awk '{print $1}')"
  [ "$(echo "$pids" | wc -w | tr -d ' ')" = "1" ] || return 1
  echo "$pids"
}

all_up() {
  local p
  for p in "$NODE1_HTTP_PORT" "${NODE_HTTP_PORTS[@]}"; do
    listening "$p" || { warn "nothing is listening on $p"; return 1; }
  done
}

roll() {
  local node="$1" port pid
  port="$(http_port_of "$node")" || die "unknown node '$node' (expected one of: ${NODE_IDS[*]})"
  local config="$RUN_DIR/blueprint-$node.yaml"
  [ -f "$config" ] || die "$config does not exist — is the stack running?"
  pid="$(watcher_pid "$node")" || die "expected exactly one running watchexec for $node; is run-local-watch.sh up?"

  if [ "$DRY_RUN" = 1 ]; then
    log "[dry run] would restart $node: stop watchexec pid $pid, wait for :$port to close, relaunch with DRAFT_CONFIG=$config, wait up to ${BUILD_TIMEOUT}s for :$port, settle ${SETTLE}s, check all five ports"
    return 0
  fi

  log "Restarting Blueprint $node (watchexec pid $pid, http :$port)"
  kill "$pid" || die "could not signal watchexec pid $pid"
  local i
  for i in $(seq 1 30); do listening "$port" || break; sleep 1; done
  listening "$port" && die "$node is still listening on :$port 30s after SIGTERM; not relaunching"

  echo "=== $(date) restarted by scripts/roll-node.sh to pick up the current web-client bundle ===" >> "$LOG_DIR/$node.log"
  (
    cd "$SVC" || exit 1
    exec env DRAFT_CONFIG="$config" nohup watchexec -r --stop-signal SIGTERM \
      -w "$SVC/main.go" -w "$SVC/go.mod" -w "$SVC/go.sum" -w "$SVC/key_value" -w "$SVC/service_discovery" \
      -- "go build -o $BIN_DIR/blueprint-$node . && exec $BIN_DIR/blueprint-$node"
  ) >> "$LOG_DIR/$node.log" 2>&1 < /dev/null &
  disown

  for i in $(seq 1 "$BUILD_TIMEOUT"); do listening "$port" && break; sleep 1; done
  if ! listening "$port"; then
    tail -5 "$LOG_DIR/$node.log" | cut -c1-200 >&2
    die "$node did not come back on :$port within ${BUILD_TIMEOUT}s — see $LOG_DIR/$node.log. Nothing else was touched."
  fi
  log "$node is listening on :$port; letting raft settle for ${SETTLE}s"
  sleep "$SETTLE"
  all_up || die "after restarting $node, not every node is up. Stopping here so no second node is taken down."

  local ui
  ui="$(curl -s -m 5 "http://127.0.0.1:$port/" | grep -o 'cdn.jsdelivr.net/npm/daisyui' | head -1)"
  if [ -n "$ui" ]; then
    warn "$node is up but still serves the old (daisyUI) UI: node_1's dx build may not have run yet ($BUNDLE)"
  else
    log "$node serves the current UI"
  fi
}

# --- arguments ----------------------------------------------------------------------------------
NODES=()
while [ $# -gt 0 ]; do
  case "$1" in
    -n|--dry-run) DRY_RUN=1 ;;
    -h|--help)    usage 0 ;;
    all)          NODES+=("${NODE_IDS[@]}") ;;
    node_*)       NODES+=("$1") ;;
    *)            usage 1 ;;
  esac
  shift
done
[ "${#NODES[@]}" -gt 0 ] || usage 1

command -v watchexec >/dev/null 2>&1 || die "watchexec is required (brew install watchexec)"
[ -d "$BUNDLE" ] || die "$BUNDLE does not exist: build node_1's web-client first (its watcher does), or a rebuilt node would embed nothing"
all_up || die "the cluster is not fully up; refusing to take a node down while another is missing"

for node in "${NODES[@]}"; do
  roll "$node"
done

if [ "$DRY_RUN" = 1 ]; then
  log "dry run complete; nothing was changed"
else
  log "done: ${NODES[*]} restarted one at a time"
fi
