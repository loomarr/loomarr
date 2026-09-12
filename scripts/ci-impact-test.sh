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
  for path in "${paths[@]}"; do
    if [[ ! -e "$ROOT/$path" ]]; then
      printf 'ci-impact-test: fixture %s references missing path %s\n' "$name" "$path" >&2
      exit 1
    fi
  done
  got="$(selected_gates "${paths[@]}")"
  if [[ "$got" != "$want" ]]; then
    printf 'ci-impact-test: %s for %s: got %s, want %s\n' "$name" "$path_set" "$got" "$want" >&2
    exit 1
  fi
done <"$FIXTURES"

# Keep the non-authoritative legacy Android summary filter aligned with the specialized classifier.
# Product jobs consume only impact_android, but a stale shadow decision misdiagnoses selection and
# previously preserved the same unnecessary OpenAPI-to-native coupling in two places.
legacy_android_filter="$(
  sed -n '/# The React Native Android TV client/,/echo "android=false"/p' \
    "$ROOT/.github/workflows/ci.yml"
)"
if grep -q 'api/openapi' <<<"$legacy_android_filter"; then
  echo 'ci-impact-test: legacy Android summary still selects schema-only OpenAPI changes' >&2
  exit 1
fi
if ! grep -q 'web/(apps/tv/' <<<"$legacy_android_filter"; then
  echo 'ci-impact-test: legacy Android summary lost the real TV app input' >&2
  exit 1
fi

clients_workflow="$(<"$ROOT/.github/workflows/ci-clients.yml")"
if ! grep -q 'run: make fe-codegen' <<<"$clients_workflow" ||
  ! grep -q 'run: make clients' <<<"$clients_workflow"; then
  echo 'ci-impact-test: shared-client gate no longer regenerates and verifies OpenAPI consumers' >&2
  exit 1
fi

all_gates='contracts,go,go_full,rust,postgres,web,clients,apple_mobile,apple_tv,expo_android_mobile,expo_android_tv,visual,e2e,tuner,image,docs,agent,android,policy'
unknown="$(selected_gates unexpected/new-runtime/file.xyz)"
if [[ "$unknown" != "$all_gates" ]]; then
  printf 'ci-impact-test: unknown path: got %s, want %s\n' "$unknown" "$all_gates" >&2
  exit 1
fi

stdin_output="$(printf '%s\n' internal/suggest/score.go docs/dev/testing.md | "$CLASSIFIER")"
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
