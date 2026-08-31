#!/usr/bin/env bash
set -euo pipefail

web_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
validator="$web_root/scripts/validate-apple-compilation-cache.sh"
test_root="$(mktemp -d /tmp/apple-cache-validation-test.XXXXXX)"
trap 'find "$test_root" -mindepth 1 -depth -delete; rmdir "$test_root"' EXIT
mkdir -p "$test_root/bin"

cat > "$test_root/bin/make" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
app=''
for argument in "$@"; do
  case "$argument" in CLIENT_APP=*) app="${argument#CLIENT_APP=}" ;; esac
done
printf '%s %s populate=%s diagnostics=%s probe=%s\n' \
  "$app" "${LOOMARR_APPLE_CACHE_MODE:-}" \
  "${LOOMARR_APPLE_CACHE_POPULATE:-0}" \
  "${LOOMARR_APPLE_CACHE_DIAGNOSTIC_REQUIREMENT:-report}" \
  "${LOOMARR_APPLE_CACHE_SOURCE_PROBE:-}" \
  >> "$APPLE_CACHE_VALIDATION_TEST_ROOT/make.calls"
mkdir -p "$LOOMARR_APPLE_CACHE_STORE"
printf 'CAS object\n' > "$LOOMARR_APPLE_CACHE_STORE/object"
STUB

cat > "$test_root/bin/cache-cli" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
case "$1" in
  fingerprint)
    if [[ -n "${LOOMARR_APPLE_LOCKFILE:-}" ]]; then
      printf 'apple-compilation-cache-v1-changed\n'
    else
      printf 'apple-compilation-cache-v1-base\n'
    fi
    ;;
  pack)
    printf 'archive fixture\n' > "$3"
    ;;
  restore)
    if [[ "$2" == *corrupt* ]]; then exit 1; fi
    mkdir "$3"
    printf 'CAS object\n' > "$3/object"
    ;;
  validate-store)
    [[ -f "$2/object" ]]
    ;;
  *)
    printf 'unexpected cache CLI command: %s\n' "$*" >&2
    exit 64
    ;;
esac
STUB
chmod +x "$test_root/bin/make" "$test_root/bin/cache-cli"

PATH="$test_root/bin:$PATH" \
  APPLE_CACHE_VALIDATION_TEST_ROOT="$test_root" \
  LOOMARR_APPLE_CACHE_CLI="$test_root/bin/cache-cli" \
  LOOMARR_APPLE_VALIDATION_ROOT="$test_root/producer" \
  "$validator" produce "$test_root/cache.tar.zst"

PATH="$test_root/bin:$PATH" \
  APPLE_CACHE_VALIDATION_TEST_ROOT="$test_root" \
  LOOMARR_APPLE_CACHE_CLI="$test_root/bin/cache-cli" \
  LOOMARR_APPLE_VALIDATION_ROOT="$test_root/consumer" \
  "$validator" consume "$test_root/cache.tar.zst"

want=$'mobile warm populate=1 diagnostics=report probe=\ntv warm populate=1 diagnostics=report probe=\nmobile warm populate=0 diagnostics=hits probe=\ntv warm populate=0 diagnostics=hits probe=\nmobile warm populate=0 diagnostics=hits-and-misses probe=source-change\nmobile cold populate=0 diagnostics=report probe=\ntv cold populate=0 diagnostics=report probe='
got="$(cat "$test_root/make.calls")"
if [[ "$got" != "$want" ]]; then
  printf 'validate-apple-compilation-cache-test: call sequence got:\n%s\n' "$got" >&2
  exit 1
fi

echo 'validate-apple-compilation-cache-test: ok'
