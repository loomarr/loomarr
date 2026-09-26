# Spike: phase 0b, 4K and tone-mapping (#1512, G10/G11)

Measurement spike only. The scripts in `spike/4k-tonemap/` are throwaway. This doc gives phase 2 the numbers
it needs to lock the output model for G10 (4K premium formats) and G11 (tone mapping is a given), plus the
CPU-only degradation ladder and the DVR playlist-size risk. Method, conditions and caveats sit next to each
table. Items marked **[maintainer]** are look or product choices that need approval.

## Conditions

- **Intel Arc A380 (VAAPI), household host.** Throwaway containers of the beta.7 production image (ffmpeg
  n8.1.2, iHD 25.2.3, libva 2.22.0) or a test variant layered on it (G11). `--cpus 4 -m 4g`, removed
  afterwards. Sources are 90 s stream-copy excerpts on the host's tmpfs, so start times are warm-disk. The CIFS
  cold-read tail is phase 0's finding and is not re-measured here.
- **Sources (genre words only):** H1 dark modern horror (HDR10), H2 bright desert sci-fi (Dolby Vision
  profile 7, HDR10 base), H3 90s family comedy (DV P7), H4 80s action (HDR10); all 4K HEVC Main10 PQ.
  S4 is a 4K SDR film (HEVC Main10 BT.709). SD is a 1080p SDR H.264 cartoon; LA is a 1080p SDR B&W film
  (1484x1080, pillarboxed).
- **"First 1 s" (start):** spawn → the muxer holds 1 s of output media (ffmpeg `-progress`, 20 ms stats
  period, fMP4 with 1 s fragments). Speed = content s / wall s. **CPU at 1x** = (user+sys) / content s.
  Every graph includes audio (source decode → AAC stereo 192k), as in production.
- Encoder settings (proposed for phase 2): HEVC premium `QVBR q22, 16M target / 24M cap` (VAAPI) or
  `vbr cq22 16M/24M` (NVENC), 1 s closed GOP, no B-frames. The 1080p baseline is phase 0's H.264 QVBR q22 8M/12M.

## Headline findings

1. **G10 is cheap on the Arc.** A 4K HEVC encode (HDR10 passthrough or SDR) starts in about 0.3 s, runs 5.8x,
   and costs 0.07 cores. Four at once still run 2.07x each. By aggregate throughput, **a 4K HEVC encode ≈ 2
   capacity units of a 1080p H.264 stream, not 4.**
2. **`tonemap_vaapi` outputs a black picture on the A380** (Y = 16 on every frame, all four films, production
   image). The merged pipeline builder uses it for Intel HDR, so this is filed as **#1516**. Phase 0's "4K HDR
   2.57x on Arc" number measured this black output.
3. **`tonemap_opencl` on the Arc works and is fast** once the image carries Intel's OpenCL runtime: 8.6x at
   1080p, 5.2x at 4K SDR output, and 6 concurrent streams at 2.55x each. It starts in 244 ms p50 with a warm
   kernel cache (1.1 s when the cache is cold).
4. **libplacebo on the Arc has no zero-copy path** (ANV can't import P010 DRM surfaces, and this Mesa build has
   no Vulkan video decode). It only works through a CPU hop (3.4x, 0.23 cores/stream at 1080p). That matters
   because **BT.2390 exists only in libplacebo.**
5. **SDR→HDR10 on the GPU:** only libplacebo maps SDR white to BT.2408's 203 nits (PQ code 573; target 572).
   Intel VPP (`scale_vaapi` out_color_transfer) pushes SDR white to about 2,600 nits, and `tonemap_vaapi`
   refuses SDR input. Items converted this way keep **byte-identical VPS/SPS/PPS** with native HDR10 items.
6. **CPU-only ladder:** 4K HEVC 10-bit decode alone needs 2.3–2.6 cores at 1x. `-skip_loop_filter all` saves
   only 4–7%, not 20–30%. `-skip_frame noref` is source-dependent (no gain on one film, 3x on another), and
   **`nonref` is not an ffmpeg value**. Keyframes-only (`nokey`) is the one rung that reliably fits
   (0.33–0.41 cores at 1x).
7. **Library:** 1,009 4K movies and 4,562 4K episodes. **No item has a second media source**, so the
   "lighter version" rung has nothing to choose from today.

## 1. G10 premium formats

### Arc (VAAPI): single stream

| Graph | Start p50 (max), n | Speed | CPU at 1x |
|---|---|---|---|
| 4K HDR10 passthrough: `scale_vaapi format=p010` → `hevc_vaapi main10 -sei hdr` | 296 ms (403), 10 | 5.7–5.8x | 0.066–0.075 |
| 4K SDR: `scale_vaapi format=nv12` → `hevc_vaapi main` (S4) | 295 ms (296), 5 | 5.7x | 0.077 |
| 4K HDR → 4K SDR: `tonemap_opencl` Hable → `hevc_vaapi main` | 345 ms (389), 10 | 5.2x | 0.117 |
| 1080p SDR H.264 (SD), reference | 128 ms (142), 5 | n/a | n/a |
| ~~4K HDR → 4K SDR via `tonemap_vaapi`~~ | 1,420 ms | 0.79x | black output (#1516) |

- **HDR10 static metadata is preserved:** mastering display and content light level travel from the source
  frames into the HEVC SEI and the fMP4 sample entry (`-sei hdr`).
- **Dolby Vision:** VAAPI decodes the HDR10 base layer. The RPU and the P7 enhancement layer are dropped, and
  the output carries no DV configuration record. **DV titles air as HDR10.** P7 decode logs "PPS changed
  between slices" (the EL), which is harmless. Profile 5 (no HDR10 base) was not in the samples, so it is
  untested and would need a tone-map from IPTPQc2.

### Arc: concurrency (60 s content each, unpaced)

| Mix | 4K stream speed | 1080p stream speed |
|---|---|---|
| 1 × 4K HDR10 | 5.80x | n/a |
| 2 × 4K HDR10 | 4.1x each | n/a |
| 3 × 4K HDR10 | 2.8x each | n/a |
| 4 × 4K HDR10 | 2.07x each | n/a |
| 1 × 4K HDR10 + 6 × 1080p SDR | 3.02x | 3.36–3.40x |
| 1 × 4K HDR10 + 10 × 1080p SDR | 2.18x | 2.29–2.34x |
| 1 × 4K SDR + 6 × 1080p SDR | 3.12x | 3.43–3.45x |

- CPU under load: 0.072 cores per 4K stream and 0.046 per 1080p stream. The G5 budget (≤ 1 core total) holds
  at these mixes.
- **Capacity unit:** with phase 0's aggregate of about 27 1080p-units, a 4K HEVC encode costs about 1.8–2.2
  units. **Proposal: count a 4K premium encode as 2 units on Intel, not 4.** The NVENC figure is in §1b.
- The two mixes measured with the `tonemap_vaapi` baseline (a 4K premium plus its 1080p tone-mapped baseline)
  showed that filter as the bottleneck (1.05x with two pairs). They are **void** because the output was
  black; the OpenCL tone-map replaces them (§2).

### 1b. NVENC (RTX 3080 Ti, native n9.0.2)

40 s excerpts on local NVMe, same audio and fMP4 settings, under the shared GPU lock.

| Graph | Start p50 (max), n = 8 | Speed | CPU at 1x | Output check |
|---|---|---|---|---|
| 4K HDR10 passthrough: `scale_cuda format=p010le` → `hevc_nvenc main10` | 684 ms (1,409) | 4.3x | 0.10 | mastering display + CLL side data preserved |
| 4K SDR: `scale_cuda format=nv12` → `hevc_nvenc main` (S4) | 514 ms (578), n = 4 | 4.2x | 0.095 | n/a |
| HDR → 1080p, `tonemap_opencl` Hable (CUDA → download → OpenCL → download → CUDA) | **578 ms** (874 cold) | 4.9x | **0.32** | YAVG 29 (not black) |
| HDR → 1080p, libplacebo BT.2390 (phase 1a's graph) | 2,590 ms (9,610) | 1.1x | 0.41 | n/a |
| HDR → 4K SDR, `tonemap_opencl` (the 4K SDR premium's HDR items) | 989 ms (1,693) | **0.69x ✗** | **0.95 ✗** | n/a |

| Mix (30 s each) | Speeds |
|---|---|
| 3 × 4K HDR10 | 1.47–1.50x each (aggregate 4.5x ≈ one stream's 4.3x) |
| 1 × 4K HDR10 + 1 × OpenCL 1080p baseline + 6 × 1080p SDR | 4K 1.51x, OpenCL 1.47x, 1080p 1.51–1.62x |
| 4 × OpenCL 1080p baselines | 2.02–2.08x each, **0.50 cores/stream** |

- **Phase 0 decision 3 is met:** `tonemap_opencl` brings the NVIDIA HDR baseline's start from libplacebo's
  2.2–2.6 s down to 0.58 s. The price is CPU: CUDA↔OpenCL has no interop in ffmpeg, so the frames round-trip
  through system memory. That costs 0.32 cores at 1x and 0.50 under load, which **breaks G5's 1-core total at
  about 2–3 concurrent HDR baselines on a GeForce host.** Record: phase 2 either caps concurrent HDR baselines
  on NVIDIA by the CPU allowance (the capacity rule already takes the min) or moves to a CUDA tone-map in the
  contingency Rust worker.
- **4K on NVIDIA:** three 4K HEVC encodes saturate the card (decode-bound; the aggregate doesn't grow).
  **Count a 4K premium encode as about 4 units on NVIDIA** (as the issue proposed), versus about 2 on the Arc.
  The unit is per family and comes from the probe.
- **A 4K SDR premium with HDR items is not viable on NVIDIA through OpenCL** (0.69x, 0.95 cores): 4K frames
  round-trip through the CPU twice. It needs a GPU-resident tone-map (libplacebo on the CUDA-derived Vulkan
  device, not measured here at 4K) or the Rust-worker contingency.

### SDR → HDR10 on the GPU (SDR items on an HDR channel)

Pattern test: a BT.709 frame at 100% white and 50% grey through each path; 10-bit PQ luma code measured. BT.2408
puts SDR reference white at 203 nits, which is PQ code 572.

| Path | White → code (nits) | Grey | Verdict |
|---|---|---|---|
| libplacebo (`colorspace=bt2020nc:color_trc=smpte2084`, no inverse tone-map) | **573 (≈ 203)** | 428 | correct (BT.2408) |
| libplacebo + `inverse_tonemapping=1` | clip mean 437 vs 317 | n/a | expands highlights; commercials would glare, **don't** |
| Intel VPP `scale_vaapi out_color_transfer=smpte2084` | 816 (≈ 2,600) | 441 | wrong: white near peak |
| `tonemap_vaapi` to PQ | n/a | n/a | refuses: "No mastering display data from input" |
| CPU `zscale npl=203`, reference attempt | 509 (≈ 100) | 373 | zscale ignores `npl` for BT.709 input; not a usable reference |

Cost on the Arc (libplacebo needs a CPU hop, see §2):
- converting at 4K output: 2.2x, 0.20 cores at 1x, start 741–910 ms;
- **converting at the source size, then GPU upscale** (`libplacebo` 1080p → `hwupload` → `scale_vaapi` 4K →
  `pad_vaapi`): 3.9–4.5x, 0.17–0.22 cores at 1x, start 434–782 ms. **Proposed graph.**

On NVENC libplacebo runs on the GPU. See §1b for its cost.

### Gapless boundaries (Arc, production-shape: one ffmpeg process per item)

HDR channel: SDR-converted (SD) → HDR10 (H1) → SDR-converted (LA, pillarboxed), 240 frames each, identical
encoder flags, colour tags set in-graph with `setparams` (encoder colour flags break format negotiation with
`scale_vaapi` in n8.1). 4K SDR channel: SD upscaled → native 4K SDR (S4) → HDR tone-mapped by
`tonemap_opencl` (H1) → SD.

| Check | HDR set (3 items) | SDR set (4 items) |
|---|---|---|
| VPS/SPS/PPS bytes (md5) | **identical** | **identical** (includes the OpenCL item) |
| Video frames per item | exactly 240 | 240 |
| Audio per item (target 480,480 samples) | +1024 priming, then +0 / +32 / −24 | as HDR |
| Per-item HDR SEI (mastering display / CLL) | **HDR item only (40 B); converted items none** | none |

- With phase 0's rules (drop the AAC priming frame, time audio by sample count, rewrite `tfdt` from frame
  counts), the residual is within ±32 samples, so every seam is **≤ 1 AAC frame**. Joining with ffmpeg's
  concat demuxer showed 63 ms gaps and audio jitter. Those come from the concat demuxer, not the stream, and
  are not a finding. The production packager (phase 0) doesn't use it.
- **HDR static metadata varies per item.** The first item's init segment decides what a player sees, and
  per-frame SEI comes and goes. **Proposal:** encode without per-frame HDR SEI (clear the frame side data
  before the encoder, or drop the `hdr` SEI flag) and have the phase 2 packager write one fixed channel-level
  `mdcv`/`clli` into the init segment (e.g. P3-D65 1000 nits, MaxCLL 1000 / MaxFALL 400). **[maintainer]**
  for the values.

### Playback

The two boundary sets above (HDR: 3 items, 30 s; SDR: 4 items, 40 s) were packaged as VOD HLS with one
`EXT-X-MAP` init, 1 s fMP4 segments and no discontinuity tags. That is valid only because the parameter sets
are identical.

| Player | 4K HDR10 HEVC (Main10) | 4K SDR HEVC (Main) |
|---|---|---|
| ExoPlayer (media3 1.9.1, Android TV emulator API 30, `OMX.google.hevc.decoder`) | **decode fails at once** (`ERROR_CODE_DECODING_FAILED`): the emulator's software HEVC decoder has no Main10. HDR can't be tested on the emulator | **1 decoder init, 0 format changes across 3 boundaries** (SD-upscaled → native 4K → OpenCL-tone-mapped HDR → SD); first frame 1.38 s; 1 rebuffer and dropped-frame reports of 7 and 26 at 32.7–40 s (near the 3rd boundary, 30.0 s). Attributed to the emulator's software 4K decode, **not proven** |
| hls.js 1.7.1, Playwright Chromium | `isTypeSupported('hvc1.2.4.L153.B0')` = false | `hvc1.1.6.L153.B0` = false |
| hls.js, Playwright Firefox | false (and no AVC either) | false |
| hls.js, Playwright WebKit | not measured: the browser process crashed | n/a |

- **HEVC on the web isn't available in any browser we can automate here.** Chrome/Edge on Linux and Windows
  only decode HEVC with hardware support, and Safari does natively. Phase 2's web client must feature-detect
  (`MediaSource.isTypeSupported`) and otherwise take the H.264 baseline.
- **HDR10 playback needs the real Shield** (Main10 hardware decode, HDMI HDR mode). Phase 2's device matching
  must take HEVC Main10 from the device's `MediaCodecList`, not assume it from Android TV.
- **ExoPlayer DVR delta updates: not established.** The delta run hit a stale non-skip test server (a harness
  fault), so ExoPlayer reloaded the full 70 KB playlist. Media3 documents EXT-X-SKIP support, but that's
  unverified here.

## 2. G11 tone-map runtime

### Test image (never pushed)

`spike/4k-tonemap/image/Dockerfile` layers on the production image. The **Mesa Vulkan ICDs (ANV, RADV,
lavapipe) are already in the production image**; only OpenCL was missing.

| Variant | How | Image size delta | Arc result |
|---|---|---|---|
| Intel compute-runtime 26.35 + IGC 2.41 | release debs from Intel's GitHub (**`intel-opencl-icd` is not in Debian trixie**) | **+432 MB** net (+543 MB with the COPY layer; use a multi-stage/bind mount) | `tonemap_opencl` works, zero-copy via `hwmap` |
| Mesa rusticl (`mesa-opencl-icd`) | Debian package | +318 MB | **no-go:** no VAAPI↔OpenCL sharing, so `opencl@va` derivation fails |

The production Dockerfile's apt snapshot pin (`snapshot.debian.org/…20260824`) has expired `Release` files,
so any new apt layer needs `-o Acquire::Check-Valid-Until=false` or a fresh pin.

### Arc: tone-map paths (HDR → 1080p SDR H.264 unless noted)

| Path | Start p50 (max) | Speed | CPU at 1x | 6 concurrent | Output |
|---|---|---|---|---|---|
| `tonemap_opencl` Hable (VAAPI → `hwmap` OpenCL → back, zero-copy) | **244 ms** (262); first-ever 1,087 ms | **8.6x** | 0.107 | **2.55x each** (agg. 15x) | correct |
| `tonemap_opencl` Hable, 4K SDR HEVC out | 345 ms (389) | 5.2x | 0.117 | n/a | correct |
| libplacebo BT.2390, CPU hop (VAAPI → download p010 → libplacebo → upload) | 657 ms (808) | 3.4x | 0.226 | 1.29x each, 0.41 cores/stream | correct |
| libplacebo on a VAAPI-derived Vulkan device (zero-copy) | fails | n/a | n/a | n/a | ANV: `VK_ERROR_OUT_OF_DEVICE_MEMORY` importing P010 (driver "FINISHME: multi-planar DRM modifiers") |
| Vulkan decode → libplacebo | fails | n/a | n/a | n/a | this ANV exposes no `VK_KHR_video_decode_queue` |
| `tonemap_vaapi` (production today) | 550 ms | 2.47x | 0.109 | 0.46x each | **black (#1516)** |

- **OpenCL kernel cache:** Intel's runtime compiles the kernels on the first use in a container (+850 ms) and
  caches them under `~/.cache/neo_compiler_cache`. **Proposal:** the G11 startup self-check (phase 1b probe)
  runs the tone-map once at boot, which warms the cache, and phase 2 points the cache at the data volume so it
  survives restarts.
- **Picture check (signalstats, same graphs):** `tonemap_opencl` output YAVG 30.7 / 82.1 / 46.7 on H1 / H2 / H4 and 29 on NVIDIA; `tonemap_vaapi` YMAX = 16 on every frame. Every OpenCL number above is for a real picture.
- **The self-check must assert on pixels**, not just exit status: a luma bound on a known HDR frame (YAVG
  well above 16). `tonemap_vaapi` exits 0 with a black picture.
- 6 concurrent OpenCL tone-maps at 2.55x each extrapolate to **about 12 HDR baselines at ≥ 1.2x**, the same
  scale as phase 0's 1080p capacity.

### NVIDIA container path

Production image (beta.7) on the dev machine with `--gpus all -e NVIDIA_DRIVER_CAPABILITIES=compute,video,utility`.
The machine's CDI spec is stale (`/dev/dri/card1` missing), so `--device nvidia.com/gpu=all` fails; that's an
environment issue, not an image one.

| Check | Result |
|---|---|
| NVIDIA OpenCL library injected by the toolkit (`compute`) | yes: `libnvidia-opencl.so.1` |
| `-init_hw_device opencl` in the production image | **fails: -1001** (no `/etc/OpenCL/vendors`, so the ICD loader finds no platform) |
| With a one-line `/etc/OpenCL/vendors/nvidia.icd` (`libnvidia-opencl.so.1`) mounted | works |
| `tonemap_opencl` 1080p in the container | start 595–606 ms (4,416 ms on the first spawn in a fresh container), 8.7x, 0.18 cores at 1x, YAVG 29 (not black) |

**Requirement:** the image ships `/etc/OpenCL/vendors/nvidia.icd` (a few bytes, inert without the NVIDIA
toolkit). The compose docs require `NVIDIA_DRIVER_CAPABILITIES` to include `compute`; `video` alone gives
NVENC but not OpenCL. This is exactly why phase 1a's dev check passed natively but would silently fail in
Docker.

### Curve side-by-side **[maintainer]**

The frames are 4 films × the darkest, middle and brightest of five candidate timestamps (15–75% of runtime),
rendered on the Arc and kept in the supervisor's scratchpad (`tonecurves/`). They are **never committed**.
- **First set (3 panels):** Hable | BT.2390 | Intel VPP. The VPP panel is black in every sheet (#1516).
- **Second set (`sheet6_*`, 2×3 panels):** Hable | Mobius | Reinhard (`tonemap_opencl`, zero-copy) over
  BT.2390 | BT.2446a | Spline (libplacebo).

Per-stream cost on the Arc (4K HDR10 film → 1080p SDR H.264, 20 s read from the library share):

| Curve | Path on the Arc | Speed | CPU at 1x | Start | NVIDIA | Software | Apple |
|---|---|---|---|---|---|---|---|
| Hable | `tonemap_opencl`, zero-copy | 5.5x | 0.121 | 464 ms | `tonemap_opencl` | `tonemap=hable` | CPU `tonemap` |
| Mobius | `tonemap_opencl` | 7.6x | 0.108 | 383 ms | `tonemap_opencl` | `tonemap=mobius` | CPU |
| Reinhard | `tonemap_opencl` | 7.8x | 0.104 | 332 ms | `tonemap_opencl` | `tonemap=reinhard` | CPU |
| BT.2390 | libplacebo, CPU hop | 3.0x | 0.244 | 693 ms | libplacebo (2.2–2.6 s start) | libplacebo on lavapipe (not measured) | none in Homebrew's ffmpeg |
| BT.2446a | libplacebo, CPU hop | 3.4x | 0.206 | 616 ms | libplacebo | same | none |
| Spline | libplacebo, CPU hop | 3.3x | 0.214 | 654 ms | libplacebo | same | none |

**Recommendation:** Hable (or Mobius/Reinhard if the maintainer prefers their brighter look, since cost is
equal). These are the only curves with a fast zero-copy path on Intel and an exact CPU equivalent in ffmpeg's
`tonemap`, so "one curve everywhere" (G11) holds on every host. The libplacebo curves cost about 2x the CPU
per HDR stream on Intel and start slowly on NVIDIA. (A BT.2390 `program_opencl` kernel is possible later.)

## 3. CPU-only degradation ladder

Software-only container (no `/dev/dri`, `--cpus 4`), H1 and H2, 20 s content, CPU tone-map Hable after
downscale, libx264 veryfast, output held at the source's 23.976 fps by `fps` (frames repeated).

| Rung | 720p speed (H1 / H2) | 720p cores at 1x | 480p speed | 480p cores at 1x | Output frames (of 480) |
|---|---|---|---|---|---|
| 0 baseline | 1.06x / 1.15x | 3.66 / 3.44 | 1.19x / 1.33x | 3.21 / 2.97 | 480 |
| 1 `-skip_loop_filter all` | 1.11x / 1.23x | 3.48 / 3.23 | 1.28x / 1.42x | 3.01 / 2.77 | 480 |
| 2 + `-skip_frame noref` | 1.11x / **2.98x** | 3.50 / **1.28** | 1.25x / **3.51x** | 3.06 / **1.03** | 480 (H2 decodes 167) |
| 3 + `-skip_frame nokey` | **5.07x / 5.11x** | **0.41 / 0.41** | 5.00x / 5.63x | 0.33 / 0.32 | 457 / 462 (about 1 fps of motion) |

Decode alone (null sink): 1.43x / 1.71x, **2.58 / 2.31 cores at 1x**. With `-skip_loop_filter all`: 2.47 /
2.14 cores, so −4 to −7%. `-skip_frame bidir` keeps 42 / 166 of 480 frames (source-dependent).

- **With about 1 core reserved for the app (3 left), rungs 0–1 don't fit.** Rung 2 fits only for sources
  whose GOPs have non-reference frames (H2 yes, H1 no), so it can't be a scheduled promise. **Rung 3
  (keyframes only) is the only reliable CPU rung for 4K HEVC 10-bit**, at about 0.4 cores. That makes it a
  slideshow with continuous audio.
- **`-skip_frame nonref` (the brief's wording) is not an ffmpeg value**: the option parser rejects it. The
  real keyword is `noref`.
- **Rung 1, the lighter version:** Emby's MediaSources (read-only) show 1,009 4K movies and 4,562 4K episodes,
  and **0 items with more than one media source**. The rung needs Loomarr's own inventory to link separate
  files as versions of one title; today it covers nothing.
- **Proposal [maintainer]:** on software-only hosts, schedule-time choice of a lighter version when one exists;
  otherwise the live monitor steps from 720p straight to 480p keyframes-only (skip `noref`, whose win can't be
  predicted). Audio never degrades. Rung 2 stays a probe-time option per source (measure whether the
  source's GOP has non-reference frames).

## 4. DVR playlist size

Synthetic live playlist (`spike/4k-tonemap/play/server.cjs`): a 900-entry window of 1 s fMP4 segments, one
new segment per second, with `EXT-X-PROGRAM-DATE-TIME` on every entry. hls.js 1.7.1 in Playwright's Chromium,
45 s per mode, `LEVEL_LOADED` stats per reload.

| Mode | Playlist bytes | Reloads | Load (avg) | Parse (avg / max) | Playback |
|---|---|---|---|---|---|
| Full playlist | **70.2 KB** | 44 | 6.9 ms | 3.3 / 9.1 ms | 0 errors, 0 dropped |
| Delta (`CAN-SKIP-UNTIL=6.0`, `_HLS_skip=YES`) | **1.1 KB** | 45 (45 of 46 requests carried `_HLS_skip=YES`) | 4.0 ms | **0.19 / 0.5 ms** | 0 errors, 0 dropped |

- A full reload costs about 3 ms of main thread per second in Chromium. That isn't a stall risk on a desktop,
  but on a TV-class CPU (roughly 5–10x slower) it becomes 15–30 ms every second, plus 70 KB/s of manifest.
- **hls.js requests and applies delta updates without `lowLatencyMode`** once the server advertises
  `CAN-SKIP-UNTIL`: 64x fewer bytes and 17x less parse.
- **Recommendation:** the phase 2 origin serves `EXT-X-SERVER-CONTROL:CAN-SKIP-UNTIL=<≥ 6 × target
  duration>` with `EXT-X-SKIP` delta playlists, and emits `EXT-X-PROGRAM-DATE-TIME` only on the first segment
  and after discontinuities (50 of each entry's 78 bytes, about 64% of the full playlist).
- ExoPlayer: see Playback.

## Go / no-go

| Item | Verdict | Condition / note |
|---|---|---|
| G10 4K HDR10 premium, Arc | **GO** | 0.3 s start, 5.8x, 0.07 cores; 2 units |
| G10 4K HDR10 premium, NVENC | **GO** | 0.7 s start, 4.3x, 0.10 cores; about 4 units (3 × 4K saturates) |
| G10 4K SDR premium, native SDR items | **GO** (both) | Arc 5.7x / NVENC 4.2x |
| G10 4K SDR premium, HDR items tone-mapped at 4K | Arc **GO** (OpenCL 5.2x); NVIDIA **NO-GO** via OpenCL (0.69x, 0.95 cores) | NVIDIA needs a GPU-resident path (open) |
| G10 SDR → HDR10 on an HDR stream | **GO** via libplacebo (BT.2408-correct); on the Arc through a CPU hop at 0.17–0.22 cores per converted item | Intel VPP paths wrong or refused |
| G10 gapless SDR-converted ↔ HDR boundaries | **GO** | identical VPS/SPS/PPS; seams ≤ 1 AAC frame with phase 0's packager rules; HDR static metadata must become channel-level (proposal) |
| G10 HEVC in hls.js | **NO-GO in the tested browsers** | Playwright's Chromium and Firefox report no HEVC; WebKit crashed; real Chrome/Edge/Safari with hardware HEVC untested. Web gets the H.264 baseline unless `isTypeSupported('hvc1…')` says otherwise |
| G10 HEVC in ExoPlayer | see Playback | n/a |
| G11 Intel runtime | **GO** with Intel compute-runtime (+432 MB) and `tonemap_opencl` | `tonemap_vaapi` **NO-GO: black (#1516)**; rusticl no-go; libplacebo zero-copy no-go |
| G11 NVIDIA container runtime | **GO** with `/etc/OpenCL/vendors/nvidia.icd` in the image + `compute` capability | CPU 0.18–0.5 cores per HDR stream caps concurrency under G5 |
| G11 startup self-check | **GO**, but it must assert luma on a real HDR frame (a clean exit proved nothing here) | it also warms the OpenCL kernel cache |
| G11 curve | **[maintainer]**: 6-curve sheets delivered; the cost favours Hable/Mobius/Reinhard (OpenCL, 0.10–0.12 cores) over BT.2390/BT.2446a/Spline (libplacebo hop, 0.21–0.24 cores on the Arc) | recommend Hable unless the look says otherwise |
| CPU ladder rung 1 (lighter version) | **covers 0 titles today** | needs multi-source inventory |
| CPU ladder rungs 2–3 (skip loop filter, noref) | **NO-GO as promises** | 4–7% and source-dependent |
| CPU ladder rung 4 (keyframes only) | **GO** as the last resort | 0.33–0.41 cores; a slideshow with continuous audio |
| DVR 900-entry playlist | **GO with delta updates** | hls.js: 70 KB → 1.1 KB, 3.3 → 0.19 ms parse |

Nothing measured invalidates G10 or G11. The one near-miss is G5 on NVIDIA HDR baselines (CPU round-trip),
which the capacity rule absorbs.

## Decisions (supervisor/maintainer, 2026-09-26)

These settle the five open questions above. Where they differ from a proposal earlier in this doc, the
decision wins.

1. **Tone curve:** Hable is the default, and all six curves (Hable, Mobius, Reinhard, BT.2390, BT.2446a, Spline)
   are selectable (maintainer decision, implemented in #1522). The per-curve cost table in §2 is what a
   non-default choice costs.
2. **HDR10 metadata:** the packager/encoder writes **static, channel-level** HDR10 SEI and strips per-item HDR
   SEI, so the TV never re-evaluates mid-stream:
   - `mdcv`: BT.2020 primaries, D65 white point, max 1000 nits, min 0.0001 nits.
   - `clli`: MaxCLL 1000, MaxFALL 400.
   This replaces the "P3-D65" example in §1's proposal.
3. **Software ladder:** the "lighter version" rung is **dropped** (0 items have one). The rungs are:
   1. 720p with `-skip_loop_filter all`;
   2. `-skip_frame noref`, **only where the source measurably gains**, otherwise skipped;
   3. keyframes-only at 480p.
   Software hosts never produce a premium format, and audio never degrades.
4. **4K capacity unit:** not a constant. The 1b resource budget **measures the 4K class per host at boot**.
   This spike's Arc ≈ 2 and NVENC ≈ 4 are documented priors and fallbacks only.
5. **4K SDR premium with HDR items: moot by design.** Dynamic range is derived independently from the lineup,
   so any HDR item makes the premium format 4K HDR, and its SDR items are converted. A 4K SDR premium never
   carries HDR items, so there's no 4K tone-map on NVIDIA and no Rust-worker contingency for it. The
   "NVIDIA NO-GO" row in Go / no-go is therefore not a gap. SDR→HDR timing on NVIDIA moves to the G10 lane.

## Carry-overs (G10 lane / certification)

Not measured here; each has an owner:

| Item | Owner |
|---|---|
| SDR → HDR10 conversion timing on NVIDIA (libplacebo on Vulkan, expected to run on the GPU) | G10 lane |
| ExoPlayer EXT-X-SKIP delta updates (this run hit a stale test server, a harness fault) | G10 lane |
| HDR10 through ExoPlayer on a real Shield (Main10 hardware decode, HDMI HDR mode; the emulator can't) | certification |
| HEVC in real Chrome/Edge (hardware HEVC) and Safari; WebKit (the Playwright build crashed) | certification |
| Dolby Vision profile 5 sources (none in the samples); DV RPU passthrough (neither encoder carries it) | G10 lane |
| Cold CIFS reads for 4K starts (phase 0 covered the tail on H.264) | G10 lane |
| Apple Silicon (needs the real Mac run) | certification |
