#!/usr/bin/env bash
# Render one CI run's queue, execution, critical-path, and occupied-runner timing as Markdown.
# With no argument it reads the current GitHub Actions run. Tests pass a captured JSON fixture.
set -euo pipefail

if (($# > 1)); then
  printf 'usage: %s [run-fixture.json]\n' "$0" >&2
  exit 2
fi

if (($# == 1)); then
  payload="$(jq -c '{run: .run, jobs: .jobs}' "$1")"
else
  : "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
  : "${GITHUB_RUN_ID:?GITHUB_RUN_ID is required}"
  if [[ -z "${GH_TOKEN:-}" ]]; then
    gh auth status --hostname github.com >/dev/null
  fi
  run="$(gh api "repos/${GITHUB_REPOSITORY}/actions/runs/${GITHUB_RUN_ID}")"
  pages="$(gh api --paginate --slurp "repos/${GITHUB_REPOSITORY}/actions/runs/${GITHUB_RUN_ID}/jobs?per_page=100")"
  payload="$(jq -cn --argjson run "$run" --argjson pages "$pages" \
    '{run: $run, jobs: [$pages[].jobs[]]}')"
fi

jq -r '
  def seconds_between($start; $finish):
    (($finish | fromdateiso8601) - ($start | fromdateiso8601));
  def minutes:
    (((. / 6) | round) / 10 | tostring) + "m";
  def escaped:
    gsub("[|]"; "\\|");

  .run.created_at as $run_created
  | [
      .jobs[]
      | select(.conclusion != "skipped")
      | select(.created_at != null and .started_at != null and .completed_at != null)
      | {
          name,
          conclusion,
          queue_seconds: seconds_between(.created_at; .started_at),
          run_seconds: seconds_between(.started_at; .completed_at),
          completed_at
        }
    ] as $jobs
  | ($jobs | map(.run_seconds) | add // 0) as $runner_seconds
  | ($jobs | max_by(.run_seconds)) as $critical
  | ($jobs | max_by(.completed_at)) as $last
  | (if $last == null then 0 else seconds_between($run_created; $last.completed_at) end) as $wall_seconds
  | "## CI timing and runner use",
    "",
    ("**End-to-end:** " + ($wall_seconds | minutes)
      + " · **Occupied runner time:** " + ($runner_seconds | minutes)
      + (if $critical == null then ""
         else " · **Longest job:** " + ($critical.name | escaped)
           + " (" + ($critical.run_seconds | minutes) + ")"
         end)),
    "",
    "Queue is time waiting for a runner; execution is occupied runner time. Skipped jobs are omitted.",
    "",
    "| Job | Result | Queue | Execution |",
    "| --- | --- | ---: | ---: |",
    ($jobs
      | sort_by(.run_seconds)
      | reverse[]
      | "| " + (.name | escaped) + " | " + .conclusion + " | "
        + (.queue_seconds | minutes) + " | " + (.run_seconds | minutes) + " |")
' <<<"$payload"
