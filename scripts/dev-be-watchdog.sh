#!/usr/bin/env sh
# dev-be-watchdog.sh — make Air's "alive but not rebuilding" wedge self-healing.
#
# THE BUG THIS KILLS (it cost this project a WEEK, then recurred after the config fixes):
# Air (github.com/air-verse/air) can end up ALIVE but with its poll loop no longer firing —
# the process is `Sl`, the port answers, but the running ./tmp/loomarr-dev binary is FROZEN at
# a build from before your latest saves. Every "my fix didn't take" / "the logs don't match the
# code" after that is you testing a stale binary. `.air.toml` already carries the two config
# mitigations (`stop_on_error = false` so a mid-refactor compile error doesn't stop the watcher;
# `poll = true` so btrfs/atomic-save inotify gaps don't silently drop events) — and they reduce
# how OFTEN this happens, but a session in 2026-08 proved it can still wedge. Config alone cannot
# fix "the watcher stopped watching"; only an out-of-band check can.
#
# So this watchdog runs BESIDE Air (started by `make dev-be`) and asks one question on a timer:
# is the running binary OLDER than the newest watched Go/Rust source? A healthy Air makes that briefly
# true after every save (it is mid-build) and then false again within ~1s. A WEDGED Air makes it
# true forever. Hysteresis (STALE_STRIKES consecutive stale checks) distinguishes the two: we only
# act after the binary has stayed stale far longer than Air's worst-case poll_interval + delay +
# build time, so a healthy Air always self-heals before we intervene.
#
# ⚠ RECOVERY NUDGES AIR — it never spawns a competing binary. Spawning `go build && exec` here
# would recreate the exact two-binaries-fighting-:8080 zombie that scripts/dev-be-guard.sh exists
# to prevent. Instead we escalate gently:
#   1. `touch` a watched source file — pokes Air's mtime poll into re-firing. Cheapest;
#      no restart, no lost connections. Fixes the common "poll loop stalled" case.
#   2. if still stale a cycle later, SIGTERM the stale loomarr-dev by comm — Air's own restart
#      logic (send_interrupt + rebuild) then rebuilds from current source. Same mechanism a
#      normal reload uses, just triggered by us.
# Both target the binary by comm (`loomarr-dev`), never `pkill -f air@` (which misses the exec'd
# child) and never a blanket `pkill air` (which would hit an unrelated Air) — the same discipline
# dev-be-guard.sh established.
#
# It exits when Air exits, so `make dev-be` leaves no orphan watchdog behind.

set -u

REPO_ROOT="${LOOMARR_REPO_ROOT:-$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd -P)}"
REPO_ROOT="$(CDPATH='' cd -- "$REPO_ROOT" && pwd -P)"
# shellcheck source=scripts/dev-processes.sh
. "$REPO_ROOT/scripts/dev-processes.sh"
BIN="$REPO_ROOT/tmp/loomarr-dev"
POLL_SECONDS="${DEV_BE_WATCHDOG_POLL:-10}"
# 3 strikes × 10s = 30s of continuous staleness before we act. Air's poll_interval (0.5s) + delay
# (0.4s) + a cold incremental Go/Rust build is comfortably under that, so a healthy rebuild never
# reaches strike 1's successor. Raise DEV_BE_WATCHDOG_STRIKES on a slow box if it ever false-fires.
STALE_STRIKES="${DEV_BE_WATCHDOG_STRIKES:-3}"

# air_alive: is an Air supervising this repo still running? (comm match, per dev-be-guard.sh.)
air_alive() {
  [ -n "$(repo_pids_by_comm air "$REPO_ROOT")" ]
}

# loomarr_pids: PIDs of the running dev binary, by comm (NOT `pgrep -f`, which misses the exec'd child).
loomarr_pids() {
  repo_pids_by_comm loomarr-dev "$REPO_ROOT"
}

# newest_src_after_bin: prints a nonempty string iff at least one watched Go/Rust input is newer
# than the binary. `-newer $BIN` is the staleness test; we only need existence, so -quit
# on the first hit keeps it O(1)-ish on a big tree.
newest_src_after_bin() {
  [ -f "$BIN" ] || { echo "nobin"; return; }
  find "$REPO_ROOT/cmd" "$REPO_ROOT/internal" "$REPO_ROOT/rust" \
    "$REPO_ROOT/Cargo.toml" "$REPO_ROOT/Cargo.lock" "$REPO_ROOT/rust-toolchain.toml" \
    -type d \( -name testdata -o -name fixtures \) -prune -o \
    -type f \( -name '*.go' -o -name '*.rs' -o -name '*.toml' -o -name 'Cargo.lock' \) \
    ! -name '*_test.go' -newer "$BIN" -print -quit 2>/dev/null
}

# newest_source_file: a single watched path to `touch` as the gentle nudge. Any watched file works —
# touching it updates its mtime, which Air's poll compares against the binary and treats as a change.
newest_source_file() {
  find "$REPO_ROOT/cmd" "$REPO_ROOT/internal" "$REPO_ROOT/rust" \
    -type d \( -name testdata -o -name fixtures \) -prune -o \
    -type f \( -name '*.go' -o -name '*.rs' \) ! -name '*_test.go' -print 2>/dev/null | head -1
}

echo "dev-be-watchdog: watching for a wedged Air (binary $BIN; ${POLL_SECONDS}s poll, ${STALE_STRIKES} strikes)." >&2

# The watchdog starts immediately before the pinned `go run air@...`. A cold tool build can take a few
# seconds, so wait for this worktree's Air instead of interpreting startup as an already-dead watcher.
startup_wait=0
until air_alive; do
  startup_wait=$((startup_wait + 1))
  if [ "$startup_wait" -ge 30 ]; then
    echo "dev-be-watchdog: Air did not start within 30s — exiting." >&2
    exit 1
  fi
  sleep 1
done

strikes=0
nudged=0
while air_alive; do
  sleep "$POLL_SECONDS"
  air_alive || break   # Air exited during the sleep — stop cleanly, no orphan.

  stale="$(newest_src_after_bin)"
  if [ -z "$stale" ] || [ "$stale" = "nobin" ]; then
    # Not stale (or no binary yet — Air's first build hasn't finished). Reset.
    strikes=0
    nudged=0
    continue
  fi

  strikes=$((strikes + 1))
  [ "$strikes" -lt "$STALE_STRIKES" ] && continue

  # Sustained staleness: Air should have rebuilt by now and hasn't.
  if [ "$nudged" -eq 0 ]; then
    poke="$(newest_source_file)"
    if [ -n "$poke" ]; then
      echo "dev-be-watchdog: binary stale for $((strikes * POLL_SECONDS))s — nudging Air (touch $poke)." >&2
      touch "$poke" 2>/dev/null || true
      nudged=1
      strikes=0   # give the nudge a full cycle to take effect before escalating
      continue
    fi
  fi

  # Nudge didn't take — Air's poll loop is truly dead. Restart the binary via Air's own path:
  # SIGTERM the stale loomarr-dev (by comm); Air rebuilds from current source on the child's exit.
  pids="$(loomarr_pids)"
  if [ -n "$pids" ]; then
    echo "dev-be-watchdog: nudge failed, Air still not rebuilding — SIGTERM stale loomarr-dev ($pids) so Air rebuilds." >&2
    for p in $pids; do kill -TERM "$p" 2>/dev/null || true; done
  fi
  strikes=0
  nudged=0
done

echo "dev-be-watchdog: Air is gone — exiting." >&2
