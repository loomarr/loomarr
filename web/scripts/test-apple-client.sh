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
readonly APPLE_COMPILATION_CACHE_XCCONFIG="${WEB_ROOT}/scripts/apple-compilation-cache.xcconfig"
readonly APPLE_COMPILATION_CACHE_CLI="${WEB_ROOT}/../scripts/apple-compilation-cache.sh"
readonly APPLE_CACHE_MODE="${LOOMARR_APPLE_CACHE_MODE:-cold}"
readonly APPLE_CACHE_STORE="${LOOMARR_APPLE_CACHE_STORE:-}"
readonly APPLE_CACHE_REQUIRE_WARM="${LOOMARR_APPLE_CACHE_REQUIRE_WARM:-0}"
readonly APPLE_CACHE_DIAGNOSTIC_REQUIREMENT="${LOOMARR_APPLE_CACHE_DIAGNOSTIC_REQUIREMENT:-report}"
readonly APPLE_CACHE_POPULATE="${LOOMARR_APPLE_CACHE_POPULATE:-0}"
readonly APPLE_CACHE_SOURCE_PROBE="${LOOMARR_APPLE_CACHE_SOURCE_PROBE:-}"
readonly APPLE_CACHE_SOURCE_PROBE_FILE="${LOOMARR_APPLE_CACHE_SOURCE_PROBE_FILE:-}"

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
case "${APPLE_CACHE_MODE}" in
  warm|cold) ;;
  *)
    printf 'LOOMARR_APPLE_CACHE_MODE must be warm or cold; found %s\n' "$APPLE_CACHE_MODE" >&2
    exit 2
    ;;
esac
if [[ "$APPLE_CACHE_REQUIRE_WARM" != 0 && "$APPLE_CACHE_REQUIRE_WARM" != 1 ]]; then
  printf 'LOOMARR_APPLE_CACHE_REQUIRE_WARM must be 0 or 1\n' >&2
  exit 2
fi
if [[ "$APPLE_CACHE_POPULATE" != 0 && "$APPLE_CACHE_POPULATE" != 1 ]]; then
  printf 'LOOMARR_APPLE_CACHE_POPULATE must be 0 or 1\n' >&2
  exit 2
fi
if [[ "$APPLE_CACHE_POPULATE" == 1 && "$APPLE_CACHE_MODE" != warm ]]; then
  printf 'LOOMARR_APPLE_CACHE_POPULATE requires warm mode\n' >&2
  exit 2
fi
case "$APPLE_CACHE_DIAGNOSTIC_REQUIREMENT" in
  report|hits|hits-and-misses) ;;
  *)
    printf 'LOOMARR_APPLE_CACHE_DIAGNOSTIC_REQUIREMENT must be report, hits, or hits-and-misses\n' >&2
    exit 2
    ;;
esac
if [[ -n "$APPLE_CACHE_SOURCE_PROBE" \
  && ! "$APPLE_CACHE_SOURCE_PROBE" =~ ^[A-Za-z0-9._-]+$ ]]; then
  printf 'LOOMARR_APPLE_CACHE_SOURCE_PROBE contains unsafe characters\n' >&2
  exit 2
fi

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
REAL_XCODEBUILD="$(command -v xcodebuild)"
readonly REAL_XCODEBUILD
xcodebuild -version
xcrun swift --version
xcode_version="$(xcodebuild -version | awk 'NR == 1 { print $2 }')"
if [[ ! "${xcode_version}" =~ ^27\. ]]; then
  printf 'Apple client verification requires Xcode 27.x; found %s\n' "${xcode_version}" >&2
  exit 2
fi

EXPO_PACKAGE_JSON="$(
  cd "${APP_DIR}"
  node -p "require.resolve('expo/package.json')"
)"
readonly EXPO_PACKAGE_JSON
readonly EXPO_TEMPLATE="${EXPO_PACKAGE_JSON%/package.json}/template.tgz"
if [[ ! -f "${EXPO_TEMPLATE}" ]]; then
  printf 'the pinned Expo package does not contain its native template: %s\n' "${EXPO_TEMPLATE}" >&2
  exit 2
fi

mkdir -p "${ARTIFACTS_DIR}" "${BUILD_DIR}"
readonly XCODE_CAPTURE_DIR="${ARTIFACTS_DIR}/xcodebuild-capture"
mkdir -p "$XCODE_CAPTURE_DIR"
cat > "$XCODE_CAPTURE_DIR/xcodebuild" <<'XCODEBUILD_CAPTURE'
#!/usr/bin/env bash
set -euo pipefail
: "${LOOMARR_APPLE_REAL_XCODEBUILD:?}"
: "${LOOMARR_APPLE_RAW_XCODE_LOG:?}"

args=("$@")
capture_result=false
for arg in "${args[@]}"; do
  case "$arg" in
    -workspace|-project) capture_result=true ;;
    -resultBundlePath) capture_result=false; break ;;
  esac
done
if [[ "$capture_result" == true && -n "${LOOMARR_APPLE_RESULT_BUNDLE_PATH:-}" ]]; then
  args+=(-resultBundlePath "$LOOMARR_APPLE_RESULT_BUNDLE_PATH")
fi

set +e
"$LOOMARR_APPLE_REAL_XCODEBUILD" "${args[@]}" 2>&1 | tee -a "$LOOMARR_APPLE_RAW_XCODE_LOG"
build_status="${PIPESTATUS[0]}"
set -e
if [[ "$capture_result" == true && -d "${LOOMARR_APPLE_RESULT_BUNDLE_PATH:-}" ]]; then
  xcrun xcresulttool get log \
    --path "$LOOMARR_APPLE_RESULT_BUNDLE_PATH" \
    --type build > "$LOOMARR_APPLE_RESULT_BUNDLE_PATH.json" 2>> "$LOOMARR_APPLE_RAW_XCODE_LOG" || \
    printf 'apple-client: could not extract xcresult build log\n' >> "$LOOMARR_APPLE_RAW_XCODE_LOG"
fi
exit "$build_status"
XCODEBUILD_CAPTURE
chmod +x "$XCODE_CAPTURE_DIR/xcodebuild"

if [[ "${APP_NAME}" == "tv" ]]; then
  (
    cd "${WEB_ROOT}"
    EXPO_TV=1 pnpm --filter @loomarr/tv exec expo prebuild --platform ios --clean --no-install \
      --template "${EXPO_TEMPLATE}"
  )
else
  (
    cd "${WEB_ROOT}"
    pnpm --filter @loomarr/mobile exec expo prebuild --platform ios --clean --no-install \
      --template "${EXPO_TEMPLATE}"
  )
fi

if [[ -n "$APPLE_CACHE_SOURCE_PROBE" ]]; then
  probe_file="${APPLE_CACHE_SOURCE_PROBE_FILE:-${APP_DIR}/ios/${SCHEME}/AppDelegate.swift}"
  if [[ ! -f "$probe_file" ]]; then
    printf 'apple-client: source invalidation probe file is missing: %s\n' "$probe_file" >&2
    exit 1
  fi
  printf '\n// Loomarr cache invalidation probe: %s\n' "$APPLE_CACHE_SOURCE_PROBE" >> "$probe_file"
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
run_expo_build() {
  local mode="$1" xcconfig logfile raw_log
  local -a command=("${expo_run[@]}")
  case "$mode" in
    warm)
      xcconfig="$APPLE_COMPILATION_CACHE_XCCONFIG"
      ;;
    cold)
      xcconfig="$APPLE_SIMULATOR_XCCONFIG"
      command+=(--no-build-cache)
      ;;
  esac
  logfile="$ARTIFACTS_DIR/build-$mode.log"
  raw_log="$ARTIFACTS_DIR/xcodebuild-$mode.log"
  : > "$raw_log"
  if [[ "${APP_NAME}" == "tv" ]]; then
    (
      cd "${APP_DIR}"
      XCODE_XCCONFIG_FILE="$xcconfig" \
        PATH="$XCODE_CAPTURE_DIR:$PATH" \
        LOOMARR_APPLE_REAL_XCODEBUILD="$REAL_XCODEBUILD" \
        LOOMARR_APPLE_RAW_XCODE_LOG="$raw_log" \
        LOOMARR_APPLE_RESULT_BUNDLE_PATH="$ARTIFACTS_DIR/build-$mode.xcresult" \
        LOOMARR_APPLE_CACHE_STORE="$APPLE_CACHE_STORE" \
        NODE_ENV=production RCT_NO_LAUNCH_PACKAGER=1 EXPO_TV=1 \
        REACT_NATIVE_NODE_MODULES_DIR="${REACT_NATIVE_MODULES_DIR}" "${command[@]}"
    ) 2>&1 | tee "$logfile" | filter_react_native_pods_notice
  else
    (
      cd "${APP_DIR}"
      XCODE_XCCONFIG_FILE="$xcconfig" \
        PATH="$XCODE_CAPTURE_DIR:$PATH" \
        LOOMARR_APPLE_REAL_XCODEBUILD="$REAL_XCODEBUILD" \
        LOOMARR_APPLE_RAW_XCODE_LOG="$raw_log" \
        LOOMARR_APPLE_RESULT_BUNDLE_PATH="$ARTIFACTS_DIR/build-$mode.xcresult" \
        LOOMARR_APPLE_CACHE_STORE="$APPLE_CACHE_STORE" \
        NODE_ENV=production RCT_NO_LAUNCH_PACKAGER=1 \
        REACT_NATIVE_NODE_MODULES_DIR="${REACT_NATIVE_MODULES_DIR}" "${command[@]}"
    ) 2>&1 | tee "$logfile" | filter_react_native_pods_notice
  fi
}

quarantine_cache() {
  local quarantine_dir="$ARTIFACTS_DIR/quarantine"
  if [[ -z "$APPLE_CACHE_STORE" || ! -e "$APPLE_CACHE_STORE" ]]; then
    return
  fi
  mkdir -p "$quarantine_dir"
  if [[ -e "$quarantine_dir/store" ]]; then
    printf 'apple-client: quarantine destination already exists: %s\n' "$quarantine_dir/store" >&2
    exit 1
  fi
  mv "$APPLE_CACHE_STORE" "$quarantine_dir/store"
}

warm_fallback=false
warm_succeeded=false
build_mode="$APPLE_CACHE_MODE"
if [[ "$build_mode" == warm ]]; then
  if [[ -z "$APPLE_CACHE_STORE" ]]; then
    printf 'apple-client: warm mode requires LOOMARR_APPLE_CACHE_STORE\n' >&2
    exit 2
  fi
  if [[ "$APPLE_CACHE_POPULATE" == 1 ]] \
    && { [[ ! -d "$APPLE_CACHE_STORE" ]] \
      || [[ -z "$(find "$APPLE_CACHE_STORE" -mindepth 1 -print -quit)" ]]; }; then
    mkdir -p "$APPLE_CACHE_STORE"
  elif ! "$APPLE_COMPILATION_CACHE_CLI" validate-store "$APPLE_CACHE_STORE" >/dev/null; then
    quarantine_cache
    printf 'apple-client: warm cache unavailable or invalid; retrying cold\n'
    warm_fallback=true
    build_mode=cold
  fi
fi

if [[ "$build_mode" == warm ]]; then
  if ! run_expo_build warm; then
    quarantine_cache
    printf 'apple-client: warm build failed; quarantined compilation cache and retrying cold\n'
    warm_fallback=true
    run_expo_build cold
  else
    warm_succeeded=true
  fi
else
  run_expo_build cold
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
readonly SCREENSHOT_PATH="${ARTIFACTS_DIR}/${APP_NAME}.png"
capture_screenshot() {
  local attempt
  for attempt in 1 2 3; do
    if xcrun simctl io "${simulator_id}" screenshot "$SCREENSHOT_PATH"; then
      return
    fi
    if (( attempt < 3 )); then
      printf 'apple-client: screenshot attempt %d failed; retrying\n' "$attempt" >&2
      sleep 2
    fi
  done
  printf 'apple-client: could not capture %s simulator screenshot after 3 attempts\n' \
    "$APP_NAME" >&2
  return 1
}
capture_screenshot
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
if [[ "$warm_succeeded" == true ]]; then
  if [[ "$APPLE_CACHE_POPULATE" == 1 ]]; then
    "$APPLE_COMPILATION_CACHE_CLI" validate-store "$APPLE_CACHE_STORE" >/dev/null
    printf 'apple-client: populated and validated compilation cache\n'
  fi
  "$APPLE_COMPILATION_CACHE_CLI" diagnostics \
    "$ARTIFACTS_DIR/build-warm.xcresult.json" \
    "$APPLE_CACHE_DIAGNOSTIC_REQUIREMENT" \
    | tee "$ARTIFACTS_DIR/cache-diagnostics.env"
fi
if [[ "$warm_fallback" == true ]] \
  && { [[ "$APPLE_CACHE_REQUIRE_WARM" == 1 ]] || [[ "$APPLE_CACHE_POPULATE" == 1 ]]; }; then
  printf 'apple-client: cold fallback passed, but this caller requires a warm build\n' >&2
  exit 1
fi
