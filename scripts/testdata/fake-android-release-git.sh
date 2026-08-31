#!/usr/bin/env bash

set -euo pipefail

case "$*" in
	'rev-parse HEAD') printf '%s\n' "$FAKE_GIT_HEAD" ;;
	'rev-parse origin/main') printf '%s\n' "${FAKE_GIT_MAIN:-$FAKE_GIT_HEAD}" ;;
	'fetch --no-tags origin main') ;;
	*)
		printf 'fake release git: unsupported invocation: %s\n' "$*" >&2
		exit 2
		;;
esac
