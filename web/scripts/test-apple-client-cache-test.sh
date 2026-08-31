#!/usr/bin/env bash
set -euo pipefail

web_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
verifier="$web_root/scripts/test-apple-client.sh"
test_root="$(mktemp -d /tmp/apple-client-cache-test.XXXXXX)"
trap 'find "$test_root" -mindepth 1 -depth -delete; rmdir "$test_root"' EXIT
mkdir -p "$test_root/bin" "$test_root/store" "$test_root/artifacts" "$test_root/build"
printf 'CAS object\n' > "$test_root/store/object"

cat > "$test_root/bin/uname" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  -s) printf 'Darwin\n' ;;
  -m) printf 'arm64\n' ;;
  *) exit 64 ;;
esac
STUB

cat > "$test_root/bin/xcodebuild" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$*" == -version ]]; then
  printf 'Xcode 26.6\nBuild version 17F113\n'
  exit 0
fi
result_bundle=''
while [[ $# -gt 0 ]]; do
  if [[ "$1" == -resultBundlePath ]]; then
    result_bundle="$2"
    shift 2
    continue
  fi
  shift
done
if [[ -z "$result_bundle" ]]; then
  printf 'main xcodebuild did not receive -resultBundlePath\n' >&2
  exit 64
fi
mkdir -p "$result_bundle"
printf 'result bundle fixture\n' > "$result_bundle/build-log"
printf 'xcodebuild fixture\n'
STUB

cat > "$test_root/bin/llvm-cas" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
if [[ $# -ne 5 || "$1" != --cas || ! -f "$2/object" || "$3" != --validate \
  || "$4" != --check-hash || "$5" != --force ]]; then
  exit 64
fi
printf 'validated successfully\n'
STUB

cat > "$test_root/bin/xcrun" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
case "$1" in
  swift)
    printf 'Apple Swift version 6.3.3\n'
    ;;
  --find)
    [[ "$2" == llvm-cas ]]
    printf '%s/bin/llvm-cas\n' "$APPLE_CACHE_TEST_ROOT"
    ;;
  simctl)
    case "$2" in
      list)
        printf '{"devices":{"com.apple.CoreSimulator.SimRuntime.iOS-26-5":[{"udid":"SIM-1","isAvailable":true,"state":"Booted"}]}}\n'
        ;;
      bootstatus)
        ;;
      launch)
        printf 'media.loomarr.mobile.prototype: %s\n' "$APPLE_CACHE_TEST_LIVE_PID"
        ;;
      io)
        if [[ ! -e "$5.attempted" ]]; then
          : > "$5.attempted"
          printf 'Timeout waiting for screen surfaces\n' >&2
          exit 60
        fi
        printf 'PNG fixture\n' > "$5"
        ;;
      *)
        printf 'unexpected simctl invocation: %s\n' "$*" >&2
        exit 64
        ;;
    esac
    ;;
  lipo)
    printf 'arm64\n'
    ;;
  xcresulttool)
    if [[ "$*" != "xcresulttool get log --path $APPLE_CACHE_TEST_ROOT/artifacts/build-warm.xcresult --type build" ]]; then
      printf 'unexpected xcresulttool invocation: %s\n' "$*" >&2
      exit 64
    fi
    hits=1
    misses=0
    if [[ -n "${LOOMARR_APPLE_CACHE_SOURCE_PROBE:-}" ]]; then
      misses=1
    fi
    printf '{"attachments":[{"data":"{\\"counters\\":{\\"clangCacheHits\\":%s,\\"clangCacheMisses\\":%s}}"}]}' \
      "$hits" "$misses"
    ;;
  *)
    printf 'unexpected xcrun invocation: %s\n' "$*" >&2
    exit 64
    ;;
esac
STUB

cat > "$test_root/bin/pnpm" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$*" == *'expo prebuild'* ]]; then
  printf 'prebuild fixture\n'
  exit 0
fi
if [[ "$*" != *'expo run:ios'* ]]; then
  printf 'unexpected pnpm invocation: %s\n' "$*" >&2
  exit 64
fi
count=0
if [[ -f "$APPLE_CACHE_TEST_ROOT/build-count" ]]; then
  count="$(cat "$APPLE_CACHE_TEST_ROOT/build-count")"
fi
count=$((count + 1))
printf '%s\n' "$count" > "$APPLE_CACHE_TEST_ROOT/build-count"
case "$count" in
  1)
    [[ "$XCODE_XCCONFIG_FILE" == */apple-compilation-cache.xcconfig ]]
    [[ "$LOOMARR_APPLE_CACHE_STORE" == "$APPLE_CACHE_TEST_ROOT/store" ]]
    xcodebuild -workspace Fixture.xcworkspace -scheme Fixture build
    if [[ "${APPLE_CACHE_TEST_WARM_RESULT:-fail}" == pass ]]; then
      if [[ "${LOOMARR_APPLE_CACHE_POPULATE:-0}" == 1 ]]; then
        printf 'CAS object\n' > "$LOOMARR_APPLE_CACHE_STORE/object"
      fi
      mkdir -p "$LOOMARR_APPLE_BUILD_DIR/LoomarrMobilePrototype.app"
      printf 'arm64 fixture\n' > "$LOOMARR_APPLE_BUILD_DIR/LoomarrMobilePrototype.app/LoomarrMobilePrototype"
      printf 'warm compile fixture passed\n'
      exit 0
    fi
    printf 'warm compile fixture failed\n'
    exit 42
    ;;
  2)
    [[ "$XCODE_XCCONFIG_FILE" == */apple-simulator.xcconfig ]]
    [[ "$*" == *'--no-build-cache'* ]]
    printf 'remark: cold compile job\n' >> "$LOOMARR_APPLE_RAW_XCODE_LOG"
    mkdir -p "$LOOMARR_APPLE_BUILD_DIR/LoomarrMobilePrototype.app"
    printf 'arm64 fixture\n' > "$LOOMARR_APPLE_BUILD_DIR/LoomarrMobilePrototype.app/LoomarrMobilePrototype"
    printf 'cold compile fixture passed\n'
    ;;
  *)
    printf 'unexpected build count: %s\n' "$count" >&2
    exit 64
    ;;
esac
STUB

cat > "$test_root/bin/sleep" <<'STUB'
#!/usr/bin/env bash
exit 0
STUB
chmod +x "$test_root/bin/"*

set +e
output="$(
  PATH="$test_root/bin:$PATH" \
    APPLE_CACHE_TEST_ROOT="$test_root" \
    APPLE_CACHE_TEST_LIVE_PID="$$" \
    LOOMARR_APPLE_ARTIFACTS_DIR="$test_root/artifacts" \
    LOOMARR_APPLE_BUILD_DIR="$test_root/build" \
    LOOMARR_APPLE_CACHE_MODE=warm \
    LOOMARR_APPLE_CACHE_STORE="$test_root/store" \
    "$verifier" mobile 2>&1
)"
status=$?
set -e
if [[ $status -ne 0 ]]; then
  printf 'test-apple-client-cache-test: warm fallback failed with %s:\n%s\n' "$status" "$output" >&2
  exit 1
fi
if [[ "$(cat "$test_root/build-count")" != 2 ]]; then
  printf 'test-apple-client-cache-test: verifier did not make exactly two build attempts\n' >&2
  exit 1
fi
if [[ -e "$test_root/store" || ! -f "$test_root/artifacts/quarantine/store/object" ]]; then
  printf 'test-apple-client-cache-test: failed warm store was not quarantined\n' >&2
  exit 1
fi
for log in build-warm.log build-cold.log xcodebuild-warm.log xcodebuild-cold.log; do
  if [[ ! -s "$test_root/artifacts/$log" ]]; then
    printf 'test-apple-client-cache-test: missing raw %s\n' "$log" >&2
    exit 1
  fi
done
if [[ ! -s "$test_root/artifacts/build-warm.xcresult.json" ]]; then
  printf 'test-apple-client-cache-test: structured cache diagnostics were not preserved\n' >&2
  exit 1
fi
for evidence in \
  'apple-client: warm build failed; quarantined compilation cache and retrying cold' \
  'apple-client: mobile simulator executable contains only arm64' \
  'apple-client: mobile built, installed, launched, and remained alive on SIM-1'; do
  if [[ "$output" != *"$evidence"* ]]; then
    printf 'test-apple-client-cache-test: missing evidence: %s\n' "$evidence" >&2
    exit 1
  fi
done

find "$test_root/artifacts" -mindepth 1 -depth -delete
find "$test_root/build" -mindepth 1 -depth -delete
mkdir "$test_root/store"
printf 'CAS object\n' > "$test_root/store/object"
unlink "$test_root/build-count"
set +e
output="$(
  PATH="$test_root/bin:$PATH" \
    APPLE_CACHE_TEST_ROOT="$test_root" \
    APPLE_CACHE_TEST_LIVE_PID="$$" \
    APPLE_CACHE_TEST_WARM_RESULT=pass \
    LOOMARR_APPLE_ARTIFACTS_DIR="$test_root/artifacts" \
    LOOMARR_APPLE_BUILD_DIR="$test_root/build" \
    LOOMARR_APPLE_CACHE_MODE=warm \
    LOOMARR_APPLE_CACHE_STORE="$test_root/store" \
    LOOMARR_APPLE_CACHE_DIAGNOSTIC_REQUIREMENT=hits \
    "$verifier" mobile 2>&1
)"
status=$?
set -e
if [[ $status -ne 0 ]]; then
  printf 'test-apple-client-cache-test: successful warm build failed with %s:\n%s\n' "$status" "$output" >&2
  exit 1
fi
if [[ "$(cat "$test_root/build-count")" != 1 ]] \
  || [[ -e "$test_root/artifacts/build-cold.log" ]] \
  || [[ -e "$test_root/artifacts/quarantine" ]]; then
  printf 'test-apple-client-cache-test: successful warm build entered fallback\n' >&2
  exit 1
fi
if [[ "$(cat "$test_root/artifacts/cache-diagnostics.env")" != $'hits=1\nmisses=0\nskipped=0' ]]; then
  printf 'test-apple-client-cache-test: warm diagnostics were not verified\n' >&2
  exit 1
fi

find "$test_root/artifacts" -mindepth 1 -depth -delete
find "$test_root/build" -mindepth 1 -depth -delete
find "$test_root/store" -mindepth 1 -depth -delete
unlink "$test_root/build-count"
set +e
output="$(
  PATH="$test_root/bin:$PATH" \
    APPLE_CACHE_TEST_ROOT="$test_root" \
    APPLE_CACHE_TEST_LIVE_PID="$$" \
    APPLE_CACHE_TEST_WARM_RESULT=pass \
    LOOMARR_APPLE_ARTIFACTS_DIR="$test_root/artifacts" \
    LOOMARR_APPLE_BUILD_DIR="$test_root/build" \
    LOOMARR_APPLE_CACHE_MODE=warm \
    LOOMARR_APPLE_CACHE_POPULATE=1 \
    LOOMARR_APPLE_CACHE_STORE="$test_root/store" \
    "$verifier" mobile 2>&1
)"
status=$?
set -e
if [[ $status -ne 0 ]]; then
  printf 'test-apple-client-cache-test: empty-store population failed with %s:\n%s\n' "$status" "$output" >&2
  exit 1
fi
if [[ "$(cat "$test_root/build-count")" != 1 ]] \
  || [[ ! -f "$test_root/store/object" ]] \
  || [[ "$output" != *'apple-client: populated and validated compilation cache'* ]]; then
  printf 'test-apple-client-cache-test: writer did not validate its populated CAS\n' >&2
  exit 1
fi

find "$test_root/artifacts" -mindepth 1 -depth -delete
find "$test_root/build" -mindepth 1 -depth -delete
unlink "$test_root/build-count"
printf 'int fixture;\n' > "$test_root/native.mm"
set +e
output="$(
  PATH="$test_root/bin:$PATH" \
    APPLE_CACHE_TEST_ROOT="$test_root" \
    APPLE_CACHE_TEST_LIVE_PID="$$" \
    APPLE_CACHE_TEST_WARM_RESULT=pass \
    LOOMARR_APPLE_ARTIFACTS_DIR="$test_root/artifacts" \
    LOOMARR_APPLE_BUILD_DIR="$test_root/build" \
    LOOMARR_APPLE_CACHE_MODE=warm \
    LOOMARR_APPLE_CACHE_STORE="$test_root/store" \
    LOOMARR_APPLE_CACHE_SOURCE_PROBE=source-change \
    LOOMARR_APPLE_CACHE_SOURCE_PROBE_FILE="$test_root/native.mm" \
    LOOMARR_APPLE_CACHE_DIAGNOSTIC_REQUIREMENT=hits-and-misses \
    "$verifier" mobile 2>&1
)"
status=$?
set -e
if [[ $status -ne 0 ]]; then
  printf 'test-apple-client-cache-test: source invalidation probe failed with %s:\n%s\n' "$status" "$output" >&2
  exit 1
fi
if ! grep -F '// Loomarr cache invalidation probe: source-change' "$test_root/native.mm" >/dev/null \
  || [[ "$(cat "$test_root/artifacts/cache-diagnostics.env")" != $'hits=1\nmisses=1\nskipped=0' ]]; then
  printf 'test-apple-client-cache-test: source change did not prove cache hits and misses\n' >&2
  exit 1
fi

echo 'test-apple-client-cache-test: ok'
