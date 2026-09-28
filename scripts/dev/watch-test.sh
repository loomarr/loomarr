#!/usr/bin/env bash
# Regression tests for the dev watchers' threshold, streak, dedupe and clear logic.
# Everything runs against a fake /proc tree and command shims, so it needs no load, GPU, tmpfs, gh or orca.

set -euo pipefail

SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)"
# Canonical, because macOS reports process cwds under /private/var while mktemp says /var.
tmp="$(CDPATH='' cd -- "$(mktemp -d)" && pwd -P)"
trap 'rm -rf "$tmp"' EXIT INT TERM

failures=0
fail() {
	printf 'FAIL: %s\n' "$*" >&2
	failures=$((failures + 1))
}

# expect OUTPUT PATTERN: the output has a line matching the extended regex.
expect() {
	printf '%s\n' "$1" | grep -Eq -- "$2" || fail "expected /$2/ in: ${1:-<nothing>}"
}

expect_none() {
	[ -z "$1" ] || fail "expected no output, got: $1"
}

# ---------------------------------------------------------------- watch-resources

P="$tmp/proc"
BIN="$tmp/bin"
FIX="$tmp/fix"
mkdir -p "$P" "$BIN" "$FIX" "$tmp/tmpfs" "$tmp/worktrees/live" "$tmp/state"

cat > "$BIN/ps" <<'EOF'
#!/usr/bin/env bash
case "$*" in
	*time=*) cat "$FIX/ps-time" ;;
	*pcpu=*) printf ' 99.0 /usr/bin/yes\n 12.0 node\n' ;;
esac
EOF
cat > "$BIN/df" <<'EOF'
#!/usr/bin/env bash
printf 'Filesystem 1024-blocks Used Available Capacity Mounted on\ntmpfs 100 %s 0 %s%% %s\n' \
	"$(cat "$FIX/df-pct")" "$(cat "$FIX/df-pct")" "$(cat "$FIX/df-mount")"
EOF
cat > "$BIN/nvidia-smi" <<'EOF'
#!/usr/bin/env bash
case "$*" in
	*utilization*) cat "$FIX/gpu" ;;
	*compute-apps*) printf '4242, ffmpeg\n' ;;
esac
EOF
cat > "$BIN/sysctl" <<'EOF'
#!/usr/bin/env bash
case "$2" in
	vm.loadavg) printf '{ %s 1.00 1.00 }\n' "$(cat "$FIX/load")" ;;
	hw.memsize) echo 17179869184 ;;
	hw.ncpu) echo 8 ;;
esac
EOF
cat > "$BIN/vm_stat" <<'EOF'
#!/usr/bin/env bash
printf 'Mach Virtual Memory Statistics: (page size of 16384 bytes)\nPages free:                               %s.\nPages active:                          500000.\nPages inactive:                             0.\nPages speculative:                          0.\nPages purgeable:                            0.\n' "$(cat "$FIX/mac-free-pages")"
EOF
cat > "$BIN/lsof" <<'EOF'
#!/usr/bin/env bash
case "$*" in
	*'-d cwd'*) cat "$FIX/lsof-cwd" ;;
	*'-t'*) cat "$FIX/lsof-lock" ;;
esac
EOF
cat > "$BIN/mount" <<'EOF'
#!/usr/bin/env bash
printf '/dev/disk3s1 on / (apfs, local)\n'
EOF
chmod +x "$BIN"/*

REAL_PATH="$PATH"
export PATH="$BIN:$PATH" FIX WATCH_PROC="$P" WATCH_CORES=10 WATCH_INTERVAL=60 WATCH_STATE_DIR="$tmp/state"
export WATCH_WORKTREE_ROOTS="$tmp/worktrees" WATCH_TMP_DIR="$tmp/tmpfs" WATCH_LOCKS="$tmp/gpu.lock"
export WATCH_OS=Linux

neutral() {
	printf '1.00 1.00 1.00 1/100 1\n' > "$P/loadavg"
	printf 'MemTotal:       10000000 kB\nMemAvailable:    5000000 kB\n' > "$P/meminfo"
	printf 'tmpfs %s tmpfs rw 0 0\n' "$tmp/tmpfs" > "$P/mounts"
	echo 10 > "$FIX/df-pct"
	echo "$tmp/tmpfs" > "$FIX/df-mount"
	echo 0 > "$FIX/gpu"
	: > "$FIX/ps-time"
	rm -rf "$P"/[0-9]* "$tmp/state"
	echo 100000 > "$FIX/now"
}

# res: one poll, 60 s after the last. The clock lives in a file because callers run res in $(...).
res() {
	local now
	now=$(($(cat "$FIX/now") + 60))
	echo "$now" > "$FIX/now"
	WATCH_NOW="$now" "$SCRIPT_DIR/watch-resources.sh" --once
}

# Load: fires only after three polls in a row above 70% of cores, once, then clears once.
neutral
printf '8.50 1.00 1.00 1/100 1\n' > "$P/loadavg"
out="$(res)$(res)"
expect_none "$out"
out="$(res)"
expect "$out" 'LOAD high: 8.50 on 10 cores for 3\+ min; top: yes 99.0%'
expect_none "$(res)"
printf '6.90 1.00 1.00 1/100 1\n' > "$P/loadavg"
expect "$(res)" 'load: back to normal'
expect_none "$(res)"
# A dip resets the streak.
printf '8.50 1.00 1.00 1/100 1\n' > "$P/loadavg"
out="$(res)$(res)"
printf '1.00 1.00 1.00 1/100 1\n' > "$P/loadavg"
out="$out$(res)"
printf '8.50 1.00 1.00 1/100 1\n' > "$P/loadavg"
out="$out$(res)$(res)"
expect_none "$out"

# Memory: below 10% of the total, reported once, cleared once.
neutral
printf 'MemTotal:       10000000 kB\nMemAvailable:     900000 kB\n' > "$P/meminfo"
expect "$(res)" 'MEMORY low: 0.9 GiB available of 10 GiB'
expect_none "$(res)"
printf 'MemTotal:       10000000 kB\nMemAvailable:    2000000 kB\n' > "$P/meminfo"
expect "$(res)" 'memory: back to normal'

# /tmp: only when it is tmpfs.
neutral
echo 70 > "$FIX/df-pct"
mkdir -p "$tmp/tmpfs/scratch-big"
out="$(res)"
expect "$out" 'tmpfs \(RAM\) 70% full; biggest: scratch-big'
echo 50 > "$FIX/df-pct"
expect "$(res)" 'back to normal'
neutral
echo 70 > "$FIX/df-pct"
printf '/dev/sda1 %s ext4 rw 0 0\n' "$tmp/tmpfs" > "$P/mounts"
expect_none "$(res)"

# Orphans: a cwd deleted under a worktree root, or inside Orca's trash; not a live cwd, not a deleted
# directory that was never a worktree.
fake_proc() { # pid comm cwd
	mkdir -p "$P/$1"
	printf '%s\n' "$2" > "$P/$1/comm"
	ln -s "$3" "$P/$1/cwd"
}
neutral
fake_proc 101 air "$tmp/worktrees/lane-gone (deleted)"
fake_proc 102 node "$tmp/elsewhere/.orca-worktree-trash/lane-old/web"
fake_proc 103 bash "$tmp/worktrees/live"
fake_proc 104 vim "$tmp/unrelated/dir (deleted)"
out="$(res)"
expect "$out" 'ORPHANS in deleted worktrees: 101:air 102:node \(stop them by PID'
case $out in *103* | *104*) fail "live or unrelated cwd reported: $out" ;; esac
expect_none "$(res)"
fake_proc 105 vite "$tmp/worktrees/lane-gone/web/apps/web (deleted)"
expect "$(res)" '101:air 102:node 105:vite'
# Same answer from the BSD-find branch (no -printf), which the harness takes on macOS.
mkdir -p "$tmp/bsdfind"
real_find="$(command -v find)"
printf '#!/usr/bin/env bash\ncase " $* " in *" -printf "*) exit 1 ;; esac\nexec %s "$@"\n' "$real_find" > "$tmp/bsdfind/find"
chmod +x "$tmp/bsdfind/find"
rm -rf "$tmp/state"
expect "$(PATH="$tmp/bsdfind:$PATH" res)" 'ORPHANS in deleted worktrees: 101:air 102:node 105:vite'
rm -rf "$P/101" "$P/102" "$P/105"
expect "$(res)" 'orphans: back to normal'

# CPU hog: measured from the cumulative-time delta, both GNU and BSD `ps -o time` shapes.
neutral
echo 0 > "$FIX/cpu"
hog_poll() { # cpu-seconds added this poll
	local cpu
	cpu=$(($(cat "$FIX/cpu") + $1))
	echo "$cpu" > "$FIX/cpu"
	printf '  777 %02d:%02d:%02d yes\n  778 1:%02d.50 /Applications/Firefox.app/Contents/MacOS/firefox\n' \
		$((cpu / 3600)) $((cpu / 60 % 60)) $((cpu % 60)) $((cpu % 60)) > "$FIX/ps-time"
	res
}
out="$(hog_poll 0)$(hog_poll 120)$(hog_poll 120)$(hog_poll 120)$(hog_poll 120)"
expect_none "$out"
out="$(hog_poll 120)"
expect "$out" 'CPU hog: pid 777 yes at 200% for 5\+ min'
case $out in *firefox*) fail "ignored desktop app reported: $out" ;; esac
expect_none "$(hog_poll 120)"
expect "$(hog_poll 30)" 'CPU hog pid 777: back to normal'
# A process that was idle for most of its life but spins now is still caught.
neutral
cpu=0
printf '  888 3-00:00:00 idler\n' > "$FIX/ps-time"
out="$(res)"
for _ in 1 2 3 4 5; do
	cpu=$((cpu + 100))
	printf '  888 3-00:%02d:%02d idler\n' $((cpu / 60)) $((cpu % 60)) > "$FIX/ps-time"
	out="$(res)"
done
expect "$out" 'CPU hog: pid 888 idler at 166%'

# GPU: five polls above 90%.
neutral
echo 95 > "$FIX/gpu"
out="$(res)$(res)$(res)$(res)"
expect_none "$out"
expect "$(res)" 'GPU busy: 95% for 5\+ min; 4242, ffmpeg'
echo 10 > "$FIX/gpu"
expect "$(res)" 'GPU: back to normal'

# Lock held by an idle sleep for two polls; a working holder is fine.
neutral
: > "$tmp/gpu.lock"
fake_proc 301 ffmpeg /
mkdir -p "$P/301/fd"
ln -s "$tmp/gpu.lock" "$P/301/fd/3"
expect_none "$(res)$(res)$(res)"
fake_proc 302 sleep /
mkdir -p "$P/302/fd"
ln -s "$tmp/gpu.lock" "$P/302/fd/3"
expect_none "$(res)"
expect "$(res)" 'LOCK gpu.lock held by an idle sleep \(pid 302\)'
rm -rf "$P/302"
expect "$(res)" 'LOCK gpu.lock: back to normal'

# macOS: sysctl load, vm_stat memory, lsof cwd and lock holders, no tmpfs, no nvidia-smi.
neutral
export WATCH_OS=Darwin WATCH_CORES=
echo 7.00 > "$FIX/load"
echo 10000 > "$FIX/mac-free-pages"
printf 'p401\ncnode\nn%s\np402\nczsh\nn%s\n' "$tmp/worktrees/lane-mac" "$tmp/worktrees/live" > "$FIX/lsof-cwd"
: > "$FIX/lsof-lock"
echo 95 > "$FIX/gpu"
out="$(res)$(res)$(res)$(res)$(res)"
expect "$out" 'LOAD high: 7.00 on 8 cores'
expect "$out" 'MEMORY low: 0.2 GiB available of 16 GiB'
expect "$out" 'ORPHANS in deleted worktrees: 401:node'
case $out in *GPU* | *RAM*) fail "macOS reported a Linux-only check: $out" ;; esac
export WATCH_OS=Linux WATCH_CORES=10

# ---------------------------------------------------------------- watch-prs

cat > "$BIN/gh" <<'EOF'
#!/usr/bin/env bash
case "$1 $2" in
	'repo view') echo example/repo ;;
	'api graphql') printf '%s\n' "$@" > "$FIX/gh-args"; jq -s '{data: {search: {nodes: .}}}' "$FIX/prs" ;;
esac
EOF
chmod +x "$BIN/gh"

# pr NUMBER HEAD STATE AUTO QUEUED LAST-QUEUE-EVENT FAILED-CHECK UPDATED [DRAFT] [BASE]
pr() {
	jq -n --argjson n "$1" --arg head "$2" --arg state "$3" --argjson auto "$4" --argjson queued "$5" \
		--arg mq "$6" --arg failed "$7" --arg updated "$8" --argjson draft "${9:-false}" --arg base "${10:-main}" '{
		number: $n, title: ("PR \($n) title"), isDraft: $draft, baseRefName: $base, headRefOid: $head,
		mergeStateStatus: $state, updatedAt: $updated,
		autoMergeRequest: (if $auto then {enabledAt: $updated} else null end),
		mergeQueueEntry: (if $queued then {state: "AWAITING_CHECKS"} else null end),
		timelineItems: {nodes: (if $mq == "removed" then [{__typename: "RemovedFromMergeQueueEvent", reason: "failed checks"}]
			elif $mq == "added" then [{__typename: "AddedToMergeQueueEvent"}] else [] end)},
		commits: {nodes: [{commit: {statusCheckRollup: {contexts: {nodes: ([
			{__typename: "CheckRun", name: "Docs / links", conclusion: "SUCCESS"},
			{__typename: "CheckRun", name: "superseded", conclusion: "CANCELLED"}]
			+ (if $failed == "" then [] else [{__typename: "CheckRun", name: $failed, conclusion: "FAILURE"},
				{__typename: "StatusContext", context: "legacy/status", state: "ERROR"}] end))}}}}]}}'
}
old=2020-01-01T00:00:00Z
new=2099-01-01T00:00:00Z
prs() { WATCH_STATE_DIR="$tmp/state" "$SCRIPT_DIR/watch-prs.sh" --once; }
rm -rf "$tmp/state"
{
	pr 1 aaa DIRTY true false "" "" "$old"
	pr 2 bbb BLOCKED true false "" "Go / contracts" "$old"
	pr 3 ccc BEHIND true false "" "" "$old"
	pr 4 ddd CLEAN false false "" "" "$old"
	pr 5 eee CLEAN false false "" "" "$new"
	pr 6 fff BLOCKED false false removed "" "$old"
	pr 7 ggg CLEAN false true added "" "$old"
	pr 8 hhh DIRTY false false "" "" "$old" true
	pr 9 iii DIRTY false false "" "" "$old" false feature-base
} > "$FIX/prs"
out="$(prs)"
expect "$out" '#1 CONFLICTS with main \(PR 1 title\)'
expect "$out" '#2 CHECKS FAILED: Go / contracts, legacy/status \(PR 2 title\)'
expect "$out" '#3 is BEHIND main with auto-merge on and will not queue itself: gh pr update-branch 3'
expect "$out" '#4 has auto-merge OFF and is not queued'
expect "$out" '#6 was DROPPED from the merge queue \(failed checks\)'
case $out in *'#5 '* | *'#7 '* | *'#8 '* | *'#9 '* | *superseded* | *'#6 has auto-merge'* | *'#1 has auto-merge'*)
	fail "reported a PR that is fine, recent, queued, draft, stacked, or already reported: $out" ;;
esac
grep -Fqx 'q=repo:example/repo is:pr is:open author:@me base:main' "$FIX/gh-args" || fail "search query: $(cat "$FIX/gh-args")"
# Dedupe per (PR, condition, head): silent on the same heads, again on a new push.
expect_none "$(prs)"
pr 1 aaa2 DIRTY true false "" "" "$old" > "$FIX/prs"
out="$(prs)"
expect "$out" '#1 CONFLICTS with main'
[ "$(printf '%s\n' "$out" | grep -c .)" = 1 ] || fail "expected one alert for the new head, got: $out"
# gh failing (offline) is quiet, not fatal.
printf 'not json' > "$FIX/prs"
expect_none "$(prs 2>&1)"

# ---------------------------------------------------------------- watch-lanes

cat > "$BIN/orca" <<'EOF'
#!/usr/bin/env bash
case "$1 $2" in
	'terminal list') cat "$FIX/orca-list" ;;
	'terminal read') cat "$FIX/orca-read-$4" ;;
esac
EOF
chmod +x "$BIN/orca"
export CLAUDE_CONFIG_DIR="$tmp/claude"
lane_a="$tmp/worktrees/lane-a"
lane_dir="$CLAUDE_CONFIG_DIR/projects/$(printf '%s' "$lane_a" | sed 's/[^A-Za-z0-9]/-/g')"
mkdir -p "$lane_dir/sess1/subagents"
jq -n --arg a "$lane_a" --arg b "$tmp/elsewhere/other-repo" '{result: {terminals: [
	{handle: "t1", worktreePath: $a, agentIdentity: "claude"},
	{handle: "t2", worktreePath: $a, agentIdentity: null},
	{handle: "t3", worktreePath: $b, agentIdentity: "claude"}]}}' > "$FIX/orca-list"
screen() { # handle text
	jq -n --arg t "$2" '{result: {terminal: {tail: ["line one", $t]}}}' > "$FIX/orca-read-$1"
}
# message ID TOKENS: one API message written as two transcript lines repeating the same usage.
message() {
	local line
	line="$(jq -cn --arg id "$1" --argjson n "$2" '{type: "assistant", message: {id: $id, usage: {output_tokens: $n}}}')"
	printf '%s\n%s\n{"type":"user"}\n' "$line" "$line"
}
lanes_once() { WATCH_STATE_DIR="$tmp/state" "$SCRIPT_DIR/watch-lanes.sh" --once; }
rm -rf "$tmp/state"
{ message m1 100000; message m2 40000; } > "$lane_dir/sess1.jsonl"
message s1 5000 > "$lane_dir/sess1/subagents/agent-1.jsonl"
screen t1 '✻ Working… (esc to interrupt)'
expect_none "$(lanes_once)"
message m3 10000 >> "$lane_dir/sess1.jsonl"
out="$(lanes_once)"
expect "$out" 'lane-a: 155000 output tokens \(warning at 150000\)'
case $out in *t3* | *other-repo*) fail "reported a terminal outside this repo: $out" ;; esac
expect_none "$(lanes_once)"
message m4 40000 >> "$lane_dir/sess1.jsonl"
expect "$(lanes_once)" 'lane-a: 195000 output tokens: CUTOFF \(190000\) reached'
# An old finished marker in the scrollback while the lane is working again is not idle.
screen t1 '✻ Baked for 1m 2s  > next task  ✻ Working… (esc to interrupt)'
expect_none "$(lanes_once)"
screen t1 '✻ Cogitated for 3m 12s'
expect "$(lanes_once)" 'lane-a: turn finished, waiting'
expect_none "$(lanes_once)"
screen t1 '❯ 1. Yes  2. No   Enter to select'
expect "$(lanes_once)" 'lane-a: WAITING ON A QUESTION MENU'
screen t1 'API Error: 529 overloaded'
expect "$(lanes_once)" 'lane-a: API trouble'
# A newer session is a new checkpoint: it re-arms the levels and meters only itself.
sleep 1
message n1 160000 > "$lane_dir/sess2.jsonl"
expect "$(lanes_once)" 'lane-a: 160000 output tokens \(warning'
# An explicit checkpoint meters every transcript born since, across sessions.
mkdir -p "$tmp/ckpt"
echo $(($(date +%s) - 60)) > "$tmp/ckpt/lane-a"
out="$(WATCH_LANES_CHECKPOINTS="$tmp/ckpt" lanes_once)"
expect "$out" 'lane-a: 355000 output tokens: LIMIT \(240000\) exceeded'
# A session born before the checkpoint but flushed after it (its /exit write) is not counted.
# Needs a filesystem that records birth times; skipped where stat cannot report one.
if [ "$(stat -c %W "$lane_dir/sess1.jsonl" 2>/dev/null || echo 0)" != 0 ]; then
	later=$(($(date +%s) + 100))
	echo "$later" > "$tmp/ckpt/lane-a"
	rm "$lane_dir/sess2.jsonl"
	touch -d "@$((later + 100))" "$lane_dir/sess1.jsonl" "$lane_dir/sess1/subagents/agent-1.jsonl"
	expect_none "$(WATCH_LANES_CHECKPOINTS="$tmp/ckpt" lanes_once)"
fi
# No orca: skip cleanly.
out="$(PATH=/usr/bin:/bin WATCH_STATE_DIR="$tmp/state" "$SCRIPT_DIR/watch-lanes.sh" --once 2>&1)" || fail "watch-lanes failed without orca"
expect "$out" 'orca CLI not found; skipping'

# ---------------------------------------------------------------- worktree-procs (real processes)

unset WATCH_PROC WATCH_WORKTREE_ROOTS
WATCH_OS="$(uname -s)"
export WATCH_OS WATCH_REAP_GRACE=2 PATH="$REAL_PATH"
wt="$tmp/reap/wt"
mkdir -p "$wt/web/apps/web" "$tmp/reap/other"
# Detach each sleeper (its subshell exits), so a stopped one is reaped by init and kill -0 sees it gone.
spawn() { # dir name
	(cd "$1" && { sleep 300 & echo $! > "$tmp/reap/$2"; })
}
spawn "$wt/web/apps/web" sub
spawn "$wt" root
spawn "$tmp/reap/other" outside
sub=$(cat "$tmp/reap/sub") root=$(cat "$tmp/reap/root") outside=$(cat "$tmp/reap/outside")
trap 'kill "$sub" "$root" "$outside" 2>/dev/null || true; rm -rf "$tmp"' EXIT INT TERM
out="$(cd "$wt" && "$SCRIPT_DIR/worktree-procs.sh" "$wt")"
expect "$out" "^$sub	sleep	$wt/web/apps/web\$"
expect "$out" "^$root	sleep	$wt\$"
# Neither the process outside nor the caller's own shell (its cwd is the worktree) is listed.
[ "$(printf '%s\n' "$out" | grep -c .)" = 2 ] || fail "expected exactly the two sleepers, got: $out"
(cd "$wt" && "$SCRIPT_DIR/worktree-procs.sh" "$wt" --kill >/dev/null 2>&1) || fail 'worktree-procs --kill failed'
kill -0 "$sub" 2>/dev/null && fail "sleeper in a subdirectory survived --kill"
kill -0 "$root" 2>/dev/null && fail "sleeper at the root survived --kill"
kill -0 "$outside" 2>/dev/null || fail "--kill stopped a process outside the worktree"
kill "$outside"
# After the worktree is deleted, --orphans finds what still runs in it (the Orca trash case).
spawn "$wt/web/apps/web" late
late=$(cat "$tmp/reap/late")
rm -rf "$wt"
out="$(WATCH_WORKTREE_ROOTS="$tmp/reap" "$SCRIPT_DIR/worktree-procs.sh" --orphans)"
expect "$out" "^$late	sleep	"
kill "$late"

if [ "$failures" -gt 0 ]; then
	printf 'watch-test: %s failure(s)\n' "$failures" >&2
	exit 1
fi
echo 'watch-test: ok'
