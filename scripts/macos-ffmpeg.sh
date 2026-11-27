#!/usr/bin/env sh
# Install and check the digest-pinned static arm64 FFmpeg for macOS: `make bootstrap` puts it in the
# repo-local .tools/ffmpeg/bin, `make doctor` checks it, and the macOS playout bench installs it into
# a runner directory.
#
# It is FFmpeg 9.0.2, the release production pins (Dockerfile FFMPEG_BUILD_ID), built by a different
# builder (martin-riedl). It has VideoToolbox and drawtext; Homebrew's bottle has no drawtext.
# A rebuilt upstream asset fails the checksum instead of silently changing what runs.
#
#   scripts/macos-ffmpeg.sh BIN_DIR           install (a no-op when BIN_DIR already holds the pins)
#   scripts/macos-ffmpeg.sh --check BIN_DIR   print what is wrong with BIN_DIR and exit 1, or exit 0
set -eu

version=9.0.2
base=https://ffmpeg.martin-riedl.de/download/macos/arm64/1789931890_9.0.2
ffmpeg_sha256=c8ed4c4e6978a03c485edbfe4e0a5dc2380f8a30bba5150531b31b094492d924
ffprobe_sha256=fcbe839537485eaee7a7a8bc5cbc0f90d53617e80943e8a5b2e31cb851197ea6

check() {
	for tool in ffmpeg ffprobe; do
		[ -x "$1/$tool" ] || { echo "$tool is missing from $1"; return 1; }
		# The build's version carries a builder suffix, as in "9.0.2-https://...".
		found="$("$1/$tool" -version 2>/dev/null | sed -n "1s/^$tool version \([^ -]*\).*/\1/p")"
		[ "$found" = "$version" ] || { echo "$1/$tool is version ${found:-unknown}, not $version"; return 1; }
	done
	"$1/ffmpeg" -hide_banner -filters 2>/dev/null | grep -w drawtext >/dev/null ||
		{ echo "$1/ffmpeg has no drawtext filter"; return 1; }
}

mode=install
if [ "${1:-}" = --check ]; then
	mode=check
	shift
fi
[ $# -eq 1 ] || { echo "usage: $0 [--check] BIN_DIR" >&2; exit 2; }
bin_dir="$1"

if [ "$mode" = check ]; then
	check "$bin_dir"
	exit
fi

mkdir -p "$bin_dir"
pins="$base $ffmpeg_sha256 $ffprobe_sha256"
if [ "$(cat "$bin_dir/.pins" 2>/dev/null)" = "$pins" ] && check "$bin_dir" >/dev/null; then
	echo "macos-ffmpeg: $version already installed in $bin_dir"
else
	rm -f "$bin_dir/.pins"
	for pin in "ffmpeg:$ffmpeg_sha256" "ffprobe:$ffprobe_sha256"; do
		tool="${pin%%:*}"
		digest="${pin#*:}"
		curl --fail --location --retry 3 --connect-timeout 20 --max-time 180 \
			--output "$bin_dir/$tool.zip" "$base/$tool.zip"
		printf '%s  %s\n' "$digest" "$bin_dir/$tool.zip" | shasum -a 256 -c -
		unzip -oq "$bin_dir/$tool.zip" -d "$bin_dir"
		rm -f "$bin_dir/$tool.zip"
		chmod +x "$bin_dir/$tool"
	done
	check "$bin_dir"
	echo "$pins" > "$bin_dir/.pins"
fi
[ -z "${GITHUB_PATH:-}" ] || echo "$bin_dir" >> "$GITHUB_PATH"
