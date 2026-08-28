#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SELECTOR="$ROOT/scripts/ci-lane.sh"

assert_lane() {
  local event="$1" scope="$2" want="$3" got
  got="$($SELECTOR "$event" "$scope")"
  if [[ "$got" != "lane=$want" ]]; then
    printf 'ci-lane-test: %s/%s: got %s, want lane=%s\n' "$event" "$scope" "$got" "$want" >&2
    exit 1
  fi
}

assert_rejected() {
  if "$SELECTOR" "$@" >/dev/null 2>&1; then
    printf 'ci-lane-test: unexpectedly accepted: %s\n' "$*" >&2
    exit 1
  fi
}

assert_lane pull_request '' pr-fast
assert_lane merge_group '' merge-queue
assert_lane workflow_dispatch release-candidate manual-release-candidate
assert_lane workflow_dispatch full manual-full

assert_rejected push
assert_rejected pull_request full
assert_rejected merge_group release-candidate
assert_rejected workflow_dispatch
assert_rejected workflow_dispatch unknown
assert_rejected pull_request '' extra

echo 'ci-lane-test: ok'
