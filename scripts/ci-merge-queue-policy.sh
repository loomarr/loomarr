#!/usr/bin/env bash
# Check, or explicitly apply, Loomarr's live merge-queue admission policy.
set -euo pipefail

repository="${LOOMARR_REPOSITORY:-loomarr/loomarr}"
ruleset_name='Main merge queue admission'

rulesets="$(gh api "repos/$repository/rulesets")"
matches="$(jq -r --arg name "$ruleset_name" '.[] | select(.name == $name) | .id' <<<"$rulesets")"
match_count="$(grep -c . <<<"$matches" || true)"
if [[ "$match_count" -ne 1 ]]; then
  printf 'ci-merge-queue-policy: found %s rulesets named %q in %s; want exactly one\n' \
    "$match_count" "$ruleset_name" "$repository" >&2
  exit 1
fi

ruleset_id="$matches"
endpoint="repos/$repository/rulesets/$ruleset_id"
ruleset="$(gh api "$endpoint")"

if [[ "${APPLY:-0}" == 1 ]]; then
  payload="$(jq '
    {
      name,
      target,
      enforcement,
      bypass_actors,
      conditions,
      rules: [.rules[] | if .type == "merge_queue"
        then .parameters.max_entries_to_build = 2
        else .
        end]
    }
  ' <<<"$ruleset")"
  gh api --method PUT "$endpoint" --input - <<<"$payload" >/dev/null
  ruleset="$(gh api "$endpoint")"
elif [[ "${APPLY:-0}" != 0 ]]; then
  printf 'ci-merge-queue-policy: APPLY must be 1 or unset\n' >&2
  exit 2
fi

if ! jq -e '
  .name == "Main merge queue admission"
  and .target == "branch"
  and .enforcement == "active"
  and .conditions.ref_name.include == ["refs/heads/main"]
  and .conditions.ref_name.exclude == []
  and .bypass_actors == []
  and (.rules | length) == 1
  and .rules[0].type == "merge_queue"
  and .rules[0].parameters.merge_method == "SQUASH"
  and .rules[0].parameters.max_entries_to_build == 2
  and .rules[0].parameters.min_entries_to_merge == 1
  and .rules[0].parameters.max_entries_to_merge == 1
  and .rules[0].parameters.min_entries_to_merge_wait_minutes == 0
  and .rules[0].parameters.grouping_strategy == "ALLGREEN"
  and .rules[0].parameters.check_response_timeout_minutes == 180
' <<<"$ruleset" >/dev/null; then
  actual="$(jq -r '.rules[] | select(.type == "merge_queue") | .parameters.max_entries_to_build' <<<"$ruleset")"
  printf 'ci-merge-queue-policy: live ruleset differs from policy (build concurrency %s; want 2)\n' \
    "${actual:-missing}" >&2
  exit 1
fi

echo 'ci-merge-queue-policy: ok (build concurrency 2, single-PR squash merge, ALLGREEN)'
