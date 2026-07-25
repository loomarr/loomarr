# Playout prior art — lessons from `mantonx/viewra`

**Read before building V6.** The maintainer has already built and debugged a Go transcoding
pipeline (`github.com/mantonx/viewra`, public). This records what transfers, what does not,
and the bugs it paid for — so V6 does not rediscover them.

**Reviewed at viewra `pushedAt 2026-04-08`, against Loomarr `ffe1b7c`.**

---

## 1. The headline: do NOT copy the architecture

Viewra's transcode session is `mediaID → seekable file → ffmpeg -ss`. Its whole seek model
is *kill ffmpeg and restart at a new offset*, wiping the output directory.

A live channel has **no mediaID, no seekable file, must never restart, and must serve many
viewers at once**. Three specific things would actively break us:

| Viewra behaviour | Why it is wrong for playout |
| --- | --- |
| `stopOtherMediaSessions` + `StopOtherQualitySessions` on every new session (`session/manager.go:164-171`) — one viewer starting a stream **kills** other sessions | A channel must be **reference-counted** (viewers++/--, tear down on zero after a grace period), and channels must coexist. This is the single biggest structural difference. |
| `-hls_playlist_type event` (`hls/hls.go:200-222`) — append-only, never trims | The playlist grows unbounded and players buffer from segment 0. Live needs `-hls_list_size` + `delete_segments+append_list+omit_endlist+program_date_time`. |
| Seek = restart ffmpeg + `os.RemoveAll` the output dir (`manager.go:227-247`) | Meaningless for live, and `-start_number` derived from `StartPosition / SegmentDuration` assumes a seekable file. **Media sequence must advance monotonically forever**, never from wall-clock position. |

**What IS worth reusing:** the `hls.Builder` arg-builder shape (§2 below), the
segment-availability `sync.Cond` + fsnotify pattern, the content-type map, and the
non-fatal-ffmpeg-warning allowlist.

## 2. Command construction — the shape to copy

A fluent builder over `[]string`, never a shelled-out string. Per-encoder divergence is
contained by **one switch dispatching to per-encoder methods**, each calling shared
helpers (`hls/hardware.go:6-25`):

```go
switch hwAccel {
case AccelNVENC: b.addNVENCEncoding(p, codec)
case AccelQSV:   b.addQSVEncoding(p, codec)
case AccelVAAPI: b.addVAAPIEncoding(p, codec)
default:         b.AddVideoEncoding() // software
}
```

The only *second* switch is the filter chain (`hls/filters.go:132-158`) — correct, because
that is where the real divergence lives (`scale_vaapi,pad_vaapi` vs
`scale_cuda,hwdownload,pad,hwupload_cuda` vs `scale_qsv` vs plain `scale,pad`).

## 3. Hardware detection — three layers, and the trap

Layered **capability listing → device presence → real trial encode**
(`transcoding/config/config.go:212-271`, `ffmpeg/hls/test_utils.go:39-73`):

1. `HARDWARE_ACCEL` env escape hatch first.
2. Device signal (`nvidia-smi` on PATH; `/dev/dri/renderD128` stat) **paired with** an
   ffmpeg-build signal (`-encoders` plus `-h encoder=<name>`). Neither is trusted alone.
3. A synthetic trial encode: `-f lavfi -i testsrc=... -frames:v 1 -c:v <enc> -f null -`.

**⚠ The trap to avoid:** `isHardwareError` classifies failures by **substring match** on
`{"cuda","nvenc","qsv","vaapi","hardware","gpu","device","driver","cannot open","not
found"}` (`hls/fallback.go:96-127`). `"not found"` and `"cannot open"` also match a missing
*input file*, so an unrelated failure can permanently demote the box to software. Classify
on **exit code + specific init-failure lines** instead.

**⚠ Also:** the fallback writes `m.config.HardwareAccel = nextAccel` (`fallback.go:90`) — a
global, non-resettable demotion for every future session. Playout wants **per-channel**
state with time-based recovery.

**⚠ And:** viewra has **three duplicate detectors** (`config.go:215`,
`ffmpeg/process/process.go:239`, `system/profiler.go:484`) that disagree — one skips the
`/dev/dri` stat despite a comment claiming otherwise. One detector, one place.

## 4. Process lifecycle — two real bugs to not repeat

- **Progress**: `-progress pipe:2` is available but the live path **scrapes stderr** with
  raw 4096-byte `Read`s looking for `frame=` (`session/session.go:311-403`). Chunked reads
  can split a token across a buffer boundary. **Use `-progress pipe:1` and parse k=v.**
- **⚠ The watchdog kills the process, not the group** (`session/watchdog.go:93`) while
  start uses `Setpgid` + `systemd-run` (`session/ffmpeg.go:158-207`). The two paths
  disagree, so children can orphan. Kill the **process group**.
- Stop is SIGINT → 100ms → `Kill()` → synchronous `Wait()` guarded by `sync.Once`. The
  `Once` exists because the monitoring goroutine may already have called `Wait` ("no child
  processes"); the *synchronous* wait is deliberate so the output dir is safe to delete.
- **⚠ No client-disconnect detection at all** — nothing watches `Request.Context().Done()`;
  idle GC sweeps on a timer. For VOD a leaked ffmpeg dies at EOF. **A live encoder never
  exits — leak one and it burns a core forever.** Disconnect handling is mandatory here,
  not optional, and `LastAccessed` polling is too coarse when Emby holds one long-lived
  connection and never re-requests.
- **⚠ `LastAccessed` is written without a lock** from HTTP handlers (`session.go:461`)
  while the cleanup goroutine iterates (`cleanup.go:121`) — a data race. Use `atomic.Int64`.

## 5. Serving — and the Emby tuner requirement we must get right

Viewra is **VOD only**: no `EXT-X-DISCONTINUITY`, no `PROGRAM-DATE-TIME`, no rolling
window. Manifests are split: **media playlists are 100% ffmpeg's job**, the master playlist
is hand-built in Go (`application/transcode/serve_master_playlist.go:435-468`). Copy that
split — hand-writing EXTINF/sequence numbers is how you get player stalls.

**⚠ Emby/Jellyfin registered as an M3U tuner want a continuous MPEG-TS byte stream, not an
m3u8** — they do their own remux/segmenting. That endpoint must:

- send **no `Content-Length`**, no `Range`/206 handling,
- `Content-Type: video/mp2t`,
- **`Flush()` on an interval**.

Viewra's `io.Copy` + fixed `Content-Length` VOD path (`api/handlers/stream.go:80,90`) is
unusable as-is for this. This is why #11 (both transports) matters: **the tuner path is the
MPEG-TS one**; HLS is for browser/app clients.

**⚠ `delete_segments` races the HTTP handler** — a client asking for a segment you just
unlinked 404s. Keep `-hls_delete_threshold` generous, and keep viewra's posture of *not*
logging segment 404s as warnings (`api/middleware/logger.go:53`: "HLS segment 404s are
normal - player probes for next segment").

## 6. Auth — the finding that changes our design

Viewra puts a **JWT in `?token=`** because `<video src>` cannot set headers
(`api/middleware/auth.go:164-179`). Two consequences for us:

- **⚠ Segment URIs inside an ffmpeg-generated playlist are RELATIVE and carry no token.**
  They only work because the player replays the same query string. If Emby fetches our
  playlist and then segments, we must **rewrite segment URIs to carry the token**.
- **⚠ A short-lived token expires mid-stream.** A TV holds a channel for hours; the next
  segment 401s. `playout_token` (V4) is deliberately long-lived and channel-scoped rather
  than a session JWT — this is the evidence for that choice.

## 7. ffmpeg patches — budget for it, but probably not for us

Viewra ships **nine patches against ffmpeg itself** (`tools/ffmpeg-viewra/patches/`), based
on jellyfin-ffmpeg. Worth knowing they exist, and worth knowing **most do not apply to us**:

| Patch | Applies to V6? |
| --- | --- |
| `0001-segment-muxer-track-start-pts` — seek makes segment boundaries drift, causing A/V drift | **No.** Seek-specific; live never seeks. |
| `0007-segment-muxer-report-keyframe-start-offset` | **No.** Same reason. |
| `0003-fix-safari-hls-empty-sdtp` — Safari crashes on empty SDTP in libx265 fMP4 | Only if we ship fMP4/HEVC to Safari. Not the tuner path. |
| `0004-fix-nvdec-exceed-32-surfaces` — "Cannot allocate more than 32 surfaces" on 4K | Possible at high channel counts on NVIDIA. **Watch for it.** |
| `0005-add-hdr-metadata-for-nvenc-hevc`, `0006-pass-dovi-sidedata`, `0008/0009 cuda tonemap` | Unlikely — playout targets SDR H.264 for compatibility. |

**We are in better shape than viewra here**: we already vendor a **pinned BtbN GPL build**
(`Dockerfile:108-115`), whereas viewra resolves ffmpeg from `VIEWRA_FFMPEG_PATH` or `PATH`.
Pinning from day one is the lesson, and V3 already did it.

## 8. Consequences for V6's design

1. **New `Channel` playout abstraction**, not a port of `TranscodeSession`. Reference-counted
   viewers, no restart-on-anything, many channels concurrently.
2. **One hardware detector**, layered as in §3, classifying failures on exit code — not
   substrings — with per-channel demotion and recovery.
3. **`-progress pipe:1`, k=v parsing**, and kill the **process group**.
4. **Client disconnect is a hard requirement** (`Request.Context().Done()` + refcount), not
   an idle sweep.
5. **The tuner endpoint is continuous MPEG-TS**: no Content-Length, no ranges, periodic
   flush. HLS is the secondary, browser-facing transport.
6. **Rewrite segment URIs to carry `playout_token`**; rely on it being long-lived.
7. **Let ffmpeg own the media playlist**; hand-write only what ffmpeg cannot know.
8. **Program boundaries need `EXT-X-DISCONTINUITY`** (and `PROGRAM-DATE-TIME` for
   XMLTV/EPG alignment) — viewra has neither, so there is no prior art here. Prefer one
   long-running ffmpeg over a concat source with normalized encode params to
   restarting per program.
9. **Segment duration**: viewra uses 1s for VOD startup latency. For N channels × M viewers
   that multiplies playlist churn; **2–4s is the saner live default**, and TARGETDURATION
   must be the ceiling of actual durations or players error out.
