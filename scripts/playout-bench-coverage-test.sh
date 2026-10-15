#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="$ROOT/scripts/playout-bench-coverage.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

fail() {
  printf 'playout-bench-coverage-test: %s\n' "$*" >&2
  exit 1
}

# needs <macos> <gpu-t4> <arc> <geforce>, each "result:reported".
needs() {
  local jobs=(macos gpu-t4 arc geforce) legs=("$@") json='{' separator='' i result reported
  for i in 0 1 2 3; do
    IFS=: read -r result reported <<<"${legs[i]}"
    json+="$separator\"${jobs[i]}\":{\"result\":\"$result\",\"outputs\":{\"reported\":\"$reported\"}}"
    separator=,
  done
  echo "$json}"
}

# summarise <expect macos t4 arc geforce> <needs-json>; prints the summary, returns the script's status.
summarise() {
  local summary="$work/summary.md"
  : >"$summary"
  local status=0
  PLAYOUT_BENCH_EXPECT_MACOS="$1" PLAYOUT_BENCH_EXPECT_T4="$2" \
    PLAYOUT_BENCH_EXPECT_ARC="$3" PLAYOUT_BENCH_EXPECT_GEFORCE="$4" \
    PLAYOUT_BENCH_NEEDS="$5" GITHUB_STEP_SUMMARY="$summary" \
    "$SCRIPT" summarise >"$work/stdout" 2>&1 || status=$?
  cat "$summary" "$work/stdout"
  return "$status"
}

expect_line() {
  grep -Fq -- "$1" <<<"$2" || fail "missing '$1' in:"$'\n'"$2"
}

# A scheduled run on a homelab with no T4 and no self-hosted GPU runner is green, and says so.
out="$(summarise true false false false "$(needs success:true skipped: skipped: skipped:)")" ||
  fail "unconfigured legs must not fail coverage:"$'\n'"$out"
expect_line '- VideoToolbox leg (macos-15) ran and reported' "$out"
expect_line '- NVENC leg (GitHub T4) skipped: PLAYOUT_BENCH_T4_RUNNER unset' "$out"
expect_line '::notice title=Playout bench coverage::NVENC leg (GitHub T4) skipped: PLAYOUT_BENCH_T4_RUNNER unset' "$out"
expect_line '- VAAPI leg (self-hosted Intel Arc) skipped: dispatch-only and run_arc not requested' "$out"
expect_line '- NVENC leg (self-hosted NVIDIA GeForce) skipped: dispatch-only and run_geforce not requested' "$out"
expect_line '1 of 4 hardware legs ran.' "$out"

# A configured leg that was skipped anyway (for example its runner never picked it up) is red.
if out="$(summarise true true false false "$(needs success:true skipped: skipped: skipped:)")"; then
  fail "a configured leg that did not run must fail coverage:"$'\n'"$out"
fi
expect_line '- NVENC leg (GitHub T4) is configured but did not pass (result: skipped)' "$out"
expect_line '::error title=Playout bench coverage::' "$out"

# A leg that passed without writing its report is red: it measured nothing.
if out="$(summarise true false true false "$(needs success:true skipped: success:false skipped:)")"; then
  fail "a leg without a report must fail coverage:"$'\n'"$out"
fi
expect_line '- VAAPI leg (self-hosted Intel Arc) passed but wrote no bench report' "$out"

# A failed requested leg is red, and every requested leg that passed is counted.
if out="$(summarise true true true true "$(needs success:true success:true success:true failure:true)")"; then
  fail "a failed leg must fail coverage:"$'\n'"$out"
fi
expect_line '- NVENC leg (self-hosted NVIDIA GeForce) is configured but did not pass (result: failure)' "$out"
expect_line '3 of 4 hardware legs ran.' "$out"

# record reports whether the bench wrote its JSON report.
mkdir -p "$work/artifacts"
GITHUB_OUTPUT="$work/output" "$SCRIPT" record "$work/artifacts" >/dev/null
expect_line 'reported=false' "$(cat "$work/output")"
touch "$work/artifacts/playout-bench-vaapi.json"
: >"$work/output"
GITHUB_OUTPUT="$work/output" "$SCRIPT" record "$work/artifacts" >/dev/null
expect_line 'reported=true' "$(cat "$work/output")"

echo 'playout-bench-coverage-test: ok'
