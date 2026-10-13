#!/usr/bin/env bash
# Fail when CocoaPods installed a precompiled framework that was not built from this app's React
# Native.
#
# Prebuilt C++ that includes React Native headers bakes in the layout of React types. The Expo
# canary's precompiled ExpoModulesCore was compiled against an older React Native whose
# ShadowNodeFamily is 64 bytes smaller than react-native-tvos 0.86.2-0's: every Expo view that
# mounted overflowed the heap, and the phone crashed later in unrelated code (#1816). The only
# frameworks allowed are React Native's own release archives, which the react-native-tvos patch
# checksum-verifies. Everything else builds from source (`expo.autolinking.ios.buildFromSource`).
set -euo pipefail

if [[ $# -ne 1 || ! -d "$1" ]]; then
  printf 'usage: %s <Pods directory>\n' "$0" >&2
  exit 2
fi
readonly PODS_DIR="${1%/}"
readonly ALLOWED=(
  React-Core-prebuilt/React.xcframework
  ReactNativeDependencies/framework/packages/react-native/ReactNativeDependencies.xcframework
  hermes-engine/destroot/Library/Frameworks/universal/hermesvm.xcframework
)

unexpected=()
while IFS= read -r -d '' framework; do
  relative="${framework#"$PODS_DIR"/}"
  allowed=false
  for entry in "${ALLOWED[@]}"; do
    if [[ "$relative" == "$entry" ]]; then
      allowed=true
    fi
  done
  if [[ "$allowed" == false ]]; then
    unexpected+=("$relative")
  fi
done < <(find "$PODS_DIR" -name '*.xcframework' -prune -print0)

if (( ${#unexpected[@]} > 0 )); then
  printf 'ios-pods: precompiled frameworks not built from this React Native:\n' >&2
  printf '  %s\n' "${unexpected[@]}" >&2
  printf 'ios-pods: build the owning package from source (expo.autolinking.ios.buildFromSource)\n' >&2
  exit 1
fi
printf 'ios-pods: only React Native release frameworks are precompiled\n'
