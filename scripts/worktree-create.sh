#!/usr/bin/env bash
# WorktreeCreate hook — creates a Claude Code worktree as a SIBLING of the repo.
#
# Wired via `.claude/settings.json` (see CLAUDE.md). Claude Code runs this instead of its
# own `git worktree` logic for `--worktree`, `EnterWorktree`, `isolation: worktree`
# subagents, and background sessions.
#
# CONTRACT (docs/en/hooks): JSON on stdin, the worktree PATH on stdout, and **any non-zero
# exit fails worktree creation**. That last part is unusual — most hooks treat only exit 2
# as blocking — and it drives the whole structure of this script.
#
# ⚠ WHY A HOOK AT ALL: Claude Code places worktrees under `.claude/worktrees/` INSIDE the
# repo by default, and `make e2e` bind-mounts the repo ROOT into the Playwright container.
# An in-repo worktree is therefore mounted into every e2e run, carrying its ~450MB
# node_modules with it. Siblings avoid that, which is why CLAUDE.md has always required
# them — this hook is what makes the requirement true for the native paths too, instead of
# a rule that `--worktree` silently violated.
#
# ⚠ THIS SCRIPT MUST COPY `.env` ITSELF. `.worktreeinclude` — the native way to carry
# gitignored files into a worktree — is NOT processed when a WorktreeCreate hook is
# installed, because the hook replaces the default git behaviour wholesale. Do not add a
# `.worktreeinclude` file expecting it to work; it would sit there looking load-bearing and
# do nothing.
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO" || { echo "worktree-create: cannot cd to $REPO" >&2; exit 1; }

# The hook payload carries the requested worktree name. Fall back to a timestamp-free
# random-ish name only if the field is absent, since a collision is better diagnosed than
# guessed at.
payload="$(cat)"
name="$(printf '%s' "$payload" | jq -r '.name // empty' 2>/dev/null)"
if [ -z "$name" ]; then
  echo "worktree-create: no .name in hook payload; cannot choose a directory" >&2
  exit 1
fi

# Sibling of the repo, matching CLAUDE.md's `../loomarr-<name>` convention.
dir="$(cd "$REPO/.." && pwd)/$(basename "$REPO")-${name}"

# ---- Hard failures: without these the worktree is not usable at all --------------------

if [ -e "$dir" ]; then
  # Reusing an existing directory is Claude Code's documented behaviour for a repeated
  # name, so hand it back rather than failing.
  if git -C "$dir" rev-parse --git-dir >/dev/null 2>&1; then
    printf '%s\n' "$dir"
    exit 0
  fi
  echo "worktree-create: $dir exists but is not a git worktree" >&2
  exit 1
fi

# Branch from the remote default so a new worktree starts clean, matching the `"fresh"`
# base Claude Code uses by default. `git worktree add -b` fails loudly if the branch exists.
base="origin/main"
git rev-parse --verify --quiet "$base" >/dev/null || base="main"
if ! git worktree add "$dir" -b "$name" "$base" >&2; then
  echo "worktree-create: git worktree add failed" >&2
  exit 1
fi

# ⚠ `.env` is gitignored and holds the live homelab credentials (LIBRARY_TOKEN,
# SEERR_API_KEY, TUNARR_API_KEY, …). Without it a worktree fails on anything touching the
# real stack, and the failure reads like a code bug rather than missing config — which is
# exactly what happened to `loomarr-msw`. DATABASE_URL is a RELATIVE sqlite path, so each
# worktree still gets its own database; the dev DBs are deliberately not copied.
#
# ⚠ SOURCE FROM THE MAIN CHECKOUT, NOT FROM `$REPO`. `$REPO` is wherever this script was
# invoked from, which may itself be a worktree that has no `.env` — and then a worktree
# without credentials silently creates more worktrees without credentials. Caught by
# testing rather than reading: run from a fresh worktree, the copy was a no-op and the new
# tree came out `.env`-less exactly like `loomarr-msw`. `--git-common-dir` always resolves
# to the MAIN checkout's `.git`, from any worktree, so its parent is the canonical source.
common="$(git rev-parse --git-common-dir 2>/dev/null)" || common=""
if [ -n "$common" ]; then
  MAIN="$(cd "$(dirname "$(cd "$common" && pwd)")" && pwd)"
else
  MAIN="$REPO"
fi

copied_env=0
for f in .env .phase0.env; do
  for src in "$MAIN/$f" "$REPO/$f"; do
    if [ -f "$src" ]; then
      cp "$src" "$dir/$f" || { echo "worktree-create: failed to copy $f" >&2; exit 1; }
      [ "$f" = ".env" ] && copied_env=1
      break
    fi
  done
done

# Loud, not fatal: a tree with no `.env` anywhere is a legitimate state (a CI checkout, a
# clone that never configured one), but the agent must be told rather than discover it as
# a puzzling runtime failure three steps later.
[ "$copied_env" -eq 1 ] || \
  echo "worktree-create: WARNING no .env found in $MAIN — live-stack work will fail until you add one" >&2

# ---- Best effort: a worktree without these is degraded, not broken ---------------------
#
# ⚠ NOTHING BELOW MAY FAIL THE SCRIPT. Any non-zero exit aborts worktree creation, so a
# flaky network or a pnpm hiccup would leave the agent with NO worktree instead of one
# missing node_modules. A worktree you can `pnpm install` in is recoverable in seconds; a
# worktree that was never created costs the whole session's setup. Hence `|| true` and a
# warning on stderr, which Claude sees.
{
  cd "$dir/web" 2>/dev/null || exit 0
  npx pnpm@11.13.1 install --frozen-lockfile >&2 || {
    echo "worktree-create: WARNING pnpm install failed — run it yourself in $dir/web" >&2
    exit 0
  }
  # REQUIRED for typechecks: packages/api/generated is gitignored, so every @loomarr/api
  # import fails to resolve in a fresh worktree until codegen runs. The install succeeding
  # while this is skipped is the confusing state CLAUDE.md warns about.
  npx pnpm@11.13.1 codegen >&2 || {
    echo "worktree-create: WARNING pnpm codegen failed — run it yourself in $dir/web" >&2
    exit 0
  }
} || true

# The path is the hook's actual output; everything above went to stderr on purpose.
printf '%s\n' "$dir"
