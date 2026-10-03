#!/usr/bin/env bash
# Fail-closed contract of the iPhone TestFlight script. macOS-only steps are not run here; the
# argument, credential, and altool paths are, with a fake xcrun.
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
script="$script_dir/build-ios-testflight.sh"

temp_dir=$(mktemp -d)
trap 'rm -rf -- "$temp_dir"' EXIT
key_dir="$temp_dir/asc"
mkdir -p "$key_dir" "$temp_dir/bin" "$temp_dir/out"
printf 'not a real key' > "$key_dir/AuthKey_ABCDE12345.p8"

valid_env=(
  LOOMARR_IOS_BUILD_NUMBER=42
  LOOMARR_APPLE_TEAM_ID=TEAM123456
  LOOMARR_ASC_KEY_PATH="$key_dir/AuthKey_ABCDE12345.p8"
  LOOMARR_ASC_KEY_ID=ABCDE12345
  LOOMARR_ASC_ISSUER_ID=69a6de70-0000-0000-0000-000000000000
  LOOMARR_ASC_APPLE_APP_ID=1234567890
  LOOMARR_IOS_OUTPUT_DIR="$temp_dir/out"
)

# Runs the script with the valid environment, plus overrides, and captures stderr and stdout.
run() {
  env "${valid_env[@]}" "$@" >"$temp_dir/stdout" 2>"$temp_dir/stderr"
}
expect_failure() {
  local description="$1" needle="$2"
  shift 2
  if run "$@"; then
    echo "$description: the script succeeded" >&2
    exit 1
  fi
  if ! grep -Fq -- "$needle" "$temp_dir/stderr" "$temp_dir/stdout"; then
    echo "$description: expected output containing: $needle" >&2
    cat "$temp_dir/stderr" >&2
    exit 1
  fi
}

prepare_env=(
  ASC_API_KEY_P8_BASE64="$(printf 'private key bytes' | base64)"
  ASC_API_KEY_ID=ABCDE12345
  ASC_API_ISSUER_ID=69a6de70-0000-0000-0000-000000000000
  APPLE_TEAM_ID=TEAM123456
  ASC_APPLE_APP_ID=1234567890
  RUNNER_TEMP="$temp_dir/runner"
  GITHUB_ENV="$temp_dir/github-env"
)
mkdir -p "$temp_dir/runner"
: > "$temp_dir/github-env"
for variable in ASC_API_KEY_P8_BASE64 ASC_API_KEY_ID ASC_API_ISSUER_ID APPLE_TEAM_ID ASC_APPLE_APP_ID; do
  if env "${prepare_env[@]}" "$variable=" "$script" prepare-credentials >"$temp_dir/stdout" 2>"$temp_dir/stderr"; then
    echo "prepare-credentials accepted an empty $variable" >&2
    exit 1
  fi
  grep -Fq "iPhone TestFlight is not configured" "$temp_dir/stdout"
  grep -Fq "$variable" "$temp_dir/stdout"
done
if env "${prepare_env[@]}" ASC_API_KEY_ID=short "$script" prepare-credentials >/dev/null 2>&1; then
  echo 'prepare-credentials accepted a malformed key id' >&2
  exit 1
fi
env "${prepare_env[@]}" "$script" prepare-credentials >"$temp_dir/stdout" 2>"$temp_dir/stderr"
[[ "$(cat "$temp_dir/runner/asc/AuthKey_ABCDE12345.p8")" == 'private key bytes' ]]
grep -Fxq "LOOMARR_ASC_KEY_PATH=$temp_dir/runner/asc/AuthKey_ABCDE12345.p8" "$temp_dir/github-env"
if [[ "$(stat -c '%a' "$temp_dir/runner/asc/AuthKey_ABCDE12345.p8" 2>/dev/null || stat -f '%Lp' "$temp_dir/runner/asc/AuthKey_ABCDE12345.p8")" != 600 ]]; then
  echo 'the API key file is readable by other users' >&2
  exit 1
fi
if grep -Fq "${prepare_env[0]#*=}" "$temp_dir/stdout" "$temp_dir/stderr"; then
  echo 'prepare-credentials printed the key' >&2
  exit 1
fi

expect_failure 'no mode' 'usage:' "$script"
expect_failure 'unknown mode' 'usage:' "$script" publish
expect_failure 'build without a version' 'usage:' "$script" build
expect_failure 'build with a malformed version' 'version must be x.y.z' "$script" build beta
expect_failure 'build with a four-part version' 'version must be x.y.z' "$script" build 0.2.0.1

for variable in LOOMARR_IOS_BUILD_NUMBER LOOMARR_APPLE_TEAM_ID LOOMARR_ASC_KEY_PATH \
  LOOMARR_ASC_KEY_ID LOOMARR_ASC_ISSUER_ID; do
  expect_failure "build without $variable" "$variable is required" "$variable=" "$script" build 0.2.0-beta.9
done
for variable in LOOMARR_ASC_KEY_PATH LOOMARR_ASC_KEY_ID LOOMARR_ASC_ISSUER_ID LOOMARR_ASC_APPLE_APP_ID; do
  expect_failure "validate without $variable" "$variable is required" "$variable=" "$script" validate
  expect_failure "upload without $variable" "$variable is required" "$variable=" "$script" upload
done

expect_failure 'zero build number' 'build number must be a positive integer' \
  LOOMARR_IOS_BUILD_NUMBER=0 "$script" build 0.2.0
expect_failure 'padded build number' 'build number must be a positive integer' \
  LOOMARR_IOS_BUILD_NUMBER=007 "$script" build 0.2.0
expect_failure 'malformed team id' 'APPLE_TEAM_ID must be 10' \
  LOOMARR_APPLE_TEAM_ID=team "$script" build 0.2.0
expect_failure 'malformed key id' 'ASC_API_KEY_ID must be 10' \
  LOOMARR_ASC_KEY_ID=short "$script" validate
expect_failure 'missing key file' 'API key file does not exist' \
  LOOMARR_ASC_KEY_PATH="$key_dir/missing.p8" "$script" validate
printf 'x' > "$key_dir/wrongname.p8"
expect_failure 'key file not named for its id' 'must be named AuthKey_' \
  LOOMARR_ASC_KEY_PATH="$key_dir/wrongname.p8" "$script" validate
expect_failure 'non-numeric apple id' 'numeric Apple ID' \
  LOOMARR_ASC_APPLE_APP_ID=abc "$script" validate

if [[ "$(uname -s)" != Darwin ]]; then
  expect_failure 'build off macOS' 'requires macOS with Xcode' "$script" build 0.2.0-beta.9
fi

# Submission modes need the verified IPA and its evidence from the build.
expect_failure 'validate before a build' 'run build first' "$script" validate
printf 'ipa bytes' > "$temp_dir/out/loomarr-ios-0.2.0-beta.9-42.ipa"
cat > "$temp_dir/out/loomarr-ios-0.2.0-beta.9-42.json" <<'JSON'
{ "bundleIdentifier": "media.loomarr.other", "version": "0.2.0", "buildNumber": "42" }
JSON
expect_failure 'evidence for another bundle id' 'unexpected bundle id' "$script" validate
write_evidence() {
  printf '{ "bundleIdentifier": "media.loomarr.mobile", "version": "0.2.0", "buildNumber": "42", "ipaSha256": "%s" }\n' \
    "$1" > "$temp_dir/out/loomarr-ios-0.2.0-beta.9-42.json"
}
write_evidence ''
expect_failure 'evidence without a hash' 'does not match the SHA-256' "$script" validate
write_evidence "$(printf 'other ipa bytes' | shasum -a 256 | awk '{ print $1 }')"
expect_failure 'IPA replaced after the build' 'does not match the SHA-256' "$script" upload
write_evidence "$(shasum -a 256 "$temp_dir/out/loomarr-ios-0.2.0-beta.9-42.ipa" | awk '{ print $1 }')"

# A fake xcrun records every altool call and prints whatever FAKE_ALTOOL_OUTPUT says.
cat > "$temp_dir/bin/xcrun" <<'FAKE'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${FAKE_XCRUN_CALLS:?}"
printf '%s\n' "${FAKE_ALTOOL_OUTPUT:-}"
exit "${FAKE_ALTOOL_STATUS:-0}"
FAKE
chmod +x "$temp_dir/bin/xcrun"
calls="$temp_dir/calls"
: > "$calls"
fake_env=(PATH="$temp_dir/bin:$PATH" FAKE_XCRUN_CALLS="$calls")

run "${fake_env[@]}" "$script" validate
grep -Fq -- '--validate-app -f' "$calls"
grep -Fq -- "--apple-id 1234567890" "$calls"
grep -Fq -- '--bundle-short-version-string 0.2.0' "$calls"
grep -Fq -- '--bundle-version 42' "$calls"
if grep -Fq 'ABCDE12345.p8' "$calls" "$temp_dir/stdout"; then
  echo 'altool arguments or output named the private key file' >&2
  exit 1
fi

: > "$calls"
run "${fake_env[@]}" "$script" upload
grep -Fq -- '--upload-package' "$calls"
if grep -Fq -- '--validate-app' "$calls"; then
  echo 'upload ran a validation as well' >&2
  exit 1
fi

# ITMS-90111 is named and never retried, even when altool also exits non-zero.
: > "$calls"
expect_failure 'ITMS-90111' 'ITMS-90111, Apple rejected the build toolchain' \
  "${fake_env[@]}" FAKE_ALTOOL_STATUS=1 \
  'FAKE_ALTOOL_OUTPUT=ERROR: ITMS-90111: Unsupported SDK or Xcode version' "$script" upload
[[ "$(wc -l < "$calls")" -eq 1 ]]

# A plain altool failure also fails the step, and ITMS-90111 detection ignores a clean run.
: > "$calls"
expect_failure 'altool failure' 'altool --upload-package failed' \
  "${fake_env[@]}" FAKE_ALTOOL_STATUS=3 FAKE_ALTOOL_OUTPUT=boom "$script" upload
[[ "$(wc -l < "$calls")" -eq 1 ]]

echo 'build-ios-testflight tests passed'
