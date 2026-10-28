#!/usr/bin/env bash
# Checks every curated filler source against its live provider (#2035, decision 0045), so a
# collection or channel that was removed or renamed fails here rather than on an install.
#
#   archive.org: the identifier must still be a collection with at least one item (metadata and
#                advanced-search APIs).
#   YouTube:     yt-dlp must list at least one entry of the channel (a flat listing: no media is
#                downloaded). yt-dlp is the version and checksum the image pins in the Dockerfile.
#                YouTube's bot check refuses some datacenter addresses; that is a warning, not a
#                failure, since it says nothing about the list.
#
# Usage: scripts/check-curated-sources.sh [sources.json]
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
list="${1:-$root/internal/filler/curated/sources.json}"

ytdlp="$(command -v yt-dlp || true)"
if [[ -z "$ytdlp" ]]; then
	case "$(uname -m)" in
	x86_64) arch=AMD64 asset=yt-dlp_linux ;;
	aarch64 | arm64) arch=ARM64 asset=yt-dlp_linux_aarch64 ;;
	*) echo "check-curated-sources: no pinned yt-dlp for $(uname -m); put yt-dlp on PATH" >&2; exit 2 ;;
	esac
	version="$(sed -n 's/^ARG YTDLP_VERSION=//p' "$root/Dockerfile")"
	sha="$(sed -n "s/^ARG YTDLP_${arch}_SHA256=//p" "$root/Dockerfile")"
	[[ -n "$version" && -n "$sha" ]] || { echo "check-curated-sources: no yt-dlp pin in the Dockerfile" >&2; exit 2; }
	ytdlp="$(mktemp -d)/yt-dlp"
	curl -fsSL --retry 5 --retry-all-errors -o "$ytdlp" \
		"https://github.com/yt-dlp/yt-dlp/releases/download/${version}/${asset}"
	echo "${sha}  ${ytdlp}" | sha256sum -c - >/dev/null
	chmod +x "$ytdlp"
fi

failed=0
fail() {
	echo "::error::$*"
	failed=1
}

while IFS=$'\t' read -r kind target label; do
	case "$kind" in
	archive)
		if ! meta="$(curl -fsSL --retry 3 --retry-all-errors "https://archive.org/metadata/${target}/metadata")"; then
			fail "archive ${target} (${label}): metadata request failed"
			continue
		fi
		if [[ "$(jq -r '.result.mediatype // empty' <<<"$meta")" != collection ]]; then
			fail "archive ${target} (${label}) is no longer a collection: removed or renamed"
			continue
		fi
		if ! items="$(curl -fsSL --retry 3 --retry-all-errors \
			"https://archive.org/advancedsearch.php?q=collection%3A${target}&rows=0&output=json" |
			jq -er '.response.numFound')"; then
			fail "archive ${target} (${label}): item search failed"
			continue
		fi
		if [[ "$items" -lt 1 ]]; then
			fail "archive ${target} (${label}) holds no items"
			continue
		fi
		echo "ok  archive ${target}: ${items} items"
		;;
	youtube)
		errors="$(mktemp)"
		if ! first="$("$ytdlp" --no-config --flat-playlist --playlist-end 1 --print id "$target" 2>"$errors")" || [[ -z "$first" ]]; then
			if grep -q "confirm you're not a bot" "$errors"; then
				echo "::warning::youtube ${label}: YouTube's bot check refused this runner; not checked"
			else
				fail "youtube ${target} (${label}) could not be listed: $(tail -n 1 "$errors")"
			fi
			rm -f "$errors"
			continue
		fi
		rm -f "$errors"
		echo "ok  youtube ${label}: lists ${first}"
		;;
	*)
		fail "unknown kind ${kind} for ${target}"
		;;
	esac
done < <(jq -r '.sources[] | [.kind, .target, .label] | @tsv' "$list")

exit "$failed"
