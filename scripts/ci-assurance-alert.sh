#!/usr/bin/env bash
# File an issue when `CI assurance` fails on a commit that reached main. Assurance finishes after
# the merge, so this issue is what stops a failed gate going unnoticed. A candidate the queue
# dropped never reached main, so its failure has nothing to report.
set -euo pipefail

repository="${GITHUB_REPOSITORY:?}"
sha="${GITHUB_SHA:?}"
run_url="${GITHUB_SERVER_URL:?}/$repository/actions/runs/${GITHUB_RUN_ID:?}"

status="$(gh api "repos/$repository/compare/main...$sha" --jq .status)"
if [[ "$status" != identical && "$status" != behind ]]; then
  echo "ci-assurance-alert: $sha is not on main ($status); nothing to report"
  exit 0
fi

gh issue create --repo "$repository" \
  --title "CI assurance failed on main at ${sha:0:9}" \
  --body "The complete gate set failed on a commit that has already merged. Revert it first and land the fix as a new PR; rerun if this is a flake or an outage: $run_url"
