#!/usr/bin/env bash

set -euo pipefail

if [[ $# -ne 1 ]]; then
	echo 'usage: ci-dispatch-scope.sh <release-candidate|full>' >&2
	exit 2
fi

case "$1" in
	release-candidate)
		cat <<'OUTPUT'
go=false
web=false
clients=false
image=true
docs=false
agent=false
android=false
release_candidate=true
OUTPUT
		;;
	full)
		cat <<'OUTPUT'
go=true
web=true
clients=true
image=true
docs=true
agent=true
android=true
release_candidate=false
OUTPUT
		;;
	*)
		printf 'unsupported manual CI scope: %s\n' "$1" >&2
		exit 2
		;;
esac
