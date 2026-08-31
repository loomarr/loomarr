#!/usr/bin/env bash
# Map a CI event to the assurance lane that owns its evidence.
set -euo pipefail

if (($# < 1 || $# > 2)); then
  printf 'usage: %s EVENT [MANUAL_SCOPE]\n' "$0" >&2
  exit 2
fi

event="$1"
scope="${2:-}"

case "$event" in
  pull_request)
    [[ -z "$scope" ]] || { echo 'ci-lane: pull_request does not accept a manual scope' >&2; exit 2; }
    echo 'lane=pr-fast'
    ;;
  merge_group)
    [[ -z "$scope" ]] || { echo 'ci-lane: merge_group does not accept a manual scope' >&2; exit 2; }
    echo 'lane=merge-queue'
    ;;
  workflow_dispatch)
    case "$scope" in
      release-candidate) echo 'lane=manual-release-candidate' ;;
      full) echo 'lane=manual-full' ;;
      apple-cache-validation) echo 'lane=manual-apple-cache-validation' ;;
      *) printf 'ci-lane: unsupported manual scope %q\n' "$scope" >&2; exit 2 ;;
    esac
    ;;
  *)
    printf 'ci-lane: unsupported event %q\n' "$event" >&2
    exit 2
    ;;
esac
