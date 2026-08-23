#!/usr/bin/env bash
set -euo pipefail

tag=${1:?usage: generate-release-notes.sh TAG OUTPUT}
output=${2:?usage: generate-release-notes.sh TAG OUTPUT}
repo=${GITHUB_REPOSITORY:-$(gh repo view --json nameWithOwner --jq .nameWithOwner)}
tmp_dir=$(mktemp -d)
trap 'rm -rf -- "$tmp_dir"' EXIT

args=(--method POST "repos/${repo}/releases/generate-notes" -f "tag_name=${tag}")
if [ -n "${PREVIOUS_TAG:-}" ]; then
  args+=(-f "previous_tag_name=${PREVIOUS_TAG}")
fi
gh api "${args[@]}" --jq .body >"${tmp_dir}/github.md"

command=(go run ./cmd/release-notes --input "${tmp_dir}/github.md" --output "$output")
header="docs/release/${tag}.md"
if [ -f "$header" ]; then
  command+=(--header "$header")
fi
"${command[@]}"
