#!/usr/bin/env bash
# Run one package subset and publish the slowest package timings to the GitHub step summary.
set -euo pipefail

mode="${1:-}"
timeout="${2:-}"
shift $(( $# >= 2 ? 2 : $# ))

if [[ "$mode" != "race" && "$mode" != "plain" ]] || [[ -z "$timeout" ]] || [[ "$#" -eq 0 ]]; then
	echo "usage: go-test-packages.sh race|plain TIMEOUT PACKAGE..." >&2
	exit 2
fi

go_bin="${GO_BIN:-go}"
log="$(mktemp "${TMPDIR:-/tmp}/loomarr-go-test.XXXXXX")"
trap 'rm -f "$log"' EXIT

args=(test -timeout "$timeout")
label="non-race"
if [[ "$mode" == "race" ]]; then
	args+=(-race)
	label="race"
fi

set +e
"$go_bin" "${args[@]}" "$@" 2>&1 | tee "$log"
status="${PIPESTATUS[0]}"
set -e

if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
	timings="$(awk '
		$1 == "ok" && $NF ~ /^[0-9.]+s$/ {
			seconds = $NF
			sub(/s$/, "", seconds)
			print seconds "\t" $2
		}
	' "$log" | sort -t $'\t' -k1,1nr)"
	{
		echo "### Go ${label} package timings (${GO_SHARD:-unsharded})"
		echo
		if [[ -n "$timings" ]]; then
			echo '| Package | Seconds |'
			echo '| --- | ---: |'
			printf '%s\n' "$timings" | awk -F '\t' 'NR <= 10 { printf "| `%s` | %.3f |\n", $2, $1 }'
			printf '%s\n' "$timings" | awk -F '\t' '{ total += $1 } END { printf "\nReported package total: %.3fs\n", total }'
		else
			echo 'No uncached package durations were reported.'
		fi
		echo
	} >> "$GITHUB_STEP_SUMMARY"
fi

exit "$status"
