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
	# -race also turns on checkptr in every package. modernc's SQLite is C translated to Go, unsafe
	# pointer arithmetic on nearly every line, and checkptr there doubled the cost of every migration
	# (a 36 s migration test took 20 s without it, #2716). The race detector still instruments it;
	# only checkptr is off, and only for modernc.
	args+=(-race "-gcflags=modernc.org/...=-d=checkptr=0")
	label="race"
fi
# Cache warming only (.github/workflows/ci-go-cache-warm.yml): build every test binary with the
# lane's exact flags and execute none. `true` stands in for each binary. A gate that set this would
# report green without running a test, so releaseverify allows it nowhere else.
if [[ "${GO_TEST_COMPILE_ONLY:-}" == 1 ]]; then
	args+=(-exec true)
fi

# GO_TEST_PART=k/n runs the k-th of n slices of one package's top-level tests: every n-th name of
# the sorted `go test -list`, built with this run's flags. Benchmarks do not run here, so they are
# not listed. An empty slice fails, because `go test -run` matching nothing exits 0.
if [[ -n "${GO_TEST_PART:-}" && "${GO_TEST_COMPILE_ONLY:-}" != 1 ]]; then
	if [[ ! "$GO_TEST_PART" =~ ^([1-9])/([1-9])$ || "$#" -ne 1 ]] || ((BASH_REMATCH[1] > BASH_REMATCH[2])); then
		echo "go-test-packages: GO_TEST_PART wants k/n and exactly one package, got '$GO_TEST_PART' for $*" >&2
		exit 2
	fi
	part="${BASH_REMATCH[1]}"
	parts="${BASH_REMATCH[2]}"
	listed="$("$go_bin" "${args[@]}" -list . "$@")" || exit
	names="$(printf '%s\n' "$listed" | awk '/^(Test|Fuzz|Example)/' | LC_ALL=C sort |
		awk -v part="$part" -v parts="$parts" '(NR - 1) % parts == part - 1' | paste -sd '|' -)"
	if [[ -z "$names" ]]; then
		echo "go-test-packages: slice $GO_TEST_PART of $* holds no tests" >&2
		exit 2
	fi
	args+=(-run "^($names)\$")
fi

set +e
"$go_bin" "${args[@]}" "$@" 2>&1 | tee "$log"
status="${PIPESTATUS[0]}"
set -e

if [[ -n "${GITHUB_STEP_SUMMARY:-}" && "${GO_TEST_COMPILE_ONLY:-}" != 1 ]]; then
	timings="$(awk '
		$1 == "ok" && $NF ~ /^[0-9.]+s$/ {
			seconds = $NF
			sub(/s$/, "", seconds)
			print seconds "\t" $2
		}
	' "$log" | sort -t $'\t' -k1,1nr)"
	{
		echo "### Go ${label} package timings (${GO_TEST_LANE:-unsharded})"
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
