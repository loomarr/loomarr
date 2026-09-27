#!/bin/sh
# Render every sample diagram with the Loomarr theme, then apply the dark-scheme map.
set -eu
cd "$(dirname "$0")"
mkdir -p out
for src in *.d2; do
	name=$(basename "$src" .d2)
	docker run --rm --network none --user "$(id -u):$(id -g)" -v "$PWD:/work" -w /work \
		terrastruct/d2:v0.7.1 --font-regular fonts/Geist-Regular.ttf --font-bold fonts/Geist-Bold.ttf \
		"$src" "out/$name.svg" 2>&1 | grep -v '^info' || true
done
node system/darken.mjs out/*.svg
for f in out/*.svg; do
	xmllint --noout "$f"
	printf '%s %s\n' "$f" "$(grep -o 'viewBox="[^"]*"' "$f" | head -1)"
done
