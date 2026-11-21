#!/usr/bin/env bash
# Fail when a change adds a migration numbered at or below the newest one on its base, or two
# migrations with one number, in either dialect (#4450). goose refuses to start a database that
# already holds a version newer than a file it has not applied, so such a migration passes every
# test on a fresh database and stops an upgraded install at boot.
#
# A pull request is checked against the main it was opened on. The merge queue checks each
# candidate against the main it is about to join, where the entries queued ahead of it are part of
# the change: two pull requests that took the same next number collide there.
#
# Usage: check-migration-order.sh EVENT_NAME EVENT_PAYLOAD_PATH   the base comes from the CI event
#        check-migration-order.sh --base REF                       an explicit base, e.g. origin/main
#
# An unusable base fails: passing a change because its base could not be read is the wrong failure
# direction. Run it only for events that have a base (pull_request, merge_group).
set -euo pipefail

usage() {
  printf 'usage: %s EVENT_NAME EVENT_PAYLOAD_PATH | --base REF\n' "$0" >&2
  exit 2
}

(($# == 2)) || usage
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ "$1" == --base ]]; then
  base="$2"
else
  base="$("$root/scripts/ci-diff-base.sh" "$1" "$2")"
fi

# A CI checkout is shallow: fetch just the base commit. Two trees compare without their history.
if [[ -n "$base" ]] && ! git cat-file -e "$base^{commit}" 2>/dev/null; then
  git fetch --quiet --no-tags --depth=1 origin "$base" 2>/dev/null || true
fi
if [[ -z "$base" ]] || ! git cat-file -e "$base^{commit}" 2>/dev/null; then
  printf 'check-migration-order: no usable base (%s); cannot tell whether a migration is out of order\n' "${base:-none}" >&2
  exit 1
fi

dir=internal/platform/store/migrations
# version prints the number a migration file name starts with, without leading zeros, or nothing
# for a file goose does not number.
version() {
  local name="${1##*/}"
  [[ "$name" =~ ^([0-9]+)_.*\.(sql|go)$ ]] || return 0
  printf '%d\n' "$((10#${BASH_REMATCH[1]}))"
}

problems=()
for dialect in sqlite postgres; do
  head=0
  while IFS= read -r path; do
    v="$(version "$path")"
    if [[ -n "$v" ]] && ((v > head)); then head="$v"; fi
  done < <(git ls-tree --name-only "$base" "$dir/$dialect/")

  # A rename is a removal and an addition here, so a renumbered migration is checked at its new
  # number.
  seen=" "
  while IFS= read -r path; do
    v="$(version "$path")"
    [[ -n "$v" ]] || continue
    if ((v <= head)); then
      problems+=("$path is numbered $(printf '%05d' "$v"), at or below $dialect's newest on the base ($(printf '%05d' "$head")); the next free number is $(printf '%05d' "$((head + 1))")")
    fi
    if [[ "$seen" == *" $v "* ]]; then
      problems+=("$path shares $(printf '%05d' "$v") with another $dialect migration this change adds")
    fi
    seen+="$v "
  done < <(git diff --name-only --no-renames --diff-filter=A "$base" HEAD -- "$dir/$dialect/")
done

if ((${#problems[@]} > 0)); then
  printf 'check-migration-order: a migration would stop an upgraded database from starting (#4450):\n' >&2
  printf '  %s\n' "${problems[@]}" >&2
  printf 'Renumber it in both dialects and fix every reference to the old number.\n' >&2
  exit 1
fi
echo 'check-migration-order: every added migration is numbered above the base'
