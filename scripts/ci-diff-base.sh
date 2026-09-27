#!/usr/bin/env bash
# Print the commit a CI event's changes are classified against, or nothing when there is no
# usable base. Callers MUST treat empty output as "select every gate": skipping because we could
# not work out what changed is the wrong failure direction.
#
# ⚠ `merge_group` carries NEITHER `pull_request.base.sha` NOR `before`. `merge_group.base_sha` is
# the main commit the batch is stacked on, so the queue selects the same gates as the PR it carries
# instead of failing closed into the full native matrix (#1570).
set -euo pipefail

if (($# != 2)); then
  printf 'usage: %s EVENT_NAME EVENT_PAYLOAD_PATH\n' "$0" >&2
  exit 2
fi

event="$1"
payload="$2"

case "$event" in
  pull_request) query='.pull_request.base.sha' ;;
  merge_group) query='.merge_group.base_sha' ;;
  *) query='.before' ;;
esac

base="$(jq -r "$query // empty" "$payload" 2>/dev/null)" || base=''
# A new branch reports the all-zero SHA; anything that is not a commit id fails closed too.
if [[ ! "$base" =~ ^[0-9a-f]{40}([0-9a-f]{24})?$ || "$base" =~ ^0+$ ]]; then
  base=''
fi
printf '%s' "$base"
