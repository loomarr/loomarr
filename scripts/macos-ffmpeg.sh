#!/usr/bin/env sh
# Install and check the digest-pinned static arm64 FFmpeg for macOS: `make bootstrap` puts it in the
# repo-local .tools/ffmpeg/bin, `make doctor` checks it, and the macOS playout bench installs it into
# a runner directory.
#
# It is FFmpeg 9.0.2, the release production pins (Dockerfile FFMPEG_BUILD_ID), built by a different
# builder (martin-riedl). It has VideoToolbox and drawtext; Homebrew's bottle has no drawtext.
# The pins are in macos-ffmpeg.pins. A rebuilt upstream asset fails the checksum instead of silently
# changing what runs.
#
#   scripts/macos-ffmpeg.sh BIN_DIR           install (a no-op when BIN_DIR already holds the pins)
#   scripts/macos-ffmpeg.sh --check BIN_DIR   print what is wrong with BIN_DIR and exit 1, or exit 0
set -eu

version=9.0.2
pins="$(grep -v '^#' "$(dirname "$0")/macos-ffmpeg.pins")"
read -r base ffmpeg_sha256 ffprobe_sha256 <<EOF
$pins
EOF

check_build() {
	for tool in ffmpeg ffprobe; do
		[ -x "$1/$tool" ] || { echo "$tool is missing from $1"; return 1; }
		# The build's version carries a builder suffix, as in "9.0.2-https://...".
		found="$("$1/$tool" -version 2>/dev/null | sed -n "1s/^$tool version \([^ -]*\).*/\1/p")"
		[ "$found" = "$version" ] || { echo "$1/$tool is version ${found:-unknown}, not $version"; return 1; }
	done
	"$1/ffmpeg" -hide_banner -filters 2>/dev/null | grep -w drawtext >/dev/null ||
		{ echo "$1/ffmpeg has no drawtext filter"; return 1; }
}

# An installed build also records the pins it came from.
check_installed() {
	check_build "$1" || return 1
	[ "$(cat "$1/.pins" 2>/dev/null)" = "$pins" ] || { echo "$1 was not installed from the current pins"; return 1; }
}

mode=install
if [ "${1:-}" = --check ]; then
	mode=check
	shift
fi
[ $# -eq 1 ] || { echo "usage: $0 [--check] BIN_DIR" >&2; exit 2; }
bin_dir="$1"

if [ "$mode" = check ]; then
	check_installed "$bin_dir"
	exit
fi

mkdir -p "$bin_dir"
if check_installed "$bin_dir" >/dev/null; then
	echo "macos-ffmpeg: $version already installed in $bin_dir"
else
	# Everything is downloaded and checked in a staging directory, so a failure leaves the installed
	# build as it was. It sits under BIN_DIR to make each move a rename.
	stage="$(mktemp -d "$bin_dir/.stage.XXXXXX")"
	trap 'rm -rf "$stage"' EXIT
	trap 'exit 1' INT TERM
	for pin in "ffmpeg:$ffmpeg_sha256" "ffprobe:$ffprobe_sha256"; do
		tool="${pin%%:*}"
		digest="${pin#*:}"
		curl --fail --location --retry 3 --connect-timeout 20 --max-time 180 \
			--output "$stage/$tool.zip" "$base/$tool.zip"
		printf '%s  %s\n' "$digest" "$stage/$tool.zip" | shasum -a 256 -c -
		unzip -oq "$stage/$tool.zip" -d "$stage"
		chmod +x "$stage/$tool"
	done
	check_build "$stage"
	# Without its record the directory is not accepted, so an interruption between the moves cannot
	# pass for a build.
	rm -f "$bin_dir/.pins"
	mv -f "$stage/ffmpeg" "$stage/ffprobe" "$bin_dir/"
	printf '%s\n' "$pins" >"$bin_dir/.pins"
fi
[ -z "${GITHUB_PATH:-}" ] || echo "$bin_dir" >>"$GITHUB_PATH"
