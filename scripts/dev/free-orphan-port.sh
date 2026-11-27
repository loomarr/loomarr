#!/usr/bin/env bash
# Free PORT when its listener runs in a deleted or trashed worktree, so a dev server starting here
# does not lose its port to a server left behind by a worktree that no longer exists.
#
# Usage: scripts/dev/free-orphan-port.sh PORT
#
# Only orphans are stopped (is_orphan in watch-lib.sh): a listener in a live worktree, or anywhere
# else, is left alone and the caller's own port check reports it. Stops by PID: TERM, then KILL after
# WATCH_REAP_GRACE seconds (default 5). Prints one line per process it stopped; silent otherwise.

set -u

WATCH_SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)"
# shellcheck source=scripts/dev/watch-lib.sh
. "$WATCH_SCRIPT_DIR/watch-lib.sh"

port="${1:-}"
case $port in '' | *[!0-9]*) echo 'usage: free-orphan-port.sh PORT' >&2; exit 2 ;; esac

# listeners: the PIDs listening on port.
listeners() {
	if command -v lsof >/dev/null 2>&1; then
		lsof -nP -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null
	elif command -v ss >/dev/null 2>&1; then
		ss -Hltnp "sport = :$port" 2>/dev/null | grep -oE 'pid=[0-9]+' | cut -d= -f2
	fi | sort -u
}

# raw_cwd PID: the process's cwd as the kernel reports it, deleted or not.
raw_cwd() {
	if [ -e "${WATCH_PROC:-/proc}/$1/cwd" ] || [ -L "${WATCH_PROC:-/proc}/$1/cwd" ]; then
		readlink "${WATCH_PROC:-/proc}/$1/cwd" 2>/dev/null
	else
		lsof -a -p "$1" -d cwd -Fn 2>/dev/null | sed -n 's/^n//p' | head -1
	fi
}

roots="$(worktree_roots)"
stopped=''
for pid in $(listeners); do
	cwd="$(raw_cwd "$pid")"
	if [ -z "$cwd" ] || ! is_orphan "$cwd" "$roots"; then
		continue
	fi
	comm="$(ps -o comm= -p "$pid" 2>/dev/null | sed 's#.*/##')"
	kill -TERM "$pid" 2>/dev/null || continue
	echo "dev: stopped ${comm:-?} (pid $pid) left listening on :$port by a removed worktree ($cwd)" >&2
	stopped="$stopped $pid"
done
[ -n "$stopped" ] || exit 0

waited=0
while [ "$waited" -lt "${WATCH_REAP_GRACE:-5}" ]; do
	alive=''
	for pid in $stopped; do kill -0 "$pid" 2>/dev/null && alive="$alive $pid"; done
	[ -n "$alive" ] || exit 0
	sleep 1
	waited=$((waited + 1))
done
# shellcheck disable=SC2086 # One word per pid.
kill -KILL $alive 2>/dev/null
exit 0
