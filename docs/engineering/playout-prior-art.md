# Playout prior art — Tunarr, ErsatzTV, viewra

**Read before building V6.** Three working systems were mined rather than guessing at an
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
