#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COLLECTOR="$ROOT/scripts/changed-paths.sh"
TEST_REPO="$(mktemp -d)"
trap 'rm -rf "$TEST_REPO"' EXIT

git -C "$TEST_REPO" init -q
git -C "$TEST_REPO" config user.email test@example.invalid
git -C "$TEST_REPO" config user.name 'Changed Paths Test'
printf 'base\n' >"$TEST_REPO/committed.txt"
printf 'base\n' >"$TEST_REPO/staged.txt"
printf 'base\n' >"$TEST_REPO/unstaged.txt"
git -C "$TEST_REPO" add .
git -C "$TEST_REPO" commit -qm base

printf 'branch\n' >"$TEST_REPO/committed.txt"
git -C "$TEST_REPO" add committed.txt
git -C "$TEST_REPO" commit -qm branch
printf 'staged\n' >"$TEST_REPO/staged.txt"
git -C "$TEST_REPO" add staged.txt
printf 'unstaged\n' >"$TEST_REPO/unstaged.txt"
printf 'untracked\n' >"$TEST_REPO/untracked.txt"

got="$(LOOMARR_REPO_ROOT="$TEST_REPO" "$COLLECTOR" HEAD~1)"
want="$(printf '%s\n' committed.txt staged.txt unstaged.txt untracked.txt)"
if [[ "$got" != "$want" ]]; then
	printf 'changed-paths-test: got:\n%s\nwant:\n%s\n' "$got" "$want" >&2
	exit 1
fi

git -C "$TEST_REPO" reset --hard -q HEAD
rm -f "$TEST_REPO/untracked.txt"
if [[ -n "$(LOOMARR_REPO_ROOT="$TEST_REPO" "$COLLECTOR" HEAD)" ]]; then
	echo 'changed-paths-test: clean tree reported changes' >&2
	exit 1
fi

echo 'changed-paths-test: ok'
