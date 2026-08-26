#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
source_dir="$root/docs/diagrams"
output_dir="$source_dir/generated"
d2_image=${D2_IMAGE:?D2_IMAGE must name the pinned D2 container image}

mkdir -p "$output_dir"

for source in "$source_dir"/*.d2; do
	name=$(basename "$source" .d2)
	relative_source="docs/diagrams/$name.d2"
	relative_output="docs/diagrams/generated/$name.svg"

	docker run --rm --network none \
		--user "$(id -u):$(id -g)" \
		--volume "$root:/work" \
		--workdir /work \
		"$d2_image" fmt "$relative_source"
	docker run --rm --network none \
		--user "$(id -u):$(id -g)" \
		--volume "$root:/work" \
		--workdir /work \
		"$d2_image" "$relative_source" "$relative_output"

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
