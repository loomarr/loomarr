#!/usr/bin/env bash
# SessionStart hook — name the other Claude sessions live on this machine.
#
# WHY THIS EXISTS (2026-08-10): two sessions on this repo independently regenerated the
# same ten visual baselines for PR #226, and one had already fixed #236 before the other
# started on it. Both were reachable by `/list-agents` the entire time. The capability was
# never missing; the prompt to use it was. So a session now opens knowing who else is here.
#
# ⚠ THIS SCRIPT MUST NEVER FAIL A SESSION START. It reads `~/.claude/sessions/*.json`,
# which is INTERNAL state, not a documented interface — the supported entry point is the
# `/list-agents` command. If that directory moves, changes shape, or `jq` is missing, this
# still prints the reminder and exits 0. A coordination aid that breaks session startup is
# strictly worse than no aid, and it would break it for every session at once.
set -uo pipefail

reg="$HOME/.claude/sessions"

# ⚠ IDENTIFY SELF BY SOCKET, NOT BY PID. `$PPID` is whatever process invoked this script —
# the hook runner or an interactive shell — not the Claude session, so filtering on it
# listed THIS session as its own peer. Caught by running the script, not by reading it.
# `CLAUDE_CODE_MESSAGING_SOCKET` is exported to every hook and Bash command and matches the
# `messagingSocketPath` in the session's own registry record, so the two always agree.
self_sock="${CLAUDE_CODE_MESSAGING_SOCKET:-}"

peers=""
if [ -d "$reg" ] && command -v jq >/dev/null 2>&1; then
  for f in "$reg"/*.json; do
    [ -e "$f" ] || continue
    pid="$(basename "$f" .json)"
    sock="$(jq -r '.messagingSocketPath // empty' "$f" 2>/dev/null)" || continue
    # Skip our own record.
    [ -n "$self_sock" ] && [ "$sock" = "$self_sock" ] && continue
    # Skip records whose process is gone: the registry outlives a crashed session, and
    # naming a dead peer sends people chasing something that cannot answer.
    kill -0 "$pid" 2>/dev/null || continue
    row="$(jq -r '"  \(.name // "unnamed")  ·  \(.cwd // "?")"' "$f" 2>/dev/null)" || continue
    [ -n "$row" ] && peers="${peers}${row}"$'\n'
  done
fi

# The backticks below are literal markdown around a slash command, so single quotes are
# correct and the "expressions don't expand" advice does not apply.
# shellcheck disable=SC2016
if [ -n "$peers" ]; then
  printf 'Other Claude sessions live on this machine:\n%s' "$peers"
  printf '⚠ Before claiming a PR, branch or shared file, check `/list-agents` and SendMessage\n'
  printf '  to say what you are taking. Two sessions duplicated a full baseline regeneration\n'
  printf '  on 2026-08-10 because neither looked. Address peers by the name AND [ref] the\n'
  printf '  listing shows — a bare name is rejected when a ref is displayed.\n'
else
  # Silent-but-for-the-reminder: no peers found, or the registry was unreadable. Both look
  # the same on purpose, because the reminder is correct either way.
  printf 'No other Claude sessions detected. Run `/list-agents` before claiming shared work.\n'
fi
exit 0
