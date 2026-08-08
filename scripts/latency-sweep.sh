#!/usr/bin/env bash
# Latency sweep — rank the API surface by p99, warm AND cold, before optimizing anything.
#
# The workflow this implements, and why each step is in it:
#
#   1. BASELINE WITH PERCENTILES, warm and cold separately. A mean hides the shape that
#      matters: GET /v1/guide once measured ~100ms median against ~450ms p99, and the two
#      numbers had entirely different causes (arrangement CPU vs. an N+1 of 25 HTTP calls).
#      Optimizing against the mean would have found neither.
#   2. READ THE FAN-OUT the run produced (loomarr_http_outbound_fanout, metrics/fanout.go).
#      This distinguishes "does too much work" from "does the same small work N times", which
#      no latency number can, and which a CPU profile actively hides — an N+1 against a remote
#      service is I/O, so the goroutine is parked and produces zero samples.
#   3. ONLY THEN PROFILE, choosing the profile type by the samples/wall-clock ratio this
#      script prints. Well under 1x parallelism means the process is WAITING, and a CPU
#      profile is the wrong instrument.
#
# Deliberately NOT a gate. It needs a running server with real data behind it, and its numbers
# are machine- and dataset-dependent — a CI threshold here would either be so loose it never
# fires or so tight it fails on an unrelated machine. It is an investigation tool; the things
# worth pinning permanently become tests (see TestFanout_*, TestPrewarmDurations_*).
#
#   scripts/latency-sweep.sh                  sweep the default routes
#   scripts/latency-sweep.sh /v1/guide        sweep only routes matching a substring
#   BASE=http://host:8080 scripts/…           point at another install
#   N=50 scripts/latency-sweep.sh             more samples per route (default 20)
set -euo pipefail

BASE="${BASE:-http://localhost:8080}"
N="${N:-20}"
FILTER="${1:-}"

# The routes worth sweeping: every GET on the read path a user actually waits on. Write and
# destructive endpoints are deliberately absent — this runs against a live install, and a sweep
# that POSTs would mutate the data it is measuring.
#
# `now` and `win` are substituted per-request so guide windows land on real time rather than a
# frozen literal that would drift into a different cache bucket as the file ages.
ROUTES=(
  "/v1/system/version"
  "/v1/channels"
  "/v1/channels/now-next"
  "/v1/guide?from={now-30m}&to={now+4h}"
  "/v1/titles"
  "/v1/jobs"
  "/v1/filler"
  "/v1/settings"
  "/v1/proposals"
  "/v1/users"
  "/v1/setup/state"
  "/v1/programming/vocabulary"
)

command -v curl >/dev/null || { echo "curl is required" >&2; exit 1; }
command -v python3 >/dev/null || { echo "python3 is required (percentiles)" >&2; exit 1; }

now_ms() { python3 -c 'import time; print(int(time.time()*1000))'; }

# expand substitutes the {now±N} placeholders so each sample asks about the present.
expand() {
  local url="$1" now; now="$(now_ms)"
  url="${url//\{now-30m\}/$((now - 1800000))}"
  url="${url//\{now+4h\}/$((now + 14400000))}"
  echo "$url"
}

# fanout_total reads the fan-out histogram's running sum for a route, so the sweep can report
# calls-per-request without serialising traffic. Absent (older binary, route never hit) ⇒ blank.
fanout_sum() {
  local route="$1"
  curl -fsS "$BASE/metrics" 2>/dev/null \
    | awk -v r="$route" '$0 ~ "loomarr_http_outbound_fanout_sum" && $0 ~ r {print $NF; exit}'
}

percentiles() { # reads whitespace-separated ms on stdin
  python3 -c '
import sys
xs = sorted(float(x) for x in sys.stdin.read().split())
if not xs:
    print("       -       -       -       -"); raise SystemExit
def pct(p):
    if len(xs) == 1: return xs[0]
    i = (len(xs) - 1) * p / 100
    lo, hi = int(i), min(int(i) + 1, len(xs) - 1)
    return xs[lo] + (xs[hi] - xs[lo]) * (i - lo)
print(f"{pct(50):8.1f}{pct(95):8.1f}{pct(99):8.1f}{xs[-1]:8.1f}")'
}

sweep() { # $1 = label, $2 = 1 to force a cold sample first
  local label="$1" cold="$2"
  printf '\n=== %s ===\n' "$label"
  printf '%-46s %7s %7s %7s %7s %8s\n' ROUTE p50 p95 p99 max fanout

  for route in "${ROUTES[@]}"; do
    [ -n "$FILTER" ] && [[ "$route" != *"$FILTER"* ]] && continue

    local path="${route%%\?*}"
    local before after fan=""
    before="$(fanout_sum "$path" || true)"

    # A cold sweep pauses first so anything with a short TTL (the arranged-cycle cache is 60s,
    # the availability memo 5s) has genuinely expired. Otherwise "cold" measures a warm cache
    # and reports a flattering number for the case users actually feel.
    if [ "$cold" = "1" ]; then sleep "${COLD_GAP:-7}"; fi

    local samples=""
    local reps="$N"
    [ "$cold" = "1" ] && reps=1   # a cold measurement is only cold once

    for _ in $(seq 1 "$reps"); do
      local url; url="$(expand "$route")"
      local t
      t="$(curl -fsS -o /dev/null -w '%{time_total}' "$BASE$url" 2>/dev/null || echo "")"
      [ -n "$t" ] && samples+=" $(python3 -c "print(float('$t')*1000)")"
    done

    after="$(fanout_sum "$path" || true)"
    if [ -n "$before" ] && [ -n "$after" ] && [ "$reps" -gt 0 ]; then
      fan="$(python3 -c "print(f'{(float('$after')-float('$before'))/$reps:.1f}')" 2>/dev/null || echo "")"
    fi

    if [ -z "$samples" ]; then
      printf '%-46s %s\n' "${route:0:46}" "     (unreachable or non-2xx)"
    else
      printf '%-46s %s %7s\n' "${route:0:46}" "$(echo "$samples" | percentiles)" "${fan:--}"
    fi
  done
}

echo "Latency sweep against $BASE  (n=$N per route)"
curl -fsS -o /dev/null "$BASE/v1/system/version" 2>/dev/null \
  || { echo "server not reachable at $BASE" >&2; exit 1; }

# COLD first: a warm sweep would populate every cache the cold one is trying to measure.
sweep "COLD (first hit, caches expired)" 1
sweep "WARM (steady state, n=$N)" 0

cat <<'NEXT'

Reading this:
  · p99 >> p50            one input class is slow — look for a cache miss or an N+1,
                          not a uniformly slow handler.
  · fanout > ~2           the route makes a downstream call per item. That is the N+1
                          shape; batching beats micro-optimizing every time.
  · COLD >> WARM          a cache is carrying the warm number. Ask whether the cold
                          path is the one users actually hit (it usually is, on arrival).

Then, and only then, profile the worst offender:
  curl -o cpu.pprof "$BASE/debug/pprof/profile?seconds=10"   # needs LOOMARR_PPROF=1
  go tool pprof -top -cum cpu.pprof

  ⚠ Check the header: "Duration: 10s, Total samples = 2.1s (21%)". If samples are far
    below duration x parallelism, the process is WAITING, not computing, and the CPU
    profile will point at whatever it does between waits. Use the goroutine dump instead:
      curl "$BASE/debug/pprof/goroutine?debug=2"
    That gap is what hid a 25-call N+1 behind a profile that blamed the scheduler.
NEXT
