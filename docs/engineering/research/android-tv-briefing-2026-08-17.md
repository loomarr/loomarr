# Android TV app for NVIDIA Shield — implementation briefing

**Compiled 2026-08-17.** Target: native Kotlin Android TV app, live HLS from a self-hosted Go server, 10-foot channel×time guide grid.

**Read this first — the three facts that shape the whole build:**

1. **The Shield is frozen on Android 11 / API 30** and will not advance. Meanwhile Google Play requires `targetSdk` 33+ by **2026-08-31** (two weeks out). You must target 33+ while *running* correctly on API 30.
2. **`TvLazyRow`/`TvLazyColumn` no longer exist.** They were deleted in `tv-foundation` 1.0.0-alpha12 (Jan 2025). Most tutorials you'll find are dead code. Use standard `LazyRow`/`LazyColumn`.
3. **The Shield cannot decode AV1**, and its VP9 is 8-bit only. All HDR must ride HEVC Main10.

---

## 1. UI stack: Compose for TV

### Verdict

**Production-ready, and it is now the only recommended path.** Leanback is officially deprecated; Google's docs label it "Building UI with the Leanback UI toolkit (deprecated)" and ship a migration guide to Compose. New work should not start on Leanback.

### Coordinates (current as of 2026-08)

```kotlin
dependencies {
    val composeBom = platform("androidx.compose:compose-bom:2026.08.00")
    implementation(composeBom)

    implementation("androidx.tv:tv-material:1.1.0")   // stable, 2026-05-06

    implementation("androidx.activity:activity-compose:1.13.0")
    implementation("androidx.compose.ui:ui-tooling-preview")
    debugImplementation("androidx.compose.ui:ui-tooling")
}
```

| Library | Version | Date | Status |
|---|---|---|---|
| `androidx.tv:tv-material` | **1.1.0** | 2026-05-06 | stable |
| `androidx.tv:tv-foundation` | **1.0.0** | 2026-05-06 | stable, **but see below** |
| `androidx.compose:compose-bom` | **2026.08.00** | 2026-08 | stable (core modules 1.12) |

> ⚠️ **Doc lag.** The Compose-for-TV *guide* page still shows `tv-material:1.0.0`; the *release notes* page shows 1.1.0 stable. The release notes are authoritative. ([guide](https://developer.android.com/training/tv/playback/compose), [releases](https://developer.android.com/jetpack/androidx/releases/tv))

> ⚠️ **Compose 1.12 raises `compileSdk` to API 37 and requires AGP ≥ 9.1.1.** If that's disruptive, pin BOM `2026.04.01` (core 1.11.0) instead.

### tv-material vs. compose material3

Use `androidx.tv.material3.*`, **not** `androidx.compose.material3.*`, for interactive/focusable components (`Button`, `Card`, `Surface`, `ListItem`, `NavigationDrawer`). The TV variants carry focus-state visuals (scale, glow, border) tuned for D-pad. Google warns explicitly: *"mixing Compose Material with Compose Material from Compose for TV can result in unexpected behavior."*

The class names collide (`androidx.tv.material3.Button` vs `androidx.compose.material3.Button`) — an easy and silent mistake. Non-interactive primitives (`Text`, `Row`, `Column`, `Box`) come from regular Compose as normal.

### What about tv-foundation?

`tv-foundation` 1.0.0 is stable but is now nearly an **empty shell** — its reason for existing (the TV lazy layouts) was removed. You most likely do **not** need this dependency at all. Add it only if you hit a specific API that lives there.

---

## 2. The guide grid

### The architecture that actually works

A channel×time EPG is **not** a `LazyVerticalGrid`. Rows have uniform height but each program has a *different width proportional to its duration*, and the time axis must stay aligned across all rows. Use:

- **Outer `LazyColumn`** — one item per channel (vertical virtualization over ~100 channels).
- **Inner `LazyRow` per channel** — programs, each with `Modifier.width(durationToDp(program))` (horizontal virtualization over 4 hours).
- **A shared horizontal scroll state** so all rows and the time ruler stay locked together.

### Migration table (this is the big API break)

| Removed | Use instead |
|---|---|
| `TvLazyColumn` | `LazyColumn` |
| `TvLazyRow` | `LazyRow` |
| `TvLazyVerticalGrid` | `LazyVerticalGrid` |
| `TvLazyHorizontalGrid` | `LazyHorizontalGrid` |
| `pivotOffsets` | `BringIntoViewSpec` via `LocalBringIntoViewSpec` |

Since **Compose Foundation 1.7.0**, the standard lazy layouts have built-in focus-positioning; they handle D-pad traversal and keep the focused item visible without TV-specific wrappers. ([source](https://developer.android.com/training/tv/playback/compose/lists))

### Pivot scrolling (the "focus line")

TV guides conventionally hold the focused item at a fixed position rather than letting it drift to the screen edge. That's `BringIntoViewSpec`:

```kotlin
@OptIn(ExperimentalFoundationApi::class)
@Composable
fun PositionFocusedItemInLazyLayout(
    parentFraction: Float = 0.3f,
    childFraction: Float = 0f,
    content: @Composable () -> Unit,
) {
    val bringIntoViewSpec = remember(parentFraction, childFraction) {
        object : BringIntoViewSpec {
            override fun calculateScrollDistance(
                offset: Float, size: Float, containerSize: Float
            ): Float {
                val initialTargetForLeadingEdge =
                    parentFraction * containerSize - (childFraction * size)
                val targetForLeadingEdge =
                    if (size <= containerSize &&
                        (containerSize - initialTargetForLeadingEdge) < size) {
                        containerSize - size          // don't overscroll at the end
                    } else {
                        initialTargetForLeadingEdge
                    }
                return offset - targetForLeadingEdge
            }
        }
    }
    CompositionLocalProvider(
        LocalBringIntoViewSpec provides bringIntoViewSpec,
        content = content,
    )
}
```

`LocalBringIntoViewSpec` applies to *every* scrollable beneath it. To opt one nested list back out, re-provide a default: `object : BringIntoViewSpec {}`.

> **Caveat:** `BringIntoViewSpec` is still `@ExperimentalFoundationApi`. It is the officially documented migration target, but expect signature churn — wrap it in one file so a breaking change is a one-place fix.

### Focus restoration

Two tiers, and picking the wrong one is a common source of "focus jumps to row 0":

**`Modifier.focusRestorer()`** — sufficient for a single screen. Remembers the last-focused child of a focus group and restores it when focus returns.

```kotlin
LazyRow(modifier = Modifier.focusRestorer()) { /* programs */ }
```

**`FocusRequester.saveFocusedChild()` / `restoreFocusedChild()`** — needed for restoring focus *across* screens (guide → player → back to guide). Pair with `focusProperties`. Use this for your back-from-player path; `focusRestorer` alone will not survive the navigation.

Stability, which the docs are inconsistent about:
- **Stable:** `focusGroup()`, `FocusRequester`, `focusProperties { enter/exit/canFocus }`, `LocalFocusManager.moveFocus()`.
- **Experimental / historically buggy:** `focusRestorer`. Known not to work in some configurations; keep the Compose version current.

Requesting initial focus needs care — calling `requestFocus()` before the target is laid out throws. Guard it:

```kotlin
val focusRequester = remember { FocusRequester() }
LaunchedEffect(Unit) {
    // Let composition/layout settle before demanding focus.
    runCatching { focusRequester.requestFocus() }
}
```

### Realistic performance on Shield-class hardware

100 rows × 4 hours is **comfortably achievable**, but only because virtualization means you never render 100 rows. Sizing the real work:

- Visible at 1080p 10-foot density: roughly **6–9 channel rows**, each showing ~4–8 programs → **~40–70 composed program cells** at any moment. That is a *small* Compose workload.
- The cost that bites is **row instantiation during fast vertical D-pad travel** — each newly visible row spins up a `LazyRow` with its own scroll state.

Budget against the **2 GB tube**, not the 3 GB Pro. Rules that matter:

1. **Measure only in a release build with R8 enabled.** Debug Compose is dramatically slower and will send you chasing phantom jank. This is the single most common false alarm.
2. **Ship a Baseline Profile.** On low-end devices these deliver ~15% scroll improvement and large startup wins. Google's own TV sample documents the setup ([JetStream](https://github.com/android/tv-samples/blob/main/JetStreamCompose/baseline-profiles.md)).
3. **Provide stable `key`s** in `items(...)` so identity survives data refresh and focus isn't dropped when the EPG updates.
4. **Hoist all work out of item lambdas.** Time→pixel math, date formatting, and label computation must be precomputed, not recalculated per scroll frame.
5. **Don't hold the whole EPG in memory as composables' input.** Compose is lazy; your data layer is not. Window the program list per channel to the visible time range plus a margin.
6. **Never nest a `LazyRow` directly in a `LazyColumn` outside an `item {}`** — it produces infinite-constraint measurement crashes.

**Where jank will actually come from:** program artwork. Decoding images on the main thread during D-pad travel is the classic TV-guide killer. Use a proper image loader with an explicitly capped memory cache.

---

## 3. Media3 / ExoPlayer for live HLS

### Coordinates

```kotlin
val media3 = "1.11.0"   // stable, 2026-08-05
implementation("androidx.media3:media3-exoplayer:$media3")
implementation("androidx.media3:media3-exoplayer-hls:$media3")
implementation("androidx.media3:media3-ui-compose:$media3")     // Compose player UI
implementation("androidx.media3:media3-datasource-okhttp:$media3") // optional
```

> Use **≥ 1.10.1** if you enable tunneling — 1.10.1 fixed a tunneling audio-session-id race that crashed at startup ([#3099](https://github.com/androidx/media/issues/3099)).

### Auth on segment URLs — pick the right mechanism

Your auth travels as a **query parameter**, not a header. That distinction decides the API.

**If a single token covers the whole session** and your server accepts it on the manifest and all segments, the simplest correct answer is a header via `setDefaultRequestProperties` — but that only works if you can move auth to a header:

```kotlin
val httpFactory = DefaultHttpDataSource.Factory()
    .setDefaultRequestProperties(mapOf("Authorization" to "Bearer $token"))
    .setUserAgent("LoomarrTV/1.0")
    .setConnectTimeoutMs(15_000)
    .setReadTimeoutMs(15_000)
```

**For a query param — and especially a rotating/expiring one — use `ResolvingDataSource`.** This is the purpose-built hook: it rewrites every `DataSpec` just before the fetch, so segment URLs discovered inside the playlist also get the token, without your server rewriting manifests per user.

```kotlin
val resolver = ResolvingDataSource.Resolver { dataSpec ->
    // Called for EVERY request: manifest, segments, keys.
    // May block — so a cached/refreshed token lookup is legal here.
    val signed = dataSpec.uri.buildUpon()
        .appendQueryParameter("auth", tokenProvider.currentToken())
        .build()
    dataSpec.withUri(signed)
}

val dataSourceFactory = ResolvingDataSource.Factory(
    DefaultHttpDataSource.Factory().setUserAgent("LoomarrTV/1.0"),
    resolver,
)

val player = ExoPlayer.Builder(context)
    .setMediaSourceFactory(
        DefaultMediaSourceFactory(context).setDataSourceFactory(dataSourceFactory)
    )
    .build()
```

The `Resolver` interface has exactly two methods, and the contract difference matters:

```java
DataSpec resolveDataSpec(DataSpec dataSpec) throws IOException;  // MAY block
default Uri resolveReportedUri(Uri uri) { return uri; }          // MUST NOT block
```

- `resolveDataSpec` — may block (so a synchronous token refresh is permitted), called on **every new connection**, so cache aggressively.
- `resolveReportedUri` — used for event reporting and cache keys, and is **not** allowed to block. Use it to strip the token back out so your cache keys and analytics don't fragment per-token.

That last point is a real trap: if the token varies, an unstripped URI means every segment looks like a distinct cache entry.

### Minimal live HLS setup

```kotlin
val player = ExoPlayer.Builder(context)
    .setMediaSourceFactory(
        DefaultMediaSourceFactory(context)
            .setDataSourceFactory(dataSourceFactory)
            .setLiveTargetOffsetMs(8_000)     // tune to your segment length
    )
    .build()

val item = MediaItem.Builder()
    .setUri(channelUrl)
    .setMimeType(MimeTypes.APPLICATION_M3U8)   // required if URL lacks .m3u8
    .setLiveConfiguration(
        MediaItem.LiveConfiguration.Builder()
            .setTargetOffsetMs(8_000)
            .setMinOffsetMs(4_000)
            .setMaxOffsetMs(20_000)
            .setMinPlaybackSpeed(0.97f)
            .setMaxPlaybackSpeed(1.03f)
            .build()
    )
    .build()

player.setMediaItem(item)
player.prepare()
player.playWhenReady = true
```

**Live-edge behaviour.** ExoPlayer nudges playback speed to hold `targetOffsetMs`. Set min/max playback speed to `1.0f` to disable that entirely (some users notice pitch drift). Tune with:

```kotlin
.setLivePlaybackSpeedControl(
    DefaultLivePlaybackSpeedControl.Builder()
        .setFallbackMaxPlaybackSpeed(1.04f)
        .build()
)
```

Useful state: `Player.isCurrentMediaItemLive`, `Player.isCurrentMediaItemDynamic`, `Player.getCurrentLiveOffset()`, `Player.seekToDefaultPosition()` (jump to live edge).

**Mandatory error handling** for a 24/7 channel app — a Shield left paused overnight *will* fall out of the live window:

```kotlin
player.addListener(object : Player.Listener {
    override fun onPlayerError(error: PlaybackException) {
        if (error.errorCode == PlaybackException.ERROR_CODE_BEHIND_LIVE_WINDOW) {
            player.seekToDefaultPosition()
            player.prepare()
        }
    }
})
```

### `#EXT-X-DISCONTINUITY`

This is a **server-side** obligation more than a client one — important for you, since you control the Go server and are splicing filler between programs.

Google's explicit guidance for HLS live streams: **use `#EXT-X-PROGRAM-DATE-TIME`, use `#EXT-X-DISCONTINUITY-SEQUENCE`, and provide a live window of 1+ minute.**

`#EXT-X-DISCONTINUITY-SEQUENCE` is not optional in practice: it is *the only way to safely and seamlessly switch variants* in a live stream containing discontinuities. Without it ExoPlayer falls back to best-effort segment tracking, which is where the "AV freeze at a discontinuity" bug reports come from. **Emit it.**

Client-side, ExoPlayer handles discontinuities natively — but note a codec change across a discontinuity is the fragile case. If your filler and program content differ in codec/resolution, expect a decoder re-init (brief black frame) at each boundary; keeping encode parameters identical across splices is far more reliable than any client setting.

### fMP4 / CMAF specifically

fMP4/CMAF is fully supported by `media3-exoplayer-hls` — no extra module or flag. What it actually requires:

- **`#EXT-X-MAP`** must be present, giving the init segment. Every fMP4 rendition needs it; this is the most common cause of "MPEG-TS works, CMAF doesn't."
- **`CODECS` attribute on every `#EXT-X-STREAM-INF`.** Chunkless preparation is **on by default** in current media3 and depends on it. Without accurate `CODECS`, startup either slows or mis-detects tracks.
- Disable chunkless preparation *only* if you mux closed captions inside segments without declaring them in the multivariant playlist:
  ```kotlin
  HlsMediaSource.Factory(dataSourceFactory).setAllowChunklessPreparation(false)
  ```
  It costs startup latency — prefer declaring caption tracks in the playlist.

Supported HLS containers: MPEG-TS, **fMP4/CMAF**, ADTS (AAC), MP3. Low-Latency HLS (Apple's) is supported; the community/LHLS variant is not.

---

## 4. Codec capability enumeration

### Building the device profile

```kotlin
data class DeviceProfile(
    val h264: Boolean, val hevc8: Boolean, val hevc10: Boolean, val av1: Boolean,
    val maxWidth: Int, val maxHeight: Int, val maxFps: Int,
    val hdr10: Boolean, val dolbyVision: Boolean, val hlg: Boolean,
    val ac3: Boolean, val eac3: Boolean, val trueHd: Boolean, val dts: Boolean,
)

private fun videoSupport(mime: String, w: Int, h: Int, fps: Double): Boolean {
    val list = MediaCodecList(MediaCodecList.ALL_CODECS)   // see trap 1
    return list.codecInfos.any { info ->
        !info.isEncoder &&
            info.supportedTypes.any { it.equals(mime, ignoreCase = true) } &&
            runCatching {
                val vc = info.getCapabilitiesForType(mime).videoCapabilities
                // areSizeAndRateSupported, not isSizeSupported — see trap 2
                vc.areSizeAndRateSupported(w, h, fps)
            }.getOrDefault(false)
    }
}
```

For 10-bit, check the **profile**, not just the MIME type:

```kotlin
private fun hevc10Bit(): Boolean {
    val list = MediaCodecList(MediaCodecList.ALL_CODECS)
    return list.codecInfos.any { info ->
        !info.isEncoder && info.supportedTypes.any {
            it.equals(MediaFormat.MIMETYPE_VIDEO_HEVC, true)
        } && runCatching {
            info.getCapabilitiesForType(MediaFormat.MIMETYPE_VIDEO_HEVC)
                .profileLevels.any { pl ->
                    pl.profile == MediaCodecInfo.CodecProfileLevel.HEVCProfileMain10 ||
                    pl.profile == MediaCodecInfo.CodecProfileLevel.HEVCProfileMain10HDR10 ||
                    pl.profile == MediaCodecInfo.CodecProfileLevel.HEVCProfileMain10HDR10Plus
                }
        }.getOrDefault(false)
    }
}
```

### Traps — this section is the whole point

1. **`REGULAR_CODECS` hides tunneled and secure decoders.** `MediaCodecList(REGULAR_CODECS)` enumerates only buffer-to-buffer codecs. Tunneled-only and secure-only decoders — exactly the ones a TV uses for HDR and DRM — appear **only** under `ALL_CODECS`. Using the wrong constant makes a capable device look incapable.

2. **`isSizeSupported()` is not a performance claim.** It tells you the decoder *accepts* the dimensions, not that it sustains them. Always use **`areSizeAndRateSupported(w, h, fps)`** for a 4K60 question. Even that is a "supported," not "achievable," answer.

3. **"Supported" ≠ "achievable."** Android distinguishes them deliberately:
   - `areSizeAndRateSupported()` — the codec *can* do it, possibly not smoothly.
   - `getAchievableFrameRatesFor()` (API 21+) — measured, realistic rates.
   - **`getSupportedPerformancePoints()` (API 29+) — the strongest guarantee**, a vetted (resolution, frame-rate) combination. Prefer it when present; fall back to `areSizeAndRateSupported` when it returns null. Your Shield is API 30, so performance points are available.

4. **The 10-bit trap.** A decoder advertising `video/hevc` says nothing about bit depth. 8-bit and 10-bit are distinct **profiles**. Devices routinely list HEVC while supporting only `HEVCProfileMain` — you must inspect `profileLevels` as above.

5. **Total capability is not per-codec capability.** A device may claim 4K60 for HEVC *and* 4K60 for AVC yet be unable to run both at once. Concurrency limits are not expressed in these APIs. Relevant if you ever preview one channel while decoding another.

6. **Software decoders masquerade as support.** A software AV1 decoder will happily report 4K support and then play at 2 fps. Filter with `MediaCodecInfo.isHardwareAccelerated()` (API 29+) — and on older paths, the `OMX.google.` / `c2.android.` name prefixes indicate software.

7. **`Display.HdrCapabilities` is deprecated.** `Display.getHdrCapabilities()` / `HdrCapabilities.getSupportedHdrTypes()` are superseded by **`Display.getMode().getSupportedHdrTypes()`** (API 34+). Since the Shield is API 30, you need the legacy call there — write both paths, gated on `Build.VERSION.SDK_INT`.

8. **HDR needs the display, not just the decoder.** A Main10 decoder on an SDR panel gets you tone-mapped or flattened output. Check codec **and** display. AOSP notes only *tunneled* decoders guarantee HDR playback; non-tunneled paths may lose HDR metadata and flatten to SDR.

9. **Let ExoPlayer decide when you can.** `MediaCodecUtil.getDecoderInfos()` already encodes years of device quirks. Use raw `MediaCodecList` to *report* a profile to your server; use ExoPlayer's own selection to *play*. Duplicating its logic is how you reintroduce bugs it already fixed.

### Audio passthrough — a genuinely different API

Passthrough capability is **not** in `MediaCodecList`; it is a property of the current audio route.

**API 33+ (correct modern path):**

```kotlin
val attrs = AudioAttributes.Builder()
    .setUsage(AudioAttributes.USAGE_MEDIA)
    .setContentType(AudioAttributes.CONTENT_TYPE_MOVIE)
    .build()

val format = AudioFormat.Builder()
    .setEncoding(AudioFormat.ENCODING_E_AC3)
    .setChannelMask(AudioFormat.CHANNEL_OUT_5POINT1)
    .setSampleRate(48_000)
    .build()

val supported = audioManager.getDirectPlaybackSupport(format, attrs) !=
    AudioManager.DIRECT_PLAYBACK_NOT_SUPPORTED
```

Or enumerate: `audioManager.getDirectProfilesForAttributes(attrs)` → `List<AudioProfile>`.

**API 29–32 (what the Shield actually runs):** `AudioTrack.isDirectPlaybackSupported(format, attrs)` — now **deprecated** in favour of the above, but it's your only option on API 30.

> ⚠️ **The pre-33 false-positive trap.** Before API 33, `isDirectPlaybackSupported()` considers *all* audio paths including disconnected ones. If HDMI supports E-AC3 but audio is currently routed to Bluetooth, it returns `true` and playback then fails. On an API-30 Shield this is your default reality — treat its answer as "the device could, somewhere," not "the current route will."

Encodings to probe: `ENCODING_AC3`, `ENCODING_E_AC3`, `ENCODING_E_AC3_JOC` (Atmos), `ENCODING_DOLBY_TRUEHD`, `ENCODING_DTS`, `ENCODING_DTS_HD`.

**Capabilities change at runtime.** The user unplugs HDMI, switches AVR input, connects Bluetooth headphones. Register an `AudioDeviceCallback` (API 23+) or `AudioRouting.OnRoutingChangedListener` (API 24+) and re-probe. ExoPlayer does this internally via `AudioCapabilitiesReceiver`; if you maintain your own profile, you must too.

---

## 5. NVIDIA Shield specifics

### Software

| | Value |
|---|---|
| Shield Experience | **9.2.4** (2026-02-25), incl. Jan 2026 security patch |
| Android base | **Android 11 → API 30** |
| Last OS bump | Dec 2021 (Experience 9.0). No Android 12+ ever shipped |
| Widevine | L1 |

NVIDIA says it will keep supporting the Shield, but has **never committed to Android 12+**. Plan for API 30 permanently.

> ⚠️ **Deadline, two weeks out:** Google Play requires Android TV apps to target **API 33+ by 2026-08-31** (extension to Nov 1). That's a `targetSdk` requirement; `minSdk` stays ≤ 30 and you must verify every API-33 behaviour change against the Android 11 runtime. Irrelevant if you only ever sideload — relevant the moment you consider Play distribution.

### Hardware — correcting a common premise

Both 2019 models use the **same Tegra X1+ SoC**. "X1 vs X1+" distinguishes the 2015/2017 generation from 2019, *not* tube from Pro. **Decode capability is identical**; only memory and I/O differ.

| | Shield TV tube (P3430) | Shield TV Pro (P2897) |
|---|---|---|
| SoC | Tegra X1+, 256-core Maxwell | identical |
| RAM | **2 GB** | **3 GB** |
| Storage | 8 GB + microSD | 16 GB, **no** microSD |
| USB | **none** | 2× USB 3.0 |

**Budget against the 2 GB tube.** After system overhead, assume a per-app heap in the low hundreds of MB. Trim `DefaultLoadControl` buffers from their 50 s defaults for 4K, and cap bitmap caches hard.

### Decoders (NVIDIA official spec page)

- **HEVC/H.265: 4K HDR @ 60 fps** — the only 4K HDR path
- **H.264, VP9, VP8, MPEG1/2: 4K @ 60 fps**
- **HDR: Dolby Vision + HDR10**
- **AV1: NOT SUPPORTED.** Absent from spec pages; Tegra X1+ has no AV1 block and there is no usable software path. **Never offer AV1.**
- **VP9 Profile 2 (10-bit): not supported** — VP9 is 8-bit/SDR only (this is why YouTube 4K HDR never worked on Shield).
- **HLG: sources disagree.** NVIDIA lists only Dolby Vision and HDR10. Verify on-device before shipping an HLG path.

**Audio (official):** Dolby Digital, DD+, Atmos, TrueHD passthrough, DTS-X/DTS-HD passthrough, up to 24-bit/192 kHz; Auro-3D added in 9.2.

### ExoPlayer quirks — verified in source

**media3 applies an ungated NVIDIA workaround to your device.** I confirmed this directly in the current `release` branch of `MediaCodecVideoRenderer.java`:

```java
private static boolean deviceNeedsNoPostProcessWorkaround() {
    if (!MediaLibraryInfo.enableWorkarounds()) return false;
    // Nvidia devices ... may apply post processing operations that can modify
    // frame output timestamps, which is incompatible with ExoPlayer's logic
    // for skipping decode-only frames.
    return "NVIDIA".equals(Build.MANUFACTURER);
}
```

When true, media3 sets `no-post-process = 1` and `auto-frc = 0` on the `MediaFormat`. Despite the comment's "prior to M" phrasing, the check has **no `SDK_INT` gate** — it fires on every NVIDIA device including your API-30 Shields, disabling NVIDIA's automatic frame-rate conversion. Expect ExoPlayer to suppress the Shield's own frame-rate adaptation; if you want frame-rate matching, drive it yourself via `setVideoChangeFrameRateStrategy`.

*(Note: an older, separate `auto-frc` workaround gated to the 2015 "foster" tablet at SDK ≤ 22 also exists in legacy ExoPlayer and does **not** apply here. The one above is current, ungated, and does.)*

**Frame-rate matching vs. audio passthrough — the most consequential Shield bug.** [androidx/media #2258](https://github.com/androidx/media/issues/2258): on Shield TV Pro 2019, with frame-rate matching enabled, `AudioCapabilities` reports multi-channel PCM instead of Dolby ~90% of the time; passthrough works ~10%. Disabling frame-rate matching restores it. **Still open.** Effectively you must choose one. If passthrough matters, default frame-rate matching off, or re-query audio capabilities after the display mode settles.

**Other open issues worth knowing:**
- [#2185](https://github.com/androidx/media/issues/2185) — Android 11, secure HEVC→AVC decoder switch gives black screen ("Failed to connect to surface"). media3 1.3.1 dropped a retry-after-50 ms workaround that ExoPlayer 2.14.1 had.
- [ExoPlayer #8041](https://github.com/google/ExoPlayer/issues/8041) — `OMX.Nvidia.h264.decode` init failure when moving a player between views. Workaround: remove and re-add the `PlayerView` to its parent.
- Tunneling: [#1574](https://github.com/androidx/media/issues/1574) (seek-to-end hangs), [#2541](https://github.com/androidx/media/issues/2541) (video-only tunneling unsupported).
- DTS-HD MA passthrough regressions are recurrent downstream ([jellyfin #5168](https://github.com/jellyfin/jellyfin-androidtv/issues/5168)).

Tunneling, if you want it: `DefaultTrackSelector.buildUponParameters().setTunnelingEnabled(true)` (the older `setTunnelingAudioSessionId` is superseded).

---

## 6. Auth on a TV (RFC 8628)

### The wire contract

**Device authorization endpoint** → JSON response:

| Field | Status | Meaning |
|---|---|---|
| `device_code` | REQUIRED | never shown to the user |
| `user_code` | REQUIRED | displayed on the TV |
| `verification_uri` | REQUIRED | where the user goes |
| `verification_uri_complete` | OPTIONAL | code embedded — **this is your QR payload** |
| `expires_in` | REQUIRED | lifetime, seconds |
| `interval` | OPTIONAL | min poll gap; **defaults to 5** if absent |

**Token endpoint:** `grant_type=urn:ietf:params:oauth:grant-type:device_code` (exact string) + `device_code` + `client_id`.

**Polling errors — the semantics people get wrong:**

| Error | Client behaviour |
|---|---|
| `authorization_pending` | keep polling; wait ≥ `interval` between attempts |
| `slow_down` | keep polling, but **permanently increase interval by 5 s**, cumulatively — not a one-off backoff, and **not terminal** |
| `access_denied` | **stop.** user refused |
| `expired_token` | **stop.** restart with a fresh code |

Only `access_denied` and `expired_token` terminate.

### Implementing it on a self-hosted Go server

**User code format (RFC §6.1) — the spec is unusually concrete:**
- Charset **`BCDFGHJKLMNPQRSTVWXZ`** — A–Z minus vowels (no accidental profanity) and minus lookalikes (no transcription errors).
- Shape `WDJB-MJHT`: 8 significant chars, dash for readability, ≈**34.5 bits**.
- **Normalize on input**: strip dashes, uppercase, drop out-of-charset characters. Don't reject — normalize.

**Rate limiting is the actual security control**, not entropy. The RFC's own worked example holds only because the server limits to ~5 attempts. An unthrottled verification endpoint on a self-hosted box is brute-forceable regardless of code length. Limit per-IP *and* per-code, and expire in 5–15 minutes.

**Device code:** no usability constraint, so use 128+ bits from a CSPRNG, and store it hashed like a session token.

**TLS.** Apps targeting API 24+ **do not trust user-added CAs** — "install the cert on the Shield" will not work. Either get a real cert (Let's Encrypt via DNS-01 on a domain pointing at a LAN IP — no app-side config, recommended), or bundle your CA:

```xml
<!-- res/xml/network_security_config.xml -->
<network-security-config>
    <domain-config>
        <domain includeSubdomains="true">loomarr.example.com</domain>
        <trust-anchors><certificates src="@raw/my_ca"/></trust-anchors>
    </domain-config>
</network-security-config>
```

The PEM must contain only PEM data, and Certificate Transparency is **not** verified on custom trust anchors. Don't re-enable global cleartext to dodge this — you'd be shipping bearer tokens in the clear across the LAN.

> ⚠️ **`androidx.security:security-crypto` is deprecated** as of 1.1.0 (2025-07-30): *"Deprecated all APIs in favour of existing platform APIs and direct use of Android Keystore."* `EncryptedSharedPreferences`, `EncryptedFile`, and `MasterKey` are all included. Most token-storage tutorials online are now stale. **Use DataStore for storage plus a Keystore-held `AES/GCM/NoPadding` key you manage.** On a TV, do **not** set `setUserAuthenticationRequired(true)` — TVs often have no enrolled credential and key generation fails.

### A simpler alternative

Since you control both ends, **Jellyfin-style Quick Connect** is less machinery: the TV shows a 6-char code; the user approves it from an *already-authenticated* client; the TV polls and gets a token. No browser redirect, no `client_id`, no consent screen.

RFC 8628 costs a little more but buys a documented contract, a defined error vocabulary, and off-the-shelf clients. Either way, steal the RFC's charset and rate-limiting posture — those are the security-relevant parts. Display a QR (`verification_uri_complete`) **and** the human-readable code+URL; a QR is useless when the phone is across the room.

---

## 7. Build / CI

### Toolchain (2026-08)

| Component | Version |
|---|---|
| AGP | **9.3.0** (Jul 2026) |
| Gradle | **9.5.0** (min *and* default for AGP 9.3) |
| JDK | **17** (min and default) |
| Build Tools | 36.0.0 |
| Max API | 37 |

Pin JDK 17 in CI — it matches AGP's default and avoids local/CI drift.

### Workflow

```yaml
name: Build TV APK
on: [push, pull_request]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v5

      - uses: actions/setup-java@v5
        with:
          distribution: temurin
          java-version: '17'
          # Do NOT also set `cache: gradle` — setup-gradle owns caching.

      - uses: android-actions/setup-android@v4

      - uses: gradle/actions/setup-gradle@v6
        with:
          cache-read-only: ${{ github.ref != 'refs/heads/main' }}

      - run: ./gradlew assembleRelease

      - uses: actions/upload-artifact@v4
        with:
          name: app-release
          path: app/build/outputs/apk/release/*.apk
```

> ⚠️ **`gradle/gradle-build-action` is deprecated** — it now just delegates to `gradle/actions/setup-gradle`. Use `setup-gradle@v6`, and do **not** stack a second cache layer (`cache: gradle` on setup-java, or a hand-rolled `actions/cache` on `~/.gradle`); two layers fighting over one directory is a common misconfiguration.

### Signing for sideload

Base64 keystore in secrets remains the standard; Play App Signing is Play-only and irrelevant here.

```bash
keytool -genkeypair -v -keystore release.jks -keyalg RSA -keysize 4096 \
  -validity 10000 -alias loomarr
base64 -w0 release.jks     # macOS: base64 -i release.jks
```

Secrets: `KEYSTORE_BASE64`, `KEYSTORE_PASSWORD`, `KEY_ALIAS`, `KEY_PASSWORD`.

```yaml
      - name: Decode keystore
        env:
          KEYSTORE_BASE64: ${{ secrets.KEYSTORE_BASE64 }}
        run: echo "$KEYSTORE_BASE64" | base64 -d > "${{ github.workspace }}/release.jks"
```

```kotlin
android {
    signingConfigs {
        create("release") {
            System.getenv("KEYSTORE_FILE")?.let {
                storeFile = file(it)
                storePassword = System.getenv("KEYSTORE_PASSWORD")
                keyAlias = System.getenv("KEY_ALIAS")
                keyPassword = System.getenv("KEY_PASSWORD")
            }
        }
    }
    buildTypes {
        release {
            signingConfig = signingConfigs.getByName("release")
                .takeIf { System.getenv("KEYSTORE_FILE") != null }
            isMinifyEnabled = true     // R8 — also required for realistic perf testing
        }
    }
}
```

Pass the secret via an **env var**, not inlined into the shell command (keeps it out of process args). Losing this keystore means users must uninstall/reinstall to update.

### TV-specific CI notes

**Build the APK, not the AAB.** `bundleRelease` produces an `.aab`, which is a **Play-distribution format the Shield cannot install**. For `adb install`, you want `assembleRelease`.

**No emulator needed** for `assembleRelease` + unit tests. If you want instrumented tests, `reactivecircus/android-emulator-runner@v2` supports `target: android-tv`, and on `ubuntu-latest` the KVM udev rule is required or the emulator crawls:

```yaml
      - name: Enable KVM
        run: |
          echo 'KERNEL=="kvm", GROUP="kvm", MODE="0666", OPTIONS+="static_node=kvm"' \
            | sudo tee /etc/udev/rules.d/99-kvm4all.rules
          sudo udevadm control --reload-rules
          sudo udevadm trigger --name-match=kvm
```

**Manifest — this is what makes it a TV app:**

```xml
<uses-feature android:name="android.software.leanback" android:required="true" />
<uses-feature android:name="android.hardware.touchscreen" android:required="false" />
```

`touchscreen` **must** be `required="false"` or the app is filtered off TV devices. The launcher activity needs `android.intent.category.LEANBACK_LAUNCHER` — omit it and the APK installs fine and is simply **invisible** on the home screen, a nasty silent failure.

Add a **Baseline Profile** generation step once the guide grid exists; it's the cheapest real win for scroll smoothness on the tube.

---

## Traps — consolidated

**Will bite you immediately**
1. `TvLazyRow`/`TvLazyColumn` **no longer exist** — most tutorials are dead code.
2. `LEANBACK_LAUNCHER` missing → app installs and is invisible.
3. `bundleRelease` → `.aab` the Shield **cannot install**. Use `assembleRelease`.
4. `touchscreen` not `required="false"` → app filtered off TVs.
5. Nesting `LazyRow` in `LazyColumn` outside `item {}` → infinite-constraint crash.

**Performance illusions**
6. Benchmarking a **debug** build — Compose debug is far slower; measure release + R8 only.
7. No Baseline Profile → materially worse scroll and startup on the 2 GB tube.
8. Work inside item lambdas re-running per scroll frame.
9. Missing/unstable `key`s in `items()` → focus drops when EPG data refreshes.

**Codec/capability**
10. `REGULAR_CODECS` **hides tunneled and secure decoders** — use `ALL_CODECS` to enumerate.
11. `isSizeSupported()` ≠ 4K60. Use `areSizeAndRateSupported()`, better `getSupportedPerformancePoints()` (API 29+).
12. `video/hevc` says nothing about **10-bit** — inspect `profileLevels` for Main10.
13. Software decoders report support then play at 2 fps — filter on `isHardwareAccelerated()`.
14. `Display.getHdrCapabilities()` **deprecated** → `Display.getMode().getSupportedHdrTypes()` (API 34+); Shield needs the legacy path.
15. Per-codec capability ≠ **concurrent** capability.

**Audio**
16. Pre-API-33 `isDirectPlaybackSupported()` reports on **disconnected routes too** — false positives on your API-30 Shield.
17. `AudioTrack.isDirectPlaybackSupported` deprecated → `AudioManager.getDirectPlaybackSupport` (API 33+), unavailable on Shield.
18. Capabilities change at runtime (HDMI/BT/AVR) — subscribe and re-probe.

**Shield-specific**
19. **AV1 will never play.** VP9 is 8-bit only. HDR must be HEVC Main10.
20. **Frame-rate matching breaks Dolby passthrough ~90% of the time** ([#2258](https://github.com/androidx/media/issues/2258), open) — pick one.
21. media3 sets `no-post-process=1`/`auto-frc=0` on **all** NVIDIA devices, ungated by SDK.
22. Budget memory for the **2 GB tube**; trim `DefaultLoadControl` for 4K.

**Streaming / server-side**
23. Omitting `#EXT-X-DISCONTINUITY-SEQUENCE` → variant-switch failures and freezes at splices. **Emit it**, plus `#EXT-X-PROGRAM-DATE-TIME`.
24. fMP4 without `#EXT-X-MAP` → CMAF fails while MPEG-TS works.
25. Missing/wrong `CODECS` on `#EXT-X-STREAM-INF` → chunkless preparation (default **on**) mis-detects tracks.
26. Not handling `ERROR_CODE_BEHIND_LIVE_WINDOW` → a paused-overnight Shield never recovers.
27. Not stripping the auth token in `resolveReportedUri` → cache keys fragment per token.
28. Codec/resolution changes across a splice → decoder re-init and a black frame. Keep encode params identical.

**Auth / storage**
29. `slow_down` is **cumulative, permanent, and non-terminal** — not exponential backoff.
30. `androidx.security:security-crypto` **fully deprecated** — `EncryptedSharedPreferences` guidance online is stale.
31. User-added CAs **not trusted** on API 24+ — bundle a CA via network security config, or use a real cert.
32. `setUserAuthenticationRequired(true)` fails on TVs with no enrolled credential.

---

## Uncertainty and disagreements

**Sources disagree**
- **HLG on Shield.** NVIDIA lists only Dolby Vision and HDR10; community sources historically said HLG renders as SDR, while some 2026 reviews claim support. **Unresolved — probe on-device.**
- **`tv-material` version.** The guide page says 1.0.0; release notes say 1.1.0 stable. Release notes win.
- **NVIDIA workaround in media3.** A secondary search suggested media3 has no NVIDIA workarounds. I verified in the current `release` branch that it does, ungated — the source is authoritative over the search result.

**Uncertain / verify before relying**
- **Shield API level 30** is inferred from "Android 11"; NVIDIA never states an API level. Safe inference, but unstated.
- **AGP 9.3.0 / Gradle 9.5.0** as latest stable moves fast; the JDK 17 floor is stabler.
- Dolby Vision **profile** support (5/8 yes, 7 single-layer only) is community-established, not NVIDIA-published.
- `BringIntoViewSpec` remains `@ExperimentalFoundationApi` — expect churn; isolate it.
- Compose BOM 2026.08.00 requires **compileSdk 37 / AGP ≥ 9.1.1**; BOM 2026.04.01 (core 1.11.0) is the conservative pin.
- I did not benchmark the guide grid on real hardware. The 100×4h sizing above is reasoned from virtualization behaviour and Compose guidance, **not measured on a Shield** — validate with a Macrobenchmark scroll test before committing to a design.

---

## Sources

**Compose / TV UI** — [androidx.tv releases](https://developer.android.com/jetpack/androidx/releases/tv) · [Compose on Android TV](https://developer.android.com/training/tv/playback/compose) · [Scrollable layouts for TV](https://developer.android.com/training/tv/playback/compose/lists) · [Change focus behavior](https://developer.android.com/develop/ui/compose/touch-input/focus/change-focus-behavior) · [Compose performance](https://developer.android.com/develop/ui/compose/performance) · [Baseline Profiles](https://developer.android.com/topic/performance/baselineprofiles/overview) · [JetStream TV sample](https://github.com/android/tv-samples/blob/main/JetStreamCompose/baseline-profiles.md) · [Compose BOM](https://developer.android.com/develop/ui/compose/bom)

**Media3** — [media3 releases](https://developer.android.com/jetpack/androidx/releases/media3) · [HLS](https://developer.android.com/media/media3/exoplayer/hls) · [Live streaming](https://developer.android.com/media/media3/exoplayer/live-streaming) · [Network stacks](https://developer.android.com/media/media3/exoplayer/network-stacks) · [ResolvingDataSource source](https://github.com/androidx/media/blob/release/libraries/datasource/src/main/java/androidx/media3/datasource/ResolvingDataSource.java) · [MediaCodecVideoRenderer source](https://github.com/androidx/media/blob/release/libraries/exoplayer/src/main/java/androidx/media3/exoplayer/video/MediaCodecVideoRenderer.java)

**Codec / audio** — [MediaCodecInfo.VideoCapabilities](https://developer.android.com/reference/android/media/MediaCodecInfo.VideoCapabilities) · [CodecCapabilities](https://developer.android.com/reference/android/media/MediaCodecInfo.CodecCapabilities) · [TV audio capabilities](https://developer.android.com/training/tv/playback/audio-capabilities) · [AOSP HDR playback](https://source.android.com/docs/core/display/hdr)

**Shield** — [Shield TV Pro specs](https://www.nvidia.com/en-us/shield/shield-tv-pro/) · [Software update 9.2.4](https://www.nvidia.com/en-us/shield/software-update/) · [Experience 9.2 notes](https://support-shield.nvidia.com/android-tv-release-notes/9.2/) · [media #2258](https://github.com/androidx/media/issues/2258) · [#2185](https://github.com/androidx/media/issues/2185) · [ExoPlayer #8041](https://github.com/google/ExoPlayer/issues/8041)

**Auth / CI** — [RFC 8628](https://datatracker.ietf.org/doc/html/rfc8628) · [Google limited-input OAuth](https://developers.google.com/identity/protocols/oauth2/limited-input-device) · [androidx.security releases](https://developer.android.com/jetpack/androidx/releases/security) · [Network security config](https://developer.android.com/privacy-and-security/security-config) · [AGP releases](https://developer.android.com/build/releases/gradle-plugin) · [gradle/actions](https://github.com/gradle/actions) · [android-emulator-runner](https://github.com/ReactiveCircus/android-emulator-runner) · [Play target API](https://developer.android.com/google/play/requirements/target-sdk) · [Jellyfin Quick Connect](https://jellyfin.org/docs/general/server/quick-connect/)
