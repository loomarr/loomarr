#!/usr/bin/env bash
# Keep one entry of each shared Actions cache family on main: the newest.
#
# A shared cache is written on main and read by pull requests and the merge queue. A family gets
# a new key whenever its inputs change (go.sum, the ISO week, the hour, the pnpm lockfile, the
# ffmpeg pin, a CodeQL analysis), and nothing removes the entry it supersedes: `actions/cache`
# never deletes, and GitHub evicts by least recent use, which takes a live cache that is read
# rarely before a superseded one that was read an hour ago. A consumer that still asks for a
# superseded key reaches the newest entry through its restore prefix, or rebuilds.
#
# The newest entry is chosen from the listing, never from a key a caller believes it saved: a
# cache save that fails only warns, and keeping "the key just saved" would then delete the last
# good entry.
#
#   plan CACHES_JSON   print the cache ids to delete, one per line, from a saved listing
#   apply              list main's caches (REPO, GH_TOKEN), then delete what plan prints
set -euo pipefail

plan() {
  local caches_json="$1"
  if [[ ! -f "${caches_json}" ]]; then
    printf 'ci-cache-retention: no cache listing at %s\n' "${caches_json}" >&2
    exit 2
  fi
  jq -r '
    # The family an entry belongs to, or null for a key this script does not manage.
    def family:
      if test("^merge-gate-go-") then
        (capture("^merge-gate-go-(?<os>[^-]+)-") | "merge-gate-go " + .os) // null
      elif test("^go-test-.*-warm$") then
        (capture("^go-test-(?<os>[^-]+)-[0-9]{4}W[0-9]{2}-(?<lane>.+)-[0-9a-f]{64}-warm$")
          | "go-test " + .os + " " + .lane) // null
      elif test("^gobuild-pg-.*-warm$") then
        (capture("^gobuild-pg-(?<os>[^-]+)-") | "gobuild-pg " + .os) // null
      elif test("^go-contracts-.*-warm$") then
        (capture("^go-contracts-(?<os>[^-]+)-") | "go-contracts " + .os) // null
      elif test("^pnpm-") then
        (capture("^pnpm-(?<os>[^-]+)-") | "pnpm " + .os) // null
      elif test("^ffmpeg-") then
        (capture("^ffmpeg-(?<os>[^-]+)-(?<arch>[^-]+)-") | "ffmpeg " + .os + " " + .arch) // null
      elif test("^codeql-overlay-base-database-") then
        (capture("-(?<language>[a-z]+)-[0-9]+\\.[0-9]+\\.[0-9]+-") | "codeql " + .language) // null
      else null end;

    .actions_caches as $caches
    | if ($caches | type) != "array" then error("actions_caches must be an array") else $caches end
    | [
        .[]
        | select(.ref == "refs/heads/main" and (.key | type) == "string")
        | . + {family: (.key | family)}
        | select(.family != null)
      ] as $managed
    | if any($managed[];
          (.id | type) != "number" or .id < 1 or (.id | floor) != .id
          or (.created_at | type) != "string" or .created_at == "")
      then error("managed cache metadata is malformed")
      else $managed
      end
    | group_by(.family)[]
    | sort_by(.created_at, .id)
    | .[:-1][]
    | .id
  ' "${caches_json}"
}

apply() {
  local repository="${REPO:?}" id
  # Not local: the EXIT trap runs after this function has returned.
  listing=$(mktemp)
  trap 'rm -f -- "${listing}"' EXIT
  gh api "repos/${repository}/actions/caches?ref=refs/heads/main&per_page=100" --paginate |
    jq -s '{actions_caches: (map(.actions_caches) | add)}' >"${listing}"
  plan "${listing}" | while read -r id; do
    gh api -X DELETE "repos/${repository}/actions/caches/${id}" >/dev/null
    printf 'ci-cache-retention: deleted superseded cache %s\n' "${id}"
  done
}

case "${1:-}" in
  plan)
    [[ $# == 2 ]] || { echo 'usage: ci-cache-retention.sh plan CACHES_JSON' >&2; exit 2; }
    plan "$2"
    ;;
  apply)
    [[ $# == 1 ]] || { echo 'usage: ci-cache-retention.sh apply' >&2; exit 2; }
    apply
    ;;
  *)
    echo 'usage: ci-cache-retention.sh {plan CACHES_JSON|apply}' >&2
    exit 2
    ;;
esac
