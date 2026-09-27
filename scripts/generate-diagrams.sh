#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
source_dir="$root/docs/diagrams"
output_dir="$source_dir/generated"
d2_image=${D2_IMAGE:?D2_IMAGE must name the pinned D2 container image}

mkdir -p "$output_dir"

d2() {
	docker run --rm --network none \
		--user "$(id -u):$(id -g)" \
		--volume "$root:/work" \
		--workdir /work \
		"$d2_image" "$@"
}

# The diagram system (docs/contributing/docs.md): every diagram imports system/theme.d2 and
# renders in Geist with the same ELK spacing, 48 px between layers and 24 px edge clearance.
d2 fmt docs/diagrams/system/theme.d2
for source in "$source_dir"/*.d2; do
	name=$(basename "$source" .d2)
	relative_source="docs/diagrams/$name.d2"
	relative_output="docs/diagrams/generated/$name.svg"

	d2 fmt "$relative_source"
	# Removed first: the log filter below hides D2's exit status, so a failed render must not
	# leave the last good SVG in place to pass the checks.
	rm -f "$root/$relative_output"
	d2 --font-regular docs/diagrams/system/fonts/Geist-Regular.ttf \
		--font-bold docs/diagrams/system/fonts/Geist-Bold.ttf \
		--elk-nodeNodeBetweenLayers 48 --elk-edgeNodeBetweenLayers 24 \
		--elk-padding "[top=56,left=32,bottom=32,right=32]" \
		"$relative_source" "$relative_output" 2>&1 | { grep -v '^info' || true; }
	# D2 can't point `style` at its theme slots, so the sources carry light token values and
	# this step adds the matching dark token for each one.
	node "$source_dir/system/darken.mjs" "$root/$relative_output"
	if ! grep -q 'data-loomarr-dark' "$root/$relative_output"; then
		echo "diagrams: generated SVG has no Loomarr dark map: $relative_output" >&2
		exit 1
	fi

	if ! grep -q 'prefers-color-scheme:dark' "$root/$relative_output"; then
		echo "diagrams: generated SVG has no automatic dark theme: $relative_output" >&2
		exit 1
	fi
	if ! grep -q 'data-d2-version="v0.7.1"' "$root/$relative_output"; then
		echo "diagrams: generated SVG came from an unexpected D2 version: $relative_output" >&2
		exit 1
	fi
done

for output in "$output_dir"/*.svg; do
	name=$(basename "$output" .svg)
	if [ ! -f "$source_dir/$name.d2" ]; then
		echo "diagrams: generated output has no source: docs/diagrams/generated/$name.svg" >&2
		exit 1
	fi
done
