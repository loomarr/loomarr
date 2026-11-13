#!/usr/bin/env bash
# Print the merge gate's selection for a merge-queue candidate: `lane`, `go=true` when it touches
# Go, and the classifier's `visual` and `e2e` decisions, which select the Playwright shards. When
# the base can't be worked out, or the classifier fails, every gate is selected: skipping a gate
# because the base was unusable is the wrong failure direction.
set -euo pipefail

if (($# != 2)); then
  printf 'usage: %s EVENT_NAME EVENT_PAYLOAD_PATH\n' "$0" >&2
  exit 2
fi

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
"$root/scripts/ci-lane.sh" "$1"
base="$("$root/scripts/ci-diff-base.sh" "$1" "$2")"
# The checkout is shallow: fetch just the base commit. Two trees diff without their history, and a
# failed fetch fails closed below.
if [[ -n "$base" ]] && ! git cat-file -e "$base^{commit}" 2>/dev/null; then
  git fetch --quiet --no-tags --depth=1 origin "$base" 2>/dev/null || true
fi
if [[ -z "$base" ]] || ! git cat-file -e "$base^{commit}" 2>/dev/null; then
  printf '%s\n' go=true visual=true e2e=true
  exit 0
fi

changed="$(git diff --name-only "$base" HEAD)"
if grep -Eq '\.go$|(^|/)go\.(mod|sum)$' <<<"$changed"; then
  echo 'go=true'
else
  echo 'go=false'
fi
if ! impact="$("$root/scripts/ci-impact.sh" <<<"$changed")"; then
  impact="$("$root/scripts/ci-impact.sh" --all)"
fi
grep -E '^(visual|e2e)=' <<<"$impact"
