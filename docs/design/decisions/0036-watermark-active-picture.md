# 0036. The watermark is anchored to the measured active picture

- **Date:** 2026-09-27
- **Status:** accepted

## Context

Every programme carries a burned-in channel bug. Anchored to the output frame, it lands in the black
bars of a letterboxed film, which looks wrong and invites burn-in on the bars.

## Decision

The bug is placed relative to the source's measured active picture, persisted with the inventory
analysis and applied at airtime (#1537). It is drawn by the GPU overlay on NVENC and VAAPI, hidden
during filler, bumpers and IDs, and configured by `policy.watermark`.

## Consequences

- Outputs without a GPU overlay path (software, generic, VideoToolbox, HDR10 premium) air without it,
  and the pipeline records why.
- A self-check proves the programme is unchanged outside the bug, the blend is correct in coded luma,
  and the SPS and PPS are byte-identical; if it fails, the watermark is off on that host and a
  Diagnostics event says so.
