#!/usr/bin/env bash
# Generate, compile, install, and launch one prototype Apple client in a simulator.
set -euo pipefail

readonly APP_NAME="${1:-mobile}"
WEB_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly WEB_ROOT
readonly APP_DIR="${WEB_ROOT}/apps/${APP_NAME}"
readonly ARTIFACTS_DIR="${LOOMARR_APPLE_ARTIFACTS_DIR:-${WEB_ROOT}/../.artifacts/apple-client/${APP_NAME}}"
readonly DERIVED_DATA="${LOOMARR_APPLE_DERIVED_DATA_DIR:-${ARTIFACTS_DIR}/derived-data}"

case "${APP_NAME}" in
  mobile)
    readonly SCHEME="LoomarrMobilePrototype"
    readonly SDK="iphonesimulator"
    readonly RUNTIME_TOKEN="iOS"
    readonly BUNDLE_ID="media.loomarr.mobile.prototype"
    readonly BUILD_PRODUCT="Release-iphonesimulator/${SCHEME}.app"
    ;;
  tv)
    readonly SCHEME="LoomarrTVPrototype"
    readonly SDK="appletvsimulator"
    readonly RUNTIME_TOKEN="tvOS"
    readonly BUNDLE_ID="media.loomarr.tv.prototype"
    readonly BUILD_PRODUCT="Release-appletvsimulator/${SCHEME}.app"
    ;;
  *)
    printf 'usage: %s [mobile|tv]\n' "$0" >&2
    exit 2
    ;;
esac

if [[ "$(uname -s)" != "Darwin" ]]; then
  printf 'Apple client verification requires macOS with Xcode\n' >&2
  exit 2
fi
for command_name in jq pod xcodebuild xcrun; do
  command -v "${command_name}" >/dev/null 2>&1 || {
    printf '%s is required for Apple client verification\n' "${command_name}" >&2
    exit 2
  }
done
xcodebuild -version
xcrun swift --version
xcode_version="$(xcodebuild -version | awk 'NR == 1 { print $2 }')"
if [[ ! "${xcode_version}" =~ ^26\. ]]; then
  printf 'Apple client verification requires Xcode 26.x; found %s\n' "${xcode_version}" >&2
  exit 2
fi

mkdir -p "${ARTIFACTS_DIR}"
rm -rf "${DERIVED_DATA}"

if [[ "${APP_NAME}" == "tv" ]]; then
  (
    cd "${WEB_ROOT}"
    EXPO_TV=1 pnpm --filter @loomarr/tv exec expo prebuild --platform ios --clean --no-install
  )
else
  (
    cd "${WEB_ROOT}"
    pnpm --filter @loomarr/mobile exec expo prebuild --platform ios --clean --no-install
  )
fi

(
  cd "${APP_DIR}/ios"
  pod install
)

simulator_json="$(xcrun simctl list devices available --json)"
simulator_id="$(jq -r --arg runtime "${RUNTIME_TOKEN}" '
  [.devices | to_entries[] | select(.key | contains($runtime)) | .value[] | select(.isAvailable)]
  | last | .udid // empty
' <<<"${simulator_json}")"
if [[ -z "${simulator_id}" ]]; then
  printf 'no available %s simulator is installed\n' "${RUNTIME_TOKEN}" >&2
  exit 1
fi

simulator_state="$(jq -r --arg id "${simulator_id}" '
  [.devices[][] | select(.udid == $id)] | first | .state // empty
' <<<"${simulator_json}")"
booted_here=false
if [[ "${simulator_state}" != "Booted" ]]; then
  xcrun simctl boot "${simulator_id}"
  booted_here=true
fi
cleanup() {
  if [[ "${booted_here}" == "true" ]]; then
    xcrun simctl shutdown "${simulator_id}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT
xcrun simctl bootstatus "${simulator_id}" -b

NODE_ENV=production RCT_NO_LAUNCH_PACKAGER=1 xcodebuild \
  -workspace "${APP_DIR}/ios/${SCHEME}.xcworkspace" \
  -scheme "${SCHEME}" \
  -configuration Release \
  -sdk "${SDK}" \
  -destination "id=${simulator_id}" \
  -derivedDataPath "${DERIVED_DATA}" \
  ONLY_ACTIVE_ARCH=YES \
  CODE_SIGNING_ALLOWED=NO \
  CODE_SIGNING_REQUIRED=NO \
  build
readonly APP_PATH="${DERIVED_DATA}/Build/Products/${BUILD_PRODUCT}"
if [[ ! -d "${APP_PATH}" ]]; then
  printf 'expected simulator application was not built at %s\n' "${APP_PATH}" >&2
  exit 1
fi

xcrun simctl install "${simulator_id}" "${APP_PATH}"
launch_output="$(xcrun simctl launch --terminate-running-process "${simulator_id}" "${BUNDLE_ID}")"
printf '%s\n' "${launch_output}"
launch_pid="${launch_output##*: }"
if [[ ! "${launch_pid}" =~ ^[0-9]+$ ]]; then
  printf 'could not parse launched process id from: %s\n' "${launch_output}" >&2
  exit 1
fi
sleep 5
xcrun simctl io "${simulator_id}" screenshot "${ARTIFACTS_DIR}/${APP_NAME}.png"
# `simctl launch` returns the simulator application's host PID. Check it from the
# host: simulator runtime command-line binaries are not guaranteed to be runnable
# through `simctl spawn` (tvOS 26.4's /bin/kill is a macOS binary, for example).
if ! /bin/kill -0 "${launch_pid}"; then
  printf 'apple-client: %s exited after launch; recent simulator log follows\n' "${APP_NAME}" >&2
  xcrun simctl spawn "${simulator_id}" log show \
    --last 2m \
    --style compact \
    --predicate "process == '${SCHEME}' OR eventMessage CONTAINS[c] '${BUNDLE_ID}'" \
    | tee "${ARTIFACTS_DIR}/${APP_NAME}.log" >&2
  exit 1
fi
printf 'apple-client: %s built, installed, launched, and remained alive on %s\n' \
  "${APP_NAME}" "${simulator_id}"
