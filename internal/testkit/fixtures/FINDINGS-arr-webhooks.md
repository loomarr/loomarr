# Sonarr/Radarr webhook contract — Phase 0 findings

Captured **verbatim** 2026-07-13 from the live homelab (via Tailscale, direct to container
ports). These fixtures are pinned truth: the Phase-6 `/hooks/arr` handler is written against
them, not against remembered field names (CLAUDE.md testing rules).

## Source versions (record in every parser that consumes these)

| App | Version | Fixtures |
| --- | --- | --- |
| Sonarr | **4.0.19.2979** (branch `main`) | `sonarr/test_webhook.json` |
| Radarr | **6.2.1.10461** (branch `master`) | `radarr/{test,grab,import}_webhook.json` |

Download client in the captures: **SABnzbd**. Method: real connection-test + a forced re-grab
of *In Flames* (2023, tmdbId 1111867) to fire genuine Grab→Download events.

## Confirmed contract facts (drive the Phase-6 handler)

1. **The import-complete event's `eventType` is the string `"Download"`** — NOT "Import" or
   "DownloadFolderImported". Confirmed live in `radarr/import_webhook.json`. (Radarr's *internal
   history* calls the same moment `downloadFolderImported`; the webhook string differs. Don't
   conflate them.) This is the §6 "naming quirk" — pinned.

2. **`downloadId` correlates Grab ↔ Download.** Identical value (`9d5f633f-b733-4542-…`) in both
   `grab_webhook.json` and `import_webhook.json`. It's the idempotency/correlation key: the
   handler ties a completed import back to the grab (and thus to the provisioning request).

3. **Title identity = `remoteMovie.tmdbId` (Radarr) / `remoteSeries.tvdbId` (Sonarr, expected).**
   Both Grab and Download carry a `movie` object *and* a `remoteMovie` object; `remoteMovie.tmdbId`
   is the stable match key back to the request. Do not parse the release title for identity.

4. **`Test` payloads are minimal & placeholder** (§6). `sonarr/test_webhook.json` has junk
   `series` (`"path":"C:\\testpath"`, `tvdbId:1234`) and a placeholder `episodes[0]`;
   `radarr/test_webhook.json` similarly. The handler MUST NOT attempt title resolution on a
   `Test` event — just ack 200 and record a per-app `last-received` timestamp (powers the §13
   onboarding webhook handshake). Two independent Radarr Test captures (button-test +
   connection-creation test) agreed on keys.

5. **Upgrade imports still mean "available."** The captured Download has `isUpgrade: true` and a
   populated `deletedFiles` array (the re-grab replaced the prior file). The handler must treat
   an upgrade Download as a normal "confirm-in-library → available" and tolerate `deletedFiles`.

## Payload key inventory (top-level)

- **Radarr Grab:** `movie, remoteMovie, release, downloadClient, downloadClientType, downloadId,
  customFormatInfo, eventType, instanceName, applicationUrl`
- **Radarr Download (import):** the above minus `release`-only, plus `movieFile{…}, isUpgrade,
  deletedFiles, release`. `movieFile` has `id, relativePath, path, quality, releaseGroup, size,
  mediaInfo, sourcePath, …`.
- **Radarr/Sonarr Test:** `series|movie` (placeholder), `episodes` (Sonarr), `eventType:"Test"`,
  `instanceName`, `applicationUrl`.

## Still to capture (not blocking Phase 6 start; structure mirrors Radarr)

- **Sonarr `Grab` + `Download`** — same lifecycle with `series`/`episodes` + `remoteSeries.tvdbId`
  instead of `movie`/`remoteMovie.tmdbId`. Capture via the same forced-grab method against a
  Sonarr episode, or reconstruct from Sonarr history, when Phase 6 begins.

## Auth note (applies to our Phase-6 arr client)

All captures used the **`X-Api-Key` header**, never `?apikey=`. Seerr's own client uses the query
param (`REFERENCE-seerr.md`) — we deliberately diverge and keep header auth per §6's anti-leak rule.

## Not a design-doc deviation

Everything here matches §6 (incl. the predicted quirks). No `loomarr-design.md` edit required.
