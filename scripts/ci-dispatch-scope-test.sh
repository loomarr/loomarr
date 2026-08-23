#!/usr/bin/env bash

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
selector="$root/scripts/ci-dispatch-scope.sh"

release_expected=$'go=false\nweb=false\nclients=false\nimage=true\ndocs=false\nagent=false\nandroid=false\nrelease_candidate=true'
full_expected=$'go=true\nweb=true\nclients=true\nimage=true\ndocs=true\nagent=true\nandroid=true\nrelease_candidate=false'

[[ "$($selector release-candidate)" == "$release_expected" ]] || {
	echo 'release-candidate scope drifted from its minimal assurance set' >&2
	exit 1
}
[[ "$($selector full)" == "$full_expected" ]] || {
	echo 'full scope no longer selects every legacy CI family' >&2
	exit 1
}
if "$selector" typo >/dev/null 2>&1; then
	echo 'unknown manual CI scope did not fail closed' >&2
	exit 1
fi

echo 'ci-dispatch-scope-test: ok'
