#!/usr/bin/env bash
# Build one prototype Android client without letting Gradle's native process
# tree exhaust a Linux development workstation.
set -euo pipefail

readonly APP_NAME="${1:-mobile}"
readonly ACTION="${2:-debug}"
readonly SCOPE_MARKER="${3:-}"
WEB_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly WEB_ROOT
readonly SCRIPT_PATH="${WEB_ROOT}/scripts/build-android-client.sh"
readonly APP_DIR="${WEB_ROOT}/apps/${APP_NAME}"
readonly MEMORY_HIGH="${LOOMARR_ANDROID_MEMORY_HIGH:-2500M}"
readonly MEMORY_MAX="${LOOMARR_ANDROID_MEMORY_MAX:-3G}"
readonly GRADLE_HEAP="${LOOMARR_ANDROID_GRADLE_HEAP:-1024m}"
readonly ARCHITECTURES="${LOOMARR_ANDROID_ARCHITECTURES:-arm64-v8a}"
readonly CPUSET="${LOOMARR_ANDROID_CPUSET:-0-3}"
readonly NATIVE_JOBS="${LOOMARR_ANDROID_NATIVE_JOBS:-1}"
readonly MIN_AVAILABLE_KB="${LOOMARR_ANDROID_MIN_AVAILABLE_KB:-6291456}"

if [[ "${APP_NAME}" != "mobile" && "${APP_NAME}" != "tv" ]]; then
  printf 'usage: %s [mobile|tv] [debug|macrobenchmark]\n' "$0" >&2
  exit 2
fi
if [[ "${ACTION}" != "debug" && "${ACTION}" != "macrobenchmark" ]]; then
  printf 'usage: %s [mobile|tv] [debug|macrobenchmark]\n' "$0" >&2
  exit 2
fi
if [[ "${ACTION}" == "macrobenchmark" && "${APP_NAME}" != "tv" ]]; then
  printf 'macrobenchmark is available only for the TV client\n' >&2
  exit 2
fi
if [[ "${APP_NAME}" == "tv" ]]; then
  readonly ENTRY_FILE="index.ts"
else
  readonly ENTRY_FILE="node_modules/expo-router/entry.js"
fi
if [[ -z "${ANDROID_HOME:-}" ]]; then
  printf 'ANDROID_HOME must point to the Android SDK\n' >&2
  exit 2
fi

if [[ "${SCOPE_MARKER}" != "--inside-memory-scope" ]]; then
  available_kb="$(awk '/^MemAvailable:/ { print $2 }' /proc/meminfo)"
  if [[ "${available_kb}" -lt "${MIN_AVAILABLE_KB}" ]]; then
    printf 'refusing native build: %s MiB available; require at least %s MiB\n' \
      "$((available_kb / 1024))" "$((MIN_AVAILABLE_KB / 1024))" >&2
    exit 1
  fi
fi

if [[ ! -x "${APP_DIR}/android/gradlew" ]] \
  || [[ "${ACTION}" == "macrobenchmark" && ! -f "${APP_DIR}/android/macrobenchmark/build.gradle" ]]; then
  (
    cd "${WEB_ROOT}"
    pnpm --filter "@loomarr/${APP_NAME}" exec expo prebuild --platform android --no-install
  )
fi

if [[ "${SCOPE_MARKER}" != "--inside-memory-scope" ]] \
  && command -v systemd-run >/dev/null 2>&1 \
  && systemctl --user show-environment >/dev/null 2>&1; then
  exec systemd-run --user --scope --quiet --nice=10 \
    -p MemoryAccounting=yes \
    -p "MemoryHigh=${MEMORY_HIGH}" \
    -p "MemoryMax=${MEMORY_MAX}" \
    -p CPUQuota=200% \
    /usr/bin/ionice -c 2 -n 7 /usr/bin/env \
    LOOMARR_ANDROID_GRADLE_HEAP="${GRADLE_HEAP}" \
    LOOMARR_ANDROID_ARCHITECTURES="${ARCHITECTURES}" \
    LOOMARR_ANDROID_CPUSET="${CPUSET}" \
    LOOMARR_ANDROID_NATIVE_JOBS="${NATIVE_JOBS}" \
    ANDROID_HOME="${ANDROID_HOME}" \
    EXPO_PUBLIC_LOOMARR_URL="${EXPO_PUBLIC_LOOMARR_URL:-}" \
    LOOMARR_TAMAGUI_COMPILER="${LOOMARR_TAMAGUI_COMPILER:-0}" \
    ANDROID_SERIAL="${ANDROID_SERIAL:-}" \
    "${SCRIPT_PATH}" "${APP_NAME}" "${ACTION}" --inside-memory-scope
fi

if [[ "${SCOPE_MARKER}" != "--inside-memory-scope" ]]; then
  printf 'warning: user systemd unavailable; native build has worker limits but no memory ceiling\n' >&2
fi

if [[ "${ACTION}" == "macrobenchmark" ]]; then
  if [[ -z "${ANDROID_SERIAL:-}" ]]; then
    printf 'ANDROID_SERIAL must identify the physical Shield for Macrobenchmark\n' >&2
    exit 2
  fi
  device_model="$(adb -s "${ANDROID_SERIAL}" shell getprop ro.product.model | tr -d '\r')"
  if [[ ! "${device_model}" =~ SHIELD ]]; then
    printf 'refusing Macrobenchmark on %s; P5 requires a physical Shield\n' "${device_model:-unknown device}" >&2
    exit 1
  fi
fi

# `assembleDebug` normally expects Metro and therefore produces an APK that opens to React Native's
# red "Unable to load script" screen when installed on a remote Shield. This target is the physical
# device proof, so embed the production JS/assets explicitly while retaining debug-native signing
# and the faster incremental native build. Keep this below the scope handoff: otherwise the outer
# and inner script processes both bundle, and the first Metro process escapes the memory limit.
# Reset Metro's transform cache because EXPO_PUBLIC_* values are compile-time inputs that Metro does
# not include in its cache key. Without this, rebuilding for another Loomarr server can retain the
# preceding server URL even though the build command and environment are correct.
if [[ "${ACTION}" == "debug" ]]; then
  mkdir -p "${APP_DIR}/android/app/src/main/assets" "${APP_DIR}/android/app/src/main/res"
  (
    cd "${APP_DIR}"
    NODE_ENV=production pnpm exec expo export:embed \
      --platform android \
      --dev false \
      --entry-file "${ENTRY_FILE}" \
      --bundle-output android/app/src/main/assets/index.android.bundle \
      --assets-dest android/app/src/main/res \
      --max-workers 1 \
      --reset-cache
  )
fi

cd "${APP_DIR}/android"
if [[ "${ACTION}" == "macrobenchmark" ]]; then
  gradle_task=":macrobenchmark:connectedBenchmarkAndroidTest"
  gradle_node_env="production"
else
  gradle_task=":app:assembleDebug"
  gradle_node_env="development"
fi
gradle_command=(./gradlew "${gradle_task}"
  --no-daemon \
  --max-workers=1 \
  "-Dorg.gradle.jvmargs=-Xmx${GRADLE_HEAP}" \
  -Pkotlin.compiler.execution.strategy=in-process \
  "-PreactNativeArchitectures=${ARCHITECTURES}")
if [[ "${ACTION}" == "macrobenchmark" ]]; then
  gradle_command+=("-Pandroid.enableAdditionalTestOutput=true")
fi
if command -v taskset >/dev/null 2>&1; then
  CMAKE_BUILD_PARALLEL_LEVEL="${NATIVE_JOBS}" NODE_ENV="${gradle_node_env}" \
    taskset --cpu-list "${CPUSET}" "${gradle_command[@]}"
else
  CMAKE_BUILD_PARALLEL_LEVEL="${NATIVE_JOBS}" NODE_ENV="${gradle_node_env}" "${gradle_command[@]}"
fi

if [[ "${ACTION}" == "macrobenchmark" ]]; then
  report_path="$(find macrobenchmark/build/outputs -type f -name '*benchmarkData.json' -printf '%T@ %p\n' \
    | sort -nr | head -1 | cut -d' ' -f2-)"
  if [[ -z "${report_path}" ]]; then
    printf 'Macrobenchmark completed without a benchmarkData.json report\n' >&2
    exit 1
  fi
  node "${WEB_ROOT}/scripts/verify-tv-macrobenchmark.mjs" "${report_path}"
fi
