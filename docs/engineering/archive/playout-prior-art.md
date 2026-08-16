# Playout prior art — Tunarr, ErsatzTV, viewra

> **ARCHIVED — this is a record, not an instruction.** V6 shipped; internal playout is the
> default backend. The design this research produced is [`docs/design.md`](../../design.md) §9.1.

**Read before building V6** *(as originally written)*. Three working systems were mined rather than guessing at an
encoder. Two of them solve *exactly* our problem — and **they disagree about the central
mechanism**, which is the most useful thing in this document.

Reviewed 2026-07-25 against Loomarr `ffe1b7c`:

| System | What it is | Why it matters |
| --- | --- | --- |
| **Tunarr** (`chrisbenincasa/tunarr`, TS/Node) | The system Loomarr currently delegates to | **The reference implementation.** Emby/Jellyfin already accept its output, so its wire contract is the one we must reproduce. |
| **ErsatzTV** (`ErsatzTV/legacy`, C#) | Independent solution to the same problem | A second, differently-shaped answer — and a dedicated `ErsatzTV.FFmpeg` project with tests. |
| **viewra** (`mantonx/viewra`, Go) | The maintainer's own VOD transcoder | Same language, but VOD; mostly a source of *warnings*. See [`playout-prior-art-viewra.md`](playout-prior-art-viewra.md). |

---

## 1. THE decision: continuous playout, two valid mechanisms

A channel plays program after program forever. Neither system restarts "the stream" per
program, but they get there in opposite ways.

### Tunarr — one long-lived ffmpeg over an HTTP ffconcat loop

`server/src/api/videoApi.ts:199-222` serves a **two-line** ffconcat playlist:

```
ffconcat version 1.0
file 'http://localhost/stream?channel=…&token=…'
file 'http://localhost/stream?channel=…&token=…'
```

Both lines are *the same URL*. Combined with `-stream_loop -1` and (from
`ConcatHttpReconnectOptions.ts`) `-reconnect 1 -reconnect_at_eof 1`, the concat demuxer
loops forever; each time it opens `/stream`, that handler asks "what is airing **right
now**?" and spawns a fresh per-program ffmpeg that streams finite MPEG-TS until the program
ends (EOF). The demuxer advances, loops, re-requests, gets the next program.

The outer process is `-c copy` (`CopyAllEncoder`) — **it never re-encodes.** Per-program
children normalise to identical resolution/framerate/codec, which is what makes `-c copy`
concatenation legal.

> **There is no splicing code.** The concat demuxer's EOF-and-advance *is* the program
> boundary. That is the whole trick, and it is remarkably little machinery.

### ErsatzTV — one ffmpeg per program, appending to a shared playlist

`ErsatzTV.FFmpeg/OutputFormat/OutputFormatHls.cs:109-134`:

```csharp
if (_isFirstTranscode)
    result.AddRange(["-hls_flags", $"{pdt}append_list{independentSegments}", _playlistPath]);
else
    result.AddRange(["-hls_flags", $"{pdt}append_list+discont_start{independentSegments}", _playlistPath]);
```

`append_list` = each new process appends to the existing `live.m3u8`; `discont_start` =
emit `#EXT-X-DISCONTINUITY` at the head of its output. Timestamps are carried across
processes explicitly: `GetPtsOffset()` (`HlsSessionWorker.cs:807-843`) reads the last
written segment's PTS and feeds it to the next process as `-output_ts_offset` (plus
`-copyts`). **The next ffmpeg is told to start where the previous one stopped.**

A work-ahead loop keeps ~60s of segments transcoded in advance
(`HlsSessionWorker.cs:236-265`), throttling to realtime only once ≥30s ahead:

```csharp
bool realtime = transcodedBuffer >= TimeSpan.FromSeconds(30);
```

### The trade-off, stated honestly

| | Tunarr (concat loop) | ErsatzTV (per-program + append_list) |
| --- | --- | --- |
| Machinery | Very little — no splice code, no PTS bookkeeping | Substantial — PTS carry-forward, work-ahead buffer, own playlist rewriter, discontinuity map |
| Failure blast radius | One long-lived process; if the outer ffmpeg dies the channel dies | Per-item; a failed program is contained |
| Error recovery | Restart the channel | **Transcodes the ffmpeg error text as a video card filling the same slot** (`HlsSessionWorker.cs:586-638`) so the timeline doesn't drift |
| Work-ahead | None — strictly realtime | ~60s buffer absorbs a slow program |
| Correctness burden | On the *children* (must normalise identically for `-c copy`) | On the *parent* (must get PTS + discontinuity right) |

**Recommendation for V6: start with Tunarr's concat loop.** Three reasons — it is the
contract Emby already accepts from Tunarr today, it is dramatically less code for a first
frame, and V6's gate is *"a test card playing"*, not *"survives a bad program at 3am"*.
ErsatzTV's error card and work-ahead buffer are the right **second** iteration, and this
document is where to find them when the first 3am failure happens.

## 2. What Emby/Jellyfin actually want (the contract we must match)

**Both systems force MPEG-TS for tuner clients.** ErsatzTV is explicit —
`IptvController.cs:70-79`: *"don't redirect to the correct channel mode for HDHR clients;
always use TS"*.

Tunarr's `/stream/channels/:id` is a **302 redirect** by channel config, and the tuner mode
lands on `.ts`, served as (`streamApi.ts:225`):

```ts
return res.header('Content-Type', 'video/mp2t').send(piped);
```

No `Content-Length`. No range handling. Chunked, streamed forever.

⚠ **The isolation that is not optional** (`streamApi.ts:181-189`):

```ts
// We have to create an intermediate stream between the raw one
// and the response so that nothing messes up the backing raw stream
// when the request closes.
const piped = session.rawStream!.pipe(new PassThrough({ allowHalfOpen: false }));
```

Each viewer gets its own tap off the **shared** session stream. In Go this is the same
requirement: one encoder, N `io.Writer` taps, and one viewer disconnecting must not disturb
the shared source.

**And the reassuring finding: Tunarr has ZERO Emby/Jellyfin-specific downstream code.** It
serves standards-clean `video/mp2t` and standards-clean XMLTV. Emby appears only as an
*upstream* media source. So there is no vendor quirk to reverse-engineer — get the standards
right and the tuner works.

## 3. The M3U tuner file — exact shape

`server/src/services/M3UService.ts`:

```
#EXTM3U url-tvg="{{host}}/api/xmltv.xml" x-tvg-url="{{host}}/api/xmltv.xml"
#EXTINF:-1 tvg-id="C1.50.tunarr.com" channel-id="C1.50.tunarr.com" CUID="C1.50.tunarr.com" tvg-chno="1" tvg-name="My Channel" tvg-logo="{{host}}/images/tunarr.png" group-title="Tunarr",My Channel
{{host}}/stream/channels/<uuid>?streamMode=<mode>
```

- `tvg-id` / `channel-id` / `CUID` all carry the **same synthetic id**, from
  `util/channels.ts`: `` `C${channelNum}.${checksum}.tunarr.com` ``, with the comment
  *"This helps differentiate channels for some players whose guides can get confused."*
  **That same string is the XMLTV `<channel id>` — it is the correlation key.**
- Stealth channels are omitted; an empty install emits one dummy entry pointing at setup.
- `{{host}}` is a stored placeholder substituted per request — worth copying, since it means
  the cached file is host-agnostic. Compare our `server.public_url` (V4).

XMLTV (`services/XmlTvWriter.ts`) emits **three** `<display-name>` values in order —
`"{number} {name}"`, `"{number}"`, `"{name}"` — plus `<icon>`, `<rating system="MPAA">`,
`previouslyShown` unconditionally, and `<episode-num system="xmltv_ns">` with a carve-out:
*"Simply drop the xmltv notation system for specials (season == 0)"*.

## 4. ffmpeg argument construction — ErsatzTV's design is the one to learn from

`ErsatzTV.FFmpeg/IPipelineStep.cs` — every option is an object contributing args to
**positional buckets**, plus a state transform:

```csharp
public interface IPipelineStep {
    string[] GlobalOptions { get; }
    string[] FilterOptions { get; }
    string[] OutputOptions { get; }
    string[] InputOptions(InputFile inputFile);
    FrameState NextState(FrameState currentState);   // ← the key idea
}
```

`NextState` threads a declared **frame state** (size, pixel format, framerate, and crucially
whether frames are in CPU or GPU memory) through the steps, so the builder *knows* whether a
scale/hwupload/hwdownload is still required rather than hardcoding a chain per encoder.

That solves the problem viewra's flat switch only contains. Hardware variants are then
per-accel `PipelineBuilder` subclasses over a shared base, and `PipelineBuilderFactory`
**gracefully degrades** to software (`when capabilities is not NoHardwareCapabilities`),
probing the actual ffmpeg binary rather than trusting device files alone.

## 5. Lessons worth stealing verbatim

**Sessions & lifetime**
- Tunarr: one encoder per channel, N viewers tapping it (`DirectStreamSession`: *"all
  participants share the same underlying Readable stream"*). Teardown on zero connections
  after a **5s grace period** that aborts if someone reconnects (*"Aborting shutdown. Got new
  connections in grace period"*), with the timer `.unref()`'d.
- ⚠ `SessionManager.ts:332-334`: *"Verify the session in the map is the same instance. A
  delayed cleanup timer from a previous session must not delete a replacement session created
  at the same key."* A real race we will hit.
- ⚠ `SessionManager.ts:462`: *"No heartbeats on a raw stream, just pause at 'now'."* HLS
  viewers heartbeat via segment requests; a raw TS viewer holds one connection and never
  re-requests — so **disconnect detection cannot rely on request activity** for the tuner path.

**Playlists (if/when we do HLS)**
- ErsatzTV lets ffmpeg write an **unbounded** playlist (`-hls_list_size 0`) and **rewrites a
  windowed one per request** (`HlsPlaylistFilter.cs`), re-emitting `TARGETDURATION` parsed
  from ffmpeg's own output rather than hardcoding it.
- ⚠ `HlsPlaylistFilter.cs:151-153`: *"still need to filter old content, but never down to an
  empty playlist"* — a guard against serving nothing.
- `#EXT-X-DISCONTINUITY-SEQUENCE` must be tracked so clients aren't confused when old
  discontinuities scroll out of the window.
- ⚠ `HlsSessionWorker.cs:203`: logs *"Transcode folder is NOT empty!"* at session start —
  **stale segments corrupt `append_list`**.
- ⚠ First-viewer wait: an 8s budget that *starts only after the playlist file appears*, "so
  slow pipeline setup (e.g. h264 profile probing) doesn't consume the budget".

**Filler**
- Tunarr treats filler as **just another lineup item** resolved by the "what's on now"
  lookup (`StreamProgramCalculator.ts:441-480`) — a commercial break is one more child
  ffmpeg, no special path. That matches Loomarr's §10 pod model well.
- *"it's pointless to show the offline screen for such a short time"* — skip-to-next-slot
  when the remaining gap is trivially small.

**Hardware / encoding**
- `NvidiaPipelineBuilder.ts:181` — a pixel-format workaround pinned to a specific FFmpeg
  commit. Expect to need version-specific fixes.
- ErsatzTV forces the **software** pipeline for HDR content on most accels
  (`PipelineBuilderFactory.cs:107`).
- `PipelineBuilderBase.cs:474` — *"workaround for ac3 audio layout changes / downmix from
  decoder so filter doesn't need to reinit (it can't, it will fail)"*.
- *"always need to specify audio codec so ffmpeg doesn't default to a codec we don't want"*.

**Sobering**
- ErsatzTV has a migration named `Remove_HlsSegmenterV2`, plus `StreamingEngine.Legacy` vs
  `NextSessionWorker` — **a second segmenter was built and abandoned, and a third is in
  progress.** Budget for getting this wrong once, and keep the backend swappable
  (which `playout.backend` already does).

## 5a. The exact flag sets (verified in source — do not guess these)

Three recipes I would otherwise have gotten wrong, each with a failure mode that only shows
up against a real player.

### The test card / offline card — `ffmpeg/ffmpegText.ts:18-45`

V6's gate is "a channel playing a test card", so this is literally the first thing to build,
and it needs **no library content at all**:

```
-f lavfi -re -stream_loop -1 -i color=c=black:s=<W>x<H>
-f lavfi -i anullsrc
-vf drawtext=fontfile=<font>:fontsize=30:...:text='<title>',drawtext=...text='<subtitle>'
-c:v <videoFormat> -c:a <audioFormat> -f mpegts pipe:1
```

Three details that matter:

- ⚠ **`anullsrc` as a SECOND input** — a silent audio track. A video-only MPEG-TS stream is
  a classic cause of a player refusing to play or showing no timeline. Ship video-only and
  you debug it against Emby.
- ⚠ **`-re`** reads the synthetic source at *realtime*. Without it a `lavfi` source
  generates flat out and floods the pipe, racing ahead of wall-clock — meaningless for live.
- **`-stream_loop -1`** on the card itself so a one-frame source never EOFs.
- A **bundled font** is required for `drawtext`. Tunarr ships `font.ttf` in its data dir.
  (Their own comment on the `color=` size param: *"this is wrong, figure out the param"* —
  so verify it rather than copying blindly.)

### Realtime pacing for real programs — `ReadrateInputOption.ts`

Better than plain `-re`:

```
-readrate 1.0 -readrate_initial_burst <seconds>
```

⚠ **The initial burst is the tune-in latency fix.** Realtime pacing alone means a joining
player waits seconds to buffer enough to start; the burst fills its buffer immediately and
then settles to 1.0x. Deliberately **not** applied to still images or the null audio source
— pacing those stalls the pipeline.

### Reconnect flags — two tiers, and they differ

| Where | Flags | Why |
| --- | --- | --- |
| **Parent** (the infinite concat), `ConcatHttpReconnectOptions.ts` | `-reconnect 1 -reconnect_at_eof 1` | `reconnect_at_eof` is what makes a child program's EOF **non-fatal** — it is the mechanism that advances to the next program rather than ending the channel. |
| **Child** (one discrete program), `HttpReconnectOptions.ts` | `-reconnect 1 -reconnect_on_network_error 1 -reconnect_streamed 1 -multiple_requests 1` | A child fetching from an HTTP media source should survive a blip mid-program. |

Applied conditionally on `protocol === 'http'` and continuity `infinite` vs `discrete` — the
two tiers must not get each other's flags. Giving the parent `reconnect_streamed` or the
child `reconnect_at_eof` would both be wrong in ways that look like intermittent stalls.

## 5b. T1 RESOLVED — how playout gets a playable input (verified against live Emby)

Fact **T1** in the original program plan: *"Loomarr does not know where media lives —
`LibraryItem` is `{ID, OfficialRating, Genres}`."* For Tunarr that never mattered, because
Tunarr resolved paths itself. Internal playout needs an actual ffmpeg input, so T1 stops
being trivia and becomes the blocker.

**Two candidate answers, tested against the live dev Emby (private endpoint redacted) 2026-07-25:**

**(a) The filesystem path.** Emby returns it with `Fields=Path`:

```
/data/movies/Falling Down (1993)/… [Remux-2160p][DV HDR10][DTS-HD MA 4.0][HEVC]….mkv
```

⚠ **Rejected.** That is *Emby's* filesystem view. Loomarr in Docker has no such path, and on
this dev setup Emby is on a different host entirely. Using it would require the operator to
mount media identically in both containers — the fragile coupling Tunarr deliberately avoids.

**(b) Emby's own HTTP stream endpoint.** ✅ **This is the answer.**

```
GET {LIBRARY_URL}/Videos/{itemId}/stream?static=true&api_key={token}
```

ffmpeg reads it directly. No shared mounts, works across hosts, and it reuses the
`library.token` playout already has. Verified end to end:

```
source:  hevc video + THREE audio tracks (dts, flac, ac3) + subrip subs, 4K DV/HDR10 remux
output:  mpegts, h264 1280x720, aac stereo 48k, 4.03s, 819KB
```

**What the real-content test taught that the synthetic card could not:**

- ⚠ **A real filter-graph failure, exactly the class ErsatzTV warns about.** `-vf scale=1280:720`
  straight into `h264_vulkan` fails with *"Impossible to convert between the formats supported
  by the filter 'Parsed_scale_0' and the filter 'auto_scale_0'"* plus a 40-line pixel-format
  dump. Cause: `scale` emits CPU frames, the hardware encoder needs GPU frames, and nothing
  uploads between them. Fix: `scale=W:H,format=nv12,hwupload` — scale on the CPU, convert,
  THEN upload. The synthetic card never hit this because it had no scale step.
- **`-map 0:v:0 -map 0:a:0` is mandatory.** Real library items carry multiple audio tracks
  (three here) and subtitles. Without explicit maps ffmpeg's default selection is not what a
  normalized profile wants, and a varying track count across programs breaks `-c copy` on the
  concat parent exactly like a varying resolution does.
- Everything a real remux throws at the encoder — HEVC, 10-bit, DV/HDR10, DTS — is handled by
  normalizing to 720p H.264 + AAC stereo. ErsatzTV forces *software* for HDR on most accels
  (prior-art §5); Vulkan handled it here, but that is one sample, not a guarantee.

**Consequence:** playout needs a `StreamURL(itemID)` on the library client — one line of URL
construction, no new API surface — and the `Slot.LibraryItemID` the scheduler already carries
is enough to resolve it. No schema change, no path configuration, no shared mounts.

## 5c. Mid-program tune-in — verified over HTTP

A channel is a wall clock, not a playlist that starts when someone watches: a viewer joining
40 minutes into a film must land 40 minutes in. That means seeking into an item, and seeking
into an HTTP-streamed 4K remux is a materially different operation from seeking a local file.

Verified against the live Emby:

```
-ss 2400 -i {emby stream URL}      # fast seek: -ss BEFORE -i
→ 2.9s wall-clock, 3.02s of output, 71 real frames decoded
```

- **`-ss` before `-i` works over HTTP.** Placement matters: before `-i` ffmpeg seeks (the
  server serves a byte range); after `-i` it decodes and discards from the start, which for
  a 40-minute offset into 4K would take minutes and burn a core doing nothing.
- ⚠ **Benign decoder noise is expected.** The seek emits `[hevc] PPS changed between slices`
  repeatedly. Output is fine — 71 frames decoded — so this is exactly the class of message
  viewra needed a non-fatal allowlist for (`session.go:383` allowlists Dolby Vision's
  "Invalid Block Addition" for the same reason). Logging it as an error trains an operator to
  ignore the log; `Process.readStderr` therefore logs at DEBUG.

## 5d. Executing the args — three failures the arg-shape tests could not see

`ProgramArgs` / `ConcatArgs` were written from §5a–§5c, and all their arg-shape tests passed
while **ffmpeg rejected the output outright**. Each failure below was found by executing the
real args (`make test-ffmpeg`, plus `PLAYOUT_TEST_STREAM_URL` against the live dev Emby), and
each is now pinned by a test.

### ⚠ `-reconnect*` are HTTP-protocol options, not global ones

```
ffmpeg -reconnect 1 -reconnect_at_eof 1 -f concat -i list.ffconcat …
→ Option reconnect not found.
→ Error opening input files: Option not found          (exit 8)
```

They are private options of ffmpeg's HTTP protocol, so against a **local file** input they are
a hard failure before ffmpeg opens anything. §5a already recorded that Tunarr applies them
*"conditionally on `protocol === 'http'`"* — the note was right and the first implementation
ignored it.

**Not a test artifact.** Filler clips are local files (§10 `FILLER_DIR`), so an unconditional
flag list means every commercial break fails to start — presenting as a channel that dies at
the first break, with an error that names an *option* rather than a file. Both tiers are now
gated on `isHTTP(url)`.

The `-protocol_whitelist` is different and stays **unconditional**: it governs what the
playlist's *entries* may use, not the playlist itself.

### ⚠ `hwupload` needs `-init_hw_device`, and the error names the wrong thing

```
[hwupload] A hardware device reference is required to upload frames to.
[AVFilterGraph] Error initializing filters
Error opening output files: Invalid argument           (exit 234)
```

The capability prober had **already found and fixed this** (`deviceInitArgs`, with a comment
warning that the message "reads like a filter bug and sends you looking in the wrong place").
`ProgramArgs` reproduced it by hand-rolling its own filter chain instead of reusing the
prober's helpers.

The lesson is about duplication, not the flag: two places deciding how frames reach the
encoder drifted within one commit. The hand-rolled version was also *wrong for QSV*, which
needs `hwupload=extra_hw_frames=64` or its lookahead intermittently fails to allocate frames.
`ProgramArgs` now composes `deviceInitArgs` + `hardwareUploadFilter`.

⚠ **"Every hardware encoder needs hwupload" is FALSE**, and a test asserting it failed against
correct code. Only the families with a separate hardware frame pool (vaapi, qsv, vulkan) need
it; nvenc, amf, videotoolbox, rkmpp and v4l2m2m accept CPU frames directly, and forcing an
upload on them *causes* the very init failure it was meant to prevent.

### ✅ The concat precondition, verified against real content

The invariant `-c copy` rests on — *independently encoded programs must be byte-compatible* —
is not checkable in Go. It needs two encodes and a remux:

```
two children, offsets 30s and 300s into a 4K DV/HDR10 HEVC remux (3 audio tracks),
each encoded separately via ProgramArgs → h264_vulkan
then: -f concat -safe 0 -i list -c copy
→ joined 4.005s, both parts present, no rejection
```

Normalization output, from the same source: `1280x720, 25/1, h264 + aac stereo 48k,
nb_streams=2`. HEVC, 10-bit, Dolby Vision, HDR10 and DTS-HD all survive being flattened to the
profile.

One test-writing note worth keeping: **count streams with `format=nb_streams`, not by counting
`stream=` lines.** An MPEG-TS repeats its program map, so ffprobe legitimately reports the same
streams several times; a line-counting version of this test saw 5 streams and failed against
completely correct output.

## 5e. The playout token is in argv, and that is not fixable

An automated security review flagged `?token=` in the parent's playlist URL: it lands in `argv`
(readable by any local user via `ps`) and in access logs. The suggested fix was to move it to a
header via `-headers` and read it server-side from there.

**Both halves were investigated and measured. The finding is real; the fix does not work.**

### `-headers` does not reach the playlist's entries

Tested against real ffmpeg with an instrumented server that records the header on each request:

```
ffmpeg -headers "X-Playout-Token: secret123\r\n" -f concat -i http://…/playlist
→ /playlist   X-Playout-Token=secret123
→ /entry      X-Playout-Token=None
```

The concat demuxer opens entries **itself** and propagates no input options to them. There is no
concat option to change this — `ffmpeg -h demuxer=concat` lists only `safe`, `auto_convert`,
`segment_time_metadata`. So a header-only scheme authenticates the playlist and then 404s every
entry: a channel that fetches its playlist and plays nothing. Tunarr puts the token in the entry
URLs for exactly this reason (§1).

### Moving the parent's own token to a header does not remove it from argv either

The obvious partial fix — keep `?token=` on entries, header for the parent — was implemented and
then **reverted after live verification**, because `ps` showed:

```
ffmpeg … -headers X-Playout-Token: <TOKEN> -i http://…/playout/playlist/sec
```

An ffmpeg header value **is itself a command-line argument**. The change moved the secret from one
argv position to another. Every credential path ffmpeg offers (`-headers`, `-cookies`,
`-auth_type`) is a `<string>` option; there is no file- or env-based alternative. Any credential
handed to ffmpeg is in argv, full stop.

### What is actually true

- ⚠ **argv exposure is inherent** to a mechanism where *ffmpeg*, not Loomarr, issues the request.
  It is not a coding mistake and cannot be coded around. Mitigation belongs at the deployment
  layer (container/user isolation), not in the URL scheme.
- ✅ **The log half was never a problem.** `logRequests` records `r.URL.Path` only, so the query
  string is not logged — confirmed in a live run (`"path":"/playout/program/ch-test"`).
- ✅ **The token is scoped to match.** §11 makes `playout_token` read-only and playout-only: a
  local user who reads it from `ps` gains the streams their household already watches, and no API
  access, no identity, no write.

Keep `?token=` in the URLs. If this is flagged again, the answer is here.

## 5f. V6 end to end with real content — what the live smoke proved

The final V6 verification, against the live dev Emby (private endpoint redacted) 2026-07-25. A channel of
three real movies, built through **`suggest.Approve`** — the approval gate, never a raw
`available` write (CLAUDE.md).

```
approved: 3 in-library picks available, 0 acquisitions wanted
  Disgrace       state=available  libraryID="641854"
  Hunger         state=available  libraryID="641892"
  Chasing Amy    state=available  libraryID="642132"
```

**Direct program fetch** (`/playout/program/live`) — 21 MB, `nb_streams=2`, h264 1920x1080 + aac.
A decoded frame is a real film image, letterboxed, not the offline card.

**Full tuner path** (`/playout/stream/live`, what a media server pulls) — h264 1080p + aac,
frames decode, **3 program requests** logged, parent ffmpeg reported no stderr.

Three properties confirmed that only real content can show:

- ✅ **A channel is a WALL CLOCK.** Two program fetches 18s apart returned *different frames*
  (differing md5). Playout resolves position from the clock, not from "wherever this request
  started" — the property `AiringAt` exists to guarantee.
- ✅ **The resolver picked a MID-CYCLE program.** Both fetches landed in *Hunger*, the second
  lineup entry, not the first. A playlist-style implementation would always start at entry one.
- ✅ **`pad` preserves profile dimensions.** *Hunger* is 2.35:1; the output is exactly 1920x1080
  with black bars. A bare aspect-preserving scale would have emitted 1920x816 and broken the
  parent's `-c copy` — the failure §5d predicted, now shown not to happen.

⚠ **Byte counts are not throughput.** The tuner path yielded ~322 KB in 22s versus 21 MB for the
direct fetch, which looks alarming and is correct: `-readrate 1.0` paces the parent to realtime,
so 22s of wall-clock is ~22s of stream. The direct fetch was unpaced buffering. Do not "fix" this.

## 6. Consequences for V6

1. **Mechanism: Tunarr's HTTP ffconcat loop.** One long-lived `-c copy` ffmpeg per channel
   over a 2-line ffconcat whose entries resolve to a "what's on now" endpoint. Least code to
   a first frame, and it is the contract Emby already accepts.
2. **Tuner endpoint is continuous MPEG-TS**: `video/mp2t`, no `Content-Length`, no ranges,
   flush periodically, and **each viewer gets its own tap** off the shared stream.
3. **One encoder per channel, reference-counted viewers**, teardown after a grace period
   that aborts on reconnect. Guard the same-key replacement race.
4. **Disconnect detection via request context** — mandatory, because the tuner path never
   re-requests anything.
5. **Per-program children must normalise identically** (resolution/framerate/codec/pixfmt)
   or `-c copy` on the parent is invalid. This is where the correctness burden sits.
6. **Filler is just another lineup item** in the "what's on now" lookup — no special path.
7. **`playout_token` in the ffconcat URLs** and the M3U entry; long-lived by design (V4).
8. **Borrow ErsatzTV's `NextState` idea** if the arg builder grows past a couple of encoders.
9. **Deferred to a second iteration, deliberately:** the error-card-fills-the-slot recovery,
   the work-ahead buffer, and HLS with a rewritten sliding-window playlist. Recorded here so
   they are found rather than reinvented.
