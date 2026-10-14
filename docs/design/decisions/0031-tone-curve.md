# 0031. Tone curve: one default plus six selectable curves

- **Date:** 2026-09-26
- **Status:** accepted
- **Supersedes:** the `tonemap_vaapi` Intel path

## Context

HDR sources must be tone-mapped for SDR outputs. On Intel, `tonemap_vaapi` aired an all-black
picture at full speed, and nothing noticed because every speed check passed (#1516). Different
tone-mappers also spell and support curves differently.

## Decision

Tone mapping is a given on every SDR output, never black and never silent (#1522).
`playout.tone_curve` selects one curve: `hable` by default, or `mobius`, `reinhard`, `bt2390`,
`bt2446a` or `spline`. Intel maps surfaces into OpenCL (`tonemap_opencl`); `tonemap_vaapi` is banned
from every path. NVENC tries OpenCL, then libplacebo, then the CPU. Curves that only libplacebo has
fall back to a declared Mobius substitute on the CPU.

## Consequences

- The image ships Intel's compute runtime and an NVIDIA OpenCL ICD file. Intel Gen8–11 iGPUs use
  the CPU tone-map because their legacy runtime is too large to ship.
- A boot self-check reads back luma from real HDR frames; a black result is a red Diagnostics
  failure and drops HDR from admission, so a channel shows a card rather than black.
- Verification must assert picture (luma), not speed.
