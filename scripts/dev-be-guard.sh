#!/usr/bin/env sh
# dev-be-guard.sh — make `make dev-be` single-instance, permanently.
#
# THE BUG THIS KILLS (it cost days): `make dev-be` runs Air with no guard, so a SECOND
# `make dev-be` starts a SECOND Air + a SECOND `./tmp/loomarr-dev`, both binding :8080. The
# newcomer loses the bind ("address already in use") and EXITS — while the stale binary keeps
# serving OLD CODE. Every "my logs didn't appear" / "my fix didn't take" after that is a zombie.
#
# This guard runs before Air. If a loomarr dev server is already up, it does ONE of two things,
# never a blanket kill:
#   - default: refuse to start, print the offending PID + how to reclaim, exit non-zero.
#   - DEV_BE_REPLACE=1: SIGTERM exactly the existing loomarr-dev binary (by its unique path,
#     NOT `pkill air` / `pkill vite`), wait for the port to free, then continue.
#
# It targets the loomarr-dev binary + air by PROCESS NAME (comm), not `pgrep -f 'air@…'`.
#
# ⚠ THE ROOT-CAUSE FIX (this cost days): `make dev-be` runs `go run …/air@v1.67.3`, which COMPILES
# and execs a child whose comm is just `air` — and Air in turn execs `./tmp/loomarr-dev` (comm
# `loomarr-dev`). Neither child carries the `air@v1.67.3` string, so `pgrep -f 'air@v1.67.3'` MISSES
# the actually-running processes. Every "kill by pgrep -f" left the real Air + binary alive, which
# is how zombies serving stale code accumulated across a session. Detect + kill by `comm` instead.

set -eu

PORT="${LOOMARR_DEV_PORT:-8080}"
BIN_MATCH='tmp/loomarr-dev'

# port_holder prints the PID listening on $PORT, or nothing. Tries ss, then lsof, then a
# health probe as a last resort (some minimal containers have neither tool).
port_holder() {
  if command -v ss >/dev/null 2>&1; then
    ss -ltnp "sport = :$PORT" 2>/dev/null | grep -oE 'pid=[0-9]+' | head -1 | cut -d= -f2
  elif command -v lsof >/dev/null 2>&1; then
    lsof -tiTCP:"$PORT" -sTCP:LISTEN 2>/dev/null | head -1
  fi
}

# loomarr_pids prints the PIDs of the actual dev server binary — matched by comm (`loomarr-dev`),
# because the running binary's command name is NOT the `go run …` string pgrep -f would need.
loomarr_pids() {
  ps -e -o pid=,comm= 2>/dev/null | awk '$2=="loomarr-dev"{print $1}'
}

# air_pids prints the PIDs of the Air processes (comm `air`) supervising this repo's dev build.
# Matched by comm for the same reason loomarr_pids is — `pgrep -f air@` misses the exec'd child.
air_pids() {
  ps -e -o pid=,comm= 2>/dev/null | awk '$2=="air"{print $1}'
}

held_by_loomarr() {
  # True when :$PORT is held AND a loomarr-dev binary is running — i.e. it's OUR zombie, safe
  # to reclaim. A port held by something ELSE is never touched; we just report and stop.
  [ -n "$(loomarr_pids)" ] && { curl -sf "http://localhost:$PORT/healthz" >/dev/null 2>&1 || [ -n "$(port_holder)" ]; }
}

if curl -sf "http://localhost:$PORT/healthz" >/dev/null 2>&1 || [ -n "$(port_holder)" ]; then
  PIDS="$(loomarr_pids)"
  HOLDER="$(port_holder || true)"

  if [ "${DEV_BE_REPLACE:-0}" = "1" ]; then
    echo "dev-be: replacing the running loomarr dev server — SIGTERM air + loomarr-dev, then restart." >&2
    # Stop the Air supervisors FIRST (by comm) so they do not respawn the binary we kill next,
    # then the loomarr-dev binaries. By comm, never `pkill -f` (which misses the exec'd children)
    # and never a bare `pkill air` (which would hit an unrelated Air). Escalate to -9 if needed.
    for p in $(air_pids); do kill -TERM "$p" 2>/dev/null || true; done
    sleep 1
    for p in $(loomarr_pids); do kill -TERM "$p" 2>/dev/null || true; done
    sleep 1
    for p in $(air_pids) $(loomarr_pids); do kill -9 "$p" 2>/dev/null || true; done
    # Wait up to 10s for the port to actually free before starting the new instance.
    i=0
    while curl -sf "http://localhost:$PORT/healthz" >/dev/null 2>&1 || [ -n "$(port_holder)" ]; do
      i=$((i + 1))
      [ "$i" -ge 20 ] && { echo "dev-be: port $PORT still held after 10s; not starting a second instance." >&2; exit 1; }
      sleep 0.5
    done
    echo "dev-be: port $PORT free — starting fresh." >&2
    exit 0
  fi

  # Default: refuse, and say EXACTLY what to do. No second instance, no zombie.
  echo "" >&2
  echo "  ✗ dev-be: something is already listening on :$PORT." >&2
  if [ -n "$PIDS" ]; then
    echo "    It is a loomarr dev server (pid ${PIDS%% *})." >&2
    echo "    Reload happens automatically on save — you probably don't need a new one." >&2
    echo "    To force a fresh start:  DEV_BE_REPLACE=1 make dev-be" >&2
  else
    echo "    Holder pid: ${HOLDER:-unknown} (NOT a loomarr dev binary — left untouched)." >&2
    echo "    Free :$PORT or set LOOMARR_DEV_PORT to a different port." >&2
  fi
  echo "" >&2
  exit 1
fi

exit 0
