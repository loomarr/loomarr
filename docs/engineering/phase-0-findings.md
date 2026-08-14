# Phase 0 — Contract-spike findings (index)

Phase 0 (design §21) verifies the risky external contracts against the maintainer's live homelab
and pins the evidence into the repo *before* any product code. Captured **2026-07-13** via
Tailscale (direct to container ports) + a local Tunarr dev instance.

The detailed findings live next to the fixtures they explain (AGENTS.md: "Fixtures are pinned
truth"). This page is the index and the summary of what changed our understanding.

## Evidence map

| Service | Version | Findings | Fixtures / spec |
| --- | --- | --- | --- |
| Tunarr | 1.3.8 | [`fixtures/tunarr/FINDINGS.md`](../../internal/testkit/fixtures/tunarr/FINDINGS.md) | [`api/vendor/tunarr-openapi.json`](../../api/vendor/tunarr-openapi.json), `fixtures/tunarr/*.json` |
| Radarr | 6.2.1.10461 | *(none — see note below)* | `fixtures/radarr/import_webhook.json` |
| Sonarr | 4.0.19.2979 | *(none — see note below)* | *(none retained)* |
| Emby | 4.10.0.17 | [`fixtures/emby/FINDINGS.md`](../../internal/testkit/fixtures/emby/FINDINGS.md) | `fixtures/emby/*.json` |
| Seerr | 3.2.0 | [`fixtures/seerr/FINDINGS.md`](../../internal/testkit/fixtures/seerr/FINDINGS.md), [`fixtures/REFERENCE-seerr.md`](../../internal/testkit/fixtures/REFERENCE-seerr.md) | `fixtures/seerr/*.json` |

> **Note on the `*arr` rows.** This table linked a `FINDINGS-arr-webhooks.md` that **never
> existed**, alongside two fixture paths that don't either — corrected 2026-08-10 rather than
> left as three dangling links. The reason there is little to point at: the inbound `*arr`
> webhook was **retired**, and state now comes from polling (`scripts/check-retired.sh`
> guards the old identifiers). `radarr/import_webhook.json` is retained as the one captured
> payload; the Sonarr capture was deferred and never taken.

## Contract surprises that would have been silent bugs (found by testing vs. memory)

1. **Tunarr assigns the channel `id`; the client-supplied id is ignored.** The Phase-10
   Programmer adapter must use the id from the create response for all follow-up calls.
2. **Radarr's import-complete webhook `eventType` is the string `"Download"`** (not "Import").
   `downloadId` is the grab↔import correlation key. Identity = `remoteMovie.tmdbId`.
3. **Seerr returns 201 (not 409) when re-requesting an available/duplicate movie.** §6's
   "201/409 = success" still holds, but Phase-6 tests must assert "2xx or 409", never *require* 409.

## Settled open questions

- **Tunarr API key (§6):** confirmed **optional** for 1.3.8 — spec has empty
  `securitySchemes`/`security`; unauthenticated reads and writes succeed.
- **Emby `AnyProviderIdEquals` casing (§6):** case-insensitive on 4.10; the lowercase `tmdb.`
  form the doc mandates works. Casing guard retained as defense for other versions.
- **Emby auth header:** `X-Emby-Token` works (§6). Seerr's unified `Authorization: MediaBrowser …
  Token=…` also works on Emby — recorded as a possible single-code-path option (a §6 change
  would be doc-first; not taken).

## Deviations from §6/§9

**None.** Every surprise above is a detail the design doc under-specified or deferred (e.g. to
Phase 10), not a contradiction. No `docs/design.md` edit was required in Phase 0.

## Environment / homelab state

- Docker daemon started + enabled (Server 29.6.1). Prereqs recorded in `PROGRESS.md`.
- Temp Radarr webhook connection (id 3) removed; Tunarr spike container + volume torn down;
  webhook listeners killed. A re-grab of *In Flames* landed a fresh file (expected).
- Secrets used from `fictional-media-server/.env` and `loomarr/.phase0.env` (git-ignored) —
  never committed.

## Still open (not blocking — deferred to their phases)

- **Sonarr `Grab`/`Download`** webhooks (only `Test` captured). Same lifecycle as Radarr with
  `series`/`remoteSeries.tvdbId`; capture at Phase-6 start.
- **Emby `AuthenticateByName` success body** (only the 401 bad-pw path pinned; no test-user
  password this session). Capture at Phase 9 with a throwaway user.
