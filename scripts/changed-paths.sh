#!/usr/bin/env bash
# Print every path changed from a base commit, including staged, unstaged, and untracked work.
set -euo pipefail

ROOT="${LOOMARR_REPO_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
BASE="${1:-origin/main}"

git -C "$ROOT" rev-parse --verify "$BASE^{commit}" >/dev/null 2>&1 || {
	printf 'changed-paths: unknown base %s\n' "$BASE" >&2
	exit 2
}

{
	git -C "$ROOT" diff --name-only "$BASE"...HEAD --
	git -C "$ROOT" diff --cached --name-only --
	git -C "$ROOT" diff --name-only --
	git -C "$ROOT" ls-files --others --exclude-standard
} | sed '/^$/d' | sort -u
