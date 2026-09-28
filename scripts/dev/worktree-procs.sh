#!/usr/bin/env bash
# List (and, with --kill, stop) every process whose cwd is inside a worktree: the retirement step that
# keeps a removed lane from leaving Air, vite, storybook or a browser running against a deleted tree.
#
# Usage: scripts/dev/worktree-procs.sh WORKTREE [--kill]
#        scripts/dev/worktree-procs.sh --orphans [--kill]   every process in a deleted or trashed worktree
# make agent-reap passes ORPHANS=1 for --orphans and APPLY=1 for --kill (the agent-gc convention).
#
# Output: one "pid<TAB>command<TAB>cwd" line per process. --kill sends TERM, waits up to
# WATCH_REAP_GRACE seconds (default 5), then KILLs survivors, by PID only: `pkill -f` matches its own
# shell. This script and its ancestors are never included, so it is safe to run from inside the worktree.

set -u

WATCH_SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)"
# shellcheck source=scripts/dev/watch-lib.sh
. "$WATCH_SCRIPT_DIR/watch-lib.sh"

target="${1:-}"
[ "${ORPHANS:-0}" = 1 ] && target=--orphans
kill_them=0
if [ "${2:-}" = --kill ] || [ "${APPLY:-0}" = 1 ]; then
	kill_them=1
fi
if [ -z "$target" ] || [ "$target" = --kill ]; then
	echo 'usage: worktree-procs.sh WORKTREE|--orphans [--kill]  (make agent-reap WORKTREE=<path>|ORPHANS=1 [APPLY=1])' >&2
	exit 2
fi

# ancestors: this process and every parent up to init, which must never be listed or signalled.
ancestors() {
	local pid=$$
	while [ -n "$pid" ] && [ "$pid" -gt 1 ] 2>/dev/null; do
		printf '%s\n' "$pid"
		pid="$(ps -o ppid= -p "$pid" 2>/dev/null | tr -d ' ')"
	done
}

if [ "$target" != --orphans ]; then
	root="$(CDPATH='' cd -- "$target" 2>/dev/null && pwd -P)" || root="${target%/}"
fi
# Leave the worktree: every subshell and pipe this script starts inherits its cwd and would match.
cd / || exit 1

matches() {
	local pid comm cwd
	if [ "$target" = --orphans ]; then
		list_orphans
		return 0
	fi
	list_cwds | while IFS="$(printf '\t')" read -r pid comm cwd; do
		path_under "${cwd% (deleted)}" "$root" || continue
		[ "$comm" = - ] && comm="$(cat "${WATCH_PROC:-/proc}/$pid/comm" 2>/dev/null || echo '?')"
		printf '%s\t%s\t%s\n' "$pid" "$comm" "$cwd"
	done
}

self="$(ancestors | tr '\n' ' ')"
# A function, because bash 3.2 (macOS) misparses a case pattern's ')' inside $(...).
is_self() {
	case " $self " in *" $1 "*) return 0 ;; esac
	return 1
}
found="$(matches | while IFS="$(printf '\t')" read -r pid rest; do
	is_self "$pid" && continue
	printf '%s\t%s\n' "$pid" "$rest"
done)"
[ -n "$found" ] || exit 0
printf '%s\n' "$found"
[ "$kill_them" = 1 ] || exit 0

pids="$(printf '%s\n' "$found" | cut -f1 | tr '\n' ' ')"
# shellcheck disable=SC2086 # One word per pid.
kill -TERM $pids 2>/dev/null
waited=0
while [ "$waited" -lt "${WATCH_REAP_GRACE:-5}" ]; do
	alive=''
	for pid in $pids; do kill -0 "$pid" 2>/dev/null && alive="$alive $pid"; done
	[ -n "$alive" ] || break
	sleep 1
	waited=$((waited + 1))
done
alive=''
for pid in $pids; do kill -0 "$pid" 2>/dev/null && alive="$alive $pid"; done
if [ -n "$alive" ]; then
	# shellcheck disable=SC2086
	kill -KILL $alive 2>/dev/null
	echo "worktree-procs: KILLed after ${WATCH_REAP_GRACE:-5}s:$alive" >&2
fi
echo "worktree-procs: stopped $pids" >&2
