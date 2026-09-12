#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SELECTOR="$ROOT/scripts/go-direct-impact.sh"
module="$(cd "$ROOT" && go list -m)"

got="$($SELECTOR internal/suggest/ground.go)"
[[ "$got" == "$module/internal/suggest" ]] || {
	printf 'go-direct-impact-test: got %s, want only internal/suggest\n' "$got" >&2
	exit 1
}

fixture="$($SELECTOR internal/recommend/testdata/channel-recommendation-snapshots-v2.json)"
[[ "$fixture" == "$module/internal/recommend" ]] || {
	printf 'go-direct-impact-test: fixture selected %s, want internal/recommend\n' "$fixture" >&2
	exit 1
}

stdin="$(printf '%s\n' internal/suggest/ground.go docs/dev/testing.md | "$SELECTOR")"
[[ "$stdin" == "$got" ]] || {
	echo 'go-direct-impact-test: stdin and argument interfaces disagree' >&2
	exit 1
}

echo 'go-direct-impact-test: ok'
