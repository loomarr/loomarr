# Findings: what patching FFmpeg could gain Loomarr (#1634)

**For:** the maintainer deciding whether Loomarr carries FFmpeg patches, and how.
**You'll get:** each playout pain point, the patches that would fix or speed it up, what was measured,
a ranked recommendation, and a phased plan. Research only: nothing here changes the image.

The question is what patches would gain this app. Whether we ship BtbN's binary, Jellyfin's, or our own
build is only the means of carrying the patches worth having, so it's covered last.

## Summary

| Rank | Pain point | Fix | Patch? | Measured benefit (RTX 3080 Ti unless noted) |
| --- | --- | --- | --- | --- |
| 1 | NVIDIA cold tune: the first fragment after a container start takes 4 s | Persist the CUDA JIT cache | **No** (config) | First 1 s fragment: **4,081 ms → 459 ms** |
| 2 | Arc watermark: stock `overlay_opencl` can't blend onto NV12, so we carry a `program_opencl` kernel with three traps | Our upstream fix, FFmpeg PR 24745 | Yes, 2 small commits | Bug region Y/U/V **209/143/83 = CPU reference exactly** (stock 178/166/16). Arc speed: pending the Arc script |
| 3 | NVIDIA HDR: the tone map copies every frame through system memory | Jellyfin `tonemap_cuda` (patch 0004), or stock Vulkan end to end | Yes, 3,300 lines, or none plus the #1523 Vulkan runtime | **1.38 → 0.53 cores** per HDR stream, 201 → 251 fps. The picture differs from today's, so it needs a re-baseline |
| 4 | VAAPI throughput | `scale_vaapi mode=fast` and `async_depth=4`, both Jellyfin defaults | **No** (options exist in stock) | Arc script, pending |
| 5 | NVENC watermark takes a `yuv420p` detour | Jellyfin `overlay_cuda` NV12 (0031) | Yes | **None:** 297 vs 302 fps. Not worth carrying |
| — | Cold seek lands a GOP early on MKV | Patch `ffmpeg_demux.c`'s 3/23 s backoff | Yes, small | None left: aiming 131 ms past the keyframe (#1607) already fixed it |
| — | Boundaries, fMP4/TS packaging, loudness, filler ingest | — | — | No pain point found that a known patch fixes |

**Recommendation.**
1. Do #1 now. It needs no FFmpeg change.
2. Carry exactly one patch, PR 24745, and only after the Arc script shows it at least matches the `program_opencl` kernel's speed.
3. Carry it with **our own build of BtbN's recipe**. Jellyfin's binaries can't take their place: see [The binary question](#the-binary-question).
4. Decide the NVIDIA HDR path (#3) separately. It's the biggest GPU-headroom gain, but it changes the picture.

## Measurement setup

- **GPU and binaries.** NVIDIA GeForce RTX 3080 Ti, driver 615.71.09, host Vulkan and OpenCL ICDs. Binaries:
  - **stock:** the Dockerfile's pin, BtbN `autobuild-2026-08-31-13-27` `n9.0.1-11-ge47273f4d9`, SHA256 verified;
  - **jf:** jellyfin-ffmpeg `v8.1.3-1` portable linux64-gpl;
  - **ours:** BtbN's recipe building `n9.0.2-15-gf3140bb01d`, which is `release/9.0` plus the two PR 24745 commits.
- **Sources.** Synthetic, `testsrc2` plus noise, 30 s each:
  - 1080p 23.976 H.264 SDR;
  - 4K HEVC Main10 PQ/BT.2020 (HDR10 tags, no mastering SEI).
- **Commands.** The builder's own argv from `internal/playout/testdata/pipeline/*.golden`. Only the input, `-ss 2.131`, `-frames:v 600` and the output file are substituted.
- **Runs.** Three runs per variant, all under the GPU lock. The host was shared with other lanes, so treat fps differences under about 10% as noise.
- **Picture.** Every row checks the picture, not just speed (#1516):
  - output `signalstats` YAVG and YMAX;
  - VMAF against the source for SDR, and against today's production output for HDR;
  - for watermarks, the bug region's Y/U/V against a CPU `overlay` reference.
- **CPU.** "cores" is (user + sys) / wall time.

## Pain points, patches and measurements

### 1. NVIDIA cold tune: the CUDA JIT cache is wiped on every container start

BtbN builds with `--enable-cuda-llvm`, so the CUDA filters (`scale_cuda`, `pad_cuda`, `overlay_cuda`,
`bwdif_cuda`) ship as PTX, and the driver compiles them on first use. The compiled kernels are cached in
`$HOME/.nv/ComputeCache`. In the image, `HOME` is `/home/nonroot` in the container's writable layer, so every
container recreate (every upgrade or redeploy) starts with an empty cache.

Measured: one 1 s (25-frame) fragment through the production SDR argv, median of five runs.

| Binary and graph | Cache empty (`CUDA_CACHE_DISABLE=1`) | Cache warm |
| --- | --- | --- |
| stock, SDR | 4,081 ms | 459 ms |
| jf, SDR | 2,036 ms | 415 ms |
| jf, HDR via `tonemap_cuda` | 4,403 ms | 712 ms |

- **Patch?** No. Set `CUDA_CACHE_PATH` to a directory under `/data`, and warm each graph shape at boot.
- **Not a fix:** switching to `--enable-cuda-nvcc`, which precompiles the kernels. It makes the build nonfree
  (`cuda_nvcc` is in `configure`'s `HWACCEL_LIBRARY_NONFREE_LIST`).
- **Not verified:** whether the boot capability probe already warms every kernel a channel later uses. It
  would cover the first tune after boot, but not a kernel it never runs, such as a tone map or overlay.

### 2. Arc watermark: `overlay_opencl` over NV12 (upstream PR 24745)

On the Arc, `overlay_vaapi` runs at about 30 fps (#1595), and `hwmap` to QSV or Vulkan failed there. Stock
`overlay_opencl` reads its alpha from the wrong plane when the main frame is NV12. We therefore carry a custom
`program_opencl` kernel (#1615) with three graph guards (`settb`, early side-input pts, matching colour tags).
Our upstream fix is [FFmpeg PR 24745](https://code.ffmpeg.org/FFmpeg/FFmpeg/pulls/24745), still open.

Measured: a white@0.6 200×80 bug on a 1080p NV12 main, run on OpenCL (NVIDIA), mean of 50 frames.

| Binary | Bug region Y / U / V | Rest of the picture Y |
| --- | --- | --- |
| CPU `overlay` (reference) | 209.0 / 143.0 / 83.0 | 117.8 |
| no bug | 170.0 / 166.0 / 16.0 | 117.8 |
| stock n9.0.1 | 178.2 / 166.0 / 16.0 (quarter luma, no chroma) | 117.8 |
| jf 8.1.3 (its own rewrite, patch 0008) | 209.0 / **166.0 / 16.0** (luma right, chroma untouched) | 117.8 |
| PR 24745 (the lane's minimal build) | 209.0 / 143.0 / 83.0 | 117.8 |
| ours (BtbN recipe + PR 24745) | 209.0 / 143.0 / 83.0 | 117.8 |

- **What the patch gains.** A stock filter replaces the custom kernel and its three traps. The `movie`,
  `alphaextract` and `mergeplanes` bug preparation also goes away.
- **What it doesn't gain here.** Speed on NVIDIA isn't the question: NVENC uses `overlay_cuda`.
- **Jellyfin's rewrite is not a substitute.** It leaves chroma unblended for a planar overlay.
- **Pending.** Whether it beats the kernel's speed on the Arc. The supervisor's script (section E) runs
  `overlay_opencl`, `overlay_vaapi` and `overlay_vulkan` with all three binaries.

### 3. NVIDIA HDR tone map: the system-memory round trip

Today's NVENC HDR graph runs `scale_cuda → hwdownload → hwupload (OpenCL) → tonemap_opencl → hwdownload →
hwupload_cuda`. The `nvenc-libplacebo` graph also downloads, and in the container it can't find a Vulkan
device (#1523).

Measured: 4K HDR10 source to 1080p SDR, NVENC. VMAF is scored against today's production output
(`hdr-ocl-bt`), because each tone mapper has its own curve.

| Graph | Binary | fps (median) | Cores | YAVG / YMAX | VMAF vs production |
| --- | --- | --- | --- | --- | --- |
| production: `tonemap_opencl` round trip | stock | 201 | 1.38 | 129.1 / 229 | — |
| same argv | ours | 231 | 1.40 | 129.1 / 229 | identical picture |
| same argv | jf | 210 | 1.40 | **95.7 / 144** | **7.8** |
| `tonemap_cuda`, no copies (jf patch 0004) | jf | 251 | **0.53** | 94.7 / 143 | 7.8 (97.5 vs jf's OpenCL) |
| production `nvenc-libplacebo` (downloads) | stock | 128 | 1.06 | 156.2 / 232 | 63.8 |
| Vulkan decode → `libplacebo` → `h264_vulkan` (stock FFmpeg 9) | stock | **255** | 0.86 | 156.4 / 237 | 63.3 (90.2 vs libplacebo today) |
| Vulkan decode → `libplacebo` → `hwmap` to CUDA → NVENC | stock | — | — | fails: `hwmap` Vulkan→CUDA is `ENOSYS` (-38) | — |
| software ladder's CPU `tonemap` | stock | 39 | 13.7 | 101.9 / 179 | 61.9 |

- **`tonemap_cuda` saves CPU.** It saves 0.85 cores per HDR stream and is 25% faster. The cost is a
  3,300-line out-of-tree patch, and its curve is calibrated differently (BT.2390 EETF, 203-nit reference
  white). Its `hable` is darker than stock `tonemap_opencl`'s `hable` on the same input. Adopting it means
  re-baselining the tone-map picture checks and choosing peak and curve settings.
- **Jellyfin's patched `tonemap_opencl` (0007) changes today's picture with an unchanged argv.** Any switch
  to Jellyfin binaries would silently change every HDR channel.
- **Stock FFmpeg 9 can already run HDR entirely in Vulkan at twice today's libplacebo speed.** It's blocked in
  the image by the missing Vulkan runtime (#1523). It also encodes with `h264_vulkan`, which scored 98.7 VMAF
  vs NVENC's 99.4 on SDR at the same rate. A small patch implementing Vulkan→CUDA `hwmap` would keep NVENC.

### 4. VAAPI throughput knobs (no patch)

- **Jellyfin patch 0034** makes `scale_vaapi mode=fast` the default.
- **Jellyfin patch 0024** raises the VAAPI encoders' default `async_depth` from 2 to 4.
- Both are plain options in stock FFmpeg, so we can set them in our argv without a patch.
- **Pending:** the Arc script measures both (section C), speed plus picture.

### 5. NVENC watermark: `overlay_cuda` on NV12 (not worth a patch)

Stock `overlay_cuda` rejects an NV12 main (-22), so the production graph scales to `yuv420p` and then re-scales
with `passthrough=0` (the n8 1088-line fix). Jellyfin's patch 0031 accepts NV12.

- **Speed:** 302 fps (jf, NV12 direct) vs 297 (production graph, jf) vs 215–287 (production graph, stock).
- **Picture:** identical, VMAF 99.28 against the source.
- **Verdict:** no measurable gain. Keep the stock graph.
- **Jellyfin 8.1.3 has the n8 bug fixed.** Stock n9.0.1 also blends correctly after NVDEC: bug region
  215.1 / 136.3 / 85.0, against 216.0 / 137.0 / 85.5 for the CPU reference.

### 6. Other areas the issue listed

- **Cold seek.** `ffmpeg_demux.c` backs a seek off 3/23 s when the demuxer lacks `AVFMT_SEEK_TO_PTS` and the
  stream has B-frames. Aiming 131 ms past the keyframe (#1607) already removes the cost, so a patch would add
  nothing.
- **Programme and commercial boundaries, fMP4 and MPEG-TS packaging.** Our boundary constraint is that an
  fMP4 rendition can't change codec mid-stream, which no patch changes. Jellyfin's `hlsenc`/segment patches
  (0001, 0027 Dolby Vision side data into `mpegtsenc`) don't apply: we package in Go and strip HDR side data.
- **Loudness and filler ingest.** These use CPU `loudnorm`, `ebur128`, `blackdetect`, `silencedetect` and
  `thumbnail` outside the playout graph, with no known defect. Jellyfin 0066 (`ffprobe` first video frame
  only) might shorten probes. Not measured.
- **FFmpeg 9.0 features.** Nothing on our paths is new beyond `transpose_cuda`, a Dolby Vision layer-split
  bitstream filter, and the removal of pre-11.1 NVENC SDKs
  ([Changelog](https://github.com/FFmpeg/FFmpeg/blob/release/9.0/Changelog)). The 9.0.1 and 9.0.2 point
  releases are mostly security fixes, including an `hlsenc` heap overflow and `mpegts` bounds. That argues
  for tracking the release branch.

## The binary question

### Jellyfin's prebuilt binaries can't replace BtbN's

- **Licence.** Patch `0026-remove-fdk-aac-from-nonfree.patch` moves `libfdk_aac` out of `configure`'s
  nonfree list, and the portable build is configured with `--enable-gpl --enable-libfdk-aac`.
  - Upstream FFmpeg refuses that combination without `--enable-nonfree`, whose result it labels
    "nonfree and unredistributable" (`configure` on `release/9.0`: `EXTERNAL_LIBRARY_NONFREE_LIST`, and
    `enabled gpl && map "die_license_disabled_gpl nonfree"`).
  - Redistributing that binary in our image carries a licence risk BtbN's GPL build doesn't.
- **Version.** 8.1.3, not 9. The n9 move (#1552) is what made `overlay_cuda` after NVDEC safe in our image.
- **Picture.** The same argv produces a different HDR picture (section 3).
- **Watermark.** Its `overlay_opencl` still leaves chroma unblended (section 2).
- **What it lacks.** PR 24745. Its build does include `tonemap_cuda`, `tonemapx`, a VAAPI `hwupload`,
  OpenCL `scale`, `yadif` and `bwdif`, plus about 100 patches, mostly for D3D11, VideoToolbox, QSV and RK3588,
  which we don't run. The [patch list](https://github.com/jellyfin/jellyfin-ffmpeg/tree/jellyfin/debian/patches)
  is grouped by the pain points above.

### Build our own with BtbN's recipe

- **What was built.** [BtbN/FFmpeg-Builds](https://github.com/BtbN/FFmpeg-Builds) at `16523e26`, with its
  `build.sh` changed only to mount a local FFmpeg tree. Source: `release/9.0` head plus the two PR 24745
  commits.
- **Build time.** **87 s** of compile on a 24-core host under load, plus 57 s to pull the prebuilt
  dependency image (`ghcr.io/btbn/ffmpeg-builds/linux64-gpl-9.0`, 8.7 GB). The dependencies aren't rebuilt.
  Not measured on a CI runner: expect several minutes on 4 cores, dominated by the image pull.
- **Parity.** `-buildconf` and the filter list match the shipped binary, except that the recipe has added
  `--enable-librsvg` since August.
- **Size.** That drift, not our patch, makes `ffmpeg` and `ffprobe` +26.8 MB each (172.7 MB vs 145.9 MB).
  A build trimmed to the features we use would be smaller. Not measured.
- **Speed and picture.** Same as stock on every NVIDIA row (sections 2, 3, 5).
- **Reproducibility trap, measured.** The first attempt built the pinned `n9.0.1-11` source against the
  `:latest` dependency image and failed: `liboapvenc.c` doesn't compile against the newer OpenAPV
  (`release/9.0` fixed it in `a7502e5ff3`). A reproducible build must pin the dependency image by digest,
  keep it, and track the release branch rather than a frozen commit.
- **Upgrade cadence.** Rebasing our two commits from n9.0.1 to n9.0.2+15 was one clean `git rebase`. Each
  FFmpeg point release, security fixes included, becomes a rebase and a rebuild, and the patches drop when
  upstream merges them.
- **GPL corresponding source.** No new process. Today's
  [source publication](evidence/ffmpeg-august-source-retention.md) already ships the FFmpeg source, the BtbN
  recipe and the dependency cache per pin. A custom build publishes the same three, with the FFmpeg tree
  including our commits.
- **arm64.** Not built here. BtbN's `linuxarm64-gpl` variant cross-compiles on an amd64 host from the same
  recipe, so the pipeline covers both architectures.

## Phased plan

Each phase ends at a maintainer decision. No phase changes the image without approval.

1. **Now, no FFmpeg change.** Persist the CUDA JIT cache under `/data` and warm each graph shape at boot
   (#1640). This is the largest user-visible gain found.
2. **Arc evidence.** The supervisor runs the Arc A/B script for sections 2 and 4 (#1641). Decide PR 24745 on
   its speed against the `program_opencl` kernel, and set `mode=fast` and `async_depth` only if the picture
   is unchanged and they're faster.
3. **Carry PR 24745 if phase 2 says so** (#1642). Build the image's FFmpeg from BtbN's recipe with a pinned
   dependency-image digest, from a `release/9.0` branch plus our commits. Publish the sources as today, then
   swap `program_opencl` for `overlay_opencl` behind the existing self-check. Drop the patch when upstream
   merges it.
4. **NVIDIA HDR headroom** (#1643). Choose between porting `tonemap_cuda`, which needs a picture re-baseline,
   and the stock Vulkan route (#1523 runtime, plus `h264_vulkan` or a small Vulkan→CUDA `hwmap` patch).
