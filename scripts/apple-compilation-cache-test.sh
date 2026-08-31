#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cache_cli="$root/scripts/apple-compilation-cache.sh"
test_root="$(mktemp -d /tmp/apple-compilation-cache-test.XXXXXX)"
trap 'find "$test_root" -mindepth 1 -depth -delete; rmdir "$test_root"' EXIT
mkdir -p "$test_root/bin"
printf 'lock fixture\n' > "$test_root/pnpm-lock.yaml"
cat > "$test_root/apple-cache.xcconfig" <<'XCCONFIG'
ONLY_ACTIVE_ARCH = YES
COMPILATION_CACHE_ENABLE_CACHING = YES
COMPILATION_CACHE_ENABLE_DIAGNOSTIC_REMARKS = YES
COMPILATION_CACHE_CAS_PATH = $(LOOMARR_APPLE_CACHE_STORE)
SWIFT_ENABLE_EXPLICIT_MODULES = YES
XCCONFIG

cat > "$test_root/bin/xcodebuild" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
case "$*" in
  -version)
    printf '%b' "${APPLE_CACHE_TEST_XCODE_VERSION:-Xcode 26.6\\nBuild version 17F113\\n}"
    ;;
  '-version -sdk iphonesimulator')
    printf '%b' "${APPLE_CACHE_TEST_IOS_SDK:-iPhoneSimulator26.5.sdk - iOS Simulator 26.5\\n}"
    ;;
  '-version -sdk appletvsimulator')
    printf '%b' "${APPLE_CACHE_TEST_TV_SDK:-AppleTVSimulator26.5.sdk - tvOS Simulator 26.5\\n}"
    ;;
  *)
    printf 'unexpected xcodebuild invocation: %s\n' "$*" >&2
    exit 64
    ;;
esac
STUB

cat > "$test_root/bin/xcrun" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
case "$*" in
  'swiftc --version')
    printf '%b' "${APPLE_CACHE_TEST_SWIFT_VERSION:-Apple Swift version 6.3.3\\nTarget: arm64-apple-macosx26.0\\n}"
    ;;
  '--find llvm-cas')
    printf '%s/bin/llvm-cas\n' "$APPLE_CACHE_TEST_ROOT"
    ;;
  *)
    printf 'unexpected xcrun invocation: %s\n' "$*" >&2
    exit 64
    ;;
esac
STUB

cat > "$test_root/bin/llvm-cas" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
if [[ $# -ne 5 || "$1" != --cas || ! -d "$2" || "$3" != --validate \
  || "$4" != --check-hash || "$5" != --force ]]; then
  printf 'unexpected llvm-cas invocation: %s\n' "$*" >&2
  exit 64
fi
if [[ ! -f "$2/object" ]]; then
  printf 'llvm-cas fixture store has no object: %s\n' "$2" >&2
  exit 1
fi
printf 'validated successfully\n'
STUB
chmod +x "$test_root/bin/xcodebuild" "$test_root/bin/xcrun" "$test_root/bin/llvm-cas"

fingerprint_with() {
  env \
    PATH="$test_root/bin:$PATH" \
    RUNNER_OS=macOS \
    RUNNER_ARCH=ARM64 \
    LOOMARR_APPLE_LOCKFILE="$test_root/pnpm-lock.yaml" \
    LOOMARR_APPLE_CACHE_XCCONFIG="$test_root/apple-cache.xcconfig" \
    "$@" \
    "$cache_cli" fingerprint
}

got="$(fingerprint_with)"
want=apple-compilation-cache-v1-c8573f56dcb8bdb0c8b47d2d910535b4d9ce08a4a9ba9ffacc29315769db4a0c
if [[ "$got" != "$want" ]]; then
  printf 'apple-compilation-cache-test: fingerprint got %s, want %s\n' "$got" "$want" >&2
  exit 1
fi

cp "$test_root/apple-cache.xcconfig" "$test_root/changed.xcconfig"
printf 'OTHER_CFLAGS = -DCHANGED\n' >> "$test_root/changed.xcconfig"
changed="$(
  PATH="$test_root/bin:$PATH" \
    RUNNER_OS=macOS \
    RUNNER_ARCH=ARM64 \
    LOOMARR_APPLE_LOCKFILE="$test_root/pnpm-lock.yaml" \
    LOOMARR_APPLE_CACHE_XCCONFIG="$test_root/changed.xcconfig" \
    "$cache_cli" fingerprint
)"
if [[ "$changed" == "$got" ]]; then
  printf 'apple-compilation-cache-test: xcconfig change did not invalidate fingerprint\n' >&2
  exit 1
fi

for invalidation in \
  'RUNNER_OS=changed-os' \
  'RUNNER_ARCH=changed-arch' \
  'APPLE_CACHE_TEST_XCODE_VERSION=Xcode 26.7\nBuild version changed\n' \
  'APPLE_CACHE_TEST_SWIFT_VERSION=Apple Swift version changed\n' \
  'APPLE_CACHE_TEST_IOS_SDK=iPhoneSimulator changed\n' \
  'APPLE_CACHE_TEST_TV_SDK=AppleTVSimulator changed\n'; do
  changed="$(fingerprint_with "$invalidation")"
  if [[ "$changed" == "$got" ]]; then
    printf 'apple-compilation-cache-test: %s did not invalidate fingerprint\n' "${invalidation%%=*}" >&2
    exit 1
  fi
done
cp "$test_root/pnpm-lock.yaml" "$test_root/changed-lock.yaml"
printf 'changed dependency\n' >> "$test_root/changed-lock.yaml"
changed="$(fingerprint_with "LOOMARR_APPLE_LOCKFILE=$test_root/changed-lock.yaml")"
if [[ "$changed" == "$got" ]]; then
  printf 'apple-compilation-cache-test: lockfile change did not invalidate fingerprint\n' >&2
  exit 1
fi

mkdir "$test_root/store"
printf 'CAS fixture\n' > "$test_root/store/object"
got="$(
  PATH="$test_root/bin:$PATH" \
    APPLE_CACHE_TEST_ROOT="$test_root" \
    "$cache_cli" validate-store "$test_root/store"
)"
want="apple-compilation-cache: validated $test_root/store"
if [[ "$got" != "$want" ]]; then
  printf 'apple-compilation-cache-test: validation got %s, want %s\n' "$got" "$want" >&2
  exit 1
fi

archive="$test_root/cache.tar.zst"
got="$(
  PATH="$test_root/bin:$PATH" \
    APPLE_CACHE_TEST_ROOT="$test_root" \
    "$cache_cli" pack "$test_root/store" "$archive"
)"
if [[ ! -s "$archive" ]] || ! zstd -t "$archive" >/dev/null 2>&1; then
  printf 'apple-compilation-cache-test: pack did not create a valid zstd archive\n' >&2
  exit 1
fi
if [[ "$got" != "apple-compilation-cache: packed $archive" ]]; then
  printf 'apple-compilation-cache-test: pack output got %s\n' "$got" >&2
  exit 1
fi

got="$(
  PATH="$test_root/bin:$PATH" \
    APPLE_CACHE_TEST_ROOT="$test_root" \
    "$cache_cli" restore "$archive" "$test_root/restored"
)"
if [[ ! -f "$test_root/restored/object" ]]; then
  printf 'apple-compilation-cache-test: restore did not recover the CAS object\n' >&2
  exit 1
fi
if [[ "$got" != "apple-compilation-cache: restored $test_root/restored" ]]; then
  printf 'apple-compilation-cache-test: restore output got %s\n' "$got" >&2
  exit 1
fi

cp "$archive" "$test_root/corrupt.tar.zst"
truncate -s 32 "$test_root/corrupt.tar.zst"
if PATH="$test_root/bin:$PATH" \
  APPLE_CACHE_TEST_ROOT="$test_root" \
  "$cache_cli" restore "$test_root/corrupt.tar.zst" "$test_root/corrupt-store" \
  >/dev/null 2>&1; then
  printf 'apple-compilation-cache-test: corrupt archive was accepted\n' >&2
  exit 1
fi
if [[ -e "$test_root/corrupt-store" ]]; then
  printf 'apple-compilation-cache-test: corrupt archive exposed a store\n' >&2
  exit 1
fi

if PATH="$test_root/bin:$PATH" \
  APPLE_CACHE_TEST_ROOT="$test_root" \
  LOOMARR_APPLE_CACHE_MAX_ARCHIVE_BYTES=1 \
  "$cache_cli" pack "$test_root/store" "$test_root/oversize.tar.zst" \
  >/dev/null 2>&1; then
  printf 'apple-compilation-cache-test: oversized archive was admitted\n' >&2
  exit 1
fi
if [[ -e "$test_root/oversize.tar.zst" ]]; then
  printf 'apple-compilation-cache-test: oversized archive escaped its temporary directory\n' >&2
  exit 1
fi

printf '{"active_caches_size_in_bytes":1000,"active_caches_count":2}\n' > "$test_root/usage.json"
got="$(
  LOOMARR_APPLE_CACHE_REPOSITORY_BUDGET_BYTES=2000000 \
    LOOMARR_APPLE_CACHE_RESERVE_BYTES=1000 \
    "$cache_cli" admit-save "$archive" "$test_root/usage.json"
)"
if [[ "$got" != 'apple-compilation-cache: save admitted' ]]; then
  printf 'apple-compilation-cache-test: admission output got %s\n' "$got" >&2
  exit 1
fi
if LOOMARR_APPLE_CACHE_REPOSITORY_BUDGET_BYTES=1000 \
  LOOMARR_APPLE_CACHE_RESERVE_BYTES=1 \
  "$cache_cli" admit-save "$archive" "$test_root/usage.json" \
  >/dev/null 2>&1; then
  printf 'apple-compilation-cache-test: save exceeded repository headroom\n' >&2
  exit 1
fi

cat > "$test_root/caches.json" <<'JSON'
{
  "actions_caches": [
    {"id": 10, "ref": "refs/heads/main", "key": "apple-compilation-cache-v1-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-100", "created_at": "2026-08-28T00:00:00Z"},
    {"id": 11, "ref": "refs/heads/main", "key": "apple-compilation-cache-v1-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-101", "created_at": "2026-08-29T00:00:00Z"},
    {"id": 12, "ref": "refs/heads/main", "key": "apple-compilation-cache-v1-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-102", "created_at": "2026-08-30T00:00:00Z"},
    {"id": 13, "ref": "refs/heads/gh-readonly-queue/main/batch", "key": "apple-compilation-cache-v1-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-099", "created_at": "2026-08-27T00:00:00Z"},
    {"id": 14, "ref": "refs/heads/main", "key": "apple-compilation-cache-v1-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb-100", "created_at": "2026-08-28T00:00:00Z"}
  ]
}
JSON
got="$(
  "$cache_cli" retention-plan \
    "$test_root/caches.json" \
    apple-compilation-cache-v1-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
    refs/heads/main \
    2
)"
if [[ "$got" != 10 ]]; then
  printf 'apple-compilation-cache-test: retention plan got %s, want only 10\n' "$got" >&2
  exit 1
fi

cp "$test_root/caches.json" "$test_root/malformed-caches.json"
jq '.actions_caches += [{"id":"unsafe", "ref":"refs/heads/main", "key":"apple-compilation-cache-v1-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-103", "created_at":"2026-08-31T00:00:00Z"}]' \
  "$test_root/caches.json" > "$test_root/malformed-caches.json"
if "$cache_cli" retention-plan \
  "$test_root/malformed-caches.json" \
  apple-compilation-cache-v1-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
  refs/heads/main \
  2 >/dev/null 2>&1; then
  printf 'apple-compilation-cache-test: retention accepted malformed matching metadata\n' >&2
  exit 1
fi

cat > "$test_root/xcodebuild.log" <<'LOG'
/fixture/a.mm: remark: compile job cache hit for 'a.o' => 'cas://a'
/fixture/b.swift: remark: compile job cache miss for 'b.o'
/fixture/c.cpp: remark: compile job cache hit for 'c.o' => 'cas://c'
/fixture/d.m: remark: compile job cache skipped for 'd.o'
LOG
got="$("$cache_cli" diagnostics "$test_root/xcodebuild.log" hits-and-misses)"
want=$'hits=2\nmisses=1\nskipped=1'
if [[ "$got" != "$want" ]]; then
  printf 'apple-compilation-cache-test: diagnostics got:\n%s\n' "$got" >&2
  exit 1
fi

echo 'apple-compilation-cache-test: ok'
