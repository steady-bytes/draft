#!/usr/bin/env bash
#
# Standalone shutdown for whichever local Draft stack is currently running --
# scripts/run-local.sh, run-local-watch.sh, or run-tooling-stack.sh. Each of
# those already tears itself down cleanly on Ctrl+C (their own `cleanup()`
# trap), but that only works from the terminal that's still attached to that
# script's shell. This script kills everything by process pattern and port
# instead, so it works regardless of which launcher started the stack, or
# whether the shell that started it is even still around (e.g. a background
# session, a closed terminal, a crashed launcher script).
#
# Deliberately avoids bash 4+ builtins (mapfile/readarray, associative
# arrays): macOS ships bash 3.2 as both /bin/bash and what `env bash`
# resolves to with no newer version installed, same constraint the other
# scripts/*.sh already write around.
#
# Usage:
#   ./scripts/stop-local.sh
#
# Safe to run any time, including when nothing is up -- every step is a
# no-op if its target isn't found.
#
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="$REPO_ROOT/.local-stack/bin"

# The full set of host ports any of the three launcher scripts binds a native
# process to. Used as a fallback sweep after the process-pattern kill below,
# to catch anything that didn't match by name (e.g. a `go build` step still
# holding a port from a prior partial start, or a service launched some other
# way). Docker-published ports (Postgres/ClickHouse/Envoy) are handled
# separately, by container name, not listed here.
PORTS="2220 2221 2222 18000 \
1111 2231 2232 2233 2234 1112 1113 1114 1115 \
9090 9091 \
9300 9301 9302 9303 9304 9305 9306 \
1313"

log()  { printf '\033[1;34m==>\033[0m %s\n' "$1"; }
warn() { printf '\033[1;33m!!\033[0m %s\n' "$1" >&2; }

# term_then_kill sends SIGTERM to every pid in $1 (a whitespace-separated
# string, not an array -- see the bash 3.2 note above), waits briefly, then
# SIGKILLs whatever's still alive. Same two-phase shutdown run-local*.sh's
# own cleanup() uses, so a service gets a chance to close its raft/db
# connections cleanly before being forced.
term_then_kill() {
  local pids="$1"
  [ -z "$pids" ] && return 0
  # shellcheck disable=SC2086
  kill $pids 2>/dev/null || true
  sleep 1
  for pid in $pids; do
    kill -0 "$pid" 2>/dev/null && kill -9 "$pid" 2>/dev/null || true
  done
}

count() { [ -z "$1" ] && echo 0 || echo "$1" | wc -w | tr -d ' '; }

# ---------------------------------------------------------------------------
# 1. watchexec supervisors (run-local-watch.sh) -- killing these first lets
#    watchexec's own --stop-signal SIGTERM handling cascade to whatever
#    binary it's currently supervising, before step 2's direct sweep would
#    otherwise race it.
# ---------------------------------------------------------------------------
watchexec_pids="$(pgrep -f "watchexec.*$REPO_ROOT" 2>/dev/null || true)"
if [ -n "$watchexec_pids" ]; then
  log "Stopping $(count "$watchexec_pids") watchexec supervisor(s)"
  term_then_kill "$watchexec_pids"
else
  log "No watchexec supervisors found"
fi

# ---------------------------------------------------------------------------
# 2. Every built binary this stack runs, whichever launcher started it --
#    run-local.sh backgrounds these directly; run-local-watch.sh execs into
#    them from inside watchexec (same $BIN_DIR/<name> path either way, so
#    this also mops up anything step 1's signal forwarding missed).
# ---------------------------------------------------------------------------
bin_pids="$(pgrep -f "$BIN_DIR/" 2>/dev/null || true)"
if [ -n "$bin_pids" ]; then
  log "Stopping $(count "$bin_pids") service process(es) under $BIN_DIR"
  term_then_kill "$bin_pids"
else
  log "No service processes found under $BIN_DIR"
fi

# ---------------------------------------------------------------------------
# 3. Docs site (Hugo) -- not under $BIN_DIR, and not watchexec-wrapped.
#    Filtered to this repo's own docs server (port 1313, or cwd under
#    docs/website) so an unrelated hugo instance elsewhere on the machine
#    isn't caught by a bare "hugo server" match.
# ---------------------------------------------------------------------------
hugo_pids=""
for pid in $(pgrep -f "hugo server" 2>/dev/null || true); do
  if ps -p "$pid" -o command= 2>/dev/null | grep -q "port 1313" \
     || lsof -p "$pid" -a -d cwd 2>/dev/null | grep -q "$REPO_ROOT/docs/website"; then
    hugo_pids="$hugo_pids $pid"
  fi
done
if [ -n "$hugo_pids" ]; then
  log "Stopping docs site (hugo)"
  term_then_kill "$hugo_pids"
else
  log "No docs site (hugo) process found"
fi

# ---------------------------------------------------------------------------
# 4. Docker containers -- every launcher names its containers "draft-*"
#    (draft-local-postgres/clickhouse/envoy, draft-tooling-stack-postgres,
#    etc). Matching the prefix instead of an exact list means this doesn't
#    need updating if a launcher script adds or renames one.
# ---------------------------------------------------------------------------
if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  container_names="$(docker ps -a -f 'name=^draft-' --format '{{.Names}}' 2>/dev/null || true)"
  if [ -n "$container_names" ]; then
    log "Removing docker container(s): $(echo "$container_names" | paste -sd, -)"
    # shellcheck disable=SC2046
    docker rm -f $(docker ps -aq -f 'name=^draft-') >/dev/null 2>&1 || true
  else
    log "No draft-* docker containers found"
  fi
else
  warn "docker not available/running -- skipping container cleanup"
fi

# ---------------------------------------------------------------------------
# 5. Fallback port sweep -- catches anything steps 1-3 missed by name (e.g. a
#    stray `go build` still holding a port from an interrupted start).
# ---------------------------------------------------------------------------
swept=0
for port in $PORTS; do
  pids="$(lsof -ti ":$port" -sTCP:LISTEN 2>/dev/null || true)"
  [ -z "$pids" ] && continue
  swept=$((swept + 1))
  warn "port $port still held after process sweep -- killing it directly:"
  # shellcheck disable=SC2086
  ps -p $pids -o pid=,command= 2>/dev/null | sed 's/^/    /' >&2
  # shellcheck disable=SC2086
  kill $pids 2>/dev/null || true
  sleep 0.5
  # shellcheck disable=SC2086
  kill -9 $pids 2>/dev/null || true
done
[ "$swept" -eq 0 ] && log "No stray processes found on any known port"

log "Done. (.local-stack/logs and .local-stack/bin are left in place -- delete .local-stack/ yourself if you want a totally clean slate.)"
