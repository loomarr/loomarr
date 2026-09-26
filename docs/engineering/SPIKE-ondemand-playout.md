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
| 2 | CPU ≤ 0.06 cores per stream (original) | TS demux path: 0.081–0.088 (H.264). **fMP4 forward path (cp2): H.264 0.054, HEVC 0.065, 4K HDR 0.117** | GO for H.264; HEVC/HDR over, covered by the capacity rule |
| 2′ | Maintainer’s revision: total ≤ 1 core at the A380’s full concurrency | TS path: 1.41 at 16. **fMP4 path (cp2): 0.71 at 12, 0.93 at 16, 1.18 at 20** (GPU limit ≈ 22) | **GO at 16**; capacity min(22, 17) = 17 |
| 3 | Concurrency before any stream < 1.2x | 16 measured, all ≥ 1.68x; ≈ 22 extrapolated (aggregate ≈ 27x). 1×4K HDR + 6×1080p: HDR 1.93x, 1080p 2.90x | GO (recorded) |
| 4 | Per-clip first packet ≤ 250 ms | 50–52 ms (1x run); 43–74 ms across runs | GO |
| 4 | No PTS gap > 1 frame | 0 off-grid video deltas (fMP4 and TS) | GO |
| 4 | Audio seam ≤ 1 AAC frame | 0 off-grid audio deltas; boundary A/V offset ≤ 9.1 ms | GO |
| 4 | A/V drift at the end ≤ 10.7 ms | −3.13 ms (fMP4 and TS) | GO |
| 4 | SPS/PPS byte-identical across encoders | identical across all 6 encoders plus the slate, after the colour fix below | GO |
| 4 | Clips within ±1 LU of target, no loudnorm | −23.0, −23.1, −23.0, −23.1 LUFS (target −23) | GO |
| 5 | 0 hls.js stalls across the break (Chromium) | 0 stalls, 0 `BUFFER_*` errors, 0 dropped frames; first frame 285 ms | GO |
| 5 | 0 ExoPlayer rebuffers / codec re-inits (logcat) | Android 11 emulator, media3 1.9.1, `bufferForPlaybackMs` 1000, live break (fMP4 path), 2 full runs: **0 rebuffers, 1 video + 1 audio decoder init, 0 format changes** across 5 boundaries; first frame 1.32–1.66 s | GO (emulator; real TV box open) |
| 5 | expo-video canary exposes `bufferOptions` | yes: `bufferOptions.minBufferForPlayback` (Android `VideoPlayerLoadControl`) | GO |

**Decisions**
- **Segment length: 1 s.** Only 1 s meets the H.264 cold-start threshold. The break and hls.js results are
  clean at 1 s. 2 s was not run through hls.js.
- **Writer (updated at cp2):** ffmpeg muxes fMP4 and Go forwards fragments, patching `mfhd`/`tfdt` in place. mediacommon `fmp4.Parts` is used only for each item's first fragment, and `mpegts.Writer` only for the TS sink.
- **Rate control (revised at cp2 on a hard sample):** QVBR, `-global_quality 22 -b:v 8M -maxrate 12M`.

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


### Checkpoint 2: cold start at scale (fMP4 path)

`results/p2b.sh`, `p2b_cold.csv`, `p2b2.sh`, `p2b_noseg.csv`, `p2b_warm.csv`; 17:00–17:10 UTC.
- **Files:** 110 files this spike never touched. Checkpoint 1's 160-file lists were regenerated with the same
  seeds and excluded.
  - 50 H.264 1080p episodes;
  - 40 HEVC 1080p episodes;
  - 20 4K HDR/DV films.
- **Settings:** 1 s segments, minimal probe, one random seek each.
- **Stages** (ms from spawn): *probe* is ffmpeg's `Input #0`; *open* runs from there to the moov write (seek,
  read, first decode, encoder init); *GOP* runs from there to the first fragment.

| Class | n | p50 | p95 | max | mean probe | mean open | mean GOP |
|---|---|---|---|---|---|---|---|
| H.264 1080p | 43 | 197 | **510** | 556 | 57 | 105 | 64 |
| HEVC 1080p | 39 | 244 | **360** | 395 | 72 | 106 | — |
| 4K HDR/DV | 20 | 1121 | **1242** | 2227 | 92 | 197 | — |

- **No segment on 8 of 110 files: all were seeks past EOF.** The seeks were 704–945 s on files of 615–688 s
  (`p2b_noseg.csv`). This is the zero-frame case that must go to the slate (requirement 5). It is not a
  cold-start failure.
- **The H.264 tail is I/O, not the packager.** In the four slowest H.264 files, *open* is 215–360 ms and GOP
  is 64–93 ms. The same 50 files re-run **warm**, alternating paths, 86 runs each:
  - TS path: p50 171, p95 360;
  - **fMP4 path: p50 131, p95 258.**
- **Pooled H.264** (checkpoint 1's 33 minimal-probe files plus these 43): **n=76, p50 189, p90 341, p95 377,
  max 556**. That passes ≤ 400 ms, but this set alone (p95 510) does not. The tail is the CIFS
  seek/read, which the cue index (c) and read-ahead (d) target.
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


### Checkpoint 2: ffmpeg muxes fMP4, Go forwards fragments (`-mode fmp4`)

`fmp4mode.go`, `results/p2a.sh`, `p2a_speed.csv`, `p2a_conc.csv`, `p2a_conc_cg.txt`, `p2a_packager.pprof`;
17:00 UTC.
- **Setup:**
  - ffmpeg writes `-f mp4 -movflags empty_moov+default_base_moof+frag_keyframe`. That gives one fragment per
    GOP, which is one 1 s segment.
  - ffmpeg also gets `-video_track_timescale 90000`, `-output_ts_offset <slot start>`, and exact
    `-frames:v N -frames:a M+1` precomputed from the schedule.
  - Go reads top-level boxes and patches `mfhd.sequence_number` and each `tfdt` in place.
- **Where mediacommon is still used:** `fmp4.Parts` unmarshal/marshal, only for each item's *first*
  fragment. That fragment needs two fixes:
  - drop the AAC priming frame;
  - normalise two video durations that ffmpeg's start shift leaves at 4923/1083 ticks (their sum is exact).
- **Continuity:** a two-item stitched run gave 290/290 video deltas of 3003 and 454/454 audio deltas of
  1024, 0 decode errors, and exact frame and audio-frame counts per item.

| Stream, 60 s content | TS demux path (cores at 1x) | fMP4 forward path | Saving |
|---|---|---|---|
| H.264 1080p | 0.080 | **0.054** | −33% |
| HEVC 1080p Main10 | 0.084 | **0.065** | −23% |
| 4K HDR (tone-map) | 0.123 | **0.117** | −5% |

The packager itself is now 100 ms of CPU for 60 s of content (**0.0017 cores**, down from 0.030). Half of that
is pipe syscalls. What remains is ffmpeg: demux, EAC3 decode, AAC encode and mux.

Concurrency, fMP4 path, 1080p H.264, unpaced, 90 s content each (cgroup `usage_usec` delta):

| Streams | Min speed | Cores/stream at 1x | Total cores at 1x |
|---|---|---|---|
| 12 | 2.27x | 0.059 | **0.71** |
| 16 | 1.70x | 0.059 | **0.93** |
| 20 | 1.36x | 0.059 | **1.18** |

Speeds match the TS path, so the GPU is the limit at 20+ (≈ 22 at ≥ 1.2x). CPU-bound capacity is
1.0 ÷ 0.059 ≈ **17**, so capacity is min(22, 17) = **17 1080p streams**. The maintainer's "total ≤ 1 core at
full concurrency" holds at 16 (0.93).
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


### ExoPlayer on the Android TV emulator (cp2)

`playback/exo/`, `results/p2e.sh`, `p2e_logcat.txt`, `p2e_events.jsonl`.
- **Harness:** a minimal media3 1.9.1 app (the ExoPlayer that expo-video wraps).
  - `DefaultLoadControl` sets `bufferForPlaybackMs` = 1000, which is what expo-video's
    `minBufferForPlayback` maps to.
  - An `AnalyticsListener` logs to logcat: first frame, state changes, decoder init/release, format changes
    and dropped frames.
- **Setup:** Android 11 x86_64 emulator (`loomarr-tv-x64`). The break schedule was served live from the
  household host (paced 1x, fMP4 path, 1 s segments).
- **No make target:** the repo has no `make android-load`. The APK was built directly with Gradle 8.11.1 and
  JDK 21.

| | Run 1 | Run 2 (exact-count fix) |
|---|---|---|
| Rebuffers during content (5 boundaries, 198 s) | 0 | 0 |
| Video decoder inits / releases | 1 / 1 (at exit) | 1 / 1 (at exit) |
| Audio decoder inits | 1 | 1 |
| `onVideoInputFormatChanged` after start | 0 | 0 |
| Dropped frames | 2, at the end-of-schedule edge only | 4, same place |

- **Why no re-inits:** every item shares the channel `init.mp4`, so ExoPlayer never sees a format change,
  even going from a 360p 23.976 clip to a tone-mapped 4K HDR film.
- **End of schedule:** the one BUFFERING event came after the last segment (pos 197.6 s of 198). The spike
  stops listing there without `#EXT-X-ENDLIST`, so it is not a rebuffer.
- **First frame:** measured from app launch with the server already producing. Three trials: **1.32, 1.34,
  1.66 s**. In one earlier run the app started before the server listened, and first frame was 7.4 s
  (ExoPlayer's load-error retry). The production client should not start the player before the stream
  answers.
- **Before the exact-count fix (requirement 10):** run 1 had item frame counts off by one. ExoPlayer still
  played through the resulting 1-frame timeline gaps.
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



### Checkpoint 2: a hard live-action sample

`results/rc2.sh`, `rc2.csv`; 17:45 UTC.
- **Sample:** a grain-heavy 35 mm film remux, AVC at about 34 Mbps. It is the top-bitrate tier of the
  library's SDR 1080p remuxes, with nature/fast-motion and dark sequences.
- **Windows:** three 15 s windows at 25/50/80% of runtime, each cut once to a lossless FFV1 reference.
  Every mode encodes from that reference (software decode, `hwupload`, `h264_vaapi`), and VMAF compares
  against it.
- **Method fix:** frames are paired **by index** (`setpts=N/FRAME_RATE/TB`). Timestamp pairing slipped a
  frame mid-window, because the MKV reference has 1 ms timestamps and the MP4 has 1/24000. That produced
  VMAF min 0 and scores that did not change with bitrate.
- **Alignment guard:** every row records Y-PSNR at offset 0 and at +1. Offset 0 wins by 9–13 dB in all
  18 rows.

| Mode | Mbps (3 windows) | VMAF mean: grain / mid / late | **p5**: grain / mid / late |
|---|---|---|---|
| CBR 5M | 5.03–5.09 | 80.0 / 96.5 / 92.7 | 55.2 / 92.3 / 89.7 |
| VBR 5M/10M | 5.12–5.24 | 80.7 / 96.4 / 92.5 | 60.9 / 92.2 / 88.2 |
| QVBR q22 5M/10M | 5.11–5.18 | 79.1 / 96.5 / 92.5 | 58.6 / 92.4 / 89.4 |
| CBR 8M | 7.96–8.09 | 86.8 / 97.6 / 93.6 | 64.4 / 93.6 / 91.0 |
| VBR 8M/12M | 8.09–8.17 | 87.7 / 97.4 / 93.6 | 69.0 / 92.6 / 91.0 |
| **QVBR q22 8M/12M** | 7.84–8.29 | 87.1 / 97.5 / 93.5 | **72.0** / **94.1** / 90.9 |

**Recommendation: QVBR, `-global_quality 22 -b:v 8M -maxrate 12M`.** This revises checkpoint 1's 5M/10M,
which came from an easy sample.
- **The grain window is bitrate-bound.** At 5 Mbps every mode sits at VMAF ≈ 80 with p5 55–61. The mode
  barely matters at that rate.
- **8 Mbps lifts the grain window by about 7 points** of VMAF mean.
- **QVBR gives the best worst-case p5** there: 72, against 69 for VBR and 64 for CBR.
- **QVBR costs little on easy content.** Its quality target keeps easy content well under the cap:
  checkpoint 1's easy sample averaged **5.81 Mbps** at 8M/12M.
- CBR spends 8 Mbps on everything, which is the worst for mixed channels.
## 7. NVENC host family (dev machine, cp2)

`results/p2g.sh`, `p2g_start.csv`, `p2g_speed.csv`, `p2g_conc.csv`.
- **Hardware and software:** RTX 3080 Ti (GeForce), driver 615, **ffmpeg n9.0.2** (not the production n8.1
  image), 24 threads, no CPU cap.
- **Media:** 120 s stream-copy excerpts of the three samples on local NVMe, so start times are warm-disk,
  not CIFS.
- **Graph:** `-hwaccel cuda -hwaccel_output_format cuda`, `scale_cuda`, `fps`, `setparams`, then
  `h264_nvenc -preset p4 -tune ll -bf 0 -g 30 -forced-idr 1 -strict_gop 1 -no-scenecut 1`, VBR cq 22,
  5M/10M.
- **HDR tone-map, what works:** `libplacebo` with ffmpeg's Vulkan hwaccel device
  (`-init_hw_device vulkan`, `-hwaccel vulkan`) fails with "Error initializing filters" on this driver. The
  working graph is CUDA decode, then `scale_cuda` to 1080p p010, `hwdownload`, and `libplacebo` (own Vulkan
  device, bt.2390), then NVENC. A screenshot check showed correct tone-mapping.
- **HDR, the CPU fallback:** the same downscale followed by `zscale`/`tonemap=hable` at 1080p costs
  **2.9 cores** per stream at 3.1x. Do not use it.

| | H.264 1080p | HEVC 1080p | 4K HDR → 1080p |
|---|---|---|---|
| First segment p50 / p95 (10 seeks) | 319 / 504 ms | 307 / 362 ms | **2135 / 2237 ms** |
| Mean open (probe → moov) | 266 ms | 240 ms | 2016 ms (libplacebo Vulkan init) |
| Speed, 60 s alone | 13.5x | 14.6x | 8.6x |
| Cores per stream at 1x | **0.034** | 0.034 | 0.165 |

| Concurrent 1080p H.264 streams | Min speed | Cores/stream | Total cores at 1x |
|---|---|---|---|
| 4 | 3.62x | 0.037 | 0.15 |
| 8 | 1.82x | 0.041 | 0.33 |
| 12 | **1.21x** | 0.044 | 0.53 |
| 16 | 1.20x, and **4 of 16 failed `OpenEncodeSession`** | — | — |

- **Capacity on GeForce: 12 streams.** The GeForce NVENC session limit fails the 13th encoder outright.
  Both 1.2x and the session cap land at about 12. Pro/datacenter cards have no session cap. The capacity
  rule needs a third term: `min(GPU throughput, CPU, NVENC sessions)`.
- **Start:** the very first NVENC run after the GPU had been idle took **4.05 s** to its first segment; the
  next five took 324–393 ms. Persistence mode is off on this machine. That the idle wake is the cause is
  a theory, not reproduced.
- **4K HDR start misses ≤ 1.8 s** (2.1–2.2 s). The cost is libplacebo creating its own Vulkan device on
  every spawn.
  - A pre-warmed device would remove it, but ffmpeg only shares a device inside one process.
  - Fallback: tone-map in CUDA (`tonemap_cuda` is not in this build) or OpenCL (`tonemap_opencl`, untested).
- **ffmpeg 9:** minimal probing (`probesize 32768`) cannot open TrueHD tracks ("Could not find codec
  parameters"). n8.1 opened all 20 4K HDR files in 2b. This is one more ffmpeg-9 difference, alongside the
  known concat break.
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
9. **Packager shape (cp2):**
   - ffmpeg muxes fMP4 with `empty_moov+default_base_moof+frag_keyframe`, `-video_track_timescale 90000`
     and `-output_ts_offset`.
   - Go forwards each `moof+mdat` and patches `mfhd`/`tfdt` in place, stamping the channel timeline from
     its own counters, never from ffmpeg's.
   - The first item's moov becomes the channel `init.mp4`. Every later item's `stsd` must match it byte
     for byte; the spike logs `stsd_diff`, and none occurred.
10. **ffmpeg's `-frames:v`/`-frames:a` are not exact.** The measured error was 1198 of 1199 video frames and
    ±2 AAC frames. Over-ask by a margin, and have Go trim the fragment where the slot fills. Otherwise the
    timeline gaps by a frame per item.
11. **Loomarr-owned metadata record.** The stream facts that minimal probing needs, plus the keyframe/cue
    index (c), belong on `inventory_source_measurements` (migration 00091). They are read once per source
    revision, not per tune.

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
- **H.264 cold start is marginal.** The pooled p95 of 377 ms (n=76) passes, but checkpoint 2's fresh set
  alone is 510 ms. The tail is CIFS seek/read.
- **(c) Keyframe/cue index: not measured (budget).** The measurement to run next is the split of *open*
  (probe → moov, 105 ms mean, 215–360 ms in the tail) into:
  - the MKV Cues lookup;
  - the data read at the target cluster.

  Then compare a direct-offset start (`-ss` replaced by a byte offset from a stored index). The index
  belongs on `inventory_source_measurements` (00091), read once per source revision.
- **(d) Read-ahead: not measured (budget).** Prefetching 4–8 MB at the airing's seek offset is the
  cheapest test of how much of that tail a warm start removes. The warm/cold gap on the same 50 files is
  the upper bound: fMP4 path p95 258 ms warm against 510 ms cold.
- **4K HDR on NVENC** misses ≤ 1.8 s (2.1–2.2 s). The cost is libplacebo's per-process Vulkan init.
  `tonemap_opencl` is untested.
- **The real TV box** (Shield, Android 11) was not measured. Only the emulator was.
- **Checkpoint 1's rate-control numbers** (easy sample) used timestamp pairing. The p5 values of 89–94
  show no slip, but they were not re-run with index pairing.
- ffmpeg n9 (dev machine) cannot minimal-probe TrueHD. Production stays on n8.1 until this and the
  concat break are addressed.
