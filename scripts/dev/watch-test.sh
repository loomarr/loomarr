#!/usr/bin/env bash
# Regression tests for the dev watchers' threshold, streak, dedupe and clear logic.
# Everything runs against a fake /proc tree and command shims, so it needs no load, GPU, tmpfs, gh or orca.

set -euo pipefail

SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)"
tmp="$(mktemp -d)"
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

if [ "$failures" -gt 0 ]; then
	printf 'watch-test: %s failure(s)\n' "$failures" >&2
	exit 1
fi
echo 'watch-test: ok'
