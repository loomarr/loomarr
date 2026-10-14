#!/usr/bin/env bash
# Print the extra Playwright flags for `make tuner-e2e-host` from TUNER_PROJECT / TUNER_REPEAT_EACH.
#
# The defaults (all projects, one repetition) print nothing, so the merge-queue run is exactly the
# command it always was. Anything else fails closed: a typo must not silently run less than asked.
# Retries stay owned by playwright.tuner.config.ts (0 by design); no flag here can raise them.

set -euo pipefail

project="${TUNER_PROJECT:-all}"
repeat="${TUNER_REPEAT_EACH:-1}"

case "$project" in
	all) ;;
	chromium | firefox | webkit) printf -- '--project=%s\n' "$project" ;;
	*)
		printf 'tuner-args: unsupported project %q (want all, chromium, firefox, or webkit)\n' "$project" >&2
		exit 2
		;;
esac

if [[ ! "$repeat" =~ ^([1-9][0-9]?|100)$ ]]; then
	printf 'tuner-args: repeat_each must be an integer from 1 to 100, got %q\n' "$repeat" >&2
	exit 2
fi
if [[ "$repeat" != 1 ]]; then
	printf -- '--repeat-each=%s\n' "$repeat"
fi
