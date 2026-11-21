#!/usr/bin/env bash
# Contract for scripts/check-migration-order.sh, against throwaway repositories: no network.

set -euo pipefail

ROOT="$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd -P)"
CHECK="$ROOT/scripts/check-migration-order.sh"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

fail() {
  printf 'check-migration-order-test: %s\n' "$1" >&2
  exit 1
}

# repo moves into a new repository whose base commit holds migrations 00001 and 00003 in both
# dialects (a gap, as main has), and sets base to that commit. It runs in this shell, not a
# subshell, so the commits that follow land in the throwaway repository.
repo() {
  local at="$WORK/$1"
  mkdir -p "$at/internal/platform/store/migrations/"{sqlite,postgres}
  cd "$at"
  git init --quiet
  git config user.email test@example.invalid
  git config user.name test
  for dialect in sqlite postgres; do
    : >"internal/platform/store/migrations/$dialect/00001_first.sql"
    : >"internal/platform/store/migrations/$dialect/00003_third.sql"
  done
  : >README.md
  git add -A
  git commit --quiet -m base
  base="$(git rev-parse HEAD)"
}

# add creates migration files and commits them on top of the base. It refuses to run outside the
# throwaway repositories, so a mistake here can never commit to the checkout under test.
add() {
  local path
  [[ "$PWD" == "$WORK"/* ]] || fail "add ran outside $WORK, in $PWD"
  for path in "$@"; do
    : >"internal/platform/store/migrations/$path"
  done
  git add -A
  git commit --quiet -m change
}

passes() { "$CHECK" --base "$1" >/dev/null 2>&1; }

repo next-free
add sqlite/00004_next.sql postgres/00004_next.sql
passes "$base" || fail 'the next free number in both dialects was refused'

repo below-head
add sqlite/00002_late.sql postgres/00002_late.sql
passes "$base" && fail 'a migration in the gap below the head passed'
refusal="$("$CHECK" --base "$base" 2>&1)" || true
[[ "$refusal" == *'the next free number is 00004'* ]] || fail 'a refusal does not name the next free number'

repo equal-head
add sqlite/00004_ok.sql postgres/00003_clash.sql
passes "$base" && fail 'a Postgres migration equal to the head passed'

repo duplicate
add sqlite/00004_one.sql sqlite/00004_two.sql postgres/00004_one.sql
passes "$base" && fail 'two added migrations sharing one number passed'

repo no-migration
printf 'more\n' >README.md
git commit --quiet -am docs
passes "$base" || fail 'a change without migrations was refused'

repo unnumbered
add sqlite/README.md
passes "$base" || fail 'a file goose does not number was read as a migration'

# Renumbering an unmerged migration above the head passes once the low number is gone.
repo renumber
add sqlite/00002_late.sql postgres/00002_late.sql
git mv internal/platform/store/migrations/sqlite/00002_late.sql internal/platform/store/migrations/sqlite/00004_late.sql
git mv internal/platform/store/migrations/postgres/00002_late.sql internal/platform/store/migrations/postgres/00004_late.sql
git commit --quiet -m renumber
passes "$base" || fail 'a migration renumbered above the head was refused'

repo unusable
passes 0000000000000000000000000000000000000001 && fail 'an unusable base passed'
"$CHECK" >/dev/null 2>&1 && fail 'a call without arguments passed'

echo 'check-migration-order-test: ok'
