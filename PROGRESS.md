# PROGRESS.md — Loomarr build tracker

One row per phase (design doc §21). A phase is **done** only when its gate (a set of
tests) is green and the evidence — commit SHA + the exact test command that proves it —
is recorded here. See `CLAUDE.md` for the prime directives; one phase per session/PR.

**Curation-rule-engine arc — self-updating channels / re-curation (Phase B, 2026-07-24).** The
second half of the rotation/re-curation plan (`.claude/plans/curation-rotation-and-recuration.md`):
a channel built from an intent no longer freezes — an **opted-in** channel periodically
re-evaluates its intent against the current library and evolves its lineup, **preferring
in-library matches, weighting net-new acquisitions by quality + intent, and never bypassing the
approval gate** (programming-design §8.2). Composed from existing parts: **B0** extracted a
`ChannelBinder` (`internal/binder`) — the "materialize approved proposal → channel" logic — out of
the API behind an interface the composition root wires ONCE, so manual-approve AND every
auto-approve path bind identically (also **closed a latent gap**: a per-user auto-approved refine
enqueued acquisitions but never rebound its channel). **Per-channel opt-in** = `policy.autoCurate`
(rides `policy_json`, NO migration — like rules/filler/window) with optional MinScorePct/MaxTitles
overrides; **global knobs** `job.recurate.schedule` (weekly), `recurate.min_score_pct` (60),
`recurate.max_titles` (40). **`recurate.Curator`** = the channel-scoped auto-curate grant: filters a
re-curation proposal's net-new acquisitions to those clearing the quality bar within the cap
(in-library adds free), then approves through the ONE `suggest.Approve` gate (audit "auto-curate")
— **never a raw `wanted` write**, fails closed. **`recurate.Runner`** + the `channel-recurate` job
iterate eligible channels (live+intent-backed+auto-curate; skips paused/detached/hand-made/
not-opted-in) → trigger a refresh refine → the worker's new `ChannelAutoCurator` considers it.
Tests: quality bar, in-library-no-acquisition, **approval-gate negative** (not-opted-in never
requests), title cap, per-channel override, runner eligibility, + the B0 fix. **Live-verified** on
the homelab: opted the 1980s Action Heroes channel in, ran the job → refine → Curator approved
(audit "auto-curate") → bound; **enqueued=0** (library already complete — the correct idempotent,
conservative-spend outcome), **zero stray wanted titles** (gate honored). Doc-first §8.2 +
config-design + design §8. Gate: `make check` (`-race`) + `test-pg` + `openapi-verify` +
`config-docs` + FE `tsc` all GREEN. **The rotation/re-curation plan is now COMPLETE** (Phase A
rotation + Phase B re-curation both shipped). Merged on green-local.

**Curation-rule-engine arc — audience ceiling is a kids/teen guardrail (2026-07-24).** The
"1980s Action Heroes" channel came out capped at **TV-14**, silently dropping its 9 R-rated
films (Die Hard, Predator, Terminator…) — a small model reflexively caps genre channels, and
`groundPolicy` kept any proposed ceiling unconditionally. **Rule (maintainer):** the ceiling
exists ONLY so a channel a user asked to be *for kids/teens* can never show adult content; an
unqualified channel is **adult-default**. **Fix:** `groundPolicy(…, intent)` now keeps a
proposed ceiling ONLY when `intentSignalsKids` matches the intent text (kids/family/cartoons/
Bluey/Saturday-morning/teen/… across description/tone/era/refine/must-include); **no signal →
the ceiling is dropped** (everything admitted). Safety asymmetry absolute: dropping an
unjustified ceiling only *loosens*; when a kids signal IS present the ceiling stays + is
enforced fail-closed, and the raise-to-admit-picks (generalized from TV-G→TV-PG to the whole
kids band) **never crosses the kids→adult line** (`KidsCeilingRank`) — a stray R pick on a kids
channel is dropped by §4, never admitted. Prompt reinforced to omit the ceiling for non-kids
channels. Tests: no-kids→drop, 6 kids phrasings→keep, raise-never-crosses-kids-line, existing
raise/never-lower + grounding fuzz still green. Doc-first §4/§8. **Live-verified:** "90s action
heroes movies" → ceiling None; "cartoons for little kids" → TV-Y7. Gate: `make check` GREEN
(`-race`); `openapi-verify` + `config-docs` no drift. Merged on green-local.

**Curation-rule-engine arc — window rotation fix (2026-07-24).** A movie channel whose
library exceeds its window (the maintainer's "1980s Action Heroes", 15 films ≈30h, 24h window)
**repeated the same subset daily and never aired the tail**. Root cause: `truncateToWindow` kept
the deck HEAD (`slots[:kept]`); the window index advanced the shuffle SEED (order) but not the
slice OFFSET, so `sequential`/`syndication` channels (stable order) looped the same prefix.
**Fix:** `windowSlice` — a ROTATING ~window-of-runtime slice whose start advances with the window
index and WRAPS the catalog (window 1 continues where window 0 left off — TILES, not slides), so
over a full cycle every program airs (coverage invariant). Deck order is now seed-stable per
channel; rotation lives in the offset. Runs on the COLLAPSED deck (franchise/two-parter never
split by the seam); idempotent within a window (no re-push). New tests: rotation coverage (the
exact defect reproduction — 15/15 films air over a cycle), tiling-vs-sliding, idempotency,
franchise-never-split-by-seam across all offsets. **Live-verified** on `ch_2986070a483d5cb0`:
with an eligible pool > window, the aired set rotates and all 15 films air. **Also surfaced (not
a bug):** the channel's **TV-14 ceiling excluded its 9 R-rated films** (Die Hard, Predator,
Terminator, …) — the §4 audience filter working as designed — which is why it looked worse (only
6 PG/PG-13 films, < one window). Maintainer chose to raise this channel to R. Gate: `make check`
GREEN (`-race`); `openapi-verify` + `config-docs` no drift. This is the **catalog-rotation half of
the rotation/re-curation plan** (`.claude/plans/curation-rotation-and-recuration.md`); **next: the
scheduled re-curation job (Phase B, per-channel auto-curate opt-in decided)**. Merged on
green-local (CI still billing-blocked).

**Curation-rule-engine arc — Phase 4: "Programming rules" editor + time-travel preview (2026-07-24).**
The authoring + legibility layer for the wall-clock curation engine (Phases 1–3 already
shipped the deterministic core, seasonal-as-a-rule, and LLM preset authoring — commits
`2f39787`/`f0d01a9`/`22ec659`). Two halves, doc-first (`docs/programming-design.md` §8.1 written
before code): **(BE)** a read-only **`GET /v1/channels/{id}/cycle?at=<rfc3339>`** cycle preview
that runs the SAME pure `ComputeDesiredAt` reconcile runs (one code path — preview can't drift)
at a chosen wall-clock, returning the resolved slots + **which curation rule is active then** +
the resolved rolling-window horizon; makes first-match-by-priority legible ("at Saturday 9am the
Weekend marathon rule wins"). New `schedule.ActiveRuleAt`/`SchedulingRule.Label`/`Describe`
(attribution derived from the SAME `pickRule` the engine uses; suggester now names LLM-authored
rules), `schedule.ResolveWindow`, `channels.Engine.CyclePreview` (read-only — heals/pushes
nothing), and the `ChannelService.CyclePreview` interface method (returns `schedule` primitives, so
`api` stays decoupled from `channels`). **(FE)** a `ChannelRulesEditor` (token-based WHEN/WHAT/HOW
picker mirroring `internal/schedule/presets.go`, `@dnd-kit` drag-to-priority where **list order IS
priority**, computed labels) + a `ChannelCyclePreview` (datetime-local + quick presets → the
attribution callout + program/pending/break slot list), both wired into the channel "rules" tab.
**Bug caught in FE review + fixed:** a hand-authored **marathon** wrote `window:"0s"` (Duration 0 =
"inherit the window" ⇒ WOULD truncate the binge) instead of `"-1ns"` (the `schedule.WindowFull`
sentinel = "whole run") — the opposite of intent; fixed + regression-guarded so a hand-authored
marathon is byte-identical to an LLM-authored one (§8.1). Gate: `make check` GREEN (`-race`);
`make openapi` regenerated + committed (`openapi-verify` clean post-commit); `config-docs` no drift;
FE `biome + typecheck + 402 vitest + web build + storybook build` GREEN (new component stories +
tests, `story-coverage` guard). **Next (Phase 5):** custom holiday calendars (the last §9 logged
item). **CI note:** GitHub Actions still billing-blocked — validated locally + merged on green-local
per the maintainer's standing call.

**Scheduler arc — Phase 5: retire the inbound webhook subsystem (2026-07-23).** The final
phase of the poll-availability arc. Availability + download progress now come entirely from
polling (library scan §4, arr queue poll §18.1), so the inbound `POST /hooks/arr` webhook has
no remaining job — deleted. Removed: `internal/ingest` (the whole package), the `/hooks/arr`
mount + `api.Options.Ingest`, the app wiring, `SecretWebhook`/`WEBHOOK_SECRET` + its reveal/
regenerate enums, the `webhook` setup check + `SetupCheck.LastReceived` + `webhook_last_received:`
settings, the `loomarr_webhook_events_total` metric + `WebhookEvent()`, the four `{sonarr,radarr}/
*_webhook.json` fixtures + `FINDINGS-arr-webhooks.md`, the FE wizard **Webhooks** step (+ its
steps/routes/shell registrations + e2e handshake block), and the secrets-panel `webhook_secret`
row. **Deliberately kept** (look-alikes, verified before deleting): filler-clip ingest
(`clipfetch`/`filler.Ingest`/`FeatureIngest`/`INGEST_*`), the outbound `event.webhook_url`
notifier, and **`provision.KeyFromWebhook` + `radarr/import_webhook.json`** (still used by the
channel-lineup key-parity path). The retirement was authorized by the Phase-2 safety proof
`TestScanAvailability_NoWebhook` (green — a `requested` title reaches `available` with no
webhook), never by weakening a gate. Gate: `make check` GREEN (`-race`); `openapi-verify` +
`config-docs` no drift; FE biome + typecheck + **382 vitest** GREEN; visual + e2e baselines
regenerated in the Playwright Docker image (WizardShell + wizard-flow snapshots; webhook-step
baselines removed). **NOTE (CI):** GitHub Actions is billing-blocked on the account (jobs fail
in ~1s with a payments notice) — Phases 4–5 validated locally and merged on green-local per the
maintainer's call; restore billing to re-enable CI.

**Scheduler arc — Phase 1: cron scheduler + 4-loop migration (2026-07-23).** First phase of
the direct-Sonarr/Radarr + poll-availability plan (retiring inbound webhooks, since verified
research shows Overseerr/Seerr is entirely poll-driven). A real job scheduler like Sonarr's
System → Tasks: `internal/scheduler` (code-defined registry + `scheduled_jobs` state table +
a leased due-claim reusing the `ClaimDueTitles` idiom — SQLite guarded UPDATE / Postgres
`FOR UPDATE SKIP LOCKED`). Schedules are **full cron** (6-field, seconds-leading, matching
Overseerr) via a new `KindCron` setting validated by `github.com/adhocore/gronx` (pure-Go,
zero transitive deps — added to design §14 + a new §18.1, doc-first). **All 4 existing loops
(reconcile, channel-sweep, filler-sync, session-sweep) migrated to scheduler jobs** — their
standalone `Run`/`WithInterval`/ticker plumbing deleted (`reconcile/runner.go`, `janitor.go`,
the `channels.Runner` loop, `go runFillerSync`); each is now Run-now-triggerable with an
editable cron from day one. `GET /v1/jobs` + `POST /v1/jobs/{name}/run` (admin-only); timing
is BE-authored and pushed over a new `job` SSE frame (the FE never computes countdowns).
FE: Settings → **Tasks** page (Sonarr-style table + "Run now") and a reusable **Modify Job**
modal — human-readable cron presets by default, an **Advanced** toggle revealing the raw
cron field. Also removed the redundant Live TV wizard step + `POST /v1/setup/livetv-connect`
(auto-wires on Connections save; config-design §6). Gate: `make check` GREEN (`-race`); FE
`biome + typecheck + vitest` GREEN (381 tests, 11 new). **Next (Phase 2):** poll-based
availability — `RecentlyAdded`/`AllItems` library scan jobs driving `LibraryConfirmed`.

**Phase 14 — domain metrics, tranche 3: §17 closed (2026-07-20).** The last non-latency,
non-state counters, each hooked at its subsystem's natural point:
`loomarr_llm_tokens_total{kind}` (prompt/completion, parsed from the provider usage block —
Ollama `prompt_eval_count`/`eval_count`, OpenAI `usage.*`; zero → no-op so a provider that
omits usage adds no phantom sample); `loomarr_filler_pods_total{match_level}` (fallback-ladder
rung, recorded in `BuildFillerList` — the attach path, NOT `Preview`, so UI previews don't
inflate it); `loomarr_channel_slot_substitutions_total` (`programWentStale` → `staleProgramCount`,
now returns the count, recorded in reconcile). **Cost is deliberately NOT a metric** — it's
tokens × a per-model posted rate that drifts and is hosted-specific, so it belongs in a
dashboard recording rule over the token series, not baked into the request path. That closes
the §17 metric list end to end (RED + runtime + state gauges + event counters + latency +
these). Gate: `make check` GREEN.

**Phase 14 — domain metrics, tranche 2: latency (2026-07-20).** Client-side RED for every
outbound dependency via ONE instrumented transport in `httpx.NewNamed` — the six RPC
adapters (library/tunarr/seerr/tmdb/ollama+openai) now build named clients, so
`loomarr_outbound_request_duration_seconds{target}` + `loomarr_outbound_requests_total{target,code}`
cover the §17 library-lookup / Tunarr-API-latency-and-errors / LLM-latency in one series
filtered by target (a transport failure → `code="error"`). Health probes stay on plain
`New` (unlabelled) to keep the metric to the operational RPC path. Plus reconcile timing:
`Engine.Reconcile` (named-return + defer) emits `loomarr_channel_reconcile_duration_seconds`
and `loomarr_channel_reconciles_total{result}`; the injected clock keeps it deterministic (0)
under fixed-time tests. `httpx → metrics` is a new edge but acyclic (metrics imports only
prometheus + provision). **Still deferred** (§17): LLM token/cost (needs provider usage),
filler pod-ladder depth, slot-drift — domain counters, not latencies. Gate: `make check` GREEN.

**Phase 14 — domain metrics, tranche 1 (2026-07-20).** The §17 *domain* series, first
tranche: the state gauges via a pull-based collector (`loomarr_titles{state}`,
`loomarr_jobs{status}`, `loomarr_active_sessions`) + the two cleanest event counters
(`loomarr_auth_logins_total{result}`, `loomarr_webhook_events_total{type}`). The collector
reads three new store count-by-state methods on *scrape* (not on the write path), so no
mutation path is instrumented; `RegisterStoreCollector` wires it once at boot from
`BuildHandler`. The store methods are dialect-neutral (GROUP BY / COUNT) — one impl on
`sqlStore`, covered by a new `ObservabilityCounts` case in the one-suite-two-backends
conformance suite. Webhook + login labels are bounded (unknown eventType → "other"; login
result ∈ success/failure, rate-limits excluded — they're the 429 signal). Scrape-time store
errors degrade to `loomarr_metrics_scrape_errors_total`, never a panic or a stale zero.
**Still deferred** (§17, honest): the latency histograms (reconcile / Tunarr-API / LLM /
library-lookup), LLM token/cost, filler pod-ladder depth, slot-drift — each needs its own
timing wrapper around an external call, a different pattern; a later tranche. Gate: `make
check` GREEN; conformance passes on sqlite (`make test-pg` for Postgres on CI/Docker).

**Phase 14 — docs set, compose audit, metrics foundation (2026-07-20).** The user-facing
help set (Quickstart, Integrations, Concepts, Member, Filler + Troubleshooting keyed to
checklist items), rewritten lean on maintainer feedback, embedded and served at `/v1/docs`
(`a50c57b`, deep-link routing fixed `eb09813`). Seed docs folded into `docs/` (`62d9369`);
README got Documentation + Operations sections (`f46ddf9`).

*Compose-profile audit* (`61e1f9c`): topology matched §16; three satellite docs had drifted
— README + compose header never showed `--profile ai` (yet the default LLM points at the
ollama service it gates), and `.env.example` called filler "a profile" when §16 is explicit
it's the `loomarr:filler` image tag. Fixed the docs; design.md was already correct.

*Metrics* — `internal/metrics` + `GET /metrics` (unauthenticated, §7), `prometheus/client_golang`
(already sanctioned in §14 line 633, so no new-dep conversation). **Scope, honestly (no silent
caps):** wired the RED basics — `loomarr_http_requests_total` / `_request_duration_seconds` /
`_requests_in_flight`, labelled by method + *matched route pattern* (bounded cardinality, not
raw path) — plus the free Go/process runtime collectors. **Deferred:** the §18 *domain* series
(records-by-state, reconcile + Tunarr-API latency, LLM tokens, filler pod-ladder depth, logins,
active sessions, job-queue depth, janitor purges). The pull-based gauges among them need store
count-by-state methods, which touch the one-suite-two-backends conformance gate — a separate
follow-up, not smuggled in here.

**Maintainer smoke — §21's DoD closed end to end (2026-07-20, cont.).** The walkthrough
now runs intent → grounded proposal (real Ollama) → approve → a channel PLAYING in Tunarr,
proven live: an 80-program kids channel from "90s Saturday morning cartoons", built by
Loomarr's own approve→reconcile against the real Emby. Eight steps; `make smoke-livetv`
wires+destroys a disposable Jellyfin for the one media-server-writing action. **Bugs the
smoke found, every one CI-green beforehand — the seams BETWEEN gate-green subsystems:**

1. `GET /v1/users` panic — int setting read through a string-only seam (`0dc957e`).
2. Empty env var destroyed the operator's saved value — `.env.example` ships `LLM_MODEL=`;
   the resolver read present-but-empty as a pin (`be860bc`).
3. FINDING 1 — a fresh install had no way in (`/`→`/login`, no account); added the
   unauthenticated `GET /v1/setup/state` the router guards branch on (`38f8215`).
4. Model selection didn't hot-apply — `persist` bypassed the settings service, so the
   choice vanished on restart (`2128db5`).
5. `make smoke` could never exit — `go run` supervises rather than execs (`e8a956b`).
6. Live TV wiring broken on ALL of Jellyfin — the enumerate GETs are write-only there
   (405); moved to `GET /System/Configuration/livetv`, works on both flavors (`57209f8`).
7. Wizard stranded operators behind un-skippable wiring steps; both wiring actions also
   surfaced in Settings (`3f3082e`).
8. FINDING 4 — approving never created the channel (§7 said it should); `channelForIntent`
   (`be9cb35`).
9. FINDING 6 — a kids channel went live with ZERO programs: discovery backfill set
   InLibrary but dropped the rating, so an audience ceiling excluded every entry (dead
   air, §9). Backfill now carries the rating (`bad5814`).

**Open (surfaced, not taken):** FINDING 5 — the `tunarr` setup check only probes
reachability, so an unset/foreign `transcode_config_id` reads green while every channel
create 400s; recommend auto-selecting Tunarr's Default + validating it. Part 2 — the
acquisition-rating gap (a not-yet-owned title has no library rating) is entangled with
§389's stamp-once-at-create-time invariant; the clean fix is one design question: should
an entry's rating refresh at reconcile when library presence resolves? (Also fixes the
upgrade case where a pre-fix cached proposal, §8 24h TTL, carries empty ratings forward.)

**Maintainer smoke — the §21 second half, mechanised (2026-07-20).** Branch
`fix/quota-panic-live-smoke`. `make smoke` stands up a THROWAWAY install (own database,
own Tunarr container) and drives it with Playwright against the real media server, TMDB
and Ollama. Not in CI and never in `make check` — those mock every external service by
rule (§19), and this exists because the seams *between* gate-green subsystems are where
the bugs have been. Seerr is deliberately omitted so no approval can start a real
download. The stack stays up between runs, so the suite is stateful by design: every step
asserts something real in both a fresh and a re-run state, because a suite that is
normally red teaches you to ignore it.

**Three real bugs in its first four steps, none of which any gate could have caught:**

1. **`GET /v1/users` panicked on every load** (shipped in #36) — an int setting read
   through a string-only seam. Unit tests all leave the config seam nil, so the typed
   accessor was never reached. Fixed with a `LiveConfigInt` seam + a regression test that
   wires it (`0dc957e`).
2. **An empty env var silently destroyed the operator's saved value** (`be860bc`).
   `.env.example` ships `LLM_MODEL=`, and the resolver read a present-but-empty var as a
   pin. The §8.1 picker persisted `llm.model` and hot-swapped the live suggester — UI said
   "In use", suggestions genuinely worked — while every *read* still resolved to the empty
   pin, so the checklist said "no model selected" right after one was, and the choice
   vanished on the next restart (verified live: `"model":""` → `"model":"qwen3.5:9b"`).
   config-design §3 now states empty-is-unset and boot WARNs each pin it ignores. Every
   settings unit test missed it because none had ever set a key to `""`.
3. **FINDING 1 — a fresh install had no way in.** `/` redirected to `/login`, which no
   credential could pass because no account existed, and nothing on the page said the
   install was unclaimed; only an operator who guessed `/wizard` escaped. §16 has always
   told operators to "open the UI at `/` and create the owning admin", so the code
   contradicted the doc. Fixed with an unauthenticated `GET /v1/setup/state` (§7, doc-first)
   that the `_authed` and `/login` guards branch on; `needsBootstrap` **fails closed**, so a
   blip on that probe cannot drop a healthy install's users into a first-run wizard.
   Covered by a Go handler test (unauthenticated + flips on bootstrap), FE unit tests for
   the failure directions, and two e2e cases (unclaimed → wizard from both entry points;
   claimed + signed out → login).

Gate evidence: `make check`, `make e2e` (7/7), `make fe` unit (223/223), and `make smoke`
against the live stack.

**Phase 13.4e — the 13.4 gate (2026-07-19).** Branch `feat/fe-gate-13.4e`. Phase 13.4 is
complete: Channels, Board, Suggest, Settings, Users, Filler, Help, and the ⌘K palette.

**The reachability guard is the gate this phase earned.** SEVEN times in 13.4 something
was built, unit-tested, and unreachable — two settings panels never mounted; a formatter
never called (so "·til 8:00 PM" was dead UI on every channel card); a 323-line settings
form rendered by nothing; a clip's tag action gated so the one clip needing correction
couldn't be; a search scope that always returned empty; a Search button wired to a
discarded setState. Component tests passed in every case, because a component test cannot
see whether anything mounts it.

`reachability.test.tsx` asserts every route in the generated tree renders real content and
every feature-gated panel appears when its flag is on. The route list is DERIVED from
`router.routesById` — a hand-maintained list is the same mistake one level up, which
`structure.test.ts` already learned. **Verified against the real regressions:** removing
the secrets panel's mount and removing the palette's mount each fail it with a precise
message.

**The approve-flow e2e is §7's gate under test.** An admin approving enqueues the
acquisition; a member is not offered approval and nothing is enqueued (§19's negative).
Assertions land on the mock's recorded state, not the screen — "the button looked like it
worked" is exactly the failure a gate exists to catch. Verified by deleting the `isAdmin`
condition on the approval queue: the member test fails.

**Contract discipline held.** The mock backend's proposal shape was wrong twice (guessed
field names), and both times the fix was to read the generated DTO rather than the
remembered shape — the same rule the fixtures carry.

**Gate:** `make check` GREEN; `make fe` GREEN (217 app / 24 core / 9 api); `make fe-visual`
GREEN (188, zero flaky); `make e2e` GREEN (5); `make openapi-verify` GREEN.
**Next:** Phase 14 — the user-facing docs set, runbook, README, and folding in the seed
docs (checking each against the design doc first: §2 and §7.2 were both stale this phase).


**Phase 13.4d — Users and Filler shipped; Help + ⌘K remain (2026-07-19).** PRs #26–#33,
all merged. The session ran backend-prep-then-screens, and the prep was worth it: the
gap sweep before writing any UI found surfaces §12/§13 assume but nobody had built.

**Backend prep (#26, #27, #28, #29, #30).** Sessions list/revoke; the `user_sync` gate
that fixed a live `useSyncUsers` 404; the credential-path flag; clip search moved off the
federated scope; the sidecar deleted and ingest folded into the core; pod preview; the
Help docs embed, version endpoint, and a storeless-boot panic fix.

**Screens (#32, #33).** Users (allowlist, sessions, explicit import) and Filler (catalog,
tagging, ingest, pod preview). Both admin-gated in the UI as a courtesy; the server
enforces (§11, §19).

**The recurring failure of this phase, worth carrying into 13.4e.** SIX things were built,
unit-tested, and unreachable — `AiModelSettings` and `SecretsSettings` never mounted;
`formatEpgTime` never called, so "·til 8:00 PM" was dead UI on every channel card;
`SettingsGroupForm` (323 lines + tests + 12 visual baselines) rendered by nothing;
`ClipCard`'s tag action gated so the one clip that needs correcting couldn't be;
`scope=clips` advertising a corpus that always returned empty. Every component test
passed in every case. Page-level tests were added for Users and Filler specifically to
assert WIRING rather than behavior — **13.4e should make reachability part of the gate**:
every route renders, every feature-gated panel appears when its flag is on.

**Gates got two real repairs.** The a11y check rejected a disabled row whose `opacity-60`
fell under the WCAG AA contrast floor. And the visual suite's "1 flaky" — present on every
run, a different story each time — was axe-core's module-global running flag re-entering
through `runPartialRecursive`; Playwright's retry kept it green so it never blocked, but a
gate that cries wolf stops being read. Retried only on that exact message; zero flaky on
every run since.

**Shared-code cleanup (#31).** Four core formatters had NO call sites (the inverse of the
duplication we went looking for), one live grammar bug at n=1, and copy-to-clipboard
hand-rolled twice with the same two defects in both copies.

**Gate:** `make check`, `make fe` (191 app tests), `make fe-visual` (188, zero flaky),
`make e2e`, `make openapi-verify` — all GREEN on main.
**Next:** 13.4d-3 (Help page + ⌘K palette — transport and `troubleshooting.md` already
exist from #30, so it is mostly rendering + client-side search), then 13.4e.


**Phase 13.4d-0c — Help transport, version, and a startup panic (2026-07-19).** Branch
`feat/be-help-13.4d0c`. The last backend slice before the 13.4d screens.

**The docHref anchors had nothing to land on.** The API has emitted deep-links like
`troubleshooting#tunarr` since phase 8, and §13 promises "every red check deep-links to
its section" — but nothing embedded `docs/`, and the troubleshooting page did not exist.
Every red check pointed at a blank page, at exactly the moment an operator is stuck.
Now: `docs/help/` embedded via `docs/embed.go`, `GET /v1/docs` + `GET /v1/docs/{slug}`,
and a **test that every anchor the API emits resolves to a real heading** — renaming a
heading fails the build instead of silently breaking a link. Only `help/` ships; the
design docs beside it are internal and deliberately excluded.

Wrote `troubleshooting.md` with all 8 sections. This is not phase 14's docs set (that
still owns Quickstart/Concepts/Member guide/etc.) — it is the one page the *existing*
backend contract already requires. Also gave `tunarr_library` its own anchor rather than
sharing `#tunarr`: it fails for a specific silent reason (unscanned media source ⇒ dead
air while everything else reads healthy) and deserves that explanation, not generic
connectivity advice.

**Version.** Nothing exposed one — §16 discusses upgrades and the runbook assumes the
operator knows what they run. `internal/buildinfo` stamps via ldflags, falls back to Go's
embedded VCS stamps, then to "dev". Surfaced on `GET /v1/system/version` together with
readiness.

**Deliberately did NOT move /healthz + /readyz into huma.** The sweep suggested it so
orval could type them, but their consumers are Docker HEALTHCHECK and orchestrators,
which hold no session — registering them in the authenticated /v1 API would put auth in
front of a container health probe. The UI gets the typed twin instead. Pinned by a test,
because "type the health endpoints" is a tempting future change.

**Found and fixed a startup panic (pre-existing, from the 2026-07-18 guide-reader work).**
Starting with no DATABASE_URL is a SUPPORTED mode — main logs "running without a store
(not ready)" and expects /readyz to explain itself. Instead the process panicked in
BuildHandler: no store ⇒ no settings service, and `resolved.str` dereferenced nil. A
container missing DATABASE_URL crash-looped instead of answering the probe that would
have told the operator why. Guarded every `resolved` getter; regression test verified to
reproduce the exact panic without the fix.

**Gate:** `make check` GREEN; `make fe` GREEN (169 app tests); `make openapi-verify` GREEN;
manually verified a storeless boot serves /healthz 200 and /readyz 503 "no store configured".
**Next:** 13.4d proper — the Filler, Users, Help, and ⌘K screens.


**Phase 13.4d-0b pt2 — the sidecar is actually gone (2026-07-19).** Branch
`feat/ingest-in-core-13.4d0b2`. PR #25 changed the DESIGN; this closes the code. Between
the two, the docs said the sidecar was removed while the repo still shipped it — real
drift, caught by the maintainer noticing `Dockerfile.ingest` in the tree.

**The move was smaller than feared.** The download logic was already `internal/ingestkit`
(a core package, fully tested with fake downloaders); only a 71-line driver in
`cmd/loomarr-ingest/` was sidecar-specific. Renamed to **`internal/clipfetch`** because
`internal/ingest` is the Sonarr/Radarr WEBHOOK handler (§6's Ingest port) — two unrelated
concepts one autocomplete apart, now that both live in the core.

**Two real bugs found by actually building and running the image:**
1. **The tooling was x86_64-only.** Inherited from `Dockerfile.ingest`, which hardcoded
   amd64 URLs for yt-dlp/deno/ffmpeg. On arm64 the image BUILT FINE and then died at run
   time with `rosetta error: failed to open elf at /lib64/ld-linux-x86-64.so.2`. Now
   fetched per `TARGETARCH`, with a build-time `--version` check on all three so a broken
   image can never ship looking healthy.
2. **Appending the stage made the fat image the DEFAULT.** Docker builds the last stage,
   so `docker build .` produced 704MB instead of the distroless core. Reordered: filler
   before core, core last. Measured after: **loomarr:latest 31.1MB, loomarr:filler 549MB.**

**The documented size was wrong.** §10/§14/§16 claimed "~170MB" (inherited from the
sidecar's own comment). Measured reality is ~518MB more. Corrected everywhere rather than
left as a comfortable number. Also dropped **ffprobe** (~99MB): Loomarr never probes media
— Tunarr assigns duration during its `local`-source scan — so bundling it was pure weight.

**`ingest` is the first environment-derived feature gate.** It probes for runnable
yt-dlp + ffmpeg rather than reading settings completeness, exactly as config-design §7's
new exception describes. Both tools are required (yt-dlp cannot merge streams without
ffmpeg, so a half-present image would accept the job and fail mid-download on the
high-res sources most worth fetching). The 409 names the remedy — "run the loomarr:filler
image" — and deliberately is NOT `feature_not_configured`, because no setting can open it.

**Ingest returns a job id, not a result.** Downloads run minutes to hours; progress rides
the SSE bus as `filler_ingest` frames, the same shape §8.1's model pull uses. The
background context is deliberately NOT the request context, or every ingest would die the
moment a browser tab closed.

**Gate:** `make check` GREEN; `make fe` GREEN (169 app tests); `make openapi-verify` GREEN;
`docker build` of BOTH targets verified, and yt-dlp/deno/ffmpeg each executed natively on
arm64 (not inferred from a successful build).
**Next:** 13.4d-0b pt3 (pod preview — §12's explicit Filler requirement), then 13.4d-0c.


**Design change — filler ingest moves into the core; the `loomarr-ingest` sidecar is removed (2026-07-19).**
Branch `design/ingest-in-core`. Doc-only; no code, no spec drift. Maintainer call, taken before building
13.4d's Filler page so the page isn't designed against a seam we were about to delete.

**What changed and why.** The sidecar's *only* stated justification was keeping ~170MB of yt-dlp+ffmpeg out of
the core image (§14). It was already Go, so §14's language policy never required it. What that slimness cost:
a second image to build/version/publish, a `filler` compose profile, a proxy endpoint in the core just to reach
the sidecar, ingest progress that could not ride the existing SSE job bus, and a Filler page whose primary
action was a hop to an optional service that might not be running. Ingest is now a normal in-core job, and the
tooling ships in an opt-in image *tag* (`loomarr:filler`) rather than an opt-in *service* — same binary, same
config, same endpoints, so operators move between tags with a restart, not a topology change.

**The one genuinely new mechanism:** `ingest` is the first feature gate that is NOT derived from settings
completeness — no amount of configuring opens it on `loomarr:latest`, because it depends on which image is
running. config-design §7's heading previously claimed `RequiredFor` computed availability and "nothing else
does"; that is now stated as an explicit, reasoned exception rather than left as a quiet contradiction. The
invariant that survives: one function computes the set, every consumer reads it — only the inputs differ.
ffmpeg is bundled (not skipped) so yt-dlp can merge separate video/audio streams; without it, high-res sources
fail or silently downgrade.

**Sections touched:** §2 (filler flow + FillerSource port row), §7 (`/v1/filler/ingest`), §10 (ingestion path,
config, probing), §13 (Filler guide), §14 (Sidecar & CI → Ingest tooling & CI), §15 (four `INGEST_*` keys),
§16 (two tags one binary; compose profiles now sqlite/postgres/ai), §21 (repo layout drops
`cmd/loomarr-ingest/`; phase-12 text); config-design §5 (Filler settings row), §7 (the exception), §6 (wizard
step). The §15 additions are the human mirror only — the Go registry entries land with the implementation, so
`make config-docs` stays drift-free until then.

**Stale docs fixed in passing (a real contradiction, not cosmetics):** §2 lines 85/98 still described filler as
"the media server scans its dedicated filler library" and "media-server filler-library sync" — but §10 was
revised long ago to Tunarr-`local` with the media server *out* of the filler path, and §2 was never updated.
Same for `/v1/filler/sync`'s table description and config-design's Filler settings row (which also referenced
`/v1/filler/sources`, an endpoint that does not exist).

**Gate:** `make check` GREEN; `make openapi-verify` GREEN (no spec drift — doc-only).
**Next:** 13.4d-0a/0b/0c (the backend surfaces 13.4d needs), then 13.4d itself.


**Phase 13.4c-2 — the model picker and the secrets panel (2026-07-19).** Branch `feat/fe-settings-13.4c2`. The two
sub-features deliberately deferred out of 13.4c, each substantial enough to deserve the room.

**The §8.1 model picker renders a judgement it does not make.** Picking a local model is a real onboarding hurdle —
a household admin should not have to know which Ollama tag fits their GPU or supports tool-calling — so the BE
probes the machine and returns a catalog already annotated with `fit` (fits/tight/wont_fit), `approxVramGiB`,
`runtimeOk`, `pulled`, `recommended`, and a `why`. The UI's whole job is to show that honestly. Two calls worth
recording: an unrunnable model is shown **disabled with its reason** rather than hidden, because "why isn't X
listed?" is a worse question than seeing X greyed out; and an unpulled tag offers **only Download**, never "use
this", because `select` 409s on a model that is not local (§8.1). Pull progress exists only on the `llm_pull` SSE
frames, so it arrives through the shared event fan-out built in 13.4a, and a terminal frame refreshes the catalog
so the row flips from Download to In-use without a reload.

**The secrets panel states consequences before the click, not after.** §4's display policy is differentiated by
PURPOSE, so the panel is a closed set of three rather than a generic list: `API_TOKEN` and `WEBHOOK_SECRET` are
values you must paste elsewhere (viewable on demand, eye toggle + copy), while `SESSION_SECRET` has nothing to
paste anywhere and is **never displayed** — Regenerate is its only affordance. Regeneration requires a second
confirm carrying the specific consequence, because they differ sharply: rotating the webhook secret silently
breaks every Sonarr/Radarr hook already configured, and rotating the session secret signs everyone out *including
the person clicking*. An operator should learn that from the button, not the aftermath. Revealing is fetched
**imperatively on click** rather than mounted as a live query — a secret should cross the wire when asked for, not
sit in the cache of every page that renders the panel.

Gates: `make check` (30 packages) + `openapi-verify` GREEN; `make fe` GREEN (**189 tests**); Docker visual GREEN
(172, **14 new baselines**, a11y clean); `make e2e` GREEN. **Next: 13.4d** (Filler · Users · Help · ⌘K) then
**13.4e** (the 13.4 gate).

**Phase 13.4c — Settings (2026-07-19).** Branch `feat/fe-settings-13.4c`. The troubleshooting console (§13) and
the first real test of whether `SettingsGroupForm` held up outside the wizard. **It did not**, and that was the
useful finding.

**The save UNIT differs by context, so form ownership had to move out.** config-design §5 specifies ONE sticky save
bar per page — Sonarr's model, chosen because connection settings change *together* (a URL and its token) and a
half-saved pair caught mid-test looks like a broken integration. But Connections alone has four blocks (media
server, requester, Tunarr, TMDB), and `SettingsGroupForm` owned both its state and its own Save button: four
blocks would have meant four save buttons and no aggregate dirty state. Extracted **`SettingsFields`** — a
*controlled* group of fields, no `<form>`, no save — which the wizard (one group per step, own save) and a
Settings page (many blocks, one bar) both compose. `SettingsGroupForm` now wraps it; **all 29 wizard tests passed
unchanged**, which is what made the refactor safe to do.

**Also corrected:** the build plan says 5 Settings pages; config-design §5 lists **6** — it omits **Filler**. §5 is
authoritative on Settings IA (a companion wins on its own domain), so six were built. And `updatedBy`/`updatedAt`
existed on every entry but were never rendered; §5's field anatomy asks for "changed by … · when", now shown —
only where a *person* set the value, since an env pin or built-in default has no author.

**Built:** the six pages behind a nav, each composing blocks with one save bar that appears only when dirty and
sends **only changed keys** (an untouched secret reads back empty (§4) and an empty-string PATCH would clear it
(§9) — the hazard fixed in #14, now guarded at the page level too). Per-block **Test** runs the same named check
the wizard uses. The re-runnable **connection checklist** sits at the top of Connections, making Settings the
troubleshooting console for the life of the install rather than a first-run-only screen.

**Both conventions guards fired on my own work** — `structure.test.ts` caught a hook filed inside another module's
folder, and `story-coverage.test.ts` caught two new Layer-2 components shipped without stories. Biome's a11y rule
caught `role="region"` where a `<section>` belongs. Three convention failures I did not have to notice myself.

**A silent no-op in my own tooling, worth recording:** this entry was missing from the 13.4c PR entirely. The
insert anchored on a marker that did not exist on that branch (13.4b was still unmerged), and `str.replace`
returns the string unchanged rather than failing — the same class of quiet failure as an ignored `maxAcquire` or a
guessed guide field. Entries now anchor on the file's stable preamble and the result is verified, not assumed.

**Deferred, flagged:** the §8.1 **AI model picker** (probe, fit-ranked catalog, hot-swap, `llm_pull` progress over
SSE) and the **generated-secrets panel** (view/copy/regenerate, §4).

Gates: `make check` (30 packages) GREEN; `make fe` GREEN (**167 tests**); Docker visual GREEN (157, **14 new
baselines**, a11y clean); `make e2e` GREEN. **Next: 13.4c-2** (model picker + secrets panel) then **13.4d**
(Filler · Users · Help · ⌘K).

**Phase 13.4b — Channels, Channel detail, Board (2026-07-19).** Branch `feat/fe-channels-13.4b`. The
"where is my stuff" surfaces, on top of the guide read landed as this slice's prerequisite (#19).

**Two derivations that belong in one place, not inline in a page.** `channelHealth` maps the API's lifecycle
(`building/live/drifted/detached`) plus slot fill onto the card's presentational health — deliberately different
vocabularies (channel-card.type.ts says a page must derive this). The interesting case is **live with unfilled
slots**: the channel IS airing (Tunarr plays flex, never dead air, §9) but is not yet what was asked for, so it
reads `pending-slots` rather than `healthy`, which would hide the backfill the operator is waiting on.
`channelOnAir` answers a *different* question — a **drifted** channel still broadcasts, it just no longer matches
intent, so it is `live` there and `drift` in health. Both are pure and tested.

**The Board leads with the journey, not the states.** §13 asks for member framing — "1 of 3 titles have landed" —
so the five provisioning states (§4) collapse into waiting · acquiring · ready. `unavailable` deliberately does NOT
become a fourth stage: a title that gave up after its TTL belongs to the "waiting" conversation (something to
retry), not a verdict on the channel — and it stays in the progress **denominator**, since dropping it would
silently shrink what was requested and read as better progress than reality.

**Built:** Channels (cards with now/next from the one-call guide endpoint, health rollup, reconcile-now, and the
§6 empty state whose single next action is "suggest a channel" — the only way to make one). Channel detail moved
the route into a directory (`channels/index.tsx` + `channels/$id.tsx`) and shows slot fill, the Tunarr link, and
the **relaxation ladder**: each applied relaxation is structured (`kind/from/to`), so a chip reads
"audience: TV-Y → TV-Y7" instead of an opaque label — the honest account of what Loomarr loosened to fill the
channel (programming-design §9). Board groups titles by stage with per-title `StateBadge` for the operator who
wants detail, and offers retry only where a human can act.

**Caught by the typechecker, not by me:** the retry initially posted `{key}`, but the enqueue contract takes the
title's real identity (`mediaType/tmdbId/tvdbId/name/year`) — the 1:1 generated client refusing a hand-guessed
body, which is exactly what it is for.

Gates: `make check` (30 packages) + `openapi-verify` GREEN; `make fe` GREEN (**172 tests**); Docker visual GREEN
(144, 0 diffs); `make e2e` GREEN. **Next: 13.4c** — Settings (5 groups, secrets lifecycle, the §8.1 model picker,
and the re-runnable checklist as the troubleshooting console), which reuses `SettingsGroupForm` from 13.3b.

**Phase 13.4b (prerequisite) — reading Tunarr's guide for now/next (2026-07-19).** Branch
`feat/fe-channels-13.4b`. The BE half of Channels: `NowNextStrip` was built in 13.2 and listed under Channels in
the build plan, but had **no data source**. `LineupEntry` carries a duration and no start time — Loomarr owns the
lineup (what should play), Tunarr owns playout (when it actually plays, §6) — so airtimes are Tunarr's truth and
cannot be recomputed here without duplicating its scheduling math.

**How the contract was established, after two wrong turns worth recording.** First I claimed this needed
maintainer-supervised live contact and handed over curl commands. That was invented: the repo vendors Tunarr's
OpenAPI (`api/vendor/tunarr-openapi.json`), a version-pinned Tunarr runs in the dev stack, `TUNARR_URL` is in
`.env`, and Tunarr is open source. The rule I cited forbids the *test suite* touching the network, not
investigating. Second — prompted by "the code is open source, why not look at it?" — reading Tunarr's own zod
schemas at tag `v1.3.8` **invalidated the capture I had just taken**: I had sampled the SINGULAR
`/api/guide/channels/{id}`, whose shape (`{index, lineupItem, startTimeMs}`) carries **no title and no stop time**.
Had that been pinned, Channels would have shipped a title-less strip plus a second lookup just to name a program.
The **plural** `/api/guide/channels` carries `start`/`stop`, a real `title`, and tmdb `identifiers[]` — and is keyed
by channel id, so a Channels LIST costs **one** upstream call, not one per card. That also dissolved the N+1
objection I had raised against doing this at all. Capture + both traps recorded in
`fixtures/tunarr/GUIDE-FINDINGS.md`; the vendored spec does not type the guide response, so the source and the
capture are the only authorities.

**Built:** `programmer.Guide` (parses the guide; tmdb id lifted from `identifiers[]` so an entry joins to a
provisioning key with no second lookup; an ungenerated guide is empty, not an error — the tolerance `GetLineup`
already gives an unprogrammed channel). `guideAdapter` reduces a timeline to the pair a card shows, using a
**containment** test rather than "the first entry", so an out-of-order or already-finished head cannot mislabel
what is on. `GET /v1/channels/now-next` serves every channel from that one call, keyed by the **Loomarr** id (the
FE never sees Tunarr ids), and a Tunarr hiccup returns empty rather than blanking the page.

**Self-review caught two more:** the new route shares a prefix with `/v1/channels/{id}` and could plausibly have
resolved to `get-channel` with `id="now-next"` — Go 1.22 prefers the literal segment, but that was assumed rather
than proven, and is now pinned; and a test carried a hand-rolled `itoa` reinventing `strconv`. Parsing is tested
against the pinned capture and **proven to fail** (read the title-less singular shape → red).

Gates: `make check` (30 packages) + `openapi-verify` GREEN; `make fe` GREEN; `make e2e` GREEN; orval generated
`useChannelsNowNext`. **Next: 13.4b proper** — the Channels page (cards, health rollup, now/next, reconcile-now),
Channel detail, and the Board.

**Phase 13.4a — the Suggest workspace + approval queue (2026-07-18).** Branch `feat/fe-suggest-13.4a`. The
product's core loop: a sentence becomes a grounded lineup. 13.4 is 8 surfaces, so it is sliced — **a** Suggest +
approval, **b** Channels + Board, **c** Settings, **d** Filler/Users/Help/⌘K, **e** the gate. Suggest went first
because the wizard already hands off to `/suggest` and it was landing on a placeholder — a broken promise in
shipped code.

**A three-way contract mismatch, fixed at the root (first commit).** `submitInput.Body` was a hand-mirrored copy
of `suggest.Intent` — the exact pattern PR #9 removed from ProposalDTO, whose "zero hand-written mirror on either
side" comment sits *twelve lines below* the mirror that had drifted. It omitted `RuntimeTgt`, so `runtimeTargetMin`
was **unreachable by any client** even though the suggester feeds it to the LLM prompt and the scorer, and §13
lists a runtime target among the constraints a user may set. Meanwhile the FE's `intentSchema` said `maxAcquire`
where the wire says `maxAcquisitions` — parses fine, serializes fine, server ignores it: **a user's acquisition cap
silently vanished**. Fixed by typing the body from the domain rather than patching one field, so `SubmitInputBody`
disappears from the spec entirely and the request body now refs the SAME `Intent` schema `Proposal.intent` already
used (spec net −29 lines). Two now-redundant `submitAdapter` copies deleted — the composition root's and a second
one duplicated inside `internal/integration`. **Both halves are guarded, both proven to fail:**
`TestSubmit_CarriesTheWholeIntent` (drop RuntimeTgt's json tag → red) and a **compile-time** `intent-contract.test.ts`
asserting the form's output *is* a valid API `Intent` (rename a field → `tsc` breaks).

**One SSE stream, many listeners.** The live phases (`searching · reasoning · scoring`) exist ONLY on the wire — no
GET returns them — but the layout already opens an EventSource for cache invalidation, so a screen calling
`useLoomarrEvents` again would open a second. `LoomarrEventsProvider` owns the single subscription and fans frames
out; `useLoomarrEventListener` is a no-op outside it, so a component still renders in a story or unit test, just
without live frames.

**Built:** `IntentForm` (the hero + templates from `packages/core`, plus §13's "intent-writing hints" — era, tone,
runtime target, acquisition cap — behind a disclosure, because the blank page is solved by one good sentence, not a
form) → `useSuggestionRun` (submit → job → live phase → the proposal, matched on `jobId` client-side since
`GET /v1/proposals` filters by status but not job, and the DTO already carries it) → `GenerationProgress` →
`ProposalReview`, with approve/deny **admin-only** (§7/§11 — a member reviews without the controls). The
`ApprovalQueue` lists everything still `submitted`: approving is the only path from a proposal to `/v1/titles`, so
that list is the audit surface for what is about to spend real resources. Per §8 the stream stays a latency
optimisation — a dropped `done` frame costs a spinner, not a proposal, because the list refetches on the same event.

Gates: `make check` (30 packages) + `openapi-verify` GREEN; `make fe` GREEN (**170 tests**); Docker visual GREEN
(144, 0 diffs); `make e2e` GREEN. **Next: 13.4b** — Channels + Channel detail + Board.

**Phase 13.3d — the phase gate: wizard flow suite + page snapshots (2026-07-18).** Branch `feat/fe-wizard-13.3d`.
Closes Phase 13.3 (frontend-build-plan §5 gate: "wizard e2e smoke vs mocked BE green; page-level snapshot per
wizard step"). `make e2e` was a stub that exited 1; it is now real.

**What it drives.** The REAL embedded SPA build (`internal/web/dist`, served with history fallback so `/wizard`
resolves exactly as it does from the Go binary's embed), against a **mocked `/v1` via Playwright route
interception** — no MSW dependency and no stub server, so there is no second API implementation to drift from
`openapi.yaml`. The mock is deliberately **stateful**: signing in flips `me`, each one-click wiring turns its own
check green, importing marks the candidate imported. A first-run flow that can't progress proves nothing.

**The smoke walks a fresh install end to end** — bootstrap → auto-login → checklist → Live TV → webhook handshake
(asserting the URL is built from the *revealed* secret and that both apps read as listening, neither as failed) →
skip → Tunarr library → import users → guided first channel → lands on `/suggest?intent=…` with `setup.completed`
flipped. Plus both halves of first-run routing: a completed setup opens Channels, an unfinished one is sent back.
**Seven page-level snapshots**, one per step, with a `mask()` for the relative-timestamp region (real behaviour we
want rendering, just not diffing).

**The gate immediately earned its keep — a bug no unit test had caught.** On the optional **users** step, Continue
was permanently disabled: `isStepDone` returns false for any step without a server check, so an operator who
*did* import users was stranded behind a button that could never enable — only Skip worked. Optional steps must
never block; they now gate on skippability rather than completion. Only walking the whole flow as a user surfaces
that. A page snapshot also caught duplicated copy (the shell's step description and the step's own paragraph said
the same sentence), now deduped to the §11 point that actually surprises people.

**Two Playwright configs, one determinism kit.** A maintainer question ("why two configs?") exposed real
duplication: the diff ratio, launch flags, reduced-motion and retries were copy-pasted, so tuning one gate would
silently diverge from the other. They now share `playwright.shared.ts`. The configs stay separate for a concrete
reason recorded there: Playwright boots **every** configured `webServer` regardless of project filter, and the
suites have different build prerequisites (`storybook-static` vs `internal/web/dist`) — merging would force both
builds to run either gate. `make e2e` mounts the repo ROOT (not `web/`) because the SPA build lands outside `web/`,
and serves via pure-JS `http-server` for the same reason the visual suite does: the bind-mounted `node_modules`
holds the HOST's native binaries, so vite/rollup cannot run inside the Linux image.

**Proven to fail:** renaming the Live TV CTA breaks the flow and the suite goes red on all three attempts, then
green again on revert. Gates: `make check` (30 packages) + `openapi-verify` GREEN; `make fe` GREEN (165 tests);
Docker visual GREEN (144, 0 baseline diffs); **`make e2e` GREEN (3 specs, 7 committed page baselines)**. CLAUDE.md's
command contract updated (`make e2e` / `make e2e-update`). **Phase 13.3 COMPLETE — next: 13.4, the core product
surfaces**, which inherits the Suggest workspace the wizard now hands off to.

**Phase 13.3c — wizard steps 3–7 + the conventions guard (2026-07-18).** Branch `feat/fe-wizard-13.3c`.
Completes the operator first-run flow, and makes the FE conventions self-enforcing after they were broken twice.

**Two more doc↔code gaps closed (BE prerequisite, doc-first).** Scoping the steps surfaced two read surfaces the
docs promise but nothing implemented — the same class as `setup.completed` in 13.3b. (1) §4 says `API_TOKEN` and
`WEBHOOK_SECRET` are "viewable on demand by admins (eye toggle + copy button)", and `secrets.go` even comments that
webhook_secret is "viewable (as URL)" — but the only route returning a generated secret's value was
`POST …/regenerate`, so an operator could see the webhook secret **only by rotating it**, breaking every webhook
already configured. Added **`GET /v1/settings/secrets/{name}`** (`{value, displayable}`; SESSION_SECRET withheld),
with a test asserting **reading never rotates**. (2) §11/§13 say the admin "picks which Emby/Jellyfin accounts get
in", but `POST /v1/users/import` takes raw media-server ids and nothing listed candidates — an admin would have
needed GUIDs. Added **`GET /v1/users/candidates`** (accounts + `imported` flag), tested through the REAL provisioner
against the testkit media server, asserting the flag flips after an import.

**Steps built.** 3 **Live TV** + 5 **Tunarr library** share a `ConnectStep`: both are idempotent one-click wirings
where **the BE check, not the click, reports success** (§6 "never silent"), so the button stays available once green.
4 **Webhook handshake** shows the paste-able `/hooks/arr?token=…` URL built from the revealed secret and **polls
`setup/status` per app**, so Sonarr and Radarr flip independently while the operator is in the other tab. 6 **Import
users** renders the candidates picker — already-imported accounts stay **visible but locked** ("where did they go?"
is worse than a disabled row), and a missing media server reads as a reason to skip, not a wall. 7 **First channel**
**hands off** rather than duplicating 13.4: it flips `setup.completed` through the ordinary PATCH and drops the
operator into Suggest with the intent prefilled. Channel templates moved from `packages/fixtures` (story data) to
**`packages/core` as product data** — §13 says they ship in the bundle — with a test asserting every template is a
valid `intentSchema` intent.

**A maintainer catch worth more than the feature work.** `src/wizard/` (15 loose files) and `src/auth/` (4) both
violated the standing folder-per-module rule — which was *already written down in memory* and broken anyway, twice.
Both are now folder-per-module with co-located `.type.ts` + `index.ts`, and the shared `wiring-step.type.ts` was
dissolved so each step owns its props. The durable fix isn't better recall, it's **`src/test/structure.test.ts`**:
a conformance test that fails on a loose file, a missing barrel, or a misnamed implementation file — the same move
`story-coverage.test.ts` makes for stories and the GritQL plugins make for arrow-functions/exports. **Proven to
fail** (drop a stray `.ts` in `src/wizard/` → red). The conventions memory now records which check enforces which
rule, so a future rule change ships with its check.

Also fixed en route: `ChecklistItem` renders `hint` only on `fail`, so the webhook step's "Listening — press Test"
silently vanished; the per-app status line now lives in the step rather than bending a shared component (and its
baselines) for one caller. Gates: `make check` (30 packages) + `openapi-verify` GREEN; `make fe` GREEN (**165
tests**, +31); Docker visual GREEN (144 passing, **0 baseline diffs**). **Next: 13.3d** — the phase gate (wizard e2e
smoke vs a mocked BE + a page-level snapshot per step).

**Fix: an empty-string PATCH could silently wipe every stored secret (2026-07-18).** Branch
`fix/settings-secret-clear` (BE-only; independent of the 13.3b/form PRs). Found while hardening the FE settings form:
the FE guard I'd added only protected *our* client, so the hole was still open in the API itself.

**The hazard.** `GET /v1/settings` deliberately returns **no value** for a secret (`internal/settings/list.go` — only
`set`/`preview`, per §4 "never echoed"), while `PATCH /v1/settings` treated `""` as **delete** for *any* key
(`write.go`). So **a GET → PATCH round-trip destroyed every stored secret** — Emby token, Seerr/TMDB/LLM keys. Any
script, the break-glass `API_TOKEN` path, or a future client (mobile, a second FE) would hit it; nothing in the API
prevented it. Non-secrets were never at risk (GET returns their real values, so their round-trip is idempotent).

**The fix (doc-first, maintainer-chosen "reject + explicit DELETE").** An empty-string PATCH on a `secret` key is now
**`invalid`** ("replace-only; send a new value, or DELETE the key to clear it") — making the round-trip *loud and
harmless* rather than quietly destructive, and matching §4's own replace-only language. Clearing keeps working through
a new explicit verb: **`DELETE /v1/settings/{key}`** (admin) drops the stored override so the key reverts to
env/default — `204` cleared · `404` unknown · `409` env-pinned (the environment wins; unset the variable to manage it
in-app). Hot-applies like any write. Docs first: config-design §9 carve-out + §8 route, design §7 endpoint table.
Nothing reachable was lost — the FE already couldn't clear a secret (a secret's baseline is `""`, so the changed-only
PATCH never included it).

**Tests are the point here:** a direct regression test reproduces the naive round-trip (`library.token: ""` alongside a
real edit) and asserts the stored secret **survives**, plus the rejection carries actionable text; `Clear` is covered
for success/unknown/env-pinned, and the route test asserts the three HTTP mappings and that DELETE is admin-only (§19).
`make check` + `make openapi-verify` GREEN; `make fe` GREEN (orval picked up the route — `useSettingsClear` is ready
for 13.4 Settings' "remove this integration").

**Forms: react-hook-form → TanStack Form (2026-07-18).** Branch `refactor/fe-tanstack-form` (stacked on 13.3b).
Maintainer call, done doc-first — the third and final leg of the deliberate TanStack consolidation (Query 13.1,
Router 13.3a, Form now).

**Doc-first (§14 Forms row + frontend-design §4.3/§6 + frontend-build-plan):** the old row justified RHF as *"the
shadcn form convention"* — a justification that was already moot, since Loomarr never adopted shadcn's RHF-bound
`<Form>` wrapper (every form hand-composes `Label`+`Input`). New rationale: `zod@^3.24` implements **Standard Schema**,
which TanStack Form consumes natively, so the `packages/core` schemas pass straight in and **`@hookform/resolvers`
disappears entirely** — two deps collapse to one; field types infer from `defaultValues` (the same end-to-end typing
as orval DTOs + typed router links); `@tanstack/form-core` is framework-agnostic so mobile shares form *logic*, not
just the schemas.

**Behavior-neutral by construction:** validators run `onSubmit` (as RHF did), not `onChange` — errors stay out of the
operator's way while typing. **`LoginForm` and the bootstrap-step tests passed completely unchanged**, which was the
intended signal that neither DOM nor UX drifted; the Docker visual suite agrees (**143 passing, 0 baseline diffs**).
The open question from the plan resolved cleanly: `bootstrapSchema`'s cross-field `.refine(…, {path:["confirm"]})`
does surface on the `confirm` field through Standard Schema.

**`SettingsGroupForm` converted from controlled-props to owning its state** (maintainer chose the wider scope), which
surfaced **two real findings**. (1) **TanStack Form reads a dot in a field name as a nested path** — and every registry
key is dotted, so `name="library.url"` wrote `{library:{url}}` instead of the flat key the API expects. Values are now
carried **positionally** (aligned with `entries`), keeping the form agnostic to key shape; the mapping back to dotted
keys lives in one tested helper. (2) Because the form now produces the PATCH body, it must submit **only changed**
fields: a stored secret always reads back as `""` (§4 never echoes it) and an empty-string PATCH **clears** an optional
key (§9) — so submitting every field would have silently wiped every stored secret on save. `changedEntries` enforces
that, with a dedicated test asserting an untouched secret is absent from the body. The form deliberately does **not**
re-seed on `entries` change (a background refetch must never overwrite typing); a caller wanting a fresh baseline
remounts with a `key`.

Gates: `make fe` GREEN (**134 tests**), Docker visual GREEN (143, **0 diffs**), `make check` GREEN (no Go changes).

**Phase 13.3b — wizard foundation + steps 1–2 (2026-07-18).** Branch `feat/fe-wizard-13.3b`. The settings-driven
wizard machinery, built on config-design §6's rule that **the wizard IS the settings system** — no parallel form
system; each step renders a registry group's form through the same `PATCH /v1/settings` path Settings (13.4) will use.

**Doc↔code gap closed (doc-first):** config-design §6 specified *"a `setup_completed` flag in the registry"* but no
such key existed (checked all 47). Added **`setup.completed`** (bool, `SETUP_COMPLETED`, Advanced, GroupAdvanced) to
`internal/settings/declared.go`, and amended §6 to name the key exactly + note it's written through the ordinary PATCH.
`api/openapi.yaml` unchanged (registry keys are data, not schema — `openapi-verify` clean); `docs/configuration.md`
regenerated (+1 row).

**Built — the reusable half (all Layer-2, story + test each):** `SettingField` — ONE `SettingEntry` as a control,
everything from contract data: `kind` picks the widget (bool→Checkbox, enum→Select, int/url/string→Input,
secret→password), `enum` fills options, `doc` is help text, **`provenance:"env"` locks the field** with a "set via
environment" chip (§3 visible provenance), a stored secret shows its masked `preview` tail with **replace-only**
editing (§4 — never echoed), `caution` explains a self-healed value, and a `SettingResult` renders `invalid(problem)`
inline / `pinned` as a badge. `SettingsGroupForm` — a group's fields + the per-group **Show advanced (n)** toggle (§5)
+ inline live-test result + Save. Two new **dependency-free ui primitives** (`Checkbox`, `Select`) as native elements —
deliberately not Radix, so no §14 dep conversation for a checkbox and a 2-option list. `humanizeSettingKey` added to
`packages/core` (`library.url` → "Library URL") because the settings API ships `doc` but **no display label**; derived
once so wizard + Settings can't drift, and it doubles as the checklist's check-name humanizer.

**Built — the wizard:** `WizardShell` (rail + card + nav; step states `done|current|pending|skipped` — a **skipped
optional step renders neutral, never red**, §6). `src/wizard/steps.ts` holds the **resume-safe derivation as pure,
tested functions**: completion comes from server truth (`GET /v1/setup/status` + whether a session exists), never from
client progress, so a refresh — or finishing from another browser — lands correctly. Required = **media_server +
tunarr** only (§6 "shortest honest path"); Seerr/AI/TMDB/filler report but never block. The three wiring checks
(`livetv`/`webhook`/`tunarr_library`) each belong to their own later step, so Connections doesn't double-count them.
**Step 1 Bootstrap** — `POST /v1/setup/bootstrap` with `bootstrapSchema` (already in core), then **auto-signs in with
the same credentials** (bootstrap issues no session) so the operator types the password once; a 409 isn't an error to
explain away, it's "this instance is past bootstrap → sign in". **Step 2 Checklist** — reuses the 13.2 `ChecklistItem`,
driven by `setup/status`, failures shown as the BE's plain-language hint + doc deep-link (no stack traces, ever).
**First-run routing:** `/` reads `setup.completed` and routes to `/wizard` until set — and **fails open** (a member's
403, a 500, a missing key ⇒ "completed"), so a non-admin is never trapped in operator-only setup.

**A real bug the tests caught:** the wizard computed its resume step before `me`/`setup-status` settled, so it painted
the wrong step and then yanked the operator forward (checklist → TV guide). Now it holds the paint until both settle
and lands right the first time. Gates: `make check` + `openapi-verify` GREEN; `make fe` GREEN (**129 tests**, +27 this
phase); Docker visual GREEN — **143 passing, 32 new baselines** (16 stories × 2 viewports), a11y clean on every new
component. **Next: 13.3c** (steps 3–7: livetv-connect, webhook handshake, tunarr-connect, user import, first channel)
then **13.3d** (gate: wizard e2e smoke vs mocked BE + a page snapshot per step).

**Phase 13.3a — auth foundation on TanStack Router (2026-07-18).** Branch `feat/fe-auth-13.3a`. The FE identity
layer the wizard + product surfaces sit behind (§11, §12) — built, then **the router was swapped react-router →
TanStack Router** mid-branch (maintainer call, doc-first) before more surfaces land, so 13.3a arrives already on
the typed router.

**Router swap (doc-first §14/§12, frontend-design, frontend-build-plan):** `react-router` v6 → **`@tanstack/react-router`
(file-based)**. Rationale in §14: end-to-end type-safe routing (typed params/search/links) matching the orval-contract
ethos; shares the TanStack Query client via router `context`; loader-based auth guards (`beforeLoad` → `redirect`, no
guard-flash). Web-only — routing was always the per-platform seam (mobile keeps Expo Router). `@tanstack/router-plugin`
(Vite) + `-cli` (`tsr generate`) generate `src/routeTree.gen.ts` — **gitignored + Biome-ignored + regenerated by
`pnpm codegen`**, exactly like orval output (never hand-edited). Route tree is `src/routes/`: `__root` (carries the
queryClient context), public `login` + `wizard`, and a pathless **`_authed`** guard layout whose `beforeLoad`
`ensureQueryData(meQueryOptions)` throws `redirect({to:"/login", search:{redirect: location.href}})` on 401 — the app
shell + all stub screens hang off it. `main.tsx` builds `createRouter({routeTree, context:{queryClient}})` + the
`Register` module augmentation (global type-safe Links).

**Auth pieces:** `useAuth` — the one interpreter of `GET /v1/auth/me` via shared `meQueryOptions` (`retry:false`; a 401
is a definite answer), narrowing the success|error union by status → `{user, isAuthenticated, isAdmin, isLoading, error}`
(feeds AppShell name/role). `RequireAuth` **deleted** — the guard is now `_authed`'s `beforeLoad`. `LoginForm` —
presentational (RHF + `zodResolver(loginSchema)` from `packages/core`, shared verbatim with mobile), block-level failure
via `ErrorState` (RFC 7807 → words), field errors inline, no user enumeration. `login` route wires `authApi.useLogin`
(invalidate me + `history.push` to the typed `redirect` search param on success; `beforeLoad` bounces an already-authed
visitor). AppShell `NavLink` → TanStack **`Link`** (active state via `data-status="active"`, so it stays a pure-className
component) + a footer sign-out; `_authed` layout feeds the real user + logout (`authApi.useLogout` → `queryClient.clear()`
→ `/login`). **Kept 1:1:** `LoginForm.onSubmit` takes generated `LoginInputBody`, identity reads `MeBody`. `Placeholder`
moved out of `src/routes/` (file-based = every file is a route) → `components/loomarr` (+ story; its "Dead air" label was
`text-static-500` at 2.94:1 — bumped to `static-400` so the newly-covered a11y gate passes, per the 13.2 precedent).

**Test/story router harness** (the swap's one real cost): TanStack `Link`/route hooks need a RouterProvider even in
isolation — added `RouterHarness`/`withRouter` in `test/story-utils` (a minimal in-memory router over the nav paths).
Coverage: a **router-level `app-router.test`** drives the REAL generated tree — signed-out → login form, authed → screen,
sign-in → Channels (replaces the old login + require-auth component tests, preserves §19 negative-auth). `make fe` GREEN
(codegen now generates the route tree before typecheck; 77 units); Docker visual GREEN — 6 `loginform` + 2 `placeholder`
baselines added, 4 `appshell` regenerated (Link renders pixel-equivalent; sub-pixel delta only). **Next: 13.3b/c** (the
7-step wizard) then **13.3d** (wizard e2e smoke vs mocked BE + a page snapshot per step).

**Contract 1:1 hardening — BE proposal typing + FE de-duplication (2026-07-18).** Branch
`feat/fe-contract-1to1`. A maintainer catch ("is anything hand-written that shouldn't be?") — audited the
FE against the generated client and found several hand-mirrored types (design §12 = no hand-written glue).
**BE:** `ProposalDTO.proposal` was `json.RawMessage` (→ OpenAPI `unknown`); now typed as `suggest.Proposal`
directly (imported the domain struct, dropping the old local `proposalBody`/`lineupItem`/`acqItem` mirrors),
so orval generates the full `Proposal`/`ProposalItem`/`Scores`/`ChannelPolicy` schema — true 1:1. **FE:**
deleted the mirrors — `Clip`/`ClipKind`/`ClipAudience` → `ClipDTO*`, `ProposalView`/`ProposalItemView` →
generated `Proposal`/`ProposalItem`, `ProblemDetail` → `ErrorModel` (deleted `mutator.type.ts`),
`ProvisioningState` → `TitleDTOState | "drift"` (5 states from the generated union + the one FE-only state).
Kept the genuine FE view models — the ⌘K `PaletteScope` (renamed from the colliding `SearchScope`) and the
derived `ChannelHealth` rollup — now documented as such. Forced a real honesty fix: generated
`ProposalItem[]` is `| null` (nil Go slice), so ProposalReview now null-guards. `make check` (BE incl. the
policy round-trip integration test) + `make fe` + the Docker visual suite (104/104, renders unchanged) all
GREEN; `api/openapi.yaml` regenerated + committed. Docs: frontend-build-plan §3 corrected.

**Phase 13.2b/c/d — remaining components + Storybook adoption (2026-07-17).** Branch
`feat/fe-gallery-visual-13.2`. **13.2b DONE:** the remaining Layer-2 components (IntentInput,
GenerationProgress, ProposalReview, PodTimeline, ClipCard, ApprovalQueueItem, SearchCommand) + a shared
`Badge` primitive + `formatClipDuration` (sub-minute-aware) — all with CVA/tokens, states, a11y, tests;
`make fe` + `make check` were GREEN before the Storybook pivot.
**DECISION (maintainer, doc-first): adopt Storybook 10 for the component gallery/workshop AND
visual/a11y testing, replacing the hand-rolled `/__gallery` registry.** This reverses frontend-design §5's
original mechanism, so the docs were updated FIRST (per prime directive #1): frontend-design §3/§4.1/§4.2/
§5/§7, design §14 (new dep row + rationale; **Chromatic rejected** — hosted SaaS breaks the offline rule),
frontend-build-plan §4/§9/§10, CLAUDE.md command contract. **Rationale:** CSF is the industry-standard
component contract + a real dev workshop (controls/autodocs/a11y panel), and it carries to the future
mobile app via `@storybook/react-native` (Expo, on-device). **Every §5 guarantee preserved:** offline
(`storybook-static`), deterministic (Playwright Docker, `document.fonts.ready`, `prefers-reduced-motion`
+ `animations:'disabled'`, frozen clock), committed baselines (`toHaveScreenshot` `maxDiffPixelRatio 0.001`),
and 100%-coverage-enforced (a test maps the component barrel → stories). a11y: `@storybook/addon-a11y` in
the workshop + `@axe-core/playwright` in the *same* Playwright pass over `storybook-static` (one browser
layer for pixels + axe; `addon-vitest` deferred as an optional future for play/interaction tests).
**Mobile-ready:** deterministic fixtures move to
a shared `packages/fixtures`, data contracts to `packages/core`, so web + future RN stories share args.
**Built:** Storybook 10 + addon-a11y; the hand-rolled registry replaced by 52 co-located CSF stories across
15 components (exports-at-end — the maintainer rejected exempting stories from the `no-inline-export` plugin,
and Storybook 10 indexes `export { … }` + `export default meta` fine); `packages/fixtures` + core contracts;
the story-coverage test. **The a11y gate immediately caught + fixed three real WCAG bugs** the earlier
components shipped: informational `text-static-500` (2.94:1, §2.1 says decorative-only) → `static-400`;
`opacity-70` on a denied approval card compositing all its text below AA → removed; a `<li role="alert">`
breaking the `<ol>` list structure → moved to a sibling `<p role="alert">`. **Visual determinism (the hard
part):** `make fe-visual`/`-update` now run Playwright **inside the pinned `mcr.microsoft.com/playwright`
image** (the reference rasterizer, §5.2) — Chromatic still rejected — reusing the host's JS-only node_modules
+ the image's browsers (no in-container install, host binaries untouched). Getting a stable green took three
fixes beyond the doc's kit: `animation: none` (reduced-motion only fast-forwards, leaving infinite spinners
on a random frame), **element-scoped** snapshots (`#storybook-root`, not the centered page whose fractional
margins shift text AA), and **retries** for residual sub-pixel jitter (a real diff still fails every attempt).
**104 `*-linux.png` baselines committed** (non-Linux suffixes gitignored). `make fe` + `make check` GREEN;
the Docker visual suite passes (flaky-on-retry ≤2/104). frontend-design §5.1/§5.2 updated to match.

**Phase 13.2a — design-system foundation: components, conventions, Biome (2026-07-17).** Branch
`feat/fe-design-system-13.2`. The Layer-2 component vocabulary + the codebase conventions + linting that
everything downstream (wizard, surfaces) builds on. **Self-hosted Geist** (@fontsource-variable, bundled
by Vite — visual-test determinism, §2.2). **shadcn primitives** (Button/Card/Input/Label, restyled via
tokens only). **Layer-2 components** (frontend-design §3): StateBadge, OnAirIndicator, NowNextStrip,
ChannelCard, EmptyState, ErrorState (RFC7807 renderer), ChecklistItem, AppShell — each with CVA/tokens,
all states, a11y (sr-only status, sentence-case DOM text so SR reads words not letter-spaced shouting).
**Maintainer conventions established mid-build (saved to auto-memory [[fe-code-conventions]])** and applied
across the WHOLE tree (app + packages, orval-generated excepted): (1) arrow-function expressions, (2)
exports in a single block at end of file, (3) folder-per-component/module with `name.tsx` + `name.type.ts`
+ `name.test.tsx` + `index.ts` barrel, (4) types isolated in `*.type.ts`, (5) barrel imports, (6) a
Vitest+Testing-Library test per module. **Biome** (maintainer's call over ESLint) for lint+format with
sensible settings, wired into `make fe` + new `fe-lint`/`fe-lint-fix` targets; Tailwind-v4 CSS excluded
(Biome's CSS parser can't read @theme/@custom-variant), `noLabelWithoutControl` off (false-positive on the
Label primitive), `useSortedClasses` on (Tailwind class sort in cn/cva/clsx — kills visual-diff churn).
**Two custom Biome GritQL plugins** (maintainer chose the Biome-native path over ESLint-hybrid/ts-morph;
current API verified via web docs since training predates the stable release) enforce the rules Biome
lacks built-ins for: `no-function-declaration.grit` (arrow expressions only) and `no-inline-export.grit`
(declare-then-export-block — catches `export const|let|var` + `export type X =`; `export function` falls
to the first plugin). Known GritQL gap (documented in-plugin): Biome 2.5 doesn't match TS
`export interface|class|enum` nodes, but the `*.type.ts` convention already routes types into export
blocks. A `no-raw-hex-color` plugin (enforce the §2 token layer) was prototyped but dropped — GritQL
regex doesn't reliably scan string-literal contents; noted as a follow-up. **Toolchain fix:**
`vitest@2→3` to dedupe `vite` to v6 (the v5/v6 skew broke the config types). Tests: **28 app + package
tests, all green** (StateBadge/OnAir/ChannelCard/EmptyState/ErrorState/ChecklistItem/AppShell + button/
card/input/label + Layout; core events/format/schemas; api mutator; tokens palette/contrast). `make fe`
GREEN (biome + codegen + typecheck 4 pkgs + tests + embedded build), `make check` GREEN. **Deferred to the
next PR (13.2b–d):** the remaining Layer-2 (IntentInput, GenerationProgress, ProposalReview, PodTimeline,
ClipCard, ApprovalQueueItem, SearchCommand), the `/__gallery` registry, and the Playwright visual + axe
harness.

**Phase 13.1 — FE workspace skeleton + token pipeline (2026-07-17).** Branch `feat/fe-scaffold-13.1`.
The greenfield frontend foundation (`frontend-design.md` §2.5/§4, §7 "Phase 1"): a pnpm monorepo under
`web/` with the shared packages the future Expo app bolts onto. **packages/tokens** — the Test Card
palette as the TS source of truth + a deterministic generator emitting `theme.css` (Tailwind v4 @theme),
a NativeWind-ready `tailwind-preset.cjs`, and `tokens.json`; its **contrast gate reproduces the design's
published WCAG ratios to the decimal** (onair base 4.01→-300 4.53, suggest 3.86→4.65) and fails the build
on any on-tint regression (§2.1). **packages/api** — orval generates typed TanStack Query hooks from
`api/openapi.yaml` (incl. the 6 routes 13.0 rescued — `useLogin`/`useMe`/`useBootstrap` now exist),
namespaced per tag, over a shared fetch mutator (same-origin, cookie creds, `X-Loomarr-Csrf`, RFC7807 →
`ApiError`). **packages/core** — the SSE invalidation bus (maps the BE's `title`/`channel`/`suggestion`/
`llm_pull` frames → coarse query invalidation, the §8 "GET is truth on reconnect" contract), zod schemas
(intent/bootstrap/login — reused by RN later), formatters. **apps/web** — Vite + React 18 + Tailwind v4 +
shadcn (new-york) with the AppShell nav rail, react-router skeleton, QueryClient + SSE providers, sonner.
The build embeds into the Go binary: **internal/web/embed.go** (`//go:embed all:dist`) serves the SPA at
`/` (SPA fallback for client routes; API prefixes guarded to 404, never HTML) with a committed
`.gitkeep` so `go build` works Go-only (serves a "run make fe" notice). Makefile gained
`fe`/`fe-tokens`/`fe-tokens-verify`/`fe-codegen`/`fe-install`. **Verified:** `make check` GREEN (added
SPA-served + guard assertions to `TestWiring_FreshInstall`; the absent-route contract shifted 405→**404**
uniformly via the SPA guard — invariant "absent ≠ 501" preserved), `make fe` GREEN (codegen + typecheck
4 pkgs + tokens/core unit tests + build), `fe-tokens-verify` GREEN. Toolchain decision recorded: **Node
20+/pnpm/Vite** (not bun/deno) — §14-decided, keeps the Expo bridge + CI determinism. Self-hosted Geist
deferred to 13.2 (visual-determinism concern; token font-stack falls back meanwhile). orval output is
gitignored (regenerated from the spec by `make fe`); token artifacts are committed (drift-checked).

**Phase 13.0 — BE contract closed for the FE (2026-07-17).** Branch `feat/be-contract-13.0`. The
prerequisite before any FE code (see `docs/frontend-build-plan.md`): make every route the FE calls
present + typed and every wizard/workspace surface fully BE-backed. Doc-first §7/§8/§13 updated, then:
**(1) OpenAPI coverage** — the 6 routes orval couldn't type (`auth/login|logout|me`, `setup/bootstrap`,
`users/import`, `users/sync`) were absent from the exported spec because `ExportOpenAPI` builds a bare
`Server{}` and the `register*` funcs nil-guard; added a `schemaOnly` flag (set only by export) so their
SCHEMAS emit into `api/openapi.yaml` while runtime nil-guarding is untouched (+296 lines, all additive).
**(2) setup/status completed** — was only `livetv` + `tunarr_library` (its own handler admitted "added by
their phases"); now aggregates the connection probes (`media_server`, `requester`, `tunarr`, `llm` incl.
local reachable+pulled / hosted key-present, `tmdb` via a cheap known-id lookup, `filler` when
configured) reusing the §8 `setup/test` registry, plus the `webhook` handshake check carrying per-app
`sonarr`/`radarr` `lastReceived` (read from the KV the ingest handler writes), each with a `docHref`
Troubleshooting deep-link. Added `llm`+`tmdb` probes to `connectionTests`. **(3) Suggester progress
events** (maintainer chose real per-step over indeterminate) — the worker published nothing intermediate;
now `Suggest` reports `searching`→`reasoning`→`scoring` via a **context-threaded** `ProgressFunc` (zero
signature churn across 13 call sites; a bare ctx = no-op), and the worker emits `done`/`failed` around it
through a narrow `ProgressEmitter` wired in the composition root to publish SSE `type=suggestion` frames
`{jobId, phase}` (parallel to `llm_pull`; the `eventEmitter` gained `SuggestionPhase`). Tests:
`TestProgress_ReportsOrderedPhases` (unit — clean run emits exactly searching→reasoning→scoring),
`TestSetupStatus_FullChecklist` (integration — connection checks carry docHref, webhook check waits with
empty lastReceived). `make check` GREEN; `make openapi` regenerated (commit makes `openapi-verify` green).
Closes findings 1–4 of the FE↔BE audit; finding 5 (coarse drop-tolerant SSE) is a documented property.

**Phase-13 FE plan reviewed vs the live BE — 5 seams found, plan written (2026-07-17).** Before writing any
FE, audited `frontend-design.md` + `design/` prototypes against the real BE contract (32 typed ops + 8
raw routes + the 3-type `title`/`channel`/`job` SSE bus). Found **5 FE↔BE seams**: (1) 6 core routes —
`auth/login|logout|me`, `setup/bootstrap`, `users/import`, `users/sync` — are **not in the exported
`openapi.yaml`** so orval can't type them; (2) `GET /v1/setup/status` returns only `livetv` +
`tunarr_library`, **not the full §13 checklist** (its own handler comment admits "added by their
phases" — they weren't); (3) **no read surface for webhook handshake timestamps** though the store
tracks them (`store.go:102,112`); (4) the **suggester emits no per-step progress** (worker publishes
nothing intermediate) so the mock's `GenerationProgress` steps are unbacked; (5) SSE is coarse +
drop-tolerant (a design property, not a bug — FE must invalidate-and-refetch). Also confirmed the design
is **already mobile/Expo-aware** (`frontend-design.md` §2.5/§4.2: shared `packages/{tokens,api,core}` +
token→NativeWind preset bridge; Expo+NativeWind+RN-Reusables pre-decided). **Maintainer decisions:**
(a) close the contract **first** as a **13.0 PR**; (b) mobile **ready now, build web only** (Expo app is
a future phase + a §14 update); (c) **add real per-step suggester progress events**
(`searching/reasoning/scoring/done/failed`). Full sequenced plan (13.0 close-contract → 13.1 monorepo +
tokens → 13.2 gallery + visual harness → 13.3 wizard → 13.4 surfaces → 13.5 gate), the page→endpoint→SSE
coverage map, and the mobile-auth flag (**native app needs CORS + per-user bearer tokens — a §11/§14
conversation, out of scope for 13**) live in **`docs/frontend-build-plan.md`**. No code yet; next action
is the 13.0 BE PR (doc-first §13/§8).

**Tunarr media-source auto-wiring (tunarr-connect) — onboarding gap closed (2026-07-16).** Branch
`feat/tunarr-autoconnect`. A live-smoke question ("is the Tunarr↔Emby wiring in onboarding?") found
a real gap: the design *required* Tunarr to have the media server as *its* source, enabled+scanned
(§6, Phase-0 #12), but nothing in the wizard did it — an operator could finish all-green yet get
**dead-air channels** (empty Tunarr program table). Maintainer chose **full automation**. **Design
win proven live:** Tunarr accepts Loomarr's **admin API key** as the source access token (verified
vs Tunarr 1.3.8 + Emby 4.10) — so **no Emby user login, no new credential, no §15 expansion**.
Delivered (doc-first §6/§7/§13): `programmer` gains `EnsureEmbySource`/`ConnectLibraries`/
`MediaLibrariesReady` (media-source CRUD + enable/scan of the movie+show libraries, idempotent);
`setup.MediaSourceConnector` orchestrates (resolves the admin userId via `library.ListUsers`, live
flavor/url/token); `POST /v1/setup/tunarr-connect` (admin, idempotent) + a `tunarr_library` check in
`/v1/setup/status`; wired in `internal/app` (the connector uses the same programmer the tests inject,
via a media-source interface type-assert); testkit Tunarr double implements the interface. Tests:
programmer unit (create-then-reuse idempotent, movies+shows-only enable, ready-gate), integration
`TestJourney_TunarrConnect` (real connector → double: connect → 2 libs → `tunarr_library` flips →
idempotent re-run), member-403 matrix + fresh-install 501 extended. **Dogfooded LIVE:** POST
tunarr-connect against real Tunarr returned the existing source **idempotently** (librariesEnabled=2)
and `/v1/setup/status` flipped `tunarr_library: ok=true`. `make check` green.

**Live BE smoke on the Mac — 2 findings (2026-07-16).** Branch `fix/picker-prober-live`. First
real drive of THIS session's changes against the homelab: native Ollama (qwen3.5:9b) + Emby/Seerr
over Tailscale, a FRESH app boot configured through the settings API (the wizard path, not env
pins). **Proven live:** the live-enable fix (fresh install `POST /v1/suggestions` → 501; PATCH
connections → **200 with no restart**); the real connection probes (`media_server` ListUsers +
the NEW `requester` `Seerr.Reachable`, both ok over Tailscale); features flip live. **Finding 1
(FIXED):** the model-picker's Prober base URL was **frozen at boot** (`llm.NewProber(set.str("llm.url"))`),
so configuring `llm.url` via the wizard left the picker reporting `reachable:false` / `pulled:false`
until a restart — the same class as the live-enable gap, and the integration harness missed it
because it seeds `llm.url` BEFORE build. Fix: `systemLLMService` builds the Prober per-call from a
live `ollamaBase()` resolver (like the suggester's Swappable hot-swap); regression added to
`TestWiring_ConfigEnablesLive` (picker reachable after PATCH); **verified live** (reachable=true,
qwen3.5:9b pulled=true after the PATCH). **Finding 2 (model-quality, documented — NOT a code/seam
bug):** qwen3.5:9b (the catalog's top-recommended local model) emits conversational prose instead
of the final JSON proposal (`invalid character 'C'`), so a real grounded job fails the JSON-repair
loop. `Think:false` is already applied on tool turns and the job failed **cleanly** (the graceful
"no valid proposal" path, which is unit-tested) — the SYSTEM operated as designed; the MODEL is the
weak link. Actionable: prefer a known-good local model (qwen3:8b / qwen3:14b Q6_K / llama3.1:8b) or
a hosted model; the catalog's qwen3.5:9b recommendation warrants review; a possible follow-up is a
final-turn `format:json` (tools dropped) to coerce weak models (doc-first, §8 grounding — separate).
**Phase 2 (a real Tunarr channel) not yet run** — needs the dev Tunarr wired to Emby (maintainer creds).

**E2E integration seams + composition-root testability + live-enable fix (2026-07-16).**
On branch `feat/e2e-integration-seams`. Pre-FE hardening: drive the WHOLE app (real composition,
not a hand-wired subset) through every FE-facing flow so the frontend meets a seam-free backend.
**Composition seam:** extracted `run()`'s 260-line wiring body into an importable `internal/app`
package — `app.BuildHandler(ctx, st, log, Overrides) (http.Handler, error)` — that both `run()`
(production) and the tests call, so tests exercise the REAL `api.Options` wiring. `cmd/loomarr/main.go`
shrank 710→133 lines (thin entrypoint); the package is split by concern (`app`/`systemllm`/
`settingsadapter`/`settingsboot`/`filler`/`adapters`/`emitter`/`ids`). `Overrides` injects the two
in-process boundaries (Tunarr `programmer.Programmer`, scripted `llm.Provider`) + a TMDB base override;
library/seerr are real adapters over testkit HTTP doubles via seeded settings. **New testkit
`Ollama`** HTTP double (`/api/version`,`/api/tags`,`/api/pull` stream) so the §8.1 picker
(probe→select→pull + SSE) runs through the real `systemLLMService`+`Prober`. **New E2E suite**
(`internal/integration`, real `app.BuildHandler`, testkit-only, in `make check`): a **new-admin
journey** (bootstrap→409-on-2nd→**local bcrypt login**→settings/feature-gates→`/setup/test` real
probe→picker probe/select(409-unpulled)/pull→intent→approve→channel-with-policy-enforcement→reconcile-
idempotent), a **member journey** (real import→media-server login→allowed set→**403 across the FULL
admin matrix** incl. settings/system-llm/setup/filler/backup that the old §19 test omitted→disable-
kills-session), and a **wiring** file (fresh-install 501/405 nil-dep matrix + the hot-apply proof), plus an **SSE
E2E** test (authenticated subscribe → pull → assert an `llm_pull` frame arrives — the FE's
live-update channel, previously only 401-tested). **Two pre-FE gaps a self-audit found, closed:**
(a) the wizard's "Test Seerr" button had NO backend — added `Seerr.Reachable` (validates URL+key,
no side effects) + the `requester` check in `connectionTests` + a testkit `/settings/main`
endpoint; the admin journey now drives all three probes (media_server/tunarr/requester); (b) the
SSE delivery test above.
**Live-enable fix (honors config-design §3 / §8.1 "no restart"):** the audit-flagged gap — a saved
connection flipped the `features` map but its route stayed **501 until restart** (services were
nil-wired at boot). Fixed by **always-constructing** the feature services (reconciler/channels/
suggester/filler, given a store) with the existing dynamic per-call providers, and moving each
handler gate from `s.X == nil` to a live check — `featureOff(ctx, feature)` (Features() snapshot) for
suggestions/filler, `unconfigured(key)` (live `set.str`) for search/channels/livetv, picker always-on.
The gate is **additive** (`nil OR live-off`), so the api-package unit tests (which wire deps directly,
no live source) are untouched. `TestWiring_ConfigEnablesLive` PROVES a PATCH to `/v1/settings` enables
`/v1/suggestions`+`/v1/search`+reconcile **with no restart**. **Known caveat:** the library *flavor*
is fixed at construction (defaults to Emby), so switching to Jellyfin still needs a restart — url/token
hot-apply; follow-up is a live flavor closure (~15 auth call sites). Gates: `make check` (`-race`, lint
0, config-docs) + `make test-pg` + boot smoke (fresh-install bootstrap 200, `/readyz` ready, clean
shutdown) all green. NOT a phase — pre-Phase-13 hardening; unblocks the FE build on a proven backend.

**LLM provider surface + pull-path fixes + Mac/Linux dev portability (2026-07-16).** Live dev
bring-up on an Apple-Silicon Mac surfaced two §8.1 pull bugs and drove a provider-surface decision
(all `make check` green). **Fixed:** (1) a model **pull aborted at 120s** — `Prober.Pull` used a
whole-request `http.Client.Timeout` (`TimeoutLLM`) that kills a multi-GB stream mid-body; added
`httpx.NewStreaming()` (no whole-request budget, ctx-governed; connect/TLS/header stages still
bounded) + regression test. (2) **pull progress now surfaces raw bytes** — exported
`llm.PullProgress{Status,Completed,Total}`; the `/v1/events` `llm_pull` SSE frame carries
`completed`/`total` so the FE renders "X of Y GB" + derives ETA (was percent-only). **Design
decision (doc-first, §8/§8.1):** the hosted LLM surface narrows to **OpenRouter** (the blessed
aggregator — one key → every frontier family) + **Custom** (a user-supplied OpenAI-compatible base
URL, gated by live validation, not an allowlist); the curated openai/anthropic/groq/gemini entries
are dropped (reachable via OpenRouter or Custom). Family-tier ranking unchanged. **Dev:**
`compose.dev.yaml` is host-agnostic now (`platform: linux/amd64`, `MEDIA_SERVER_IP` override); NVIDIA
transcode is an opt-in `compose.dev.gpu.yaml` overlay (`make dev-gpu`). Verified live: app native vs
Emby+Seerr+TMDB (Matrix grounding), Ollama on Metal, the §8.1 picker (probe→pull→select). A
cross-cutting fix/refinement, **not a phase** — Phase 13 (Web UI) is still next.

**Auth/identity rework (§11) — COMPLETE (2026-07-15).** On branch `feat/auth-rework` (commits
`4879470`..`4af00e2`), NOT yet merged to `main`. Replaced the claim-on-login / lazy-self-provision
model with **Loomarr-owned identity**: the `users` table is the source of truth + allowlist. Gate:
`make check` (`-race`, lint 0) + `make test-pg` (migration `00009` on both dialects) + `openapi-verify`
+ `config-docs-verify`, **plus a live boot smoke with ZERO media-server config** — `POST /v1/setup/bootstrap`
created the owning admin, a 2nd call 409'd, local admin login returned an HttpOnly/SameSite=Strict session
cookie, wrong password 401'd, and the users table held exactly one row (admin, bcrypt hash set). Delivered:
migration `00009` (nullable `users.password_hash` — set ⇒ local/bcrypt user, null ⇒ imported media-server
user, the credential-path discriminator); `login.go` enforces the allowlist (a name+hash verifies in-app,
else verify vs the media server AND confirm the id is imported — an un-imported user is **rejected even with
valid Emby creds**, no lazy provision; all failures return one `ErrInvalidCredentials`, no user enumeration;
works with a nil media server = local-only); `Provisioner.Bootstrap` (first local admin, once via
`CountAdmins()==0`) + `Provisioner.Import` (explicit media-server ids, admin-only, the ONLY add path);
`POST /v1/setup/bootstrap` (unauthenticated, self-gated) + `POST /v1/users/import` (admin-only);
`store.GetUserByName`. **Closed BOTH lazy-provision hatches:** login (`syncUser` add-branch removed) AND
periodic sync (`UserSync.Sync` now skips un-imported users — it refreshes, never adds, else a sync would
silently re-import everyone). `bcrypt` promoted to a direct dep (§14 updated). Existing auth/flow tests
updated to seed the allowlist first (a stricter contract, not weakened). Reworked doc §11 + reconciled
§13 wizard (Claim→Bootstrap + Import steps), §16, §19 test spec, §21 phase-9/13 gate text. Supersedes the
deferred `loomarr-auth-rework` memory item. Unblocks Phase 13's wizard "Bootstrap" + "Import users" steps.

**Settings subsystem — cross-phase config retrofit — COMPLETE (2026-07-15).** Built `config-design.md`
for real (the deferred Phase-1/8/9 config work) on branch `feat/settings-subsystem` (commits
`7aa3fcc`..`17fe3cb`). Gate: `make check` (`-race`, lint 0) + `make test-pg` (settings audit columns on
both dialects) + `make openapi-verify` + `make config-docs-verify` all green, **plus a live boot smoke**
(temp SQLite): `/healthz` 200, `/readyz` ready, three generated secrets minted + persisted with audit
stamp, `GET /v1/settings` 403 unauth / 47 settings with the API_TOKEN break-glass (secrets **masked**,
value withheld), env-pin reported + **refused** on PATCH ("set via environment"), `job.workers` hot-applied
to db, and the feature gate flipped `acquisition` true the instant `seerr.url` was saved — all with **no
restart**. Delivered: a typed **registry** (single source of truth, ~45 keys transcribed from §15),
`env > database > default` resolution with **asymmetric errors** (bad env → boot fail; bad db → self-heal +
caution), `_FILE` secret loading + `<VAR>`+`_FILE` ambiguity boot-error, in-memory snapshot + `Watch`
**hot-apply**, the secrets lifecycle (idempotent gen, `Redactor` into slog — the **log-grep gate** proves
no secret is ever logged, masked reads, regen side-effects), feature gating from `RequiredFor` (the
requester OR-gate is the one explicit case), the `/v1/settings` + `/v1/setup/test` + secrets-regenerate
API, and `make config-docs` (→ `docs/configuration.md`, drift-gated in `make check` too). New
`internal/settings` package; `config.Config` shrunk to the env-only bootstrap set (§1: `DATABASE_URL`/
`AUTO_MIGRATE`/`LISTEN_ADDR`/`LOG_LEVEL`/`TZ`). **Full read-through rewire** — every consumer resolves via
the snapshot (library/requester/Tunarr connection providers read PER CALL; `reconcile`/`channels` runners
gained `WithInterval` re-tune; the LLM `Watch(llm.*)`-rebuilds). Migration `00008` adds settings
`updated_at`/`updated_by` (2nd real ALTER after `00007`). Closes the ChannelPolicy registry-default
deferral: the `SCHED_*`/`SEASONAL_MODE` policy defaults (§15) now resolve through the registry, not Go
constants. Like ChannelPolicy, a **cross-phase retrofit** (deepens Phase 1/8/9), not a new phase-table row.
**Unblocks Phase 13's wizard-as-settings** (`config-design.md` §5–§7). NOT yet merged to `main` (branch
awaits review). Known small follow-up: `Router`/`ExportOpenAPI` still duplicate the route-registration
list (a shared `registerAll` is the real fix); the media-server/tunarr connection Test probes are shallow
reachability checks.

**Phase 12.5 — End-to-end integration (the seams) — COMPLETE (2026-07-14).** All live-smoke seams
closed: #6/#7/#8/#12/#13 (earlier), then #9 (acquisitions→`ch.Lineup` pending), #10 (provisioner→
scheduler `eventEmitter`), #11 (`/v1/events` SSE), and the §10 filler redesign (Loomarr-owned
commercials via a Tunarr `local` source + per-channel filler-lists). The Emby ~4s Live-TV playback
stop was a **Firefox** client quirk (no code change; troubleshooting note added). Phases 0–12.5 done;
**next: Phase 13 (Web UI + onboarding — recreate the imported `design/` prototypes pixel-perfect in
Vite+React+Tailwind+shadcn; gallery + fe-visual + axe gates).** Real captures earlier: Ollama tool-use
+ Emby SearchTerm shape.
Remaining follow-ups (non-blocking): (a) ~~live TMDB capture~~ **DONE** 2026-07-13 (key supplied →
`fixtures/tmdb/*`; adapter confirmed correct; live grounding smoke passed); (b) Anthropic LLM
provider (opt-in); (c) Archive.org downloader live HTTP walk (sidecar manual-smoke, stubbed);
(d) carried from Phase 6 — Sonarr `import_webhook.json` fixture (28GB re-download; Sonarr webhook
conn id 3 left up to catch it — remove after). Phase-0 findings:
[`docs/engineering/phase-0-findings.md`](docs/engineering/phase-0-findings.md). Deferred captures:
Sonarr Grab/Download → Phase 6; Emby login success body → Phase 9.

## Live manual-smoke findings — 2026-07-13/14 (maintainer's real stack)

First end-to-end run against the live homelab (Emby 4.10 + Sonarr/Radarr/Seerr over Tailscale
`100.75.125.45`, local Tunarr 1.3.8 with **RTX 3080 Ti `cuda`/NVENC transcode wired + verified**,
Ollama `llama3.1:8b` on GPU). The run drove intent → grounded suggester → approval gate → channel.
It surfaced a **chain of unwired seams** (two independently-correct subsystems, no wire between
them; unit tests pass because each side is tested in isolation). **Composition-root lesson:**
most of these live in `cmd/loomarr/main.go`, which builds the domain objects but never constructs
the adapters that connect them.

**FIXED this session (each with a regression test proven to fail against the old code; `make check` green):**

- **#6** `createChannel` ignored `intentRef` → channels built with an EMPTY lineup. Fixed: `internal/api/channel_lineup.go` (`lineupFromIntent` + approval-gated resolver) + `channels.go`. Tests in `channel_lineup_test.go`.
- **#7** program slots had `DurationMs: 0` → Tunarr rejects (`duration > 0`). Fixed: `schedule.Availability.Resolve` now returns `(itemID, durationMs, available)`; engine adapter fills it from `library.Client.ItemDurationMs` (RunTimeTicks); doc §9 updated. Tests in `schedule/lineup_test.go`.
- **#8** `approveProposal` only enqueued acquisitions → in-library picks never became `available` Records → unschedulable. Fixed: `internal/api/suggestions.go` now creates an `available` Record (with LibraryID) per in-library lineup pick. Test in `suggestions_test.go`.
- **infra #1** nonroot image + root-owned `/data` volume → SQLite `CANTOPEN`. Fixed: `loomarr-init` chown sidecar (sqlite profile) in `docker/compose.yaml` + doc §16.
- **infra #5** Tunarr 1.3.8 requires `channel.transcodeConfigId` = valid UUID (empty → 400). Fixed: `TUNARR_TRANSCODE_CONFIG_ID` now passed through `docker/compose.yaml`.

**OPEN — tracked follow-ups (own doc-first PRs; NOT started). Rooted in `cmd/loomarr/main.go` unless noted:**

- **#9 (SEVERE) — FIXED this session.** *acquisitions never entered a channel's `Lineup`.* `lineupEntries` dropped every non-in-library item, so an acquired title, once it landed `available`, was **never placed — not by event, not by the sweep** (the sweep re-derives desired from `ch.Lineup`, which permanently lacked the acquisition key). FIX (`internal/api/channel_lineup.go`): `lineupEntries(p suggest.Proposal)` now builds an entry for **both** `p.Lineup` **and** `p.Acquisitions` (in-library first, then acquisitions; de-duped by `provision.Key`), dropping the `InLibrary` gate entirely — availability is decided at reconcile time by `resolveEntry` against the live library (§9), not by the proposal's possibly-stale flag. A stale `InLibrary:true` whose media is gone resolves to a pending slot (maintainer decision, matches §9 "revalidate at reconcile time"). So an acquisition enters `ch.Lineup` as a pending slot on create and swaps to a program **in place via the 10-min sweep alone** when it lands (#10's event path is now pure latency, not correctness). Regression test `TestCreateChannelBindsAcquisitionsAsPendingEntries` (`channel_lineup_test.go`) — **proven to fail against the old drop logic** (1 entry, not 2). `make check` green. Complements the #8 approve path (`suggestions.go`): approve enqueues the acquisition as `wanted` AND the channel now holds the pending entry; they rendezvous on the `Key` when the webhook flips it `available`.
- **#10 — FIXED this session.** *provisioner availability events → scheduler feed was `nil`.* Both `reconcile.New(…, nil, …)` and the ingest handler only logged terminal events; `Engine.OnAvailability` had zero non-test callers. FIX: one composition-root `eventEmitter` (`cmd/loomarr/main.go`) implements the emitter port for **both** the reconciler (existing `reconcile.Emitter`) and the ingest handler (new local `ingest.Emitter` — accept-interfaces idiom, structural typing, no cross-pkg import; maintainer decision). It fans each `DomainEvent` to `engine.OnAvailability` (backfill the referencing channels) AND `eventBus.Publish`. The engine is wired via `setEngine` after construction; the field is an `atomic.Pointer[channels.Engine]` since the reconciler goroutine (started earlier) reads it on the hot `Emit` path while setup writes it once — `-race` clean. A nil engine (pre-wire / scheduler unconfigured) still reaches the bus; never load-bearing (sweep is the backstop). Regression tests: `ingest.TestImportEmitsAvailabilityEvent` + `reconcile.TestReconcilerEmitsTerminalEvents` (both emit sources: webhook confirm→available, missed-webhook→available, give-up→unavailable; non-terminal ticks emit nothing). Now that #9 carries acquisition keys into `ch.Lineup`, this makes backfill event-driven (sub-sweep latency) rather than sweep-only.
- **#11 — FIXED this session.** *`GET /v1/events` SSE never emitted.* Zero `.Publish(` calls existed; subscribers waited forever. FIX: the same `eventEmitter` publishes a `title` frame (`{key,state,name}`) to the bus on every terminal transition, so `/v1/events` delivers state changes. Regression test `cmd/loomarr.TestEventEmitterPublishesToBus` (domain event → `title` frame reaches a subscriber; also asserts the nil-engine path is safe). Latency-only (`GET /v1/suggestions/{id}` etc. stay source of truth). App boot-smoked: `/healthz` 200, `/readyz` 503 (no store), clean shutdown with the new wiring.
- **#12 (was the smoke blocker — now DIAGNOSED + a channel proven to play)** — *Tunarr manual-programming content-id contract.* Two parts:
  1. **Setup (was the actual blocker):** Tunarr's Emby libraries were **not enabled/scanned** — so Tunarr's program table was empty and *any* content add (UI or API) FK-failed. Fix is operator setup: `PUT /api/media-sources/{id}/libraries/{libId} {enabled:true}` then `POST …/scan`. Enabling Movies+TV indexed **2205 movies**. Belongs in the Phase-14 ops runbook + `docker/tunarr-dev-setup.md`.
  2. **Adapter fix (real, still OPEN):** `internal/programmer/lineup.go:91` sends `{type:"content", id: LibraryItemID}` with the raw **Emby** id. Tunarr's programming `id` must be **Tunarr's own program `uuid`**, obtained by matching our pick against Tunarr's indexed catalog **by TMDB id** (`/api/media-libraries/{lib}/programs` → each program carries `identifiers[{type:tmdb},{type:emby}]` + its `uuid`; or `POST /api/programming/batch/lookup {externalIds:["emby|<id>"]}` once indexed). FIX: resolve slot → Tunarr uuid before the programming push (a Programmer-side lookup keyed on tmdb).
  **FIXED IN CODE (not around it):** the programmer adapter now resolves media-server item id → Tunarr program uuid via a cached index of Tunarr's persisted `/programs` (doc §6; `internal/programmer/resolve.go`; unindexed item → flex, never dead air). **PROVEN through Loomarr end-to-end:** a movie channel and a **series** channel both built by Loomarr's own reconcile (no hand-rolled scripts) and streamed 1920×1080 H.264/AAC with the RTX 3080 Ti transcoding (NVENC 74% / NVDEC 92%). The manual-smoke half of the DoD is met.
- **#13 (NEW, FIXED) — series support in the scheduler.** A series lineup pick is one show id with no runtime and no single Tunarr program; the scheduler now **expands a series entry into one program slot per episode** (`internal/library/episodes.go` `ListEpisodes`; `schedule.Availability.ResolveEpisodes`; `ComputeDesired` expands then orders by strategy — sequential = episode order, shuffle = shuffled). Doc §9 updated. **PROVEN:** the "90s Sitcoms" channel expanded 5 approved series → **720 episode programs (297h)**, shuffled, resolved, pushed, streaming — all through Loomarr's reconcile.
- **METHOD NOTE (maintainer feedback):** earlier in the session I sidestepped #12/#13 with Python scripts that called Tunarr directly and called the channel "working" — that tests Tunarr, not Loomarr, and hides the bug. Corrected: fix the app, drive the app. Both fixes above are proven through Loomarr's own endpoints. (Memory: `loomarr-test-the-app-not-around-it`.)
- **lesser** — #3 split-host: no *advertised* Tunarr URL distinct from `TUNARR_URL` (Emby must fetch the m3u/xmltv from a media-server-routable host; §15 gap). #4: `llama3.1:8b` curates weakly (themeFit 0 on a real intent) — bigger local model or the deferred Anthropic provider. `deleteChannel ?purge=true` accepted but unimplemented (only detaches; `internal/api/channels.go`). Reconcile create-conflict on channel *number* isn't adopted (re-create attempt collides) — hardening.

An audit agent traced both ends of every design-doc "X feeds/drives/on-event Y" claim and **confirmed correctly-wired**: filler catalog-sync → pod assembly; webhook → confirm-via-library → `available`; guide-poke after channel-affecting reconcile; slot drift-revalidation on the sweep; the three fixes above.

## Phase table

| Phase | Status | Gate evidence (commit SHA + proving command) | Notes / deviations |
| --- | --- | --- | --- |
| 0 · Contract spikes | evidence pinned | Fixtures in `internal/testkit/fixtures/*` + `api/vendor/tunarr-openapi.json`; index: `docs/engineering/phase-0-findings.md` | Tunarr 1.3.8 (CRUD ✓, key optional), Radarr 6.2.1 full lifecycle ✓, Sonarr 4.0.19 Test ✓, Emby 4.10 authed ✓, Seerr 3.2.0 requester ✓. **No §6/§9 deviations.** Deferred: Sonarr Grab/Download (P6), Emby login-success body (P9). |
| 1 · Scaffold + harness | done | `make check` green (vet + golangci-lint v2.12.2 0 issues + `go test -race`); `docker build` → 8.31MB distroless image serving `/healthz` 200 as nonroot | Module `github.com/mantonx/loomarr` (go 1.26), MIT. `config`+`httpx`+`api`+`testkit`+`cmd/loomarr`; `/healthz`+`/readyz`+graceful shutdown. Makefile contract (unimpl targets fail loudly). Dockerfile (distroless static, cgo-free), `docker/compose.yaml` (sqlite/postgres/ai/filler). `.env.example` covers all §15. golangci v2 config mirrors nexus-open. |
| 2 · Provisioner domain + state machine | done | `make check` green; `go test ./internal/provision/` — Key derivation, **webhook-key parity vs real Radarr fixture**, happy path, + all 5 §4 invariants (terminal monotonicity, emit-only-terminal, idempotent no-op, library-is-truth, deadline discipline) | Pure domain, no I/O. `Title`/`Key`/`State`/`Record` (§3); `Apply(rec, ev, now) → (Record, []DomainEvent)` (§4) — clock passed in for determinism. Illegal transitions are no-ops, not errors. |
| 3 · Store + SQLite | done | `make check` green (`-race`); `go test ./internal/store/` — conformance suite (round-trip, upsert-idempotent, not-found, list, **ClaimDue + concurrent claim**, settings) + downgrade guard + unknown-scheme; app boots on real SQLite, migrates, `/readyz` ready | `Store` iface (§5); shared `database/sql` impl (one path, `?`↔`$N` rebinding); `modernc.org/sqlite` WAL+busy_timeout, single-conn; goose embedded per-dialect migrations + **startup downgrade guard**; **`ClaimDueTitles` leases rows (deadline→now+lease)** so concurrent callers/replicas never double-claim — SQLite guarded UPDATE, PG `FOR UPDATE SKIP LOCKED`. Conformance is one suite, backend-agnostic (Phase 4 reuses it). |
| 4 · Postgres backend | done | `make test-pg` green (testcontainers `postgres:16-alpine`, 3.3s) — **same conformance suite**, incl. `ClaimDueConcurrent` under real `FOR UPDATE SKIP LOCKED` (passed 5× under `-race`); app boots + migrates on dev-compose Postgres, `/readyz` ready | `pgx` stdlib shim, `$N` placeholder rebinding, PG per-dialect migrations. Postgres test behind `//go:build integration` so default `make check` needs no Docker. Concurrent claim is the meaningful case here (SQLite serializes; PG genuinely races). |
| 5 · Library adapter | done | `make check` green; `go test ./internal/library/` — Lookup present/absent (pinned fixtures), **both-flavor token headers** (Emby `X-Emby-Token` vs Jellyfin `MediaBrowser`), **both-flavor login headers**, auth success + **§11 bad-pw 401 negative path**, ListUsers, SeasonPrecision | Shared Emby/Jellyfin `Client` (one impl, flavor differs only in auth headers — auth.go); `Lookup`/`AuthenticateByName`/`ListUsers` (§6, §11) via `httpx` 10s timeout; `SEASON_PRECISION` series(default)/seasons policy. Testkit `MediaServer` mock serves pinned fixtures + captures headers (both flavors, CLAUDE.md). Login-success body synthesized (real capture deferred to P9). |
| 6 · Requester + ingest | done | `make check` green; `go test ./internal/{ingest,requester}/` — Seerr 201/OK/409-success + 500-fails + no-TMDB; `/hooks/arr` bad-secret 401, Test→200+timestamp, Grab→downloading(+deadline reset), **Import→available ONLY after library confirms (inv. 4)**, untracked-ignored, malformed 400; app wires `/hooks/arr` end-to-end | Seerr requester (`X-Api-Key`, 201-with-existing-media path per P0); `/hooks/arr` handler maps Grab/`Download`(quirk)/Test via Phase-2 keys + Phase-5 Lookup; constant-time secret. **Sonarr Grab+Test captured live** (`sonarr/{grab,test}_webhook.json`); import fixture pending a 28GB re-download (webhook conn left up to catch it) — import *logic* already tested via Radarr's real import fixture. |
| 7 · Reconciler + janitor | done | `make check` green (`-race`) + `make test-pg` re-green (claim SQL changed); `go test ./internal/reconcile/` — retry-wanted (success/fail), **missed-webhook re-check→available**, **deadline give-up→unavailable+Cancel**, not-due/terminal untouched, janitor runs-all/failure-nonfatal; app starts+ticks+**clean shutdown** verified | Ticker `Runner` → `Tick` claims due batch → per-title retry(wanted)/give-up(in-flight w/ library re-check). Janitor scaffold + `Sweeper` iface (targets registered by P9/P11). Requester gains `Cancel`. **2 bugs caught+fixed:** ClaimDue excluded `wanted` (fixed both dialects); lease/deadline interplay blocked give-up. Also fixed shutdown ordering (cancel reconciler before drain). |
| 8 · Self-documenting API | done | `make check` green (`-race`) + `make openapi-verify` green (drift guard); `go test ./internal/api/` — OpenAPI 3.1 + State-enum=code, enqueue(admin)/idempotent, mutation-requires-admin 403, list-needs-state 400, get 404, delete→unavailable, backup 200 SQLite-magic + admin-only, docs-offline; app serves all routes end-to-end | Huma v2 on `humago` (§7.1); `/v1/titles*` (idempotent enqueue, admin-gated POST/DELETE), `/openapi.{json,yaml}`, offline `/docs` (no CDN), `GET /v1/backup` (SQLite `VACUUM INTO` stream; PG→501). Auth seam: `API_TOKEN` Bearer authorizer (Phase 9 adds sessions). `make openapi`→committed `api/openapi.yaml` via `cmd/openapi`. Streaming backup is a plain mux handler. `/v1/events` SSE deferred to Phase 11 (event bus). |
| 9 · Users & auth | done | `make check` green (`-race`) + `make test-pg` (users/sessions schema; SQLite-INTEGER↔PG-BOOLEAN) + `openapi-verify` green. **Gate tests:** `go test ./internal/{auth,api}/` — token-hashed-at-rest, resolve, **disabled-user-session-dies**, **disable-revokes-sessions**, bootstrap roles, bad-pw, rate-limit; HTTP: **member 403 on admin routes (§19)**, admin allowed, **session-dies-on-disable end-to-end (§19)**, CSRF-required, HttpOnly+SameSite=Strict cookie, logout, break-glass token, user-sync admin/member-403 | Sessions (256-bit token, SHA-256 at rest, revocable rows); `/v1/auth/{login,logout,me}` + `/v1/users{,/{id},/sync}`; first-admin bootstrap; login rate-limit (x/time/rate); session janitor sweeper. Fills the Phase-8 `Authorizer` seam (session cookie + API_TOKEN Bearer break-glass). **Real Emby login-success body captured** (`emby/auth_success_response.json`, scrubbed) — validated the mock shape. |
| 10 · Scheduler + Tunarr | done | `make check` green (`-race`, lint 0) + `make test-pg` (channel round-trip/list/delete + **ClaimDueChannels concurrent** under real `FOR UPDATE SKIP LOCKED`) + `openapi-verify` deterministic; app boots with the channel scheduler + sweep, `/readyz` ok, clean shutdown. **Gate tests:** `go test ./internal/{schedule,programmer,channels,setup,api}/` — pure desired-lineup (3 strategies, deterministic shuffle, pending policy); Tunarr adapter server-assigns-id + slot↔lineup + 400-on-empty vs pinned fixtures; **reconcile create→idempotent-no-op, backfill-on-event, event-loss-recovery-via-sweep, drift-substitution, guide-poke-only-when-affecting**; **idempotent livetv-connect second-call-no-op** (unit + over-the-wire); §19 auth negatives (member 403 on create/reconcile/connect/status) | `internal/{schedule,programmer,channels,setup}` + `store` channels (migration `00003`, `ClaimDueChannels` per-channel lease §18) + `library` Live TV wiring + `/v1/channels*` + `/v1/setup/{status,livetv-connect}`. Server-assigns-channel-id honored. **Fixed a pre-existing exporter spec-drift gap** (auth/users routes were never in the committed spec). Added `TUNARR_TRANSCODE_CONFIG_ID` to §15 doc-first. **Live capture DONE** (maintainer-supervised, reversible register→capture→delete vs real Emby 4.10.0.17; Emby verified reverted): `internal/testkit/fixtures/livetv/*` + `FINDINGS.md`. Pinned truth: m3u tuner `{Type,Url}`✓, xmltv provider uses **`Path`**✓, delete via `?Id=`, and **guide-refresh resolves the per-install task Id by the stable Key "RefreshGuide"** (`/ScheduledTasks/Running/<id>`; Key form 404s) — adapter corrected + tested (`library/livetv_test.go`). M3U registration is fetch-validating (unreachable URL → 500), so the real connect is manual-smoke (§21). Fixtures scrubbed of IPs/zip/device-id. |
| 11 · Suggester | done | `make check` green (`-race`, lint 0) + `make test-pg` (jobs/proposals schema; **ClaimDueJobs concurrent** under real `FOR UPDATE SKIP LOCKED` + intent-hash cache) + `openapi-verify` deterministic; app boots with the suggester + worker pool (Ollama provider), `/readyz` ok, clean shutdown. **Gate tests:** `go test ./internal/{catalog,tmdb,llm,suggest,api}/` — **GROUNDING: fabricated-title-never-reaches-proposal**, in-library→lineup vs acquisition classification, acquisition re-validated vs TMDB (404 drops), cap→alternates, deterministic scoring; worker: cache-dedup, job-runs+proposal-persists, **hung-LLM hits JOB_TIMEOUT while pool keeps draining**; **APPROVAL GATE: member approve→403 with ZERO titles enqueued, admin approve→acquisitions become wanted titles (the only proposal→/v1/titles path)**; search any-auth + q-required; events auth-required | `internal/{catalog,tmdb,llm,suggest,events}` + `store` jobs/proposals (migration `00004`, `ClaimDueJobs` lease §18, intent-hash cache) + `library.Search` + `/v1/suggestions*` + `/v1/search` + `/v1/events` SSE. **Real captures:** Ollama tool-use round-trip (`fixtures/llm/*`, arguments-as-object/format:json pinned) + Emby `SearchTerm` shape (`emby/search_matrix.json`). Grounding chokepoint: LLM proposes ONLY via catalog_search; a pick survives only if the tool surfaced its id. Scoring theme-first (0.5/0.35/0.15, maintainer). **TMDB captured + live-verified 2026-07-13** (`fixtures/tmdb/*` + FINDINGS; adapter was already correct — /search/multi + exists 200/404 confirmed; `tmdb/fixture_test` parses the real shape). **Live grounding smoke passed:** federated `/v1/search?q=matrix` against real Emby + real TMDB + Ollama — The Matrix trilogy `in_library=true` (from Emby), TMDB-only titles `in_library=false`, deduped-by-id once. **Deferred (non-blocking):** Anthropic provider (opt-in). |
| 12 · Commercials & filler | done | `make check` green (`-race`, lint 0) + `make test-pg` (clips schema + filter/tags/prune conformance) + `openapi-verify` deterministic; app boots with the pod assembler wired into the scheduler + periodic filler sync (sync 401 non-fatal → degrades to flex). **Gate tests:** `go test ./internal/{filler,channels,ingestkit,library,store,api}/` — **POD-MATCHING: seeded-deterministic, era/audience match, category-variety (no back-to-back), no-repeat-in-window, PodMax density, fallback ladder (exact→widen→audience→embedded bumper card, never dead air)**; **FILLER-NEVER-A-PROGRAM** (structural + explicit test); catalog sync **tag-preservation-on-resync** + idempotent + prune; **AI tagging grounding** (hallucinated enum/year dropped, never persisted); scheduler pod-fill (gaps→matched pods, programs untouched, deterministic seed, no-pods=flex); filler API admin negatives (member 403 on patch/sync/tag); sidecar dispatch/resilience | `internal/filler` (Clip + pure Assemble + sync + Classify/Tagger + PodAdapter) + `store` clips (migration `00005`) + `library.ListFillerClips` (duration from server RunTimeTicks; §10 core-never-probes) + `/v1/filler*` + `cmd/loomarr-ingest` sidecar (Go, `Dockerfile.ingest` bundling yt-dlp+ffmpeg, `filler` compose profile — core has no download config). Filler is a parallel universe to provisioning (clip identity = media-server item id). |
| 12.5 · End-to-end integration (the seams) | **done** — `go test ./internal/integration/` (`TestPipeline_ApproveToChannelWithProgramsAndPodBreaks`) + live manual smoke | Gate: an integration test (intent→suggest→approve→create→reconcile→ pushed Tunarr lineup has real programs **with pod breaks**) + the live manual smoke. **Both met:** the integration test wires the REAL domain objects (store, channel engine, real store-availability, real filler pod assembler, real approval path) and drives approve→create(intentRef)→reconcile through the real HTTP API (only Tunarr faked via the testkit double) — asserting the pushed lineup has ≥3 real program slots, flex break gaps, AND a grounded filler-list attached (commercials), plus second-reconcile idempotency (0 pushes, 0 filler writes). **Proven to FAIL when a seam regresses** (disabling the filler-list attach → "no filler-list attached"; disabling the lineup push → "no lineup pushed"). Runs under `make check` (testkit only, no network, §19). | **Why this phase exists:** the 2026-07-13/14 live smoke proved phases 0–12 were gate-green in isolation but had unwired seams (per-phase unit gates never test the composition). **DONE en route (live-smoke commits `dc14f40`, `ac79a80`):** #6 create-binds-lineup, #7 duration, #8 in-library→available, #12 Tunarr content-id resolution, #13 series→episode expansion — a movie channel AND a 297h series channel built by Loomarr's own reconcile, streaming GPU-transcoded 1080p. **#9/#10/#11 + commercials CLOSED this session** (#9 acquisitions now enter `ch.Lineup` as pending entries + sweep places them; #10 one composition-root `eventEmitter` fans terminal events to `engine.OnAvailability` from both the reconciler and the webhook ingest handler — backfill now event-driven; #11 same emitter publishes `title` frames so `/v1/events` SSE delivers. **Commercials — §10 filler redesign implemented:** filler is now Loomarr-owned via a Tunarr `local` media source + per-channel filler-lists (media server out of the filler path). Clip identity media-server-item-id → **Tunarr program uuid** (migration `00006`, forward-only drop+recreate empty); sync reads Tunarr's local-source `/programs` (was Emby); `PodFiller.FillGap`→`BuildFillerList` (per-channel pool, not per-gap); `reconcile.fillPods`→`attachFillerList` (break gaps stay **flex**, Tunarr fills from the list); `Programmer.EnsureFillerList` builds+attaches the list, **content-based idempotent** (compares the actual program set, not count — a review caught a count-only bug that would freeze commercials on a re-tag). `FILLER_LIBRARY`→`FILLER_DIR` (§15 doc-first). New `programmer/filler.go` + tests; `make check`/`make test-pg`/`openapi-verify` green.) **Emby ~4s Live-TV playback stop RESOLVED 2026-07-14:** root cause was a **Firefox** client-side playback quirk (NOT Loomarr/Tunarr/Emby backend, NOT the earlier-suspected Simkl plugin) — the backend stream is healthy; it plays fine in another client. No code change; captured as a troubleshooting entry (`docs-livetv-integration.md` → folds into the Troubleshooting page in Phase 14). **Phase 12.5 COMPLETE — all seams closed; Phase 13 (UI) unblocked.** See design §21 phase 12.5 + memory `loomarr-filler-redesign`/`loomarr-wiring-backlog`. |
| 13 · Web UI + onboarding | done | 13.4e gate `887f833` (#35). `make check` + `openapi-verify` GREEN; `make fe` GREEN (217 app / 24 core / 9 api); `make fe-visual` GREEN (188, zero flaky); `make e2e` GREEN (5). **Gate tests:** `reachability.test.tsx` (every generated route renders real content, every feature-gated panel appears when its flag is on — DERIVED from `router.routesById`, verified to fail when a panel's mount is removed); the **approve-flow e2e** as §7 under test (admin approve enqueues; member is offered no approval and nothing is enqueued — §19 negative, asserted on the mock's recorded state, verified to fail when the `isAdmin` gate is deleted). | Vite React+TS + orval hooks + TanStack Router/Form, embedded SPA (`internal/web`). Built 13.1→13.4e: design-system + token pipeline (#5, #7) → Storybook gallery + visual/a11y harness (#8) → auth foundation + wizard (bootstrap, checklist, steps 3–7) (#11, #12, #16, #17) → the 8 surfaces: **Suggest** + approval queue (#18), **Channels**/detail/**Board** (#19, #20), **Settings** (6 pages, one save bar each, §8.1 model picker, secrets panel) (#21, #23), **Users** (#32), **Filler** (#33), **Help** + ⌘K palette (#34), per-user quotas (#36). **Recurring lesson (13.4d/e):** seven things were built, unit-tested, and mounted by *nothing* — the reachability gate is what this phase earned. **Deviations, doc-first:** ProposalDTO + submit body typed from the domain (orval 1:1, no hand-mirror — #9, #18); react-hook-form → TanStack Form (§14, #15); Settings IA = 6 pages per config-design §5 (build plan said 5). Visual seed (`design/*.dc.html` + `loomarr-frontend-design.md`) recreated with the two §7 deltas (badge `-300` stops; `static-500` disabled-only). |
| 14 · Docs, harden & ship | done | `make check` + `openapi-verify` GREEN at `0a4a7f2`. **§21 DoD, both halves met:** automated (`make check`/`make e2e`/`make smoke` — compose sqlite+postgres start clean, member→template→grounded proposal→admin-approve→reconcile creates the channel with pods+filler, Import backfills, Help/`/docs`/OpenAPI 3.1 all resolve) + **manual smoke on the maintainer's real stack** (`5cdf6a9`, `60746c1`: wizard all-green → intent → approve → an 80-program kids channel PLAYING in Tunarr with ad pods, real Emby+TMDB+Ollama; `make smoke-livetv` wires+destroys a disposable Jellyfin). | `docs/help/` set (Quickstart, Integrations, Concepts, Member, Filler, Troubleshooting) embedded + served at `/v1/docs` (`a50c57b`, deep-links `eb09813`); seed docs folded into `docs/` (`62d9369`); README Documentation + Operations sections (`f46ddf9`). `internal/metrics` + `/metrics` (unauth, §7): **§17 closed end to end** — RED (`040a127`) → state gauges + login/webhook counters (`5fb073e`) → outbound-client latency + reconcile timing (`07d2359`) → LLM tokens + filler-ladder depth + slot drift (`299b0aa`). CI + release pipeline + community health (`beaa741`); LICENSE + third-party notices + UI-less image fix (`1a10cac`). **9 bugs the maintainer smoke found** (seams between gate-green subsystems) fixed en route (`8a732b5`). **Accepted deltas vs §21 checklist:** runbook lives as the README **Operations** section (no dedicated file); **no shipped dashboard JSON** — cost is deliberately a dashboard recording-rule over the token series, not a baked-in metric (per the metrics entry above). |

## v2 program (post-14) — the mock delta

Phases 0–14 shipped the app. The **v2 program** reshapes it against the v2 mocks and one large
architectural change. Plan: **`docs/engineering/v2-build-plan.md`** (39 phases, one PR each, gates
per phase). Evidence: `docs/engineering/v2-mock-delta-2026-07-24.md`.

**The headline decision (D-I):** Loomarr stops being *"not a transcoder/streamer"* (`design.md:39`)
and plays out its own channels. That reframes Track T from a parallel track into the spine —
**V2b alone blocks 28 of the 39 phases**.

| Phase | Status | Gate evidence (commit + proving command) | Notes |
| --- | --- | --- | --- |
| — · Delta research + build plan | done | `a4f183d` (#68) — 4 CI jobs green | Both v2 mocks read in full (markup **and** the 190KB state block; an MCP fetch had silently truncated 48% of the desktop file). Mocks were generated **from this repo**, so much apparent delta is the mock reflecting shipped code. Register re-verified at HEAD: **5 entries already fixed**, C3 narrowed, **4 new defects (A1–A4)**. Phase graph verified mechanically: acyclic, no dangling refs, all 27 defects mapped. |
| V2b · D-I design amendment | done | `3b0bca2` (#69) — `make check` green; docs only | §1 playout moved out of Explicit non-goals; **new §9.1** (playout backends, per-channel `playout.backend`); §10 mid-roll **in scope** for internal playout (opt-in detection per channel); §11 **device auth for segments** (a TV holds no session cookie — the one path that isn't a person); §14/§16 ffmpeg core + ffprobe back + **one 549MB image** (18× the old default). §20: two dead bullets struck — **closes S12**. Swept the *consumers* too (10 stale `loomarr:filler` refs in endpoint tables, feature gates, env docs, compose) — amending only the defining section is how §20's dead bullets survived. |
| V0 · `DATABASE_URL` envDefault | done | `60cc6a6` (#70) — `go test ./internal/config/`, verified failing without the fix | `design.md:644` promised the default; the struct tag never carried it, so a bare `docker run` came up **store-less and never-ready**. Survived because compose sets the same value. **Closes S1.** |
| V1 · Mount `ChannelIconField` | done | `5fefd0c` (#71) — reachability assertion, verified failing when unmounted | The component shipped complete (stories, 5 visual baselines, admin gate) and was **imported by nothing** — the eighth instance of the bug `reachability.test.tsx` exists for. Mounted on Overview (the icon is what the family sees in the guide). Also corrected the §12 surface map, which claimed a home (`Settings → Identity`) with **no implementation behind it** — a row asserting a UI that doesn't exist makes the audit look clean. **Closes D-H.** |
| V2 · `C3′` + `C10` + `A4` | done | `ae93332` (#72) — separation-delta test verified failing without the fix | **C3′:** `MergeFromProposal` refreshes `separation` like the other four policy fields, but `policyDeltas` didn't diff it — a refine could widen or drop a no-repeat window with nothing shown before Approve. Duration comparison tidies zero-unit padding (`168h` vs `168h0m0s`) or an unchanged window reads as a diff. **C10:** deleted the dead `channel-pods` component — writing the story the coverage gate asked for would have gone green *and kept the dead code*. **A4:** two `TODO(learning)` docblocks describing unimplemented functions sat above complete implementations (`fillCommercials`, `composite`); none remain in the tree. |
| V17a · Read the clip sidecars (`F1`) | done | `2eaaca4` (#73) — 4 tests, all verified failing with the old argument | Every AI-tagged clip's prompt read **`Source description: tunarr-local`** — `sync.go:131` sets a PROVENANCE enum and `tagjob.go` passed it as the *description*, so the classifier got a misleading constant while the real title/description sat unread. Both ingest paths were already writing info-JSON sidecars with a comment saying *"AI tagging reads, §10"*; nothing parsed them. New `internal/filler/sidecar.go`. **`upload_date` deliberately NOT used for Era** — it's when a clip was uploaded, not when it aired. Testkit gained `LLM.LastMessages`/`Prompt()`: the defect lived in the PROMPT, which no output assertion could catch. |
| V23 · Deny-reason UI (`A1`) | in review | PR #74 — 4 tests; visual verified against a REBUILT storybook-static | `denyInput.Body.Reason` has always existed, persisted, and been **rendered** by `ApprovalQueueItem` with a story and a test — but all three call sites sent `data: {}`, so it was always empty and a denied member learned nothing. Deny is now two-step (arm → reason → send); **Approve stays one click** — approving needs no explanation, declining is where someone is left guessing. Reason is optional; empty sends `undefined`, not `""`. |

| V6 · Internal playout to first frame | **done** | `2b0b9a4` … `c6aa0e7` — `make check` + `make test-ffmpeg` green; **verified on the maintainer's own Emby** | Loomarr serves its own channels. Emby pulls `/playout/tuner.m3u`, real library films play at 1080p, breaks cut to real commercials, and the full transcode runs on the GPU. Mechanism is Tunarr's HTTP ffconcat loop (prior-art §1): a `-c copy` parent over a 2-line playlist whose entries both resolve to "what's on now" — the demuxer's EOF-and-advance IS the program boundary, so there is no splicing code. Mid-program tune-in verified live (joined 39 min into a film). |
| V6b · XMLTV listings | **done** | `ac17450`, `c8ed906`, `5382fda`, `7fccc63` — `make check` green; live guide 253 programmes / 541ms | `/playout/guide.xml`. Same `CyclePreview` the encoder reads, so listings cannot drift from playout (a test samples 200 instants and asserts they agree). Breaks are **not** advertised (#12). Full metadata: `<desc>` 253/253, `<date>` 253/253, `<category>` 249/253, `<rating>` 247/253 — one bulk `/Items?Ids=` call, 120 items in 24ms, so no cache was needed. |

| V13b · `GET /v1/guide` | **done** | `eeb6338`, `ec8436c` — `make check` + `openapi-verify` green; live 38 blocks with provenance + runtime on all | The JSON time-grid backend. `kind` replaces `gap bool` (a boolean could not tell a commercial pod from a pending acquisition); gaps preserved, unlike `Upcoming`. All 8 gaps in the mock-delta §2f closed — incl. per-airing pod composition, episode runtime, server-assembled provenance, `guide.timezone` + `guide.retention_hours` (§15). |
| V14a · Guide time-grid UI | **done** | `431fd32`, `038b17f` — `make fe` green (460 tests); rail/chips verified by screenshot | The grid itself, built to `-v2.dc.html`. Flex rail + percentage blocks, so ruler/block misalignment is structurally impossible. GuideDetailCard, per-clip pod rendering, airing highlight, channel marks, health chips, row menu, day/window controls. **The IA rename (`/channels`→`/guide`, `/users`→`/people`, two navs) is deliberately NOT here** — see below. |

| V14b · IA rename + two navs | **done** | `a6ac496` — CI green; 461 unit + 342 visual + 7 e2e | `/board`→`/queue`, `/users`→`/people` (git mv). **Two AUTHORED navs** replace `NAV.filter(i => !i.admin \|\| isAdmin)`: a member gets `Guide · Request a channel · My requests · Help` — the same `/suggest` and `/queue` routes under member-facing names, which a filter structurally cannot do (it hides, never renames). `NavItem.admin` deleted. `/v1/users/*` unchanged — this is a frontend rename, not an API one. |

**Next up (free — no deps, no pending decisions):** V7 (local accounts — closes S2/S3, the largest
remaining free phase), V19 (per-title refine rationale), V24 (`A3` — surface proposal data the DTO
already carries but nothing renders: `channelName`, `eraBalance`, `overall`, and the
`mustInclude`/`mustExclude` intent inputs).
**The spine is COMPLETE:** V3 → V4 → V5 → V6 → V6b → V13b → V14a → V14b.

**V14 was split, revisiting D-D.** The plan bundled the IA rename with the grid; the plan's own
sequencing note names splitting as the escape hatch. Bundling would have put the rename's mechanical
churn and the grid's new pixels in ONE visual-baseline diff, where neither can be reviewed
independently. Both halves have now landed (V14a, V14b).

**Two pieces of the v2 nav are deliberately NOT done**, and are additive whenever they are wanted:

1. **`Dashboard`** — belongs to V16. A nav entry pointing at a placeholder is worse than no entry.
2. **Folding `Channels` into `Guide`** — the mock's intent and the right end state, but origination
   ("Add a channel") still lives on the list and the grid has no affordance for it. Removing the list
   today would strand the everyday way a channel is made. Both doors are real; neither is a redirect.
   `Suggest` stays in the admin nav for the same reason. Recorded in §12 so the remainder is visible.

⚠ **Two mock-reading lessons, recorded because both cost real work.** The v2 prototypes are
`design/loomarr-prototype-desktop-v2.dc.html` (502KB, 2026-07-24) — NOT the 146KB July-13 file. I
read the old one, concluded no grid design existed, and built one from scratch; the v2 mock had a
complete TV Guide, and `design/SYNC-LOG-2026-07-24.md` says so outright. And the mock's amber is the
`signal` token (`#FFB020`); `onair` is the RED live-dot (`#E5484D`). Mapping them backwards made the
now-line and every channel number the wrong colour, which no assertion catches — only a screenshot.

### Playout: five traps that each cost a live channel

Every one was found by RUNNING it, and four had green tests over them the whole time. A test can
only assert what you already believe.

1. **Probe the machine you will RUN on.** Verified `h264_vulkan` on the host, set
   `PLAYOUT_ENCODER` for the *container* — which had no `/dev/dri` and no driver libs. Every
   encode died instantly and the log said **nothing**, because a dying encoder closes stdout,
   which reads as a clean EOF. Fixed the guard (`n == 0` regardless of error) and the image
   (one vendor-neutral driver package set, ~120MB, serving Intel/AMD/Vulkan; NVENC needs nothing
   in-image — `libcuda` is injected by the container toolkit).
2. **`format=yuv420p` for every CPU-frame encoder, not just software.** A 10-bit HEVC source
   reaches nvenc as `yuv420p10le` and it refuses with *"No capable devices found"* — a message
   that names the DEVICE, so it reads as a missing GPU.
3. **DECODE dominates on 4K, not encode.** CPU decode + GPU encode measured 341% CPU, *higher*
   than all-software (260%), because the decode had merely been throttled by the slow encoder.
   Adding `-hwaccel cuda` → 42%.
4. **Never `-hwaccel_output_format`, and keep scale/pad on the CPU.** `scale_cuda` has **no pad
   option** (verified: a 4:3 source emits 1440x1080, not letterboxed 1920x1080), which breaks the
   parent's `-c copy` on any channel mixing aspect ratios.
5. **Hardware CAN do quality-targeted rate control.** Capped CBR crushed hard scenes; a comment
   in our own code claimed hardware could not use a quality target. Wrong about NVENC —
   `-rc vbr -cq 21 -b:v 0` with a 2× ceiling took SSIM 0.98262 → 0.98581. `-b:v 0` is
   load-bearing: a non-zero bitrate degenerates cq back to CBR.

**Guide display, learned by two wrong answers against the real Emby.** XMLTV says `<title>` is the
series and `<sub-title>` the episode. Emby parses both correctly but its guide GRID renders
`Name` alone — so episode-in-title showed *"Bart the Mother"* with no show, and series-in-title
showed *"The Simpsons"* with no episode. Now combined (`"The Simpsons: Bart the Mother"`), with
`<sub-title>` still emitted for clients that use it. Tunarr can afford the strict split precisely
because it emits `<desc>`; once ours landed, the constraint changed — revisiting is reasonable.

**Pattern worth noting across V1/V17a/V23:** each was a *complete* feature with one missing link —
a built component nobody imported, a sidecar nobody parsed, a persisted field nobody captured. All
three passed their own unit tests. The gates that catch this class are reachability assertions and
prompt capture, not more component tests.

**Not scheduled, need a design decision first:** C5, C6, C8 — the v2 mock shows **no UI for any of
them**, so there is nothing to port. Each violates §12's surface-map rule; the honest options are
*add the UI*, *remove the capability*, or *document it as API-only*.

## Environment (recorded Phase 1; verify with `docker info`, `go version`)

| Prereq | State (2026-07-13) | Note |
| --- | --- | --- |
| Go | `go1.26.5` ✓ | Design requires 1.22+; sibling `nexus-open` uses `go 1.26.0`. |
| Node | `v26.4.0` ✓ | Design requires 20+. |
| Docker daemon | **active** ✓ (Server 29.6.1) | Started + `enable`d 2026-07-13 (`systemctl is-enabled` → enabled). Compose v5.3.1. **Hard requirement from Phase 4** — now satisfied. |
| make | GNU Make 4.4.1 ✓ | |
| goose | not on PATH | Installed as a Go tool dep in Phase 1/3 (`go run`/tool), not required on PATH. |

## Project facts (design doc §20 — resolved)

- **Module path:** `github.com/mantonx/loomarr` — matches sibling convention `github.com/mantonx/nexus-open`.
- **License:** MIT (matches `nexus-next`).
- **House conventions to mirror:** `nexus-next` Makefile verb style, `.golangci.yml`, `modernc.org/sqlite` driver.

## Phase-0 findings (fill during contract spikes — this is the pinned evidence)

> Per §21 phase 0 and CLAUDE.md "Ask the maintainer": if any contract deviates from
> §6/§9, **stop and update `loomarr-design.md` first**, then proceed.

- [x] **Tunarr** (local `chrisbenincasa/tunarr:latest` → **v1.3.8**, ffmpeg 7.1.1, node 22.20.0). Spec vendored to `api/vendor/tunarr-openapi.json` (OpenAPI 3.0.3, 117 paths). Throwaway channel CRUD exercised: create 201 → GET 200 → DELETE 200 → gone (404). **API-key question SETTLED: no key required** — spec has empty `securitySchemes`/`security`, unauth reads+writes succeed; `TUNARR_API_KEY` confirmed optional for 1.3.8. Findings: `internal/testkit/fixtures/tunarr/FINDINGS.md`. **Contract surprise: server assigns channel `id`; client-supplied id is ignored** (Phase-10 adapter must use the create-response id).
- [x] **Sonarr** (v4.0.19.2979): `Test` webhook captured verbatim → `internal/testkit/fixtures/sonarr/test_webhook.json` (minimal placeholder shape per §6 — has junk `series`/`episodes`, handler must not resolve a real title from Test).
- [x] **Radarr** (v6.2.1.10461): **full lifecycle captured live** — `Test`, `Grab`, `Download`(import) → `radarr/{test,grab,import}_webhook.json`. Findings: `internal/testkit/fixtures/FINDINGS-arr-webhooks.md`. Confirmed §6 quirks: import event `eventType` is the string **`"Download"`**; `downloadId` correlates Grab↔Download; identity key is `remoteMovie.tmdbId`; upgrade import has `isUpgrade:true`+`deletedFiles`. Method: forced re-grab of *In Flames* via `POST /release {guid,indexerId,movieId}` → real SABnzbd download → real import webhook. Temp webhook conn (id 3) **removed** (verified: only Emby+Mail remain).
- [~] **Sonarr** `Grab`/`Download` not yet captured (only `Test`). Structure mirrors Radarr with `series`/`remoteSeries.tvdbId`; capture at Phase-6 start via same method or Sonarr history. Not blocking.
- [x] **Seerr codebase cross-check** (maintainer lead, `github.com/seerr-team/seerr@develop`): reviewed `server/api/servarr/{base,radarr}.ts`. Reference note: `internal/testkit/fixtures/REFERENCE-seerr.md`. Key takeaways: Seerr add-movie is check-then-act idempotent (`getMovieByTmdbId`); Seerr passes arr key as `?apikey=` query param — **we deliberately diverge, keeping `X-Api-Key` header** per §6. Seerr's connection-leak issue (#2297/#2303) is live evidence for §6's mandatory per-service timeouts.
- [x] **Media server — Emby** (v4.10.0.17): full authed round-trip captured → `internal/testkit/fixtures/emby/{system_info_authed,users_list,lookup_present,lookup_absent,auth_badpw_response}.json` + `FINDINGS.md`. `X-Emby-Token` header auth 200; `AnyProviderIdEquals=tmdb.<id>` presence check works (present→Items[1], absent→Items[], both 200); casing case-insensitive on 4.10 (lowercase per §6 anyway); `/Users` gives `Policy.IsAdministrator/IsDisabled` for §11; `AuthenticateByName` bad-pw→401. **Seerr cross-check (maintainer lead, `jellyfin.ts`):** Seerr uses ONE unified `Authorization: MediaBrowser … Token=…` header for both flavors — verified it also returns 200 on Emby. Recorded as a §6 *option* (single code path), not taken without a doc-first change.
- [x] **Seerr** (v3.2.0) requester → `internal/testkit/fixtures/seerr/{request_available_201,request_repeat}.json` + `FINDINGS.md`. `POST /api/v1/request` with `X-Api-Key`. **Refinement (not a deviation):** re-requesting an available/duplicate movie returns **201 with existing media** (not 409), `downloadStatus:[]` (nothing new queued). §6's "201/409=success" holds, but Phase-6 tests must assert "2xx or 409", never *require* 409 on an available title.
- [ ] Deviations from §6/§9 (if any): **none so far** — Tunarr id-assignment + lineup-needs-programming are details the doc deferred to Phase 10, not contradictions. No `loomarr-design.md` edit required yet.

### Dev infrastructure (persistent — keep running through Phases 10–12)

- **`tunarr-dev`** — persistent local Tunarr for developing the Programmer adapter against.
  Compose: `docker/compose.dev.yaml` (image `chrisbenincasa/tunarr:1.3.8@sha256:88122a21…`,
  volume `tunarr-dev-config`, `restart: unless-stopped`). Up at `localhost:8000`. Setup +
  Emby-wiring steps: `docker/tunarr-dev-setup.md`.
- **Emby media source wired** into `tunarr-dev` (id `fa31064d-50e1-405d-9edc-3364e0754ffd`,
  `{"healthy":true}`) — sees 7 Emby libraries incl. **Movies** (`childType:movie`) and
  **TV shows** (`childType:show`). Phase-10 note: `POST /api/emby/login` only returns a token;
  the source is a *separate* `POST /api/media-sources` carrying it.

### Temp homelab state to revert after Phase 0

- Radarr webhook connection `loomarr-phase0-capture` (id **3**) — DELETE via `/api/v3/notification/3` once import captured.
- Local Tunarr spike container `tunarr-spike` (+ volume `tunarr-spike-data`) — `docker rm -f` when done.
- Forced re-grab of *In Flames* landed a fresh file on the media server (expected; a re-download of an owned title).
