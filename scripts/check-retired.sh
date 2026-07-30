#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
RETIRED=(
  'hooks/arr|the inbound arr webhook was deleted; acquisition state comes from polling'
  'WEBHOOK_SECRET|never existed as a generated secret; only session_secret and api_token do'
  'capture-collections.sh|deleted; running the app against a real Emby answered every question it existed to ask (design §6 records the findings)'
)
ALLOW_PATH='^(PROGRESS\.md|docs/engineering/|scripts/check-retired\.sh)'
ALLOW_LINE='[Rr]etired|[Ss]uperseded|no longer exists|was deleted|used to'
SEARCH=(docs web/apps/web/src README.md CLAUDE.md)
fail=0
for row in "${RETIRED[@]}"; do
  id="${row%%|*}"; why="${row#*|}"
  hits="$(grep -rInF "$id" "${SEARCH[@]}" 2>/dev/null | grep -Ev "$ALLOW_PATH" | grep -Ev "$ALLOW_LINE" || true)"
  if [[ -n "$hits" ]]; then
    fail=1
    printf '\nRETIRED IDENTIFIER STILL REFERENCED: %s\n  %s\n\n' "$id" "$why"
    printf '%s\n' "$hits" | sed 's/^/    /'
  fi
done
[[ "$fail" -ne 0 ]] && exit 1
printf 'retired-verify: clean (%d identifiers checked)\n' "${#RETIRED[@]}"
