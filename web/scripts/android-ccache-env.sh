# shellcheck shell=bash
# Shared ccache setup for the Android TV bundle and the Android client builds. Source it, then call
# configure_android_ccache REPO_ROOT. It sets CCACHE_LAUNCHER to the validated launcher, or to empty
# when LOOMARR_ANDROID_CCACHE_LAUNCHER is unset; the Expo config plugin
# with-memory-safe-android-build.cjs hands that launcher to every CMake subproject.

configure_android_ccache() {
  local repo_root=$1
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
    CCACHE_BASEDIR=$(realpath "${CCACHE_BASEDIR:-${repo_root}}")
    [[ "${CCACHE_BASEDIR}" == "${repo_root}" ]] || {
      printf 'CCACHE_BASEDIR must be this worktree root for worktree-independent keys\n' >&2
      exit 2
    }
    [[ -z "${CCACHE_SLOPPINESS:-}" ]] || { printf 'CCACHE_SLOPPINESS is forbidden\n' >&2; exit 2; }
    CCACHE_MAXSIZE="${CCACHE_MAXSIZE:-2G}"
    export CCACHE_DIR CCACHE_BASEDIR CCACHE_COMPILERCHECK=content CCACHE_MAXSIZE
    "${CCACHE_LAUNCHER}" --set-config=compiler_check=content
    "${CCACHE_LAUNCHER}" --set-config=max_size="${CCACHE_MAXSIZE}"
    "${CCACHE_LAUNCHER}" --zero-stats
  fi
  readonly CCACHE_LAUNCHER
}
