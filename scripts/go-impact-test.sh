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

tags_csv="$(cd "$ROOT" && make -s print-tags-csv)"
all_count="$(cd "$ROOT" && go list -tags "$tags_csv" ./... | wc -l)"
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

# GH #1279: a package compiled ONLY under a custom build tag (every file carries a
# `//go:build` line, e.g. internal/eval under `eval`) is invisible to plain `go list ./...`.
# `make lint` is given `--build-tags` and would gladly check it, so the "select everything"
# fallback this change triggers must select it too — a narrower-than-everything fallback is
# exactly the blind spot that let a tag-only lint failure pass locally and fail in CI.
tag_only_selection="$($SELECTOR internal/eval/attribution.go)"
tag_only_count="$(wc -l <<<"$tag_only_selection")"
[[ "$tag_only_count" -eq "$all_count" ]] || {
  printf 'go-impact-test: tag-only package change selected %d/%d packages (internal/eval must be in the fallback)\n' "$tag_only_count" "$all_count" >&2
  exit 1
}
grep -qx "$module/internal/eval" <<<"$tag_only_selection" || {
  echo 'go-impact-test: fallback selection omitted the tag-only internal/eval package' >&2
  exit 1
}

[[ -z "$($SELECTOR docs/contributing/testing.md)" ]] || {
  echo 'go-impact-test: docs-only change selected Go packages' >&2
  exit 1
}

stdin="$({ printf '%s\n' internal/suggest/ground.go docs/contributing/testing.md; } | "$SELECTOR")"
[[ "$stdin" == "$leaf" ]] || {
  echo 'go-impact-test: stdin and argument interfaces disagree' >&2
  exit 1
}

echo 'go-impact-test: ok'
