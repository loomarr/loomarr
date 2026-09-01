#!/usr/bin/env bash
# Emit the package list for ONE shard of the Go test suite, so CI can split
# `make test` across runners for wall-clock. See the GO_SHARD note in the Makefile.
#
#   ./scripts/go-shard.sh          -> "./..."   (the whole tree — the default, always)
#   ./scripts/go-shard.sh 2/3      -> the 2nd of 3 serpentine slices of `go list ./...`
#   ./scripts/go-shard.sh --verify 3
#                                  -> assert the 3 slices reconstruct `go list ./...` exactly
#
# ⚠ SERPENTINE, NOT CONTIGUOUS BLOCKS or straight round-robin, and not a cost table. `go list` is
# alphabetical, so contiguous blocks pile related packages together. Straight round-robin can
# create an every-N phase alignment: measured 2026-09-01, three of the heaviest current packages
# (`app`, `channels`, and `store`) all landed on shard 1, whose hosted test step took 11m57s while
# the peers took 6m30s and 5m40s. Grouping the same stream into N-wide rows and alternating each
# row's direction retains automatic assignment while breaking that alignment. Against the exact
# hosted package timings, the modeled totals move from 1058/683/523s to 766/683/816s. A hand-written
# package-cost table would be correct only on the day it was written and then drift silently.
#
# ⚠ THE --verify MODE IS NOT OPTIONAL DECORATION. A sharding bug that DROPS a package does not
# fail anything: the dropped tests simply never run and every shard stays green, which is the
# worst outcome available here — a gate that reports success over code it did not execute. CI
# runs --verify for exactly that reason.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# One `go list` per invocation, reused: it walks the module and is far from free.
packages() { go list ./...; }

# Shard i of n, 1-indexed. Read each N-package row left-to-right, then right-to-left, alternating.
# For three shards the assignments are 1,2,3 / 3,2,1 / 1,2,3 ... . Every complete row contributes
# one package to every shard, while adjacent alphabetical groups do not keep the same phase.
slice() {
  local index="$1" total="$2"
  packages | awk -v i="$index" -v n="$total" '
    {
      row = int((NR - 1) / n)
      offset = (NR - 1) % n
      shard = (row % 2 == 0) ? offset + 1 : n - offset
      if (shard == i) print
    }
  '
}

usage() {
  echo "usage: go-shard.sh [i/n | --verify n]" >&2
  exit 2
}

# --verify: every package appears in exactly one shard, and the union is the full list.
if [ "${1:-}" = "--verify" ]; then
  total="${2:-}"
  if ! [[ "$total" =~ ^[0-9]+$ ]] || [ "$total" -lt 1 ]; then
    usage
  fi

  all="$(packages | sort)"
  union="$(for ((i = 1; i <= total; i++)); do slice "$i" "$total"; done | sort)"

  # Compare the SORTED UNION against the full list. `comm` needs sorted input and reports
  # both directions, so a package that went missing and one that got duplicated into two
  # shards are distinguishable in the output rather than both reading as "differs".
  if [ "$all" = "$union" ]; then
    echo "go-shard: OK — $total shards cover all $(echo "$all" | wc -l | tr -d ' ') packages, no duplicates"
    exit 0
  fi

  echo "go-shard: SHARD SPLIT IS NOT A PARTITION of go list ./... (shards=$total)" >&2
  echo "--- packages missing from every shard (these would go UNTESTED, green) ---" >&2
  comm -23 <(echo "$all") <(echo "$union") >&2
  echo "--- packages appearing more than once across shards (wasted, not unsafe) ---" >&2
  comm -13 <(echo "$all") <(echo "$union") >&2
  exit 1
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
# 0, so a bad GO_SHARD would otherwise be a silent green over zero tests.
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
