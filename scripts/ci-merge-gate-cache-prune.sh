#!/usr/bin/env bash
# Delete every merge-gate Go cache except the one just saved, so the gate holds a single entry of the
# repository's cache quota.
set -euo pipefail

keep="${1:?usage: ci-merge-gate-cache-prune.sh KEEP_KEY}"
repository="${REPO:?}"

gh api "repos/$repository/actions/caches?key=merge-gate-go-&per_page=100" \
  --jq ".actions_caches[] | select(.key != \"$keep\") | .id" |
  while read -r id; do
    gh api -X DELETE "repos/$repository/actions/caches/$id"
  done
