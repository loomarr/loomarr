#!/usr/bin/env bash
# Run the complete local Go suite or one protected CI lane behind the `make test` interface.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

lane="${GO_TEST_LANE:-}"
sharder="${GO_TEST_SHARDER:-$ROOT/scripts/go-shard.sh}"
race_policy="${GO_TEST_RACE_POLICY:-$ROOT/scripts/go-race-policy.sh}"
package_runner="${GO_TEST_PACKAGE_RUNNER:-$ROOT/scripts/go-test-packages.sh}"
make_bin="${GO_TEST_MAKE_BIN:-make}"

case "$lane" in
  "")
    shard_args=()
    lane_flags="${GOFLAGS:-}"
    lane_label="unsharded"
    "$make_bin" -C "$ROOT" rust-test-worker eval-contract
    ;;
  certification-[12]/2)
    if [[ -n "${GOFLAGS:-}" ]]; then
      echo "go-test-lane: lane-scoped GOFLAGS are owned by the runner" >&2
      exit 2
    fi
    shard_args=(--certification "${lane#certification-}")
    lane_flags="-p=1"
    lane_label="$lane"
    ;;
  [1-2]/2)
    if [[ -n "${GOFLAGS:-}" ]]; then
      echo "go-test-lane: lane-scoped GOFLAGS are owned by the runner" >&2
      exit 2
    fi
    shard_args=("$lane")
    lane_flags="-p=4"
    lane_label="$lane"
    ;;
  *)
    echo "go-test-lane: invalid lane '$lane' (want 1/2, 2/2, certification-1/2, certification-2/2, or empty for the full local suite)" >&2
    exit 2
    ;;
esac

if [[ -z "$lane" ]]; then
  packages="$($sharder)"
else
  packages="$($sharder "${shard_args[@]}")"
fi
if [[ -z "$packages" ]]; then
  echo "go-test-lane: lane '$lane_label' contains no packages" >&2
  exit 2
fi

race_packages="$(printf '%s\n' "$packages" | "$race_policy" --race)"
plain_packages="$(printf '%s\n' "$packages" | "$race_policy" --no-race)"

run_packages() {
  local mode="$1" package_list="$2" package
  [[ -n "$package_list" ]] || return 0
  local package_args=()
  while IFS= read -r package; do
    [[ -n "$package" ]] && package_args+=("$package")
  done <<< "$package_list"
  GOFLAGS="$lane_flags" GO_BIN="${GO_BIN:-go}" GO_TEST_LANE="$lane_label" \
    "$package_runner" "$mode" 25m "${package_args[@]}"
}

run_packages race "$race_packages"
run_packages plain "$plain_packages"
