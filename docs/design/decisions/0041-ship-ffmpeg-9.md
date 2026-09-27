# 0041. Ship ffmpeg 9

- **Date:** 2026-09-27
- **Status:** accepted
- **Supersedes:** the ffmpeg n8.1 ceiling

## Context

The image pinned ffmpeg n8.1 only because ffmpeg 9 could not advance the beta.7 chain's concat past a
chunked HTTP entry. The channel packager ([0037](0037-one-packager-per-channel-format.md)) retired that
chain. Meanwhile n8.1.2 dropped the NVENC watermark overlay's picture, so the watermark self-check
disabled it on NVIDIA hosts.

## Decision

Ship ffmpeg n9.0.1 from the same retained monthly BtbN release (#1552). A version bump is judged by
picture checks (watermark luma, tone-map content, parameter sets), never by encode speed alone.

## Consequences

- The NVENC watermark self-check passes; the image grew by about 1.6 MB.
- The redistribution manifest, release verifier, third-party notices and published GPL source must
  move with the pin; the n9 corresponding source was published before redistribution (#1558).
- ffmpeg 9 enables GPU interop that later work builds on, such as libplacebo on a CUDA-derived Vulkan
  device (#1559, open).
