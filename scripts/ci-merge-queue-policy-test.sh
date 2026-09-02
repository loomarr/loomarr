#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fixture() {
  jq -n --argjson concurrency "$1" '{
    id: 42,
    name: "Main merge queue admission",
    target: "branch",
    enforcement: "active",
    conditions: {ref_name: {exclude: [], include: ["refs/heads/main"]}},
    rules: [{
      type: "merge_queue",
      parameters: {
        merge_method: "SQUASH",
        max_entries_to_build: $concurrency,
        min_entries_to_merge: 1,
        max_entries_to_merge: 1,
        min_entries_to_merge_wait_minutes: 0,
        grouping_strategy: "ALLGREEN",
        check_response_timeout_minutes: 180
      }
    }],
    bypass_actors: []
  }'
}

fake_bin="$TMP/bin"
state="$TMP/ruleset.json"
mkdir -p "$fake_bin"
fixture 1 > "$state"

# shellcheck disable=SC2016 # POLICY_STATE expands when the generated fake executes.
printf '%s\n' \
  '#!/usr/bin/env bash' \
  'set -euo pipefail' \
  'if [[ " $* " == *" --method PUT "* ]]; then' \
  '  jq . > "$POLICY_STATE"' \
  'elif [[ "$*" == "api repos/test/repo/rulesets" ]]; then' \
  '  printf '\''[{"id":42,"name":"Main merge queue admission"}]\n'\''' \
  'else' \
  '  jq . "$POLICY_STATE"' \
  'fi' > "$fake_bin/gh"
chmod +x "$fake_bin/gh"

run_policy() {
  PATH="$fake_bin:$PATH" POLICY_STATE="$state" LOOMARR_REPOSITORY=test/repo \
    "$ROOT/scripts/ci-merge-queue-policy.sh"
}

if run_policy >/dev/null 2>&1; then
  echo 'ci-merge-queue-policy-test: serialized build policy was accepted' >&2
  exit 1
fi

fixture 2 > "$state"
run_policy | grep -q 'build concurrency 2'

fixture 1 > "$state"
APPLY=1 run_policy | grep -q 'build concurrency 2'
jq -e '
  .name == "Main merge queue admission"
  and .target == "branch"
  and .enforcement == "active"
  and .conditions.ref_name.include == ["refs/heads/main"]
  and .conditions.ref_name.exclude == []
  and .bypass_actors == []
  and (.rules | length) == 1
  and .rules[0].type == "merge_queue"
  and .rules[0].parameters.max_entries_to_build == 2
  and .rules[0].parameters.min_entries_to_merge == 1
  and .rules[0].parameters.max_entries_to_merge == 1
  and .rules[0].parameters.grouping_strategy == "ALLGREEN"
' "$state" >/dev/null

echo 'ci-merge-queue-policy-test: ok'
