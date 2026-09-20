#!/usr/bin/env bash
# Build and inspect the permanent-identity React Native Android TV App Bundle.
set -euo pipefail

readonly VERSION_NAME="${1:-}"
WEB_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly WEB_ROOT
REPO_ROOT="$(cd "${WEB_ROOT}/.." && pwd)"
readonly REPO_ROOT
readonly APP_DIR="${WEB_ROOT}/apps/tv"
readonly OUTPUT_DIR="${ANDROID_RELEASE_OUTPUT_DIR:-${REPO_ROOT}/.artifacts/android-release}"
readonly GRADLE_HEAP="${LOOMARR_ANDROID_GRADLE_HEAP:-1280m}"
readonly ARCHITECTURES="armeabi-v7a,arm64-v8a,x86,x86_64"
readonly GRADLE_WORKERS=1
readonly NATIVE_JOBS="${LOOMARR_ANDROID_NATIVE_JOBS:-1}"
readonly CCACHE_MAXSIZE="${CCACHE_MAXSIZE:-2G}"

if [[ -z "${VERSION_NAME}" ]]; then
  printf 'usage: %s <major.minor.patch[-beta.N|-rc.N]>\n' "$0" >&2
  exit 2
fi
if [[ -z "${ANDROID_HOME:-}" ]]; then
  printf 'ANDROID_HOME must point to the Android SDK\n' >&2
  exit 2
fi

CCACHE_LAUNCHER=""
if [[ -n "${LOOMARR_ANDROID_CCACHE_LAUNCHER:-}" ]]; then
  [[ "${LOOMARR_ANDROID_CCACHE_LAUNCHER}" == /* ]] || {
    printf 'LOOMARR_ANDROID_CCACHE_LAUNCHER must be an absolute path\n' >&2
    exit 2
  }
  CCACHE_LAUNCHER=$(realpath "${LOOMARR_ANDROID_CCACHE_LAUNCHER}")
  [[ -x "${CCACHE_LAUNCHER}" ]] || {
    printf 'LOOMARR_ANDROID_CCACHE_LAUNCHER must be an executable absolute path\n' >&2
    exit 2
  }
  "${CCACHE_LAUNCHER}" --version | grep -Fxq 'ccache version 4.14' || {
    printf 'LOOMARR_ANDROID_CCACHE_LAUNCHER must be ccache version 4.14\n' >&2
    exit 2
  }
  [[ -n "${CCACHE_DIR:-}" ]] || { printf 'CCACHE_DIR is required when ccache is enabled\n' >&2; exit 2; }
  mkdir -p "${CCACHE_DIR}"
  CCACHE_DIR=$(realpath "${CCACHE_DIR}")
  CCACHE_BASEDIR=$(realpath "${CCACHE_BASEDIR:-${REPO_ROOT}}")
  [[ "${CCACHE_BASEDIR}" == "${REPO_ROOT}" ]] || {
    printf 'CCACHE_BASEDIR must be this worktree root for worktree-independent keys\n' >&2
    exit 2
  }
  [[ -z "${CCACHE_SLOPPINESS:-}" ]] || { printf 'CCACHE_SLOPPINESS is forbidden\n' >&2; exit 2; }
  export CCACHE_DIR CCACHE_BASEDIR CCACHE_COMPILERCHECK=content CCACHE_MAXSIZE
  "${CCACHE_LAUNCHER}" --set-config=compiler_check=content
  "${CCACHE_LAUNCHER}" --set-config=max_size="${CCACHE_MAXSIZE}"
  "${CCACHE_LAUNCHER}" --zero-stats
fi
readonly CCACHE_LAUNCHER

record_ccache_evidence() {
  local phase=$1
  [[ -n "${CCACHE_LAUNCHER}" && -n "${ANDROID_BUILD_PROFILE_DIR:-}" ]] || return 0
  mkdir -p "${ANDROID_BUILD_PROFILE_DIR}"
  "${CCACHE_LAUNCHER}" --version > "${ANDROID_BUILD_PROFILE_DIR}/ccache-version.txt"
  "${CCACHE_LAUNCHER}" --show-config > "${ANDROID_BUILD_PROFILE_DIR}/ccache-config.txt"
  "${CCACHE_LAUNCHER}" --print-stats --format=json > "${ANDROID_BUILD_PROFILE_DIR}/ccache-stats-${phase}.json"
}
record_ccache_evidence pre

"${REPO_ROOT}/scripts/check-android-release-env.sh"

[[ "$(node -p "require('${APP_DIR}/package.json').main")" == "index.ts" ]] || {
  printf 'Shield production entry must remain apps/tv/index.ts\n' >&2
  exit 1
}
grep -Fq 'from "./src/app"' "${APP_DIR}/index.ts" || {
  printf 'Shield production entry must register src/app\n' >&2
  exit 1
}

KEYSTORE_PATH="$(cd "$(dirname "${LOOMARR_ANDROID_KEYSTORE_PATH}")" && pwd)/$(basename "${LOOMARR_ANDROID_KEYSTORE_PATH}")"
readonly KEYSTORE_PATH
EXPO_PACKAGE_JSON="$(cd "${APP_DIR}" && node -p "require.resolve('expo/package.json')")"
readonly EXPO_PACKAGE_JSON
readonly EXPO_TEMPLATE="${EXPO_PACKAGE_JSON%/package.json}/template.tgz"
[[ -f "${EXPO_TEMPLATE}" ]] || {
  printf 'the pinned Expo package does not contain its native template: %s\n' "${EXPO_TEMPLATE}" >&2
  exit 2
}

export LOOMARR_SHIELD_RELEASE_CHANNEL=play
export LOOMARR_ANDROID_KEYSTORE_PATH="${KEYSTORE_PATH}"
export EXPO_PUBLIC_LOOMARR_CLIENT_VERSION="${VERSION_NAME}"

(
  cd "${WEB_ROOT}"
  CI=1 EXPO_TV=1 pnpm --filter @loomarr/tv exec expo prebuild --clean --platform android \
    --no-install --template "${EXPO_TEMPLATE}"
)

gradle_args=(
  bundleRelease
  --no-daemon
  --build-cache
  "--max-workers=${GRADLE_WORKERS}"
  "-Dorg.gradle.jvmargs=-Xmx${GRADLE_HEAP}"
  -Pkotlin.compiler.execution.strategy=in-process
  "-PreactNativeArchitectures=${ARCHITECTURES}"
)
if [[ -n "${ANDROID_BUILD_PROFILE_DIR:-}" ]]; then
  mkdir -p "${ANDROID_BUILD_PROFILE_DIR}"
  gradle_args+=(--profile)
  PROFILE_NATIVE_JOBS="${NATIVE_JOBS}" PROFILE_GRADLE_WORKERS="${GRADLE_WORKERS}" \
    PROFILE_GRADLE_HEAP="${GRADLE_HEAP}" PROFILE_ARCHITECTURES="${ARCHITECTURES}" node <<'JS'
const fs = require("node:fs");
const path = require("node:path");
fs.writeFileSync(path.join(process.env.ANDROID_BUILD_PROFILE_DIR, "gradle-settings.json"), JSON.stringify({
  nativeJobs: Number(process.env.PROFILE_NATIVE_JOBS),
  gradleWorkers: Number(process.env.PROFILE_GRADLE_WORKERS),
  gradleHeap: process.env.PROFILE_GRADLE_HEAP,
  architectures: process.env.PROFILE_ARCHITECTURES.split(","),
}, null, 2) + "\n", { mode: 0o600 });
JS
fi

gradle_status=0
(
  cd "${APP_DIR}/android"
  CMAKE_BUILD_PARALLEL_LEVEL="${NATIVE_JOBS}" NODE_ENV=production EXPO_TV=1 \
    LOOMARR_ANDROID_CCACHE_LAUNCHER="${CCACHE_LAUNCHER}" \
    ./gradlew "${gradle_args[@]}"
) || gradle_status=$?
if [[ -n "${ANDROID_BUILD_PROFILE_DIR:-}" && -d "${APP_DIR}/android/build/reports/profile" ]]; then
  cp -R "${APP_DIR}/android/build/reports/profile" "${ANDROID_BUILD_PROFILE_DIR}/gradle"
fi
if (( gradle_status != 0 )); then
  record_ccache_evidence post
  exit "${gradle_status}"
fi
record_ccache_evidence post

if [[ -n "${CCACHE_LAUNCHER}" ]]; then
  ccache_evidence_output=""
  if [[ -n "${ANDROID_BUILD_PROFILE_DIR:-}" ]]; then
    ccache_evidence_output="${ANDROID_BUILD_PROFILE_DIR}/ccache-ninja-launchers.txt"
  fi
  "${WEB_ROOT}/scripts/verify-android-ccache-evidence.sh" \
    "${CCACHE_LAUNCHER}" "${WEB_ROOT}" "${ccache_evidence_output}"
fi

readonly GENERATED_AAB="${APP_DIR}/android/app/build/outputs/bundle/release/app-release.aab"
[[ -f "${GENERATED_AAB}" ]] || {
  printf 'Gradle did not produce the expected Shield bundle: %s\n' "${GENERATED_AAB}" >&2
  exit 1
}
mkdir -p "${OUTPUT_DIR}"
readonly ARTIFACT_STEM="loomarr-tv-${VERSION_NAME}-${LOOMARR_ANDROID_VERSION_CODE}"
readonly OUTPUT_AAB="${OUTPUT_DIR}/${ARTIFACT_STEM}.aab"
readonly EVIDENCE_PATH="${OUTPUT_DIR}/${ARTIFACT_STEM}.json"
cp "${GENERATED_AAB}" "${OUTPUT_AAB}"
"${WEB_ROOT}/scripts/verify-shield-aab.sh" \
  "${OUTPUT_AAB}" "${VERSION_NAME}" "${LOOMARR_ANDROID_VERSION_CODE}" "${EVIDENCE_PATH}"
