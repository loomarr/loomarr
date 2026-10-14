# 0006. Filler is its own pipeline, not the *arr path

- **Date:** 2026-07-13
- **Status:** accepted

## Context

Commercials, bumpers and station IDs are core to the "feels like real TV" goal, but they are not
titles: TMDB does not list them, Seerr cannot request them and Sonarr or Radarr cannot fetch them.
The provisioning loop has nothing to say about a thirty-second advert.

## Decision

Filler gets its own sourcing pipeline (drop folders, Internet Archive collections, YouTube
playlists, hand-made bumpers), its own catalog metadata and its own matching logic. The scheduler
consumes the result; the provisioning loop never sees it.

## Consequences

- Filler needs media tooling (`yt-dlp`, `ffmpeg`) in the core, which ships in the one image.
- Clips are Loomarr-owned and identified by Loomarr, not by a media server or TMDB.
- Tagging, grounding and readiness are filler-specific concerns with their own design docs.
