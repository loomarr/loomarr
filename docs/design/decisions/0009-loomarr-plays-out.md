# 0009. Loomarr plays out its own streams

- **Date:** 2026-07-25
- **Status:** accepted
- **Supersedes:** the founding non-goal "not a transcoder or streamer; Tunarr does playback"

## Context

Loomarr began as a programmer that handed playout to Tunarr. Every capability that separates
television from a playlist lives at the encoder: mid-programme breaks (impossible at Tunarr's
programme boundaries), honest transcode telemetry, and per-channel control of cut points. The
workarounds were accumulating faster than the features they avoided.

## Decision

Loomarr plays out its own channels and publishes its own M3U tuner and XMLTV guide. Tunarr stays a
first-class alternative backend, chosen globally with a per-channel override. The scheduler, lineup,
pods and approval gate stay backend-agnostic.

## Consequences

- ffmpeg and ffprobe became core runtime dependencies, and the image grew to carry them.
- A restart now interrupts internal channels; restart copy has to say so per backend.
- Every "what is on now" reader must answer for the backend actually streaming the channel.
- Hardware that cannot transcode can still use Tunarr; an install that works is never forced to
  migrate. The full consequences are archived in
  [`playout-2026-07-consequences.md`](../../../project/design-archive-2026-09/playout-2026-07-consequences.md).
