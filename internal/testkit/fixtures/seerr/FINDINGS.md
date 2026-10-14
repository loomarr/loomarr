# Seerr requester contract — Phase 0 findings

Captured 2026-07-13 against live **Seerr v3.2.0** (`ghcr.io/seerr-team/seerr:v3.2.0`), via
Tailscale `:5055`. This is the requester Loomarr's Phase-6 adapter drives (§6).

## Endpoint & auth

- `POST {SEERR_URL}/api/v1/request`, header **`X-Api-Key`**, body `{"mediaType","mediaId"}`
  (`mediaId` = TMDB id; add `"seasons"` for TV). Verified `settings/main` → 200 with the key.

## Idempotency — refines the §6 assumption (NOT a deviation)

§6 says: "Treat **201** and **409** as success (idempotency)." Live behavior on v3.2.0:

- Requesting an **already-available** movie (tmdbId 1241983, status 5) → **HTTP 201** with the
  existing media record (`media.status: 5`, `media.id: 2728`), NOT 409. Fixture
  `request_available_201.json`.
- A **second identical** POST immediately after → **also 201** with the same record. Fixture
  `request_repeat.json`.
- `media.downloadStatus: []` on the response confirms **no new acquisition was queued** — safe.

**Takeaway for Phase 6:** 201 is the observed success path for duplicate/available requests;
409 is rarer than the doc's phrasing implies (likely narrower cases — still-pending duplicate,
quota/permission rejection). The handler must accept **both 201 and 409 as success** (§6 stands),
but Phase-6 tests must NOT *require* a 409 on an available title — real Seerr returns 201 there.
Assert "2xx or 409", not "expect 409".

## Cross-reference (maintainer lead — Seerr's own arr client)

See `../REFERENCE-seerr.md`. Seerr→arr add is check-then-act idempotent (`getMovieByTmdbId`).
Seerr's connection-leak issue (#2297/#2303) reinforces §6's mandatory per-service timeouts.

## The §6 operational trap (must surface in §13 docs)

Seerr has its own approval workflow: if Loomarr's Seerr service user lacks auto-approve, every
Loomarr-approved acquisition stalls in Seerr's *second* pending queue until the deadline expires.
Not capturable as a fixture (config-dependent), but the integrations/troubleshooting docs (§13)
must instruct granting the Loomarr service user auto-approve, and cover the "stuck in requested"
symptom.

## Fixtures

`request_available_201.json`, `request_repeat.json`. **No design-doc deviation** — §6's
"201/409 = success" holds; this only pins that 201-with-existing-media is the common real path.
