# Playout

Formerly `design.md` §9.1 (playout backends, the channel packager, admission, playout status) and
§6's Tunarr and Live TV wiring contracts. Loomarr plays out its own channels (decision
[0009](decisions/0009-loomarr-plays-out.md)); Tunarr remains a supported alternative backend. What
plays and when is decided by the scheduler ([`scheduling.md`](scheduling.md)); a backend decides only
how the bytes reach the television. Tuning, pause and the Android TV client are in
[`playback-clients.md`](playback-clients.md).

The decisions behind the beta.8 packager are records 0031–0033 and 0035–0041 in the
[index](README.md#decisions).

## The channel packager

A channel encodes only while someone watches it. Nothing is encoded ahead of time, and with no
viewer no encoder process runs. Each channel and format has one long-lived Go **packager**
(`PackagerHLS` in `internal/playout`, the timeline core in `internal/playout/packager`) that owns the
channel's segment timeline. It starts on the first tune and stops `DefaultGrace` (30 s) after the last
viewer leaves. There is no boot warm-up, and the channel packager is the only live path.

**The pipeline.**

- **One encoder per item.** Each scheduled item (a programme part, a break clip or a card) gets its own
  ffmpeg, built by the pipeline builder (`Build` and `BuildItem`, then `FragmentArgs`). The encoder
  emits fMP4 fragments already placed on the channel's timeline, and the packager forwards them into
  one gapless playlist: one init, 1 s closed-GOP segments and a `DVRHorizon` (15 min) window. Segments
  live on disk under `playout.hls_dir` (default `<data dir>/hls`, never tmpfs). Each process's scratch
  root is owned by a `flock`, and a later process sweeps only roots whose lock it can take.
- **Uniform output, always transcoded.** Every item is transcoded into the format's fixed geometry,
  frame rate, codec profile, colour labels and AAC-LC stereo 48 kHz, with identical encoder arguments
  per class. No boundary changes decoder state, so a programme-to-break handoff needs no decoder reset.
  Item commands drop source metadata and chapters (`-map_metadata -1`, `-map_chapters -1`) so the init
  never inherits a source's tags. The builder sets a square sample aspect ratio (`setsar=1`) so a
  scope film and a 16:9 episode write identical SPS fields. There is no live loudness filter; filler
  clips get one static gain (`FillerGain`).
- **Run-ahead and the listing gate.** The encoded timeline may lead the wall clock by at most
  `RunAhead` (12 s), enforced by back-pressure. The next item's encoder must be producing `SlateLead`
  (2 s) before its air time. The playlist lists media only up to now + `ListAhead` (6 s), which is also
  its `HOLD-BACK`. The first manifest waits until `FirstManifest` (4 s) of media is listable and a real
  item has defined the init; the tune-in item gets `FirstItemWait` (10 s). Web clients set
  `liveSyncDuration` to the same 6 s, so the playhead lands on the wall clock.
- **Slate and rejoin.** An empty slot, a failed schedule lookup or an item that is not producing by its
  deadline is filled with house-format slate. The slate is shared per (host, output) and starts only
  when needed, never beside the tune-in encoder. It **never defines the channel init**, because it goes
  through the same builder tail as items and so matches their sample descriptions. After at most
  `SlateRetry` (10 s) the schedule is asked again and the programme rejoins in progress at the right
  offset. A decoder-configuration mismatch between items is logged with both sample descriptions.
- **Two audiences, one timeline.** A browser tunes a **master playlist** naming a `<format>.m3u8`
  variant, with `BANDWIDTH`, `CODECS` (read from the channel init), `RESOLUTION` and `FRAME-RATE`.
  Asset names are flat: `<format>-init.mp4` and `<format>-segNNNNNNNN.m4s`. A media server's tuner
  reads the same timeline as one continuous MPEG-TS from the packager's TS writer (`Attach`), which
  joins each tuner at the segment airing now and keeps its own continuity counters. A channel watched
  in a browser and on a TV is one encode. A 376 s tuner read across six item boundaries, measured
  live, had no continuity breaks on any PID, video timestamp steps of exactly one frame and audio
  steps of exactly one AAC frame, and no decode errors.
- **Stills.** A warm channel's still is decoded once per segment from its newest listed segment. A cold
  channel gets one CPU-decoded frame of the airing on now, read from the source file and cached per
  airing; nothing is pre-encoded, and a break has no still. The channel-switch overlay shows it before
  video plays.

**Formats.** Every channel airs a baseline of 1080p SDR H.264 (High profile) for every device.
`DeriveChannelFormats` derives one optional **premium HEVC format** from the lineup's measured
inventory facts. Top resolution and dynamic range are derived independently: any 4K item together with
any HDR item gives `4k-hevc-hdr` (Main10, BT.2020, PQ), 4K items without HDR give `4k-hevc-sdr`, and
anything else gets the baseline alone. A channel therefore costs at most two encodes, and its dynamic
range never changes mid-stream: on an HDR stream, SDR and HLG items are converted on the GPU
(libplacebo, no inverse tone-map) and the channel's static HDR10 SEI is the packager's to write. The
10-bit letterbox is never `pad_vaapi`, which writes all-zero (green) bars into P010 frames (#1673): the
libplacebo conversion draws an SDR or HLG item's box itself at the output size (boxing at source size
and upscaling after blended the edge row into the bar), and a letterboxed PQ item is padded by
`pad_opencl` on the surface mapped from VAAPI, or refused on a host without that mapping. A host
drops a premium format, and says why, when it is software-only, when its encoder has no GPU graph
(QSV, AMF and other generic families) or, for HDR, when libplacebo is missing. `GET
/v1/channels/{id}/formats` reports the baseline, what this host airs, what the lineup would warrant and
any drop reason.

**The premium is served on a client's opt-in.** The master names the premium beside the baseline from
its output alone: CODECS (the running init's own, or the HEVC Main/Main 10 string predicted before one
runs), RESOLUTION, FRAME-RATE and `VIDEO-RANGE=PQ` for HDR10. Only a request for the premium's media
playlist starts its packager, so a baseline viewer never starts a 4K encode. That packager holds one
`premium_4k` lease for the channel's whole lineup. The class probe measures `premium_4k` at 2160p on GPU
hosts through the packager's own item builder, taking the costlier of its item paths (a PQ item, an SDR
item converted to HDR10). A measured host without that cell, or without room, leaves the premium out of
the master and refuses a premium play with 503. An HDR10 packager inserts the channel's static SEI
before the first slice of every IDR and writes `mdcv`/`clli` (and `colr` when missing) into the served
init; decoder comparisons keep reading the encoder's own init. A browser plays the premium only when
`MediaSource.isTypeSupported` accepts its exact CODECS, and pins that one level for the tune. On TV,
ExoPlayer's default track selector picks it only when the device's decoder can play it.

**One builder, full-GPU graphs per hardware family.** `playout.Build` takes a host profile, a source
and an output, and returns the ffmpeg pieces. Frames stay on the GPU from decode to encode, and a stage
leaves it only through a declared fallback recorded in `Pipeline.Fallbacks`. Scaling comes **before**
tone-mapping, and padding stays on the GPU.

| Family | Graph | Status |
| --- | --- | --- |
| VAAPI (Intel iGPU and Arc) | decode, `scale_vaapi`, OpenCL tone-map, `pad_vaapi`, `h264_vaapi` or `hevc_vaapi` | measured on an Arc |
| VAAPI (AMD) | same, with the CPU tone-map after the GPU downscale | unverified at runtime |
| NVENC | CUDA decode, `scale_cuda`, tone-map, `pad_cuda`, `h264_nvenc` or `hevc_nvenc` | measured on a GeForce |
| Software | libx264 `veryfast`, on a degradation ladder (below) | measured |
| VideoToolbox | `scale_vt` and `h264_videotoolbox`, HDR tone-mapped on the CPU | unverified: no Mac has run it |
| Generic (QSV, Vulkan, AMF, RKMPP, V4L2M2M) | CPU filters and the encoder's own upload | unverified fallback |

VideoToolbox uses GPU deinterlacing only when the configured ffmpeg binary advertises
`yadif_videotoolbox`. Otherwise interlaced sources use a declared CPU decode/deinterlace fallback
before scaling, retaining the hardware encoder. GPU-decoded frames download in their original
software pixel format before any conversion to the channel's output; 10-bit SDR therefore downloads
as P010 before converting to NV12. These fallbacks still require measured host-capacity evidence.

Rate control is quality-based VBR at quality 22 on every rung. The 1080p budget is an 8 Mbit/s target
with a 12 Mbit/s cap, and lower rungs scale both by pixel count (720p 3.6/5.3 Mbit/s, 480p 1.6/2.4,
floor 1.0/1.5). Host capability profiles are data, so every family is golden-tested on any machine.

**Tone mapping is a given, never black and never silent.** Every HDR source is tone-mapped on every
SDR output. `playout.tone_curve` picks the one curve (`hable` by default; `mobius`, `reinhard`,
`bt2390`, `bt2446a`, `spline`), and each tone-mapper spells it its own way. `tonemap_opencl` has no BT.2390, so the three
libplacebo-only curves run libplacebo and then the CPU with a declared Mobius substitute.

- Intel HDR maps the surface into OpenCL (`tonemap_opencl`) and back, zero-copy. `tonemap_vaapi` is
  banned from every path after it aired an all-black picture. The image ships Intel's compute runtime
  and an NVIDIA OpenCL ICD file; Intel Gen8–11 iGPUs would need a legacy runtime too large to ship, so
  they use the CPU tone-map (`docs/reference/hardware.md`).
- NVENC tries `tonemap_opencl`, then `libplacebo` on its own Vulkan device, then the CPU.
- A tone-mapper whose runtime is missing fails before any output, and the retry ladder demotes exactly
  that tone-mapper.
- A boot self-check encodes a few HDR frames through the live builder and reads them back with
  `signalstats`. The darkest frame's maximum luma must exceed 64 and the mean must sit within 24–220 of
  the 16–235 scale. A black picture is a red Diagnostics failure and is not demoted, because the ladder
  demotes only on errors and would air it; HDR is then dropped from the budget, so HDR channels show a
  card instead of black.

**The software ladder: never refuse for speed.** On a CPU-only host a heavy title degrades instead of
being refused. Rung 0 is full quality. Rung 1 skips the loop filter (with a 720-line working size only
for HDR or above-1080p sources). Rung 2 also skips non-reference frames. Rung 3 decodes keyframes only
at 480 lines. Every rung ends in the same scale, pad and colour labels, so the output format stays
constant, and audio never degrades. `StartRung` picks the best rung projected to reach 1.2× from the
measured cost: SDR up to 1080p starts at full quality, and 4K or HDR starts keyframes-only when nothing
is measured. `RungMonitor` steps down after 8 s below 0.97× (or 3 s of accumulated lag) and steps up
only after a 60 s dwell with headroom of the next rung's measured cost ratio times 1.2. The packager
drives it live (#1551): a software item's encoder is watched by its lease's `RungMonitor`, and a step
restarts the airing on the new rung at its current offset, with `ladderResumeWait` (10 s) to produce.

**The retry ladder.** An item whose encoder fails before any output proves something about its source
on this host, and the next attempt demotes the failing stage: a GPU decode fault
(`IsHardwareDecodeFault`) retries with a CPU decode into the same encoder, and a failed GPU tone-mapper
retries with the next tone-mapper for the curve, ending at the CPU. **The encoder never changes**,
because every item must match the channel's init. A slow item cancelled at its deadline is not a fault
and demotes nothing. ⚠ The chain's reactive step of evicting the resident suggester model when a
hardware encode produced nothing ([suggester](suggester.md#model-residency)) left with the chain: the packager's ladder has no eviction
step, and `internal/playout` no longer references the evictor.

**Direct files, Loomarr-owned facts.** Playout reads the media file directly: the library item's path
mapped through `library.path_map` (§15), with the media server's HTTP stream only as the fallback when
no mapping resolves a readable file. Stream facts, the keyframe index, loudness and break candidates
come from Loomarr's own per-source measurements (inventory, `inventory_source_analysis`), never from the
media server at airtime. An unmeasured local source gets one synchronous probe recorded against its
revision; without facts, ffmpeg probes and the gap is logged. Background analysis runs one source at a
time at low priority, reads only a bounded sample of each file (a 95 GiB remux costs about 34 MiB) and
never decodes a whole file. Audio selection follows `playout.audio_language`, described below.

**Natural mid-programme breaks** are placed by the scheduler
([`scheduling.md`](scheduling.md#break-placement-by-backend)). The packager only sees their result: a
part resumes at its cut through the packager item's `Seek`.

**Watermarks.** Every programme carries a burned-in channel bug by default, blended on the GPU
(`overlay_cuda` on NVENC; on VAAPI, Loomarr's own OpenCL kernel through `program_opencl` on the surface
mapped from VAAPI, #1613) and hidden during filler, bumpers and IDs. The programme's frames never go
through a CPU filter for it. Software,
generic, VideoToolbox and HDR10 premium outputs never draw it, the programme still airs, and the
pipeline says why in `Fallbacks`. `policy.watermark` holds `{enabled, corner, opacity, look, size,
margin, image, callsign}`, and a nil field means the default (on, top-right). A nil opacity or look
means the install setting, `playout.watermark_opacity_pct` (default 40%) or `playout.watermark_look`
(default `text`), both on Settings → Playback (#1617) and read per programme item, so a change
re-renders the bugs from each channel's next programme. The generated callsign bug has four looks:
`plate` (the callsign knocked out of a rounded plate), `text` (the Plate's letters, no plate),
`outline` (the letters as a hollow stroke) and `small-plate` (the Plate at 75% size, no shadow); an
uploaded image keeps its own shape. `POST
/v1/channels/{id}/watermark` uploads a custom PNG or WebP. Placement anchors to the measured active
picture, so a letterboxed film gets the bug inside its picture. A self-check encodes a clip with and
without a test bug and asserts that the programme is unchanged outside it, that the test bug's blend
is 65% white (its own alpha, independent of the channel default) over the measured background in
coded (limited-range) luma and neutral in chroma, that the SPS and PPS are byte-identical, and that
the overlay adds at most 10 ms per frame to the bug-off encode (the household Arc's `overlay_vaapi`
drew a correct bug at 1.25x realtime, #1595). If it fails, including on a VAAPI host without an
OpenCL runtime, the watermark is off on that host and a `watermark.disabled` Diagnostics event says
so.

**Admission is one ledger.** See "Admission is one measured ledger" below.

**What this replaced.** Beta.7 and earlier ran a three-process live chain per channel, and beta.8
development briefly built a second one to serve it:

- The **per-programme chain** (a shared channel session, an MPEG-TS block mux, an HLS remux and a
  loopback self-reach) is **SUPERSEDED** (retired 2026-09-27, #1542). It paid 15–20 s to first frame at
  worst and could not hold a decoder steady across a boundary.
- **Prepared media** (pre-encoding channels ahead of time, V55–V56, with its adjacent warm probe and
  readiness planner) is **SUPERSEDED**. The maintainer ruled it the wrong architecture on 2026-09-26,
  after it starved a household server's CPU and wedged its store, and withdrew it (#1509, #1510) along
  with the continuous-stream design (#1460, #1507). Phase 4 of #1512 (#1547) removed its code
  (decision [0040](decisions/0040-prepared-media-withdrawn.md)).
- **Load certification** (`playout-load-cert`, `internal/playoutcert`) retired with the chain.
- **Per-client `EncodePlan` sessions**, direct-play stream copy and the HEVC-follows-content rule
  (V47, V48, V50) are **SUPERSEDED** by the uniform-output packager above: a channel's format follows
  its lineup, never the watching device, and there is no copy path.

The exit criteria (G1–G11) live in #1512.

## Which backend plays a channel

**A channel names its backend.** `playout.backend` is a registry setting (§15) with a **per-channel
override**. A channel set to “Follow the default” resolves the live global value; a channel pinned to
`internal` or `tunarr` keeps that choice when the default changes. This inherited shape is intentional:
the UI says which channels follow the default, while a per-channel pin is the way to exclude one from
a fleet-wide default change. Reconciliation resolves the effective backend once per attempt so one
operation never straddles two targets.

A global backend write is a fleet transition, not a URL flip: Loomarr first records the durable
in-progress target, then reconciles every active inherited channel against it before rewiring the
media server's single owned tuner/listing pair. Pinned, paused, and detached channels are excluded.
If convergence fails, the existing tuner registration remains in place and the transition stays
pending for retry (configuration mechanics in `config-design.md` §8).

The publication checkpoint is a **system-owned row in the §5 settings KV**, not a registry setting:
`system.playout_backend_transition` stores versioned JSON `{version, applied, prepared}`. `applied`
names the backend whose tuner/listing pair is currently published to the media server; `prepared` is
empty in steady state and names the durable in-progress target during a transition. It is recorded
before fleet convergence and therefore is not proof that convergence completed; every retry runs
the idempotent fleet barrier again before publisher work.
Internal device routes are readable while internal is either applied **or prepared**, so the target
M3U/XMLTV exists before the connector points the media server at it. Publication may advance `applied`
only to non-empty `prepared`, then clears `prepared`. The row has no environment variable, Settings API
field, or UI control — it records transition progress, not operator intent. On upgrade, a missing row
initializes `applied` from the currently resolved desired backend and initializes `prepared` empty.
Older supported releases had no separate applied checkpoint, and channel presence is not evidence of
which tuner was published. Unknown versions, malformed JSON, missing fields, and unknown backend names
fail closed without replacing the row, so corruption can never silently point the media server at an
unprepared backend.

| | **Loomarr (internal)** | **Tunarr** |
| --- | --- | --- |
| Streams | Loomarr, via bundled `ffmpeg` | Tunarr |
| Break placement | between programs **and mid-roll** (§10) | between programs only |
| Transcode telemetry | real, per-session (§12) | none — Loomarr can't see inside Tunarr |
| Extra service to run | no | yes |
| Right when | you want mid-roll, fewer moving parts, or visibility into playback | your hardware can't transcode, or your install already works |

**Tunarr is not deprecated.** It remains first-class and supported: the honest answer for hardware
that can't transcode is "let Tunarr do it", and an install that already works should never be forced
to migrate.

⚠ **Per-channel overrides are not yet a complete mixed-tuner installation.** Reconciliation,
playback routing, and now/next resolve the backend per channel, but the media-server connector owns
one Loomarr tuner/listing pair selected by the global backend. A channel pinned to the non-global
backend therefore does not automatically appear in Emby/Jellyfin Live TV. Full mixed delivery needs
two owned tuner/listing pairs (or one aggregate Loomarr feed) and is follow-up work; the override is
still useful for direct/in-app internal playback and controlled backend migration, but the UI must
not imply that a mixed media-server guide is already wired.

**A committed internal schedule change is a playout cutover.** Reconciliation persists the new
`Desired` cycle before retiring any process-local packager that may still be reading the previous
cycle; the next viewer request starts at the new cycle's current wall-clock offset. An unchanged
reconcile leaves the running packager alone. In Postgres deployments the channel invalidation carries
a compact fingerprint of the accepted cycle, so every replica retires its stale packager—not only
the replica that performed the reconcile. This ordering keeps the Guide and pixels on the same
committed cycle without interrupting playback for deadline-only reconcile writes.

## What internal playout serves

- **Segments** over **both HLS and MPEG-TS**, from the one packager timeline. Both, because media
  servers differ in what they accept and the compatibility matrix is not ours to police. The MPEG-TS
  stream (`/playout/stream/{id}`) is what a media server or ffmpeg pulls. The **HLS pair** (a master
  playlist, then `<format>.m3u8` and its init and segment files) is what a **browser or a native app**
  plays, because a `<video>` element cannot consume raw MPEG-TS. Both read the same fMP4 segments, so
  a channel is one encode however many people and devices watch it.
- **A channel still** (`GET /playout/still/{id}`, signed like the HLS pair; the play-url carries its
  signed URL) is one JPEG frame. A warm channel's still comes from its newest listed segment, so it is
  never older than one segment. A cold channel's still is one frame of the airing on now, decoded from
  the source on the CPU and cached per airing (HDR sources are tone-mapped with the CPU after a
  960-pixel downscale). A request never starts an encoder. It exists so the channel-switch overlay
  (#1458) has a picture to show before video plays; a break has no still and answers 404, and the
  overlay then shows its card on the plain background.
- **Scheduled break fallback is not an empty Channel.** If a filler pod has no playable clip, the
  synthetic card says “We'll be right back,” preserves the break's wall-clock identity, and is
  bounded by the time remaining in that break so it cannot cover the next programme. “Nothing
  scheduled” is reserved for a genuinely empty/unairable lineup; transient source failures remain
  “Program unavailable.”
- **An M3U tuner** (`/playout/tuner.m3u`) — the channel list the media server registers.
- **An XMLTV guide** (`/playout/guide.xml`) — the listings provider.

Both device-facing documents expose the **transport-published internal catalog**: effective-internal
channels that are on air. This normally equals the in-app surfable catalog; while internal is the
durable `prepared` target it additionally includes inherited channels so the media server can read
the target feed before cutover, without exposing that target through ordinary UI routing. Paused,
detached, and empty channels remain visible in
Loomarr's own Guide as channel rows for diagnosis and control, but their now/next and upcoming
programme answers are empty; they are absent from M3U/XMLTV and direct internal tune requests return
404. When an internal channel leaves this catalog through a lifecycle write (pause, detach, purge,
or an effective-backend change), the committed write immediately stops every packager
of that channel and re-scans the media-server tuner. Reconciliation from live to empty
also re-scans so the dead channel is removed. Additions re-scan too: a transition from empty to its
first playable programme changes the tuner channel list and cannot use only an EPG refresh.

On Postgres that stop is cross-replica and commit-ordered. Lifecycle-sensitive channel writes and
the system-owned backend-publication checkpoint emit a `LISTEN/NOTIFY` invalidation from the same
database statement or transaction as the durable write; Postgres delivers it only after commit.
Every replica holds a dedicated listener, applies each committed stop transition to its process-local
packagers, and performs a full durable reconciliation after subscribing or
re-subscribing. A listener disconnect or a durable reconciliation/read failure closes new playout
admission and retires every local live delivery before reconnecting, so a missed notification cannot
leave an encoder running or admit a replacement session. Admission reopens only after `LISTEN` is
active and the durable channel/checkpoint state has converged locally. A five-second connection
heartbeat bounds detection of a half-open listener socket; it probes liveness, not lifecycle state.
The Postgres playout seam also revalidates durable channel/checkpoint eligibility at each MPEG-TS,
HLS playlist, and HLS asset admission boundary; a read failure denies admission, and ordering that
check with teardown prevents a request admitted just before commit from escaping the stop. This is
event-driven delivery,
not the channel scheduler's periodic sweep: “immediately” means Postgres queues the invalidation at
commit and a connected replica handles it on receipt, subject to ordinary database/network scheduling;
there is no polling interval in the off-air path. SQLite remains single-replica and uses the existing
post-commit in-process stop.

Pause is local ownership state in v1. Loomarr stops its own playout and guide answers, but a retained
managed Tunarr projection keeps playing its last lineup; detach and internal/Tunarr transitions
likewise preserve that historical projection until explicit purge. Making a remote Tunarr projection
durably off-air requires persisted projection lifecycle state and retry, not a one-shot lineup clear.

## Admission

One ledger, `playout.ResourceBudget`, admits every stream against what this host **measured**, not a
static number. A packager takes one lease when it starts, and a stream is admitted against
`min(GPU throughput at 1.2×, CPU allowance ÷ measured CPU per stream, encoder sessions)`, per stream
class and ladder rung. The classes are 8-bit SDR up to 1080p, 10-bit up to 1080p, and 4K or HDR with
tone-mapping. A new stream **drops a rung before it is refused**, and a live session is never evicted.

- **Measured capacity.** A boot class probe runs synthetic clips per class through the live builder's
  real graphs and stores the result under `playout.state_dir`, keyed to the ffmpeg, GPU and encoder
  fingerprint. The HDR class is re-measured at every start, because a driver update can break a
  tone-mapper without changing the fingerprint. Live encodes refine the **CPU** term within bounds;
  speed cannot be observed at 1× pacing.
- **The probe is background work.** It runs at nice 19 and yields the moment a live transcode is
  admitted (`BackgroundContext` is cancelled inside `Reserve`), so a tune never waits on it. An
  interrupted measurement is discarded and rerun once playback is idle, never taken for a failed
  tone-mapper.
- **Encoder sessions are capacity.** The probe records `opened + in use` NVIDIA encoder sessions, so
  sessions held by another application count against the cap.
- **Operator cap.** `playout.max_channels` can only lower the result, never raise it.
- **Priced by the first item.** Starting a packager resolves the item airing now and books that item's
  class, so a heavy first item on a nearly full host is demoted or refused (503) before any encoder
  starts. A card slot or a failed lookup books SDR until a real item re-prices the lease. The
  schedule reuses the resolved item, so it is not resolved twice.
- **The CPU-only case.** On a software host, admission picks the best software rung whose measured CPU
  cost fits the allowance. A class measured below 1.2× is never refused for speed and degrades
  instead. Admission refuses only when even keyframes-only does not fit, and the viewer gets a 503.
- **Live steps re-price the lease** (`Lease.StepSoftware`): a step down always applies and releases the
  CPU, and a step up applies only if it fits beside the other leases.
- **CPU cost is learned from delivered media.** An item's encoder CPU is divided by the frames it put on
  the timeline, not by its slot, so an early close cannot over-count.
- **Background media work yields to playback.** While any live transcode runs
  (`PlaybackNeedsHeadroom`), capped background work such as filler processing waits.

The dashboard's `capacity` and the status endpoint's `budget` read this ledger. The ledger still has a
`copy` class that costs nothing, but packager items are always transcoded, so it is unused there.

**Watching from Loomarr's own UI (V46).** The Web UI plays a channel in the browser directly — a
**Watch** sub-section on the channel-detail page (§12), also reachable from the guide's per-row menu.
It plays the HLS pair above. Two facts make this a real feature and not just a `<video src>`:

- *A browser needs HLS, and on most browsers a JS shim.* Safari/iOS play `.m3u8` natively; Chrome/
  Firefox/Edge need `hls.js` (§14) over Media Source Extensions. The player picks the native path
  when the browser advertises it and falls back to the shim otherwise — the same `.m3u8` a future
  native app hands to AVPlayer/ExoPlayer unchanged, which is why the transport is HLS rather than
  anything browser-specific.
- *A person is watching, but the stream still authenticates a device.* The browser must not hold the
  `playout_token` (§11: it is the media server's device secret, not a per-person credential). So a
  **session-authenticated** op — `POST /v1/channels/{id}/play-url` — mints a **short-lived signed
  URL** the client feeds to its player. The signature is an HMAC over `channel + expiry` keyed by the
  existing `playout_token`: **no new secret**, the token never leaves the server, and the URL
  self-expires (the mock's "signed with the playout token, good for 8 hours"). The HLS routes accept
  **either** the device token (a media server) **or** a valid signed URL (a browser/native app);
  everything else about segment auth below is unchanged.

**Audio track selection is ours to make, because nobody else is left to make it.** With no explicit
`-map`, ffmpeg picks one stream per type by "best" — for audio that means **the most channels**,
ties broken by lowest index. It does not read language tags and it does not honour the `default`
disposition. So a release whose Russian dub is 5.1 and whose English track is 2.0 plays **in
Russian, every time, deterministically** — which is exactly what a dev-install channel did. Direct
playback never showed it because the media server applies the viewer's language preference; internal
playout calls `ffmpeg` itself and bypasses that entirely.

`playout.audio_language` (§15, default `eng`) names the preferred track. The selection is
**preference, not requirement**: `-map 0:a:m:language:<pref>?` with the trailing `?` making the
match optional, plus a `-map 0:a:0` fallback so a file with no tagged track of that language still
gets audio. A hard map without the `?` fails the whole encode on an untagged file — a channel that
goes black rather than one that speaks the wrong language, which is strictly worse. Empty means
"whatever ffmpeg would have picked", preserving today's behaviour for anyone who wants it.

*A per-channel override is now offered; a per-viewer one is still not (V46).* When the Watch UI (above)
gave audio a visible control, the question "whose choice is this?" had to be answered honestly. A
per-**viewer** track is still refused for the reason it always was — it forks the encode per viewer and
breaks one-encoder-per-channel. But a per-**channel** override is the *same shape* as the instance
default already described here, just resolved with `policy.playout.audio_language` precedence over the
global (like every other channel policy, §15) — so the Watch tab's Audio control is **admin-scoped and
channel-wide**: it re-picks the track for the shared stream, for everyone, exactly as changing the
instance default would. The per-**title** decision (a subtitled original vs a dub, one program at a
time) remains a separate, unscoped feature; a channel-wide knob does not pre-empt it.

**Subtitles are not a setting yet.** The track probe reports what the current item carries, but the
encoder deliberately maps video plus one audio stream and drops subtitles. The previously-drawn
global and Watch-tab burn-in controls were therefore inert and are retired in V55. A future subtitle
control must add the real encoder filter and its direct-play/transcode consequences in the same
change; a selectable per-viewer soft track would still require per-viewer output and is outside the
one-encoder-per-channel model.

Both files carry a **`playout_token`** (§15, a generated secret): every segment request is signed, so
only the operator's media server can pull the stream. Regenerating it invalidates the media server's
old wiring credential and invokes the durable Live TV publication repair, so the UI gates it behind
a typed confirmation. On Postgres the durable generated-secret row is read at publication and
request-authorization boundaries, so a rotation handled by one replica cannot leave another accepting
the old token, rejecting the new tuner URL, or repairing the registration back to stale credentials.
SQLite keeps the in-process value because its contract permits only one replica.

**Segment auth is a second authorization path, and §11 says so explicitly.** A television cannot hold
a session cookie, so segment routes authenticate a **device** by token, not a **person** by session.
This is the only route family that bypasses the allowlist model, it is read-only, and it is scoped to
playout. It is described in §11 alongside the credential paths rather than left implicit here.

## Playout status

Playout has several ways to fail that all present identically to a viewer (a black frame) but have
different causes: a codec the target can't decode, a hardware encode starved of VRAM by the resident
LLM (§8.2), a transcode running below realtime, a channel with no session at all. Diagnosing which
one, this build learned, means correlating three things the running app knows but did not expose
together: the **live encoders** (`Stats()` — per (channel, target): encoder, hardware/software, and
crucially *realtime speed*, where a sustained value **below 1.0×** is the stutter/stall signal), the
**GPU + its VRAM** (`nvidia-smi`), and the **resident LLM** sharing that VRAM (Ollama `/api/ps`).

**`GET /v1/playout/status`** (admin-only, §11) composes exactly those into one health picture:

- A **GPU/VRAM header** — total and used VRAM, encoder-engine utilisation, and the resident LLM's
  footprint — because the shared-GPU contention (§8.2) is invisible from the encoder rows alone.
- One **health row per running (channel, format)**: its encoder + hardware/software, its speed, and a
  verdict — **`ok`** (comfortably ≥1.0×),
  **`degraded`** (near 1.0×, at risk), or **`stalled`** (below 1.0× — the channel is losing to
  wall-clock and will buffer) — each with a one-line reason an operator can act on.

It is a **read-only projection of live state**, never a control surface: it changes nothing, so it is
safe to poll and safe to hand a support request. It is the in-app twin of `scripts/playout-diag.sh`
(the shell-level process/GPU forensics), and it is what the dashboard's playout panel renders. Where
`GET /v1/playout/sessions` reports raw per-encoder telemetry, the doctor adds the *verdict and the
context* — the GPU/LLM picture and the ok/degraded/stalled judgement — so "why is it black?" has an
answer without shelling into the box.

## Tunarr

On a Tunarr-backed channel the scheduler projects its lineup through the `Programmer` port, a
hand-written client for only the Tunarr endpoints Loomarr uses. Tunarr has no authentication, so
Loomarr stores only its URL. Tunarr owns transcoding, streaming and its M3U/XMLTV; Loomarr owns the
lineup and filler.

- Each Programmer operation reads `tunarr.url`, `tunarr.transcode_config_id` and the filler attach
  policy from one settings snapshot, so one reconcile never straddles two Tunarr instances.
  Endpoint-derived caches are scoped to the normalised Tunarr URL.
- `POST /v1/setup/tunarr-connect` wires the media server as Tunarr's media source (reusing the admin
  API key), enables the movie and show libraries and scans them. `/v1/setup/status` reports a
  `tunarr_library` check until that is done.
- Programming entries need Tunarr's own program id, so the adapter maps media-server item ids through
  Tunarr's persisted `/programs` index (never the ephemeral browse handles). An unindexed item airs
  as flex, and a reconcile with misses triggers one best-effort library scan.

## Live TV wiring

For Loomarr's channels to appear in the family's TV guide, the media server consumes the **tuner + guide** surface of the durably applied playout backend: Loomarr's own M3U/XMLTV routes for internal playout, or Tunarr's routes for Tunarr playout. This is one owned tuner/listing pair, never per-channel registration. Once wired, channel changes propagate through the selected backend's output; Loomarr then pokes the media server so the change appears in minutes rather than after its nightly refresh. **The poke is operation-specific (§9):** a *new or removed* channel needs a **tuner re-scan** (re-read the M3U channel list — a guide refresh alone won't surface it); an *existing* channel's lineup change needs a **guide refresh** (EPG data).

- **Endpoints (both flavors, Emby lineage):** `POST /LiveTv/TunerHosts` (type `m3u`, `Url` = the applied backend's playlist URL) and `POST /LiveTv/ListingProviders` (type `xmltv`, `Url` = its guide URL), using the admin `LIBRARY_TOKEN`. **M3U is preferred over HDHomeRun emulation** — explicit and discovery-free, so registration is deterministic.
- **One-time & never silent.** There is no per-channel media-server registration. Wiring is an idempotent consequence of saving a relevant backend, URL, media-server connection, or playout-token setting. Those mutations and every prepare/publish/retire effect run inside the durable transition coordinator described in §9.1 and `config-design.md` §8. `POST /v1/setup/livetv-reconnect` (admin — §7) force-repairs the durably applied internal or Tunarr target under that same cross-replica lock when a stale channel→stream binding needs clearing: it enumerates, removes, and re-adds both the Loomarr-owned tuner and listing provider, and fails visibly if any wiring operation fails. *There is no `livetv-connect` route; it was removed when wiring became automatic, and `scripts/check-retired.sh` bans the name.*
- **Idempotent & self-healing on URL change.** Enumerate first via **`GET /System/Configuration/livetv`** — one read that returns `{TunerHosts, ListingProviders}` — and if the applied pair is already registered the connect is a no-op. Duplicate tuners are a classic Emby mess; tests assert **second-call-no-op** (Phase 10 gate). **Reconcile is by *identity*, not URL string.** Loomarr tags every tuner it registers with `FriendlyName: "loomarr"`, so `Connect` owns exactly the tuners it created: when the applied URL pair changes (for example, the operator repoints `TUNARR_URL`), it first **prepares** the new pair by adding and verifying both the target tuner and target listing while the old pair remains registered. Only after both target registrations exist does it **retire** the stale Loomarr-owned pair (`DELETE /LiveTv/TunerHosts?Id=<id>` and `DELETE /LiveTv/ListingProviders?Id=<id>` → 204, Phase-0 capture). A failed tuner or listing add therefore leaves the working pair untouched; a retry completes the missing half idempotently. A tuner the household added by hand (any other `FriendlyName`) is **never touched** (§9 ownership: Loomarr owns only what it created). Listing providers carry no `FriendlyName`, so the stale one is identified as the Loomarr-shaped `xmltv` provider whose `Path` is a Tunarr or internal-playout guide URL that no longer matches. Preparation, freshness, and retirement are separate connector operations: a backend transition may publish a prepared internal feed before asking the media server to re-scan it, durably activate that backend, and retire the old pair afterward. The ordinary `Connect` composition still performs all three in one call. **A connect that changed anything (added or retired a tuner/listing) then pokes the media server — a tuner re-scan *and* a guide refresh — so the freshly-registered tuner's channels are discovered and their EPG populated immediately, rather than after the media server's nightly scan** (the newly-wired tuner has zero channels in the media server's view until it re-reads the M3U — a guide refresh alone won't surface them; §9 poke semantics). Both pokes are **best-effort**: a poke failure degrades freshness but never fails the wiring. A no-op connect (nothing changed) skips the pokes — there is nothing new to discover. *The Emby-lineage `GET /LiveTv/TunerHosts` / `GET /LiveTv/ListingProviders` are **write-only on Jellyfin** — `POST` works, `GET` returns **405** (verified against Jellyfin 10.10.3). Enumerating through them therefore failed on every Jellyfin install, so the idempotency check could not run and the connect either errored or duplicated the tuner on each attempt. The Phase-10 capture was Emby-only, which is how it survived: §6 claims both flavors, and only Emby was ever exercised. The config endpoint answers 200 on **both**, so this is one code path rather than a flavor branch.*
- **Version fragility → live capture.** The endpoints exist on both flavors, but **payload fields and the guide-refresh task id drift across versions.** A Phase-0-style maintainer-supervised capture (folded into Phase 10, §21) pins the exact accepted request/response payloads + the guide-refresh task id from the real Emby/Jellyfin into `internal/testkit/fixtures/`; the adapter is written against those pins, not memory. Any contract deviation ⇒ update this doc first.
- **Division of labor follows the selected backend (§9.1):** Loomarr always decides *what plays and when*. With internal playout it also owns streaming/transcode and M3U/XMLTV publication; with Tunarr playout it projects the schedule through `Programmer` and Tunarr owns those runtime surfaces. Emby/Jellyfin consume whichever one the durable applied checkpoint publishes.

## Constraints that follow from owning playout

The consequences recorded when Loomarr took over playout are archived in
[`playout-2026-07-consequences.md`](../../project/design-archive-2026-09/playout-2026-07-consequences.md).
These still bind:

- **ffmpeg and ffprobe are core runtime dependencies** and ship in the single image.
- **Capability is per ffmpeg build.** Every optional encoder and filter is probed from the binary,
  never inferred. A card whose `drawtext` is unavailable degrades to an unlabelled colour field,
  never to a dead channel.
- **Every "what is on now" reader answers for the backend actually streaming that channel,** or not
  at all. Internal now/next comes from `BroadcastsBetween`, the resolver the encoder and XMLTV share.
- **Restart interrupts internal channels.** Restart copy is per backend: internal channels drop for a
  few seconds, Tunarr channels keep playing, and `GET /v1/playout/sessions` supplies the count.
- **One process-tree owner.** Every encoder and filler ffmpeg runs under the same supervisor and
  Unix process group, so cancellation, a crash or an in-process restart sweeps descendants. A bare
  `exec.CommandContext` around a lifecycle-owned encoder is a violation.
