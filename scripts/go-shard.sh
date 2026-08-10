#!/usr/bin/env bash
# Emit the package list for ONE shard of the Go test suite, so CI can split
# `make test` across runners for wall-clock. See the GO_SHARD note in the Makefile.
#
#   ./scripts/go-shard.sh          -> "./..."   (the whole tree — the default, always)
#   ./scripts/go-shard.sh 2/3      -> the 2nd of 3 round-robin slices of `go list ./...`
#   ./scripts/go-shard.sh --verify 3
#                                  -> assert the 3 slices reconstruct `go list ./...` exactly
#
# ⚠ ROUND-ROBIN, NOT CONTIGUOUS BLOCKS, and not a cost table. `go list` is alphabetical, so
# contiguous blocks would pile `internal/a*` into shard 1 and leave the tail idle. Round-robin
# interleaves, which spreads the expensive packages (measured 2026-08-09: store 48s, images 36s,
# api 36s, integration 31s, auth 30s) across shards without anyone maintaining a list of what is
# slow. A hand-written cost table is the thing this repo keeps getting bitten by — it would be
# correct the day it was written and wrong by the next phase.
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

# Shard i of n, 1-indexed. NR is awk's 1-based record number, so shard i takes the records
# where (NR-1) % n == i-1, i.e. NR % n == i % n (which lands shard n on NR % n == 0).
slice() {
  local index="$1" total="$2"
  packages | awk -v i="$index" -v n="$total" 'NR % n == (i % n)'
}

usage() {
  echo "usage: go-shard.sh [i/n | --verify n]" >&2
  exit 2
}

# --verify: every package appears in exactly one shard, and the union is the full list.
if [ "${1:-}" = "--verify" ]; then
  total="${2:-}"
  [[ "$total" =~ ^[0-9]+$ ]] && [ "$total" -ge 1 ] || usage

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

# No spec: the whole tree. This is the DEFAULT on purpose — a local `make check` must run the
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
