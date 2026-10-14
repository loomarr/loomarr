#!/usr/bin/env bash
# Guards the Makefile's hand-maintained TAGS list against the tree (GH #227 §1).
#
# `make vet-tags` and `make lint` only see the tags named in TAGS. A `//go:build newtag` file
# added without touching the Makefile is therefore invisible to the gate on the day it lands —
# the same silent drift `scripts/check-retired.sh` exists to catch, and the same one that let
# `TestLiveChain_RealFfmpegAdvancesThroughPrograms` sit uncompiled for months.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# Every tag IDENTIFIER mentioned in a build constraint, deduped.
#
# Tokenising on non-identifier characters means `&&`, `||`, `!` and parens act as separators,
# so a compound constraint (`//go:build ffmpeg && cgo`) contributes BOTH tags rather than
# parsing as one. That is deliberately over-inclusive: naming a tag the tree does not need
# costs nothing at vet time, while missing one recreates the blind spot.
tree_tags="$(
  grep -rhE '^//go:build ' --include='*.go' . 2>/dev/null \
    | sed 's|^//go:build ||' \
    | tr -c 'A-Za-z0-9_' '\n' \
    | grep -vE '^$' \
    | sort -u
)"

# The list the Makefile actually gates on.
# shellcheck disable=SC2086  # unquoted ON PURPOSE: TAGS is a space-separated list and the
# word splitting is what turns it into one tag per line for `comm`.
declared_tags="$(printf '%s\n' ${TAGS:-} | grep -vE '^$' | sort -u)"

# ---------------------------------------------------------------------------
# THE COMPARISON POLICY: both directions fail.
#
# `comm -23` = in the TREE, not DECLARED. Undisputed — those files are invisible to
# `make vet-tags` and `make lint`, which is the drift that let
# `TestLiveChain_RealFfmpegAdvancesThroughPrograms` sit uncompiled for months.
#
# `comm -13` = DECLARED, not in the tree. This was the script's one open question, and the
# answer is the same, for a reason the alternatives argue for themselves:
#   - warn   → a warning printed by a job that exits 0 is one nobody reads
#   - ignore → a stale entry costs nothing at vet time, and the list quietly stops meaning
#              what it says. That is precisely how the retired-identifier list rotted.
# The objection to failing is that deleting the last `//go:build eval` file reddens an
# otherwise-unrelated PR. It does — and that PR is exactly where the tag should be dropped,
# the same same-PR discipline `check-retired.sh` already demands. A list that may overstate
# its coverage is not a guard; it is a claim.
# ---------------------------------------------------------------------------

fail=0

while IFS= read -r tag; do
  [[ -z "$tag" ]] && continue
  fail=1
  printf '\nUNDECLARED BUILD TAG: %s\n' "$tag"
  printf '  These files are skipped by "make vet-tags" and "make lint" — nothing compiles them.\n'
  printf '  Fix: add %s to TAGS in the Makefile.\n\n' "$tag"
  grep -rlE "^//go:build .*\b${tag}\b" --include='*.go' . 2>/dev/null | sed 's|^\./|    |'
done < <(comm -23 <(printf '%s\n' "$tree_tags") <(printf '%s\n' "$declared_tags"))

while IFS= read -r tag; do
  [[ -z "$tag" ]] && continue
  fail=1
  printf '\nDECLARED BUILD TAG NOT IN THE TREE: %s\n' "$tag"
  printf '  TAGS claims coverage for a tag no //go:build line uses, so the list overstates\n'
  printf '  what the gate actually sees.\n'
  printf '  Fix: remove %s from TAGS in the Makefile, in the PR that removed its last file.\n' "$tag"
done < <(comm -13 <(printf '%s\n' "$tree_tags") <(printf '%s\n' "$declared_tags"))

if [[ "$fail" -ne 0 ]]; then
  printf '\ntree tags:     %s\ndeclared TAGS: %s\n' \
    "$(echo "$tree_tags" | tr '\n' ' ')" "$(echo "$declared_tags" | tr '\n' ' ')"
  exit 1
fi

printf 'tags-verify: clean (%d tags: %s)\n' \
  "$(echo "$declared_tags" | grep -c .)" "$(echo "$declared_tags" | tr '\n' ' ')"
