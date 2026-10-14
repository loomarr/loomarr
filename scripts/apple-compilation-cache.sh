#!/usr/bin/env bash
# Prepare and validate Loomarr's portable Xcode compilation cache.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly root
readonly schema=loomarr-apple-compilation-cache-v1
readonly key_prefix=apple-compilation-cache-v1

sha256_stream() {
  shasum -a 256 | awk '{print $1}'
}

fingerprint() {
  local runner_os runner_arch lockfile xcconfig_override
  local xcode_hash swift_hash ios_sdk_hash tv_sdk_hash xcconfig_hash lockfile_hash digest
  runner_os="${RUNNER_OS:-$(uname -s)}"
  runner_arch="${RUNNER_ARCH:-$(uname -m)}"
  lockfile="${LOOMARR_APPLE_LOCKFILE:-$root/web/pnpm-lock.yaml}"
  if [[ ! -f "$lockfile" ]]; then
    printf 'apple-compilation-cache: lockfile is missing: %s\n' "$lockfile" >&2
    exit 1
  fi

  xcode_hash="$(xcodebuild -version 2>&1 | sha256_stream)"
  swift_hash="$(xcrun swiftc --version 2>&1 | sha256_stream)"
  ios_sdk_hash="$(xcodebuild -version -sdk iphonesimulator 2>&1 | sha256_stream)"
  tv_sdk_hash="$(xcodebuild -version -sdk appletvsimulator 2>&1 | sha256_stream)"
  xcconfig_override="${LOOMARR_APPLE_CACHE_XCCONFIG:-}"
  if [[ -n "$xcconfig_override" ]]; then
    if [[ ! -f "$xcconfig_override" ]]; then
      printf 'apple-compilation-cache: xcconfig is missing: %s\n' "$xcconfig_override" >&2
      exit 1
    fi
    xcconfig_hash="$(sha256_stream < "$xcconfig_override")"
  else
    xcconfig_hash="$(
      {
        printf 'apple-simulator.xcconfig\0'
        cat "$root/web/scripts/apple-simulator.xcconfig"
        printf '\0apple-compilation-cache.xcconfig\0'
        cat "$root/web/scripts/apple-compilation-cache.xcconfig"
      } | sha256_stream
    )"
  fi
  lockfile_hash="$(sha256_stream < "$lockfile")"

  digest="$(
    printf '%s\n' \
      "schema=$schema" \
      "runner_os=$runner_os" \
      "runner_arch=$runner_arch" \
      "xcode=$xcode_hash" \
      "swift=$swift_hash" \
      "iphonesimulator=$ios_sdk_hash" \
      "appletvsimulator=$tv_sdk_hash" \
      'configuration=Release' \
      'platform=simulator' \
      "xcconfig=$xcconfig_hash" \
      "lockfile=$lockfile_hash" \
      | sha256_stream
  )"
  printf '%s-%s\n' "$key_prefix" "$digest"
}

validate_store() {
  local store="$1" llvm_cas
  if [[ ! -d "$store" ]] || [[ -z "$(find "$store" -mindepth 1 -print -quit)" ]]; then
    printf 'apple-compilation-cache: store is missing or empty: %s\n' "$store" >&2
    exit 1
  fi
  llvm_cas="$(xcrun --find llvm-cas)"
  if [[ ! -x "$llvm_cas" ]]; then
    printf 'apple-compilation-cache: llvm-cas is unavailable: %s\n' "$llvm_cas" >&2
    exit 1
  fi
  "$llvm_cas" --cas "$store" --validate --check-hash --force >/dev/null
  printf 'apple-compilation-cache: validated %s\n' "$store"
}

pack_store() {
  local store="$1" archive="$2" archive_parent temp_dir temp_archive archive_bytes
  local max_archive_bytes="${LOOMARR_APPLE_CACHE_MAX_ARCHIVE_BYTES:-1258291200}"
  validate_store "$store" >/dev/null
  if [[ ! "$max_archive_bytes" =~ ^[1-9][0-9]*$ ]]; then
    printf 'apple-compilation-cache: invalid archive byte ceiling: %s\n' "$max_archive_bytes" >&2
    exit 2
  fi
  if [[ -e "$archive" ]]; then
    printf 'apple-compilation-cache: refusing to overwrite archive: %s\n' "$archive" >&2
    exit 1
  fi
  archive_parent="$(dirname "$archive")"
  if [[ ! -d "$archive_parent" ]]; then
    printf 'apple-compilation-cache: archive directory is missing: %s\n' "$archive_parent" >&2
    exit 1
  fi
  temp_dir="$(mktemp -d "$archive.tmp.XXXXXX")"
  temp_archive="$temp_dir/cache.tar.zst"
  if ! tar -C "$store" -cf - . | zstd -1 -T0 -o "$temp_archive" >/dev/null; then
    find "$temp_dir" -mindepth 1 -depth -delete
    rmdir "$temp_dir"
    return 1
  fi
  archive_bytes="$(wc -c < "$temp_archive" | tr -d ' ')"
  if (( archive_bytes > max_archive_bytes )); then
    printf 'apple-compilation-cache: archive is %s bytes; ceiling is %s\n' \
      "$archive_bytes" "$max_archive_bytes" >&2
    find "$temp_dir" -mindepth 1 -depth -delete
    rmdir "$temp_dir"
    exit 1
  fi
  zstd -t "$temp_archive" >/dev/null
  mv "$temp_archive" "$archive"
  rmdir "$temp_dir"
  printf 'apple-compilation-cache: packed %s\n' "$archive"
}

restore_archive() {
  local archive="$1" store="$2" store_parent temp_dir extracted_store
  if [[ ! -f "$archive" ]]; then
    printf 'apple-compilation-cache: archive is missing: %s\n' "$archive" >&2
    exit 1
  fi
  if [[ -e "$store" ]]; then
    printf 'apple-compilation-cache: refusing to overwrite store: %s\n' "$store" >&2
    exit 1
  fi
  store_parent="$(dirname "$store")"
  if [[ ! -d "$store_parent" ]]; then
    printf 'apple-compilation-cache: store directory is missing: %s\n' "$store_parent" >&2
    exit 1
  fi
  zstd -t "$archive" >/dev/null
  temp_dir="$(mktemp -d "$store.tmp.XXXXXX")"
  extracted_store="$temp_dir/store"
  mkdir "$extracted_store"
  if ! zstd -dc "$archive" | tar -C "$extracted_store" -xf -; then
    find "$temp_dir" -mindepth 1 -depth -delete
    rmdir "$temp_dir"
    return 1
  fi
  if ! validate_store "$extracted_store" >/dev/null; then
    find "$temp_dir" -mindepth 1 -depth -delete
    rmdir "$temp_dir"
    return 1
  fi
  mv "$extracted_store" "$store"
  rmdir "$temp_dir"
  printf 'apple-compilation-cache: restored %s\n' "$store"
}

admit_bytes() {
  local required_bytes="$1" usage_json="$2" label="$3" active_bytes projected_bytes
  local budget_bytes="${LOOMARR_APPLE_CACHE_REPOSITORY_BUDGET_BYTES:-10737418240}"
  local reserve_bytes="${LOOMARR_APPLE_CACHE_RESERVE_BYTES:-536870912}"
  local max_archive_bytes="${LOOMARR_APPLE_CACHE_MAX_ARCHIVE_BYTES:-1258291200}"
  for byte_value in "$budget_bytes" "$reserve_bytes" "$max_archive_bytes"; do
    if [[ ! "$byte_value" =~ ^[0-9]+$ ]]; then
      printf 'apple-compilation-cache: invalid byte bound: %s\n' "$byte_value" >&2
      exit 2
    fi
  done
  if [[ ! -f "$usage_json" ]]; then
    printf 'apple-compilation-cache: admission inputs are missing\n' >&2
    exit 1
  fi
  active_bytes="$(jq -er '.active_caches_size_in_bytes | select(type == "number" and . >= 0 and floor == .)' "$usage_json")"
  if (( required_bytes > max_archive_bytes )); then
    printf 'apple-compilation-cache: archive is %s bytes; ceiling is %s\n' \
      "$required_bytes" "$max_archive_bytes" >&2
    exit 1
  fi
  projected_bytes=$((active_bytes + required_bytes + reserve_bytes))
  if (( projected_bytes > budget_bytes )); then
    printf 'apple-compilation-cache: %s refused; projected use with reserve is %s of %s bytes\n' \
      "$label" "$projected_bytes" "$budget_bytes" >&2
    exit 1
  fi
  printf 'apple-compilation-cache: %s admitted\n' "$label"
}

admit_capacity() {
  local usage_json="$1"
  local max_archive_bytes="${LOOMARR_APPLE_CACHE_MAX_ARCHIVE_BYTES:-1258291200}"
  admit_bytes "$max_archive_bytes" "$usage_json" capacity
}

admit_save() {
  local archive="$1" usage_json="$2" archive_bytes
  if [[ ! -f "$archive" ]]; then
    printf 'apple-compilation-cache: admission inputs are missing\n' >&2
    exit 1
  fi
  zstd -t "$archive" >/dev/null
  archive_bytes="$(wc -c < "$archive" | tr -d ' ')"
  admit_bytes "$archive_bytes" "$usage_json" save
}

retention_plan() {
  local caches_json="$1" prefix="$2" ref="$3" keep="$4"
  if [[ ! -f "$caches_json" ]]; then
    printf 'apple-compilation-cache: cache inventory is missing: %s\n' "$caches_json" >&2
    exit 1
  fi
  if [[ ! "$prefix" =~ ^apple-compilation-cache-v1-[0-9a-f]{64}$ ]]; then
    printf 'apple-compilation-cache: refusing unsafe retention prefix: %s\n' "$prefix" >&2
    exit 2
  fi
  if [[ "$ref" != refs/heads/main ]]; then
    printf 'apple-compilation-cache: refusing retention outside refs/heads/main: %s\n' "$ref" >&2
    exit 2
  fi
  if [[ "$keep" != 1 && "$keep" != 2 ]]; then
    printf 'apple-compilation-cache: retention must keep one or two generations\n' >&2
    exit 2
  fi
  jq -er --arg prefix "$prefix-" --arg ref "$ref" --argjson keep "$keep" '
    .actions_caches as $caches
    | if ($caches | type) != "array" then error("actions_caches must be an array") else $caches end
    | [
        .[]
        | select(.ref == $ref)
        | select((.key | type) == "string" and (.key | startswith($prefix)))
      ] as $matching
    | if any($matching[];
        (.id | type) != "number"
        or (.id < 1 or (.id | floor) != .id)
        or (.created_at | type) != "string"
      ) then error("matching cache metadata is malformed") else $matching end
    | sort_by(.created_at, .id)
    | reverse
    | .[$keep:]
    | .[].id
  ' "$caches_json"
}

diagnostics() {
  local log="$1" requirement="$2" metrics hits misses skipped
  if [[ ! -f "$log" ]]; then
    printf 'apple-compilation-cache: Xcode log is missing: %s\n' "$log" >&2
    exit 1
  fi
  metrics="$(jq -er '
    [
      .. | objects
      | .data?
      | select(type == "string")
      | fromjson?
      | .counters?
      | select(type == "object")
    ]
    | add // {}
    | def count($name):
        (.[$name] // 0) as $value
        | if ($value | type) == "number"
            and $value >= 0
            and ($value | floor) == $value
          then $value
          else error("invalid Xcode cache counter: \($name)")
          end;
    [
      count("clangCacheHits") + count("swiftCacheHits"),
      count("clangCacheMisses") + count("swiftCacheMisses"),
      count("clangCacheSkipped") + count("swiftCacheSkipped")
    ]
    | @tsv
  ' "$log")"
  read -r hits misses skipped <<< "$metrics"
  case "$requirement" in
    hits)
      if (( hits == 0 )); then
        printf 'apple-compilation-cache: expected at least one cache hit\n' >&2
        exit 1
      fi
      ;;
    hits-and-misses)
      if (( hits == 0 || misses == 0 )); then
        printf 'apple-compilation-cache: expected both cache hits and misses\n' >&2
        exit 1
      fi
      ;;
    report) ;;
    *)
      printf 'apple-compilation-cache: diagnostic requirement must be report, hits, or hits-and-misses\n' >&2
      exit 2
      ;;
  esac
  printf 'hits=%s\nmisses=%s\nskipped=%s\n' "$hits" "$misses" "$skipped"
}

usage() {
  printf 'usage: %s fingerprint | validate-store STORE | pack STORE ARCHIVE | restore ARCHIVE STORE | admit-capacity USAGE_JSON | admit-save ARCHIVE USAGE_JSON | retention-plan CACHES_JSON PREFIX REF KEEP | diagnostics LOG REQUIREMENT\n' "$0" >&2
  exit 2
}

case "${1:-}" in
  fingerprint)
    [[ $# -eq 1 ]] || usage
    fingerprint
    ;;
  validate-store)
    [[ $# -eq 2 ]] || usage
    validate_store "$2"
    ;;
  pack)
    [[ $# -eq 3 ]] || usage
    pack_store "$2" "$3"
    ;;
  restore)
    [[ $# -eq 3 ]] || usage
    restore_archive "$2" "$3"
    ;;
  admit-capacity)
    [[ $# -eq 2 ]] || usage
    admit_capacity "$2"
    ;;
  admit-save)
    [[ $# -eq 3 ]] || usage
    admit_save "$2" "$3"
    ;;
  retention-plan)
    [[ $# -eq 5 ]] || usage
    retention_plan "$2" "$3" "$4" "$5"
    ;;
  diagnostics)
    [[ $# -eq 3 ]] || usage
    diagnostics "$2" "$3"
    ;;
  *)
    usage
    ;;
esac
