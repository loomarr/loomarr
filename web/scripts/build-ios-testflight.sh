#!/usr/bin/env bash
# Archive, export, verify, validate, and upload the permanent-identity Expo iPhone app (ADR 0042).
#
#   build-ios-testflight.sh prepare-credentials      fail on missing secrets, write the API key
#   build-ios-testflight.sh build <x.y.z[-beta.N]>   archive + export + verify the IPA
#   build-ios-testflight.sh validate                 altool --validate-app on the verified IPA
#   build-ios-testflight.sh upload                   altool --upload-package, never retried
#
# Every credential arrives through the environment; nothing here prints one.
set -euo pipefail

readonly MODE="${1:-}"
WEB_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly WEB_ROOT
readonly APP_DIR="${WEB_ROOT}/apps/mobile"
readonly OUTPUT_DIR="${LOOMARR_IOS_OUTPUT_DIR:-${WEB_ROOT}/../.artifacts/ios-testflight}"
readonly BUNDLE_ID="media.loomarr.mobile"
# Expo names the Xcode project after the sanitised app name: "Loomarr" -> Loomarr.
readonly SCHEME="Loomarr"

usage() {
  printf 'usage: %s prepare-credentials | build <x.y.z[-beta.N]> | validate | upload\n' "$0" >&2
  exit 2
}

require_env() {
  local name
  for name in "$@"; do
    if [[ -z "${!name:-}" ]]; then
      printf 'ios-testflight: %s is required but empty or unset\n' "$name" >&2
      exit 2
    fi
  done
}

require_commands() {
  local command_name
  for command_name in "$@"; do
    command -v "$command_name" >/dev/null 2>&1 || {
      printf 'ios-testflight: %s is required\n' "$command_name" >&2
      exit 2
    }
  done
}

# The key, issuer, and team are needed to sign (build) and to talk to App Store Connect.
require_credentials() {
  require_env LOOMARR_ASC_KEY_PATH LOOMARR_ASC_KEY_ID LOOMARR_ASC_ISSUER_ID
  if [[ ! "$LOOMARR_ASC_KEY_ID" =~ ^[A-Z0-9]{10}$ ]]; then
    printf 'ios-testflight: ASC_API_KEY_ID must be 10 uppercase letters or digits\n' >&2
    exit 2
  fi
  if [[ ! -f "$LOOMARR_ASC_KEY_PATH" ]]; then
    printf 'ios-testflight: the API key file does not exist\n' >&2
    exit 2
  fi
  # altool finds the private key only as AuthKey_<id>.p8 inside API_PRIVATE_KEYS_DIR.
  if [[ "$(basename "$LOOMARR_ASC_KEY_PATH")" != "AuthKey_${LOOMARR_ASC_KEY_ID}.p8" ]]; then
    printf 'ios-testflight: the API key file must be named AuthKey_<ASC_API_KEY_ID>.p8\n' >&2
    exit 2
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
    printf 'ios-testflight: ITMS-90111, Apple rejected the build toolchain (Xcode or macOS is not a GM release).\n' >&2
    printf 'ios-testflight: not retrying; see the Xcode and macOS versions logged by the build step.\n' >&2
    exit 1
  fi
  rm -f -- "$log"
  if ((status != 0)); then
    printf 'ios-testflight: altool %s failed with exit status %s\n' "$1" "$status" >&2
    exit 1
  fi
}

# Reads the verified IPA path and its Apple metadata back from the evidence the build wrote.
load_evidence() {
  local evidence
  evidence="$(find "$OUTPUT_DIR" -maxdepth 1 -type f -name 'loomarr-ios-*.json' -print -quit 2>/dev/null || true)"
  if [[ -z "$evidence" ]]; then
    printf 'ios-testflight: no verification evidence in %s; run build first\n' "$OUTPUT_DIR" >&2
    exit 1
  fi
  IPA="${evidence%.json}.ipa"
  if [[ ! -f "$IPA" ]]; then
    printf 'ios-testflight: the verified IPA is missing: %s\n' "$IPA" >&2
    exit 1
  fi
  SHORT_VERSION="$(node -p "require(process.argv[1]).version" "$evidence")"
  BUILD_NUMBER="$(node -p "require(process.argv[1]).buildNumber" "$evidence")"
  EVIDENCE_BUNDLE_ID="$(node -p "require(process.argv[1]).bundleIdentifier" "$evidence")"
  if [[ "$EVIDENCE_BUNDLE_ID" != "$BUNDLE_ID" ]]; then
    printf 'ios-testflight: evidence names unexpected bundle id %s\n' "$EVIDENCE_BUNDLE_ID" >&2
    exit 1
  fi
  # Submit only the bytes the build verified, not whatever now has the IPA's name.
  local expected_sha256 actual_sha256
  expected_sha256="$(node -p "require(process.argv[1]).ipaSha256" "$evidence")"
  actual_sha256="$(shasum -a 256 "$IPA" | awk '{ print $1 }')"
  if [[ ! "$expected_sha256" =~ ^[0-9a-f]{64}$ || "$actual_sha256" != "$expected_sha256" ]]; then
    printf 'ios-testflight: %s does not match the SHA-256 the build verified\n' "$IPA" >&2
    exit 1
  fi
}

submit() {
  local verb="$1"
  require_credentials
  require_env LOOMARR_ASC_APPLE_APP_ID
  if [[ ! "$LOOMARR_ASC_APPLE_APP_ID" =~ ^[0-9]+$ ]]; then
    printf 'ios-testflight: ASC_APPLE_APP_ID must be the numeric Apple ID of the app record\n' >&2
    exit 2
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
    --platform ios --apple-id "$LOOMARR_ASC_APPLE_APP_ID" \
    --bundle-id "$BUNDLE_ID" --bundle-version "$BUILD_NUMBER" \
    --bundle-short-version-string "$SHORT_VERSION" \
    --api-key "$LOOMARR_ASC_KEY_ID" --api-issuer "$LOOMARR_ASC_ISSUER_ID" \
    --output-format json
  printf 'ios-testflight: %s succeeded for %s (%s)\n' "$verb" "$SHORT_VERSION" "$BUILD_NUMBER"
}

build() {
  local release_version="${1:-}"
  if [[ -z "$release_version" ]]; then
    usage
  fi
  if [[ ! "$release_version" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
    printf 'ios-testflight: version must be x.y.z with an optional suffix; got "%s"\n' "$release_version" >&2
    exit 2
  fi
  require_env LOOMARR_IOS_BUILD_NUMBER LOOMARR_APPLE_TEAM_ID
  if [[ ! "$LOOMARR_IOS_BUILD_NUMBER" =~ ^[1-9][0-9]{0,8}$ ]]; then
    printf 'ios-testflight: build number must be a positive integer; got "%s"\n' "$LOOMARR_IOS_BUILD_NUMBER" >&2
    exit 2
  fi
  if [[ ! "$LOOMARR_APPLE_TEAM_ID" =~ ^[A-Z0-9]{10}$ ]]; then
    printf 'ios-testflight: APPLE_TEAM_ID must be 10 uppercase letters or digits\n' >&2
    exit 2
  fi
  require_credentials
  if [[ "$(uname -s)" != "Darwin" ]]; then
    printf 'ios-testflight: the iPhone build requires macOS with Xcode\n' >&2
    exit 2
  fi
  require_commands xcodebuild xcrun plutil lipo codesign unzip pod node pnpm shasum file

  # Evidence for triaging ITMS-90111 and for the first dispatch's open questions.
  xcodebuild -version
  sw_vers
  local xcode_version
  xcode_version="$(xcodebuild -version | awk 'NR == 1 { print $2 }')"
  if [[ ! "$xcode_version" =~ ^27\. ]]; then
    printf 'ios-testflight: requires Xcode 27.x; found %s\n' "$xcode_version" >&2
    exit 2
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
      printf 'ios-testflight: %s overrides the qualified dependency selection\n' "$native_override" >&2
      exit 2
    fi
  done

  local expo_package_json expo_template
  expo_package_json="$(cd "$APP_DIR" && node -p "require.resolve('expo/package.json')")"
  expo_template="${expo_package_json%/package.json}/template.tgz"
  if [[ ! -f "$expo_template" ]]; then
    printf 'ios-testflight: the pinned Expo package has no native template: %s\n' "$expo_template" >&2
    exit 2
  fi

  # app.config.cjs validates the version and fixes the identity; read the result back.
  export LOOMARR_IOS_RELEASE_CHANNEL=testflight
  export LOOMARR_IOS_VERSION="$release_version"
  local config_json short_version config_build config_bundle
  config_json="$(cd "$APP_DIR" && pnpm exec expo config --json)"
  short_version="$(node -p "JSON.parse(process.argv[1]).version" "$config_json")"
  config_build="$(node -p "JSON.parse(process.argv[1]).ios.buildNumber" "$config_json")"
  config_bundle="$(node -p "JSON.parse(process.argv[1]).ios.bundleIdentifier" "$config_json")"
  if [[ "$config_bundle" != "$BUNDLE_ID" ]]; then
    printf 'ios-testflight: unexpected bundle identifier %s\n' "$config_bundle" >&2
    exit 1
  fi
  if [[ "$config_build" != "$LOOMARR_IOS_BUILD_NUMBER" ]]; then
    printf 'ios-testflight: build number was rewritten to %s\n' "$config_build" >&2
    exit 1
  fi

  (
    cd "$WEB_ROOT"
    pnpm --filter @loomarr/mobile exec expo prebuild --platform ios --clean --no-install \
      --template "$expo_template"
  )
  # react-native-svg resolves React Native through the app-local react-native-tvos alias link.
  (
    cd "${APP_DIR}/ios"
    NODE_ENV=production RCT_NO_LAUNCH_PACKAGER=1 \
      REACT_NATIVE_NODE_MODULES_DIR="${APP_DIR}/node_modules" pod install
  )
  local workspace="${APP_DIR}/ios/${SCHEME}.xcworkspace"
  if [[ ! -d "$workspace" ]]; then
    printf 'ios-testflight: prebuild did not produce %s\n' "$workspace" >&2
    exit 1
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
    -destination 'generic/platform=iOS' -archivePath "$archive" \
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
    printf 'ios-testflight: export produced no IPA\n' >&2
    exit 1
  fi
  final_ipa="${OUTPUT_DIR}/loomarr-ios-${release_version#v}-${LOOMARR_IOS_BUILD_NUMBER}.ipa"
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
    printf 'ios-testflight: the IPA has no application bundle\n' >&2
    exit 1
  fi
  info="${app}/Info.plist"
  plist_value() { plutil -extract "$1" raw -o - "$info" 2>/dev/null || true; }
  expect() {
    local what="$1" actual="$2" expected="$3"
    if [[ "$actual" != "$expected" ]]; then
      printf 'ios-testflight: IPA %s is "%s"; expected "%s"\n' "$what" "$actual" "$expected" >&2
      exit 1
    fi
  }
  expect CFBundleIdentifier "$(plist_value CFBundleIdentifier)" "$BUNDLE_ID"
  expect CFBundleShortVersionString "$(plist_value CFBundleShortVersionString)" "$short_version"
  expect CFBundleVersion "$(plist_value CFBundleVersion)" "$LOOMARR_IOS_BUILD_NUMBER"
  expect ITSAppUsesNonExemptEncryption "$(plist_value ITSAppUsesNonExemptEncryption)" false
  expect DTPlatformName "$(plist_value DTPlatformName)" iphoneos
  if [[ -z "$(plist_value NSLocalNetworkUsageDescription)" ]]; then
    printf 'ios-testflight: IPA has no NSLocalNetworkUsageDescription\n' >&2
    exit 1
  fi
  if [[ ! -f "${app}/PrivacyInfo.xcprivacy" ]]; then
    printf 'ios-testflight: IPA has no PrivacyInfo.xcprivacy at the app bundle root\n' >&2
    exit 1
  fi
  # The app-level manifest makes exactly the declaration app.config.cjs writes, and no tracking claim.
  node -e '
    const manifest = JSON.parse(process.argv[1]);
    const fail = (problem) => {
      console.error("ios-testflight: IPA privacy manifest " + problem);
      process.exit(1);
    };
    if (manifest.NSPrivacyTracking !== undefined && manifest.NSPrivacyTracking !== false) {
      fail("declares NSPrivacyTracking");
    }
    const types = manifest.NSPrivacyAccessedAPITypes;
    if (!Array.isArray(types) || types.length !== 1) {
      fail("has " + (Array.isArray(types) ? types.length : "no") + " accessed-API entries; expected 1");
    }
    if (types[0].NSPrivacyAccessedAPIType !== "NSPrivacyAccessedAPICategoryUserDefaults") {
      fail("declares " + types[0].NSPrivacyAccessedAPIType + "; expected NSPrivacyAccessedAPICategoryUserDefaults");
    }
    if (JSON.stringify(types[0].NSPrivacyAccessedAPITypeReasons) !== JSON.stringify(["CA92.1"])) {
      fail("UserDefaults reasons are not exactly CA92.1");
    }
  ' "$(plutil -convert json -o - "${app}/PrivacyInfo.xcprivacy")"
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
      printf 'ios-testflight: IPA %s is built for a simulator platform\n' "${binary#"${app}/"}" >&2
      exit 1
    fi
  done < <(find "$app" -type f -print0)
  codesign --verify --deep --strict "$app"

  node -e '
    const fs = require("node:fs");
    const [evidence, bundleIdentifier, version, buildNumber, releaseVersion, xcode, sha256, archs] =
      process.argv.slice(1);
    fs.writeFileSync(
      evidence,
      JSON.stringify(
        { bundleIdentifier, version, buildNumber, releaseVersion, xcode, ipaSha256: sha256, architectures: archs },
        null,
        2,
      ) + "\n",
    );
  ' "${ipa%.ipa}.json" "$BUNDLE_ID" "$short_version" "$LOOMARR_IOS_BUILD_NUMBER" \
    "$release_version" "$xcode_version" "$(shasum -a 256 "$ipa" | awk '{ print $1 }')" "$archs"
  printf 'ios-testflight: verified %s\n' "$ipa"
}

# Fails with one readable list of everything the release environment lacks, then writes the API
# key where altool and xcodebuild expect it. The key is decoded straight to disk and never printed.
prepare_credentials() {
  local -a missing=()
  local name
  for name in ASC_API_KEY_P8_BASE64 ASC_API_KEY_ID ASC_API_ISSUER_ID; do
    [[ -n "${!name:-}" ]] || missing+=("secret ${name}")
  done
  for name in APPLE_TEAM_ID ASC_APPLE_APP_ID; do
    [[ -n "${!name:-}" ]] || missing+=("variable ${name}")
  done
  if ((${#missing[@]} > 0)); then
    printf '::error title=iPhone TestFlight is not configured::Set in the ios-testflight environment: %s. See docs/contributing/releasing.md#iphone-testflight.\n' "${missing[*]}"
    printf 'ios-testflight: missing %s\n' "${missing[*]}" >&2
    exit 1
  fi
  require_env RUNNER_TEMP GITHUB_ENV
  if [[ ! "$ASC_API_KEY_ID" =~ ^[A-Z0-9]{10}$ ]]; then
    printf 'ios-testflight: ASC_API_KEY_ID must be 10 uppercase letters or digits\n' >&2
    exit 1
  fi
  local key_dir="${RUNNER_TEMP}/asc" key_path
  key_path="${key_dir}/AuthKey_${ASC_API_KEY_ID}.p8"
  (
    umask 077
    mkdir -p "$key_dir"
    printf '%s' "$ASC_API_KEY_P8_BASE64" | base64 --decode > "$key_path"
  )
  if [[ ! -s "$key_path" ]]; then
    printf 'ios-testflight: ASC_API_KEY_P8_BASE64 decoded to nothing\n' >&2
    exit 1
  fi
  printf 'LOOMARR_ASC_KEY_PATH=%s\n' "$key_path" >> "$GITHUB_ENV"
}

case "$MODE" in
  prepare-credentials) prepare_credentials ;;
  build) build "${2:-}" ;;
  validate) submit --validate-app ;;
  upload) submit --upload-package ;;
  *) usage ;;
esac
