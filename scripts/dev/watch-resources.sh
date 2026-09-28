#!/usr/bin/env bash
# Watch the dev machine and print one line when a condition crosses its threshold, and one when it clears.
# Read-only: it reports; the developer or the supervising agent decides what to stop.
#
# Usage: scripts/dev/watch-resources.sh [--once]
# Every threshold scales to the machine and is an env override (docs/contributing/dev-watchers.md):
#   WATCH_INTERVAL=60        seconds between polls
#   WATCH_LOAD_RATIO=0.7     1-min load above this share of the cores ...
#   WATCH_LOAD_MINUTES=3     ... for this long
#   WATCH_MEM_MIN_PCT=10     available memory below this share of the total
#   WATCH_TMP_DIR=/tmp       watched only when it is RAM-backed (tmpfs), unless WATCH_TMP_ANY_FS=1
#   WATCH_TMP_PCT=60         ... above this percent full
#   WATCH_WORKTREE_ROOTS     colon-separated dirs whose deleted subdirs count as worktrees (default: derived)
#   WATCH_HOG_CORES=1.5      one process above this many cores ...
#   WATCH_HOG_MINUTES=5      ... for this long
#   WATCH_HOG_IGNORE         space-separated command globs never reported as hogs (desktop apps)
#   WATCH_GPU_PCT=90         NVIDIA utilisation above this ...
#   WATCH_GPU_MINUTES=5      ... for this long
#   WATCH_LOCKS              space-separated lock files an idle `sleep` must not hold
#   WATCH_LOCK_POLLS=2       polls in a row a `sleep` holds a lock before it is reported

set -u

WATCH_SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)"
# shellcheck source=scripts/dev/watch-lib.sh
. "$WATCH_SCRIPT_DIR/watch-lib.sh"

interval="${WATCH_INTERVAL:-60}"
proc="${WATCH_PROC:-/proc}"
hog_ignore="${WATCH_HOG_IGNORE:-firefox* chrome* chromium* Chrome* Google* electron* Electron* code Code* slack* Slack* Xorg Xwayland kwin* gnome-shell plasmashell mutter WindowServer kernel_task Safari* com.apple.* zoom* spotify* Spotify*}"
locks="${WATCH_LOCKS:-/tmp/loomarr-gpu.lock /tmp/loomarr-heavy-gate.lock}"

# polls_for MINUTES: consecutive polls that cover MINUTES at this interval (at least one).
polls_for() {
	local n=$((($1 * 60 + interval - 1) / interval))
	[ "$n" -lt 1 ] && n=1
	printf '%s\n' "$n"
}

top_processes() {
	ps -A -o pcpu= -o comm= 2>/dev/null | sort -rn | head -3 |
		awk '{ c = $2; sub(/.*\//, "", c); printf "%s%s %s%%", (NR > 1 ? ", " : ""), c, $1 }'
}

load_1m() {
	if [ "$WATCH_OS" = Darwin ]; then
		sysctl -n vm.loadavg | tr -d '{}' | awk '{ print $1 }'
	else
		cut -d' ' -f1 "$proc/loadavg"
	fi
}

check_load() {
	local cores l1 n polls
	cores="$(watch_cores)"
	l1="$(load_1m)"
	polls="$(polls_for "${WATCH_LOAD_MINUTES:-3}")"
	if watch_gt "$l1" "$(awk -v c="$cores" -v r="${WATCH_LOAD_RATIO:-0.7}" 'BEGIN { print c * r }')"; then
		n="$(watch_streak load yes)"
		[ "$n" -ge "$polls" ] &&
			watch_alert load high "LOAD high: $l1 on $cores cores for ${WATCH_LOAD_MINUTES:-3}+ min; top: $(top_processes)"
	else
		watch_streak load no >/dev/null
		watch_clear load load
	fi
}

# mem_kib: "available total" in KiB.
mem_kib() {
	if [ "$WATCH_OS" = Darwin ]; then
		vm_stat | awk -v total="$(sysctl -n hw.memsize)" '
			/page size of/ { for (i = 1; i <= NF; i++) if ($i == "of") ps = $(i + 1) }
			/^Pages (free|inactive|speculative|purgeable):/ { gsub(/\./, "", $NF); pages += $NF }
			END { printf "%d %d\n", pages * ps / 1024, total / 1024 }'
	else
		awk '/^MemAvailable:/ { a = $2 } /^MemTotal:/ { t = $2 } END { printf "%d %d\n", a, t }' "$proc/meminfo"
	fi
}

check_memory() {
	local avail total
	read -r avail total <<EOF
$(mem_kib)
EOF
	[ "${total:-0}" -gt 0 ] || return 0
	if [ $((avail * 100 / total)) -lt "${WATCH_MEM_MIN_PCT:-10}" ]; then
		watch_alert mem low "MEMORY low: $(awk -v a="$avail" 'BEGIN { printf "%.1f", a / 1048576 }') GiB available of $(awk -v t="$total" 'BEGIN { printf "%.0f", t / 1048576 }') GiB"
	else
		watch_clear mem memory
	fi
}

# fs_type MOUNTPOINT
fs_type() {
	if [ "$WATCH_OS" = Darwin ]; then
		mount | awk -v mp="$1" '$3 == mp { t = $4; gsub(/[(,]/, "", t); print t; exit }'
	else
		awk -v mp="$1" '$2 == mp { t = $3 } END { print t }' "$proc/mounts"
	fi
}

check_tmp() {
	local dir="${WATCH_TMP_DIR:-/tmp}" pct mp biggest
	read -r pct mp <<EOF
$(df -P "$dir" 2>/dev/null | awk 'NR == 2 { gsub(/%/, "", $5); print $5, $6 }')
EOF
	[ -n "${pct:-}" ] || return 0
	if [ "${WATCH_TMP_ANY_FS:-0}" != 1 ] && [ "$(fs_type "$mp")" != tmpfs ]; then
		return 0
	fi
	if [ "$pct" -gt "${WATCH_TMP_PCT:-60}" ]; then
		if [ ! -f "$WATCH_STATE/alert.tmp" ]; then
			biggest="$(du -xsk "$dir"/* 2>/dev/null | sort -rn | head -3 |
				awk '{ n = $2; sub(/.*\//, "", n); printf "%s%s %dM", (NR > 1 ? ", " : ""), n, $1 / 1024 }')"
		fi
		watch_alert tmp full "$dir (RAM) ${pct}% full; biggest: ${biggest:-?}"
	else
		watch_clear tmp "$dir"
	fi
}

check_orphans() {
	local found
	found="$(list_orphans | awk -F '\t' '{ printf " %s:%s", $1, $2 }')"
	if [ -n "$found" ]; then
		watch_alert orphans "$found" "ORPHANS in deleted worktrees:$found (stop them by PID: make agent-reap)"
	else
		watch_clear orphans orphans
	fi
}

# cpu_times: "pid<TAB>cumulative-cpu-seconds<TAB>command" for every process.
# `ps -o time` is cumulative CPU on both GNU ("[DD-]HH:MM:SS") and BSD ("[DD-][HH:]MM:SS.ss") ps; the
# delta between polls is the real recent rate. `pcpu` is not: GNU reports a lifetime average, which
# misses a long-lived process that has only just started spinning.
cpu_times() {
	ps -A -o pid= -o time= -o comm= 2>/dev/null | awk '
		{
			t = $2; d = 0
			if (index(t, "-")) { split(t, a, "-"); d = a[1]; t = a[2] }
			n = split(t, f, ":"); s = 0
			for (i = 1; i <= n; i++) s = s * 60 + f[i]
			c = $0; sub(/^[ \t]*[0-9]+[ \t]+[^ \t]+[ \t]+/, "", c); sub(/.*\//, "", c)
			printf "%s\t%.2f\t%s\n", $1, d * 86400 + s, c
		}'
}

hog_ignored() {
	local pattern
	for pattern in $hog_ignore; do
		# shellcheck disable=SC2254 # The ignore list is globs on purpose.
		case $1 in $pattern) return 0 ;; esac
	done
	return 1
}

check_hogs() {
	local now prev_at elapsed polls prev="$WATCH_STATE/cpu.prev" next="$WATCH_STATE/cpu.next"
	local pid pct streak comm f key
	now="${WATCH_NOW:-$(date +%s)}"
	prev_at="$(cat "$WATCH_STATE/cpu.at" 2>/dev/null || echo 0)"
	printf '%s' "$now" > "$WATCH_STATE/cpu.at"
	elapsed=$((now - prev_at))
	polls="$(polls_for "${WATCH_HOG_MINUTES:-5}")"
	[ -f "$prev" ] || : > "$prev"
	# Join this sample with the last one by pid+command (a reused pid starts over) and carry the streak.
	cpu_times | awk -F '\t' -v OFS='\t' -v prev="$prev" -v el="$elapsed" -v cores="${WATCH_HOG_CORES:-1.5}" '
		BEGIN { while ((getline line < prev) > 0) { split(line, p, "\t"); secs[p[1] "|" p[3]] = p[2]; run[p[1] "|" p[3]] = p[4] } }
		{
			k = $1 "|" $3; streak = 0; pct = 0
			if ((k in secs) && el > 0) {
				pct = ($2 - secs[k]) * 100 / el
				if (pct > cores * 100) streak = run[k] + 1
			}
			print $1, $2, $3, streak, int(pct)
		}' > "$next"
	mv "$next" "$prev"
	while IFS="$(printf '\t')" read -r pid _ comm streak pct; do
		[ "${streak:-0}" -ge "$polls" ] || continue
		hog_ignored "$comm" && continue
		watch_alert "hog.$pid" "$comm" "CPU hog: pid $pid $comm at ${pct}% for ${WATCH_HOG_MINUTES:-5}+ min"
	done < "$prev"
	for f in "$WATCH_STATE"/alert.hog.*; do
		[ -f "$f" ] || continue
		key="${f##*/alert.}"
		pid="${key#hog.}"
		awk -F '\t' -v p="$pid" -v n="$polls" '$1 == p && $4 >= n { found = 1 } END { exit !found }' "$prev" ||
			watch_clear "$key" "CPU hog pid $pid"
	done
}

check_gpu() {
	local util n
	[ "$WATCH_OS" = Darwin ] && return 0
	command -v nvidia-smi >/dev/null 2>&1 || return 0
	util="$(nvidia-smi --query-gpu=utilization.gpu --format=csv,noheader,nounits 2>/dev/null | sort -rn | head -1 | tr -dc 0-9)"
	[ -n "$util" ] || return 0
	if [ "$util" -gt "${WATCH_GPU_PCT:-90}" ]; then
		n="$(watch_streak gpu yes)"
		[ "$n" -ge "$(polls_for "${WATCH_GPU_MINUTES:-5}")" ] &&
			watch_alert gpu busy "GPU busy: ${util}% for ${WATCH_GPU_MINUTES:-5}+ min; $(nvidia-smi --query-compute-apps=pid,process_name --format=csv,noheader 2>/dev/null | head -3 | tr '\n' ' ')"
	else
		watch_streak gpu no >/dev/null
		watch_clear gpu GPU
	fi
}

# lock_holders LOCK: pids with LOCK open.
lock_holders() {
	if [ "$WATCH_OS" = Darwin ]; then
		lsof -w -t -- "$1" 2>/dev/null
	else
		find "$proc"/[0-9]*/fd -lname "$1" 2>/dev/null | awk -F/ '{ print $(NF - 2) }' | sort -u
	fi
}

pid_comm() {
	if [ "$WATCH_OS" = Darwin ]; then
		ps -o comm= -p "$1" 2>/dev/null | sed 's#.*/##'
	else
		cat "$proc/$1/comm" 2>/dev/null
	fi
}

check_locks() {
	local lock pid idle n name
	for lock in $locks; do
		name="$(basename "$lock")"
		idle=''
		if [ -e "$lock" ]; then
			for pid in $(lock_holders "$lock"); do
				[ "$(pid_comm "$pid")" = sleep ] && idle="$idle $pid"
			done
		fi
		if [ -n "$idle" ]; then
			n="$(watch_streak "lock.$name" yes)"
			[ "$n" -ge "${WATCH_LOCK_POLLS:-2}" ] &&
				watch_alert "lock.$name" "$idle" "LOCK $name held by an idle sleep (pid$idle): whoever started it should release it"
		else
			watch_streak "lock.$name" no >/dev/null
			watch_clear "lock.$name" "LOCK $name"
		fi
	done
}

poll() {
	check_load
	check_memory
	check_tmp
	check_orphans
	check_hogs
	check_gpu
	check_locks
}

# snapshot: one reading for the session-start hook. It must stay well under a second and print no
# paths or names, so orphans are only counted, and only on Linux (a macOS lsof scan can take seconds).
snapshot() {
	local avail total pct mp tmp_note='' orphans=''
	read -r avail total <<EOF
$(mem_kib 2>/dev/null)
EOF
	read -r pct mp <<EOF
$(df -P "${WATCH_TMP_DIR:-/tmp}" 2>/dev/null | awk 'NR == 2 { gsub(/%/, "", $5); print $5, $6 }')
EOF
	[ -n "${pct:-}" ] && tmp_note="; /tmp ${pct}% full"
	[ -n "${mp:-}" ] && [ "$(fs_type "$mp" 2>/dev/null)" = tmpfs ] && tmp_note="$tmp_note (RAM)"
	[ "$WATCH_OS" = Darwin ] || orphans="; $(list_orphans | wc -l | tr -d ' ') processes in deleted worktrees"
	printf 'Dev machine: load %s on %s cores; %s GiB memory available of %s%s%s.\n' \
		"$(load_1m 2>/dev/null)" "$(watch_cores 2>/dev/null)" \
		"$(awk -v a="${avail:-0}" 'BEGIN { printf "%.1f", a / 1048576 }')" \
		"$(awk -v t="${total:-0}" 'BEGIN { printf "%.0f", t / 1048576 }')" "$tmp_note" "$orphans"
	printf '%s\n' 'Supervisors and long-running agents: arm the dev watchers with "make dev-watch" (docs/contributing/dev-watchers.md).'
}

if [ "${1:-}" = --snapshot ]; then
	snapshot 2>/dev/null
	exit 0
fi
watch_state_init resources
if [ "${1:-}" = --once ]; then
	poll
	exit 0
fi
while :; do
	poll
	watch_sleep "$interval"
done
