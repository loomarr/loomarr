# First client: Nvidia Shield (Android TV)

**Date:** 2026-08-17 · **Branch:** `shield-client` · **Base:** `origin/main` @ `6d5599a4`

**Decision:** build a full 10-foot guide app for Android TV against the **already-shipped** prepared
playout subsystem. There is no backend prerequisite.

---

## Correction to the earlier draft

An earlier version of this plan proposed a six-phase "V55 production slice" as Part A. **That work
already exists on `main`.** The earlier draft was written from the `filler-mediatools` branch, which
predates it, and from that branch's stale `PROGRESS.md`. Do not resurrect Part A.

What is on `main` today (~2,700 lines, 25 files):

| Concern | Where |
| --- | --- |
| Content identity + rendition contract | `internal/prepared/library.go` |
| Atomic publication (private workspace → validated rename) | `internal/prepared/library.go` |
| Preparation worker + ffmpeg | `internal/prepared/preparer.go`, `ffmpeg.go`, `internal/app/preparedjob.go` |
| Readiness planner | `internal/prepared/readiness.go`, `planner.go` |
| Retention / eviction | `internal/prepared/retention.go` |
| Virtual origin | `internal/playout/prepared_origin.go` |
| Composition + tune-side resolver | `internal/app/preparedplayout.go` |

Also shipped: **V57 tuner surf UX** (100-channel browser gate, prepared-only adjacent warming,
latest-request-wins) and **V60 shared DVR time-shift** (15-minute horizon over prepared history).

The stale `internal/playout/prototype_prepared/` directory in the `filler-mediatools` tree is a
superseded, never-committed spike. Beta-readiness item **B-4** already says not to commit it.

---

## The seam the client consumes

`internal/prepared/library.go` states the extension point explicitly:

> New transport adapters select a compatible contract; they do not add platform names to publication
> identity.

So the Shield adapter **selects a `RenditionContract`** and never adds "androidtv" to a publication
key. Prepared bytes are shared with the browser.

The existing client contract, unchanged:

1. `POST /v1/channels/{id}/play-url` (`internal/api/channelplayurl.go:29`) with a `DeviceProfile`
   body → signed URL.
2. Server resolves `DeviceProfile → EncodePlan` at mint time; the plan rides the URL as `?plan=`
   (`internal/api/playout.go:599`).
3. `GET /v1/playout/hls/{id}/master.m3u8` — credential is the **`sig`** query param.
   `rewritePlaylistAuth` (`playout.go:631`) already appends it to segment URIs **and** to
   `#EXT-X-MAP:URI` (the fMP4 init segment; without it, HEVC black-screens). So ExoPlayer needs **no**
   custom `DataSource` header/query injection.
4. Prepared hit ⇒ no encoder. Miss ⇒ existing live session.

### ⚠ Plan selection is CHANNEL-driven, not client-driven (V50)

The play-url path calls `ServedPlan(channelCodec, profile)` (`copyplan.go:237`), **not** `resolve`:

- **Non-HEVC channel ⇒ `PlanBaseline` unconditionally.** No client capability can promote it.
- HEVC channel + client proves HEVC ⇒ `hevc8`, or `hevc10` with `video10bit` **and** surround.
- **`PlanFull` is unreachable by any client** — it is the tuner set (`copyplan.go:192`).

So "Shield gets a `-c copy` and near-zero GPU" is true **only on HEVC channels** on the **live** path.

### ⚠⚠ On the PREPARED path, EncodePlan is ignored entirely

This is the single most important fact for this client, and it is not written down anywhere else.

- `request.Plan` appears **zero times** in `internal/playout/prepared_origin.go`.
- `OpenAsset(_ string, _ EncodePlan, rel string)` (`prepared_origin.go:110`) **discards the plan in
  its own signature.**
- `CanonicalPreparedRendition` (`prepared_policy.go:11`) is the sole rendition producer and hardcodes
  `h264` / `high` / `yuv420p` / `sdr` / `aac` / `stereo`. Only resolution and bitrate vary, from the
  operator's `playout.quality_tier`.

**Every prepared tune serves h264/AAC-stereo regardless of what the client negotiated.** The whole
`DeviceProfile → EncodePlan` negotiation is effectively dead code on the prepared path. It is *safe*
(h264 is universally decodable) but it means:

1. The Shield's HEVC/10-bit/surround capability buys **nothing** on prepared channels today.
2. The extensive V48/V50 codec-negotiation comments describe the **live** path only. They sit near
   code the prepared origin silently ignores. **Comments lag code here — verify before trusting.**

Whether to wire plan → rendition selection is a real product decision (see Open questions). The
identity model already supports it cleanly: a richer rendition is just another `RenditionContract`,
and `library.go:31` says adapters "select a compatible contract."

### ⚠ Three DeviceProfile fields are inert

`maxResolution`, `hdr`, and the `?quality=` query param are accepted, propagated, and **read by no
decision path**. The real ladder comes from the operator setting `playout.quality_tier`
(`internal/app/playoutadapter.go:933`). **Do not build a client-side quality picker** — it will
silently do nothing. Send the fields for forward-compatibility; do not surface them as UI.

### Other contract facts that shape the client

- `url` in the play-url response is **empty when `server.public_url` is unset** — a hard prerequisite
  for any native client, and the #1 predicted support ticket per the beta punch-list (D-4).
- Signed URL **TTL is 8 hours** (`playoutsign.go:48`); re-mint before `expiresAt`. Rotating the device
  playout token invalidates outstanding URLs mid-view.
- Despite the filename, `master.m3u8` is a **single-variant media playlist** — no `#EXT-X-STREAM-INF`,
  no ABR ladder to select from.
- **Every auth failure is a 404**, deliberately indistinguishable (`playout.go:126`). Do not treat 404
  as "channel gone".
- Filter the channel list on **`inAppPlayable`** — it is the same predicate the play-url `409` uses.
- `?mode=prepared` returns **204** on a prepared miss and never starts a live encoder — useful for
  adjacent-channel warming without burning GPU.

---

## Backend prerequisites (small, but real)

The prepared origin was validated against hls.js, which is far more lenient than ExoPlayer. Three
defects must land before or alongside the client. All are small and self-contained.

### P1 — No native login path ⚠ TOP BLOCKER

`sessionauth.go:36` offers exactly two role paths: a `SameSite=Strict` HttpOnly session cookie from a
browser form login (`authroutes.go:169`), or `Authorization: Bearer <API_TOKEN>` which resolves to
**`RoleAdmin` with no user identity** (`sessionauth.go:65`). No pairing endpoints exist anywhere in
`internal/api` or `internal/auth`.

So a TV app's only options today are replaying a `SameSite=Strict` cookie from a non-browser client,
or **shipping the household admin token to every TV in the living room.** The second is a genuine
security regression.

**Work:** an RFC 8628-style device-code grant issuing a per-device, **member-scoped, revocable**
token. Nothing else can be user-tested without it.

*Note:* playback auth itself is already native-ready — `sig` on every segment URI and on
`#EXT-X-MAP:URI`, no cookies, no same-origin assumption. It is only **minting** that is blocked.

### P2 — `EXT-X-DISCONTINUITY-SEQUENCE` is never emitted

Zero occurrences repo-wide. `renderPreparedManifest` emits bare `#EXT-X-DISCONTINUITY`
(`prepared_origin.go:259`) into a window that slides as segments age past `DVRHorizon` (`:211`).

RFC 8216 §4.3.3.3 requires the discontinuity sequence to increment when a discontinuity scrolls off
the top. hls.js dead-reckons from PDT and tolerates the omission; **ExoPlayer uses it to correlate
discontinuity indices across playlist reloads.** Expected symptom: position jumps or a decoder reset
every time a programme boundary ages out — i.e. roughly every 15 minutes of viewing.

### P3 — `PROGRAM-DATE-TIME` is emitted once, at the head only

`prepared_origin.go:253` writes one PDT for `refs[0]`, **outside** the loop that emits discontinuities.
Everything after the first boundary is dead-reckoned from `EXTINF` sums.

ExoPlayer maps wall clock across a discontinuity from PDT, and V60's time-shift readout ("1M 23S
BEHIND") is wall-clock based — so the native DVR readout will drift after the first programme
boundary. **Fix:** emit a PDT immediately after each `#EXT-X-DISCONTINUITY`. Same function as P2.

---

## Phases

Each is a PR, per the repo's PR-per-sub-phase convention. **P1–P3 gate S2.**

### S0 — Decisions (settle first)

| Question | Recommendation |
| --- | --- |
| Language / UI | Kotlin + **Compose for TV** (`androidx.tv:tv-material:1.1.0`, BOM `2026.08.00`). Leanback is officially deprecated. |
| Player | **Media3 / ExoPlayer `1.11.0`** |
| Location | in-repo `android/` — the API contract moves with the server |
| Auth on a TV | **device-code pairing** (see P1); typing a password on a D-pad is hostile |
| Distribution | sideload APK for beta |
| SDK levels | **`targetSdk` 33+, `minSdk` 30** — see below |

Full research briefing: [`android-tv-briefing-2026-08-17.md`](../research/android-tv-briefing-2026-08-17.md).

**Corrections to assumptions worth knowing before S3 starts:**

- **`TvLazyRow`/`TvLazyColumn` were DELETED**, not deprecated (tv-foundation 1.0.0-alpha12, Jan 2025).
  Most Compose-TV guide-grid tutorials online are dead code. Use standard `LazyRow`/`LazyColumn` —
  since Foundation 1.7.0 they have built-in TV focus positioning (`pivotOffsets` → `BringIntoViewSpec`).
- **The Shield is frozen on Android 11 / API 30** (last OS update Dec 2021), but Play requires
  `targetSdk` 33+ from **2026-08-31 — two weeks away.** Target 33+, run on 30.
- **Shield tube vs Pro does not differ by decoder.** Both 2019 models use Tegra X1+; only RAM (2GB vs
  3GB) and I/O differ. **Budget the grid against the 2GB tube.**
- **The Shield can never decode AV1**, and its VP9 is 8-bit only — so all HDR must ride HEVC Main10.
  Relevant to Open question 1.
- `androidx.security:security-crypto` is **fully deprecated**; use DataStore + Android Keystore for
  the device token, and do **not** set `setUserAuthenticationRequired(true)` on a TV.

**⚠ A Shield bug forces an early product decision:** frame-rate matching breaks Dolby passthrough
~90% of the time ([androidx/media #2258](https://github.com/androidx/media/issues/2258), still open).
You largely cannot have both. Given prepared playout is AAC-stereo-only today (see above), this only
bites if Open question 1 goes ahead.

### S1 — Pairing + DeviceProfile

- Device-code pairing against a new `/v1/auth/device` pair of endpoints (poll + approve in web UI).
- Build the profile from `MediaCodecList` at runtime — **never hardcode**. Shield Tube and Shield Pro
  differ, and AV1 support varies.
- Absent/empty profile ⇒ safe h264/aac baseline. A client that does not prove a capability never
  receives it. Send a well-formed body: the generated spec marks it `required` with
  `additionalProperties: false`, so unknown fields may be rejected.
- To earn `hevc10`, declare `video:["hevc"]`, `video10bit:true`, **and** `audio:["eac3","ac3"]` —
  surround is part of the 10-bit test (`copyplan.go:198`). `h264`/`aac` are the implied floor.

**Gate:** on a real Shield, an **HEVC** channel plays with the server reporting a `hevc10` copy plan;
an h264 channel correctly reports baseline.

### S2 — Playback

Full-screen ExoPlayer over the signed HLS URL, with the paired app entering a watching-first remote
flow. Watching owns transient now/next chrome; up/down and Channel Up/Down tune adjacent playable
Channels, digits jump to an exact Channel number, OK opens Guide, Menu opens the Surf rail, and Back
returns to the last tuned Channel. Surf is an overlay over the still-mounted player and groups
available favourites, session recents, then all playable Channels. The client does not fabricate
favourites while the server has no user-preference contract for them.

**Carry the `sig` with `ResolvingDataSource`, not header injection** — the credential is a query
param, already appended server-side to every segment URI and to `#EXT-X-MAP:URI`. Non-obvious detail
from the briefing: **strip the token in `resolveReportedUri`**, or ExoPlayer's cache keys fragment
per token and every re-mint invalidates the cache.

Use `?mode=prepared` for adjacent-channel warming — it returns **204** on a miss and never starts an
encoder, so surfing cannot accidentally spin up GPU work.

**Gate:** surf 10 channels; every tune reaches first frame within the §9.1 p95 budget; zero encoder
starts on prepared hits.

### S3 — The 10-foot guide grid

Channel × time grid, D-pad navigable, tune on select. Fed by the existing guide endpoints — the same
source of truth as XMLTV and the web guide.

The grid follows the Android TV composite contract: All/Favourites/Recent filter chips above the
grid, a visible position rail beside a long Channel list, and a bottom detail bar that tracks the
focused airing. Up/down changes Channel, left/right changes airing, OK tunes the focused Channel,
and Back cancels to Watching. Optional empty filters fall back without dropping focus.

**Use these (all `RoleMember`, all JSON):**

| Endpoint | Use |
| --- | --- |
| `GET /v1/guide?from=&to=` | the grid itself; defaults to now → now+4h (`guide.go:124`) |
| `GET /v1/channels` | channel list; **filter on `inAppPlayable`** |
| `GET /v1/channels/now-next` | the banner |
| `GET /v1/channels/{id}/upcoming?limit=` | detail card (default 6, max 24) |

`GuideAiring` (`guide.go:44`) carries `kind` (`program|filler|pending|flex`), title, series/season/
episode, start/stop ms, description, genres, year, rating, and thumb — enough for a rich grid cell.

**Do not use `/v1/playout/guide.xml`** — it is device-token-authed for media servers, and a native
client should not hold a device token.

TV focus handling is the hard part: focus must never be lost, and the grid must virtualize or it will
jank on a Shield at 60fps. Use standard `LazyRow`/`LazyColumn` (the `Tv*` variants are gone), with
`BringIntoViewSpec` for the focus line and `focusRestorer` for return-from-detail.

**The 100 × 4h sizing is reasoned from virtualization behaviour, not measured.** Validate with a
Macrobenchmark scroll test on a 2GB tube before committing to the design.

**Gate:** navigate a 100-channel × 4-hour grid without dropped focus or dropped frames.

### S4 — Program detail + polish

Detail card on select, overscan-safe margins, D-pad-only operability.

### S5 — Time-shift (optional)

V60's horizon is real but **there is no server-side seek contract** — no seek endpoint, no `?start=`
or `?offset=`. The contract is "seek within the playlist." Aside from four Go files, every V60 change
was under `web/apps/web/src/`, so **none of the logic is reusable**: it must be rewritten in Kotlin.

`DVRHorizon = 15m` is declared in Go (`hls.go:48`) **and re-declared as a client-side literal** in
`use-hls-player.ts:30`. A third native copy makes drift likelier — worth exposing in an API response
instead. Depends on **P3**, or the readout will be wrong after the first boundary.

---

## Open questions for the maintainer

1. **Wire `EncodePlan` → prepared rendition selection?** Today the Shield's HEVC/10-bit capability
   buys nothing on prepared channels. Adding a second `RenditionContract` doubles prepared disk for
   HEVC-capable clients but gives real quality/bandwidth wins on a TV. The `RenditionContract` type
   already has `HDR`, `AudioLayout`, and `PixelFormat` fields that nothing populates.
2. **Reconcile the two encoder paths first?** The software packager already accepts `hevc`/`h265` and
   `yuv420p10le` (`ffmpeg.go:120`), but the hardware adapter hard-rejects anything that is not
   h264 + yuv420p + SDR + 2000ms segments (`prepared_encoder.go:44`). Audio is AAC-only even for 5.1
   (`ffmpeg.go:99`), so AC3/EAC3 passthrough is impossible today.
3. **Scope of P1's token model** — per-device revocable tokens imply a device list UI in settings.

---

## Risks

- **Verify against `origin/main`, not a branch.** This plan was already wrong once for exactly that
  reason. `filler-mediatools` predates the entire prepared subsystem.
- **Comments lag the code.** This repo has repeatedly documented architecture that was never built
  (V47's discontinuity marker) and shipped architecture read as unbuilt (V55, here). Check source.
- **`MediaCodecList` lies by omission.** A codec listed is not a codec that plays at 4K60. Gate on
  real playback, not on the capability list.
- **No Android toolchain in CI yet.** S1 needs a build lane or the gate is manual forever.

## Out of scope

Roku, Apple TV, retiring the live path, whole-library normalization, Play Store distribution.
