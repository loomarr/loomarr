#!/usr/bin/env bash
# Generate, compile, install, and launch one prototype Apple client in a simulator.
set -euo pipefail

readonly APP_NAME="${1:-mobile}"
WEB_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly WEB_ROOT
readonly APP_DIR="${WEB_ROOT}/apps/${APP_NAME}"
readonly ARTIFACTS_DIR="${LOOMARR_APPLE_ARTIFACTS_DIR:-${WEB_ROOT}/../.artifacts/apple-client/${APP_NAME}}"
readonly BUILD_DIR="${LOOMARR_APPLE_BUILD_DIR:-${ARTIFACTS_DIR}/build}"
readonly APPLE_SIMULATOR_XCCONFIG="${WEB_ROOT}/scripts/apple-simulator.xcconfig"

filter_react_native_pods_notice() {
  awk -f "${WEB_ROOT}/scripts/filter-react-native-pods-notice.awk"
}

case "${APP_NAME}" in
  mobile)
    readonly SCHEME="LoomarrMobilePrototype"
    readonly RUNTIME_TOKEN="iOS"
    readonly BUNDLE_ID="media.loomarr.mobile.prototype"
    ;;
  tv)
    readonly SCHEME="LoomarrTVPrototype"
    readonly RUNTIME_TOKEN="tvOS"
    readonly BUNDLE_ID="media.loomarr.tv.prototype"
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
for command_name in jq xcodebuild xcrun; do
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

mkdir -p "${ARTIFACTS_DIR}" "${BUILD_DIR}"

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

expo_run=(
  pnpm exec expo run:ios
  --scheme "${SCHEME}"
  --configuration Release
  --device "${simulator_id}"
  --no-bundler
  --output "${BUILD_DIR}"
)
# react-native-svg resolves React Native from CocoaPods' installation root. The workspace uses
# the react-native-tvos npm alias, so Node follows the app's `react-native` symlink to a physical
# `react-native-tvos` directory and the podspec cannot infer the alias on its own. Give native
# podspecs the app-local node_modules directory, where the canonical symlink is present.
readonly REACT_NATIVE_MODULES_DIR="${APP_DIR}/node_modules"
if [[ "${APP_NAME}" == "tv" ]]; then
  (
    cd "${APP_DIR}"
    XCODE_XCCONFIG_FILE="${APPLE_SIMULATOR_XCCONFIG}" \
      NODE_ENV=production RCT_NO_LAUNCH_PACKAGER=1 EXPO_TV=1 \
      REACT_NATIVE_NODE_MODULES_DIR="${REACT_NATIVE_MODULES_DIR}" "${expo_run[@]}"
  ) 2>&1 | filter_react_native_pods_notice
else
  (
    cd "${APP_DIR}"
    XCODE_XCCONFIG_FILE="${APPLE_SIMULATOR_XCCONFIG}" \
      NODE_ENV=production RCT_NO_LAUNCH_PACKAGER=1 \
      REACT_NATIVE_NODE_MODULES_DIR="${REACT_NATIVE_MODULES_DIR}" "${expo_run[@]}"
  ) 2>&1 | filter_react_native_pods_notice
fi

# Release simulator builds default to every standard architecture. Hosted Apple jobs run on one
# architecture and launch on the same host, so compiling another slice only duplicates the native
# dependency graph. The xcconfig scopes the override to this simulator proof; fail closed if Expo or
# Xcode stops honoring it instead of silently returning to a universal binary.
readonly APP_BINARY="${BUILD_DIR}/${SCHEME}.app/${SCHEME}"
if [[ ! -f "${APP_BINARY}" ]]; then
  printf 'apple-client: built executable is missing: %s\n' "${APP_BINARY}" >&2
  exit 1
fi
HOST_ARCH="$(uname -m)"
readonly HOST_ARCH
APP_ARCHS="$(xcrun lipo -archs "${APP_BINARY}")"
readonly APP_ARCHS
if [[ "${APP_ARCHS}" != "${HOST_ARCH}" ]]; then
  printf 'apple-client: expected only host architecture %s; built %s\n' \
    "${HOST_ARCH}" "${APP_ARCHS}" >&2
  exit 1
fi
printf 'apple-client: %s simulator executable contains only %s\n' "${APP_NAME}" "${HOST_ARCH}"

# Expo owns dependency installation, CocoaPods, compilation, installation, and
# initial launch. Relaunch once to obtain the host PID used by the liveness check.
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
