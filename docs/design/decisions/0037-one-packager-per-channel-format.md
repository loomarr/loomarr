# 0037. One packager per (channel, format)

- **Date:** 2026-09-27
- **Status:** accepted
- **Supersedes:** the per-programme playout chain

## Context

Beta.7 ran a three-process chain per channel (a shared session, an MPEG-TS block mux, an HLS remux
and a loopback self-reach). It paid 15–20 s to first frame at worst and could not hold a decoder
steady across a programme boundary.

## Decision

Each channel and format has one long-lived Go packager that owns the channel's segment timeline
(#1512). One ffmpeg per scheduled item emits fMP4 fragments already placed on that timeline, and the
packager forwards them into one gapless playlist. Browsers read HLS; media-server tuners read the same
timeline as continuous MPEG-TS (#1538). The packager is the only live path (#1542).

## Consequences

- A channel watched in a browser and on a TV is one encode.
- The packager starts on the first tune and stops 30 s after the last viewer leaves.
- Load certification and per-client encode sessions retired with the chain.
