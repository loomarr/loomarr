#!/usr/bin/env bash
# Contract of check-ios-pods-binaries.sh over synthetic Pods trees: React Native's release
# frameworks pass, and any other precompiled framework (the Expo canary's ExpoModulesCore, #1816)
# fails with its path.
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
check="$script_dir/check-ios-pods-binaries.sh"

temp_dir=$(mktemp -d)
trap 'rm -rf -- "$temp_dir"' EXIT

react_native_pods() {
  local pods="$1"
  mkdir -p "$pods/React-Core-prebuilt/React.xcframework/ios-arm64" \
    "$pods/ReactNativeDependencies/framework/packages/react-native/ReactNativeDependencies.xcframework" \
    "$pods/hermes-engine/destroot/Library/Frameworks/universal/hermesvm.xcframework" \
    "$pods/RNScreens/ios"
}

# Runs the check and requires the exact exit status and, when given, an exact stderr line, so a
# crash or a usage error cannot pass for a rejection.
expect_status() {
  local description="$1" expected="$2" line="$3" pods="$4" status=0
  "$check" "$pods" >"$temp_dir/stdout" 2>"$temp_dir/stderr" || status=$?
  if [[ "$status" != "$expected" ]]; then
    echo "$description: exit status $status, expected $expected" >&2
    cat "$temp_dir/stderr" >&2
    exit 1
  fi
  if [[ -n "$line" ]] && ! grep -Fxq -- "$line" "$temp_dir/stderr"; then
    echo "$description: expected stderr line: $line" >&2
    cat "$temp_dir/stderr" >&2
    exit 1
  fi
}

pods="$temp_dir/source-built/Pods"
react_native_pods "$pods"
expect_status "React Native release frameworks alone" 0 "" "$pods/"

pods="$temp_dir/precompiled-expo/Pods"
react_native_pods "$pods"
mkdir -p "$pods/ExpoModulesCore/ExpoModulesCore.xcframework/ios-arm64/ExpoModulesCore.framework"
expect_status "precompiled ExpoModulesCore" 1 "  ExpoModulesCore/ExpoModulesCore.xcframework" "$pods"

# A React framework at an unexpected path is someone else's build, not the verified archive.
pods="$temp_dir/moved-react/Pods"
mkdir -p "$pods/SomePod/React.xcframework"
expect_status "React.xcframework outside React-Core-prebuilt" 1 "  SomePod/React.xcframework" "$pods"

expect_status "missing Pods directory" 2 "" "$temp_dir/missing"

echo "check-ios-pods-binaries: ok"
