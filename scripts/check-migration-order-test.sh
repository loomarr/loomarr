#!/usr/bin/env bash
# Contract for scripts/check-migration-order.sh, against throwaway repositories: no network.

set -euo pipefail

# An inherited repository location (a git hook, a wrapper, go test's environment) would send every
# throwaway commit below into that repository instead.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_COMMON_DIR

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
  git config commit.gpgsign false
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

# A pull request takes its base from the event payload.
repo pull-request
add sqlite/00002_late.sql postgres/00002_late.sql
printf '{"pull_request":{"base":{"sha":"%s"}}}\n' "$base" >"$WORK/pull-request.json"
"$CHECK" pull_request "$WORK/pull-request.json" >/dev/null 2>&1 &&
  fail 'a pull request adding a migration below the head passed'
printf '{"pull_request":{"base":{"sha":"%s"}}}\n' "$(git rev-parse HEAD)" >"$WORK/pull-request.json"
"$CHECK" pull_request "$WORK/pull-request.json" >/dev/null 2>&1 ||
  fail 'a pull request adding no migration against its base was refused'

# A queue candidate is one squashed commit on the entry ahead of it. The entry ahead adds 00005,
# so this entry's 00004 lands below a migration installs have already run, though it is above
# main's newest.
# queued writes the payload a candidate's run gets: base_sha is main's tip, below the entry ahead.
queued() {
  printf '{"merge_group":{"base_sha":"%s"}}\n' "$base" >"$WORK/merge-group.json"
  "$CHECK" merge_group "$WORK/merge-group.json" 2>&1
}

repo queue-behind-higher
add sqlite/00005_ahead.sql postgres/00005_ahead.sql
add sqlite/00004_behind.sql postgres/00004_behind.sql
refusal="$(queued)" && fail 'a queue entry numbered below the entry ahead of it passed'
[[ "$refusal" == *'00004_behind.sql is numbered 00004'* ]] ||
  fail "a queue entry below the entry ahead of it was refused for another reason: $refusal"

repo queue-behind-same
add sqlite/00004_ahead.sql postgres/00004_ahead.sql
add sqlite/00004_behind.sql postgres/00004_behind.sql
refusal="$(queued)" && fail 'a queue entry sharing the number of the entry ahead of it passed'
[[ "$refusal" == *'00004_behind.sql is numbered 00004'* ]] ||
  fail "a queue entry sharing the entry ahead's number was refused for another reason: $refusal"

repo queue-behind-lower
add sqlite/00004_ahead.sql postgres/00004_ahead.sql
add sqlite/00005_behind.sql postgres/00005_behind.sql
queued >/dev/null || fail 'a queue entry numbered above the entry ahead of it was refused'

repo unusable
passes 0000000000000000000000000000000000000001 && fail 'an unusable base passed'
"$CHECK" >/dev/null 2>&1 && fail 'a call without arguments passed'

# A base without the migrations directory means it moved, so nothing would ever be compared.
repo moved
git rm --quiet -r internal/platform/store/migrations
git commit --quiet -m moved
moved_base="$(git rev-parse HEAD)"
mkdir -p internal/platform/store/migrations/sqlite
add sqlite/00001_again.sql
passes "$moved_base" && fail 'a base without migrations passed'

echo 'check-migration-order-test: ok'
