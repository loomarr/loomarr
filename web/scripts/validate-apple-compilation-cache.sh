#!/usr/bin/env bash
# Exercise Xcode's compilation CAS across fresh hosted runners before CI is allowed to consume it.
set -euo pipefail

web_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly web_root
repo_root="$(cd "$web_root/.." && pwd)"
readonly repo_root
readonly cache_cli="${LOOMARR_APPLE_CACHE_CLI:-$repo_root/scripts/apple-compilation-cache.sh}"
readonly validation_root="${LOOMARR_APPLE_VALIDATION_ROOT:-${RUNNER_TEMP:-$repo_root/.artifacts}/apple-cache-validation}"

usage() {
  printf 'usage: %s produce ARCHIVE | consume ARCHIVE\n' "$0" >&2
  exit 2
}

prepare_root() {
  if [[ -e "$validation_root" ]] \
    && [[ -n "$(find "$validation_root" -mindepth 1 -print -quit)" ]]; then
    printf 'apple-cache-validation: root is not empty: %s\n' "$validation_root" >&2
    exit 1
  fi
  mkdir -p "$validation_root"
}

run_client() {
  local app="$1" label="$2" mode="$3" populate="$4" diagnostics="$5" probe="$6"
  (
    cd "$repo_root"
    LOOMARR_APPLE_ARTIFACTS_DIR="$validation_root/$label-artifacts" \
      LOOMARR_APPLE_BUILD_DIR="$validation_root/$label-build" \
      LOOMARR_APPLE_CACHE_MODE="$mode" \
      LOOMARR_APPLE_CACHE_STORE="$validation_root/store" \
      LOOMARR_APPLE_CACHE_POPULATE="$populate" \
      LOOMARR_APPLE_CACHE_REQUIRE_WARM="$([[ "$mode" == warm ]] && printf 1 || printf 0)" \
      LOOMARR_APPLE_CACHE_DIAGNOSTIC_REQUIREMENT="$diagnostics" \
      LOOMARR_APPLE_CACHE_SOURCE_PROBE="$probe" \
      make client-apple-simulator CLIENT_APP="$app"
  )
}

produce() {
  local archive="$1" fingerprint
  prepare_root
  if [[ -e "$archive" ]]; then
    printf 'apple-cache-validation: archive already exists: %s\n' "$archive" >&2
    exit 1
  fi
  fingerprint="$($cache_cli fingerprint)"
  printf '%s\n' "$fingerprint" > "$validation_root/fingerprint.txt"
  mkdir "$validation_root/store"

  run_client mobile mobile-populate warm 1 report ''
  run_client tv tv-populate warm 1 report ''
  "$cache_cli" validate-store "$validation_root/store"
  "$cache_cli" pack "$validation_root/store" "$archive"

  if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
    printf 'fingerprint=%s\narchive=%s\n' "$fingerprint" "$archive" >> "$GITHUB_OUTPUT"
  fi
  printf 'apple-cache-validation: producer passed mobile and TV and packed %s\n' "$archive"
}

consume() {
  local archive="$1" baseline changed corrupt_archive changed_lock
  prepare_root
  baseline="$($cache_cli fingerprint)"
  "$cache_cli" restore "$archive" "$validation_root/store"

  run_client mobile mobile-warm warm 0 hits ''
  run_client tv tv-warm warm 0 hits ''
  run_client mobile mobile-source-change warm 0 hits-and-misses source-change

  corrupt_archive="$validation_root/corrupt-cache.tar.zst"
  cp "$archive" "$corrupt_archive"
  truncate -s 32 "$corrupt_archive"
  if "$cache_cli" restore "$corrupt_archive" "$validation_root/corrupt-store" \
    >"$validation_root/corrupt-restore.log" 2>&1; then
    printf 'apple-cache-validation: corrupt archive was accepted\n' >&2
    exit 1
  fi
  if [[ -e "$validation_root/corrupt-store" ]]; then
    printf 'apple-cache-validation: corrupt archive exposed a store\n' >&2
    exit 1
  fi

  changed_lock="$validation_root/changed-pnpm-lock.yaml"
  cp "$repo_root/web/pnpm-lock.yaml" "$changed_lock"
  printf '\n# Apple cache dependency invalidation probe\n' >> "$changed_lock"
  changed="$(LOOMARR_APPLE_LOCKFILE="$changed_lock" "$cache_cli" fingerprint)"
  if [[ "$changed" == "$baseline" ]]; then
    printf 'apple-cache-validation: lockfile change did not invalidate the outer key\n' >&2
    exit 1
  fi

  run_client mobile mobile-corrupt-fallback cold 0 report ''
  run_client tv tv-corrupt-fallback cold 0 report ''
  "$cache_cli" validate-store "$validation_root/store"
  printf 'apple-cache-validation: consumer passed restore, hits, source invalidation, corruption, and cold fallback\n'
}

case "${1:-}" in
  produce)
    [[ $# -eq 2 ]] || usage
    produce "$2"
    ;;
  consume)
    [[ $# -eq 2 ]] || usage
    consume "$2"
    ;;
  *) usage ;;
esac
