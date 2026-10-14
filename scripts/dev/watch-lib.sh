#!/usr/bin/env bash
# Shared helpers for the dev watchers (scripts/dev/watch-*.sh). Source this file; do not execute it.
#
# Portability: macOS ships bash 3.2, so nothing here uses associative arrays, `mapfile`, `${v,,}` or
# namerefs. Per-key state (alert dedupe, streak counters, previous samples) lives in small files under
# one state directory instead. GNU/BSD differences (stat, md5sum/md5, date) go through the helpers
# below; `cksum` is the one hash both systems ship.
#
# The watchers are observers: nothing sourced from here signals, kills or edits anything.

WATCH_OS="${WATCH_OS:-$(uname -s)}"

# Resolve Orca's IDE CLI once. On Linux, bare orca can be the GNOME screen reader.
# ORCA_CLI_COMMAND is an executable path/name, not shell code or an argument list.
watch_orca_cli() {
	if [ -n "${ORCA_CLI_COMMAND:-}" ]; then
		printf '%s\n' "$ORCA_CLI_COMMAND"
	elif [ -n "${ORCA_DEV_REPO_ROOT:-}" ]; then
		printf '%s\n' orca-dev
	elif [ "$WATCH_OS" = Linux ]; then
		printf '%s\n' orca-ide
	else
		printf '%s\n' orca
	fi
}

# watch_state_init NAME [persistent]: sets WATCH_STATE to a directory for this watcher's state.
# Persistent state (PR dedupe) survives restarts; other state starts clean for each run.
watch_state_init() {
	if [ -n "${WATCH_STATE_DIR:-}" ]; then
		WATCH_STATE="$WATCH_STATE_DIR/$1"
	elif [ "${2:-}" = persistent ]; then
		WATCH_STATE="${XDG_STATE_HOME:-$HOME/.local/state}/loomarr-watch/$1"
	else
		WATCH_STATE="$(mktemp -d "${TMPDIR:-/tmp}/loomarr-watch-$1.XXXXXX")"
		# shellcheck disable=SC2064 # Expand now: the path is fixed for this run.
		trap "rm -rf '$WATCH_STATE'" EXIT
		trap 'exit 130' INT
		trap 'exit 143' TERM
	fi
	mkdir -p "$WATCH_STATE"
	WATCH_NAME="$1"
}

# watch_key RAW: a file-name-safe key. Check names and paths hold '/', which broke a raw-key marker once.
watch_key() {
	printf '%s' "$1" | tr -c 'A-Za-z0-9._-' '_'
}

watch_say() {
	printf '%s %s: %s\n' "$(date +%H:%M:%S)" "$WATCH_NAME" "$*"
}

# watch_alert KEY IDENTITY MESSAGE: print MESSAGE unless KEY is already alerting with the same IDENTITY.
# The identity is what makes an alert new (the set of orphan PIDs, a PR head); the message may carry
# numbers that change every poll without re-alerting.
watch_alert() {
	local f
	f="$WATCH_STATE/alert.$(watch_key "$1")"
	if [ ! -f "$f" ] || [ "$(cat "$f")" != "$2" ]; then
		printf '%s' "$2" > "$f"
		watch_say "$3"
	fi
}

# watch_clear KEY LABEL: when KEY was alerting, print that LABEL is back to normal and forget it.
watch_clear() {
	local f
	f="$WATCH_STATE/alert.$(watch_key "$1")"
	if [ -f "$f" ]; then
		rm -f "$f"
		watch_say "$2: back to normal"
	fi
}

# watch_streak KEY yes|no: count consecutive polls where a condition held; prints the new count.
watch_streak() {
	local f n=0
	f="$WATCH_STATE/streak.$(watch_key "$1")"
	if [ "$2" = yes ]; then
		[ -f "$f" ] && n="$(cat "$f")"
		n=$((n + 1))
		printf '%s' "$n" > "$f"
	else
		rm -f "$f"
	fi
	printf '%s\n' "$n"
}

# watch_first KEY: succeeds the first time KEY is seen (persisted), fails every time after.
watch_first() {
	local f
	f="$WATCH_STATE/seen.$(watch_key "$1")"
	[ -f "$f" ] && return 1
	: > "$f"
}

# watch_sleep SECONDS: bash runs a trap only after a foreground command returns, so a plain `sleep 60`
# would hold off Ctrl-C (and the state cleanup) for up to a minute.
watch_sleep() {
	sleep "$1" &
	wait $!
}

watch_hash() {
	cksum | cut -d' ' -f1
}

# Float comparison without bc: watch_gt A B succeeds when A > B.
watch_gt() {
	awk -v a="$1" -v b="$2" 'BEGIN { exit !(a + 0 > b + 0) }'
}

watch_cores() {
	if [ -n "${WATCH_CORES:-}" ]; then
		printf '%s\n' "$WATCH_CORES"
	elif [ "$WATCH_OS" = Darwin ]; then
		sysctl -n hw.ncpu
	else
		getconf _NPROCESSORS_ONLN 2>/dev/null || nproc
	fi
}

# file_mtime / file_birth: epoch seconds. GNU stat -c, BSD stat -f. Birth is 0 when unknown.
file_mtime() {
	stat -c %Y "$1" 2>/dev/null || stat -f %m "$1"
}

file_birth() {
	local b
	b="$(stat -c %W "$1" 2>/dev/null)" || b="$(stat -f %B "$1" 2>/dev/null)" || b=0
	case $b in '' | *[!0-9]*) b=0 ;; esac
	printf '%s\n' "$b"
}

# watch_repo_root: the checkout this script lives in, whichever worktree that is.
watch_repo_root() {
	git -C "${WATCH_SCRIPT_DIR:-.}" rev-parse --show-toplevel 2>/dev/null
}

# list_cwds: one "pid<TAB>comm<TAB>cwd" line per process this user can inspect.
# Linux reads WATCH_PROC (default /proc; tests point it at a fake tree), where a cwd removed beneath a
# running process reads as "<path> (deleted)". macOS asks lsof, which reports the old path as-is.
#
# Linux reads every cwd in one `find` pass (a readlink fork per process took a second on a busy box) and
# leaves comm as "-"; list_orphans reads comm only for the few matches.
list_cwds() {
	local proc="${WATCH_PROC:-/proc}" d
	if [ "$WATCH_OS" = Darwin ]; then
		lsof -w -a -d cwd -Fpcn 2>/dev/null | awk '
			/^p/ { pid = substr($0, 2) }
			/^c/ { comm = substr($0, 2) }
			/^n/ { printf "%s\t%s\t%s\n", pid, comm, substr($0, 2) }'
		return 0
	fi
	if find / -maxdepth 0 -printf '' 2>/dev/null; then
		find "$proc"/ -mindepth 2 -maxdepth 2 -name cwd -type l -printf '%h\t-\t%l\n' 2>/dev/null |
			awk -F '\t' -v OFS='\t' '{ sub(/.*\//, "", $1); if ($1 ~ /^[0-9]+$/) print }' | sort -n
		return 0
	fi
	# BSD find has no -printf (the test harness runs this branch on macOS against a fake tree).
	for d in "$proc"/[0-9]*; do
		[ -L "$d/cwd" ] && printf '%s\t-\t%s\n' "${d##*/}" "$(readlink "$d/cwd")"
	done | sort -n
	return 0
}

# cwd_is_gone CWD: the directory was deleted beneath the process (Linux marks it; macOS: it no longer exists).
cwd_is_gone() {
	case $1 in *' (deleted)') return 0 ;; esac
	[ ! -d "$1" ]
}

# path_under PATH ROOT: PATH is ROOT or inside it.
path_under() {
	case $1 in "$2" | "$2"/*) return 0 ;; esac
	return 1
}

# worktree_roots: lines naming where this repo's worktrees live. A trailing '*' is a name prefix
# (make agent-worktree puts siblings at <primary>-<topic>); a plain line is a directory.
worktree_roots() {
	local primary line parent
	if [ -n "${WATCH_WORKTREE_ROOTS:-}" ]; then
		printf '%s\n' "$WATCH_WORKTREE_ROOTS" | tr ':' '\n'
		return 0
	fi
	primary="$(git -C "$WATCH_SCRIPT_DIR" worktree list --porcelain 2>/dev/null | sed -n 's/^worktree //p' | head -1)"
	[ -n "$primary" ] || return 0
	printf '%s\n%s-*\n' "$primary" "$primary"
	git -C "$WATCH_SCRIPT_DIR" worktree list --porcelain 2>/dev/null | sed -n 's/^worktree //p' | sed 1d |
		while IFS= read -r line; do
			parent="$(dirname "$line")"
			# A worktree beside the primary is covered by the prefix; one in a tool's workspace dir
			# (Orca's) makes that dir a root, so worktrees removed from it later still match.
			[ "$parent" = "$(dirname "$primary")" ] || printf '%s\n' "$parent"
		done | sort -u
}

# is_orphan CWD ROOTS: the cwd is in a trashed worktree, or in a deleted directory under a worktree root.
is_orphan() {
	local cwd="$1" path root
	case $cwd in */.orca-worktree-trash/* | */.Trash/*) return 0 ;; esac
	cwd_is_gone "$cwd" || return 1
	path="${cwd% (deleted)}"
	while IFS= read -r root; do
		[ -n "$root" ] || continue
		case $root in
			*'*') case $path in "${root%\*}"*) return 0 ;; esac ;;
			*) path_under "$path" "$root" && return 0 ;;
		esac
	done <<EOF
$2
EOF
	return 1
}

# list_orphans: "pid<TAB>comm<TAB>cwd" for every process whose cwd is in a deleted or trashed worktree.
list_orphans() {
	local roots pid comm cwd
	roots="$(worktree_roots)"
	list_cwds | while IFS="$(printf '\t')" read -r pid comm cwd; do
		[ -n "$pid" ] || continue
		# Linux marks a removed cwd, so only marked or trashed cwds need a closer look there.
		if [ "$WATCH_OS" != Darwin ]; then
			case $cwd in *' (deleted)' | */.orca-worktree-trash/* | */.Trash/*) ;; *) continue ;; esac
		fi
		if is_orphan "$cwd" "$roots"; then
			[ "$comm" = - ] && comm="$(cat "${WATCH_PROC:-/proc}/$pid/comm" 2>/dev/null || echo '?')"
			printf '%s\t%s\t%s\n' "$pid" "$comm" "$cwd"
		fi
	done
}
