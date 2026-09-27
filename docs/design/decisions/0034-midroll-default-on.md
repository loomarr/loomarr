# 0034. Mid-programme breaks on by default, with a per-channel off switch

- **Date:** 2026-09-27
- **Status:** accepted
- **Supersedes:** mid-roll detection as a per-channel opt-in

## Context

Tunarr can only insert filler between programmes. Once Loomarr owned its encoder (decision 0009),
it could cut inside a programme, which is what broadcast television does. The earlier design made
this opt-in because detection meant decoding whole files. Measuring each source once per revision
(chapters, then a targeted black-and-silence search) made detection cheap enough to run for every
channel (#1530, maintainer decision 2026-09-26).

## Decision

Internal-playout channels get mid-programme breaks by default. `policy.midRoll` is the per-channel
off switch (absent inherits on, `false` is off). Tunarr channels never get them. Cuts land only at
measured fades near the due point; a due break with no fade is skipped, never forced. One
breaks-per-hour cadence covers mid-programme and between-programme breaks.

## Consequences

- A long programme can air unsplit until it has been measured; placement never blocks on it.
- Container chapter marks are not trusted alone: live, a scene-selection chapter sat in a bright
  picture, so a chapter must read black on both sides (#1529).
- The guide shows a split programme as one entry; breaks stay a scheduling detail.
- Splits are frozen for programmes on air or starting soon, so a new measurement never re-cuts
  what a viewer is watching.
