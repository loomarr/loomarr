# 0027. Loomarr owns its media inventory; the library is an importer (V66)

- **Date:** 2026-09-05
- **Status:** accepted
- **Supersedes:** reading media facts from the media server on demand

## Context

Playout, scheduling and search each asked Emby or Jellyfin for what they needed, in provider-shaped
responses, at the moment they needed it. Measured facts (keyframes, loudness, break points) had no place
to live, a media-server outage removed knowledge Loomarr had already had, and a second importer would
have meant a second set of response types in every consumer.

## Decision

`inventory.Service` owns one durable, provider-neutral model of Media Items, Media Sources, their
Origins and Observations. The media server populates it through `ApplySnapshot`; consumers read only
inventory types. Identity merges only on grounded external ids or explicit links. Absence is recorded
only after a completed scan.

## Consequences

- Playout reads stream facts and measured analysis at airtime without calling the media server.
- Another importer (for example a direct file scanner) can use the same write port.
- Inventory grants no authority: acquisition state and filler admission stay with their own modules.
- The schema carries six inventory tables plus per-revision source analysis, bounded and sanitized at
  both the domain and store boundaries.
