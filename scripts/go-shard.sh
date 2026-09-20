#!/usr/bin/env bash
# Emit the package list for one internal lane of the Go test suite, so CI can split
# `make test` across runners without making media certification compete for a worker.
#
#   ./scripts/go-shard.sh          -> "./..."   (the whole tree — the default, always)
#   ./scripts/go-shard.sh 2/4      -> the 2nd ordinary measured-weight slice
#   ./scripts/go-shard.sh --certification 2/2
#                                  -> the second reviewed media-certification lane
#   ./scripts/go-shard.sh --plan 4 -> print each ordinary shard's modeled package-seconds
#   ./scripts/go-shard.sh --worker-plan 4
#                                  -> print each shard's bounded two-worker makespan
#   ./scripts/go-shard.sh --verify 4
#                                  -> assert exact coverage and every latency/balance budget
#
# The partition uses longest-processing-time assignment over a small, reviewed set of measured
# package costs. Packages below the materiality floor cost one modeled second, so every current and
# future package remains assigned even before it has a hosted timing. This replaces alphabetical
# placement, which drifted from a balanced 2026-09-01 sample to 1430/657/583 package-seconds in
# merge-group run 35472062915. Latency-sensitive media packages live in two reviewed serial lanes;
# the remaining weighted packages are balanced across four ordinary lanes. Separate runners let the
# two certification groups overlap without allowing package concurrency inside either group.
#
# ⚠ THE --verify MODE IS NOT OPTIONAL DECORATION. A sharding bug that DROPS a package does not
# fail anything: the dropped tests simply never run and every shard stays green, which is the
# worst outcome available here — a gate that reports success over code it did not execute. CI
# runs --verify for exactly that reason.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

WEIGHTS="${GO_SHARD_WEIGHTS:-$ROOT/scripts/go-race-weights.tsv}"
CERTIFICATION="${GO_SHARD_CERTIFICATION:-$ROOT/scripts/go-certification-lanes.tsv}"
RACE_POLICY="${GO_SHARD_RACE_POLICY:-$ROOT/scripts/go-race-policy.sh}"
CERTIFICATION_LANES=2
MAX_WEIGHT_SECONDS=540
MAX_ORDINARY_AGGREGATE_SECONDS=600
MAX_IMBALANCE_PERCENT=125

if [[ ! -r "$WEIGHTS" ]]; then
  echo "go-shard: weight file is not readable: $WEIGHTS" >&2
  exit 2
fi
if [[ ! -r "$CERTIFICATION" ]]; then
  echo "go-shard: certification lane file is not readable: $CERTIFICATION" >&2
  exit 2
fi
if [[ ! -x "$RACE_POLICY" ]]; then
  echo "go-shard: race policy is not executable: $RACE_POLICY" >&2
  exit 2
fi

# One `go list` per invocation, reused: it walks the module and is far from free.
packages() { go list ./...; }

# Expand the reviewed module-relative manifest to `lane<TAB>import-path` rows without sorting it.
# The manifest order is reviewable and stable; set operations sort their own copies when required.
certification_rows() {
  local module line lane path extra
  module="$(go list -m)"
  while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line%%#*}"
    line="${line#"${line%%[![:space:]]*}"}"
    line="${line%"${line##*[![:space:]]}"}"
    [[ -z "$line" ]] && continue
    read -r lane path extra <<< "$line"
    if [[ -n "$extra" || ! "$lane" =~ ^[12]$ || -z "$path" ]]; then
      echo "go-shard: invalid certification lane row: $line" >&2
      return 2
    fi
    if [[ "$path" = "$module"/* ]]; then
      printf '%s\t%s\n' "$lane" "$path"
    elif [[ "$path" = ./* ]]; then
      printf '%s\t%s/%s\n' "$lane" "$module" "${path#./}"
    elif [[ "$path" != /* && "$path" != *[[:space:]]* ]]; then
      printf '%s\t%s/%s\n' "$lane" "$module" "$path"
    else
      echo "go-shard: invalid certification package path: $path" >&2
      return 2
    fi
  done < "$CERTIFICATION"
}

certification_paths() {
  certification_rows | cut -f2
}

certification_lane_paths() {
  local lane="$1"
  certification_rows | awk -v target="$lane" '$1 == target { print $2 }'
}

ordinary_packages() {
  comm -23 <(packages | sort) <(certification_paths | sort -u)
}

# Assign the largest measured package to the currently lightest shard. Ties are deterministic:
# package path for work ordering, then the lowest shard number for placement. Output stays in the
# original `go list` order so callers receive a stable package list.
partition() {
  local mode="$1" index="$2" total="$3" module
  module="$(go list -m)"
  ordinary_packages | awk -v mode="$mode" -v target="$index" -v n="$total" -v module="$module" -v weights_file="$WEIGHTS" '
    BEGIN {
      while ((getline line < weights_file) > 0) {
        if (line ~ /^[[:space:]]*(#|$)/) continue
        fields = split(line, part, /[[:space:]]+/)
        if (fields != 2 || part[2] !~ /^[1-9][0-9]*$/) {
          print "go-shard: invalid weight row: " line > "/dev/stderr"
          invalid = 1
          exit 2
        }
        weight[part[1]] = part[2] + 0
      }
      close(weights_file)
    }
    {
      count++
      package[count] = $0
      relative = $0
      prefix = module "/"
      if (index(relative, prefix) == 1) relative = substr(relative, length(prefix) + 1)
      cost[count] = (relative in weight) ? weight[relative] : 1
      order[count] = count
    }
    END {
      if (invalid) exit 2
      for (left = 1; left <= count; left++) {
        best = left
        for (candidate = left + 1; candidate <= count; candidate++) {
          a = order[candidate]
          b = order[best]
          if (cost[a] > cost[b] || (cost[a] == cost[b] && package[a] < package[b])) best = candidate
        }
        swap = order[left]
        order[left] = order[best]
        order[best] = swap
      }
      for (rank = 1; rank <= count; rank++) {
        item = order[rank]
        shard = 1
        for (candidate = 2; candidate <= n; candidate++) {
          if (load[candidate] < load[shard]) shard = candidate
        }
        assigned[item] = shard
        load[shard] += cost[item]
      }
      if (mode == "plan") {
        for (shard = 1; shard <= n; shard++) print shard, load[shard]
        exit
      }
      for (item = 1; item <= count; item++) {
        if (assigned[item] == target) print package[item]
      }
    }
  '
}

certification_load() {
  local lane="$1" module
  module="$(go list -m)"
  certification_lane_paths "$lane" | awk -v module="$module" -v weights_file="$WEIGHTS" '
    BEGIN {
      while ((getline line < weights_file) > 0) {
        if (line ~ /^[[:space:]]*(#|$)/) continue
        split(line, part, /[[:space:]]+/)
        weight[part[1]] = part[2] + 0
      }
      close(weights_file)
    }
    {
      relative = $0
      prefix = module "/"
      if (index(relative, prefix) == 1) relative = substr(relative, length(prefix) + 1)
      total += (relative in weight) ? weight[relative] : 1
    }
    END { print total + 0 }
  '
}

slice() {
  partition packages "$1" "$2"
}

plan() {
  partition plan 0 "$1"
}

# Model the package scheduler used by an ordinary lane. The race and non-race groups execute
# sequentially, each with GOFLAGS=-p=2, so their independent two-worker LPT makespans must be
# added. Keeping this distinct from aggregate package-seconds lets the coverage plan reject both
# excessive total work and a latency regression hidden by the bounded package overlap.
weighted_two_worker_makespan() {
  local module
  module="$(go list -m)"
  awk -v module="$module" -v weights_file="$WEIGHTS" '
    BEGIN {
      while ((getline line < weights_file) > 0) {
        if (line ~ /^[[:space:]]*(#|$)/) continue
        split(line, part, /[[:space:]]+/)
        weight[part[1]] = part[2] + 0
      }
      close(weights_file)
    }
    {
      count++
      package[count] = $0
      relative = $0
      prefix = module "/"
      if (index(relative, prefix) == 1) relative = substr(relative, length(prefix) + 1)
      cost[count] = (relative in weight) ? weight[relative] : 1
      order[count] = count
    }
    END {
      for (left = 1; left <= count; left++) {
        best = left
        for (candidate = left + 1; candidate <= count; candidate++) {
          a = order[candidate]
          b = order[best]
          if (cost[a] > cost[b] || (cost[a] == cost[b] && package[a] < package[b])) best = candidate
        }
        swap = order[left]
        order[left] = order[best]
        order[best] = swap
      }
      for (rank = 1; rank <= count; rank++) {
        item = order[rank]
        worker = (load[2] < load[1]) ? 2 : 1
        load[worker] += cost[item]
      }
      print (load[1] > load[2]) ? load[1] : load[2]
    }
  '
}

ordinary_worker_load() {
  local lane="$1" total="$2" shard_packages race_packages plain_packages
  local race_load=0 plain_load=0
  shard_packages="$(slice "$lane" "$total")"
  race_packages="$(printf '%s\n' "$shard_packages" | "$RACE_POLICY" --race)"
  plain_packages="$(printf '%s\n' "$shard_packages" | "$RACE_POLICY" --no-race)"
  if [[ -n "$race_packages" ]]; then
    race_load="$(printf '%s\n' "$race_packages" | weighted_two_worker_makespan)"
  fi
  if [[ -n "$plain_packages" ]]; then
    plain_load="$(printf '%s\n' "$plain_packages" | weighted_two_worker_makespan)"
  fi
  echo $((race_load + plain_load))
}

worker_plan() {
  local total="$1" lane
  for ((lane = 1; lane <= total; lane++)); do
    printf '%s %s\n' "$lane" "$(ordinary_worker_load "$lane" "$total")"
  done
}

usage() {
  echo "usage: go-shard.sh [i/n | --certification i/2 | --plan n | --worker-plan n | --verify n]" >&2
  exit 2
}

# --verify: every package appears in exactly one ordinary shard or certification lane.
if [ "${1:-}" = "--verify" ]; then
  total="${2:-}"
  if ! [[ "$total" =~ ^[0-9]+$ ]] || [ "$total" -lt 1 ]; then
    usage
  fi

  all="$(packages | sort)"
  union="$( { for ((i = 1; i <= total; i++)); do slice "$i" "$total"; done; certification_paths; } | sort)"

  # Compare the SORTED UNION against the full list. `comm` needs sorted input and reports
  # both directions, so a package that went missing and one that got duplicated into two
  # shards are distinguishable in the output rather than both reading as "differs".
  if [ "$all" = "$union" ]; then
    coverage="go-shard: OK — $total ordinary shards plus $CERTIFICATION_LANES certification lanes cover all $(echo "$all" | wc -l | tr -d ' ') packages, no duplicates"
  else
    echo "go-shard: LANE SPLIT IS NOT A PARTITION of go list ./... (ordinary shards=$total)" >&2
    echo "--- packages missing from every shard (these would go UNTESTED, green) ---" >&2
    comm -23 <(echo "$all") <(echo "$union") >&2
    echo "--- packages appearing more than once across shards (wasted, not unsafe) ---" >&2
    comm -13 <(echo "$all") <(echo "$union") >&2
    exit 1
  fi

  modeled="$(plan "$total")"
  if ! printf '%s\n' "$modeled" | awk -v max="$MAX_ORDINARY_AGGREGATE_SECONDS" -v ratio="$MAX_IMBALANCE_PERCENT" '
    NR == 1 { min = $2; high = $2 }
    { if ($2 < min) min = $2; if ($2 > high) high = $2 }
    END { exit !(high <= max && high * 100 <= min * ratio) }
  '; then
    echo "go-shard: modeled aggregate split exceeds ${MAX_ORDINARY_AGGREGATE_SECONDS}s or ${MAX_IMBALANCE_PERCENT}% balance budget" >&2
    while read -r shard seconds; do
      printf '  shard %s: %ss\n' "$shard" "$seconds" >&2
    done <<< "$modeled"
    exit 1
  fi
  worker_modeled="$(worker_plan "$total")"
  if ! printf '%s\n' "$worker_modeled" | awk -v max="$MAX_WEIGHT_SECONDS" -v ratio="$MAX_IMBALANCE_PERCENT" '
    NR == 1 { min = $2; high = $2 }
    { if ($2 < min) min = $2; if ($2 > high) high = $2 }
    END { exit !(min > 0 && high <= max && high * 100 <= min * ratio) }
  '; then
    echo "go-shard: modeled bounded-worker split exceeds ${MAX_WEIGHT_SECONDS}s or ${MAX_IMBALANCE_PERCENT}% balance budget" >&2
    while read -r shard seconds; do
      printf '  shard %s: %ss\n' "$shard" "$seconds" >&2
    done <<< "$worker_modeled"
    exit 1
  fi
  certification_modeled="$(for ((i = 1; i <= CERTIFICATION_LANES; i++)); do printf '%s %s\n' "$i" "$(certification_load "$i")"; done)"
  if ! printf '%s\n' "$certification_modeled" | awk -v max="$MAX_WEIGHT_SECONDS" -v ratio="$MAX_IMBALANCE_PERCENT" '
    NR == 1 { min = $2; high = $2 }
    { if ($2 < min) min = $2; if ($2 > high) high = $2 }
    END { exit !(min > 0 && high <= max && high * 100 <= min * ratio) }
  '; then
    echo "go-shard: certification split exceeds ${MAX_WEIGHT_SECONDS}s or ${MAX_IMBALANCE_PERCENT}% balance budget" >&2
    while read -r lane seconds; do
      printf '  certification lane %s/%s: %ss\n' "$lane" "$CERTIFICATION_LANES" "$seconds" >&2
    done <<< "$certification_modeled"
    exit 1
  fi
  echo "$coverage"
  while read -r shard seconds; do
    printf 'go-shard: modeled shard %s = %ss\n' "$shard" "$seconds"
  done <<< "$modeled"
  while read -r shard seconds; do
    printf 'go-shard: modeled shard %s bounded-worker makespan = %ss\n' "$shard" "$seconds"
  done <<< "$worker_modeled"
  while read -r lane seconds; do
    printf 'go-shard: modeled certification lane %s/%s = %ss\n' "$lane" "$CERTIFICATION_LANES" "$seconds"
  done <<< "$certification_modeled"
  exit 0
fi

if [ "${1:-}" = "--certification" ]; then
  [ "$#" -eq 2 ] || usage
  spec="$2"
  index="${spec%%/*}"
  total="${spec##*/}"
  [[ "$index" =~ ^[0-9]+$ ]] || usage
  [[ "$total" =~ ^[0-9]+$ ]] || usage
  [ "$total" -eq "$CERTIFICATION_LANES" ] || usage
  if [ "$index" -lt 1 ] || [ "$index" -gt "$CERTIFICATION_LANES" ]; then
    usage
  fi
  out="$(certification_lane_paths "$index")"
  if [ -z "$out" ]; then
    echo "go-shard: certification lane $spec is EMPTY" >&2
    exit 2
  fi
  echo "$out"
  exit 0
fi

if [ "${1:-}" = "--plan" ]; then
  total="${2:-}"
  if ! [[ "$total" =~ ^[0-9]+$ ]] || [ "$total" -lt 1 ]; then
    usage
  fi
  plan "$total"
  exit 0
fi

if [ "${1:-}" = "--worker-plan" ]; then
  total="${2:-}"
  if ! [[ "$total" =~ ^[0-9]+$ ]] || [ "$total" -lt 1 ]; then
    usage
  fi
  worker_plan "$total"
  exit 0
fi

# No spec: the whole tree. This is the DEFAULT on purpose — `make verify SCOPE=all` must run the
# entire suite. Sharding is never implicit, or someone runs a fraction of the gate and reads
# the green as the whole thing.
spec="${1:-}"
if [ -z "$spec" ]; then
  echo "./..."
  exit 0
fi

index="${spec%%/*}"
total="${spec##*/}"
[[ "$index" =~ ^[0-9]+$ ]] || usage
[[ "$total" =~ ^[0-9]+$ ]] || usage
[ "$total" -ge 1 ] || usage
# Out of range is a hard error, never an empty package list: `go test` with no packages exits
# 0, so a bad lane identity would otherwise be a silent green over zero tests.
if [ "$index" -lt 1 ] || [ "$index" -gt "$total" ]; then
  echo "go-shard: shard index $index out of range 1..$total" >&2
  exit 2
fi

out="$(slice "$index" "$total")"
# Likewise: a shard that legitimately resolves to nothing (more shards than packages) must be
# loud, because `go test` would accept the empty list and report success.
if [ -z "$out" ]; then
  echo "go-shard: shard $index/$total is EMPTY — more shards than packages?" >&2
  exit 2
fi
echo "$out"
