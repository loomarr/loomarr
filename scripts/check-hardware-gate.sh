#!/usr/bin/env bash
# Has this commit passed the playout bench on every GPU family that blocks a release?
#
#   scripts/check-hardware-gate.sh <full commit id>
#
# Software is covered by required CI. Intel and NVIDIA run on maintainer machines, so they need a
# dispatched bench with both legs passing on this exact commit. A skipped leg counts as missing:
# that is how the nightly bench stays green with no GPU tested at all.

set -euo pipefail

COMMIT="${1:-}"
REPOSITORY="${GITHUB_REPOSITORY:-loomarr/loomarr}"
REQUIRED=(
	'Self-hosted — Intel Arc (VAAPI)'
	'Self-hosted — NVIDIA GeForce (NVENC)'
)

[[ "$COMMIT" =~ ^[0-9a-f]{40}$ ]] || {
	echo 'usage: check-hardware-gate.sh <full commit id>' >&2
	exit 2
}

runs="$(gh api "repos/$REPOSITORY/actions/workflows/playout-bench.yml/runs?head_sha=$COMMIT&event=workflow_dispatch&per_page=100" \
	--jq '.workflow_runs[].id')"

passed=''
for run in $runs; do
	jobs="$(gh api "repos/$REPOSITORY/actions/runs/$run/jobs?per_page=100" \
		--jq '.jobs[] | select(.conclusion == "success") | .name')"
	passed+="$jobs"$'\n'
done

missing=0
for job in "${REQUIRED[@]}"; do
	if grep -Fxq -- "$job" <<<"$passed"; then
		printf 'hardware-gate: passed: %s\n' "$job"
	else
		printf 'hardware-gate: MISSING: %s\n' "$job" >&2
		missing=1
	fi
done
if [ "$missing" != 0 ]; then
	printf 'hardware-gate: run playout-bench.yml on %s with run_arc and run_geforce\n' "$COMMIT" >&2
	exit 1
fi
