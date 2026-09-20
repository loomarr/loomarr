#!/usr/bin/env bash
# Emit the package list for one internal lane of the Go test suite, so CI can split
# `make test` across runners without making media certification compete for a worker.
#
#   ./scripts/go-shard.sh          -> "./..."   (the whole tree — the default, always)
#   ./scripts/go-shard.sh 2/6      -> the 2nd ordinary measured-weight slice
#   ./scripts/go-shard.sh --isolated
#                                  -> the reviewed media-certification lane
#   ./scripts/go-shard.sh --plan 6 -> print each ordinary shard's modeled package-seconds
#   ./scripts/go-shard.sh --verify 6
#                                  -> assert exact coverage and every latency/balance budget
#
# The partition uses longest-processing-time assignment over a small, reviewed set of measured
# package costs. Packages below the materiality floor cost one modeled second, so every current and
# future package remains assigned even before it has a hosted timing. This replaces alphabetical
# placement, which drifted from a balanced 2026-09-01 sample to 1430/657/583 package-seconds in
# merge-group run 35472062915. Latency-sensitive media packages live in one reviewed serial lane;
# the remaining weighted packages are balanced across six ordinary lanes.
#
# ⚠ THE --verify MODE IS NOT OPTIONAL DECORATION. A sharding bug that DROPS a package does not
# fail anything: the dropped tests simply never run and every shard stays green, which is the
# worst outcome available here — a gate that reports success over code it did not execute. CI
# runs --verify for exactly that reason.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

WEIGHTS="${GO_SHARD_WEIGHTS:-$ROOT/scripts/go-race-weights.tsv}"
ISOLATED="${GO_SHARD_ISOLATED:-$ROOT/scripts/go-isolated-packages.txt}"
MAX_WEIGHT_SECONDS=540
MAX_IMBALANCE_PERCENT=125

if [[ ! -r "$WEIGHTS" ]]; then
  echo "go-shard: weight file is not readable: $WEIGHTS" >&2
  exit 2
fi
if [[ ! -r "$ISOLATED" ]]; then
  echo "go-shard: isolated package file is not readable: $ISOLATED" >&2
  exit 2
fi

# One `go list` per invocation, reused: it walks the module and is far from free.
packages() { go list ./...; }

# Expand the reviewed module-relative manifest to import paths without sorting it. The manifest
# order is reviewable and stable; set operations sort their own copies when required.
isolated_paths() {
  local module line
  module="$(go list -m)"
  while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line%%#*}"
    line="${line#"${line%%[![:space:]]*}"}"
    line="${line%"${line##*[![:space:]]}"}"
    [[ -z "$line" ]] && continue
    if [[ "$line" = "$module"/* ]]; then
      printf '%s\n' "$line"
    elif [[ "$line" = ./* ]]; then
      printf '%s/%s\n' "$module" "${line#./}"
    elif [[ "$line" != /* && "$line" != *[[:space:]]* ]]; then
      printf '%s/%s\n' "$module" "$line"
    else
      echo "go-shard: invalid isolated package row: $line" >&2
      return 2
    fi
  done < "$ISOLATED"
}

ordinary_packages() {
  comm -23 <(packages | sort) <(isolated_paths | sort -u)
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

isolated_load() {
  local module
  module="$(go list -m)"
  isolated_paths | awk -v module="$module" -v weights_file="$WEIGHTS" '
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

usage() {
  echo "usage: go-shard.sh [i/n | --isolated | --plan n | --verify n]" >&2
  exit 2
}

# --verify: every package appears in exactly one ordinary shard or the certification lane.
if [ "${1:-}" = "--verify" ]; then
  total="${2:-}"
  if ! [[ "$total" =~ ^[0-9]+$ ]] || [ "$total" -lt 1 ]; then
    usage
  fi

  all="$(packages | sort)"
  union="$( { for ((i = 1; i <= total; i++)); do slice "$i" "$total"; done; isolated_paths; } | sort)"

  # Compare the SORTED UNION against the full list. `comm` needs sorted input and reports
  # both directions, so a package that went missing and one that got duplicated into two
  # shards are distinguishable in the output rather than both reading as "differs".
  if [ "$all" = "$union" ]; then
    coverage="go-shard: OK — $total ordinary shards plus certification cover all $(echo "$all" | wc -l | tr -d ' ') packages, no duplicates"
  else
    echo "go-shard: LANE SPLIT IS NOT A PARTITION of go list ./... (ordinary shards=$total)" >&2
    echo "--- packages missing from every shard (these would go UNTESTED, green) ---" >&2
    comm -23 <(echo "$all") <(echo "$union") >&2
    echo "--- packages appearing more than once across shards (wasted, not unsafe) ---" >&2
    comm -13 <(echo "$all") <(echo "$union") >&2
    exit 1
  fi

  modeled="$(plan "$total")"
  if ! printf '%s\n' "$modeled" | awk -v max="$MAX_WEIGHT_SECONDS" -v ratio="$MAX_IMBALANCE_PERCENT" '
    NR == 1 { min = $2; high = $2 }
    { if ($2 < min) min = $2; if ($2 > high) high = $2 }
    END { exit !(high <= max && high * 100 <= min * ratio) }
  '; then
    echo "go-shard: modeled split exceeds ${MAX_WEIGHT_SECONDS}s or ${MAX_IMBALANCE_PERCENT}% balance budget" >&2
    while read -r shard seconds; do
      printf '  shard %s: %ss\n' "$shard" "$seconds" >&2
    done <<< "$modeled"
    exit 1
  fi
  certification_seconds="$(isolated_load)"
  if [ "$certification_seconds" -gt "$MAX_WEIGHT_SECONDS" ]; then
    echo "go-shard: certification lane exceeds ${MAX_WEIGHT_SECONDS}s modeled budget (${certification_seconds}s)" >&2
    exit 1
  fi
  echo "$coverage"
  while read -r shard seconds; do
    printf 'go-shard: modeled shard %s = %ss\n' "$shard" "$seconds"
  done <<< "$modeled"
  printf 'go-shard: modeled certification lane = %ss\n' "$certification_seconds"
  exit 0
fi

if [ "${1:-}" = "--isolated" ]; then
  [ "$#" -eq 1 ] || usage
  out="$(isolated_paths)"
  if [ -z "$out" ]; then
    echo "go-shard: certification lane is EMPTY" >&2
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
