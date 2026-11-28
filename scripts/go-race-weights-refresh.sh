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
# since the last change that moved a heavy package's cost). Every lane job counts, whatever its
# conclusion: an `ok` line is printed only when a package's tests finished, so it is a whole
# measurement even in a lane that later timed out, and a package cut off mid-run prints none. A lane
# over its timeout is exactly the one that needs re-measuring (#2716).
#
# A package with jobs of its own (go-split-lanes.tsv) gets one row per job, keyed by the lane that
# timed it: `<package>@race-<k>/<n>` and `<package>@plain`. Its race and plain times never share a
# median, and its samples from other lanes are dropped.
#
# GO_RACE_WEIGHTS_ONLY=<package> prints the existing weight file with only that package's rows
# replaced, for a package whose lanes change while other packages' runs are not yet comparable.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
gh_bin="${GO_RACE_WEIGHTS_GH:-gh}"
module="github.com/loomarr/loomarr/"
weights_file="${GO_SHARD_WEIGHTS:-$ROOT/scripts/go-race-weights.tsv}"
split_manifest="${GO_SHARD_SPLIT:-$ROOT/scripts/go-split-lanes.tsv}"
only="${GO_RACE_WEIGHTS_ONLY:-}"
split_packages="$(awk '$1 !~ /^#/ && NF == 3 { printf "%s ", $2 }' "$split_manifest")"

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
    awk -F '\t' '$2 ~ /^(success|failure|cancelled)$/ && $3 ~ /race-policy tests \(/ {
      lane = $3
      sub(/.*race-policy tests \(/, "", lane)
      sub(/\).*/, "", lane)
      suffix = "-"
      if (match(lane, /-race-[1-9]\/[1-9]$/)) suffix = "@" substr(lane, RSTART + 1)
      else if (lane ~ /-plain$/) suffix = "@plain"
      print $1 ":" suffix
    }')"
  for entry in $jobs; do
    job="${entry%%:*}"
    suffix="${entry#*:}"
    jobs_seen=$((jobs_seen + 1))
    # A log line is "<job>\t<step>\t<timestamp> ok  \t<module><package>\t<seconds>s". awk rather
    # than grep -P, which BSD grep (macOS) does not have.
    "$gh_bin" run view "$run" --job "$job" --log |
      awk -F '\t' -v module="$module" -v suffix="$suffix" -v split_list="$split_packages" '
        BEGIN { split(split_list, listed, " "); for (i in listed) is_split[listed[i]] = 1 }
        NF >= 3 && $(NF - 2) ~ /[[:space:]]ok  $/ && index($(NF - 1), module) == 1 &&
          $(NF - 1) !~ /[[:space:]]/ && $NF ~ /^[0-9.]+s$/ {
          seconds = $NF
          sub(/s$/, "", seconds)
          name = substr($(NF - 1), length(module) + 1)
          if (suffix != "-") name = name suffix
          else if (name in is_split) next
          print name "\t" seconds
        }' >> "$samples" || true
  done
done

if [[ "$jobs_seen" -eq 0 || ! -s "$samples" ]]; then
  echo "go-race-weights-refresh: no race-policy lane timings in runs: $*" >&2
  exit 1
fi

if [[ -n "$only" ]]; then
  # shellcheck disable=SC2016 # an awk condition: $1 is awk's field, not a shell variable
  of_package='$1 == only || index($1, only "@") == 1'
  if ! awk -F '\t' -v only="$only" "($of_package) { found = 1 } END { exit !found }" "$samples"; then
    echo "go-race-weights-refresh: no timings for $only in runs: $*" >&2
    exit 1
  fi
  grep '^#' "$weights_file"
  echo "# Rows for $only: runs $*."
  awk -F '\t' -v only="$only" "!/^#/ && !($of_package)" "$weights_file"
  awk -F '\t' -v only="$only" "$of_package" "$samples" > "$samples.only"
  mv "$samples.only" "$samples"
else
  echo "# Measured \`go test\` seconds per package in the Go race-policy lanes: the median over the"
  echo "# finished packages of the lane jobs in merge-group runs $*, rounded up. Regenerate with"
  echo "# \`make go-race-weights RUNS=\"...\"\` (scripts/go-race-weights-refresh.sh); never edit by hand."
  echo "# Packages below five seconds use the one-second floor in go-shard.sh."
fi
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
