#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CLASSIFIER="$ROOT/scripts/ci-impact.sh"

assert_gate() {
  local name="$1" path="$2" gate="$3" want="$4"
  local got
  got="$($CLASSIFIER "$path" | sed -n "s/^${gate}=//p")"
  if [[ "$got" != "$want" ]]; then
    printf 'ci-impact-test: %s: %s for %s: got %s, want %s\n' "$name" "$gate" "$path" "$got" "$want" >&2
    exit 1
  fi
}

assert_gate leaf_go internal/suggest/ground.go go true
assert_gate leaf_go internal/suggest/ground.go go_full false
assert_gate leaf_go internal/suggest/ground.go postgres false
assert_gate store_go internal/store/store.go postgres true
assert_gate app_go internal/app/build.go go_full true
assert_gate playout_go internal/playout/session.go windows true
assert_gate rust rust/loomarr-image/src/lib.rs rust true
assert_gate rust rust/loomarr-image/src/lib.rs go false
assert_gate generic_web web/apps/web/src/people/users-page/users-page.tsx web true
assert_gate generic_web web/apps/web/src/people/users-page/users-page.tsx tuner false
assert_gate page_visual web/apps/web/src/people/users-page/users-page.tsx visual true
assert_gate visual web/apps/web/src/components/loomarr/button.stories.tsx visual true
assert_gate e2e web/apps/web/src/wizard/steps/steps.tsx e2e true
assert_gate tuner web/apps/web/src/channels/use-hls-player/use-hls-player.ts tuner true
assert_gate openapi api/openapi.yaml android true
assert_gate openapi api/openapi.yaml web true
assert_gate docs docs/dev/testing.md docs true
assert_gate docs docs/dev/testing.md go false
assert_gate embedded_help docs/help/quickstart.md go true
assert_gate android android/app/src/main/AndroidManifest.xml android true
assert_gate release_workflow .github/workflows/release.yml contracts true
assert_gate release_workflow .github/workflows/release.yml web false
assert_gate ci_workflow .github/workflows/ci.yml tuner true
assert_gate unknown unexpected/new-runtime/file.xyz go true
assert_gate unknown unexpected/new-runtime/file.xyz android true

stdin_output="$(printf '%s\n' internal/suggest/ground.go docs/dev/testing.md | "$CLASSIFIER")"
grep -qx 'go=true' <<<"$stdin_output"
grep -qx 'docs=true' <<<"$stdin_output"
grep -qx 'go_full=false' <<<"$stdin_output"

git -C "$ROOT" ls-files --cached --others --exclude-standard | "$CLASSIFIER" --check-known >/dev/null

echo 'ci-impact-test: ok'
