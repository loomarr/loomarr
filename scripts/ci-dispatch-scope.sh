#!/usr/bin/env bash

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mode="${2:-legacy}"

if [[ $# -lt 1 || $# -gt 2 || ("$mode" != legacy && "$mode" != impact) ]]; then
	echo 'usage: ci-dispatch-scope.sh <release-candidate|full|apple-cache-validation> [legacy|impact]' >&2
	exit 2
fi

case "$1" in
	release-candidate)
		if [[ "$mode" == impact ]]; then
			"$root/scripts/ci-impact.sh" </dev/null | awk -F= 'BEGIN { OFS = "=" }
				$1 == "contracts" || $1 == "rust" || $1 == "image" { $2 = "true" }
				{ print }'
			exit 0
		fi
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
		if [[ "$mode" == impact ]]; then
			"$root/scripts/ci-impact.sh" --all
			exit 0
		fi
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
	apple-cache-validation)
		if [[ "$mode" == impact ]]; then
			"$root/scripts/ci-impact.sh" </dev/null | awk -F= 'BEGIN { OFS = "=" }
				$1 == "policy" { $2 = "true" }
				{ print }'
			exit 0
		fi
		cat <<'OUTPUT'
go=false
web=false
clients=false
image=false
docs=false
agent=false
android=false
release_candidate=false
OUTPUT
		;;
	*)
		printf 'unsupported manual CI scope: %s\n' "$1" >&2
		exit 2
		;;
esac
