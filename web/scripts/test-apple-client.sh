#!/usr/bin/env bash
# Generate, compile, install, and launch one prototype Apple client in a simulator.
set -euo pipefail

readonly APP_NAME="${1:-mobile}"
WEB_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly WEB_ROOT
readonly APP_DIR="${WEB_ROOT}/apps/${APP_NAME}"
readonly ARTIFACTS_DIR="${LOOMARR_APPLE_ARTIFACTS_DIR:-${WEB_ROOT}/../.artifacts/apple-client/${APP_NAME}}"
readonly DERIVED_DATA="${ARTIFACTS_DIR}/derived-data"

case "${APP_NAME}" in
  mobile)
    readonly SCHEME="LoomarrMobilePrototype"
    readonly SDK="iphonesimulator"
    readonly DESTINATION="generic/platform=iOS Simulator"
    readonly RUNTIME_TOKEN="iOS"
    readonly BUNDLE_ID="media.loomarr.mobile.prototype"
    readonly BUILD_PRODUCT="Release-iphonesimulator/${SCHEME}.app"
    ;;
  tv)
    readonly SCHEME="LoomarrTVPrototype"
    readonly SDK="appletvsimulator"
    readonly DESTINATION="generic/platform=tvOS Simulator"
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

NODE_ENV=production RCT_NO_LAUNCH_PACKAGER=1 xcodebuild \
  -workspace "${APP_DIR}/ios/${SCHEME}.xcworkspace" \
  -scheme "${SCHEME}" \
  -configuration Release \
  -sdk "${SDK}" \
  -destination "${DESTINATION}" \
  -derivedDataPath "${DERIVED_DATA}" \
  CODE_SIGNING_ALLOWED=NO \
  CODE_SIGNING_REQUIRED=NO \
  build

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
readonly APP_PATH="${DERIVED_DATA}/Build/Products/${BUILD_PRODUCT}"
if [[ ! -d "${APP_PATH}" ]]; then
  printf 'expected simulator application was not built at %s\n' "${APP_PATH}" >&2
  exit 1
fi

xcrun simctl install "${simulator_id}" "${APP_PATH}"
xcrun simctl launch --terminate-running-process "${simulator_id}" "${BUNDLE_ID}"
sleep 5
xcrun simctl io "${simulator_id}" screenshot "${ARTIFACTS_DIR}/${APP_NAME}.png"
xcrun simctl spawn "${simulator_id}" launchctl list | grep -F "${BUNDLE_ID}" >/dev/null
printf 'apple-client: %s built, installed, launched, and remained alive on %s\n' \
  "${APP_NAME}" "${simulator_id}"
