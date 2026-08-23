#!/usr/bin/env bash
# Build one prototype Android client without letting Gradle's native process
# tree exhaust a Linux development workstation.
set -euo pipefail

readonly APP_NAME="${1:-mobile}"
readonly SCOPE_MARKER="${2:-}"
WEB_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly WEB_ROOT
readonly SCRIPT_PATH="${WEB_ROOT}/scripts/build-android-client.sh"
readonly APP_DIR="${WEB_ROOT}/apps/${APP_NAME}"
readonly MEMORY_HIGH="${LOOMARR_ANDROID_MEMORY_HIGH:-3750M}"
readonly MEMORY_MAX="${LOOMARR_ANDROID_MEMORY_MAX:-4G}"
readonly GRADLE_HEAP="${LOOMARR_ANDROID_GRADLE_HEAP:-1280m}"
readonly ARCHITECTURES="${LOOMARR_ANDROID_ARCHITECTURES:-arm64-v8a}"
readonly CPUSET="${LOOMARR_ANDROID_CPUSET:-0-3}"

if [[ "${APP_NAME}" != "mobile" && "${APP_NAME}" != "tv" ]]; then
  printf 'usage: %s [mobile|tv]\n' "$0" >&2
  exit 2
fi
if [[ -z "${ANDROID_HOME:-}" ]]; then
  printf 'ANDROID_HOME must point to the Android SDK\n' >&2
  exit 2
fi

if [[ ! -x "${APP_DIR}/android/gradlew" ]]; then
  (
    cd "${WEB_ROOT}"
    pnpm --filter "@loomarr/${APP_NAME}" exec expo prebuild --platform android --no-install
  )
fi

if [[ "${SCOPE_MARKER}" != "--inside-memory-scope" ]] \
  && command -v systemd-run >/dev/null 2>&1 \
  && systemctl --user show-environment >/dev/null 2>&1; then
  exec systemd-run --user --scope --quiet \
    -p MemoryAccounting=yes \
    -p "MemoryHigh=${MEMORY_HIGH}" \
    -p "MemoryMax=${MEMORY_MAX}" \
    /usr/bin/env \
    LOOMARR_ANDROID_GRADLE_HEAP="${GRADLE_HEAP}" \
    LOOMARR_ANDROID_ARCHITECTURES="${ARCHITECTURES}" \
    LOOMARR_ANDROID_CPUSET="${CPUSET}" \
    ANDROID_HOME="${ANDROID_HOME}" \
    "${SCRIPT_PATH}" "${APP_NAME}" --inside-memory-scope
fi

if [[ "${SCOPE_MARKER}" != "--inside-memory-scope" ]]; then
  printf 'warning: user systemd unavailable; native build has worker limits but no memory ceiling\n' >&2
fi

cd "${APP_DIR}/android"
gradle_command=(./gradlew assembleDebug
  --no-daemon \
  --max-workers=1 \
  "-Dorg.gradle.jvmargs=-Xmx${GRADLE_HEAP}" \
  -Pkotlin.compiler.execution.strategy=in-process \
  "-PreactNativeArchitectures=${ARCHITECTURES}")
if command -v taskset >/dev/null 2>&1; then
  NODE_ENV=development taskset --cpu-list "${CPUSET}" "${gradle_command[@]}"
else
  NODE_ENV=development "${gradle_command[@]}"
fi
