#!/usr/bin/env bash
# Install the digest-pinned static arm64 FFmpeg (VideoToolbox included) the macOS playout bench runs on.
#
# This is FFmpeg 9.0.2, not the production 8.1 pin (scripts/ci-ffmpeg.sh): no older macOS arm64 build is
# published. The bench drives the transcode builders and never the concat advance FFmpeg 9 breaks.
# A rebuilt upstream asset fails the checksum instead of silently moving the numbers.
#
#   scripts/playout-bench-macos-ffmpeg.sh BIN_DIR
set -euo pipefail

[[ $# -eq 1 ]] || { echo "usage: $0 BIN_DIR" >&2; exit 2; }
bin_dir="$1"
base=https://ffmpeg.martin-riedl.de/download/macos/arm64/1789931890_9.0.2
ffmpeg_sha256=c8ed4c4e6978a03c485edbfe4e0a5dc2380f8a30bba5150531b31b094492d924
ffprobe_sha256=fcbe839537485eaee7a7a8bc5cbc0f90d53617e80943e8a5b2e31cb851197ea6

mkdir -p "$bin_dir"
for tool in ffmpeg ffprobe; do
  want="${tool}_sha256"
  curl --fail --location --retry 3 --connect-timeout 20 --max-time 180 \
    --output "$bin_dir/$tool.zip" "$base/$tool.zip"
  echo "${!want}  $bin_dir/$tool.zip" | shasum -a 256 -c -
  unzip -oq "$bin_dir/$tool.zip" -d "$bin_dir"
  rm -f "$bin_dir/$tool.zip"
  chmod +x "$bin_dir/$tool"
done
[[ -z "${GITHUB_PATH:-}" ]] || echo "$bin_dir" >> "$GITHUB_PATH"
