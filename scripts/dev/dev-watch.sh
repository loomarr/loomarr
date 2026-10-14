#!/usr/bin/env bash
# Run every dev watcher that applies on this machine in the foreground until Ctrl-C (make dev-watch).
# Each prints only when something needs attention, prefixed with its name; silence means all is well.
#   resources  always
#   prs        when gh is installed and signed in
#   lanes      when the orca CLI is installed
# See docs/contributing/dev-watchers.md for thresholds and overrides.

set -u

DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)"
# shellcheck source=scripts/dev/watch-lib.sh
. "$DIR/watch-lib.sh"
pids=''
watching='resources'

start() {
	"$DIR/$1" &
	pids="$pids $!"
}

stop_all() {
	# shellcheck disable=SC2086 # One word per pid.
	[ -z "$pids" ] || kill $pids 2>/dev/null
}
trap stop_all EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

start watch-resources.sh
if command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
	start watch-prs.sh
	watching="$watching, prs"
else
	echo 'dev-watch: gh is not installed or not signed in; the PR watcher is off'
fi
if command -v "$(watch_orca_cli)" >/dev/null 2>&1; then
	start watch-lanes.sh
	watching="$watching, lanes"
fi
echo "dev-watch: watching $watching; quiet means all is well (Ctrl-C stops)"
wait
