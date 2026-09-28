#!/usr/bin/env bash
# Watch the current gh user's open PRs against main and print one line per problem that needs a person:
# conflicts, failed checks, BEHIND with auto-merge on, dropped from the merge queue, or auto-merge off
# while not queued. Read-only: it never updates, re-arms or merges anything.
#
# Usage: scripts/dev/watch-prs.sh [--once]
#   WATCH_PRS_INTERVAL=180      seconds between polls
#   WATCH_PRS_AUTO_GRACE=1200   seconds a PR may sit with auto-merge off before it is reported
#                               (lanes arm auto-merge once CI is green)
#
# Each alert fires once per (PR, condition, head SHA), remembered across restarts, so a problem that
# comes back on a new push alerts again. Drafts and stacked PRs (based on another branch) belong to
# their owner and are skipped.

set -u

WATCH_SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)"
# shellcheck source=scripts/dev/watch-lib.sh
. "$WATCH_SCRIPT_DIR/watch-lib.sh"

interval="${WATCH_PRS_INTERVAL:-180}"
grace="${WATCH_PRS_AUTO_GRACE:-1200}"

# GitHub clears autoMergeRequest once a PR enters the merge queue, so "auto-merge off" means nothing
# without mergeQueueEntry; and a PR the queue dequeued shows only as a RemovedFromMergeQueueEvent.
# shellcheck disable=SC2016 # GraphQL variables, not shell expansions.
query='query($q: String!) {
  search(query: $q, type: ISSUE, first: 50) {
    nodes {
      ... on PullRequest {
        number title isDraft baseRefName headRefOid mergeStateStatus updatedAt
        autoMergeRequest { enabledAt }
        mergeQueueEntry { state }
        timelineItems(itemTypes: [ADDED_TO_MERGE_QUEUE_EVENT, REMOVED_FROM_MERGE_QUEUE_EVENT], last: 1) {
          nodes { __typename ... on RemovedFromMergeQueueEvent { reason } }
        }
        commits(last: 1) { nodes { commit { statusCheckRollup { contexts(first: 100) { nodes {
          __typename
          ... on CheckRun { name conclusion }
          ... on StatusContext { context state }
        } } } } } }
      }
    }
  }
}'

# pr_rows: one row per open, non-draft PR against main, fields split by \037 (tab is IFS whitespace,
# so `read` would collapse an empty field and shift the rest):
# number head merge-state auto queued dropped-reason failed-checks age-seconds title
pr_rows() {
	local repo
	repo="$(cd "$WATCH_SCRIPT_DIR" && gh repo view --json nameWithOwner --jq .nameWithOwner 2>/dev/null)" || return 1
	[ -n "$repo" ] || return 1
	gh api graphql -f q="repo:$repo is:pr is:open author:@me base:main" -f query="$query" 2>/dev/null |
		jq -r '.data.search.nodes[]
			| select(.number != null and (.isDraft | not) and .baseRefName == "main")
			| (.timelineItems.nodes[-1] // {}) as $mq
			| [ .number, .headRefOid, .mergeStateStatus,
				(.autoMergeRequest != null), (.mergeQueueEntry != null),
				(if $mq.__typename == "RemovedFromMergeQueueEvent" then ($mq.reason // "no reason given") else "" end),
				([ .commits.nodes[0].commit.statusCheckRollup.contexts.nodes[]?
					| select((.__typename == "CheckRun" and (.conclusion == "FAILURE" or .conclusion == "TIMED_OUT" or .conclusion == "STARTUP_FAILURE"))
						or (.__typename == "StatusContext" and (.state == "FAILURE" or .state == "ERROR")))
					| (.name // .context) ] | unique | join(", ")),
				((now - (.updatedAt | fromdateiso8601)) | floor),
				.title[0:60] ] | map(tostring) | join("\u001f")'
}

report() { # number kind head message
	watch_first "$1.$2.$3" && watch_say "#$1 $4"
}

poll() {
	local rows n head state auto queued dropped failed age title
	rows="$(pr_rows)" || return 0
	while IFS="$(printf '\037')" read -r n head state auto queued dropped failed age title; do
		[ -n "$n" ] || continue
		[ "$state" = DIRTY ] && report "$n" conflicts "$head" "CONFLICTS with main ($title)"
		[ -n "$failed" ] && report "$n" checks "$head" "CHECKS FAILED: $failed ($title)"
		[ "$state" = BEHIND ] && [ "$auto" = true ] && [ "$queued" = false ] &&
			report "$n" behind "$head" "is BEHIND main with auto-merge on and will not queue itself: gh pr update-branch $n ($title)"
		if [ "$queued" = false ] && [ "$auto" = false ]; then
			if [ -n "$dropped" ]; then
				report "$n" dropped "$head" "was DROPPED from the merge queue ($dropped); fix it and re-arm auto-merge ($title)"
			elif [ "$age" -gt "$grace" ] && [ "$state" != DIRTY ] && [ -z "$failed" ]; then
				report "$n" autooff "$head" "has auto-merge OFF and is not queued ($title)"
			fi
		fi
	done <<EOF
$rows
EOF
}

command -v gh >/dev/null 2>&1 || {
	echo 'watch-prs: gh CLI not found; skipping' >&2
	exit 0
}
watch_state_init prs persistent
if [ "${1:-}" = --once ]; then
	poll
	exit 0
fi
while :; do
	poll
	sleep "$interval"
done
