#!/usr/bin/env bash
# Go dead-code report: functions no binary under ./cmd can reach, counted per package.
#
#   scripts/deadcode.sh report             print "<count> <package>" lines, then "total <n>"
#   scripts/deadcode.sh track BASELINE     report, then open or update the one tracking issue
#                                          when the total exceeds BASELINE's (needs gh, GH_TOKEN)
#
# This is a monthly report, not a PR gate: deadcode cannot see code reached only from
# build-tagged tests (ffmpeg, eval, integration), so some of what it lists is deliberate.
# Baseline: docs/engineering/evidence/deadcode-baseline.txt, written by `make deadcode-baseline`.
set -euo pipefail

# Run with `go run`, like air and golangci-lint: a tool, never a go.mod dependency.
readonly DEADCODE="golang.org/x/tools/cmd/deadcode@v0.50.0"
readonly ISSUE_TITLE="Go dead code grew past its baseline"

report() {
  local raw
  raw="$(go run "$DEADCODE" ./cmd/...)"
  # Each finding reads "internal/pkg/file.go:12:6: unreachable func: Name".
  local counts
  counts="$(printf '%s\n' "$raw" | awk -F: 'NF > 1 {
      path = $1; sub(/\/[^\/]*$/, "", path); count[path]++
    }
    END { for (p in count) printf "%d %s\n", count[p], p }' | sort -k2)"
  printf '%s\n' "$counts" | sed '/^$/d'
  printf '%s\n' "$counts" | awk '{ n += $1 } END { printf "total %d\n", n + 0 }'
}

total_of() {
  awk '$1 == "total" { print $2 }' "$1"
}

track() {
  local base="$1" current before after body number
  current="$(mktemp)"
  trap 'rm -f "$current"' EXIT
  report >"$current"
  cat "$current"
  before="$(total_of "$base")"
  after="$(total_of "$current")"
  if [[ -z "$before" || -z "$after" ]]; then
    printf 'deadcode: missing "total" line in %s or the new report\n' "$base" >&2
    exit 2
  fi
  printf 'deadcode: baseline %s, now %s\n' "$before" "$after"
  if ((after <= before)); then
    return 0
  fi

  # shellcheck disable=SC2016 # the backticks are Markdown code spans, not expansions
  body="$(
    printf 'Unreachable Go functions grew from **%s** to **%s** (`make deadcode`, run %s).\n\n' \
      "$before" "$after" "${GITHUB_RUN_ID:-local}"
    printf 'Per-package change against `%s`:\n\n```diff\n' "$base"
    diff "$base" "$current" || true
    printf '```\n\nDelete what is dead, then `make deadcode-baseline` and commit the new baseline.\n'
    printf 'Code reached only from build-tagged tests is a known false positive.\n'
  )"

  number="$(gh issue list --state open --search "in:title \"$ISSUE_TITLE\"" \
    --json number,title --jq ".[] | select(.title == \"$ISSUE_TITLE\") | .number" | head -n 1)"
  if [[ -n "$number" ]]; then
    gh issue comment "$number" --body "$body"
  else
    gh issue create --title "$ISSUE_TITLE" --body "$body"
  fi
}

case "${1:-report}" in
  report) report ;;
  track)
    [[ $# -eq 2 ]] || {
      printf 'usage: %s track BASELINE\n' "$0" >&2
      exit 2
    }
    track "$2"
    ;;
  *)
    printf 'usage: %s [report | track BASELINE]\n' "$0" >&2
    exit 2
    ;;
esac
