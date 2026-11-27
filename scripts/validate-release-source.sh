#!/usr/bin/env bash

set -euo pipefail

: "${GITHUB_REF_NAME:?GITHUB_REF_NAME is required}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
: "${GITHUB_SHA:?GITHUB_SHA is required}"

./scripts/check-release-tag.sh "$GITHUB_REF_NAME"
head=$(git rev-parse HEAD)
if [[ "$head" != "$GITHUB_SHA" ]]; then
  echo "checked-out release source does not match the triggering tag commit" >&2
  exit 1
fi
git fetch --no-tags origin main
if [[ "$head" != "$(git rev-parse origin/main)" ]]; then
  echo "release tag must point at the current main commit" >&2
  exit 1
fi

run=$(gh api \
  "/repos/${GITHUB_REPOSITORY}/actions/workflows/ci.yml/runs?head_sha=${GITHUB_SHA}&per_page=20" \
  --jq '[.workflow_runs[] | select(.head_branch == "main" and .event == "workflow_dispatch")] | sort_by(.created_at) | last | [.id, .conclusion, .event] | @tsv')
# cut, not `read`: a tab is IFS whitespace, so a running run's empty conclusion would collapse and
# shift its event into the conclusion's place.
run_id=$(cut -f1 <<< "$run")
conclusion=$(cut -f2 <<< "$run")
event=$(cut -f3 <<< "$run")
# The newest dispatch run is the evidence, whatever its scope: a full-scope run dispatched after
# the release-candidate run has to finish green and carry the candidate's jobs too, or be followed
# by another release-candidate run.
if [[ -n "$run_id" && -z "$conclusion" ]]; then
  echo "the newest manual CI run on the tagged commit ($run_id) has not finished; wait for it, or dispatch a release-candidate run after it" >&2
  exit 1
fi
if [[ "$conclusion" != success ]]; then
  echo "tagged main commit does not have a successful release-candidate CI run (found: ${conclusion:-none})" >&2
  exit 1
fi
if [[ "$event" != workflow_dispatch ]]; then
  echo "tagged main commit has no release-candidate CI evidence (found: ${event:-none})" >&2
  exit 1
fi

jobs=$(gh api --paginate \
  "/repos/${GITHUB_REPOSITORY}/actions/runs/${run_id}/jobs?per_page=100" \
  --jq '.jobs[] | [.name, .conclusion] | @tsv')

require_job() {
  local name=$1
  local expected=${2:-success}
  if ! grep -Fqx "$name"$'\t'"$expected" <<< "$jobs"; then
    echo "successful CI run did not prove ${name} (${expected})" >&2
    exit 1
  fi
}

require_job "CI"
for platform in linux/amd64 linux/arm64; do
  require_job "Image — release build / Image — release build (${platform})"
done
require_job "Release candidate — exact main scope"
for shard in 1/2 2/2; do
  require_job "Go — repository contracts / Go — repository contracts (${shard})"
done
require_job "Rust — repository contracts / Rust — repository contracts"
require_job "Rust image — runtime certification / Rust image — runtime certification"

# A final tag also needs its migrations rehearsed on a real backup. That runs at home, where the
# backup lives, and records a commit status: make migration-rehearsal ... RECORD=1.
case "$GITHUB_REF_NAME" in
  *-rc.*) ;;
  *)
    rehearsal=$(gh api "/repos/${GITHUB_REPOSITORY}/commits/${GITHUB_SHA}/statuses?per_page=100" \
      --jq '[.[] | select(.context == "loomarr/migration-rehearsal")] | first | .state // ""')
    if [[ "$rehearsal" != success ]]; then
      echo "a final tag needs a passed migration rehearsal on this commit (found: ${rehearsal:-none})" >&2
      echo "run: make migration-rehearsal BACKUP=<backup directory> IMAGE=ghcr.io/loomarr/loomarr-staging:${GITHUB_SHA} RECORD=1" >&2
      exit 1
    fi

    # Intel and NVIDIA block a final release too (software is in the CI run above).
    ./scripts/check-hardware-gate.sh "$GITHUB_SHA"
    ;;
esac
