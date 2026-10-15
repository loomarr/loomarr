#!/usr/bin/env bash
# Hardware playout bench coverage (#1566). A skipped hardware leg must be visible, not green by omission.
#
#   record <artifact-dir>  run by each leg: sets the step output reported=true when the bench wrote its
#                          playout-bench-<family>.json report, reported=false otherwise.
#   summarise              run by the coverage job: one line per leg in the job summary. It fails only
#                          when a leg whose gate is set (or that ran anyway) did not pass or did not
#                          report. A leg skipped because its gate is unset is a notice, not a failure, so a
#                          homelab without a paid T4 runner or a self-hosted GPU runner stays green.
#
# summarise reads PLAYOUT_BENCH_NEEDS (the workflow's toJSON(needs)) and PLAYOUT_BENCH_EXPECT_<LEG>,
# which playout-bench.yml sets to the same expression as that leg's `if:`.
set -euo pipefail

usage() {
  printf 'usage: %s record <artifact-dir> | summarise\n' "$0" >&2
  exit 2
}

record() {
  local dir="$1" reported=false
  if compgen -G "$dir/playout-bench-*.json" >/dev/null; then
    reported=true
  fi
  echo "reported=$reported" >>"${GITHUB_OUTPUT:?GITHUB_OUTPUT is required}"
  echo "playout bench report present: $reported"
}

summarise() {
  : "${PLAYOUT_BENCH_NEEDS:?PLAYOUT_BENCH_NEEDS is required}"
  local summary="${GITHUB_STEP_SUMMARY:-/dev/stdout}" failed=0 ran=0
  # job id | expectation variable | label | why it is skipped when its gate is unset
  local legs=(
    "macos|PLAYOUT_BENCH_EXPECT_MACOS|VideoToolbox leg (macos-15)|run_macos not requested"
    "gpu-t4|PLAYOUT_BENCH_EXPECT_T4|NVENC leg (GitHub T4)|PLAYOUT_BENCH_T4_RUNNER unset"
    "arc|PLAYOUT_BENCH_EXPECT_ARC|VAAPI leg (self-hosted Intel Arc)|dispatch-only and run_arc not requested"
    "geforce|PLAYOUT_BENCH_EXPECT_GEFORCE|NVENC leg (self-hosted NVIDIA GeForce)|dispatch-only and run_geforce not requested"
  )
  {
    echo "## Hardware bench coverage"
    echo
  } >>"$summary"
  local leg job variable label reason expected result reported line
  for leg in "${legs[@]}"; do
    IFS='|' read -r job variable label reason <<<"$leg"
    expected="${!variable:?$variable is required}"
    result="$(jq -r --arg job "$job" '.[$job].result // "missing"' <<<"$PLAYOUT_BENCH_NEEDS")"
    reported="$(jq -r --arg job "$job" '.[$job].outputs.reported // "false"' <<<"$PLAYOUT_BENCH_NEEDS")"
    if [[ "$expected" != "true" && "$result" == "skipped" ]]; then
      line="$label skipped: $reason"
      echo "::notice title=Playout bench coverage::$line"
    elif [[ "$result" == "success" && "$reported" == "true" ]]; then
      line="$label ran and reported"
      ran=$((ran + 1))
    elif [[ "$result" == "success" ]]; then
      line="$label passed but wrote no bench report"
      echo "::error title=Playout bench coverage::$line"
      failed=1
    else
      line="$label is configured but did not pass (result: $result)"
      echo "::error title=Playout bench coverage::$line"
      failed=1
    fi
    echo "- $line" >>"$summary"
  done
  {
    echo
    echo "$ran of ${#legs[@]} hardware legs ran."
  } >>"$summary"
  return "$failed"
}

case "${1:-}" in
  record)
    (($# == 2)) || usage
    record "$2"
    ;;
  summarise)
    (($# == 1)) || usage
    summarise
    ;;
  *) usage ;;
esac
