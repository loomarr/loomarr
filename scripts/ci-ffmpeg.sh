#!/usr/bin/env bash
# Use the exact retained media toolchain shipped by the production image.
set -euo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
mode="${1:-}"
read_pin() {
  local value
  value="$(sed -n "s/^ARG $1=//p" "$repo_root/Dockerfile")"
  [[ -n "$value" && "$value" != *$'\n'* ]] || { echo "missing or duplicate FFmpeg pin: $1" >&2; exit 1; }
  printf '%s' "$value"
}

release="$(read_pin FFMPEG_RELEASE)"
build_id="$(read_pin FFMPEG_BUILD_ID)"
[[ "$release" =~ ^autobuild-[0-9]{4}-[0-9]{2}-[0-9]{2}-[0-9]{2}-[0-9]{2}$ ]] || exit 1
[[ "$build_id" =~ ^n([0-9]+)\.([0-9]+)\.[0-9]+-[0-9]+-g[0-9a-f]+$ ]] || exit 1
series="${BASH_REMATCH[1]}.${BASH_REMATCH[2]}"
release_date="${release:10:10}"
version_id="$build_id-${release_date//-/}"
case "$(uname -m)" in
  x86_64) asset_arch=linux64; digest="$(read_pin FFMPEG_AMD64_SHA256)" ;;
  aarch64|arm64) asset_arch=linuxarm64; digest="$(read_pin FFMPEG_ARM64_SHA256)" ;;
  *) echo "unsupported FFmpeg runner architecture" >&2; exit 1 ;;
esac
[[ "$digest" =~ ^[0-9a-f]{64}$ ]] || exit 1
build="ffmpeg-${build_id}-${asset_arch}-gpl-${series}"
archive="${build}.tar.xz"
if [[ "$mode" == metadata && $# -eq 1 ]]; then
  printf 'sha256=%s\n' "$digest"
  exit 0
fi
[[ ( "$mode" == download && $# -eq 2 ) || ( "$mode" == install && $# -eq 3 ) ]] || {
  echo "usage: $0 metadata | download CACHE_DIR | install CACHE_DIR BIN_DIR" >&2
  exit 2
}
cache_dir="$2"
cached_archive="$cache_dir/$archive"
if [[ "$mode" == download && ! -f "$cached_archive" ]]; then
  mkdir -p "$cache_dir"
  partial="$(mktemp "$cache_dir/.ffmpeg.XXXXXX")"
  trap 'rm -f -- "$partial"' EXIT
  curl --fail --location --retry 3 --connect-timeout 20 --max-time 180 \
    --output "$partial" "https://github.com/BtbN/FFmpeg-Builds/releases/download/$release/$archive"
  actual="$(sha256sum "$partial")"
  [[ "${actual%% *}" == "$digest" ]] || { echo "FFmpeg archive checksum mismatch" >&2; exit 1; }
  mv -- "$partial" "$cached_archive"
fi
# A cache hit is not provenance. Reject corrupt archives before extracting anything.
actual="$(sha256sum "$cached_archive")"
[[ "${actual%% *}" == "$digest" ]] || { echo "FFmpeg archive checksum mismatch" >&2; exit 1; }
if [[ "$mode" == install ]]; then
  bin_dir="$3"
  mkdir -p "$bin_dir"
  tar -xJf "$cached_archive" --strip-components=2 -C "$bin_dir" "$build/bin/ffmpeg" "$build/bin/ffprobe"
  for tool in ffmpeg ffprobe; do
    version="$("$bin_dir/$tool" -version)"
    [[ "$version" == "$tool version $version_id "* ]] || { echo "installed $tool differs from production pin" >&2; exit 1; }
  done
fi
