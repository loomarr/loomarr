#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SELECTOR="$ROOT/scripts/go-direct-impact.sh"
module="$(cd "$ROOT" && go list -m)"

got="$($SELECTOR internal/proposals/suggest/ground.go)"
[[ "$got" == "$module/internal/proposals/suggest" ]] || {
	printf 'go-direct-impact-test: got %s, want only internal/proposals/suggest\n' "$got" >&2
	exit 1
}

fixture="$($SELECTOR internal/channels/recommend/testdata/channel-recommendation-snapshots-v2.json)"
[[ "$fixture" == "$module/internal/channels/recommend" ]] || {
	printf 'go-direct-impact-test: fixture selected %s, want internal/channels/recommend\n' "$fixture" >&2
	exit 1
}

stdin="$(printf '%s\n' internal/proposals/suggest/ground.go docs/contributing/testing.md | "$SELECTOR")"
[[ "$stdin" == "$got" ]] || {
	echo 'go-direct-impact-test: stdin and argument interfaces disagree' >&2
	exit 1
}

echo 'go-direct-impact-test: ok'
