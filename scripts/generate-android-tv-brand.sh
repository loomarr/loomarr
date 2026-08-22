#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH='' cd -- "$script_dir/.." && pwd)
font_path=${LOOMARR_GEIST_FONT:-}

if [ -z "$font_path" ]; then
	font_path=$(find "$repo_root/web/node_modules/.pnpm" \
		-path '*/@fontsource-variable/geist/files/geist-latin-wght-normal.woff2' \
		-print -quit 2>/dev/null || true)
fi

if [ -z "$font_path" ] || [ ! -f "$font_path" ]; then
	echo 'generate-android-tv-brand: Geist is unavailable; run make bootstrap first' >&2
	exit 2
fi

command -v magick >/dev/null 2>&1 || {
	echo 'generate-android-tv-brand: ImageMagick is required' >&2
	exit 2
}

output_dir="$repo_root/android/app/src/main/res/drawable-nodpi"
output="$output_dir/banner.png"
mkdir -p "$output_dir"

# This is the compact BrandLockup from the web shell at TV-banner scale: the canonical
# 56:6 signal strip, Geist uppercase wordmark, and 0.12em tracking on the static-950 ground.
magick -size 320x180 'xc:#0B0C0E' \
	-fill '#FFB020' -draw 'rectangle 26,83 35,96' \
	-fill '#F5D90A' -draw 'rectangle 36,83 45,96' \
	-fill '#3DD68C' -draw 'rectangle 46,83 55,96' \
	-fill '#4CC9E8' -draw 'rectangle 56,83 65,96' \
	-fill '#D6409F' -draw 'rectangle 66,83 75,96' \
	-fill '#E5484D' -draw 'rectangle 76,83 85,96' \
	-fill '#8B93A3' -draw 'rectangle 86,83 95,96' \
	-font "$font_path" -pointsize 28 -kerning 3.36 \
	-fill '#F7F8FA' -stroke '#F7F8FA' -strokewidth 0.35 \
	-annotate +113+102 'LOOMARR' -depth 8 "$output"

echo "generated $output"
