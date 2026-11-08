#!/usr/bin/env bash
# Mirror the open demo pack's pinned files onto a loomarr/loomarr release, each under its SHA-256,
# so a demo deploy downloads from GitHub instead of the files' own hosts (#2268).
#
#   scripts/mirror-demo-pack.sh <release tag>      e.g. demo-pack-v1
#
# The manifest's "mirror" names that release's download URL. Each file is fetched from its own URL
# and checked against its pin before upload; files already on the release are skipped, so a rerun
# finishes an interrupted mirror. A new pack version gets a new release.

set -euo pipefail

TAG="${1:-}"
REPOSITORY="${GITHUB_REPOSITORY:-loomarr/loomarr}"
MANIFEST="$(dirname "$0")/../internal/demolibrary/openpack.json"
[[ "$TAG" =~ ^demo-pack-v[0-9]+$ ]] || {
	echo 'usage: mirror-demo-pack.sh demo-pack-v<N>' >&2
	exit 2
}

if ! gh release view "$TAG" --repo "$REPOSITORY" >/dev/null 2>&1; then
	gh release create "$TAG" --repo "$REPOSITORY" --prerelease --title "Demo content pack mirror ($TAG)" \
		--notes "Openly licensed films, episodes and ads for the demo site, each named by its SHA-256. Licences and credits: docs/demo-content-credits.md. Not a Loomarr release."
fi
have="$(gh release view "$TAG" --repo "$REPOSITORY" --json assets --jq '.assets[].name')"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/demo-pack-mirror.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

jq -r '[.. | objects | select(has("sha256") and has("url"))] | unique_by(.sha256)[] | [.sha256, .size, .url] | @tsv' "$MANIFEST" |
	while IFS=$'\t' read -r sha size url; do
		if grep -Fxq "$sha" <<<"$have"; then
			echo "mirror-demo-pack: already there: $sha"
			continue
		fi
		curl -fsSL --retry 5 --retry-all-errors -o "$WORK/$sha" "$url"
		got_size="$(stat -c %s "$WORK/$sha")"
		got_sha="$(sha256sum "$WORK/$sha" | cut -d' ' -f1)"
		if [ "$got_size" != "$size" ] || [ "$got_sha" != "$sha" ]; then
			echo "mirror-demo-pack: $url does not match its pin ($got_size bytes, $got_sha)" >&2
			exit 1
		fi
		gh release upload "$TAG" --repo "$REPOSITORY" "$WORK/$sha"
		rm -f "$WORK/$sha"
		echo "mirror-demo-pack: mirrored $sha ($size bytes)"
	done
echo "mirror-demo-pack: https://github.com/$REPOSITORY/releases/download/$TAG/ holds every pinned file"
