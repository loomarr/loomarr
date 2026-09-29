#!/usr/bin/env bash
# Playback soak certification (#1037, #1512 G4): keeps the TV app playing for HOURS, steps to the
# next channel every ROTATE_MINUTES, and counts stalls per viewer-hour and decoder
# re-instantiations mid-play (a programme <-> commercial boundary must not re-init the codec).
# Survives adb disconnects (network or USB) and app restarts; rewrites the report every
# REPORT_MINUTES so an interrupted run still leaves one. Ctrl-C stops early and reports; a run
# started in the background (nohup, &) begins with SIGINT ignored, so stop it with the
# `kill -TERM <pid>` it prints at start.
#
#   ADB_SERIAL=<serial> scripts/shield-cert/soak.sh
#
# Environment:
#   ADB_SERIAL      required; the device to drive
#   HOURS           run length (default 24, the gate's length)
#   MINUTES         a shorter run for trying the script; the report still judges against HOURS
#   ROTATE_MINUTES  minutes on each channel before CHANNEL_UP (default 30; 0 never rotates)
#   KEYS            channel (default) or dpad, as in surf.sh
#   REPORT_MINUTES  report refresh period (default 10)
#   OUT             report directory (default .artifacts/shield-cert/soak-<device time>)
#   LAUNCH          1 (default) relaunches the app whenever it is not in the foreground
#   PACKAGE, CODEC_INIT_RE  as in surf.sh / lib.sh
#
# The capture keeps only LoomarrCert marks and codec lines: never a URL or a title.
set -euo pipefail

# shellcheck source=scripts/shield-cert/lib.sh
. "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

hours=${HOURS:-24}
rotate=${ROTATE_MINUTES:-30}
report_every=${REPORT_MINUTES:-10}
minutes=${MINUTES:-$((hours * 60))}
case "$hours$minutes$rotate$report_every" in
*[!0-9]*) cert_die 'HOURS, MINUTES, ROTATE_MINUTES and REPORT_MINUTES are whole numbers' 2 ;;
esac
case "${KEYS:-channel}" in
channel) step_key=KEYCODE_CHANNEL_UP ;;
dpad) step_key=KEYCODE_DPAD_UP ;;
*) cert_die 'KEYS must be channel or dpad' 2 ;;
esac

cert_require_serial
cert_online || cert_die "device $ADB_SERIAL is not online"
package=$(cert_package)
started=$(cert_device_epoch)
out=${OUT:-.artifacts/shield-cert/soak-$started}
mkdir -p "$out"
marks="$out/marks.log"
: >"$marks"
fifo="$out/.logcat.fifo"
codec_re=${CODEC_INIT_RE:-$default_codec_re}

deadline=$((started + minutes * 60))
reconnects=0
relaunches=0
adb_pid=
filter_pid=
since=$started

stop_capture() {
	if [ -n "$adb_pid" ]; then kill "$adb_pid" 2>/dev/null || true; fi
	if [ -n "$filter_pid" ]; then wait "$filter_pid" 2>/dev/null || true; fi
	adb_pid=
	filter_pid=
	rm -f "$fifo"
}

# Resumes the capture after the newest line already kept, so a reconnect neither drops nor repeats.
start_capture() {
	local last
	last=$(awk 'END { if (NR) print $1 }' "$marks")
	if [ -n "$last" ]; then since=${last%%.*}; fi
	rm -f "$fifo"
	mkfifo "$fifo"
	adb -s "$ADB_SERIAL" logcat -v epoch -T "$since.000" >"$fifo" 2>/dev/null &
	adb_pid=$!
	awk -v after="${last:-0}" '$1 + 0 > after + 0 { print; fflush() }' <"$fifo" | cert_logcat_filter >>"$marks" &
	filter_pid=$!
}

report() {
	local now
	now=$(cert_device_epoch 2>/dev/null || true)
	case "$now" in '' | *[!0-9]*) now=$(awk 'END { if (NR) print int($1) }' "$marks") ;; esac
	awk -v mode=soak -v json="$out/soak.json" -v codec_re="$codec_re" -v end_epoch="${now:-$started}" \
		-v reconnects="$reconnects" -v target_h="$hours" \
		-f "$cert_dir/marks.awk" "$marks" >"$out/summary.txt"
}

reattach() {
	stop_capture
	reconnects=$((reconnects + 1))
	printf '%s: device %s dropped; waiting (reconnect %d)\n' "$(date '+%H:%M:%S')" "$ADB_SERIAL" "$reconnects" >&2
	until cert_online; do
		cert_wait_for_device >/dev/null 2>&1 &
		local waiter=$!
		sleep 10
		kill "$waiter" 2>/dev/null || true
	done
	start_capture
}

finish() {
	stop_capture
	report
	cat "$out/summary.txt"
	printf 'relaunches %d; report: %s/soak.json\n' "$relaunches" "$out" >&2
}
trap 'finish; exit 130' INT TERM

if [ "${LAUNCH:-1}" = 1 ]; then cert_launch "$package"; fi
start_capture
printf 'soaking %s on %s for %s min, rotating every %s min (stop early: kill -TERM %d)\n' "$package" "$ADB_SERIAL" "$minutes" "$rotate" "$$" >&2

next_rotate=$((started + rotate * 60))
next_report=$((started + report_every * 60))
while :; do
	# Waited on rather than run in the foreground, so a TERM reaches the trap at once.
	sleep 30 &
	wait "$!" || true
	if ! cert_online || ! kill -0 "$adb_pid" 2>/dev/null; then reattach; fi
	now=$(cert_device_epoch 2>/dev/null || true)
	case "$now" in '' | *[!0-9]*) continue ;; esac
	[ "$now" -lt "$deadline" ] || break
	if [ "${LAUNCH:-1}" = 1 ] && ! cert_adb shell dumpsys activity activities 2>/dev/null | grep -Eq "ResumedActivity.*$package/"; then
		relaunches=$((relaunches + 1))
		cert_launch "$package" || true
	fi
	if [ "$rotate" -gt 0 ] && [ "$now" -ge "$next_rotate" ]; then
		cert_keyevent "$step_key" || true
		next_rotate=$((now + rotate * 60))
	fi
	if [ "$now" -ge "$next_report" ]; then
		report || true
		next_report=$((now + report_every * 60))
	fi
done
trap - INT TERM
finish
