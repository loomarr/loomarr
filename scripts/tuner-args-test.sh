#!/usr/bin/env bash

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
args="$root/scripts/tuner-args.sh"

expect() {
	local want="$1" project="$2" repeat="$3" got
	got="$(TUNER_PROJECT="$project" TUNER_REPEAT_EACH="$repeat" "$args" | tr '\n' ' ')"
	[[ "$got" == "$want" ]] || {
		printf 'tuner-args(%s,%s) = %q, want %q\n' "$project" "$repeat" "$got" "$want" >&2
		exit 1
	}
}

reject() {
	if TUNER_PROJECT="$1" TUNER_REPEAT_EACH="$2" "$args" >/dev/null 2>&1; then
		printf 'tuner-args(%s,%s) was accepted but must fail closed\n' "$1" "$2" >&2
		exit 1
	fi
}

# The workflow_call path (no dispatch inputs) and the explicit defaults add no flags at all.
[[ -z "$(env -u TUNER_PROJECT -u TUNER_REPEAT_EACH "$args")" ]] || {
	echo 'unset inputs must add no Playwright flags' >&2
	exit 1
}
expect '' all 1
expect '' '' ''
expect '--project=webkit --repeat-each=20 ' webkit 20
expect '--project=chromium ' chromium 1
expect '--repeat-each=100 ' all 100

reject safari 1
reject 'webkit --retries=3' 1
reject webkit 0
reject webkit 101
reject webkit 2x
reject webkit '20 --retries=3'

echo 'tuner-args-test: ok'
