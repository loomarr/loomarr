#!/usr/bin/env bash
# Split a list of Go packages (read from stdin) into the ones that run UNDER `-race` and the
# ones that run without it, so `make test` can pay the race detector's ~2-3x cost only where a
# data race is actually possible. See the RACE_OFF list and the fail-safe note below.
#
#   ./scripts/go-shard.sh 1/3 | ./scripts/go-race-policy.sh --race     -> this shard's race pkgs
#   ./scripts/go-shard.sh 1/3 | ./scripts/go-race-policy.sh --no-race  -> this shard's non-race pkgs
#   ./scripts/go-race-policy.sh --verify
#       -> assert every RACE_OFF entry is a real package in `go list ./...` (CI red on drift)
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

module_path() { go list -m; }

# The RACE_OFF entries as fully-qualified import paths, one per line, sorted.
race_off_paths() {
	local mod
	mod="$(module_path)"
	local p
	for p in "${RACE_OFF[@]}"; do
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
	echo "       go-race-policy.sh --verify           (checks RACE_OFF against go list ./...)" >&2
	exit 2
}

mode="${1:-}"
[ -n "$mode" ] || usage

if [ "$mode" = "--verify" ]; then
	# Every RACE_OFF entry must resolve to a package the module actually contains. A ghost entry
	# (typo, renamed, deleted) would otherwise silently opt nothing out — harmless for coverage,
	# but it means the list is lying about what it excludes, so we fail loud.
	all="$(go list ./... | sort)"
	off="$(race_off_paths)"
	if [ -z "$off" ]; then
		echo "go-race-policy: OK — RACE_OFF is empty, every package runs under -race"
		exit 0
	fi
	# Entries present in RACE_OFF but absent from `go list ./...`.
	ghosts="$(comm -23 <(printf '%s\n' "$off") <(printf '%s\n' "$all"))"
	if [ -n "$ghosts" ]; then
		echo "go-race-policy: RACE_OFF names packages that do not exist in go list ./..." >&2
		printf '  %s\n' "$ghosts" >&2
		exit 1
	fi
	echo "go-race-policy: OK — all $(printf '%s\n' "$off" | wc -l | tr -d ' ') RACE_OFF entries are real packages"
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

off_paths="$(race_off_paths)"
stdin_sorted="$(printf '%s\n' "$stdin_pkgs" | sort -u)"

if [ -z "$off_paths" ]; then
	# Empty opt-out list: everything runs under -race, nothing runs without it.
	case "$mode" in
		--race) printf '%s\n' "$stdin_sorted" ;;
		--no-race) : ;; # emit nothing
	esac
	exit 0
fi

case "$mode" in
	# Packages in the input that are NOT opted out -> run under -race.
	--race) comm -23 <(printf '%s\n' "$stdin_sorted") <(printf '%s\n' "$off_paths") ;;
	# Packages in the input that ARE opted out -> run without -race.
	--no-race) comm -12 <(printf '%s\n' "$stdin_sorted") <(printf '%s\n' "$off_paths") ;;
esac
