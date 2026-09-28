# Findings: what Loomarr could do by embedding or extending FFmpeg (#1654)

**For:** the maintainer choosing which FFmpeg bets to fund after [#1634](FINDINGS-ffmpeg-patches-2026-09-27.md).
**You'll get:** each direction from #1654 with its prototype, the numbers, and a verdict, then a ranking by
impact, effort and risk, and the two recommended bets. Research only: nothing here changes the product or
the image.

## Summary

| Rank | Direction | Verdict | Measured (RTX 3080 Ti unless noted) | Effort | Risk |
| --- | --- | --- | --- | --- | --- |
| 1 | 3, reframed: skip the stream-info probe when the stored facts are complete | **Bet A, do now** | 1080p H.264 item start, median of 3, **377–391 → 254–268 ms** with the production argv. Picture bit-identical (PSNR inf). **No patch**: stock `-nofind_stream_info` | XS | Low |
| 2 | 1 and 2 together: a long-lived channel media worker that keeps the GPU device warm | **Bet B, phased prototype** | Item start **241–517 → 78–155 ms**, CPU **−60%** for 10 s items, SPS/PPS byte-identical to today's, 0 boundary gap | L | Medium |
| 3 | 6: live break detection | Cheap signal, small value | Encoder packet sizes found 4/4 black runs within 1 frame at zero cost; CPU `blackdetect` is 0.4 cores and would need a GPU download | S | Low |
| 4 | 4: one fused GPU pass | **Dead end on NVIDIA**; Arc bound pending | Whole watermark chain costs ≤12% throughput. The GeForce's 12-session cap binds first | L | High |
| 5 | 5: AV1 on the Arc | Low impact for a household; Arc numbers pending | Structurally a second encode per channel | M | Medium |
| — | 2 as written: a pool of warm NVENC sessions | **Dead end** | A session opens in **15 ms**. Holding one costs a GeForce session slot (12 max) and ~25–30 MiB VRAM | — | — |
| — | 3 as written: seek from our byte offsets | **Dead end** | MKV open plus seek is 3 I/O seeks and under 1 ms locally. A byte seek saves at most 2 round trips | — | — |

**Recommendation.**
1. **Bet A now.** Add `-nofind_stream_info` to the argv exactly where `minimalProbe` already applies (the stored
   facts are complete). One flag and a goldens update.
2. **Bet B as a phased prototype.** An NVENC-first worker as a separate process: one per GPU, holding the
   CUDA device, one encoder session per channel, inputs swapped underneath. It plugs into the packager's
   existing seam, `Item.Open` returning an fMP4 reader.
3. Drop the warm session pool and the byte-offset seek. Keep direction 4 and direction 5 behind the Arc rows
   (the supervisor's script, below).

## Measurement setup

- **Machine.** NVIDIA GeForce RTX 3080 Ti, driver 615.71.09, 24 cores shared with other lanes (load 2–5
  during the runs). Every GPU command ran under the shared GPU lock.
- **Binaries.**
  - **Process baseline:** BtbN `n9.0.1-11-ge47273f4d9` (the Dockerfile's pin), and the host's shared-library
    FFmpeg `n9.0.2`.
  - **Worker prototype:** C against the host's `n9.0.2` libraries. rsmpeg, ffmpeg-next and cgo all wrap the
    same C API, so the language doesn't change the numbers.
- **CUDA JIT cache.** Warm for every row. A cold cache adds about 3.6 s to the first tune; #1640 persists it.
- **Items.** Four 250-frame (10 s) items, alternating two synthetic sources (`testsrc2` plus noise):
  - 1080p 23.976 H.264 MKV (from #1634), seeks 2.131 s and 20.131 s;
  - 720p 29.97 HEVC MKV (made here), seeks 5.131 s and 12.5 s.
- **Graph and encoder.** The `nvenc-opencl` golden's SDR graph (`scale_cuda`, `pad_cuda`, `fps=25`,
  `setparams`). `h264_nvenc` p4/ll, VBR cq 22, 8/12 Mbps, GOP 25, no B-frames, forced IDR.
- **Output and timing.** The packager's own output shape: fMP4 with
  `empty_moov+default_base_moof+frag_keyframe`. "First fragment" is the time from the item's start to its
  first complete `moof`+`mdat`, the tune-in cost `packager.airItem` logs as `spawn_ms + first_fragment_ms`.
  One dev-backend log line (2026-09-26 23:45) shows `first_fragment_ms=343` from production code, inside the
  baseline's range.
- **Picture.** Checked on every output: per-frame YAVG (no black frame), frame count, and SPS/PPS hashes.
  PSNR is used where two variants should be identical.

## Directions 1 and 2: a long-lived worker, and the warm pool it makes unnecessary

### What was built

A worker (`lw`, about 250 lines of C) that creates one CUDA device and one `h264_nvenc` session at start,
then for each item:
1. opens the input with the production probe limits;
2. seeks to the keyframe;
3. opens an NVDEC decoder on the shared device and drops frames to the target (what `-ss` does);
4. builds the filter graph for that input;
5. feeds frames to the **same** encoder, forcing an IDR on the item's first frame;
6. writes all items into one continuous fMP4.

The baseline (`base`, Go) spawns `ffmpeg` per item with the `FragmentArgs` argv and reads its stdout like
`packager.readStream` does.

### Per-item start: spawn or swap to the first 1 s fragment (ms)

Worker with the probe: three runs. The other columns: two runs, because run 3 was lost to the exit hang
described under traps.

| Item | Process, production argv (+AAC), BtbN | Process, video only, BtbN | Process, video only, host n9.0.2 | Worker, warm | Worker, warm, no probe |
| --- | --- | --- | --- | --- | --- |
| 1080p H.264 @2.131 (the worker's first decoder) | 379–380 | 409–517 | 524–806 | 139–155 | 110–118 |
| 720p HEVC @5.131 | 241–246 | 265–266 | 283–445 | 88–98 | 92–96 |
| 1080p H.264 @20.131 | 353–365 | 378–379 | 410–562 | 127–129 | **82–83** |
| 720p HEVC @12.5 | 241–247 | 247–258 | 285–461 | 78–83 | 79–82 |

- **The worker's cold start** is paid once, not per item: CUDA device **116–228 ms**, encoder session
  **14–18 ms**.
- **Where a process's 250–400 ms goes,** read off the worker's own breakdown:
  - CUDA device creation: 116–228 ms, the largest part;
  - the stream-info probe: about 40 ms in-library on H.264, about 120 ms through the CLI (see direction 3);
  - NVDEC open to the first decoded frame: 22–35 ms;
  - filter graph: 8–15 ms;
  - encoder open: about 15 ms;
  - one 25-frame GOP through NVENC: about 50 ms.
- **The <100 ms target** is met for 720p HEVC, and for 1080p H.264 once the probe is skipped and the
  process's first NVDEC init is behind it. The floor is one GOP plus NVDEC init (about 75 ms). Going lower
  needs a shorter first fragment, which is a packager change, not an FFmpeg one.

### Boundaries, CPU and memory

| Check | Worker (4 items, 996 frames, one process) | Process per item (4 processes) |
| --- | --- | --- |
| Timestamp step across all 995 packet pairs | 3600 ticks (1/25 s at 90 kHz) every time: **no gap, no overlap** | Stitched by the packager, as today |
| Keyframes | IDR on each item's first frame; GOP cadence restarts per item | Same (each process starts on an IDR) |
| SPS/PPS | **One** SPS and **one** PPS hash across all 40 IDRs | Same hashes as the worker for both sources, so `SameDecoderConfig` accepts a mix |
| Darkest frame (YAVG) | 121.3: no black frame at any boundary | — |
| CPU (user + sys) | **0.69–0.75 s** in total | 1.82 s (BtbN), 2.11 s (host n9.0.2), video only |
| Peak RSS | 348–351 MB for the whole run | 318–387 MB **per process** |

The CPU saving is fixed per item (about 0.28 CPU-s), so it matters most for short items: commercials,
bumpers and filler. On a 30-minute programme it is noise.

### Direction 2: what keeping things warm costs

`lw -pool N` opens N idle encoder sessions on one device and holds them:

| Idle sessions held | GPU memory above idle | CPU | Result |
| --- | --- | --- | --- |
| 1 | 267 MiB (CUDA context about 240, plus the session) | 0 | ok |
| 4 | 340 MiB | 0 | ok |
| 13 | 593 MiB at 12 open | 0 | **13th fails:** `OpenEncodeSessionEx failed: incompatible client key (21)` |

- **Verdict:** a warm session pool is the wrong thing to keep warm. A session opens in about 15 ms, and every
  idle one takes one of the GeForce's 12 concurrent sessions, which is channel capacity.
- **The warmth worth keeping is the CUDA device context** (116–228 ms, about 240 MiB, no session slot). One
  context per GPU serves every channel. A long-lived worker is exactly that, so direction 2 folds into
  direction 1.

### Traps found

- **Exit hang.** A process that returns from `main` with the CUDA device and NVENC session still referenced
  hangs at exit, until our 120 s timeout killed it, on host n9.0.2. Freeing the encoder, then the device,
  explicitly fixes it (75 ms teardown). A real worker must tear down in that order and be killed on a
  deadline.
- **Frame pools.** The encoder needs its own `hw_frames_ctx` to open before any input exists. It then accepts
  frames from each item's filter-graph pool on the same device (NVENC registers input surfaces by pointer), so
  graphs can be rebuilt per item under one encoder.

### Effort and risk

- **Licence.** Loomarr is MIT. Linking a GPL build of libav* into the Loomarr binary through cgo would make
  the distributed binary a GPL combined work. **The worker must stay a separate executable**, at arm's
  length like the `ffmpeg` CLI today, and published with the same corresponding source.
- **Seam.** The packager already consumes one fMP4 reader per item (`Item.Open`) and checks the init with
  `SameDecoderConfig`. The worker's SPS/PPS match today's byte for byte, so a worker-backed `Item.Open` can
  sit beside the CLI one and fall back per item.
- **Surface.** The CLI argv builder covers nine encoder families, tone-map ladders, watermarks and the software
  ladder. A worker should start with NVENC SDR only and fall back to the CLI for everything else.
- **Blast radius.** One worker per GPU means one crash stops every channel on it. A worker per channel keeps
  today's isolation but pays the device context per channel (about 240 MiB, no session slot). The prototype
  measured the latter's per-item numbers.

## Direction 3: index-driven seek, and the probe it uncovered

- **Byte-offset seek: dead end.** Opening and seeking an MKV costs **3 I/O seeks** (the header, the Cues index
  near the end, then the target cluster), MP4 costs 2, and all of it takes under 1 ms on a local disk.
  Matroska's Cues already are a keyframe index. Seeking from our own byte offsets could save at most 2 round
  trips on the network mount (`library.path_map`), which is 1–4 ms on a LAN. The +131 ms keyframe aim (#1607)
  already handles accuracy.
- **What does cost time is `avformat_find_stream_info`.** On the H.264 source it decodes frames in software to
  fill in facts we have already stored. In the worker that costs about 40 ms. Through the CLI it costs about
  **120 ms**, because the CLI opens a frame-threaded decoder on a 24-core host. HEVC barely pays it.
- **Stock FFmpeg can skip it:** `-nofind_stream_info`, an input option of the CLI. With the production argv
  (BtbN, video plus AAC, three runs):

| Item | Probe (today) | `-nofind_stream_info` |
| --- | --- | --- |
| 1080p H.264 @2.131 | 388–474 ms | 266–292 ms |
| 1080p H.264 @20.131 | 370–599 ms | 252–278 ms |
| 720p HEVC @5.131 | 247–432 ms | 254–279 ms |
| 720p HEVC @12.5 | 238–342 ms | 249–314 ms |

- **Picture.** Identical: the two outputs have PSNR inf, 250 video frames and 471 audio frames each, and the
  same start times and durations.
- **Where it's safe.** Only where the stored facts are complete, which is the condition that already gates
  `minimalProbe` (`MissingFacts` empty, `internal/playout/pipeline.go`). Where a fact is missing, the probe
  still runs.
- **Pending.** The same rows on the Arc: row A of the kit.

## Direction 4: one fused GPU pass

- **NVIDIA, from #1634's rows** (three runs each):
  - production SDR graph: 327–362 fps at 0.66–0.69 cores;
  - with the production watermark graph (`yuv420p` detour, `overlay_cuda`, re-scale): 295–299 fps (ours
    build);
  - so everything after the decoder costs at most about 12% of throughput, and the CPU doesn't move.
- **Why that isn't user-visible.** At 1080p25 each channel needs 25 fps. The GeForce's 12-session cap
  (measured above) is reached at about 300 fps, before throughput runs out.
- **The NVIDIA HDR gain is the system-memory copy.** It is 1.38 → 0.53 cores, and `tonemap_cuda` or the Vulkan
  route (#1643) already removes it. A fused kernel would add at most the watermark's 12% on top.
- **Verdict:** a dead end on NVIDIA. It carries the #1615 kernel's traps with no visible gain.
- **Arc.** The watermark kernel costs 2–7% (#1641, row D). Row C of the kit bounds what fusing scale and tone
  map could save: the production HDR graph against the same graph without the tone map.

## Direction 5: AV1 on the Arc

- **Not measurable here.** The RTX 3080 Ti (Ampere) has no AV1 encoder. Row B of the kit measures
  `h264_vaapi` against `av1_vaapi` at 2, 4 and 8 Mbps: fps, cores, actual bitrate, VMAF against a lossless
  CPU reference, and the picture.
- **Structural cost, whatever the VMAF:**
  - An fMP4 rendition can't change codec, so AV1 would be a **second rendition, which means a second encode
    per channel**. That breaks the "one GPU stream per channel" budget of #1512.
  - Every client without AV1 decode still needs the H.264 rendition.
  - Browser support varies ([caniuse: AV1](https://caniuse.com/av1)). TV-client decode support was not
    verified here.
- **Where it could pay:** a household LAN streams 8 Mbps without trouble, so the saving only matters for remote
  viewers on thin links. Revisit it with remote viewing, not before.

## Direction 6: live break detection

- **The prototype.** A 32 s synthetic source with four black and silent runs (0.1–0.84 s long) went through
  the production NVENC argv. Frames were classified from the **encoder's own packet sizes**, a non-keyframe
  under 1/20 of the median P-frame counting as black.

| Truth (`blackdetect`) | Packet-size proxy |
| --- | --- |
| 6.00–6.84 | 6.04–6.84 |
| 15.20–16.04 | 15.24–16.04 |
| 24.00–24.40 | 24.04–24.40 |
| 28.00–28.12 | 28.04–28.12 |

- **Result:** 4 of 4 runs found, each one frame (40 ms) late, because the first black frame still carries the
  change. The packager already holds every packet's size in `forward`, so the proxy costs nothing.
- **The CPU alternative** is `blackdetect`: 13 CPU-s for 32 s of 1080p, about 0.4 cores in real time. It
  would also need a download from the GPU, which the playout graph doesn't allow.
- **Limits:**
  - A flat, static frame (a title card) also encodes small, so the proxy needs the audio side. Audio is
    already decoded on the CPU, so a PCM level check is nearly free.
  - Accuracy against real stored marks wasn't measured: the lane has no real media, by the public-repo rule.
- **Value:** the timeline is fixed ahead of time (a channel is a wall clock) and the packager runs only 2 s
  ahead. A break found live can't move a scheduled break. It can only confirm, or flag, an ingest mark at air
  time. That makes it a monitoring signal, not a scheduling input.

## Direction 7: other ideas

- **GPU loudness:** not pursued. Audio is decoded and encoded on the CPU and is cheap.
- **Zero-copy muxing:** not pursued. The packager already packages in Go, in-process.
- **Subtitle burn-in on the GPU:** not investigated.

## Arc kit (supervisor-run)

- **Location:** `.agent-data/innovate/arc1654/` in this lane's worktree, in the #1634 kit's style. It sources
  that kit's helpers verbatim (`lib-1634.sh`) and runs the stock binary only.
- **Run:** `IMAGE=<loomarr image> ./run-arc-1654.sh | tee arc-1654.txt`. It takes about 10 minutes, and every
  `ffmpeg` runs under `timeout`.
- **Rows:**
  - **A.** Per-process fixed cost, and first-fragment time with and without the probe (PSNR check).
  - **B.** AV1 against H.264 at equal bitrate.
  - **C.** The HDR tone-map stage bound.
  - **D.** The packet-size break proxy on VAAPI.
- **Tested so far:** syntax, and that each argv substitution target appears exactly once in its golden. The
  rows haven't run, because this machine has no Arc.

## Recommended bets

- **Bet A: skip the probe when the facts are complete (#1656).** One argv flag in the builder, beside `minimalProbe`,
  plus a goldens update. It gains about 120 ms per H.264 item start on NVIDIA, with an identical picture.
  Confirm on the Arc with row A first.
- **Bet B: a long-lived NVENC worker (#1657).** Phased, each phase ending at a maintainer decision:
  1. A separate executable, in C or Rust over libav*, that speaks a small stdin protocol ("air path, seek,
     frames") and writes fMP4 to stdout. NVENC SDR items only, behind a setting, with the CLI as the
     fallback. Measure `first_fragment_ms` in the dev backend's log.
  2. Choose one worker per GPU or one per channel, on crash and isolation evidence.
  3. Watermark, HDR, VAAPI and the other families, only if phase 1's numbers hold.
