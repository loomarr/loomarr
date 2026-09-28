#!/usr/bin/env bash
# Channel-surf certification (#1037, #1512 G3): presses a remote key every INTERVAL seconds for
# MINUTES minutes on one device and reports key-to-first-frame latency (warm and cold), the held
# frame (the switch readout, OSD) and the still, as p50/p95, from the TV app's LoomarrCert logcat marks.
#
#   ADB_SERIAL=<serial> scripts/shield-cert/surf.sh
#
# Environment:
#   ADB_SERIAL   required; the device to drive (adb devices -l)
#   INTERVAL     seconds between keys (default 5)
#   MINUTES      how long to surf (default 5)
#   KEYS         channel (CHANNEL_UP, the default) or dpad (DPAD_UP): both step to the next channel
#   JUMP         space-separated channel numbers to tune by digits instead of stepping, e.g.
#                "101 104 102 105": far jumps land on channels nobody warmed (cold tunes). The
#                entry commits 1.2 s after the last digit, so a jump is timed from that commit.
#   OUT          report directory (default .artifacts/shield-cert/surf-<device time>)
#   LAUNCH       1 (default) brings the app to the foreground first; 0 leaves the screen alone
#   SETTLE       seconds to wait after launching before the first key (default 10)
#   PACKAGE      the app id when it is neither loomarr.media nor the prototype
#   CODEC_INIT_RE  see lib.sh
#
# The capture keeps only LoomarrCert marks and codec lines: never a URL or a title.
set -euo pipefail

# shellcheck source=scripts/shield-cert/lib.sh
. "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

interval=${INTERVAL:-5}
minutes=${MINUTES:-5}
keys=${KEYS:-channel}
jump=${JUMP:-}
case "$keys" in
channel) step_key=KEYCODE_CHANNEL_UP ;;
dpad) step_key=KEYCODE_DPAD_UP ;;
*) cert_die "KEYS must be channel or dpad" 2 ;;
esac
case "$interval$minutes" in
*[!0-9]*) cert_die 'INTERVAL and MINUTES are whole numbers' 2 ;;
esac
[ "$interval" -ge 1 ] || cert_die 'INTERVAL must be at least 1 second' 2

cert_require_serial
cert_online || cert_die "device $ADB_SERIAL is not online"
package=$(cert_package)
start=$(cert_device_epoch)
out=${OUT:-.artifacts/shield-cert/surf-$start}
mkdir -p "$out"

if [ "${LAUNCH:-1}" = 1 ]; then
	cert_launch "$package"
	sleep "${SETTLE:-10}"
fi

fifo="$out/.logcat.fifo"
rm -f "$fifo"
mkfifo "$fifo"
since=$(cert_device_epoch)
adb -s "$ADB_SERIAL" logcat -v epoch -T "$since.000" >"$fifo" 2>/dev/null &
adb_pid=$!
cert_logcat_filter <"$fifo" >"$out/marks.log" &
filter_pid=$!
cleanup() {
	kill "$adb_pid" 2>/dev/null || true
	wait "$filter_pid" 2>/dev/null || true
	rm -f "$fifo"
}
trap cleanup EXIT

press() {
	local number digit i
	if [ -z "$jump" ]; then
		cert_keyevent "$step_key"
		return
	fi
	# shellcheck disable=SC2086 # JUMP is a word list on purpose
	set -- $jump
	i=$((presses % $#))
	shift "$i"
	number=$1
	set --
	while [ -n "$number" ]; do
		digit=${number%"${number#?}"}
		number=${number#?}
		set -- "$@" "KEYCODE_$digit"
	done
	# One `input` call: a call per digit takes about a second each, longer than the app's 1.2 s
	# number-entry window. No OK key: the entry commits itself when that window closes (OK after
	# digits also opens the guide today, which would hide the switch and swallow later digits).
	cert_keyevent "$@"
}

total=$((minutes * 60 / interval))
presses=0
printf 'surfing %s on %s: %d keys, one every %ss\n' "${jump:+jump }${jump:-$keys}" "$package" "$total" "$interval" >&2
while [ "$presses" -lt "$total" ]; do
	press
	presses=$((presses + 1))
	sleep "$interval"
done
# Let the last tune reach its first frame before the capture stops.
sleep 3
cleanup
trap - EXIT

awk -v mode=surf -v json="$out/surf.json" -v codec_re="${CODEC_INIT_RE:-$default_codec_re}" \
	-f "$cert_dir/marks.awk" "$out/marks.log" | tee "$out/summary.txt"
printf 'report: %s/surf.json\n' "$out" >&2
