#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CLASSIFIER="$ROOT/scripts/ci-impact.sh"
FIXTURES="$ROOT/scripts/testdata/ci-impact.tsv"

selected_gates() {
  "$CLASSIFIER" "$@" | awk -F= '
    $2 == "true" {
      if (selected != "") selected = selected ","
      selected = selected $1
    }
    END { print selected }
  '
}

while IFS=$'\t' read -r name path_set want; do
  [[ -z "$name" || "$name" == \#* ]] && continue
  IFS=';' read -r -a paths <<<"$path_set"
  got="$(selected_gates "${paths[@]}")"
  if [[ "$got" != "$want" ]]; then
    printf 'ci-impact-test: %s for %s: got %s, want %s\n' "$name" "$path_set" "$got" "$want" >&2
    exit 1
  fi
done <"$FIXTURES"

all_gates='contracts,go,go_full,rust,postgres,windows,web,clients,apple_mobile,apple_tv,expo_android_mobile,expo_android_tv,visual,e2e,tuner,image,docs,agent,android'
unknown="$(selected_gates unexpected/new-runtime/file.xyz)"
if [[ "$unknown" != "$all_gates" ]]; then
  printf 'ci-impact-test: unknown path: got %s, want %s\n' "$unknown" "$all_gates" >&2
  exit 1
fi

stdin_output="$(printf '%s\n' internal/suggest/ground.go docs/dev/testing.md | "$CLASSIFIER")"
stdin_selected="$(awk -F= '$2 == "true" { if (selected != "") selected = selected ","; selected = selected $1 } END { print selected }' <<<"$stdin_output")"
if [[ "$stdin_selected" != 'contracts,go,postgres,image,docs' ]]; then
  printf 'ci-impact-test: stdin paths: got %s, want contracts,go,postgres,image,docs\n' "$stdin_selected" >&2
  exit 1
fi

all_output="$($CLASSIFIER --all)"
while IFS= read -r decision; do
  [[ "$decision" == *=true ]] || {
    printf 'ci-impact-test: --all returned non-true decision: %s\n' "$decision" >&2
    exit 1
  }
done <<<"$all_output"
if "$CLASSIFIER" --all internal/suggest/ground.go >/dev/null 2>&1; then
  echo 'ci-impact-test: --all unexpectedly accepted a path' >&2
  exit 1
fi

git -C "$ROOT" ls-files --cached --others --exclude-standard | "$CLASSIFIER" --check-known >/dev/null

echo 'ci-impact-test: ok'
