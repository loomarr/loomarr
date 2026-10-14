# 0038. 1 s segments

- **Date:** 2026-09-27
- **Status:** accepted

## Context

Tune-in and channel-switch time is dominated by how long the first segment takes to exist.
The phase 0 target of #1512 was a cold-tune first segment within 400 ms at p95 for 1080p H.264.

## Decision

The packager writes 1 s closed-GOP segments. Measured in phase 0 over fresh files with minimal
probing, 1 s segments reached a first segment in 348 ms at p95; 2 s segments missed the target at
496 ms.

## Consequences

- More segments and playlist entries per minute, bounded by the `DVRHorizon` window.
- Hold-back, run-ahead and listing thresholds (`ListAhead` 6 s, `RunAhead` 12 s) are expressed in
  whole seconds of this timeline, and web clients set `liveSyncDuration` to match.
