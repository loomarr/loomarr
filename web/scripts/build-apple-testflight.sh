#!/usr/bin/env bash
# Archive, export, verify, validate, and upload a permanent-identity Expo Apple app to TestFlight:
# the iPhone app (ADR 0042) or the Apple TV app, each its own App Store Connect record.
#
#   build-apple-testflight.sh plan <event> <payload>            decide apps, upload, and version for a run
#   build-apple-testflight.sh <app> prepare-credentials         fail on missing secrets, write the API key
#   build-apple-testflight.sh <app> build <x.y.z[-beta.N]>      archive + export + verify the IPA
#   build-apple-testflight.sh <app> validate                    altool --validate-app on the verified IPA
#   build-apple-testflight.sh <app> upload                      altool --upload-package, never retried
#
# <app> is iphone or apple-tv. Every credential arrives through the environment; nothing here prints
# one.
set -euo pipefail

WEB_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly WEB_ROOT
REPO_ROOT="$(cd "${WEB_ROOT}/.." && pwd)"
readonly REPO_ROOT
# Every app the workflow can release, in the order the plan lists them.
readonly APPS=(iphone apple-tv)

usage() {
  printf 'usage: %s plan <event> <payload> | <iphone|apple-tv> prepare-credentials | build <x.y.z[-beta.N]> | validate | upload\n' "$0" >&2
  exit 2
}

fail() {
  printf 'apple-testflight: %s\n' "$*" >&2
  exit "${FAIL_STATUS:-1}"
}

# The one table of what differs between the apps. Expo names the Xcode project after the sanitised
# app name: "Loomarr" -> Loomarr, "Loomarr TV" -> LoomarrTV.
select_app() {
  APP="$1"
  case "$APP" in
    iphone)
      APP_LABEL="iPhone"
      APP_PACKAGE="@loomarr/mobile"
      APP_DIR="${WEB_ROOT}/apps/mobile"
      BUNDLE_ID="media.loomarr.mobile"
      SCHEME="Loomarr"
      DESTINATION="generic/platform=iOS"
      DEVICE_PLATFORM="iphoneos"
      ALTOOL_PLATFORM="ios"
      APP_ID_VARIABLE="ASC_APPLE_APP_ID"
      ;;
    apple-tv)
      APP_LABEL="Apple TV"
      APP_PACKAGE="@loomarr/tv"
      APP_DIR="${WEB_ROOT}/apps/tv"
      BUNDLE_ID="media.loomarr.tv"
      SCHEME="LoomarrTV"
      DESTINATION="generic/platform=tvOS"
      DEVICE_PLATFORM="appletvos"
      ALTOOL_PLATFORM="appletvos"
      APP_ID_VARIABLE="ASC_APPLE_TV_APP_ID"
      ;;
    *) usage ;;
  esac
  OUTPUT_DIR="${LOOMARR_APPLE_OUTPUT_DIR:-${REPO_ROOT}/.artifacts/testflight-${APP}}"
  readonly APP APP_LABEL APP_PACKAGE APP_DIR BUNDLE_ID SCHEME DESTINATION DEVICE_PLATFORM \
    ALTOOL_PLATFORM APP_ID_VARIABLE OUTPUT_DIR
}

require_release_version() {
  if [[ ! "$1" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
    FAIL_STATUS=2 fail "version must be x.y.z with an optional suffix; got \"$1\""
  fi
}

require_env() {
  local name
  for name in "$@"; do
    if [[ -z "${!name:-}" ]]; then
      FAIL_STATUS=2 fail "${name} is required but empty or unset"
    fi
  done
}

require_commands() {
  local command_name
  for command_name in "$@"; do
    command -v "$command_name" >/dev/null 2>&1 || FAIL_STATUS=2 fail "${command_name} is required"
  done
}

# The key, issuer, and team are needed to sign (build) and to talk to App Store Connect.
require_credentials() {
  require_env LOOMARR_ASC_KEY_PATH LOOMARR_ASC_KEY_ID LOOMARR_ASC_ISSUER_ID
  if [[ ! "$LOOMARR_ASC_KEY_ID" =~ ^[A-Z0-9]{10}$ ]]; then
    FAIL_STATUS=2 fail 'ASC_API_KEY_ID must be 10 uppercase letters or digits'
  fi
  if [[ ! -f "$LOOMARR_ASC_KEY_PATH" ]]; then
    FAIL_STATUS=2 fail 'the API key file does not exist'
  fi
  # altool finds the private key only as AuthKey_<id>.p8 inside API_PRIVATE_KEYS_DIR.
  if [[ "$(basename "$LOOMARR_ASC_KEY_PATH")" != "AuthKey_${LOOMARR_ASC_KEY_ID}.p8" ]]; then
    FAIL_STATUS=2 fail 'the API key file must be named AuthKey_<ASC_API_KEY_ID>.p8'
  fi
}

# Apple accepts a build only from the GM toolchain. Name that failure instead of retrying it.
run_altool() {
  local log status=0
  log="$(mktemp)"
  xcrun altool "$@" >"$log" 2>&1 || status=$?
  cat "$log"
  if grep -q 'ITMS-90111' "$log"; then
    rm -f -- "$log"
    printf 'apple-testflight: ITMS-90111, Apple rejected the build toolchain (Xcode or macOS is not a GM release).\n' >&2
    fail 'not retrying; see the Xcode and macOS versions logged by the build step.'
  fi
  rm -f -- "$log"
  if ((status != 0)); then
    fail "altool $1 failed with exit status ${status}"
  fi
}

# Reads the verified IPA path and its Apple metadata back from the evidence the build wrote.
load_evidence() {
  local evidence
  evidence="$(find "$OUTPUT_DIR" -maxdepth 1 -type f -name "loomarr-${APP}-*.json" -print -quit 2>/dev/null || true)"
  if [[ -z "$evidence" ]]; then
    fail "no verification evidence in ${OUTPUT_DIR}; run build first"
  fi
  IPA="${evidence%.json}.ipa"
  if [[ ! -f "$IPA" ]]; then
    fail "the verified IPA is missing: ${IPA}"
  fi
  SHORT_VERSION="$(node -p "require(process.argv[1]).version" "$evidence")"
  BUILD_NUMBER="$(node -p "require(process.argv[1]).buildNumber" "$evidence")"
  EVIDENCE_BUNDLE_ID="$(node -p "require(process.argv[1]).bundleIdentifier" "$evidence")"
  if [[ "$EVIDENCE_BUNDLE_ID" != "$BUNDLE_ID" ]]; then
    fail "evidence names unexpected bundle id ${EVIDENCE_BUNDLE_ID}"
  fi
  # Submit only the bytes the build verified, not whatever now has the IPA's name.
  local expected_sha256 actual_sha256
  expected_sha256="$(node -p "require(process.argv[1]).ipaSha256" "$evidence")"
  actual_sha256="$(shasum -a 256 "$IPA" | awk '{ print $1 }')"
  if [[ ! "$expected_sha256" =~ ^[0-9a-f]{64}$ || "$actual_sha256" != "$expected_sha256" ]]; then
    fail "${IPA} does not match the SHA-256 the build verified"
  fi
}

submit() {
  local verb="$1" app_id_name="LOOMARR_${APP_ID_VARIABLE}"
  require_credentials
  require_env "$app_id_name"
  local app_id="${!app_id_name}"
  if [[ ! "$app_id" =~ ^[0-9]+$ ]]; then
    FAIL_STATUS=2 fail "${APP_ID_VARIABLE} must be the numeric Apple ID of the ${APP_LABEL} app record"
  fi
  require_commands node shasum
  load_evidence
  require_commands xcrun
  API_PRIVATE_KEYS_DIR="$(dirname "$LOOMARR_ASC_KEY_PATH")"
  export API_PRIVATE_KEYS_DIR
  # --validate-app names its file with -f; --upload-package takes the file directly.
  local -a file_args=("$IPA")
  if [[ "$verb" == --validate-app ]]; then
    file_args=(-f "$IPA")
  fi
  run_altool "$verb" "${file_args[@]}" \
    --platform "$ALTOOL_PLATFORM" --apple-id "$app_id" \
    --bundle-id "$BUNDLE_ID" --bundle-version "$BUILD_NUMBER" \
    --bundle-short-version-string "$SHORT_VERSION" \
    --api-key "$LOOMARR_ASC_KEY_ID" --api-issuer "$LOOMARR_ASC_ISSUER_ID" \
    --output-format json
  printf 'apple-testflight: %s %s succeeded for %s (%s)\n' "$APP_LABEL" "$verb" "$SHORT_VERSION" "$BUILD_NUMBER"
}

build() {
  local release_version="${1:-}"
  if [[ -z "$release_version" ]]; then
    usage
  fi
  require_release_version "$release_version"
  require_env LOOMARR_APPLE_BUILD_NUMBER LOOMARR_APPLE_TEAM_ID
  if [[ ! "$LOOMARR_APPLE_BUILD_NUMBER" =~ ^[1-9][0-9]{0,8}$ ]]; then
    FAIL_STATUS=2 fail "build number must be a positive integer; got \"${LOOMARR_APPLE_BUILD_NUMBER}\""
  fi
  if [[ ! "$LOOMARR_APPLE_TEAM_ID" =~ ^[A-Z0-9]{10}$ ]]; then
    FAIL_STATUS=2 fail 'APPLE_TEAM_ID must be 10 uppercase letters or digits'
  fi
  require_credentials
  if [[ "$(uname -s)" != "Darwin" ]]; then
    FAIL_STATUS=2 fail "the ${APP_LABEL} build requires macOS with Xcode"
  fi
  require_commands xcodebuild xcrun plutil lipo codesign unzip pod node pnpm shasum file

  # Evidence for triaging ITMS-90111 and for the first dispatch's open questions.
  xcodebuild -version
  sw_vers
  local xcode_version
  xcode_version="$(xcodebuild -version | awk 'NR == 1 { print $2 }')"
  if [[ ! "$xcode_version" =~ ^27\. ]]; then
    FAIL_STATUS=2 fail "requires Xcode 27.x; found ${xcode_version}"
  fi
  xcodebuild -help | grep -E 'manageAppVersionAndBuildNumber|testFlightInternalTestingOnly' || true

  # Like the simulator verifier, qualify only the reviewed package pins.
  local native_override
  for native_override in REACT_NATIVE_OVERRIDE_HERMES_DIR HERMES_ENGINE_TARBALL_PATH \
    HERMES_COMMIT RCT_BUILD_HERMES_FROM_SOURCE RCT_HERMES_V1_ENABLED ENTERPRISE_REPOSITORY \
    RCT_USE_RN_DEP RCT_USE_LOCAL_RN_DEP RCT_DEPS_VERSION \
    RCT_USE_PREBUILT_RNCORE RCT_TESTONLY_RNCORE_VERSION RCT_TESTONLY_RNCORE_TARBALL_PATH \
    REACT_NATIVE_OVERRIDE_NIGHTLY_BUILD_VERSION RNTV_TESTONLY_LOCAL_RNCORE_REPOSITORY; do
    if printenv "$native_override" >/dev/null; then
      FAIL_STATUS=2 fail "${native_override} overrides the qualified dependency selection"
    fi
  done

  local expo_package_json expo_template
  expo_package_json="$(cd "$APP_DIR" && node -p "require.resolve('expo/package.json')")"
  expo_template="${expo_package_json%/package.json}/template.tgz"
  if [[ ! -f "$expo_template" ]]; then
    FAIL_STATUS=2 fail "the pinned Expo package has no native template: ${expo_template}"
  fi

  # The TV app's config plugin, Podfile, and React Native build for tvOS only under EXPO_TV.
  if [[ "$APP" == apple-tv ]]; then
    export EXPO_TV=1
  fi

  # The run number is only a floor: App Store Connect refuses a build number at or below its latest,
  # which any upload from outside this workflow overtakes (#1900). validate and upload read the
  # chosen number back from the evidence, so it is fixed here, before the identity is generated.
  local floor_build="$LOOMARR_APPLE_BUILD_NUMBER"
  LOOMARR_APPLE_BUILD_NUMBER="$(node "$WEB_ROOT/scripts/asc-next-build-number.mjs" "$BUNDLE_ID" "$floor_build")"
  export LOOMARR_APPLE_BUILD_NUMBER
  printf 'apple-testflight: %s build number %s (floor %s)\n' "$APP_LABEL" "$LOOMARR_APPLE_BUILD_NUMBER" "$floor_build"

  # app.config.cjs validates the version and fixes the identity; read the result back.
  export LOOMARR_APPLE_RELEASE_CHANNEL=testflight
  export LOOMARR_APPLE_VERSION="$release_version"
  local config_json short_version config_build config_bundle
  config_json="$(cd "$APP_DIR" && pnpm exec expo config --json)"
  short_version="$(node -p "JSON.parse(process.argv[1]).version" "$config_json")"
  config_build="$(node -p "JSON.parse(process.argv[1]).ios.buildNumber" "$config_json")"
  config_bundle="$(node -p "JSON.parse(process.argv[1]).ios.bundleIdentifier" "$config_json")"
  if [[ "$config_bundle" != "$BUNDLE_ID" ]]; then
    fail "unexpected bundle identifier ${config_bundle}"
  fi
  if [[ "$config_build" != "$LOOMARR_APPLE_BUILD_NUMBER" ]]; then
    fail "build number was rewritten to ${config_build}"
  fi

  (
    cd "$WEB_ROOT"
    pnpm --filter "$APP_PACKAGE" exec expo prebuild --platform ios --clean --no-install \
      --template "$expo_template"
  )
  # react-native-svg resolves React Native through the app-local react-native-tvos alias link.
  (
    cd "${APP_DIR}/ios"
    NODE_ENV=production RCT_NO_LAUNCH_PACKAGER=1 \
      REACT_NATIVE_NODE_MODULES_DIR="${APP_DIR}/node_modules" pod install
  )
  "${WEB_ROOT}/scripts/check-ios-pods-binaries.sh" "${APP_DIR}/ios/Pods"
  local workspace="${APP_DIR}/ios/${SCHEME}.xcworkspace"
  if [[ ! -d "$workspace" ]]; then
    fail "prebuild did not produce ${workspace}"
  fi

  mkdir -p "$OUTPUT_DIR"
  local archive="${OUTPUT_DIR}/${SCHEME}.xcarchive" export_dir="${OUTPUT_DIR}/export"
  rm -rf -- "$archive" "$export_dir"
  # Xcode 16+ needs the key on every call, not only -allowProvisioningUpdates.
  local -a auth=(
    -allowProvisioningUpdates
    -authenticationKeyPath "$LOOMARR_ASC_KEY_PATH"
    -authenticationKeyID "$LOOMARR_ASC_KEY_ID"
    -authenticationKeyIssuerID "$LOOMARR_ASC_ISSUER_ID"
  )
  NODE_ENV=production RCT_NO_LAUNCH_PACKAGER=1 \
    REACT_NATIVE_NODE_MODULES_DIR="${APP_DIR}/node_modules" \
    xcodebuild archive \
    -workspace "$workspace" -scheme "$SCHEME" -configuration Release \
    -destination "$DESTINATION" -archivePath "$archive" \
    DEVELOPMENT_TEAM="$LOOMARR_APPLE_TEAM_ID" CODE_SIGN_STYLE=Automatic \
    "${auth[@]}"

  local export_options="${OUTPUT_DIR}/ExportOptions.plist"
  cat > "$export_options" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>method</key><string>app-store-connect</string>
  <key>destination</key><string>export</string>
  <key>teamID</key><string>${LOOMARR_APPLE_TEAM_ID}</string>
  <key>signingStyle</key><string>automatic</string>
  <key>uploadSymbols</key><true/>
  <key>manageAppVersionAndBuildNumber</key><false/>
</dict>
</plist>
PLIST
  xcodebuild -exportArchive \
    -archivePath "$archive" -exportPath "$export_dir" \
    -exportOptionsPlist "$export_options" \
    "${auth[@]}"

  local exported_ipa final_ipa
  exported_ipa="$(find "$export_dir" -maxdepth 1 -type f -name '*.ipa' -print -quit)"
  if [[ -z "$exported_ipa" ]]; then
    fail 'export produced no IPA'
  fi
  final_ipa="${OUTPUT_DIR}/loomarr-${APP}-${release_version#v}-${LOOMARR_APPLE_BUILD_NUMBER}.ipa"
  cp -- "$exported_ipa" "$final_ipa"
  verify_ipa "$final_ipa" "$short_version" "$xcode_version" "$release_version"
}

# Inspect the exact bytes that will be validated and uploaded, then write evidence beside them.
verify_ipa() {
  local ipa="$1" short_version="$2" xcode_version="$3" release_version="$4"
  local app info executable archs
  VERIFY_DIR="$(mktemp -d)"
  trap 'rm -rf -- "$VERIFY_DIR"' EXIT
  local verify_dir="$VERIFY_DIR"
  unzip -q "$ipa" -d "$verify_dir"
  app="$(find "${verify_dir}/Payload" -maxdepth 1 -name '*.app' -print -quit)"
  if [[ -z "$app" ]]; then
    fail 'the IPA has no application bundle'
  fi
  info="${app}/Info.plist"
  plist_value() { plutil -extract "$1" raw -o - "$info" 2>/dev/null || true; }
  expect() {
    local what="$1" actual="$2" expected="$3"
    if [[ "$actual" != "$expected" ]]; then
      fail "IPA ${what} is \"${actual}\"; expected \"${expected}\""
    fi
  }
  expect CFBundleIdentifier "$(plist_value CFBundleIdentifier)" "$BUNDLE_ID"
  expect CFBundleShortVersionString "$(plist_value CFBundleShortVersionString)" "$short_version"
  expect CFBundleVersion "$(plist_value CFBundleVersion)" "$LOOMARR_APPLE_BUILD_NUMBER"
  expect ITSAppUsesNonExemptEncryption "$(plist_value ITSAppUsesNonExemptEncryption)" false
  expect DTPlatformName "$(plist_value DTPlatformName)" "$DEVICE_PLATFORM"
  # Only the iPhone app browses for a server: the Apple TV app excludes the LAN discovery pod.
  if [[ "$APP" == iphone && -z "$(plist_value NSLocalNetworkUsageDescription)" ]]; then
    fail 'IPA has no NSLocalNetworkUsageDescription'
  fi
  # tvOS shows the layered home-screen icon and the Top Shelf image from the compiled brand assets
  # (config-tv's appleTVImages); App Store Connect refuses an Apple TV app without them.
  if [[ "$APP" == apple-tv ]]; then
    if [[ -z "$(plist_value CFBundleIcons.CFBundlePrimaryIcon)" ]]; then
      fail 'IPA has no tvOS app icon (CFBundleIcons.CFBundlePrimaryIcon)'
    fi
    expect TVTopShelfImage "$(plist_value TVTopShelfImage.TVTopShelfPrimaryImage)" "Top Shelf Image"
    expect "wide TVTopShelfImage" "$(plist_value TVTopShelfImage.TVTopShelfPrimaryImageWide)" "Top Shelf Image Wide"
  fi
  if [[ ! -f "${app}/PrivacyInfo.xcprivacy" ]]; then
    fail 'IPA has no PrivacyInfo.xcprivacy at the app bundle root'
  fi
  # The app-level manifest carries our UserDefaults/CA92.1 declaration plus whatever React Native's
  # pod aggregation added; the checker enforces no tracking and complete, unique entries.
  local privacy_categories
  privacy_categories="$(node "${WEB_ROOT}/scripts/check-ios-privacy-manifest.cjs" \
    "$(plutil -convert json -o - "${app}/PrivacyInfo.xcprivacy")")"
  printf 'apple-testflight: IPA privacy manifest accessed-API categories: %s\n' "$privacy_categories"
  # A device binary has arm64 only; any simulator slice or platform makes App Store Connect refuse
  # it. That holds for every Mach-O in the bundle: the executable, frameworks, dylibs, and plug-ins.
  executable="${app}/$(plist_value CFBundleExecutable)"
  archs="$(xcrun lipo -archs "$executable")"
  expect "executable architectures" "$archs" arm64
  local binary binary_archs
  while IFS= read -r -d '' binary; do
    [[ "$(file -b "$binary")" == Mach-O* ]] || continue
    binary_archs="$(xcrun lipo -archs "$binary")"
    expect "${binary#"${app}/"} architectures" "$binary_archs" arm64
    if [[ "$(xcrun vtool -show-build "$binary" 2>/dev/null || true)" == *SIMULATOR* ]]; then
      fail "IPA ${binary#"${app}/"} is built for a simulator platform"
    fi
  done < <(find "$app" -type f -print0)
  codesign --verify --deep --strict "$app"

  node -e '
    const fs = require("node:fs");
    const [evidence, bundleIdentifier, version, buildNumber, releaseVersion, xcode, sha256, archs, privacy] =
      process.argv.slice(1);
    fs.writeFileSync(
      evidence,
      JSON.stringify(
        {
          bundleIdentifier,
          version,
          buildNumber,
          releaseVersion,
          xcode,
          ipaSha256: sha256,
          architectures: archs,
          privacyAccessedAPITypes: JSON.parse(privacy),
        },
        null,
        2,
      ) + "\n",
    );
  ' "${ipa%.ipa}.json" "$BUNDLE_ID" "$short_version" "$LOOMARR_APPLE_BUILD_NUMBER" \
    "$release_version" "$xcode_version" "$(shasum -a 256 "$ipa" | awk '{ print $1 }')" "$archs" \
    "$privacy_categories"
  printf 'apple-testflight: verified %s\n' "$ipa"
}

# The CI impact gate (scripts/ci-impact.sh) that is the one definition of an app's inputs.
impact_gate() {
  case "$1" in
    iphone) printf 'apple_mobile' ;;
    apple-tv) printf 'apple_tv' ;;
  esac
}

# Prints what one workflow run releases as GitHub output records: apps= (a JSON list), upload=,
# version=. A dispatch is an explicit request for one app that carries its own version and upload
# choice. A push to main releases and uploads each app whose CI impact gate its diff selects, and
# its version is the newest release tag reachable from the pushed commit. As in CI, a push whose
# diff cannot be classified treats every path as changed. Run it from the checkout being released,
# with full history and tags.
plan() {
  local event="${1:-}" payload="${2:-}"
  if [[ -z "$event" || -z "$payload" ]]; then
    usage
  fi
  if [[ ! -f "$payload" ]]; then
    FAIL_STATUS=2 fail "the event payload does not exist: ${payload}"
  fi
  require_commands git jq
  local upload version='' base impact app gate selected
  local -a apps=()
  case "$event" in
    workflow_dispatch)
      app="$(jq -r '.inputs.app // ""' "$payload")"
      if [[ "$app" != iphone && "$app" != apple-tv ]]; then
        FAIL_STATUS=2 fail "the app input must be iphone or apple-tv; got \"${app}\""
      fi
      apps=("$app")
      version="$(jq -r '.inputs.version // ""' "$payload")"
      require_release_version "$version"
      upload="$(jq -r '.inputs.upload // "" | tostring' "$payload")"
      if [[ "$upload" != true && "$upload" != false ]]; then
        FAIL_STATUS=2 fail "the upload input must be true or false; got \"${upload}\""
      fi
      ;;
    push)
      base="$("${REPO_ROOT}/scripts/ci-diff-base.sh" push "$payload")"
      if [[ -z "$base" ]] || ! git cat-file -e "${base}^{commit}" 2>/dev/null; then
        printf 'apple-testflight: no usable diff base; treating every path as changed\n' >&2
        impact="$("${REPO_ROOT}/scripts/ci-impact.sh" --all)"
      elif ! impact="$(git diff --name-only "$base" HEAD | "${REPO_ROOT}/scripts/ci-impact.sh")"; then
        printf 'apple-testflight: the CI impact classifier failed; treating every path as changed\n' >&2
        impact="$("${REPO_ROOT}/scripts/ci-impact.sh" --all)"
      fi
      for app in "${APPS[@]}"; do
        gate="$(impact_gate "$app")"
        selected="$(sed -n "s/^${gate}=//p" <<<"$impact")"
        if [[ "$selected" != true && "$selected" != false ]]; then
          fail "the CI impact classifier did not decide the ${gate} gate"
        fi
        if [[ "$selected" == true ]]; then
          apps+=("$app")
        fi
      done
      upload=false
      if ((${#apps[@]} > 0)); then
        upload=true
        if ! version="$(git describe --tags --abbrev=0 --match 'v[0-9]*' HEAD 2>/dev/null)"; then
          fail 'no v* release tag is reachable from HEAD'
        fi
        require_release_version "$version"
        printf 'apple-testflight: the push changes %s; releasing %s\n' "${apps[*]}" "${version#v}" >&2
      else
        printf 'apple-testflight: the push changes no Apple app input; nothing to release\n' >&2
      fi
      ;;
    *)
      FAIL_STATUS=2 fail "plan supports push and workflow_dispatch, not \"${event}\""
      ;;
  esac
  printf 'apps=%s\nupload=%s\nversion=%s\n' \
    "$(jq -cn '$ARGS.positional' --args -- ${apps[@]+"${apps[@]}"})" "$upload" "${version#v}"
}

# Fails with one readable list of everything the release environment lacks for this app, then writes
# the API key where altool and xcodebuild expect it. The key is decoded straight to disk and never
# printed.
prepare_credentials() {
  local -a missing=()
  local name
  for name in ASC_API_KEY_P8_BASE64 ASC_API_KEY_ID ASC_API_ISSUER_ID; do
    [[ -n "${!name:-}" ]] || missing+=("secret ${name}")
  done
  for name in APPLE_TEAM_ID "$APP_ID_VARIABLE"; do
    [[ -n "${!name:-}" ]] || missing+=("variable ${name}")
  done
  if ((${#missing[@]} > 0)); then
    printf '::error title=%s TestFlight is not configured::Set in the ios-testflight environment: %s. See docs/contributing/releasing.md#apple-testflight.\n' "$APP_LABEL" "${missing[*]}"
    fail "missing ${missing[*]}"
  fi
  # Each app is its own record: one Apple ID in both variables would validate one app's IPA
  # against the other's record.
  if [[ -n "${ASC_APPLE_APP_ID:-}" && "${ASC_APPLE_APP_ID:-}" == "${ASC_APPLE_TV_APP_ID:-}" ]]; then
    fail 'ASC_APPLE_APP_ID and ASC_APPLE_TV_APP_ID name the same App Store Connect record'
  fi
  require_env RUNNER_TEMP GITHUB_ENV
  if [[ ! "$ASC_API_KEY_ID" =~ ^[A-Z0-9]{10}$ ]]; then
    fail 'ASC_API_KEY_ID must be 10 uppercase letters or digits'
  fi
  local key_dir="${RUNNER_TEMP}/asc" key_path
  key_path="${key_dir}/AuthKey_${ASC_API_KEY_ID}.p8"
  (
    umask 077
    mkdir -p "$key_dir"
    printf '%s' "$ASC_API_KEY_P8_BASE64" | base64 --decode > "$key_path"
  )
  if [[ ! -s "$key_path" ]]; then
    fail 'ASC_API_KEY_P8_BASE64 decoded to nothing'
  fi
  printf 'LOOMARR_ASC_KEY_PATH=%s\n' "$key_path" >> "$GITHUB_ENV"
}

case "${1:-}" in
  plan) plan "${2:-}" "${3:-}" ;;
  iphone | apple-tv)
    select_app "$1"
    case "${2:-}" in
      prepare-credentials) prepare_credentials ;;
      build) build "${3:-}" ;;
      validate) submit --validate-app ;;
      upload) submit --upload-package ;;
      *) usage ;;
    esac
    ;;
  *) usage ;;
esac
