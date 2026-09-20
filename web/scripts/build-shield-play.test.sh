#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
build="$script_dir/build-shield-play.sh"
verifier="$script_dir/verify-android-ccache-evidence.sh"

temp_dir=$(mktemp -d)
trap 'rm -rf -- "$temp_dir"' EXIT
launcher="$temp_dir/ccache"
# The generated fixture expands this at execution time.
# shellcheck disable=SC2016
printf '#!/usr/bin/env bash\nprintf "%%s\\n" "${FAKE_CCACHE_STATS:?}"\n' > "$launcher"
chmod +x "$launcher"
actual_rule="$temp_dir/web/apps/tv/android/app/.cxx/Release/hash/arm64-v8a/CMakeFiles/rules.ninja"
lto_probe="$temp_dir/web/apps/tv/android/app/.cxx/Release/hash/arm64-v8a/CMakeFiles/_CMakeLTOTest-CXX/bin/CMakeFiles/rules.ninja"
mkdir -p "$(dirname "$actual_rule")" "$(dirname "$lto_probe")"
printf 'command = %s clang++\n' "$launcher" > "$actual_rule"
printf 'command = clang++\n' > "$lto_probe"

FAKE_CCACHE_STATS='{"cache_miss":1,"direct_cache_hit":0,"preprocessed_cache_hit":0}' \
  "$verifier" "$launcher" "$temp_dir/web" "$temp_dir/launchers.txt"
grep -Fxq "$actual_rule" "$temp_dir/launchers.txt"
if grep -Fq '_CMakeLTOTest' "$temp_dir/launchers.txt"; then
  echo 'ccache evidence included a CMake internal compiler probe' >&2
  exit 1
fi

if FAKE_CCACHE_STATS='{"cache_miss":0,"direct_cache_hit":0,"preprocessed_cache_hit":0}' \
  "$verifier" "$launcher" "$temp_dir/web" >/dev/null 2>&1; then
  echo 'ccache evidence accepted a build with no cacheable compiler calls' >&2
  exit 1
fi

for needle in \
  'CCACHE_BASEDIR' \
  'CCACHE_COMPILERCHECK=content' \
  'CCACHE_MAXSIZE' \
  'CCACHE_SLOPPINESS' \
  'LOOMARR_ANDROID_GRADLE_WORKERS' \
  '--parallel' \
  'must be an absolute path' \
  '--zero-stats' \
  '--print-stats --format=json' \
  'ccache-ninja-launchers.txt' \
  'verify-android-ccache-evidence.sh' \
  'LOOMARR_ANDROID_CCACHE_LAUNCHER'; do
  grep -Fq -- "$needle" "$build"
done

# The build source must retain this literal optional-environment guard.
# shellcheck disable=SC2016
grep -Fq 'if [[ -n "${LOOMARR_ANDROID_CCACHE_LAUNCHER:-}" ]]' "$build"
if ANDROID_HOME=/private/tmp LOOMARR_ANDROID_GRADLE_WORKERS=0 "$build" 1.0.0 >/dev/null 2>"$temp_dir/gradle-workers-zero.err"; then
  echo 'build wrapper accepted zero Gradle workers' >&2
  exit 1
fi
grep -Fq 'LOOMARR_ANDROID_GRADLE_WORKERS must be 1 or 2' "$temp_dir/gradle-workers-zero.err"
if ANDROID_HOME=/private/tmp LOOMARR_ANDROID_GRADLE_WORKERS=3 "$build" 1.0.0 >/dev/null 2>"$temp_dir/gradle-workers-three.err"; then
  echo 'build wrapper accepted more than two Gradle workers' >&2
  exit 1
fi
grep -Fq 'LOOMARR_ANDROID_GRADLE_WORKERS must be 1 or 2' "$temp_dir/gradle-workers-three.err"
if ANDROID_HOME=/private/tmp LOOMARR_ANDROID_CCACHE_LAUNCHER=relative-ccache "$build" 1.0.0 >/dev/null 2>&1; then
  echo 'build wrapper accepted a relative ccache launcher' >&2
  exit 1
fi
