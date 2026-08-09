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
# TODO(maintainer): the comparison policy — see the note in the session, this is
# the one genuine judgment call in this script.
#
# `comm -23` gives tags in the TREE but not DECLARED (undisputed: must fail — that is
# the drift that recreates the bug). `comm -13` gives tags DECLARED but not in the tree.
#
# The open question is what the second case should do:
#   - fail  → the list can never claim coverage it does not have, but deleting the last
#             `//go:build eval` file turns CI red on an otherwise-unrelated PR
#   - warn  → honest and non-blocking, but a warning in a green job is one nobody reads
#   - ignore→ a stale tag costs nothing at vet time; the list just slowly stops meaning
#             what it says, which is how the retired-identifier list rotted
#
# Fill in below. Exit non-zero with a message naming the offending tag(s) and what to do
# about it — check-retired.sh's failure text is the model: it says which file, which
# identifier, and what to use instead.
# ---------------------------------------------------------------------------

echo "tree tags:     $(echo "$tree_tags" | tr '\n' ' ')"
echo "declared TAGS: $(echo "$declared_tags" | tr '\n' ' ')"
