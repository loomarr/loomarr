#!/usr/bin/env bash
# Stop merge-queue CI runs that can no longer merge, so their runners go to runs that can.
#
# Two kinds of run qualify. A run whose merge group is gone: when an entry leaves the queue, GitHub
# deletes the ref of every group behind it and starts replacements, but it leaves the old runs
# running. And a run with a job that already ended unsuccessfully: `CI` waits for every other job,
# so without this the queue learns of the failure only when the slowest job finishes.
#
# A healthy run is never touched. Every doubt leaves a run alone: an API read that fails skips the
# sweep, and an empty ref list says nothing about any one run. Stopping a run does not decide its
# verdict. `CI` still runs after the cancel and fails on the failed job.
set -euo pipefail

repository="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
interval="${QUEUE_JANITOR_INTERVAL_SECONDS:-60}"
deadline=$((SECONDS + ${QUEUE_JANITOR_MINUTES:-50} * 60))
stopped=' '

active_runs() {
  local status
  for status in in_progress queued; do
    gh api "repos/$repository/actions/workflows/ci.yml/runs?event=merge_group&status=$status&per_page=50" \
      --jq '.workflow_runs[] | [.id, .head_branch] | @tsv'
  done
}

# Ends with the first job that dooms the run. `CI` is the verdict itself, not a cause of one.
ended_job() {
  gh api "repos/$repository/actions/runs/$1/jobs?per_page=100" --jq '
    [.jobs[] | select(.name != "CI" and (.conclusion == "failure" or .conclusion == "cancelled"))
      | .name] | first // empty'
}

stop_run() {
  [[ "$stopped" != *" $1 "* ]] || return 0
  stopped+="$1 "
  printf 'ci-queue-janitor: stopping run %s: %s\n' "$1" "$2"
  gh api --method POST "repos/$repository/actions/runs/$1/cancel" >/dev/null ||
    printf 'ci-queue-janitor: could not stop run %s\n' "$1" >&2
}

# Returns non-zero once the queue has nothing running. The ref list is read after the run list, so
# a run that started between the two reads still finds its ref.
sweep() {
  local runs live id branch job
  runs="$(active_runs)" || return 0
  [[ -n "$runs" ]] || return 1
  live="$(gh api --paginate "repos/$repository/git/matching-refs/heads/gh-readonly-queue/" --jq '.[].ref')" || return 0
  while IFS=$'\t' read -r id branch; do
    if [[ -n "$live" ]] && ! grep -qxF "refs/heads/$branch" <<<"$live"; then
      stop_run "$id" "merge group $branch is gone"
      continue
    fi
    job="$(ended_job "$id")" || continue
    if [[ -n "$job" ]]; then
      stop_run "$id" "$job did not pass"
    fi
  done <<<"$runs"
}

while sweep; do
  ((SECONDS < deadline)) || break
  sleep "$interval"
done
