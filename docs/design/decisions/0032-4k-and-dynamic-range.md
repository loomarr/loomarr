# 0032. 4K is a given; dynamic range is independent of resolution

- **Date:** 2026-09-26
- **Status:** accepted

## Context

A household library mixes 1080p SDR, 4K SDR and HDR sources. One uniform output per channel
([0039](0039-uniform-channel-output.md)) must still let 4K and HDR libraries look their best, without
making every device pay for it.

## Decision

Every channel airs a 1080p SDR H.264 baseline. `DeriveChannelFormats` derives at most one premium
HEVC format from the lineup's measured inventory, deciding resolution and dynamic range separately:
any 4K item plus any HDR item gives `4k-hevc-hdr` (Main10, BT.2020, PQ), 4K without HDR gives
`4k-hevc-sdr` (#1527). The premium is served only when a client opts in (#1554).

## Consequences

- A channel costs at most two encodes, and its dynamic range never changes mid-stream: SDR and HLG
  items on an HDR stream are converted on the GPU without inverse tone-mapping.
- A host drops the premium, and says why, when it is software-only, has no full-GPU graph, or lacks
  libplacebo for HDR.
- A baseline viewer never starts a 4K encode.
