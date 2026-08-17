#!/usr/bin/env bash

set -euo pipefail

: "${GITHUB_REF_NAME:?GITHUB_REF_NAME is required}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
: "${GITHUB_SHA:?GITHUB_SHA is required}"

./scripts/check-release-tag.sh "$GITHUB_REF_NAME"
head=$(git rev-parse HEAD)
if [[ "$head" != "$GITHUB_SHA" ]]; then
  echo "checked-out release source does not match the triggering tag commit" >&2
  exit 1
fi
git fetch --no-tags origin main
if [[ "$head" != "$(git rev-parse origin/main)" ]]; then
  echo "release tag must point at the current main commit" >&2
  exit 1
fi

run=$(gh api \
  "/repos/${GITHUB_REPOSITORY}/actions/workflows/ci.yml/runs?head_sha=${GITHUB_SHA}&per_page=20" \
  --jq '[.workflow_runs[] | select(.head_branch == "main" and (.event == "push" or .event == "workflow_dispatch"))] | sort_by(.created_at) | last | [.id, .conclusion] | @tsv')
IFS=$'\t' read -r run_id conclusion <<< "$run"
if [[ "$conclusion" != success ]]; then
  echo "tagged main commit does not have a successful CI run (found: ${conclusion:-none})" >&2
  exit 1
fi

jobs=$(gh api --paginate \
  "/repos/${GITHUB_REPOSITORY}/actions/runs/${run_id}/jobs?per_page=100" \
  --jq '.jobs[] | [.name, .conclusion] | @tsv')
for platform in linux/amd64 linux/arm64; do
  if ! grep -Fqx "Image — release build (${platform})"$'\t'"success" <<< "$jobs"; then
    echo "successful CI run did not prove the release image for ${platform}" >&2
    exit 1
  fi
done
