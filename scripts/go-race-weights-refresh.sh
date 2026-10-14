#!/usr/bin/env bash
# Regenerate scripts/go-race-weights.tsv from real hosted runs, so the shard model reads measured
# seconds instead of hand-kept numbers.
#
#   ./scripts/go-race-weights-refresh.sh RUN_ID...   -> the weight file on stdout
#   make go-race-weights RUNS="RUN_ID..."            -> rewrite scripts/go-race-weights.tsv
#
# For every successful Go race-policy lane job in the given workflow runs, each `ok <package> <N>s`
# line is one sample: the package's test-binary wall time under that lane's own concurrency
# (`-p=4` ordinary, `-p=1` certification), which is the unit go-shard.sh models. A package's weight
# is the median of its samples rounded up to whole seconds. There is no headroom: the budgets in
# go-shard.sh carry the margin, derived from #1570's wall-clock target. Packages under five seconds
# are left to go-shard.sh's one-second floor, so the file lists only material costs.
#
# Pass runs with the same package set and test code you want to model (e.g. every merge_group run
# since the last change that moved a heavy package's cost). Failed or cancelled lane jobs are
# skipped: a package that failed early has no representative time.
set -euo pipefail

gh_bin="${GO_RACE_WEIGHTS_GH:-gh}"
module="github.com/loomarr/loomarr/"

if [[ "$#" -eq 0 ]]; then
  echo "usage: go-race-weights-refresh.sh RUN_ID..." >&2
  exit 2
fi

samples="$(mktemp "${TMPDIR:-/tmp}/loomarr-race-weights.XXXXXX")"
trap 'rm -f "$samples"' EXIT

jobs_seen=0
for run in "$@"; do
  if ! [[ "$run" =~ ^[0-9]+$ ]]; then
    echo "go-race-weights-refresh: invalid run id '$run'" >&2
    exit 2
  fi
  jobs="$("$gh_bin" run view "$run" --json jobs --jq '.jobs[] | [.databaseId, .conclusion, .name] | @tsv' |
    awk -F '\t' '$2 == "success" && $3 ~ /race-policy tests \(/ { print $1 }')"
  for job in $jobs; do
    jobs_seen=$((jobs_seen + 1))
    "$gh_bin" run view "$run" --job "$job" --log |
      grep -oP "\\sok  \\t\\Q${module}\\E\\S+\\t[0-9.]+s$" |
      awk -F '\t' -v module="$module" '{ sub(module, "", $2); sub(/s$/, "", $3); print $2 "\t" $3 }' \
        >> "$samples" || true
  done
done

if [[ "$jobs_seen" -eq 0 || ! -s "$samples" ]]; then
  echo "go-race-weights-refresh: no successful race-policy lane timings in runs: $*" >&2
  exit 1
fi

echo "# Measured \`go test\` seconds per package in the Go race-policy lanes: the median over successful"
echo "# lane jobs of merge-group runs $*, rounded up. Regenerate with"
echo "# \`make go-race-weights RUNS=\"...\"\` (scripts/go-race-weights-refresh.sh); never edit by hand."
echo "# Packages below five seconds use the one-second floor in go-shard.sh."
sort -t $'\t' -k1,1 -k2,2n "$samples" | awk -F '\t' '
  function flush() {
    if (count == 0) return
    median = (count % 2) ? value[(count + 1) / 2] : (value[count / 2] + value[count / 2 + 1]) / 2
    seconds = int(median)
    if (seconds < median) seconds++
    if (seconds >= 5) print seconds "\t" package
    count = 0
  }
  $1 != package { flush(); package = $1 }
  { value[++count] = $2 + 0 }
  END { flush() }
' | sort -t $'\t' -k1,1nr -k2,2 | awk -F '\t' '{ print $2 "\t" $1 }'
