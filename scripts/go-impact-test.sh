#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SELECTOR="$ROOT/scripts/go-impact.sh"

leaf="$($SELECTOR internal/suggest/ground.go)"
module="$(cd "$ROOT" && go list -m)"
grep -qx "$module/internal/suggest" <<<"$leaf"
grep -qx "$module/internal/app" <<<"$leaf"
grep -qx "$module/cmd/loomarr" <<<"$leaf"
if grep -qx "$module/internal/store" <<<"$leaf"; then
  echo 'go-impact-test: a suggest leaf selected unrelated internal/store' >&2
  exit 1
fi

all_count="$(cd "$ROOT" && go list ./... | wc -l)"
app_count="$($SELECTOR internal/app/build.go | wc -l)"
[[ "$app_count" -eq "$all_count" ]] || {
  printf 'go-impact-test: cross-cutting app change selected %d/%d packages\n' "$app_count" "$all_count" >&2
  exit 1
}

unknown_count="$($SELECTOR unexpected/new-runtime/file.xyz 2>/dev/null | wc -l)"
[[ "$unknown_count" -eq "$all_count" ]] || {
  printf 'go-impact-test: unknown path selected %d/%d packages\n' "$unknown_count" "$all_count" >&2
  exit 1
}

[[ -z "$($SELECTOR docs/dev/testing.md)" ]] || {
  echo 'go-impact-test: docs-only change selected Go packages' >&2
  exit 1
}

stdin="$({ printf '%s\n' internal/suggest/ground.go docs/dev/testing.md; } | "$SELECTOR")"
[[ "$stdin" == "$leaf" ]] || {
  echo 'go-impact-test: stdin and argument interfaces disagree' >&2
  exit 1
}

echo 'go-impact-test: ok'
