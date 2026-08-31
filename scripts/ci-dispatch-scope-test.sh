#!/usr/bin/env bash

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
selector="$root/scripts/ci-dispatch-scope.sh"

release_expected=$'go=false\nweb=false\nclients=false\nimage=true\ndocs=false\nagent=false\nandroid=false\nrelease_candidate=true'
full_expected=$'go=true\nweb=true\nclients=true\nimage=true\ndocs=true\nagent=true\nandroid=true\nrelease_candidate=false'
apple_cache_expected=$'go=false\nweb=false\nclients=false\nimage=false\ndocs=false\nagent=false\nandroid=false\nrelease_candidate=false'
release_impact_expected='contracts,rust,image'
full_impact_expected='contracts,go,go_full,rust,postgres,web,clients,apple_mobile,apple_tv,expo_android_mobile,expo_android_tv,visual,e2e,tuner,image,docs,agent,android,policy'
apple_cache_impact_expected='policy'

selected_gates() {
	awk -F= '$2 == "true" { values = values (values ? "," : "") $1 } END { print values }'
}

[[ "$($selector release-candidate)" == "$release_expected" ]] || {
	echo 'release-candidate scope drifted from its minimal assurance set' >&2
	exit 1
}
[[ "$($selector full)" == "$full_expected" ]] || {
	echo 'full scope no longer selects every legacy CI family' >&2
	exit 1
}
[[ "$($selector apple-cache-validation)" == "$apple_cache_expected" ]] || {
	echo 'Apple cache validation scope drifted from its isolated assurance set' >&2
	exit 1
}
[[ "$($selector release-candidate impact | selected_gates)" == "$release_impact_expected" ]] || {
	echo 'release-candidate impact scope drifted from contracts, Rust, and image' >&2
	exit 1
}
[[ "$($selector full impact | selected_gates)" == "$full_impact_expected" ]] || {
	echo 'full impact scope no longer selects every specialized gate' >&2
	exit 1
}
[[ "$($selector apple-cache-validation impact | selected_gates)" == "$apple_cache_impact_expected" ]] || {
	echo 'Apple cache validation scope no longer selects policy only' >&2
	exit 1
}
if "$selector" typo >/dev/null 2>&1; then
	echo 'unknown manual CI scope did not fail closed' >&2
	exit 1
fi

echo 'ci-dispatch-scope-test: ok'
