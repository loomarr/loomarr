# Reference: Seerr's Sonarr/Radarr integration (cross-check for Phases 5–6)

Source: `github.com/seerr-team/seerr` @ `develop`, files `server/api/servarr/{base,radarr,sonarr}.ts`
and `server/api/externalapi.ts`. Running homelab image: `ghcr.io/seerr-team/seerr:v3.2.0`.
Captured 2026-07-13 as a design reference — **not vendored code**, and Seerr's choices are
not automatically ours where the design doc says otherwise (noted below).

## What Seerr is, precisely

Seerr is a **requester**: it sends `POST /movie` (Radarr) / `POST /series` (Sonarr) to the arr
apps. That's the same role as Loomarr's **Phase-6 Seerr adapter** — Loomarr talks to *Seerr's*
`/api/v1/request`, and Seerr in turn talks to the arrs. Seerr is authoritative for the
requester contract, and a useful (not binding) reference for arr field names.

## Radarr add-movie shape (Seerr's `radarr.ts`) — informs our understanding of arr internals

`POST /movie` body:
```
{ title, qualityProfileId, profileId, titleSlug, minimumAvailability,
  tmdbId, year, rootFolderPath, monitored, tags,
  addOptions: { searchForMovie: <bool> } }
```
Idempotency is **check-then-act**, not 409-driven: Seerr calls `getMovieByTmdbId()` first and
branches — already-available (log + return), exists-but-unmonitored (`PUT /movie` to monitor),
exists-and-monitored (return; optional search). Good robustness pattern to mirror in any
direct-arr path we add.

`RadarrMovie` returned object keys we can rely on: `id, tmdbId, imdbId, titleSlug, hasFile,
isAvailable, monitored, path, movieFile{}`. **`tmdbId` is the stable identity** — matches what
our captured Grab webhook uses (`remoteMovie.tmdbId`).

## Base client (Seerr's `base.ts`) — one thing we deliberately DON'T copy

- URL = `http(s)://hostname:port{baseUrl}{path}`; arr paths passed with the `/api/v3` prefix
  by callers.
- **API key passed as `?apikey=` query param.** ⚠️ Loomarr uses the **`X-Api-Key` header**
  instead (all Phase-0 captures did) — design §6's anti-leak rule ("never `api_key` query
  param — leaks to logs") is stated for the media server but is the right default everywhere.
  Keep header auth for our Sonarr/Radarr/Seerr clients; do not adopt Seerr's query-param style.
- Timeout comes from `network.apiRequestTimeout` (settings-driven), applied to axios. Matches
  our §6 "shared HTTP factory with hard timeouts." Seerr issue #2297/#2303 documents an HTTP
  **connection leak → downstream deadlock** when arr calls hang without timeout — direct
  evidence for why §6 mandates per-service hard timeouts and why *writes never client-retry*.
- Cached reads: `getProfiles()` / `getRootFolders()` cached 3600s. Reasonable pattern for our
  own arr lookups if we add a direct requester (`SONARR_*`/`RADARR_*` path, §15 footnote *).

## Where this maps in our build

- **Phase 6** (Seerr requester): confirms `POST /api/v1/request` with `{mediaType, mediaId=TMDBID,
  seasons}` and the 201/409-as-success rule (§6). Seerr's own approval-queue trap (§6
  "operational trap") is real — its issue tracker (#1517, #1994) shows requests stalling when
  the service user lacks auto-approve. The §13 integrations doc must call this out.
- **Phase 5** (library): Seerr supports Emby/Jellyfin/Plex natively — validates our flavor split.

## Not a design-doc deviation

Nothing here contradicts §6/§9. The query-param-key choice is Seerr's, and we consciously
diverge (header auth). No `loomarr-design.md` edit required.
