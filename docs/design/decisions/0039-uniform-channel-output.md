# 0039. Always transcode to one uniform channel output

- **Date:** 2026-09-27
- **Status:** accepted
- **Supersedes:** per-client encode plans, direct-play stream copy and HEVC-follows-content (V47, V48, V50)

## Context

A channel strings together sources with different codecs, geometry, frame rates and colour. Copying
a compatible source or adapting to each client meant decoder resets at boundaries and one encode per
watching device.

## Decision

Every item is transcoded into the format's fixed geometry, frame rate, codec profile, colour labels
and AAC-LC stereo 48 kHz, with identical encoder arguments per class (#1512). No boundary changes
decoder state, so a programme-to-break handoff needs no reset, and one encode serves web, TV and the
media server.

## Consequences

- There is no stream-copy path, even for a source that already matches.
- A channel's format follows its lineup ([0032](0032-4k-and-dynamic-range.md)), never the device.
- The slate goes through the same builder tail, so it never defines a different init.
