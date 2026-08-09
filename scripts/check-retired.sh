#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
RETIRED=(
  # §10 V51b — four per-capability sweeps became one ingest pipeline. Their schedule keys are the
  # dangerous half: `docs/help/` ships inside the binary and is read as INSTRUCTIONS, so a page
  # telling an operator to tune `JOB_FILLER_VISION_SCHEDULE` sends them to set an env var nothing
  # reads, on a box where vision is now scheduled by `job.filler_pipeline.schedule`. That is the
  # exact shape the deleted `/hooks/arr` webhook left behind.
  'job.filler_language.schedule|V51b: the language gate is a rung of the ingest pipeline; use job.filler_pipeline.schedule'
  'job.filler_split.schedule|V51b: splitting is a rung of the ingest pipeline; use job.filler_pipeline.schedule'
  'job.filler_transcribe.schedule|V51b: transcription is a rung of the ingest pipeline; use job.filler_pipeline.schedule'
  'job.filler_vision.schedule|V51b: vision is a rung of the ingest pipeline; use job.filler_pipeline.schedule'
  'JOB_FILLER_LANGUAGE_SCHEDULE|V51b: replaced by JOB_FILLER_PIPELINE_SCHEDULE'
  'JOB_FILLER_SPLIT_SCHEDULE|V51b: replaced by JOB_FILLER_PIPELINE_SCHEDULE'
  'JOB_FILLER_TRANSCRIBE_SCHEDULE|V51b: replaced by JOB_FILLER_PIPELINE_SCHEDULE'
  'JOB_FILLER_VISION_SCHEDULE|V51b: replaced by JOB_FILLER_PIPELINE_SCHEDULE'
  # ⚠ Not a rename: "how often do we go LOOKING for compilations" stopped being a question with
  # an answer, because every long recording reaches the split rung as it is ingested. An operator
  # told to raise this to split more often would be tuning nothing; the real bound is
  # `filler.pipeline.max_splits`.
  'filler.split.every|V51b: splitting is a pipeline rung, not a sweep; the bound is filler.pipeline.max_splits'
  'FILLER_SPLIT_EVERY|V51b: splitting is a pipeline rung, not a sweep; the bound is FILLER_PIPELINE_MAX_SPLITS'
  # V51b folded on-file loudness normalisation into the transcode rung. `NormalizeInPlace` had no
  # production caller at all — the capability existed and the setting that gated it was inert —
  # so a doc describing it as a separate pass describes something that never ran.
  'NormalizeInPlace|V51b: loudness is applied by the transcode rung, in the pass that is already re-encoding'
  # §10 V51e — Incoming became ONE conveyor. `asks` and `pipeline` were separate arrays over
  # overlapping populations, and on a fresh scan 84 of 85 clips appeared in BOTH: a row demanding
  # a decision above a row captioned "nothing here needs you". The names are the dangerous half
  # here for the same reason the schedule keys were — a doc or a comment that still says "the
  # asks list" sends the next reader looking for a field the response does not have, and the
  # honest answer (`clips`, with `needsDecision` per row) is one word away from it.
  'IncomingAskDTO|V51e: one belt, one type — IncomingClipDTO, with needsDecision saying which end a clip is at'
  'NonTerminalOnly|V51e: PipelineFilter.ConveyorOnly returns running AND review — the two halves of one belt'
  'body.Asks|V51e: the response carries `clips`; a clip appears exactly once, whichever end it is at'
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
  # Live TV wiring stopped being an operator ACTION: it is idempotent and fully derived from the
  # Tunarr connection, so it auto-runs on a Connections save (settings.go autoWireAfterSave) and a
  # manual endpoint would be a redundant no-op. The route was deleted; five documents kept telling
  # operators to call it, including the wizard walkthrough and the §7 route table — the exact
  # "docs/help ships as instructions" failure this script exists for. ⚠ NOT the same thing as
  # /v1/setup/livetv-reconnect, which is the force-re-wire for a stale channel→stream binding.
  'setup/livetv-connect|Live TV wiring auto-runs on a Connections save (settings.go autoWireAfterSave); there is no manual route. The force-re-wire is /v1/setup/livetv-reconnect'
  # V50a: the primitive vendor moved Radix → Base UI (design §14). Both are headless React
  # libraries with near-identical part names, so a copy-pasted snippet or a re-added dependency
  # would look ordinary in review while quietly pulling a second vendor back into the tree — which
  # is precisely what the consolidation bought. `asChild` rides along because it is the one API
  # that cannot survive the move: Base UI composes through a `render` PROP, so a prop still named
  # for merging onto a CHILD is the half-migrated vocabulary that outlives whoever reintroduced it.
  '@radix-ui|the primitive vendor is Base UI since V50a (design §14) — import from @base-ui/react'
  'asChild|Radix composition prop; Base UI composes with render={<El />} (design §14, V50a)'
  # V53e: 31 test files each hand-rolled a `stubFetch` that replaced global fetch. Every one was
  # UNTYPED (so a fixture could omit required fields indefinitely) and UNBOUND (so assertions
  # matched a url SUBSTRING the test itself wrote). The migration found ~40 defects across those
  # files, including catch-alls answering 13 real endpoints with `{}` in the suite whose entire
  # job is proving screens render real content. Without this line nothing stops #32.
  #
  # ⚠ THE CARVE-OUT IS THE SEARCH PATH, not an allow-rule: `packages/api/src/mutator/mutator.test.ts`
  # legitimately stubs fetch — it TESTS `customFetch`, asserting on `credentials: "include"` and the
  # CSRF header, neither of which an MSW resolver can observe because MSW intercepts BELOW the layer
  # under test. It survives only because SEARCH covers `web/apps/web/src` and not `web/packages`.
  # Anyone widening SEARCH to `web/` must add an explicit exemption for that file in the same edit,
  # or the guard fails on the one file that is right.
  'vi.stubGlobal("fetch"|V53e: use the shared MSW layer (src/test/msw) — a hand-rolled fetch stub is untyped AND unbound to a route'
  # V51f: three `filler.Policy` fields that were set in TESTS AND NOWHERE ELSE — no settings key,
  # no env var, no policy field, no UI. `EraStrict` is deleted outright (a narrow era range gives
  # a channel strictness through a control an operator can actually see); the duration bounds keep
  # their struct fields but are now wired to real settings, so the OLD names are what must not come
  # back. ⚠ These are listed because the code READ convincingly: `coverage.go`, `fit.go` and
  # `coverage-meter.tsx` all carried special copy for the strict-era branch, and `PoolReport.Eligible`
  # was headlined as "the number that surprises operators" while being arithmetically identical to
  # `Commercials` on every install ever run. Prose could not have caught that; a grep can.
  'EraStrict|deleted in V51f — it was unreachable (tests only). A narrow policy.filler.era range is how a channel gets era strictness'
  'FILLER_MIN_CLIP_SECONDS|the setting is FILLER_MIN_CLIP_DURATION (a duration like 15s), matching the neighbouring FILLER_MIN_DURATION'
  'FILLER_MAX_CLIP_SECONDS|the setting is FILLER_MAX_CLIP_DURATION (a duration like 90s), matching the neighbouring FILLER_MIN_DURATION'
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
#
# ⚠ scripts/ is searched for the same reason, found the same way: latency-sweep.sh had been
# probing the retired /v1/suggestions since V41 — a hand-maintained ROUTE array sitting in the
# one directory the ban could not see, so the sweep silently measured a 404 as if it were an
# endpoint. This file excludes itself via ALLOW_PATH, so the RETIRED array above does not
# self-trip.
SEARCH=(docs internal docker scripts web/apps/web/src README.md CLAUDE.md .env.example)
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
