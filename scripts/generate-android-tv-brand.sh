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
store_dir="$repo_root/android/store-listing"
mkdir -p "$output_dir"
mkdir -p "$store_dir"
generated_banner=$(mktemp)
trap 'rm -f "$generated_banner"' EXIT

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
	-annotate +113+102 'LOOMARR' -depth 8 "png:$generated_banner"

# ImageMagick may rewrite harmless PNG metadata across host versions. Preserve the tracked bytes
# when the rendered pixels are identical so regenerating store art does not create launcher noise.
if [ -f "$output" ] && magick compare -metric AE "$output" "$generated_banner" null: 2>/dev/null; then
	rm -f "$generated_banner"
else
	mv "$generated_banner" "$output"
fi

# Play listing artwork is the exact launcher lockup rendered natively at each required size, not a
# bitmap upscale or a second logo. Every output is opaque RGB because Play rejects alpha in the TV
# banner and feature graphic.
magick -size 1280x720 'xc:#0B0C0E' \
	-fill '#FFB020' -draw 'rectangle 104,332 140,384' \
	-fill '#F5D90A' -draw 'rectangle 144,332 180,384' \
	-fill '#3DD68C' -draw 'rectangle 184,332 220,384' \
	-fill '#4CC9E8' -draw 'rectangle 224,332 260,384' \
	-fill '#D6409F' -draw 'rectangle 264,332 300,384' \
	-fill '#E5484D' -draw 'rectangle 304,332 340,384' \
	-fill '#8B93A3' -draw 'rectangle 344,332 380,384' \
	-font "$font_path" -pointsize 112 -kerning 13.44 \
	-fill '#F7F8FA' -stroke '#F7F8FA' -strokewidth 1.4 \
	-annotate +452+408 'LOOMARR' -alpha off -type TrueColor -depth 8 \
	"$store_dir/tv-banner-1280x720.png"

magick -size 1024x500 'xc:#0B0C0E' \
	-fill '#FFB020' -draw 'rectangle 110,229 137,268' \
	-fill '#F5D90A' -draw 'rectangle 140,229 167,268' \
	-fill '#3DD68C' -draw 'rectangle 170,229 197,268' \
	-fill '#4CC9E8' -draw 'rectangle 200,229 227,268' \
	-fill '#D6409F' -draw 'rectangle 230,229 257,268' \
	-fill '#E5484D' -draw 'rectangle 260,229 287,268' \
	-fill '#8B93A3' -draw 'rectangle 290,229 317,268' \
	-font "$font_path" -pointsize 84 -kerning 10.08 \
	-fill '#F7F8FA' -stroke '#F7F8FA' -strokewidth 1.05 \
	-annotate +371+286 'LOOMARR' -alpha off -type TrueColor -depth 8 \
	"$store_dir/feature-graphic-1024x500.png"

magick -size 512x512 'xc:#0B0C0E' \
	-fill '#FFB020' -draw 'rectangle 39,245 53,274' \
	-fill '#F5D90A' -draw 'rectangle 54,245 68,274' \
	-fill '#3DD68C' -draw 'rectangle 69,245 83,274' \
	-fill '#4CC9E8' -draw 'rectangle 84,245 98,274' \
	-fill '#D6409F' -draw 'rectangle 99,245 113,274' \
	-fill '#E5484D' -draw 'rectangle 114,245 128,274' \
	-fill '#8B93A3' -draw 'rectangle 129,245 143,274' \
	-font "$font_path" -pointsize 42 -kerning 5.04 \
	-fill '#F7F8FA' -stroke '#F7F8FA' -strokewidth 0.525 \
	-annotate +170+281 'LOOMARR' -alpha off -type TrueColor -depth 8 \
	"$store_dir/play-icon-512x512.png"

echo "generated $output and Play listing brand assets in $store_dir"
