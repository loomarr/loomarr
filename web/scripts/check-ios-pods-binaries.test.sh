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

pods="$temp_dir/source-built/Pods"
react_native_pods "$pods"
if ! "$check" "$pods/" >"$temp_dir/stdout" 2>"$temp_dir/stderr"; then
  echo "React Native release frameworks alone: the check failed" >&2
  cat "$temp_dir/stderr" >&2
  exit 1
fi

pods="$temp_dir/precompiled-expo/Pods"
react_native_pods "$pods"
mkdir -p "$pods/ExpoModulesCore/ExpoModulesCore.xcframework/ios-arm64/ExpoModulesCore.framework"
if "$check" "$pods" >"$temp_dir/stdout" 2>"$temp_dir/stderr"; then
  echo "precompiled ExpoModulesCore: the check passed" >&2
  exit 1
fi
if ! grep -Fxq '  ExpoModulesCore/ExpoModulesCore.xcframework' "$temp_dir/stderr"; then
  echo "precompiled ExpoModulesCore: expected its path in the failure" >&2
  cat "$temp_dir/stderr" >&2
  exit 1
fi

# A React framework at an unexpected path is someone else's build, not the verified archive.
pods="$temp_dir/moved-react/Pods"
mkdir -p "$pods/SomePod/React.xcframework"
if "$check" "$pods" >"$temp_dir/stdout" 2>"$temp_dir/stderr"; then
  echo "React.xcframework outside React-Core-prebuilt: the check passed" >&2
  exit 1
fi

if "$check" "$temp_dir/missing" >"$temp_dir/stdout" 2>"$temp_dir/stderr"; then
  echo "missing Pods directory: the check passed" >&2
  exit 1
fi

echo "check-ios-pods-binaries: ok"
