# 0051. HDR is tone-mapped by libplacebo on Vulkan

- **Date:** 2026-11-22
- **Status:** proposed. Two questions are open (below).
- **Supersedes:** the three tone-mappers and their curve table in [playout](../playout.md)
  ("Tone mapping is a given") once accepted; [0032](0032-4k-and-dynamic-range.md) keeps its formats

## Context

An HDR programme on an SDR output goes through one of three tone-mappers: `tonemap_opencl`, libplacebo
or the CPU chain (`zscale` + `tonemap`). Each implements its curves its own way, so a curve's name does
not fix its picture. Measured on a 4K HDR10 source, with only the tone-map stage changed, the CPU
tone-mapper's Mobius and libplacebo's Mobius differ by 21.8 dB PSNR (SSIM 0.898).

Only libplacebo can show some sources correctly. Dolby Vision profile 5 has no HDR10 base layer, so its
picture needs the stream's reshaping data (the RPU) applied. HDR10+ and Dolby Vision carry per-scene
metadata that only libplacebo reads. BT.2390, ST 2094-10 and ST 2094-40 exist only in libplacebo.

The same libplacebo graph renders nearly the same picture on different Vulkan devices. On the dev
RTX 3080 Ti and on Mesa's lavapipe (CPU Vulkan, already in the image through `mesa-vulkan-drivers`),
using the image's ffmpeg binary, Mobius, BT.2390 and ST 2094-40 agree at 59.1–60.1 dB PSNR (SSIM 0.999).

## Decision

1. **libplacebo is the only HDR tone-mapper on a GPU family.** Frames reach Vulkan without leaving the
   GPU: NVIDIA through a Vulkan device derived from CUDA (ffmpeg 9, measured in #1559), and Intel and
   AMD by mapping VAAPI surfaces into Vulkan. `tonemap_opencl` retires.
2. **The curve setting offers libplacebo's curves:** spline, linear, gamma, clip, hable, reinhard,
   mobius, BT.2390, BT.2446a, ST 2094-10 and ST 2094-40. Every curve runs on every GPU family, with no
   substitutes.
3. **A frame maps the same way wherever a stream starts.** Per-frame peak detection is off. It adapts
   over a window of frames, so a restart mid-programme would re-learn the brightness. The curve follows
   the stream's own metadata instead: static for HDR10, per-scene for HDR10+ and Dolby Vision. Every
   libplacebo option is set explicitly, so a libplacebo update inside an ffmpeg pin bump can't change
   the picture through a new default.
4. **Dolby Vision profiles 5 and 8 are tone-mapped with their RPU applied.** The frame's side data
   survives until libplacebo, and is stripped after it. On the 4K HDR10 output, libplacebo converts
   profile 5 to PQ BT.2020 under the channel's static HDR10 signalling. Profile 7 airs its base layer
   with the RPU; the enhancement layer is ignored because ffmpeg does not decode it.
5. **HDR10+ drives the curve on SDR outputs and is stripped from the HDR10 output.** A channel signals
   one static HDR10 block ([0032](0032-4k-and-dynamic-range.md)), and per-scene metadata would differ at
   every join between items.
6. **"Identical" means within a measured bound of a libplacebo reference, not bit-exact.** Floating-point
   math differs between vendors. The boot self-check keeps its never-black luma test and also compares
   its frames to a shipped reference rendered on lavapipe. CI renders the same references on lavapipe,
   because hosted runners have no GPU. The proposed bound is 55 dB PSNR (luma), below the 59 dB
   measured between NVIDIA and lavapipe. It still needs confirming on the Intel Arc.

## Open questions

- **CPU-only hosts.** On the software family's real graph, pinned to 8 cores, libplacebo on lavapipe
  costs 7.6–9.5 cores per real-time 4K HDR stream at the 720-line working size, against 2.0–2.7 for
  the CPU chain, and ran at 0.33–0.84× real time against 2.1–3.4×. Letting libplacebo scale straight to
  1080p costs 33 cores. At the 480-line size of the lowest quality rung it still costs 4.6–4.7 cores.
  Lavapipe cannot be a live tone-mapper. The proposal: a CPU-only host keeps the CPU chain, as a
  declared exception that is not identical and says so in Diagnostics. The alternative is a card
  instead of HDR programmes.
- **The default curve.** Hable is the default because it was the only curve with a zero-copy path on
  both Intel and NVIDIA and an exact CPU equivalent. Decision 1 removes that reason.

## Consequences

- When a GPU tone-mapper fails at start, the retry ladder falls back to the CPU chain, declared as a
  substitute that is not identical. Never black outranks identical.
- NVIDIA hosts need NVIDIA's Vulkan driver inside the container. With
  `NVIDIA_DRIVER_CAPABILITIES=all`, the toolkit mounts `/etc/vulkan/icd.d/nvidia_icd.json`, but the
  loader cannot create an instance through `libGLX_nvidia.so.0`. A missing GL dispatch library
  (libglvnd) is the likely cause; unverified (#1523).
- Intel keeps `tonemap_opencl` until VAAPI→Vulkan interop is measured at today's cost, about 0.1 CPU
  cores per HDR stream with zero copy. The retirement then removes the Intel compute runtime and the
  NVIDIA OpenCL ICD file from the image, adds `tonemap_opencl` to `scripts/check-retired.sh`, and updates
  `docs/design/dependencies.md`.
- The capacity probe re-measures HDR per curve; the cost table moves with the new graphs.
- libplacebo refuses to create its own device on a software GPU ("Found no suitable device"), so a host
  without a real Vulkan GPU never silently tone-maps on lavapipe. A device handed in by ffmpeg
  (`-init_hw_device vulkan`) bypasses that check, so the builder must never derive one from lavapipe
  for a live stream.
- [playout](../playout.md) and the `playout.tone_curve` setting (design §15) change with the
  implementation, not with this record.
