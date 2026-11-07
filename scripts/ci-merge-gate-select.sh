#!/usr/bin/env bash
# Print `changed=true` when a merge-queue candidate touches Go, or when that cannot be worked out:
# skipping the compile because the base was unusable is the wrong failure direction.
set -euo pipefail

if (($# != 2)); then
  printf 'usage: %s EVENT_NAME EVENT_PAYLOAD_PATH\n' "$0" >&2
  exit 2
fi

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
base="$("$root/scripts/ci-diff-base.sh" "$1" "$2")"
# The checkout is shallow: fetch just the base commit. Two trees diff without their history, and a
# failed fetch fails closed below.
if [[ -n "$base" ]] && ! git cat-file -e "$base^{commit}" 2>/dev/null; then
  git fetch --quiet --no-tags --depth=1 origin "$base" 2>/dev/null || true
fi
if [[ -z "$base" ]] || ! git cat-file -e "$base^{commit}" 2>/dev/null; then
  echo 'changed=true'
  exit 0
fi

if git diff --name-only "$base" HEAD | grep -Eq '\.go$|(^|/)go\.(mod|sum)$'; then
  echo 'changed=true'
else
  echo 'changed=false'
fi
