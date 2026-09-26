# Spike: on-demand playout, phase 0 measurements (#1512)

Measured 2026-09-26 on the household install: Intel i5-11500 with an Intel Arc A380 over VAAPI. Every run
used a throwaway container from the production image (ffmpeg n8.1) capped at `--cpus 4 -m 4g`, the same
cap as production, and was removed afterwards. Media was read directly from the CIFS library mount,
read-only. The running Loomarr container, its settings, jobs and data were not touched.

Code: `spike/ondemand-playout/` (its own Go module, never imported by production). Raw data and the exact
scripts are in `spike/ondemand-playout/results/`. Sample paths are replaced by `$SAMPLE_*` placeholders.

Samples (genre only):
- H.264 1080p: an animated series, 8-bit, EAC3 2.0, 23.976 fps.
- HEVC 1080p: a live-action sitcom, Main10 SDR bt709, 25 fps, EAC3 5.1.
- 4K HDR: a live-action film, HEVC DV/HDR10, TrueHD 7.1.
- Four commercial clips from the filler library: 640x360 at 23.976, 640x480 at 29.97, 640x480 at 50, and
  1280x720 at 29.97 with 44.1 kHz audio.
- For the distinct-file cold-start runs: 160 different H.264 1080p episodes.

## Verdict per phase-0 threshold

| # | Threshold | Result | Go? |
|---|---|---|---|
| 1 | Cold tune → first segment p95: H.264 1080p ≤ 400 ms | 1 s segments with minimal probing, 40 fresh files: **348 ms** (max 370). 2 s segments: **496 ms** (pooled 40 runs) ✗ | **GO at 1 s** (2 s recorded as a miss) |
| 1 | HEVC 1080p ≤ 500 ms | 1 s: 197 ms; 2 s: 238 ms (one file, 20 seeks) | GO |
| 1 | 4K HDR ≤ 1.8 s | 1 s: 896 ms; 2 s: 1291 ms (one file, 20 seeks) | GO |
| 2 | Speed ≥ 15x (1080p), ≥ 2x (4K HDR) | H.264 17.6–19.1x, HEVC 18.9x, 4K HDR 2.57x | GO |
| 2 | CPU ≤ 0.06 cores per stream (original) | 0.081–0.088 (H.264), 0.082 (HEVC), 0.118–0.125 (HDR) | **MISS**, superseded ↓ |
| 2′ | Maintainer's revision: total ≤ 1 core at the A380's full concurrency | 1.41 cores at 16 streams; 1.06 at 12; 0.87 at 10 | **MISS** at full GPU concurrency; see the capacity rule |
| 3 | Concurrency before any stream < 1.2x | 16 measured, all ≥ 1.68x; ≈ 22 extrapolated (aggregate ≈ 27x). 1×4K HDR + 6×1080p: HDR 1.93x, 1080p 2.90x | GO (recorded) |
| 4 | Per-clip first packet ≤ 250 ms | 50–52 ms (1x run); 43–74 ms across runs | GO |
| 4 | No PTS gap > 1 frame | 0 off-grid video deltas (fMP4 and TS) | GO |
| 4 | Audio seam ≤ 1 AAC frame | 0 off-grid audio deltas; boundary A/V offset ≤ 9.1 ms | GO |
| 4 | A/V drift at the end ≤ 10.7 ms | −3.13 ms (fMP4 and TS) | GO |
| 4 | SPS/PPS byte-identical across encoders | identical across all 6 encoders plus the slate, after the colour fix below | GO |
| 4 | Clips within ±1 LU of target, no loudnorm | −23.0, −23.1, −23.0, −23.1 LUFS (target −23) | GO |
| 5 | 0 hls.js stalls across the break (Chromium) | 0 stalls, 0 `BUFFER_*` errors, 0 dropped frames; first frame 285 ms | GO |
| 5 | 0 ExoPlayer rebuffers / codec re-inits (logcat) | **not measured** (moved to checkpoint 2) | open |
| 5 | expo-video canary exposes `bufferOptions` | yes: `bufferOptions.minBufferForPlayback` (Android `VideoPlayerLoadControl`) | GO |

**Decisions**
- **Segment length: 1 s.** Only 1 s meets the H.264 cold-start threshold. The break and hls.js results are
  clean at 1 s. 2 s was not run through hls.js.
- **Writer:** mediacommon for the fMP4 and MPEG-TS *writers*. Replace its TS *demux* on the hot path
  (see "Packager CPU").
- **Rate control:** QVBR, `-global_quality 22 -b:v 5M -maxrate 10M`.

## 1. Cold tune → first HLS segment

Measured from ffmpeg spawn to the first fMP4 segment renamed onto disk, one item per run, unpaced. Container
start is excluded; production spawns ffmpeg in-process. `results/cold*.csv`.

| Set | Segments | n | p50 | p95 | max |
|---|---|---|---|---|---|
| H.264, one file, 20 random seeks | 2 s | 20 | 194 | **788** | 934 |
| H.264, one file | 1 s | 20 | 142 | 328 | 462 |
| HEVC, one file | 2 s / 1 s | 20 / 20 | 179 / 130 | 238 / 197 | 248 / 232 |
| 4K HDR, one file | 2 s / 1 s | 20 / 20 | 1220 / 832 | 1291 / 896 | 1340 / 903 |
| H.264, same seeks, cold vs page-cached | 2 s | 20 / 20 | 207 / 184 | 317 / 202 | 496 / 210 |
| H.264, 40 distinct files, default probe | 1 s | 39 | 226 | **541** | 990 |
| H.264, 40 *fresh* files (V0), default probe | 1 s | 36 | 182 | 448 | 564 |
| H.264, 40 fresh files (V1), `-analyzeduration 0 -probesize 32768 -fpsprobesize 0` | 1 s | 33 | 189 | **348** | 370 |
| H.264, 40 fresh files (V2), V1 + `-f matroska` | 1 s | 38 | 178 | 371 | 980 |

Stage split, p50/p95 in ms. Open+probe+seek ends when ffmpeg prints `Input #0`.

| Variant | open + probe + seek | → first AU | → segment |
|---|---|---|---|
| V0 | 43/114 | 85/213 | 51 |
| V1 | 43/113 | 80/233 | 51 |

- **The tail is cold CIFS reads.** With the file page-cached, p95 dropped from 317 to 202 ms. Source keyframes
  are a fixed 2.00 s GOP, so accurate seeking decodes at most 2 s; that rules seek decoding out.
- Minimal probing trims the tail but not the median.
- Missing runs (V0 4, V1 7, V2 2) were the harness seeking past the end of short episodes (seek up to 960 s
  on ~600 s files). ffmpeg then exits 0 with **zero frames**. That is a production finding (below), not
  a stall. One earlier no-output run (a 45-min file, seek 945 s) did **not reproduce**; no stderr was
  captured that time.
- V1 has 33 valid samples, and V2 (same probe options) shows one 980 ms outlier. The pass is real but thin.
  Checkpoint 2 should re-run it with 100+ fresh files, together with the cue-index and read-ahead
  mitigations.

## 2. Speed and CPU per stream

`results/speed.csv`, `results/conc.csv`. 60–90 s of content per run. Cores at 1x = CPU seconds ÷ content
seconds.

| Stream | Speed (alone) | Cores at 1x (paced) | Cores at 1x (unpaced) |
|---|---|---|---|
| H.264 1080p → house format | 17.6x (19.1x in the concurrency run) | 0.081 | 0.084–0.088 |
| HEVC 1080p Main10 | 18.9x | — | 0.082 |
| 4K DV/HDR10 → scale p010 → tonemap_vaapi | 2.57x | 0.125 | 0.118 |

Split for H.264, 60 s of content: ffmpeg video+audio → TS is 3.39 s (**0.056 cores**). Video alone is 0.031
and EAC3 decode + AAC encode is 0.017. The spike packager adds 1.80 s (**0.030 cores**).

### Packager CPU (pprof, `results/packager.pprof`)
- 60% of packager CPU is go-astits `isPESComplete` plus `runtime.memmove`. mediacommon's TS reader re-scans
  the accumulating PES on every 188-byte packet, which is quadratic on large IDR frames.
- The fMP4/TS writers are a few percent.

**Recommendation:** keep mediacommon's writers. Either replace the demux with a length-driven PES
reassembler for our own fixed-shape ffmpeg output (about 150 lines), or have ffmpeg emit fMP4 with
`-output_ts_offset` and have Go only forward fragments and rewrite `mfhd`/`tfdt`. The second is
the supervisor's mitigation 4, queued for checkpoint 2.

## 3. Concurrency on the A380

N concurrent 1080p H.264 streams, unpaced, 90 s of content each, one container at 4 CPUs.
`results/conc.csv`, `conc_rss.txt`, `conc_cg.txt`; 16:35–16:40 UTC, 5.5 min total.

| Streams | Min speed | Cores/stream at 1x | Total cores at 1x |
|---|---|---|---|
| 1 | 19.06x | 0.084 | 0.08 |
| 2 | 13.58x | 0.082 | 0.16 |
| 4 | 6.80x | 0.083 | 0.33 |
| 6 | 4.54x | 0.086 | 0.52 |
| 8 | 3.41x | 0.086 | 0.69 |
| 10 | 2.73x | 0.087 | 0.87 |
| 12 | 2.27x | 0.088 | **1.06** |
| 16 | 1.68x | 0.088 | **1.41** |
| 1×4K HDR + 2 | 2.29x HDR / 8.76x | 0.121 HDR | 0.29 |
| 1×4K HDR + 4 | 2.11x / 4.57x | 0.118 | 0.46 |
| 1×4K HDR + 6 | 1.93x / 2.90x | 0.118 | 0.64 |

- **GPU-bound capacity:** aggregate throughput is about 27x, so about **22** 1080p streams at ≥ 1.2x
  (extrapolated; 16 measured).
- **Memory:** RSS per stream is about 97 MB for 1080p ffmpeg and 130–177 MB for 4K HDR ffmpeg; the packager
  is about 18 MB. 16 streams ≈ 1.9 GB of the 4 GB cgroup.
- **Throttling:** cgroup `nr_throttled` rose by 23 periods (2.6 s throttled) over the whole unpaced sweep.
  At 1x, the load is ≤ 1.4 cores of a 4-core quota.

**Capacity rule (design consequence, not a threshold change):**
`playout capacity = min(GPU capacity at ≥ 1.2x, 1.0 core ÷ measured cores per stream)`.
- Today: min(22, 1.0 ÷ 0.087) ≈ **11** 1080p streams. That is CPU-bound.
- With the demux removed (≈ 0.056): ≈ **17**.
- Further per-stream savings (estimates, unmeasured):
  - Audio passthrough when the source is already AAC-LC stereo 48 kHz: −0.017 (uniform output still holds).
  - `-threads`/`-filter_threads 1` on the VAAPI path: a small gain; the CPU side is mostly demux, audio and mux.
  - A cheaper shared audio path: most of the 0.017.

## 4. Commercial-break test

Schedule: H.264 programme (40 s) → four clips of 29.5 s (mixed fps, resolution and sample rate) → 4K HDR
programme (40 s). Run at 1x with 12 s run-ahead, 1 s segments and minimal probing, served live over
HTTP. `results/break-1x-*`, `results/break-unpaced-*`.

| Item | First video AU after spawn | A/V offset at item end | SPS/PPS | Loudness (integrated) |
|---|---|---|---|---|
| programme, H.264 | 162 ms (cold tune) | −6.6 ms | ref | −25.9 (not normalised, by design) |
| clip, 360p 23.976 (gain −1.4 dB) | 52 ms | +1.3 ms | identical | **−23.0** |
| clip, 480p 29.97 (gain +8.2 dB) | 50 ms | +9.1 ms | identical | **−23.1** |
| clip, 480p 50 (gain −1.5 dB) | 52 ms | −4.4 ms | identical | **−23.0** |
| clip, 720p 29.97, 44.1 kHz (gain −6.5 dB) | 52 ms | +3.5 ms | identical | **−23.1** |
| programme, 4K HDR | 290 ms | −3.1 ms | identical | −27.5 |

- **Timestamps:** 5934 video packets, all 3003-tick deltas; 9281 AAC frames, all 1024-sample deltas. Both
  hold in `channel.ts` and in the fMP4 concatenation, with zero decode errors from ffmpeg.
- **Gains:** static gains came from an `ebur128` pass over each clip ("ingest"). There is no loudnorm
  anywhere in the pipeline.
- **Slate fallback:** a clip whose encoder was held for 20 s missed air−2 s. The pre-encoded slate
  (encoded by the same encoder, so the same SPS) filled its 10 s slot. The output stayed gapless: 0
  off-grid deltas, end offset 7.3 ms. `results/slate-fallback.txt`.

## 5. Playback

**hls.js 1.7.1 in Chromium (Playwright)** (`playback/hlsjs-stalls.cjs`, `results/hlsjs-1s.json`):
- Configuration: `lowLatencyMode:false`, `liveSyncDuration:6`, `startFragPrefetch:true`, live over an SSH
  tunnel.
- First frame arrived **285 ms** after `loadSource`. The first manifest is held until 4 s is listable, which
  a burst reaches in about 0.3 s.
- **0 `waiting` events and 0 errors across all six items.** The only stall was at currentTime 197.95 s,
  which is the channel's end (198.0 s, no more items).
- The buffer never dropped below 5.33 s mid-run. Chromium rendered all 5934 frames and dropped none.

**ExoPlayer on the Android TV emulator: not run at this checkpoint** (budget). It moves to checkpoint 2.

**expo-video** `58.0.0-canary-20260812` exposes `player.bufferOptions.minBufferForPlayback`, implemented by
`android/.../VideoPlayerLoadControl.kt`.

## 6. Rate control (VMAF vs source)

Live-action HEVC sitcom, 20 s, h264_vaapi on the A380, same GPU graph at the source's 25 fps so frames
align. `results/ratecontrol*.txt`, `rc.sh`.

| Mode | Args | Bitrate | VMAF mean | VMAF p5 |
|---|---|---|---|---|
| CBR | `-b:v 5M` | 5.07 Mbps | 95.13 | 92.12 |
| VBR | `-b:v 5M -maxrate 10M` | 5.17 | 94.92 | 89.62 |
| **QVBR** | `-b:v 5M -maxrate 10M -global_quality 22` | **4.85** | **95.06** | **93.94** |
| QVBR | `-b:v 8M -maxrate 12M -global_quality 22` | 5.81 | 95.56 | 94.56 |
| ICQ | `-global_quality 22` | 4.34 | 94.73 | 93.46 |
| ICQ | `-global_quality 26` | 2.55 | 92.39 | 90.73 |

- **Recommendation:** QVBR q22 with a 5 Mbps target and a 10 Mbps cap. It gives the best worst-case (p5)
  per bit and bounds the peak for the TS/Emby path. VBR at the same budget has the worst p5.
- **Caveat:** this sample is clean and easy (every mode ≈ 95). A grainy, high-motion live-action sample is
  still needed to pick q. The NVENC reference on #1512 showed 5 Mbps flat costing 6.7 VMAF on hard
  content.

## Production requirements found by the spike

1. **Conform frame metadata in-graph:**
   `setparams=color_primaries=bt709:color_trc=bt709:colorspace=bt709:range=tv:chroma_location=left` after
   `fps`.
   - Without it, the SPS VUI mirrors each source's colour description and chroma siting. The 720p clip and
     the 4K HDR film produced different SPS bytes.
   - **Do not** use output `-color_*` flags. On ffmpeg 8 they join format negotiation and insert a
     software `auto_scale`, which fails the VAAPI HDR graph ("Impossible to convert between … fps and
     auto_scale").
2. `h264_vaapi -sei 0`: no per-instance identifier SEI. `-bf 0`, `-g` = segment frames; every I-frame is an
   IDR.
3. `scale_vaapi … force_original_aspect_ratio=decrease:force_divisible_by=2`, then `pad_vaapi`. Some sources
   scale to odd widths (1915).
4. **Minimal probing:** `-analyzeduration 0 -probesize 32768 -fpsprobesize 0`, with stream facts supplied
   by Loomarr. `playoutadapter.go` already has container, duration, bitrate and codecs from Emby
   `MediaStreams`.
5. **A zero-frame EOF is "not ready".** A seek at or past the end, or any encoder that exits 0 without a
   frame, must trigger the slate. The spike's readiness check waits on first-AU *or* process exit.
6. Audio: drop the first AAC frame (1024 priming samples). Time audio by sample count and truncate each
   item where its audio end passes the video end by half a frame. This keeps every boundary within
   ±512 samples (≤ 10.7 ms) with no audio gaps.
7. Every ffmpeg needs `-nostdin`. Without it, ffmpeg consumed the parent's stdin in the spike's own
   scripts.
8. HLS scratch must stay off tmpfs. At about 8 Mbps × 15 min DVR per channel, it would count against the
   4 GB cgroup.

## Writer library: mediacommon vs hand-written

| | mediacommon v2.9.5 | Hand-written |
|---|---|---|
| fMP4 init + `moof/mdat` | `fmp4.Init`/`fmp4.Part` marshal; worked first try; hls.js and ffprobe clean | ~300 lines of box writing to own |
| MPEG-TS mux (Emby/Tunarr sink) | `mpegts.Writer`; continuous, 0 decode errors | PAT/PMT/PES/PCR/CC: the error-prone part |
| TS demux of encoder output | works, but is **60% of packager CPU** (go-astits PES re-scan) | a length-driven PES reader for our fixed ffmpeg output is small and cheap |

**Decision:** mediacommon for the writers. Drop the TS demux from the hot path, either with our own
minimal PES reader or with ffmpeg → fMP4 fragment forwarding (checkpoint 2 measures both against the
current path).

## What failed or is unfinished
- The H.264 cold tune at 2 s segments misses (p95 496 ms pooled). The recorded decision is 1 s.
- CPU: the per-stream 0.06 misses. The revised total ≤ 1 core misses at full GPU concurrency (1.41 at 16).
  It holds by construction under the capacity rule above.
- **Checkpoint 2:**
  - ExoPlayer rebuffers and MediaCodec re-inits on the emulator (and the real TV box).
  - Cold-start mitigations: cue index, read-ahead, and ffmpeg-side fMP4.
  - NVENC and software-only host families.
  - A harder rate-control sample.
  - A bigger fresh-file cold-start set.
- Single-file limits: HEVC and 4K HDR cold start were each measured on one file with 20 seeks.
- 2 s segments were not run through hls.js.
