#!/usr/bin/env bash
# Split a list of Go packages (read from stdin) into the ones that run UNDER `-race` and the
# ones that run without it, so `make test` can pay the race detector's ~2-3x cost only where a
# data race is actually possible. See the RACE_OFF list and the fail-safe note below.
#
#   ./scripts/go-shard.sh 1/3 | ./scripts/go-race-policy.sh --race     -> this shard's race pkgs
#   ./scripts/go-shard.sh 1/3 | ./scripts/go-race-policy.sh --no-race  -> this shard's non-race pkgs
#   ./scripts/go-race-policy.sh --verify
#       -> assert every RACE_OFF and RACE_AND_PLAIN entry is a real package in `go list ./...`
#          (CI red on drift), and that no package is on both lists
#
# A package on RACE_AND_PLAIN is in BOTH outputs, so it runs twice: under -race and without it.
# See that list below for why.
#
# ⚠ FAIL-SAFE BY INVERSION. This is an OPT-OUT list, never an opt-in one. Race stays ON for every
# package by default; RACE_OFF names the few proven to have no concurrency worth checking. The
# consequence of forgetting to update the list is therefore always the SAFE direction: a new
# package is race-checked until someone deliberately opts it out, and the worst a stale RACE_OFF
# entry can do is keep a package SLOWER than necessary — never leave a real race unchecked. An
# opt-IN list would have the opposite, dangerous failure: forget to add a concurrent package and
# its races run undetected, green. That asymmetry is the whole reason this is written this way.
#
# ⚠ EARNING A SPOT ON RACE_OFF. A package belongs here only if it has NO goroutines, no shared
# mutable state across test cases, and no concurrency in the code under test — pure
# transform/table-test packages. When in doubt, leave it OFF the list (i.e. race stays ON). The
# race detector has caught real bugs in this repo (a huma global written per-Router build; a
# filler/scheduler write through a dead context), so the bar for removing coverage is high.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# Packages that run WITHOUT -race. Each MUST be a real import path under this module. `make
# go-race-verify` (and CI) asserts that, so a rename or deletion that leaves a ghost here turns
# CI red rather than silently opting a non-existent package out of nothing.
#
# Keep this list SHORT and JUSTIFIED. The measurement that seeds it lives in the PR that adds
# this file; re-measure before growing it. A package earns its place by being demonstrably
# concurrency-free AND slow enough under -race to matter — not merely by being currently green.
# Seeded 2026-08-18 by timing `go test -race` per package on the then-current tree and then
# scanning each cheap package for ANY concurrency — goroutines, channels, sync primitives, AND
# `httptest.NewServer` (which runs the handler in its own goroutine, so a test that uses one has
# real test-vs-server concurrency the race detector should watch). Only packages with NONE of
# those, in both source and tests, are listed. They are pure config / table-test / doc-generator
# packages whose -race cost is ~1s each; the list exists to shave that fixed overhead, not to
# touch anything concurrent. The heavy packages (app, store, api, images, integration, channels,
# auth, …) are deliberately absent: they are exactly where -race has caught real bugs here.
RACE_OFF=(
	internal/catalog
	internal/config
	internal/taxonomy
	internal/provision
	internal/activity
	internal/setup
	cmd/dev-docs
	cmd/arch-docs
	docs
)

# Packages that run under -race AND without it. Race stays ON for them, so this list cannot
# shrink race coverage. It exists for a package whose tests sweep pure planning logic over many
# cases and also share memoised state across parallel tests: the race detector is needed for the
# shared state, but it costs the sweeps several times their plain run. Such a package's tests read
# a `race` build-tag constant: under -race they walk a small sample that keeps the pinned
# regression cases and the parallel paths through the shared state, and without it they walk
# the full sample, so every assertion still runs in CI.
#
# internal/schedule (#2007): the full sample took 1021s under -race on four cores and 113s
# without it, and shard 1 hit its 15-minute limit on every run from #1939 on.
#
# Each entry runs in both phases of its lane, so its one measured weight in
# scripts/go-race-weights.tsv (the median of its race and plain samples) is counted in both.
RACE_AND_PLAIN=(
	internal/schedule
)

module_path() { go list -m; }

# The given entries as fully-qualified import paths, one per line, sorted.
import_paths() {
	local mod
	mod="$(module_path)"
	local p
	for p in "$@"; do
		# Allow entries written either as full import paths or as module-relative (./internal/x).
		case "$p" in
			"$mod"/*) printf '%s\n' "$p" ;;
			./*) printf '%s/%s\n' "$mod" "${p#./}" ;;
			*) printf '%s/%s\n' "$mod" "$p" ;;
		esac
	done | sort -u
}

usage() {
	echo "usage: go-race-policy.sh --race|--no-race   (reads package list on stdin)" >&2
	echo "       go-race-policy.sh --verify           (checks RACE_OFF and RACE_AND_PLAIN against go list ./...)" >&2
	exit 2
}

mode="${1:-}"
[ -n "$mode" ] || usage

off_paths="$(import_paths "${RACE_OFF[@]}")"
both_paths="$(import_paths "${RACE_AND_PLAIN[@]}")"

if [ "$mode" = "--verify" ]; then
	# Every entry must resolve to a package the module actually contains. A ghost entry (typo,
	# renamed, deleted) would otherwise silently select nothing — harmless for coverage, but it
	# means the list is lying about what it names, so we fail loud.
	all="$(go list ./... | sort)"
	for list in RACE_OFF RACE_AND_PLAIN; do
		if [ "$list" = RACE_OFF ]; then paths="$off_paths"; else paths="$both_paths"; fi
		[ -n "$paths" ] || continue
		ghosts="$(comm -23 <(printf '%s\n' "$paths") <(printf '%s\n' "$all"))"
		if [ -n "$ghosts" ]; then
			echo "go-race-policy: $list names packages that do not exist in go list ./..." >&2
			printf '  %s\n' "$ghosts" >&2
			exit 1
		fi
	done
	# A package cannot both opt out of -race and run under it.
	clash="$(comm -12 <(printf '%s\n' "$off_paths") <(printf '%s\n' "$both_paths"))"
	if [ -n "$clash" ]; then
		echo "go-race-policy: packages on both RACE_OFF and RACE_AND_PLAIN:" >&2
		printf '  %s\n' "$clash" >&2
		exit 1
	fi
	echo "go-race-policy: OK — all $(printf '%s\n' "$off_paths" "$both_paths" | grep -c .) RACE_OFF and RACE_AND_PLAIN entries are real packages"
	exit 0
fi

case "$mode" in
	--race | --no-race) ;;
	*) usage ;;
esac

# Read the candidate package list (typically one shard's slice) from stdin.
stdin_pkgs="$(cat)"
if [ -z "$stdin_pkgs" ]; then
	echo "go-race-policy: no packages on stdin" >&2
	exit 2
fi

# Preserve the sharder's descending-cost order. Sorting here used to undo its LPT schedule and
# made heavy packages start minutes late even though the aggregate lane weights were balanced.
# The lists go in as variables, not as a first input file: an empty file breaks awk's NR == FNR.
# Space-separated, since BSD awk rejects a newline in a -v value (import paths hold no spaces).
printf '%s\n' "$stdin_pkgs" | awk -v mode="$mode" -v off="$(printf '%s' "$off_paths" | tr '\n' ' ')" \
	-v both="$(printf '%s' "$both_paths" | tr '\n' ' ')" '
	BEGIN {
		n = split(off, o, " "); for (i = 1; i <= n; i++) if (o[i] != "") is_off[o[i]] = 1
		n = split(both, b, " "); for (i = 1; i <= n; i++) if (b[i] != "") is_both[b[i]] = 1
	}
	NF && !seen[$0]++ {
		if (mode == "--race" && !($0 in is_off)) print
		if (mode == "--no-race" && ($0 in is_off || $0 in is_both)) print
	}'
