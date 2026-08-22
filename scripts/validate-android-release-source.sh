#!/usr/bin/env bash

set -euo pipefail

: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
: "${GITHUB_SHA:?GITHUB_SHA is required}"
: "${GITHUB_REF:?GITHUB_REF is required}"

if [[ "$GITHUB_REF" != refs/heads/main ]]; then
	echo 'android release: workflow must run from the main branch' >&2
	exit 1
fi

head=$(git rev-parse HEAD)
if [[ "$head" != "$GITHUB_SHA" ]]; then
	echo 'android release: checked-out source does not match the workflow commit' >&2
	exit 1
fi
git fetch --no-tags origin main
if [[ "$head" != "$(git rev-parse origin/main)" ]]; then
	echo 'android release: source must be the current main commit' >&2
	exit 1
fi

run=$(
	gh api \
		"/repos/${GITHUB_REPOSITORY}/actions/workflows/ci.yml/runs?head_sha=${GITHUB_SHA}&per_page=20" \
		--jq '[.workflow_runs[] | select(.head_branch == "main" and (.event == "push" or .event == "workflow_dispatch"))] | sort_by(.created_at) | last | [.id, .conclusion] | @tsv'
)
IFS=$'\t' read -r run_id conclusion <<<"$run"
if [[ "$conclusion" != success ]]; then
	echo "android release: main commit does not have a successful CI run (found: ${conclusion:-none})" >&2
	exit 1
fi

jobs=$(
	gh api --paginate \
		"/repos/${GITHUB_REPOSITORY}/actions/runs/${run_id}/jobs?per_page=100" \
		--jq '.jobs[] | [.name, .conclusion] | @tsv'
)
if ! grep -Fqx $'Android TV — lint + unit + assemble\tsuccess' <<<"$jobs"; then
	echo 'android release: successful CI run did not execute the Android gate' >&2
	exit 1
fi

echo "android release: source commit $GITHUB_SHA has a successful Android CI gate"
