# Findings: what Loomarr could do by embedding or extending FFmpeg (#1654)

**For:** the maintainer choosing which FFmpeg bets to fund after [#1634](FINDINGS-ffmpeg-patches-2026-09-27.md).
**You'll get:** each direction from #1654 with its prototype, the numbers from NVIDIA (here) and the Arc A380
(the supervisor's run of this lane's kit), a verdict, and a ranking by impact, effort and risk. Research only:
nothing here changes the product or the image.

## Summary

| Rank | Direction | Verdict | Measured | Effort | Risk |
| --- | --- | --- | --- | --- | --- |
| 1 | 1 and 2 together: a long-lived channel media worker that keeps the GPU device warm | **Bet B (#1657), phased prototype, NVIDIA first** | NVIDIA item start **241–266 → 78–98 ms** (clean rows), CPU about −60% on 10 s items, 0 boundary gap, SPS/PPS byte-identical to today's. Arc: the device costs only 32–42 ms per process, so the gain there is smaller | L | Medium |
| 2 | 5: AV1 on the Arc | **Worth a design issue**, for remote viewers | Arc: AV1 at 2.4 Mb/s (actual) scores VMAF 92.9, as H.264 does at 4.0 Mb/s (92.8): **about 40–45% less bitrate at equal VMAF**, at the same 252 fps and 0.33–0.37 cores | M | Medium |
| 3 | 4: one fused GPU pass | **Arc: worth a prototype. NVIDIA: dead end** | Arc HDR upper bound **+47%** (160 → 236 fps without the tone-map stage). NVIDIA: the whole watermark chain is ≤12%, and the 12-session cap binds first | L | High |
| 4 | 3, reframed: skip the stream-info probe | **Conditional (#1656): proven on NVIDIA, Arc proof pending** | NVIDIA, at a production-aimed seek: about **45–50 ms** faster (a 26-frame run, median 473 → 420–429 ms), and 600 frames plus audio **byte-identical**. At a seek under 130 ms past a keyframe, the audio differs by 16 samples (0.33 ms) | XS | Low if gated |
| 5 | 6: live break detection | Cheap signal, small value | Both GPUs: packet sizes found 4/4 black runs within 1 frame at zero cost | S | Low |
| — | 2 as written: a pool of warm NVENC sessions | **Dead end** | A session opens in **15 ms**. Holding one costs a GeForce session slot (12 max) and ~25–30 MiB VRAM | — | — |
| — | 3 as written: seek from our byte offsets | **Dead end** | MKV open plus seek is 3 I/O seeks and under 1 ms locally. A byte seek saves at most 2 round trips | — | — |

**Recommendation.**
1. **Bet B.** An NVENC-first worker as a separate process that holds the CUDA device, with inputs swapped under
   one encoder session per channel. It plugs into the packager's existing seam, `Item.Open` returning an fMP4
   reader.
2. **AV1 and the Arc HDR kernel** are the next candidates. Both pay only in specific places: AV1 for thin-link
   remote viewers, the kernel for HDR channel count on the Arc.
3. **Bet A only after the Arc v2 row** (below) shows byte identity, and then gated to opens with no seek or a
   keyframe-aimed seek.
4. Drop the warm session pool and the byte-offset seek.

### Correction to the first version of this report

- **What version 1 said.** Skipping the probe saved about 120 ms, "because the CLI opens a frame-threaded
  decoder", with a bit-identical picture.
- **Both claims were wrong,** and the Arc run caught it.
  - Both of the 1080p test seeks (2.131 s and 20.131 s) sat under 130 ms past a keyframe. With the probe,
    the CLI backs the seek off by 3/23 s and decodes a whole extra GOP (details under direction 3). Most of
    the "probe cost" was that GOP. The probe's own cost is about 45–50 ms. Rerunning with `-threads 1`
    confirmed it: the output was byte-identical and there was no speedup.
  - "Bit-identical" covered only the video. The audio differed, and the identity check missed it.
- **Also inflated.** The 1080p process rows in the worker comparison fell into the same trap. The HEVC rows
  did not, so they are the clean comparison.

## Measurement setup

- **Machine.** NVIDIA GeForce RTX 3080 Ti, driver 615.71.09, 24 cores shared with other lanes. Every GPU
  command ran under the shared GPU lock.
- **Binaries.**
  - **Process baseline:** BtbN `n9.0.1-11-ge47273f4d9` (the Dockerfile's pin), and the host's shared-library
    FFmpeg `n9.0.2`.
  - **Worker prototype:** C against the host's `n9.0.2` libraries. rsmpeg, ffmpeg-next and cgo all wrap the
    same C API.
- **Arc.** The A380 rows are the supervisor's run of this lane's kit (2026-09-28, rc.2 image, stock n9.0.1),
  posted on #1654.
- **CUDA JIT cache.** Warm for every row. A cold cache adds about 3.6 s to the first tune; #1640 persists it.
- **Sources.** Synthetic (`testsrc2` plus noise), both with B-frames and 2 s GOPs:
  - 1080p 23.976 H.264 MKV from #1634, keyframes every 2.002 s;
  - 720p 29.97 HEVC MKV, made here.
- **Items.** 250 frames (10 s) each.
- **Graph and encoder.** The `nvenc-opencl` golden's SDR graph. `h264_nvenc` p4/ll, VBR cq 22, 8/12 Mbps,
  GOP 25, no B-frames, forced IDR.
- **Output and timing.** The packager's output shape: fMP4 with `empty_moov+default_base_moof+frag_keyframe`.
  "First fragment" is the time from the item's start to its first complete `moof`+`mdat`, as
  `packager.airItem` logs it (`spawn_ms + first_fragment_ms`). One dev-backend log line (2026-09-26 23:45)
  shows `first_fragment_ms=343` from production code.
- **Identity** means byte-identical files (`cmp`), or `framemd5` per stream: the pts, size and hash of every
  frame. Every identity check was sabotage-checked: a one-frame seek shift reports 1200 differing lines and
  23.1 dB.
- **Picture on every row.** Per-frame YAVG (no black frame), frame counts, and SPS/PPS hashes.

## Directions 1 and 2: a long-lived worker, and the warm pool it makes unnecessary

### What was built

A worker (`lw`, about 250 lines of C) that creates one CUDA device and one `h264_nvenc` session at start,
then for each item:
1. opens the input with the production probe limits;
2. seeks with `avformat_seek_file`, which has no CLI backoff;
3. opens an NVDEC decoder on the shared device and drops frames to the target;
4. builds that input's filter graph;
5. feeds frames to the **same** encoder, forcing an IDR on the item's first frame;
6. writes all items into one continuous fMP4.

The baseline (`base`, Go) spawns `ffmpeg` per item with the `FragmentArgs` argv and reads its stdout like
`packager.readStream` does.

### Per-item start: spawn or swap to the first 1 s fragment (ms)

Worker with the probe: three runs. The other columns: two runs, because run 3 was lost to the exit hang (see
traps). **Rows marked "trap"** seek under 130 ms past a keyframe, where the process decodes an extra GOP. The
worker doesn't, so on those rows the process-vs-worker gap is overstated by roughly 80 ms.

| Item | Process, production argv (+AAC), BtbN | Process, video only, BtbN | Process, video only, host n9.0.2 | Worker, warm | Worker, warm, no probe |
| --- | --- | --- | --- | --- | --- |
| 1080p H.264 @2.131, trap (the worker's first decoder) | 379–380 | 409–517 | 524–806 | 139–155 | 110–118 |
| 720p HEVC @5.131, clean | 241–246 | 265–266 | 283–445 | 88–98 | 92–96 |
| 1080p H.264 @20.131, trap | 353–365 | 378–379 | 410–562 | 127–129 | 82–83 |
| 720p HEVC @12.5, clean | 241–247 | 247–258 | 285–461 | 78–83 | 79–82 |

- **The worker's cold start** is paid once, not per item: CUDA device **116–228 ms**, encoder session
  **14–18 ms**.
- **Where an NVIDIA process's time goes,** read off the worker's own breakdown:
  - CUDA device creation: 116–228 ms, the largest part;
  - the stream-info probe on H.264: about 40–50 ms;
  - NVDEC open to the first decoded frame: 22–35 ms;
  - filter graph: 8–15 ms;
  - encoder open: about 15 ms;
  - one 25-frame GOP through NVENC: about 50 ms.
- **On the Arc** a VAAPI device, an `h264_vaapi` session and one frame take **32–42 ms** per process (row A).
  The CUDA context is what makes NVIDIA's per-process cost large. The worker's gain on the Arc wasn't
  measured, and will be much smaller.
- **The <100 ms target** is met for warm items, except the first decoder in a process. The floor is one GOP
  plus NVDEC init (about 75 ms). Going lower needs a shorter first fragment, which is a packager change.

### Boundaries, CPU and memory

| Check | Worker (4 items, 996 frames, one process) | Process per item (4 processes) |
| --- | --- | --- |
| Timestamp step across all 995 packet pairs | 3600 ticks (1/25 s at 90 kHz) every time: **no gap, no overlap** | Stitched by the packager, as today |
| Keyframes | IDR on each item's first frame; GOP cadence restarts per item | Same |
| SPS/PPS | **One** SPS and **one** PPS hash across all 40 IDRs | Same hashes as the worker for both sources, so `SameDecoderConfig` accepts a mix |
| Darkest frame (YAVG) | 121.3: no black frame at any boundary | — |
| CPU (user + sys) | **0.69–0.75 s** in total | 1.82 s (BtbN), 2.11 s (host n9.0.2), video only. Two of the four items include the trap's extra GOP |
| Peak RSS | 348–351 MB for the whole run | 318–387 MB **per process** |

The CPU saving is fixed per item (up to about 0.28 CPU-s), so it matters most for short items: commercials,
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
- **The warmth worth keeping is the CUDA device context** (116–228 ms, about 240 MiB, no session slot). A
  long-lived worker is exactly that, so direction 2 folds into direction 1.

### Traps found

- **Exit hang.** A process that returns from `main` with the CUDA device and NVENC session still referenced
  hangs at exit on host n9.0.2. Freeing the encoder, then the device, fixes it (75 ms teardown).
- **Frame pools.** The encoder needs its own `hw_frames_ctx` to open before any input exists. It accepts frames
  from each item's filter-graph pool on the same device, so graphs can be rebuilt per item under one encoder.

### Effort and risk

- **Licence.** Loomarr is MIT. Linking a GPL build of libav* into the Loomarr binary through cgo would make
  the distributed binary a GPL combined work. **The worker must stay a separate executable,** published with
  its corresponding source.
- **Seam.** `Item.Open` already returns one fMP4 reader per item, and the init is checked with
  `SameDecoderConfig`. The worker's SPS/PPS match today's byte for byte, so it can sit beside the CLI and fall
  back per item.
- **Surface.** Start with NVENC SDR only. The argv builder's nine encoder families, tone-map ladders and
  watermarks stay on the CLI.
- **Blast radius.** One worker per GPU means one crash stops every channel on it. A worker per channel keeps
  today's isolation, at one device context (about 240 MiB) per channel.

## Direction 3: index-driven seek, and the probe

### Byte-offset seek: dead end

- Opening and seeking an MKV costs **3 I/O seeks** (the header, the Cues index, the target cluster), and MP4
  costs 2. All of it takes under 1 ms on a local disk.
- Matroska's Cues already are a keyframe index. Our own byte offsets could save at most 2 round trips on the
  network mount, which is 1–4 ms on a LAN.

### Skipping the probe: what it changes, and when the output is identical

The mechanism, from release/9.0's `fftools/ffmpeg_demux.c`:
1. For a demuxer that seeks by DTS (Matroska), the CLI moves an input seek **3/23 s (130 ms) earlier** when any
   stream has a nonzero `codecpar->video_delay`. This is the backoff #1607 aims around with `demuxSeekBackoff`.
2. `video_delay` (the B-frame reorder depth) is filled in by `avformat_find_stream_info`. **Without the probe it
   stays 0, so there is no backoff.**
3. At a seek under 130 ms past a keyframe, the two runs therefore start decoding at **different keyframes**. At
   2.131 s, the probe run lands on the keyframe at 0 s, and the no-probe run on the one at 2.002 s.
4. The video after the accurate-seek trim is identical, pixels and pts. The audio isn't. MKV stores audio
   timestamps in whole milliseconds, and the CLI counts samples from the first decoded packet's rounded
   timestamp (1877.33 → 1877 ms). The trim keeps **104** samples of the first frame with the probe and **120**
   without it: **16 samples, 0.33 ms**. Every later AAC frame then covers different samples, so the encoded
   audio bytes differ.

Identity of the production argv's output, NVIDIA, 600 frames with audio, MPEG-TS:

| Seek | Probe vs `-nofind_stream_info` |
| --- | --- |
| none (an item that starts at 0) | **byte-identical** |
| 2.142 s and 20.160 s (keyframe + 140 ms, like production's +131 ms tune-in aim) | **byte-identical** |
| 2.131 s (129 ms past a keyframe) | video identical (pts and pixels); audio shifted 16 samples |
| probe vs probe, two runs | byte-identical (the encode is deterministic) |

- **Speed at the clean seeks** (a 26-frame run, 5 runs each): probe 418–548 ms, median 473; no probe 391–489
  ms, medians 420 and 429. About **45–50 ms saved**.
- **The Arc's first result was a comparator bug.** Probe vs no-probe scored "PSNR 24.3". The v1 kit compared
  two MPEG-TS files with `psnr`, which pairs frames **by timestamp**, and the CLI logs
  `Correcting start time of Input #1 by 61333 us`. So even **a file against itself scores 23–24 dB**
  (reproduced here: 23.25). The Arc run also used the 2.131 trap seek, so its 406–427 → 177–205 ms includes
  the extra GOP. Kit v2's row A repeats it with `cmp` and `framemd5`, at the trap seek, at keyframe + 140 ms
  and with no seek, plus a comparator self-check.
- **Where Bet A could apply.** Only opens with no seek, or with a keyframe-aimed seek (tune-in with an indexed
  keyframe, `tuneInSeek`), and only where the stored facts are complete (`minimalProbe`). Other seeks would
  change the audio by up to a sub-millisecond trim, so they must keep the probe.

## Direction 4: one fused GPU pass

- **NVIDIA, from #1634's rows:**
  - production SDR graph: 327–362 fps;
  - with the watermark graph: 295–299 fps;
  - so everything after the decoder costs at most about 12%. The 12-session cap (about 300 fps of 1080p25)
    is reached first. The HDR copy (1.38 → 0.53 cores) is already #1643's.
  - **Verdict: dead end.**
- **Arc, row C.**
  - The production HDR graph (`hwmap` to OpenCL, `tonemap_opencl`, back) runs at **160 fps**, 0.67 cores.
  - The same graph without the tone-map stage runs at **236 fps**. Its picture is wrong by design: YMAX 150.
  - A fused scale and tone-map kernel can therefore gain **at most +47%** HDR throughput. The real gain is
    lower, because the tone map still has to run. At 25 fps per channel that is at most about 6 → 9 HDR
    channels.
- **Worth a prototype on the Arc**, with the #1615 kernel's traps (`settb`, early side-input pts, colour tags)
  and a picture re-baseline as the cost.

## Direction 5: AV1 on the Arc

Arc row B, VBR with maxrate 1.5×, 600 frames, VMAF against a lossless CPU reference:

| Encoder | Target | Actual | VMAF | fps | Cores |
| --- | --- | --- | --- | --- | --- |
| `h264_vaapi` | 2 Mb/s | 2,121 kb/s | 88.8 | 252 | 0.44 |
| `h264_vaapi` | 4 Mb/s | 4,035 kb/s | 92.8 | 253 | 0.38 |
| `h264_vaapi` | 8 Mb/s | 8,118 kb/s | 95.1 | 254 | 0.36 |
| `av1_vaapi` | 2 Mb/s | 2,390 kb/s | 92.9 | 252 | 0.33 |
| `av1_vaapi` | 4 Mb/s | 4,598 kb/s | 94.7 | 253 | 0.37 |
| `av1_vaapi` | 8 Mb/s | 9,590 kb/s | 96.4 | 254 | 0.33 |

- **Bitrate.** Counted at the rates actually produced, AV1 matches H.264's quality with **about 40–45% fewer
  bits**: 2.4 Mb/s against 4.0 at VMAF about 92.9, and 4.6 against 8.1 at 94.7–95.1. It costs the same speed
  and CPU, and the picture is correct on every row.
- **Rate control.** AV1 overshoots its VBR target by 15–20%, so it needs tuning before use.
- **Structure.** An fMP4 rendition can't change codec, so AV1 is a separate channel *format*. The packager is
  one per channel format with refcounted viewers, so an AV1 encode runs **only while an AV1 viewer watches**.
  When H.264 and AV1 viewers watch the same channel, that's two encodes, against #1512's one GPU stream per
  channel.
- **Clients.** Browser support varies ([caniuse: AV1](https://caniuse.com/av1)). TV-client decode support
  wasn't verified here. The RTX 3080 Ti (Ampere) has no AV1 encoder.
- **Impact.** Real for remote or thin-link viewers. Small on a household LAN, where 8 Mb/s is easy. Worth a
  design issue when remote viewing is on the roadmap.

## Direction 6: live break detection

- **The prototype.** A 32 s synthetic source with four black and silent runs went through the production argv.
  Frames were classified from the **encoder's own packet sizes**: a non-keyframe under 1/20 of the median
  P-frame counts as black.

| Truth (`blackdetect`) | NVIDIA proxy | Arc proxy (row D) |
| --- | --- | --- |
| 6.00–6.84 | 6.04–6.84 | 6.04–6.84 |
| 15.20–16.04 | 15.24–16.04 | 15.24–16.04 |
| 24.00–24.40 | 24.04–24.40 | 24.04–24.40 |
| 28.00–28.12 | 28.04–28.12 | 28.04–28.12 |

- **Result:** 4 of 4 runs found on both GPUs, each one frame late. The packager already holds every packet's
  size, so the proxy costs nothing.
- **The CPU alternative** is `blackdetect`: 13 CPU-s for 32 s of 1080p. It would also need a download from the
  GPU, which the playout graph doesn't allow.
- **Limits:**
  - A flat, static frame also encodes small, so pair the proxy with the audio level.
  - Accuracy against real stored marks wasn't measured, because the lane has no real media.
- **Value:** the timeline is fixed ahead of time and the packager runs only 2 s ahead. A live detection can
  confirm, or flag, an ingest mark at air time, but it can't move a break.

## Direction 7: other ideas

- **GPU loudness:** not pursued. Audio is CPU work and cheap.
- **Zero-copy muxing:** not pursued. The packager already packages in Go.
- **Subtitle burn-in on the GPU:** not investigated.

## Arc kit (supervisor-run)

- **Location:** `.agent-data/innovate/arc1654/` in this lane's worktree, in the #1634 kit's style.
- **Run:** `IMAGE=<loomarr image> ./run-arc-1654.sh | tee arc-1654.txt`. Every `ffmpeg` runs under `timeout`.
- **Rows B–D** are the first run's, above.
- **Row A (v2)** replaces the timestamp-paired `psnr` with `cmp` plus per-stream `framemd5`. It runs the trap
  seek, keyframe + 140 ms and no seek, and ends with a comparator self-check (a file against itself must be
  byte-identical).
- **Tested here:** the new comparator and seeks, on NVIDIA with the NVENC golden. The VAAPI argv rewrites were
  dry-run against the golden.

## Recommended bets

- **Bet B: a long-lived NVENC worker (#1657).** Phased, each phase ending at a maintainer decision:
  1. A separate executable, in C or Rust over libav*, that speaks a small stdin protocol ("air path, seek,
     frames") and writes fMP4 to stdout. NVENC SDR items only, behind a setting, with the CLI as the fallback.
     Measure `first_fragment_ms` in the dev backend's log.
  2. Choose one worker per GPU or one per channel, on crash and isolation evidence.
  3. Watermark, HDR and the other families, only if phase 1's numbers hold. VAAPI last: its per-process
     device cost is only 32–42 ms.
- **Bet A: skip the probe, gated (#1656).** Recommend it only after the Arc v2 row shows byte identity at no
  seek and at keyframe + 140 ms. Then apply it only to those opens with complete facts. It gains about 45–50
  ms per H.264 start.
