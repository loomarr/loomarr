# PROGRESS.md — Loomarr build tracker

One row per phase (design doc §21). A phase is **done** only when its gate (a set of
tests) is green and the evidence — commit SHA + the exact test command that proves it —
is recorded here. See `CLAUDE.md` for the prime directives; one phase per session/PR.

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
| 13 · Web UI + onboarding | todo (visual seed imported; **12.5 done → unblocked, next**) | — | Vite React+TS, orval hooks, first-run wizard. Playwright installs here. Onboarding tests part of the gate. **Visual reference imported ahead of build (2026-07-13):** `design/loomarr-prototype-{desktop,mobile}.dc.html` (+ `support.js`, `design/README.md`) — the Claude Design mock (14 screens desktop, 3 mobile), authoritative for *look*; recreate pixel-perfectly in Vite/React with the two §7 deltas (badge `-300` stops; `static-500` disabled-only). `loomarr-frontend-design.md` seed saved at repo root (tokens/component registry/visual-test gates). CLAUDE.md updated: Prime Directive #6 (Go-only), Phase-13 map row, `fe-tokens`/`fe-visual`/`fe-visual-update` targets, 3-artifact Seed docs section. |
| 14 · Docs, harden & ship | todo | — | `docs/` set, profiles, runbook, metrics, README. |

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
