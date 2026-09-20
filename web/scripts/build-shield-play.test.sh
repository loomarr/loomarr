#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
build="$script_dir/build-shield-play.sh"

for needle in \
  'CCACHE_BASEDIR' \
  'CCACHE_COMPILERCHECK=content' \
  'CCACHE_MAXSIZE' \
  'CCACHE_SLOPPINESS' \
  'must be an absolute path' \
  '--zero-stats' \
  '--print-stats --format=json' \
  'ccache-ninja-launchers.txt' \
  'LOOMARR_ANDROID_CCACHE_LAUNCHER'; do
  grep -Fq -- "$needle" "$build"
done

# The build source must retain this literal optional-environment guard.
# shellcheck disable=SC2016
grep -Fq 'if [[ -n "${LOOMARR_ANDROID_CCACHE_LAUNCHER:-}" ]]' "$build"
grep -Fq '*/.cxx/*/CMakeFiles/rules.ninja' "$build"
# The build source must retain the web-root expression literally.
# shellcheck disable=SC2016
grep -Fq 'find "${WEB_ROOT}"' "$build"

if ANDROID_HOME=/private/tmp LOOMARR_ANDROID_CCACHE_LAUNCHER=relative-ccache "$build" 1.0.0 >/dev/null 2>&1; then
  echo 'build wrapper accepted a relative ccache launcher' >&2
  exit 1
fi
