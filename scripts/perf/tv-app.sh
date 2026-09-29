#!/usr/bin/env bash
# TV app resource pass (#1794): JS-thread CPU while surfing, overlay frame cost, and memory over a
# long watch, on an Android TV device or emulator over adb. The app must be paired and on Watching.
#
#   ADB_SERIAL=emulator-5584 MODE=surf MINUTES=3 INTERVAL=5 scripts/perf/tv-app.sh
#   ADB_SERIAL=emulator-5584 MODE=soak MINUTES=30 scripts/perf/tv-app.sh
#
# surf: presses CHANNEL_UP every INTERVAL seconds. A sampler on the device reads the utime+stime of
#       the React Native JS thread (mqt_js / mqt_v_js), the UI thread and RenderThread every 250 ms,
#       so each press gets its JS CPU in the WINDOW seconds after it (default 1.25, the Shield trace's
#       tune window) and per interval. `dumpsys gfxinfo` is reset first and read last: frame time
#       percentiles and janky frames while the switch overlay animates.
# soak: no keys; every 60 s `dumpsys meminfo` (total PSS, Java heap, native heap, graphics) and the
#       threads' CPU per minute. Growth over the run is the leak signal.
#
# Thread CPU is read from /proc on the device, so it measures running time only, like a CPU
# profile. Writes OUT (default .artifacts/perf/<date>/tv-<mode>) and prints Markdown.
set -euo pipefail

: "${ADB_SERIAL:?ADB_SERIAL is required (adb devices -l)}"
MODE="${MODE:-surf}"
MINUTES="${MINUTES:-3}"
INTERVAL="${INTERVAL:-5}"
WINDOW="${WINDOW:-1.25}"
PACKAGE="${PACKAGE:-loomarr.media}"
root="$(cd "$(dirname "$0")/../.." && pwd)"
OUT="${OUT:-$root/.artifacts/perf/$(date +%F)/tv-$MODE}"
mkdir -p "$OUT"
adb_() { adb -s "$ADB_SERIAL" "$@"; }

pid="$(adb_ shell pidof "$PACKAGE" | tr -d '\r')"
[ -n "$pid" ] || { echo "tv-app: $PACKAGE is not running on $ADB_SERIAL" >&2; exit 1; }
# Thread ids by name: the JS thread, the UI (main) thread, and the RenderThread that draws frames.
threads="$(adb_ shell "for t in /proc/$pid/task/*; do echo \${t##*/} \$(cat \$t/comm); done" | tr -d '\r')"
js="$(echo "$threads" | awk '$2 ~ /^mqt_(v_)?js$/ {print $1; exit}')"
render="$(echo "$threads" | awk '$2 == "RenderThread" {print $1; exit}')"
[ -n "$js" ] || { echo "tv-app: no mqt_js thread in $PACKAGE ($pid)" >&2; exit 1; }
echo "tv-app: $MODE on $ADB_SERIAL, $PACKAGE pid $pid, js $js, ui $pid, render ${render:-none}, $(date -Iseconds)" | tee "$OUT/run.txt" >&2

# On-device sampler: uptime (s), then utime+stime ticks of js, ui, render. Stops by itself.
secs=$((MINUTES * 60 + 5))
sampler="end=\$(( \$(cut -d. -f1 /proc/uptime) + $secs ));
while [ \$(cut -d. -f1 /proc/uptime) -lt \$end ]; do
  u=\$(cut -d' ' -f1 /proc/uptime); l=\$u
  for t in $js $pid ${render:-$pid}; do s=\$(cat /proc/$pid/task/\$t/stat 2>/dev/null); s=\${s##*) }; set -- \$s; l=\"\$l \$((\${12} + \${13}))\"; done
  echo \$l; sleep 0.25
done"
adb_ shell "$sampler" | tr -d '\r' > "$OUT/threads.txt" &
sampler_pid=$!
hz=100 # USER_HZ on Android

if [ "$MODE" = surf ]; then
	adb_ shell dumpsys gfxinfo "$PACKAGE" reset > /dev/null
	: > "$OUT/presses.txt"
	end=$(($(date +%s) + MINUTES * 60))
	while [ "$(date +%s)" -lt "$end" ]; do
		adb_ shell "cut -d' ' -f1 /proc/uptime; input keyevent KEYCODE_CHANNEL_UP" | head -1 | tr -d '\r' >> "$OUT/presses.txt"
		sleep "$INTERVAL"
	done
	adb_ shell dumpsys gfxinfo "$PACKAGE" > "$OUT/gfxinfo.txt"
	wait "$sampler_pid" || true
	echo "### TV surf — $(date '+%F %H:%M %Z'), CHANNEL_UP every ${INTERVAL}s for ${MINUTES} min"
	awk -v w="$WINDOW" -v hz="$hz" -v iv="$INTERVAL" '
		FNR == NR { press[++np] = $1; next }
		{ t[++n] = $1; js[n] = $2; ui[n] = $3; rt[n] = $4 }
		function at(arr, x,   i) { for (i = 1; i <= n; i++) if (t[i] >= x) return arr[i]; return arr[n] }
		END {
			for (p = 1; p <= np; p++) {
				a = press[p]
				wj = (at(js, a + w) - at(js, a)) / hz * 1000
				ij = (at(js, a + iv) - at(js, a)) / hz * 1000
				wu = (at(ui, a + w) - at(ui, a)) / hz * 1000
				wr = (at(rt, a + w) - at(rt, a)) / hz * 1000
				sj += wj; si += ij; su += wu; sr += wr
				if (wj > mj) mj = wj
				v[p] = wj
			}
			asort(v)
			i50 = int(np * 0.5) + 1
			i95 = int(np * 0.95) + 1
			if (i95 > np) i95 = np
			printf "\n| presses | JS ms in %.2fs window, mean | p50 | p95 | max | JS ms per %ss interval | UI ms in window | RenderThread ms in window |\n", w, iv
			print "|---|---|---|---|---|---|---|---|"
			printf "| %d | %.0f | %.0f | %.0f | %.0f | %.0f | %.0f | %.0f |\n", np, sj/np, v[i50], v[i95], mj, si/np, su/np, sr/np
		}' "$OUT/presses.txt" "$OUT/threads.txt"
	echo
	echo "gfxinfo while surfing:"
	grep -E "Total frames rendered|Janky frames|percentile|Number (Missed Vsync|Slow UI thread|Slow bitmap|Slow issue draw|Frame deadline missed)" "$OUT/gfxinfo.txt" | head -12 | sed 's/^/- /'
else
	echo "min,total_pss_kb,java_heap_kb,native_heap_kb,graphics_kb" > "$OUT/meminfo.csv"
	for m in $(seq 0 "$MINUTES"); do
		adb_ shell dumpsys meminfo "$PACKAGE" | tr -d '\r' > "$OUT/meminfo-$m.txt"
		awk -v m="$m" '
			/^ *Java Heap:/ { j = $3 } /^ *Native Heap:/ { nh = $3 } /^ *Graphics:/ { g = $2 }
			/^ *TOTAL PSS:/ { tp = $3 } /^ *TOTAL:/ && !tp { tp = $2 }
			END { print m "," tp "," j "," nh "," g }' "$OUT/meminfo-$m.txt" >> "$OUT/meminfo.csv"
		[ "$m" -lt "$MINUTES" ] && sleep 60
	done
	wait "$sampler_pid" || true
	echo "### TV soak — $(date '+%F %H:%M %Z'), ${MINUTES} min on one channel"
	echo
	echo "| min | total PSS MB | Java heap MB | native heap MB | graphics MB |"
	echo "|---|---|---|---|---|"
	awk -F, 'NR > 1 && (($1 % 5) == 0) { printf "| %s | %.0f | %.0f | %.0f | %.0f |\n", $1, $2/1000, $3/1000, $4/1000, $5/1000 }' "$OUT/meminfo.csv"
	awk -v hz="$hz" 'NR == 1 { t0 = $1; j0 = $2; u0 = $3; r0 = $4 } { t = $1; j = $2; u = $3; r = $4 }
		END { d = t - t0; if (d > 0) printf "\nThread CPU over the soak: JS %.1f%%, UI %.1f%%, RenderThread %.1f%% of one core\n", (j-j0)/hz/d*100, (u-u0)/hz/d*100, (r-r0)/hz/d*100 }' "$OUT/threads.txt"
fi
