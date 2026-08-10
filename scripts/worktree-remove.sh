#!/usr/bin/env bash
# WorktreeRemove hook — tears down a sibling worktree created by worktree-create.sh.
#
# CONTRACT (docs/en/hooks): side-effect only. Exit codes are IGNORED and failures are
# logged in debug mode only, so this script can never block removal and never reports an
# error anywhere a human will see it. Everything here is therefore best-effort by
# construction — the opposite of worktree-create.sh, where any non-zero exit is fatal.
#
# ⚠ It must still be CAREFUL, precisely because nothing checks it. `git worktree remove`
# without --force refuses when the tree has uncommitted changes or untracked files, and
# that refusal is the desired behaviour: losing an agent's unpushed work to an automatic
# cleanup is far worse than leaving a directory on disk. Do not add --force here.
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

payload="$(cat)"
dir="$(printf '%s' "$payload" | jq -r '.worktreePath // .path // empty' 2>/dev/null)"
[ -n "$dir" ] || exit 0
[ -d "$dir" ] || exit 0

# Only ever touch siblings we could have created. A payload naming something else — the
# main checkout, a path outside Projects/ — is ignored rather than acted on.
case "$dir" in
  "$REPO") exit 0 ;;
  "$(cd "$REPO/.." && pwd)/$(basename "$REPO")-"*) ;;
  *) exit 0 ;;
esac

# No --force: a worktree holding real work stays on disk. `git worktree prune` then clears
# the administrative entry if the directory did go away, so `git worktree list` cannot
# accumulate the phantom registrations that manual removal leaves behind.
git -C "$REPO" worktree remove "$dir" >&2 2>/dev/null || \
  echo "worktree-remove: left $dir in place (it has uncommitted or untracked work)" >&2
git -C "$REPO" worktree prune >/dev/null 2>&1 || true
exit 0
