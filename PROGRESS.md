# PROGRESS.md — Loomarr build tracker

One row per phase (design doc §21). A phase is **done** only when its gate (a set of
tests) is green and the evidence — commit SHA + the exact test command that proves it —
is recorded here. See `CLAUDE.md` for the prime directives; one phase per session/PR.

**Active phase: 8 — Self-documenting API.** Phases 0–7 done (see the phase table).
Follow-up: Sonarr `import_webhook.json` fixture still to be captured (28GB re-download in flight;
Sonarr webhook conn id 3 left up to catch it — remove after). Phase-0 findings:
[`docs/engineering/phase-0-findings.md`](docs/engineering/phase-0-findings.md). Deferred captures:
Sonarr Grab/Download → Phase 6; Emby login success body → Phase 9.

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
| 8 · Self-documenting API | todo | — | Huma v2 / `humago`; `/openapi.*`, `/docs`; `GET /v1/backup`; `make openapi` + committed spec; contract tests. |
| 9 · Users & auth | todo | — | Sessions, bootstrap, user sync, role enforcement, `API_TOKEN`, login rate-limit. **Auth & roles tests are the gate** (incl. §19 negatives). |
| 10 · Scheduler + Tunarr | todo | — | Desired-vs-actual reconcile + sweep; backfill; `/v1/channels*`. Reconcile-against-mock-Tunarr tests are the gate. **The point of the app.** |
| 11 · Suggester | todo | — | Grounding + validation; deterministic scoring; jobs/proposals/SSE; `/v1/search`. **Grounding tests are the gate.** |
| 12 · Commercials & filler | todo | — | Catalog sync; pod assembly + fallback ladder; `loomarr-ingest` sidecar. **Filler-never-a-program + pod-matching tests are the gate.** |
| 13 · Web UI + onboarding | todo | — | Vite React+TS, orval hooks, first-run wizard. Playwright installs here. Onboarding tests part of the gate. |
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
