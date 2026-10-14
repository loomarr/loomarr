#!/usr/bin/env bash
# One state of the server resource pass (#1794): sample the whole process tree, profile the Go
# server, and write a Markdown report under .artifacts/perf/<date>/<state>/.
#
#   scripts/perf/server-pass.sh idle        nobody watching, filler jobs as they are (paused on a lane)
#   scripts/perf/server-pass.sh evening     a household evening: 1 steady + 1 surfing viewer, and the
#                                           filler jobs UNPAUSED for this window only (re-paused on exit)
#   scripts/perf/server-pass.sh load        VIEWERS steady viewers (default 4) + 1 surfing every SURF s
#                                           (default 15; SURF=0 for steady viewers only)
#   scripts/perf/server-pass.sh premium     1 viewer on the 4K HEVC HDR variant (CHANNELS=<the 4K channel>)
#
# Env: BASE (default this worktree's backend), DURATION seconds (idle 3600, evening 900, else 300),
# VIEWERS, CHANNELS (comma list; default every channel numbered 101+ — the demo library's),
# METRICS_TOKEN_FILE or LOOMARR_METRICS_TOKEN (the backend's /metrics credential).
# The backend must run with LOOMARR_PPROF=1 (/debug/pprof). Viewer runs take the GPU lock
# (flock /tmp/loomarr-gpu.lock) for exactly as long as encoders can run: the viewers plus drain.
#
# What each file is:
#   summary.md         the sampler's per-role table (CPU, PSS, disk, GPU), Go metrics, job wakes
#   cpu.md             CPU profile by subsystem and initiator (attribute.py)
#   allocs.md          what the window allocated, by subsystem (allocs diff over the window)
#   heap.md            what is live at the end, by subsystem
#   wall-*.md          wall-clock blocking (syscall, sync, net) from a 15 s execution trace: where
#                      the server WAITS, which a CPU profile cannot show
#   goroutines.txt     goroutine dump at the end (growth = compare runs)
#   viewers*.json      every tune: time to master, to first segment; stalls
#   drain.json         seconds from the last viewer fetch until encoders and sessions are gone
set -euo pipefail

state="${1:?usage: server-pass.sh idle|evening|load|premium}"
here="$(cd "$(dirname "$0")" && pwd)"
root="$(cd "$here/../.." && pwd)"
if [ -z "${BASE:-}" ]; then
	eval "$("$root/scripts/dev-env.sh" export)"
	BASE="http://localhost:$LOOMARR_DEV_PORT"
fi
case "$state" in
	idle) DURATION="${DURATION:-3600}" ;;
	evening) DURATION="${DURATION:-900}" ;;
	load | premium) DURATION="${DURATION:-300}" ;;
	*) echo "server-pass: unknown state $state" >&2; exit 2 ;;
esac
[ "$state" = evening ] && [ "$DURATION" -gt 1200 ] && { echo "server-pass: evening window capped at 1200 s (jobs unpaused)" >&2; exit 2; }
VIEWERS="${VIEWERS:-4}"
name="$state"
[ "$state" = load ] && name="load-$VIEWERS"
out="${OUT:-$root/.artifacts/perf/$(date +%F)/$name}"
mkdir -p "$out"
jar="$out/cookies.txt"
curl -fsS -c "$jar" -b "$jar" -H 'X-Loomarr-Csrf: 1' -H 'Content-Type: application/json' -d '{}' "$BASE/v1/auth/dev-login" -o /dev/null
api() { curl -fsS -c "$jar" -b "$jar" -H 'X-Loomarr-Csrf: 1' -H 'Content-Type: application/json' "$@"; }
if [ -z "${CHANNELS:-}" ]; then
	CHANNELS="$(api "$BASE/v1/channels" | python3 -c '
import json, sys
d = json.load(sys.stdin)
d = d.get("channels", d.get("items", d)) if isinstance(d, dict) else d
print(",".join(c["id"] for c in sorted(d, key=lambda c: c.get("number", 0)) if c.get("number", 0) >= 101))')"
fi
pprof() { curl -fsS "$BASE/debug/pprof/$1" -o "$out/$2"; }

filler_jobs="filler-pipeline filler-split-sweep filler-fetch channel-maintenance"
pause_jobs() {
	for j in $filler_jobs; do api -d "{\"paused\":$1}" "$BASE/v1/jobs/$j/pause" -o /dev/null || echo "server-pass: could not set $j paused=$1" >&2; done
}
if [ "$state" = evening ]; then
	trap 'pause_jobs true; echo "server-pass: filler jobs re-paused" >&2' EXIT
	trap 'exit 130' INT TERM
	pause_jobs false
fi

echo "server-pass: $state for ${DURATION}s → $out" >&2
{ echo "state=$state duration=$DURATION viewers=$VIEWERS channels=$CHANNELS"; date -Iseconds; uptime; } > "$out/run.txt"
pprof allocs allocs-start.pb.gz
python3 "$here/sample.py" --base "$BASE" --duration "$DURATION" --interval "${INTERVAL:-$([ "$DURATION" -ge 1800 ] && echo 30 || echo 5)}" \
	--out "$out" --label "$state" --db "$root/.agent-data/loomarr.db" --hls "$root/.agent-data/hls" > "$out/summary.md" &
sampler=$!

viewers_pid=
if [ "$state" != idle ]; then
	case "$state" in
		evening) steady=1 surf=120 ;;
		load) steady="$VIEWERS" surf="${SURF:-15}" ;;
		premium) steady=1 surf=0 ;;
	esac
	prem=; [ "$state" = premium ] && prem=--premium
	# One lock holder for every encode this run causes: the viewers, then the drain (grace sessions
	# still encode after the last fetch).
	# shellcheck disable=SC2016 # the inner sh expands its own positional parameters
	flock /tmp/loomarr-gpu.lock sh -c '
		here="$1"; base="$2"; dur="$3"; steady="$4"; surf="$5"; chans="$6"; out="$7"; prem="$8"
		python3 "$here/viewers.py" --base "$base" --dev-login --viewers "$steady" --duration "$dur" --channels "$chans" $prem --out "$out/viewers.json" 2>"$out/viewers.log" &
		a=$!
		b=
		if [ "$surf" -gt 0 ]; then
			python3 "$here/viewers.py" --base "$base" --dev-login --viewers 1 --surf "$surf" --duration "$dur" --channels "$chans" --out "$out/viewers-surf.json" 2>"$out/viewers-surf.log" &
			b=$!
		fi
		wait $a $b
		last=$(python3 -c "import json,sys; print(max(json.load(open(f)).get(\"last_fetch_epoch\",0) for f in sys.argv[1:]))" "$out"/viewers*.json)
		python3 "$here/drain.py" --base "$base" --since "$last" --out "$out/drain.json" > "$out/drain.log"
	' _ "$here" "$BASE" "$DURATION" "$steady" "$surf" "$CHANNELS" "$out" "$prem" &
	viewers_pid=$!
fi

# Profile the middle of the window, once things have settled.
warm=$((DURATION / 4 > 60 ? 60 : DURATION / 4))
sleep "$warm"
prof_s="${PROFILE_S:-$((DURATION - warm - 30 > 120 ? 120 : DURATION - warm - 30))}"
pprof "profile?seconds=$prof_s" cpu.pb.gz
pprof "trace?seconds=15" trace.out
wait "$sampler"
pprof allocs allocs-end.pb.gz
pprof heap heap.pb.gz
pprof "goroutine?debug=1" goroutines.txt
[ -n "$viewers_pid" ] && wait "$viewers_pid"

python3 "$here/attribute.py" "$out/cpu.pb.gz" > "$out/cpu.md"
python3 "$here/attribute.py" "$out/allocs-end.pb.gz" --base "$out/allocs-start.pb.gz" --index alloc_space > "$out/allocs.md"
python3 "$here/attribute.py" "$out/heap.pb.gz" --index inuse_space > "$out/heap.md"
for kind in syscall sync net; do
	go tool trace -pprof="$kind" "$out/trace.out" > "$out/wall-$kind.pb" 2>/dev/null &&
		python3 "$here/attribute.py" "$out/wall-$kind.pb" > "$out/wall-$kind.md" 2>/dev/null || echo "(no $kind blocking in the trace)" > "$out/wall-$kind.md"
done
{
	echo "# $state — $(date '+%F %H:%M %Z')"
	cat "$out/summary.md" "$out/cpu.md" "$out/allocs.md" "$out/heap.md"
	for kind in syscall sync net; do echo "## wall-clock: $kind"; cat "$out/wall-$kind.md"; done
	echo "## goroutines by first non-runtime frame"
	python3 - "$out/goroutines.txt" <<-'PY'
		import re, sys
		counts, n = {}, 0
		for block in open(sys.argv[1]).read().split("\n\n"):
		    m = re.match(r"(\d+) @", block)
		    if not m:
		        continue
		    frames = re.findall(r"^#\s+0x[0-9a-f]+\s+(\S+)", block, re.M)
		    top = next((f for f in frames if not f.startswith(("runtime.", "internal/", "sync.", "time.Sleep", "context.", "net.", "os/signal"))), frames[0] if frames else "?")
		    counts[re.sub(r"\+0x[0-9a-f]+$", "", top)] = counts.get(re.sub(r"\+0x[0-9a-f]+$", "", top), 0) + int(m.group(1))
		    n += int(m.group(1))
		print(f"total {n}")
		for k, v in sorted(counts.items(), key=lambda kv: -kv[1])[:15]:
		    print(f"- {v} {k}")
	PY
	for f in "$out"/viewers*.json "$out"/drain.json; do [ -f "$f" ] && { echo "## $(basename "$f")"; python3 -c "import json,sys; d=json.load(open(sys.argv[1])); d.pop('tune_log',None); d.pop('events',None); print(json.dumps(d))" "$f"; }; done
} > "$out/report.md"
echo "server-pass: report $out/report.md" >&2
