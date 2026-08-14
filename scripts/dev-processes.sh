#!/usr/bin/env sh
# Shared process-ownership helpers for dev-be. Source this file; do not execute it.

canonical_dir() {
	CDPATH='' cd -- "$1" 2>/dev/null && pwd -P
}

process_cwd() {
	pid="$1"
	raw_cwd=
	if [ -e "/proc/$pid/cwd" ]; then
		raw_cwd="$(readlink "/proc/$pid/cwd" 2>/dev/null || true)"
	elif command -v lsof >/dev/null 2>&1; then
		raw_cwd="$(lsof -a -p "$pid" -d cwd -Fn 2>/dev/null | sed -n 's/^n//p' | head -1)"
	fi
	if [ -n "$raw_cwd" ]; then
		canonical_dir "$raw_cwd"
	fi
}

repo_pids_by_comm() {
	wanted_comm="$1"
	wanted_root="$(canonical_dir "$2")"
	ps -e -o pid=,comm= 2>/dev/null | awk -v comm="$wanted_comm" '
		{
			pid=$1
			$1=""
			sub(/^[[:space:]]+/, "")
			name=$0
			sub(/^.*\//, "", name)
			if (name==comm) print pid
		}' |
	while IFS= read -r pid; do
		[ -n "$pid" ] || continue
		cwd="$(process_cwd "$pid")"
		[ "$cwd" = "$wanted_root" ] && printf '%s\n' "$pid"
	done
}

pid_in_list() {
	wanted="$1"
	shift
	for pid in "$@"; do
		[ "$pid" = "$wanted" ] && return 0
	done
	return 1
}
