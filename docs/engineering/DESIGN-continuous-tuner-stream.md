# Design: one continuous tuner stream per device (#1460)

Status: **proposal for maintainer review. Nothing here is built.** Owner issue: #1460 (piece 5 of
#1418). Measured at `55086406` (beta.7). Spike code: [`docs/engineering/spike-continuous-tuner/`](spike-continuous-tuner/)
(throwaway, its own Go module, not part of any production build).

## 1. Recommendation

Build it, in this shape:

1. **The server splices; the client never renegotiates the source.** Each device holds one long-lived
   HTTP response carrying a single fragmented-MP4 timeline in the house format. A channel change is a
   small `POST` (a control message). The server joins the new channel at its next video access point
   (prepared v3 guarantees one every 200 ms or less) and keeps writing to the *same* response.
2. **Copy-only, in-process, no ffmpeg child on the hit path.** Prepared v3 segments hold one `moof`
   per 2 s with the 200 ms access points inside it as sync samples. A ~200-line Go re-fragmenter
   cuts at any access point without touching the codec data (section 4.3).
3. **Web (Chromium, Firefox): one MediaSource, one SourceBuffer, flush-and-append on a marker.** The
   measured splice showed no stall, no decoder reset event and no dropped frame beyond the noise
   floor. Channel change to first new frame was **260 ms in Chromium and 490 ms in Firefox** with a
   250 ms flush margin, against 300 to 500 ms for a prepared hit today.
4. **TV (Android): one progressive fMP4 source with a server-rebased continuous timeline**, with the
   player's forward buffer tuned down. This is a desk analysis, not a measurement (section 3.2).
5. **Anything that cannot splice falls back to today's per-channel HLS path** (still-frame overlay
   from #1458 unchanged). The continuous stream is an additive fast path per device, not a
   replacement of `/v1/.../play-url`. It is switched off per device, so the risk is bounded.
6. **WebKit/Safari is out of the first phase.** It was not measurable here (section 2.3) and the
   repo already routes it to native HLS. It keeps the current path until measured on a Mac.

The measured facts that make this credible: same-format splice needs no decoder reset in two
engines; the cut is about **2 ms of CPU for 8.6 s of media** in Go against **42 to 61 ms** to start
one ffmpeg child; and a rebased timeline removes `EXT-X-DISCONTINUITY` from the design entirely.

The facts that limit it: switch latency is bounded below by *how much the client has already
buffered* (2.1 s of buffer means 2.1 s to the new picture unless the client flushes), so this design
needs a client that can flush (Web can, ExoPlayer cannot); and a channel with no prepared media for
the current airing cannot be spliced within 200 ms, it can only be joined at its live GOP.

## 2. Web: one SourceBuffer across a splice

### 2.1 What was measured

Two synthetic "channels" were packaged with the exact prepared v3 arguments
(`internal/prepared/ffmpeg.go`: `-bf 0`, forced keyframes every 200 ms, AAC stereo, 2 s fMP4 HLS,
h264 High 4.1, yuv420p, 1280x720 at 30 fps), then copy-remuxed to 200 ms fragments. A page feeds
channel A's fragments into one `SourceBuffer` (`mode: segments`), then at a control point appends
channel B's init segment and fragments into the same buffer.
Harness: `spike-continuous-tuner/{make-media.sh,splice.html,run.mjs,run-firefox-bidi.mjs}`;
raw output: `spike-continuous-tuner/RESULTS.txt`.

Presented frames come from `requestVideoFrameCallback`; each frame is classified A/B by pixel mean.
"Splice window" is the worst (wall delta minus media delta) over the 15 frames either side of the
first new-channel frame; "elsewhere" is the same number for the rest of the run, i.e. the noise floor.

| Case | Chromium 1234 headless | Firefox 156 headless (system build) |
| --- | --- | --- |
| A to B, client buffer 0.4 s, drain | window 17 ms, floor 25 ms; **543 ms** to new frame; 0 dropped | window 12, floor 31; **594 ms** |
| A to B, client buffer 2 s, drain | window 18, floor 25; **2159 ms** | window 29, floor 13; **2211 ms** |
| A to B, flush future buffer at playhead+250 ms | window 2, floor 18; **261 ms**; 1 dropped frame | window 11, floor 12; **490 ms** |
| A to C (480p, different `avcC`), drain | window 2, floor 27; 529 ms; one `resize` event, no stall | window 15, floor 13; 605 ms; one `resize` event |
| Control: B appended with no `timestampOffset` | buffered range stays `[0, 4.02]`; the timeline never advances | `waiting` at the splice; same range |

Reading:

- **The splice adds nothing measurable.** The window numbers sit at or below the noise floor. There
  is one continuous buffered range `[0, 8.04]`, no `waiting`/`stalled`/`error` event at the splice,
  and no corrupted frames. A second `moov` with an identical codec configuration is accepted.
- **Latency is buffer depth plus one access point.** Draining a 0.4 s buffer costs about 0.5 s; a 2 s
  buffer costs about 2.1 s. Flushing the buffered-ahead range (`remove(playhead+0.25, Infinity)`)
  gets Chromium to 261 ms. A stream that runs ahead of real time on the client therefore *defeats*
  the feature; the server must pace at 1x and the client must keep its forward buffer shallow.
- **A codec/size change also worked in-band** in both engines (one `resize` event, no stall, no
  `changeType` call). That is a control result, not a promise: the house format exists so this is never
  needed, and the TV cannot be assumed to behave the same.
- **The failure mode is the timestamp, not the codec.** Appending the second channel without shifting
  `timestampOffset` overwrote the played range instead of extending it. Every channel starts at its
  own media time, so the server or client must rebase, always.
- Limits: two engines, headless, software decode, one host, 12 s synthetic clips (`testsrc2`,
  `rgbtestsrc`), one splice per case, 3 to 5 s per case. Firefox's frame callbacks fired for roughly
  60 to 80% of decoded frames (its `totalVideoFrames` is 194 to 238), so its per-frame gaps are
  coarser than Chromium's. The synthetic encode's keyframe gap is up to 221 ms (a first-frame
  timestamp artefact of my lavfi encode); real prepared output is verified to 200 ms by
  `internal/prepared/ffmpeg_access.go`.

### 2.2 What the current Web player already does

`web/packages/player/src/browser/browser.ts` already keeps a MediaSource across tunes through hls.js
`transferMedia` (`clearTransferredBuffers`, lines 151 to 260) but builds a new hls.js controller and
clears the buffers per channel. This design replaces that per-channel controller with a thin
fetch-to-SourceBuffer feeder for devices on the tuner stream. hls.js stays for the fallback and for
DVR/pause, which need the shared window (`docs/design.md`, prepared HLS window).

### 2.3 WebKit, not measured

`playwright webkit` fails to launch on this host (`Host system is missing dependencies`, needs
`sudo npx playwright install-deps`); I did not install system packages. Playwright's own Firefox
build has no H.264, so Firefox was driven as the *system* Firefox over WebDriver BiDi
(`run-firefox-bidi.mjs`). For WebKit there is only a desk finding: `browser.ts:229` routes Safari to
native HLS because of MediaSource replacement problems, and iPhone Safari only has
`ManagedMediaSource`. **Open item: someone with a Mac runs `node run.mjs webkit`.**

## 3. TV (Android, Media3 through expo-video)

### 3.1 Which method this is

**Desk analysis.** I did not build the APK or drive the emulator: it needs a server that already
emits a spliced stream, which is the thing being designed. Everything below is from the app code and
the expo-video source; each claim marks what is verified and what is not.

### 3.2 What the player allows

- The app hands the player a URL: `web/packages/player/src/native/native.tsx:228` calls
  `player.replaceAsync({ contentType: "hls", uri, headers, useCaching: false })`.
- expo-video builds `DefaultMediaSourceFactory(context).setDataSourceFactory(...)`
  (`expo-video/android/.../utils/DataSourceUtils.kt:172`). There is **no JS hook for a custom
  `DataSource`**; a custom one would need a native module or a config plugin, which is a maintenance
  cost this design avoids.
- So there are two realistic sources, both reachable from JS: `contentType: "hls"` or
  `contentType: "progressive"`.

| Option | What splicing needs | Verdict |
| --- | --- | --- |
| HLS playlist, `EXT-X-DISCONTINUITY` on each splice | Player re-parses the map/init and applies a timestamp adjuster; playlist target-duration and `liveSyncDurationCount`-style latency (2 to 3 segments) sets the floor | Works today, but latency floor is segment-based (seconds). Same path as today. |
| HLS with a rebased continuous timeline, no discontinuity | Nothing on the client; server rewrites `tfdt` so the stream is one ordinary timeline | Cleanest client contract; latency still governed by segment/part duration |
| **Progressive fMP4 over one chunked response, rebased timeline** | One init, then `moof+mdat` forever; no second `moov`; ExoPlayer sees one endless fragmented file | **Recommended for TV**: no new init to parse, latency set by forward buffer only |
| Custom `DataSource` | Native code | Rejected: not reachable from expo-video |

Unverified and must be measured first in phase 0: (a) that `FragmentedMp4Extractor` accepts an
endless progressive response without a `sidx`/duration (it is the documented shape for fragmented
progressive files, but not proven for this player version); (b) the **forward buffer floor**. The
TV app sets no `bufferOptions` (grep of `web/packages/player` and `web/apps/tv/src`), and the
bundled `DefaultLoadControl.java` uses `DEFAULT_BUFFER_FOR_PLAYBACK_MS = 1000` and a 50 s minimum
buffer. Because ExoPlayer cannot drop the buffered-ahead range on demand, TV switch latency is
`minBufferForPlayback` + up to 200 ms + network, so the app must set `bufferOptions.minBufferForPlayback`
low (an expo-video option) and the server must never send ahead of real time. Expect roughly
0.4 to 0.7 s on the Shield, not the 0.3 s Web can reach. This is an estimate, not a result.

## 4. Server

### 4.1 Where things live today (verified in code)

- `internal/playout/session.go`: `Manager` owns one long-lived ffmpeg mux per `(channel, EncodePlan)`,
  fanning raw MPEG-TS out to viewers (`Attach`, `AttachSink`, `pump`, `broadcast`).
- `internal/playout/prepared_origin.go`: `PreparedOrigin` renders short wall-clock HLS manifests over
  immutable fMP4 publications and starts **no media process**; `MPEGTSBlockSource` feeds prepared
  blocks into the live session through a finite fMP4-to-TS **ffmpeg child per block**.
- `PreparedAiring` carries `Specification`, `StartedAt`, `Offset`, `Identity`; the resolver answers
  "what airs on channel C at instant T" (`PreparedResolver.ResolvePrepared`). That is the pinned-format
  Airing handoff the issue asks to reuse.
- `docs/design.md` (prepared packaging v3): no B-frames, one video stream, consecutive video access
  points at most 200 ms apart including the tail, verified before publication.

### 4.2 Where the splice point comes from

From the publication, not from a new index. A video sync sample is an access point, and v3 guarantees
one within 200 ms of any offset. Measured on my synthetic media (spike media is one frame looser than
the verified contract): 77 keyframes in 12 s, max gap 221 ms. The splice point is *the first access
point at or after* `max(now + margin, requested offset)`; the requested offset is the resolver's
current Airing offset (channels air on a wall clock, so a tune is always mid-programme).

Not every prepared segment boundary is an access point: v3 segments are 2 s fragments with sync
samples inside one `moof`, so a splice at 200 ms needs the in-process re-fragmenter below. An
existing ffmpeg child seeks to the same points, but it also has to start.

### 4.3 Copy-only in-process splice: feasible, measured

`spike-continuous-tuner/splicecut/` parses a v3 segment (`styp`, `sidx`, one `moof` with video and
audio `traf`), finds video sync samples, and writes new `moof+mdat` fragments per access point with
`tfdt` rewritten and audio cut to the same instant. It only copies sample bytes.

| Operation | Cost (this host, warm page cache) |
| --- | --- |
| Parse 6 segments (12 s) | 35 to 140 us |
| Cut and copy 8.6 s (530 KB) into 41 fragments | 1.9 to 3.8 ms |
| Cut and copy 12 s (4.9 MB, 57 fragments) | 27 ms |
| ffmpeg child, `-ss 3.4 -c copy`, first byte | 42 to 61 ms (8 runs) |

Output was checked with `ffmpeg -f null` (no decode errors), `ffprobe -count_frames` (258 frames for
8.6 s at 30 fps, exactly right), first packet a keyframe, video DTS strictly increasing, and it
played in both browsers (the spike's splice uses the re-fragmented shape). It does not test: HEVC,
10-bit, mid-file `sidx`, edit lists, or files over a network mount. Production needs a real box
parser with bounds checks (untrusted-size fields), a fuzz target, and validation against real
publications.

### 4.4 How the control message reaches the session

New per-device object, **`TunerStream`**, keyed by device id (the device authentication path in
`docs/design.md` already gives the request a device identity):

- `GET /v1/tuner/stream` opens the response. The handler owns it for the life of the connection and
  paces writes at 1x wall clock. It returns the init segment first.
- `POST /v1/tuner/tune` `{ "seq": n, "channelId": ..., "playheadMs": ..., "bufferedEndMs": ... }`
  is validated like `play-url` (same authorisation and the existing admission check,
  `Manager.AdmitProgram`), then handed to the `TunerStream` over a channel. It returns `202` with the
  resolved airing; the picture arrives on the stream.
- `seq` orders control messages; the stream carries the same `seq` back in the marker, so a stale
  marker after a fast double tune is discarded.
- The `TunerStream` asks the same resolver as everything else for the Airing, takes the immutable
  publication, and starts the re-fragmenter at the access point. **No second scheduler, no second
  packager.**

The alternative of sending the control message on the stream connection itself (WebSocket) was not
measured and is not needed: POST latency is 0.08 to 0.5 s in #1418's play-url numbers, all hidden
behind the still-frame overlay and the 200 ms cut.

### 4.5 Ffmpeg child per block versus in-process

Recommend in-process for the **prepared hit** path only. A live session is still an ffmpeg mux
process, and its output is MPEG-TS, so a *live* channel joined into the tuner stream needs one
long-lived TS-to-fMP4 copy child per live session (not per block), the same shape as the existing
`hls: starting remux`. It can only join at the live GOP (up to seconds, section 5.2).

## 5. Protocol

### 5.1 Framing

One response body of length-prefixed frames (`u8 type`, `u32 length`, payload). Length prefixes,
not sniffing box headers, so a parser cannot desync and unknown types are skippable.

| Type | Payload | When |
| --- | --- | --- |
| `INIT` | `ftyp+moov` (house format) | First frame, and again only on a format change or reconnect |
| `FRAG` | one `moof+mdat`, at most about 200 ms of media | Continuous, paced 1x |
| `MARK` | JSON: `seq`, `channelId`, `airingId`, `flushFrom` (media ms or absent), `mode` | Immediately before the first `FRAG` of a new channel |
| `PING` | empty | Every 5 s if idle; lets a client detect a dead link |

### 5.2 Timestamps

- **Server-rebased and continuous.** The server keeps a running end time per track and writes each
  new fragment's `tfdt` from it, applying **one shift to both tracks** so the channel's own A/V
  relationship is preserved (at most one audio frame, about 21 ms, of overlap or gap at a splice; the
  spike's single `timestampOffset` did exactly this and was clean in both engines). Per-track
  independent rebasing was rejected: it accumulates drift across hundreds of surfs.
- **`flushFrom` mode (Web).** The tune carries the client's `playheadMs`; the server places the new
  channel at `playhead + margin`, sets `flushFrom` to that, and the client `remove(flushFrom, Infinity)`s
  before appending. The stream timeline can therefore step backwards on a flush, which is what
  measured well (261 ms).
- **`drain` mode (TV, progressive).** No flush is possible: `flushFrom` is absent and the timeline
  only moves forward. Latency is the client's buffer depth.
- No `EXT-X-DISCONTINUITY` and no client `timestampOffset` arithmetic in either mode.

## 6. Failure modes

| Situation | Behaviour |
| --- | --- |
| **Target channel not prepared** (the beta.7 "MISS", 3.2 to 5.8 s) | The tuner stream does not stall the viewer: it answers the tune with `409 needs-fallback`; the client keeps showing the still-frame overlay and starts the existing per-channel HLS session (no regression from beta.7). The neighbour warmer (#1457) keeps working unchanged. |
| **Live-only channel** (no prepared media by design) | Same as above in phase 1. In phase 3 a live session's TS is remuxed once per session to fMP4 and joined at its GOP; its floor is a GOP (up to 2 s), not 200 ms, and it is measured separately. |
| **Codec/format change that cannot splice** (HEVC/HDR, another resolution, 5.1) | The house format (#1418 decision 1) makes it a bug. If one appears, the server closes the stream with an explicit `MARK mode=reload` and the client falls back to per-channel HLS. Browsers were seen to survive a resize in-band; ExoPlayer is not assumed to. |
| **Reconnect** (network drop, sleep, server restart) | Client reopens `GET /v1/tuner/stream?resume=<seq>`; the server sends `INIT` and a fresh timeline anchored at the current airing of the last tuned channel. Web appends the new `INIT` with a new `timestampOffset`; TV opens a new progressive source (one decoder restart, same as today). Reconnect is not gapless and does not claim to be. |
| **Slow client** | The existing per-viewer ring (`viewerBufferBytes`) drops the connection instead of buffering without bound; the client reconnects. A tuner stream is never allowed to run ahead of 1x, so a slow client is a stall, not a queue. |
| **Airing boundary mid-stream** | Same pinned-format Airing handoff as `MPEGTSBlockSource`: the next prepared Airing is joined at its first access point; a gap plays the existing dead-air card. |
| **Rapid surfing** | `seq` latest wins; superseded tunes are cancelled before a fragment is written. |
| **Capacity** | One tuner stream is one device stream. `Manager.AdmitProgram` still applies to live sessions; prepared hits cost no transcode. |

## 7. What it replaces (and does not)

Replaces, for opted-in devices: per-tune signed `play-url` mint, per-channel hls.js controller and
`transferMedia` juggling on Web, the per-channel `replaceAsync` on TV, and most of the need for
neighbour pre-warm on prepared channels (pre-warm still covers the live fallback).

Does not replace: HLS play-url (fallback, DVR/pause, Safari), raw MPEG-TS for Emby/Tunarr (`/stream`),
the still-frame overlay (#1458, used on fallback and on slow splices), XMLTV/guide.

## 8. Phased build plan and acceptance

Baselines to beat (#1418): Web prepared hit **0.3 to 0.5 s**; TV emulator **0.52 to 1.77 s** (median
about 1.3 s at 1 s dwell) after pre-warm; MISS 3.2 to 5.8 s (web) and 6.6 to 8.0 s (TV, before pre-warm).

| Phase | Deliverable | Acceptance measurement |
| --- | --- | --- |
| 0. Measure the unknowns | WebKit run on a Mac; a hand-fed progressive fMP4 into `expo-video` on the emulator (endless response, `minBufferForPlayback` at 250 ms, splice between two clips) | WebKit gap and reset behaviour reported; TV: decoder does not restart, first frame after a splice below 800 ms; if it fails, TV keeps HLS and this design ships Web-only |
| 1. Server prepared splice | Box parser, re-fragmenter, `TunerStream`, `GET stream`, `POST tune`, `seq`, fuzz target, `409` fallback | Cut is bit-exact on a real publication set; 100+ channel surf produces zero decode errors; p95 splice CPU under 5 ms |
| 2. Web client | Feeder + SourceBuffer, `flushFrom`, fallback to hls.js | Chromium and Firefox: click to first new frame **at most 0.3 s (Chromium) / 0.5 s (Firefox) median on prepared, 1 s dwell, 4 channels**, no `waiting` at the splice, no memory growth over 200 surfs |
| 3. TV client | Progressive source, `bufferOptions` | Key press to `player.ready` **median at most 0.8 s, worst at most 1.2 s** on the same emulator run as #1418 (its 1.77 s worst is the number to beat) |
| 4. Live join | Per-session TS-to-fMP4 remux, join at GOP | Live-channel switch at most 2.5 s (versus 3.2 to 5.8 s) |
| 5. Certification (#1037) | 100+ channel run | Measured against #1037's production-path certification |

The Web target is deliberately **no worse than the prepared hit that already exists**: the value of
this design is removing the misses and the per-tune round trips, not beating 0.3 s.

## 9. Not pursued

Continuous live transcoding of every channel, WebRTC, LL-HLS alone (all from #1460); custom
`DataSource` on TV; per-track independent timestamp rebasing; a WebSocket control channel.

## 10. Open questions for the maintainer

1. **Ship Web first?** Web is measured and TV is not. Recommendation: yes, with TV gated on phase 0.
2. **Opt-in per device, or default on?** Recommendation: per-device flag through beta.8, default on
   only after the certification run.
3. **Is `409 needs-fallback` acceptable UX** for a miss, with the still-frame overlay covering the gap,
   or should phase 4 (live join) be required before the feature is on?
4. **WebKit on a Mac**: who runs it, and is Safari a supported target for beta.8?
