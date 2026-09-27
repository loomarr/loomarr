#!/usr/bin/env bash
# Contract for scripts/ci-diff-base.sh: every CI event resolves to the commit its gates must be
# classified against, and the merge queue is path-filtered exactly like the PR it carries (#1570).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RESOLVER="$ROOT/scripts/ci-diff-base.sh"
CLASSIFIER="$ROOT/scripts/ci-impact.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

fail() {
  printf 'ci-diff-base-test: %s\n' "$*" >&2
  exit 1
}

# Payload fragments in the shapes GitHub sends. A pull_request synchronize also carries `before`
# (the previous push); the PR base, not the previous push, is the right diff base.
event() {
  local name="$1" body="$2"
  printf '%s\n' "$body" > "$tmp/$name.json"
  printf '%s' "$tmp/$name.json"
}

assert_base() {
  local label="$1" event_name="$2" payload="$3" want="$4" got
  got="$("$RESOLVER" "$event_name" "$payload")"
  [[ "$got" == "$want" ]] || fail "$label: got '$got', want '$want'"
}

a=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
b=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
c=cccccccccccccccccccccccccccccccccccccccc
zero=0000000000000000000000000000000000000000

assert_base 'pull_request uses the PR base, not the previous push' pull_request \
  "$(event pr "{\"before\":\"$c\",\"pull_request\":{\"base\":{\"sha\":\"$a\"},\"head\":{\"sha\":\"$b\"}}}")" "$a"
# ⚠ merge_group carries NEITHER pull_request.base.sha NOR before. Without base_sha the queue fails
# closed and every batch pays the full native matrix.
assert_base 'merge_group uses the queue base' merge_group \
  "$(event mg "{\"merge_group\":{\"base_sha\":\"$a\",\"head_sha\":\"$b\",\"base_ref\":\"refs/heads/main\"}}")" "$a"
assert_base 'push uses the previous head' push "$(event push "{\"before\":\"$a\",\"after\":\"$b\"}")" "$a"

# Fail closed: no usable base prints nothing, and the caller then selects every gate.
assert_base 'merge_group without base_sha' merge_group "$(event mg-bare '{"merge_group":{"head_sha":"'"$b"'"}}')" ''
assert_base 'new branch push (zero before)' push "$(event push-new "{\"before\":\"$zero\"}")" ''
assert_base 'workflow_dispatch has no diff base' workflow_dispatch "$(event dispatch '{"inputs":{"scope":"full"}}')" ''
assert_base 'unreadable payload' merge_group "$tmp/missing.json" ''
assert_base 'non-commit base' merge_group "$(event mg-bad '{"merge_group":{"base_sha":"main; true"}}')" ''
if "$RESOLVER" merge_group >/dev/null 2>&1; then
  fail 'accepted a call without an event payload path'
fi

# End to end: a queue batch stacked on main is classified against main, so it selects only what
# its own PR changed. This is the property that keeps a docs-only or Go-only batch off the
# 20-30 minute native builds.
repo="$tmp/repo"
git init -q "$repo"
git -C "$repo" -c user.email=ci@example.invalid -c user.name=ci commit -q --allow-empty -m main
main_sha="$(git -C "$repo" rev-parse HEAD)"
queue_payload="$(event mg-real "{\"merge_group\":{\"base_sha\":\"$main_sha\",\"head_sha\":\"unused\"}}")"

assert_batch_gates() {
  local label="$1" want="$2" path got base
  shift 2
  git -C "$repo" checkout -q --detach "$main_sha"
  for path in "$@"; do
    mkdir -p "$repo/$(dirname "$path")"
    printf 'change\n' > "$repo/$path"
  done
  git -C "$repo" add -A
  git -C "$repo" -c user.email=ci@example.invalid -c user.name=ci commit -q -m batch
  base="$("$RESOLVER" merge_group "$queue_payload")"
  got="$(git -C "$repo" diff --name-only "$base" HEAD | "$CLASSIFIER" | sed -n 's/=true$//p' | paste -sd, -)"
  [[ "$got" == "$want" ]] || fail "$label queue batch: got gates '$got', want '$want'"
}

assert_batch_gates docs-only docs docs/research/ci-wall-clock.md
assert_batch_gates go-only contracts,go,postgres,image internal/suggest/score.go

echo 'ci-diff-base-test: ok'
