#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
RETIRED=(
  'hooks/arr|the inbound arr webhook was deleted; acquisition state comes from polling'
  'WEBHOOK_SECRET|never existed as a generated secret; only session_secret and api_token do'
  'capture-collections.sh|deleted; running the app against a real Emby answered every question it existed to ask (design §6 records the findings)'
  # The packaging question §10 says "keeps being re-decided": sidecar → opt-in tag → single
  # image. Both intermediate answers left instructions behind that read as current — a
  # Sources row literally labelled "ingest sidecar", and copy telling operators to switch to
  # an image tag that is not published. Exactly the docs/help failure this script exists for.
  'loomarr-ingest|the ingest sidecar was folded into the core (internal/clipfetch); there is no separate service or image'
  'loomarr:filler|the two-tag split was replaced by the SINGLE image (§16) — yt-dlp/ffmpeg always ship, so telling an operator to switch tags is a dead end'
  # V38b: clips arrive because you added a SOURCE, not because you pasted a URL. The panel was
  # the odd one out once Sources had registration, per-row search, pulls and auto-fetch — and
  # leaving its name in help text would send an operator hunting a box that is not there.
  'IngestPanel|the paste-a-URL box was retired (V38b); clips arrive from a registered source — add one under Filler → Sources'
  # V41: CONTEXT.md defines the artifact as a PROPOSAL and explicitly bans "suggestion", but the
  # routes said /v1/suggestions and one operationId (submit-suggestion) sat among five
  # *-proposal siblings in the same file. The paths moved; this keeps the old ones from coming
  # back in help text an operator would follow to a 404.
  'v1/suggestions|renamed to /v1/proposals (V41) — CONTEXT.md defines the artifact as a Proposal and bans "suggestion"'
  # V47b: renamed the playout "doctor" to "playout status" — same read-only health projection,
  # clearer name. The old operation id and path must not survive in help text an operator would
  # follow to a 404.
  'get-playout-doctor|renamed to get-playout-status — same read-only playout health projection'
  'playout/doctor|renamed to /v1/playout/status — same read-only playout health projection'
  # V48: the playout copy-audience query changed from ?target=browser|mediaserver to
  # ?plan=baseline|hevc8|hevc10|full (a client DeviceProfile resolves to an EncodePlan). The VALUE
  # tokens are the retired identifiers — the bare word "target" survives as the SessionStat/health
  # DTO field by design, so only the `target=browser`/`target=mediaserver` query strings are banned.
  'target=browser|the playout copy-audience query is now ?plan= (V48); browser → ?plan=baseline'
  'target=mediaserver|the playout copy-audience query is now ?plan= (V48); mediaserver → ?plan=full'
)
ALLOW_PATH='^(PROGRESS\.md|docs/engineering/|scripts/check-retired\.sh|internal/web/dist/)'
# A line may name a retired identifier when it is EXPLAINING that it is retired — that is how
# §10's "keeps being re-decided" history survives, and how a corrective comment ("this used to
# say X") points at the thing it corrects.
#
# ⚠ Two mechanisms, and the difference matters. The PHRASES below are inferred intent, and
# chasing them is a losing game: every rewording needs a new phrase, and each one loosens the
# check for everybody. `retired-ok` is the EXPLICIT opt-out — put it on the line when the
# mention is deliberate. Prefer it; a reader can see the claim, and it cannot be tripped by
# accident the way "no longer" can.
ALLOW_LINE='retired-ok|[Rr]etired|[Ss]uperseded|no longer exist|was deleted|was removed|used to|does not exist|removed because|keeps being re-decided'
# ⚠ internal/ and docker/ are searched too. They were not before, which is how a Go doc
# comment could keep describing "the sidecar's OWN configuration" — and how a dead
# LoadConfig reading five env vars absent from §15 survived as apparently-live architecture.
SEARCH=(docs internal docker web/apps/web/src README.md CLAUDE.md .env.example)
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
