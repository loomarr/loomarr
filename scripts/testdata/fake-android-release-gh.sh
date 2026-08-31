#!/usr/bin/env bash

set -euo pipefail

case "$*" in
	*'/actions/workflows/ci.yml/runs?'*) printf '123\tsuccess\n' ;;
	*'/actions/runs/123/jobs?'*) printf '%b' "$FAKE_GITHUB_JOBS" ;;
	*)
		printf 'fake release gh: unsupported invocation: %s\n' "$*" >&2
		exit 2
		;;
esac
