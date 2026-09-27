# 0021. Tuning is a latest-request-wins state machine

- **Date:** 2026-08-15
- **Status:** accepted
- **Supersedes:** tuning by route churn

## Context

Pressing Channel Up several times is the ordinary television gesture. When each route mount started
its own playback, every intermediate Channel minted a URL, fetched a manifest and attached a decoder,
and a slow earlier response could replace the newest Channel.

## Decision

One Tuner controller owns playback (V57). Each request gets a monotonically increasing attempt id; a
newer request aborts the older one's work, and completions that are no longer current change
nothing. There is at most one media element and one decoder, and prefetch never creates a hidden
player. The route follows the tuned Channel instead of driving it.

## Consequences

- A burst of requests plays only the last target.
- The contract is platform-neutral, so the Android TV and future native adapters implement the same
  catalog, attempt and telemetry vocabulary behind their own players.
- Tune latency is measured per attempt and gated (#1512).
