#!/usr/bin/env bash
# Report Orca's settings for this repository that differ from docs/agents/harness.md#orca. Orca keeps
# them per machine, outside the repository, so nothing else in the tree can check them.
#
# Prints an "Orca:" section with one WARNING per differing setting, or nothing when Orca is not
# installed or does not know this repository. Always exits 0: Orca is optional.

set -u

WATCH_SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)"
# shellcheck source=scripts/dev/watch-lib.sh
. "$WATCH_SCRIPT_DIR/watch-lib.sh"

orca="$(watch_orca_cli)"
command -v "$orca" >/dev/null 2>&1 || exit 0
if ! command -v jq >/dev/null 2>&1; then
	printf 'Orca:\n  NOTE: install jq to check Orca'"'"'s repository settings\n'
	exit 0
fi
primary="$(git -C "$WATCH_SCRIPT_DIR" worktree list --porcelain 2>/dev/null | sed -n 's/^worktree //p' | head -1)"
id="$("$orca" repo list --json 2>/dev/null | jq -r --arg path "$primary" '.result.repos[]? | select(.path == $path) | .id' | head -1)"
[ -n "$id" ] || exit 0
repo="$("$orca" repo show --repo "$id" --json 2>/dev/null | jq -c '.result.repo // empty')"
if [ -z "$repo" ]; then
	printf 'Orca:\n  WARNING: could not read Orca'"'"'s settings for this repository (%s repo show --repo %s)\n' "$orca" "$id"
	exit 0
fi
setting() { jq -r "$1 // \"\"" <<<"$repo"; }

echo 'Orca:'
warned=0
warn() {
	echo "  WARNING: $*"
	warned=1
}
[ "$(setting .worktreeBaseRef)" = origin/main ] ||
	warn "new worktrees branch from the primary checkout's branch; run: $orca repo set-base-ref --repo $id --ref origin/main"
[ "$(setting .hookSettings.setupRunPolicy)" = run-by-default ] ||
	warn "setup does not run by default (setupRunPolicy=$(setting .hookSettings.setupRunPolicy)); set Setup run policy to run-by-default in Repository settings"
[ "$(setting .hookSettings.setupAgentStartupPolicy)" = wait-for-setup ] ||
	warn "agents start before setup finishes (setupAgentStartupPolicy=$(setting .hookSettings.setupAgentStartupPolicy)); set agent startup to wait-for-setup in Repository settings"
[ -z "$(setting .hookSettings.scripts.setup)$(setting .hookSettings.scripts.archive)" ] ||
	warn 'a local setup or archive script replaces orca.yaml; clear it in Repository settings'
[ "$warned" = 1 ] || echo '  settings match docs/agents/harness.md#orca'
exit 0
