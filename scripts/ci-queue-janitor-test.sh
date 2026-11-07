#!/usr/bin/env bash
# Prove which merge-queue runs the janitor stops, against a fake GitHub API.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fake_bin="$TMP/bin"
mkdir -p "$fake_bin"

# The fake serves API-shaped JSON from files and applies the caller's own --jq filter to it, so the
# janitor's real filters run. A `fail-<kind>` file makes that read fail. Cancels land in
# cancelled.txt.
# shellcheck disable=SC2016 # JANITOR_STATE expands when the generated fake executes.
printf '%s\n' \
  '#!/usr/bin/env bash' \
  'set -euo pipefail' \
  'state="$JANITOR_STATE"' \
  'path=""' \
  'filter="."' \
  'while (($#)); do' \
  '  case "$1" in' \
  '    repos/*) path="$1" ;;' \
  '    --jq) filter="$2"; shift ;;' \
  '  esac' \
  '  shift' \
  'done' \
  'serve() { jq -r "$filter" "$1"; }' \
  'case "$path" in' \
  '  */actions/runs/*/cancel)' \
  '    id="${path%/cancel}"; printf "%s\n" "${id##*/}" >> "$state/cancelled.txt" ;;' \
  '  *status=in_progress*)' \
  '    [[ ! -e "$state/fail-runs" ]] || exit 1' \
  '    serve "$state/runs.json" ;;' \
  '  *status=queued*) serve "$state/queued.json" ;;' \
  '  */git/matching-refs/*)' \
  '    [[ ! -e "$state/fail-refs" ]] || exit 1' \
  '    serve "$state/refs.json" ;;' \
  '  */jobs*)' \
  '    [[ ! -e "$state/fail-jobs" ]] || exit 1' \
  '    id="${path%/jobs*}"; serve "$state/jobs-${id##*/}.json" ;;' \
  '  *) echo "fake gh: unexpected call: $path" >&2; exit 2 ;;' \
  'esac' > "$fake_bin/gh"
chmod +x "$fake_bin/gh"

reset_state() {
  rm -rf "$TMP/state"
  mkdir -p "$TMP/state"
  echo '{"workflow_runs":[]}' > "$TMP/state/runs.json"
  echo '{"workflow_runs":[]}' > "$TMP/state/queued.json"
  echo '[]' > "$TMP/state/refs.json"
  : > "$TMP/state/cancelled.txt"
}

append() {
  local file="$TMP/state/$1"
  jq "$2" "$file" > "$file.next"
  mv "$file.next" "$file"
}

add_run() {
  local list="${2:-runs.json}"
  append "$list" ".workflow_runs += [{id: $1, head_branch: \"gh-readonly-queue/main/pr-$1\"}]"
  echo '{"jobs":[]}' > "$TMP/state/jobs-$1.json"
}

add_live_ref() {
  append refs.json ". += [{ref: \"refs/heads/gh-readonly-queue/main/pr-$1\"}]"
}

add_job() {
  append "jobs-$1.json" ".jobs += [{name: \"$2\", conclusion: $3}]"
}

run_janitor() {
  PATH="$fake_bin:$PATH" JANITOR_STATE="$TMP/state" GITHUB_REPOSITORY=test/repo \
    QUEUE_JANITOR_MINUTES=0 "$ROOT/scripts/ci-queue-janitor.sh" >/dev/null
}

expect_cancelled() {
  local label="$1" want="$2" got
  got="$(sort -n "$TMP/state/cancelled.txt" | paste -sd' ' -)"
  if [[ "$got" != "$want" ]]; then
    printf 'ci-queue-janitor-test: %s: cancelled [%s], want [%s]\n' "$label" "$got" "$want" >&2
    exit 1
  fi
}

reset_state
add_run 1
add_live_ref 1
add_job 1 'Go' '"success"'
add_job 1 'Docs' '"skipped"'
add_job 1 'Apple' null
run_janitor
expect_cancelled 'a healthy live run' ''

reset_state
add_run 1
add_run 2
add_live_ref 2
run_janitor
expect_cancelled 'a run whose group is gone beside a live one' '1'

reset_state
add_run 3 queued.json
add_live_ref 4
run_janitor
expect_cancelled 'a queued run whose group is gone' '3'

reset_state
add_run 1
add_live_ref 1
add_job 1 'Playwright (2/4)' '"failure"'
add_job 1 'Apple' null
run_janitor
expect_cancelled 'a live run with a failed job' '1'

reset_state
add_run 1
add_live_ref 1
add_job 1 'Go (1/2)' '"cancelled"'
run_janitor
expect_cancelled 'a live run with a timed-out lane' '1'

reset_state
add_run 1
add_live_ref 1
add_job 1 'CI' '"failure"'
run_janitor
expect_cancelled 'a run whose only failure is the verdict' ''

reset_state
add_run 1
run_janitor
expect_cancelled 'an empty ref list says nothing about a run' ''

reset_state
add_run 1
touch "$TMP/state/fail-refs"
run_janitor
expect_cancelled 'a failed ref read' ''

reset_state
add_run 1
add_live_ref 1
touch "$TMP/state/fail-jobs"
run_janitor
expect_cancelled 'a failed job read' ''

reset_state
touch "$TMP/state/fail-runs"
run_janitor
expect_cancelled 'a failed run read' ''

reset_state
run_janitor
expect_cancelled 'an idle queue' ''

echo 'ci-queue-janitor-test: ok'
