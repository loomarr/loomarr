#!/usr/bin/env bash
# Live check of the resource watcher on this machine (Linux or macOS): plant each condition for real,
# confirm the watcher reports it once, remove it, and confirm the watcher reports it clear.
# Run from any checkout of the repo: scripts/dev/watch-verify.sh   (about two minutes, exit 0 = pass)
#
# It plants only things it owns and removes them: a `sleep` in a deleted directory beside the primary
# checkout (the orphan), `yes > /dev/null` (the hog, judged at 0.8 cores so one thread crosses it),
# a `sleep` holding a private lock file, and on Linux a small tmpfs in a user namespace (/tmp full).
# It never touches the shared GPU or heavy-gate locks.

set -u

DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)"
work="$(CDPATH='' cd -- "$(mktemp -d)" && pwd -P)"
primary="$(git -C "$DIR" worktree list --porcelain | sed -n 's/^worktree //p' | head -1)"
probe="$primary-zz-watch-verify-$$"
log="$work/watch.log"
pids=''
failures=0

# Inline, not a function: shellcheck 0.9 (CI's) calls a trap-only function unreachable. $pids is one
# word per pid and is read when the trap runs.
trap 'kill $pids 2>/dev/null; rm -rf "$work" "$probe"' EXIT INT TERM

check() { # name pattern
	if grep -Eq -- "$2" "$log"; then
		printf 'PASS  %s\n' "$1"
	else
		printf 'FAIL  %s (no line matching /%s/)\n' "$1" "$2"
		failures=$((failures + 1))
	fi
}

once() { # name pattern: the alert must not repeat while the condition holds
	if [ "$(grep -Ec -- "$2" "$log")" = 1 ]; then
		printf 'PASS  %s\n' "$1"
	else
		printf 'FAIL  %s (expected exactly one line matching /%s/)\n' "$1" "$2"
		failures=$((failures + 1))
	fi
}

echo "watch-verify: $(uname -s) $(uname -r), bash $BASH_VERSION, $(getconf _NPROCESSORS_ONLN) cores"

printf '\n-- unit tests\n'
if "$DIR/watch-test.sh"; then echo 'PASS  watch-test.sh'; else echo 'FAIL  watch-test.sh'; failures=$((failures + 1)); fi

printf '\n-- session snapshot\n'
start=$(date +%s)
"$DIR/watch-resources.sh" --snapshot
echo "snapshot took about $(($(date +%s) - start)) s (must be under 1)"

printf '\n-- live: orphan, CPU hog, idle lock (about 90 s)\n'
WATCH_INTERVAL=10 WATCH_HOG_CORES=0.8 WATCH_HOG_MINUTES=1 WATCH_LOCKS="$work/test.lock" \
	"$DIR/watch-resources.sh" > "$log" 2>&1 &
pids="$pids $!"
mkdir -p "$probe/web"
(cd "$probe/web" && { sleep 600 & echo $! > "$work/orphan.pid"; })
rm -rf "$probe"
yes > /dev/null &
hog=$!
pids="$pids $hog"
: > "$work/test.lock"
sleep 45 3> "$work/test.lock" &
pids="$pids $!"
orphan=$(cat "$work/orphan.pid")
pids="$pids $orphan"
sleep 80
kill "$orphan" "$hog" 2>/dev/null
sleep 25
cat "$log"
check 'orphan reported' "ORPHANS in deleted worktrees: $orphan:sleep"
once 'orphan reported once' 'ORPHANS in deleted worktrees'
check 'orphan cleared' 'orphans: back to normal'
check 'CPU hog reported' "CPU hog: pid $hog yes"
check 'CPU hog cleared' "CPU hog pid $hog: back to normal"
check 'idle lock reported' 'LOCK test.lock held by an idle sleep'
check 'idle lock cleared' 'LOCK test.lock: back to normal'

if [ "$(uname -s)" = Linux ]; then
	printf '\n-- live: /tmp full on an 8 MiB tmpfs\n'
	if command -v unshare >/dev/null 2>&1 && unshare -rm true 2>/dev/null; then
		mkdir -p "$work/mnt"
		# shellcheck disable=SC2016 # Expanded by the inner shell, from its positional arguments.
		unshare -rm bash -c '
			mount -t tmpfs -o size=8m tmpfs "$1" || exit 1
			WATCH_INTERVAL=3 WATCH_TMP_DIR="$1" WATCH_LOCKS=/nonexistent "$2/watch-resources.sh" & w=$!
			sleep 4; mkdir -p "$1/scratch"; head -c 6291456 /dev/zero > "$1/scratch/dump.bin"
			sleep 7; rm -rf "$1/scratch"; sleep 7; kill $w; wait $w 2>/dev/null; umount "$1"' _ "$work/mnt" "$DIR" > "$log" 2>&1
		cat "$log"
		check '/tmp full reported' '\(RAM\) 75% full; biggest: scratch'
		check '/tmp full cleared' 'mnt: back to normal'
	else
		echo 'SKIP  /tmp full: user namespaces are unavailable here'
	fi
else
	echo 'SKIP  /tmp full: macOS /tmp is on disk, not tmpfs, so the check does not apply'
fi

if [ "$failures" = 0 ]; then
	printf '\nwatch-verify: all checks passed\n'
	exit 0
fi
printf '\nwatch-verify: %s check(s) failed\n' "$failures"
exit 1
