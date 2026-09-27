# 0035. CPU degradation ladder: never refuse on a slow CPU

- **Date:** 2026-09-27
- **Status:** accepted

## Context

A CPU-only household host cannot encode a 4K or HDR source at full quality in real time. Refusing
the channel leaves the viewer with nothing; airing below real time stalls it (#1517).

## Decision

On a software host a heavy title degrades instead of being refused (#1533). Rungs trade decode work
for speed (skip the loop filter, then non-reference frames, then keyframes only at 480 lines), and
every rung ends in the same scale, pad and colour labels, so the output format never changes. Audio
never degrades. `StartRung` picks the best rung projected to reach 1.2× from measured cost, and the
packager steps rungs live by measured speed (#1551). Admission refuses only when even keyframes-only
does not fit.

## Consequences

- A slow CPU yields a lower-quality picture, never a refused or stalled channel.
- Rung steps re-price the admission lease, so a step up happens only when it fits beside other
  streams.
- Rung costs are measured per host by the class probe ([0033](0033-measured-admission.md)).
