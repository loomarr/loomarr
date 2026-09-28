#!/usr/bin/env bash
# Watch supervised agent lanes (Orca terminals running an agent in one of this repo's secondary
# worktrees) and print one line when a lane crosses a budget level, finishes a turn, waits on a question
# menu, or shows API trouble. Read-only: it never types into a terminal. The supervisor acts.
#
# Usage: scripts/dev/watch-lanes.sh [--once]      (exits quietly when the orca CLI is absent)
#   WATCH_LANES_INTERVAL=20   seconds between polls (short, so a budget overshoot stays small)
#   WATCH_LANE_WARN=150000    output tokens: warning
#   WATCH_LANE_CUTOFF=190000  output tokens: the brief's cutoff (interrupt the lane and ask for its report)
#   WATCH_LANE_LIMIT=240000   output tokens: hard limit
#   WATCH_LANES_CHECKPOINTS   directory of "<lane>" files, each holding a checkpoint start epoch; a
#                             lane with one is metered over every transcript born since then (its
#                             session plus subagents and restarts). Without one, the lane's newest
#                             Claude session and its subagents are metered.
#
# The meter is output_tokens over assistant messages, counted once per message id: one API message is
# written as several transcript lines (one per content block) repeating the same usage, and summing
# lines overcounted about 3.5x.

set -u

WATCH_SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)"
# shellcheck source=scripts/dev/watch-lib.sh
. "$WATCH_SCRIPT_DIR/watch-lib.sh"

interval="${WATCH_LANES_INTERVAL:-20}"
warn="${WATCH_LANE_WARN:-150000}"
cutoff="${WATCH_LANE_CUTOFF:-190000}"
limit="${WATCH_LANE_LIMIT:-240000}"
claude_projects="${CLAUDE_CONFIG_DIR:-$HOME/.claude}/projects"

# lanes: "handle<TAB>worktree" for each agent terminal in a secondary worktree of this repo.
lanes() {
	local roots primary handle path root
	roots="$(worktree_roots)"
	primary="$(git -C "$WATCH_SCRIPT_DIR" worktree list --porcelain 2>/dev/null | sed -n 's/^worktree //p' | head -1)"
	orca terminal list --json 2>/dev/null |
		jq -r '.result.terminals[]? | select(.agentIdentity != null and .worktreePath != null) | [.handle, .worktreePath] | @tsv' |
		while IFS="$(printf '\t')" read -r handle path; do
			if [ -z "$handle" ] || [ "$path" = "$primary" ]; then
				continue
			fi
			while IFS= read -r root; do
				case $root in
					'') ;;
					*'*') case $path in "${root%\*}"*) printf '%s\t%s\n' "$handle" "$path" && break ;; esac ;;
					*) [ "$path" != "$root" ] && path_under "$path" "$root" && printf '%s\t%s\n' "$handle" "$path" && break ;;
				esac
			done <<EOF
$roots
EOF
		done
}

# transcripts WORKTREE LANE: the transcript files this lane's current checkpoint covers.
transcripts() {
	local dir ckpt newest f
	# Claude Code names a project dir after its path with every non-alphanumeric character as '-'.
	dir="$claude_projects/$(printf '%s' "$1" | sed 's/[^A-Za-z0-9]/-/g')"
	[ -d "$dir" ] || return 0
	ckpt="${WATCH_LANES_CHECKPOINTS:+$WATCH_LANES_CHECKPOINTS/$2}"
	if [ -n "$ckpt" ] && [ -f "$ckpt" ]; then
		ckpt="$(cat "$ckpt")"
		find "$dir" -name '*.jsonl' -type f 2>/dev/null | while IFS= read -r f; do
			# Birth, not mtime: a finished session's exit flush can land after a new checkpoint starts.
			# Where birth is unknown, fall back to mtime.
			b="$(file_birth "$f")"
			[ "$b" -gt 0 ] || b="$(file_mtime "$f")"
			[ "$b" -ge $((ckpt - 5)) ] && printf '%s\n' "$f"
		done
		return 0
	fi
	# ls -t is the portable newest-first; transcript names are UUIDs, so no quoting surprises.
	# shellcheck disable=SC2012
	newest="$(ls -t "$dir"/*.jsonl 2>/dev/null | head -1)"
	[ -n "$newest" ] || return 0
	printf '%s\n' "$newest"
	[ -d "${newest%.jsonl}" ] && find "${newest%.jsonl}" -name '*.jsonl' -type f 2>/dev/null
	return 0
}

# file_tokens FILE: output tokens in one transcript, cached by mtime+size so a quiet lane costs no jq.
file_tokens() {
	local sig cache n
	sig="$(file_mtime "$1"):$(wc -c < "$1" | tr -d ' ')"
	cache="$WATCH_STATE/tokens.$(printf '%s' "$1" | watch_hash)"
	if [ -f "$cache" ] && [ "$(sed -n 1p "$cache")" = "$sig" ]; then
		sed -n 2p "$cache"
		return 0
	fi
	n="$(jq -s '[.[] | select(.type == "assistant" and .message.usage != null)]
		| group_by(.message.id) | map(.[-1].message.usage.output_tokens // 0) | add // 0' "$1" 2>/dev/null)"
	printf '%s\n%s\n' "$sig" "${n:-0}" > "$cache"
	printf '%s\n' "${n:-0}"
}

check_budget() { # lane worktree
	local total=0 f n level=0 first ident
	first=''
	while IFS= read -r f; do
		[ -n "$f" ] || continue
		[ -n "$first" ] || first="$f"
		n="$(file_tokens "$f")"
		total=$((total + n))
	done <<EOF
$(transcripts "$2" "$1")
EOF
	[ "$total" -ge "$warn" ] && level=1
	[ "$total" -ge "$cutoff" ] && level=2
	[ "$total" -ge "$limit" ] && level=3
	# A new checkpoint (or a new session) re-arms every level.
	ident="$(cat "${WATCH_LANES_CHECKPOINTS:-/nonexistent}/$1" 2>/dev/null || printf '%s' "$first")"
	case $level in
		1) watch_alert "budget.$1" "$ident:1" "$1: $total output tokens (warning at $warn)" ;;
		2) watch_alert "budget.$1" "$ident:2" "$1: $total output tokens: CUTOFF ($cutoff) reached; interrupt it and ask for its report" ;;
		3) watch_alert "budget.$1" "$ident:3" "$1: $total output tokens: LIMIT ($limit) exceeded" ;;
	esac
}

check_terminal() { # lane handle
	local tail sig
	tail="$(orca terminal read --terminal "$2" --json 2>/dev/null |
		jq -r '(.result.terminal.tail // []) | if type == "array" then .[-10:] | join(" ") else . end' 2>/dev/null)"
	[ -n "$tail" ] || return 0
	sig="$(printf '%s' "$tail" | watch_hash)"
	if printf '%s' "$tail" | grep -Eq 'attempt 10/10|API Error|Request timed out'; then
		watch_alert "api.$2" "$sig" "$1: API trouble in its terminal"
	fi
	# A lane blocked on its own question menu shows no finished-turn marker and would sit silently.
	if printf '%s' "$tail" | grep -q 'Enter to select'; then
		watch_alert "ask.$2" "$sig" "$1: WAITING ON A QUESTION MENU (answer it)"
		return 0
	fi
	# Finished = a turn-duration marker and no live activity: old markers stay in the scrollback after a
	# restart, so a spinner or "esc to interrupt" means it is still working.
	if printf '%s' "$tail" | grep -Eq '[A-Z][a-z]+ed for [0-9]' &&
		! printf '%s' "$tail" | grep -Eq 'esc to interrupt|still thinking|…'; then
		watch_alert "idle.$2" "$sig" "$1: turn finished, waiting"
	fi
}

poll() {
	local handle path lane
	while IFS="$(printf '\t')" read -r handle path; do
		[ -n "$handle" ] || continue
		lane="$(basename "$path")"
		check_budget "$lane" "$path"
		check_terminal "$lane" "$handle"
	done <<EOF
$(lanes)
EOF
}

command -v orca >/dev/null 2>&1 || {
	echo 'watch-lanes: orca CLI not found; skipping' >&2
	exit 0
}
watch_state_init lanes
if [ "${1:-}" = --once ]; then
	poll
	exit 0
fi
while :; do
	poll
	watch_sleep "$interval"
done
