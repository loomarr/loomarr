# Loomarr system design

**Status:** Being split into [`docs/design/`](design/README.md) (#779), one subsystem at a time.
A section that has moved is replaced by a pointer; every section still here remains the living
source of truth. Amend it in the same PR before behavior changes.

**Audience:** Builders and coding agents. User-facing instructions live under
[Get started](get-started.md) and [`docs/guides/`](guides/install-docker.md).

Read the section your task cites rather than loading this document end to end. `CONTEXT.md` owns
vocabulary, `PROGRESS.md` owns work status, and the companion design documents own programming,
configuration, and frontend detail. This document wins when behavior overlaps.

---

## 1. Purpose

Moved to [`design/overview.md`](design/overview.md#purpose).

## 2. Architecture

Moved to [`design/overview.md`](design/overview.md#architecture); the generated package map is
[`design/package-map.md`](design/package-map.md).

---

## 3. Provisioner domain model

Moved to [`design/acquisition.md`](design/acquisition.md#titles-and-keys).

## 4. Provisioning state machine

Moved to [`design/acquisition.md`](design/acquisition.md#state-machine).

## 5. Persistence — Postgres **and** SQLite

The store, retention, PostgreSQL concurrency and the SQLite → PostgreSQL migration moved to
[`design/storage.md`](design/storage.md); the media inventory and cached series episodes moved to
[`design/library.md`](design/library.md); airing history moved to
[`design/scheduling.md`](design/scheduling.md#airing-history); the activity and diagnostic evidence
tables moved to [`design/observability.md`](design/observability.md).

---

## 6. External contracts

Client resilience rules and the requesters moved to [`design/acquisition.md`](design/acquisition.md);
the Emby and Jellyfin contract moved to [`design/library.md`](design/library.md); the Tunarr
programmer and Live TV wiring moved to [`design/playout.md`](design/playout.md#tunarr).

---

## 7. HTTP API

The API reference is the OpenAPI spec (`api/openapi.yaml`, served at `/docs`); the hand-kept route
table that stood here was deleted in #779. §7.1's rules moved to
[`design/overview.md`](design/overview.md#http-api); route authorization is §11.

### 7.2 Search (federated, no index)

Moved to [`design/library.md`](design/library.md#search).

---

## 8. Suggester (AI suggestion engine)

Grounding, reference-backed Intent, providers, §8.1 model selection and §8.2 model residency moved
to [`design/suggester.md`](design/suggester.md). The specialized local model experiment is archived
in [`engineering/archive/design-2026-09/`](engineering/archive/design-2026-09/README.md). Approval,
Proposal execution and the decision traces moved to
[`design/proposal-workflow.md`](design/proposal-workflow.md).

---

## 9. Scheduler / lineup builder — *the point of the app*

Moved to [`design/scheduling.md`](design/scheduling.md), including guide freshness, backfill and
"what does not change" between playout backends.

---

## 9.1 Playout backends — *Loomarr serves its own streams*

The channel packager, backend selection, what internal playout serves, admission and playout status
moved to [`design/playout.md`](design/playout.md). The founding reversal is decision
[0009](design/decisions/0009-loomarr-plays-out.md). Tuning, pause and the Android TV client moved to
[`design/playback-clients.md`](design/playback-clients.md). The two sections below are release and
testing mechanics bound for `docs/dev/` (#1572).

### Browser and real-runtime certification is layered (V58)

V57 proves the controller contract against deterministic browser-owned HLS bytes. It does not claim
that a real Loomarr process can package those bytes on every development host, that Linux Playwright
WebKit is Safari, or that 100 surfable Channels means 100 simultaneous encoders. V58 keeps those
claims separate so one green test cannot silently stand in for another.

The **controller matrix** runs the same 100-Channel catalog through Playwright Chromium, Firefox,
and WebKit. Every engine must preserve latest-request-wins, one video element, exact warmed-URL reuse,
still-only adjacent warming, and a genuinely decoded H.264 frame. Chromium and Firefox enforce
the absolute media first-frame budgets per engine rather than pooling their samples. Playwright
WebKit records those two percentiles as diagnostics while retaining every correctness, OSD, manifest,
and raw-runner gate: it cannot exercise branded Safari's native-HLS route and instead measures the
non-shipping hls.js fallback. The shipping-browser soak enforces the same **1.5 s** arbitrary and
**750 ms** adjacent budgets on real Safari; a fast Chromium sample cannot hide a slow shipping browser.
Each engine must complete one bounded cold decode before the surf samples begin; those samples measure
an already-running tuner, while the real-runtime gate and shipping-browser soak own cold boot timing.
The matrix also runs a raw MediaSource control with the same representative bytes and the same one
persistent video element: two unmeasured decoder-startup replacements followed by twenty steady-state
replacements whose nearest-rank p95 must be below 500 ms. Five observations make p95 merely the
maximum and cannot distinguish an isolated runner scheduling interruption. A fresh element per sample is not equivalent; it hides
the replacement lifecycle this gate exists to measure. This is a runner-validity signal only; it
never normalizes, subtracts from, or changes the product budgets above. Playback join is judged from
media-event timestamps captured inside the
browser. Playwright may wait longer to collect those timestamps, because driver polling latency is
not playback latency, but the recorded frame-to-`playing` interval must still satisfy its 250 ms
bound. Likewise, arbitrary navigation starts its clock inside the browser's trusted input handler;
automation-driver delivery time is not work performed by the product.
The controller matrix runs natively as a dedicated serial macOS CI job rather than after visual and
wizard suites on a reused Linux runner. It retains zero retries and absolute Chromium/Firefox product
thresholds; isolation removes unrelated browser lifecycle and CPU pressure, while macOS exercises the
closest available WebKit compatibility target. Playwright WebKit remains correctness evidence rather
than Safari performance certification; only a run on shipping Safari may certify Safari's native-HLS
latency budgets.

The **real-runtime gate** starts the real composition root over an isolated SQLite store, the real
channel packager and HLS origin, and real ffmpeg/ffprobe. Only true external systems (the media-server
API and its library) may be test doubles, and they serve pinned representative media rather than
prebuilt HLS responses. The browser must bootstrap/authenticate through the real API, tune a real
Channel, receive an HLS manifest produced by Loomarr, and report a decoded frame. A process restart
then repeats the tune, proving a cold boot rather than only a warm in-process packager. A secondary worktree overrides `server.public_url` to its own
isolated backend after sourcing shared integration credentials; otherwise the parent ffmpeg re-opens
the primary port, emits zero bytes, and every HLS request hides that routing error behind its
45-second readiness timeout. Missing and corrupt representative inputs must reach the designed
offline/retry state instead of an unexplained black frame.

The **shipping-browser and hardware soak** is maintainer-run evidence: current Chrome, Firefox, and
Safari against the isolated Loomarr runtime, with representative H.264, HEVC/10-bit, multichannel
audio, and corrupt/missing inputs while GPU capacity is contended. It records boot-to-ready and
request-to-first-decoded-frame timings plus the channel's format and pipeline. It never drives the
maintainer's normal database or media-server configuration, and no agent invokes the `make smoke*`
targets. Android TV, Roku, and Apple TV remain later adapters over the same controller vocabulary.

### Android TV distribution uses one permanent React Native identity

`loomarr.media` is the permanent production application id for the accepted React Native Shield
replacement. Ordinary development and Storybook builds retain the isolated prototype identity;
only an explicit Shield release configuration may select the production id, application name,
launcher icon, and TV banner. Both the sideload and Play configurations use that release identity
and fail closed unless they receive a supported SemVer name and its valid derived Android version
code.

Shield client releases use SemVer names and a deterministic, increasing `versionCode`. The code
allocates two decimal digits each to minor and patch and four release slots within a patch:
`major * 100000000 + minor * 1000000 + patch * 10000 + channel`, where `beta.N` occupies 1–7999,
`rc.N` occupies 8001–8999, and the stable release is 9999. Major is bounded to 20 so every result
stays below Android's version-code ceiling. The build derives the code from the version name; an
operator does not type two independent identities that can drift.

The sideload artifact is a signed APK containing the production React Native entry and only the
`arm64-v8a` native libraries required by the Shield. The Play producer compiles one unsigned Android
App Bundle from the same React Native TV source, the exact merge-result commit, and a
source-controlled release identity. It contains `armeabi-v7a`, `arm64-v8a`, `x86`, and
`x86_64`; every packaged 64-bit ELF LOAD segment is aligned for 16 KiB pages. Android's 16 KiB
devices are 64-bit, so the required `arm64-v8a` and `x86_64` libraries carry that alignment while
the separately required 32-bit TV ABIs retain their platform alignment. CI verifies package, name,
code, launcher activity, TV launcher metadata, icon/banner resources, embedded startup identity,
JavaScript bundle, ABI set, and the unsigned artifact digest, then retains that bundle with evidence
bound to the exact workflow run and commit. Before release dispatch, the maintainer's compile-free
emulator harness verifies the same digest, installs device-specific splits, and supplies the visible
clean-install, discovery, manual fallback, startup-animation, pairing, playback, and playbar evidence
that archive inspection cannot. The protected Internal-release job downloads that immutable artifact
by id, rejects missing/expired/ambiguous provenance, digest drift, any pre-existing signature, and
unexpected `META-INF` material, signs it with the durable upload key using the pinned JDK, proves
every non-signature ZIP entry is unchanged, and re-runs the certificate-bound verifier before
optional publication. It performs no Gradle, CMake, Expo prebuild, Node installation, or Apple build.
There is no rebuild fallback and no name-only/latest-artifact selection. The sideload path still
requires all four keystore inputs and records the same applicable artifact evidence. Local release
tests create ephemeral signing material. The sideload test
also cleanly uninstalls any prior `loomarr.media` package from a Loomarr-owned Android TV emulator,
installs the APK, and cold-launches the Leanback activity.

Android build performance (#1050) is measured without changing the artifact contract. The
`android-profile` Make target runs the normal four-ABI Android gate, retaining runner identity,
wall time, actual Gradle settings and local `--profile` reports in a separate diagnostic artifact.
It never uses an externally uploaded build scan or adds diagnostic files to the unsigned promotion
artifact. Local builds default to one native worker and one Gradle worker. CI runs at most two
Gradle projects in parallel while retaining one native compiler/link slot inside each task; the
wrapper rejects any Gradle worker count other than one or two. The bounded hosted experiment cut
fresh-source builds from the 30m56s one-worker cold control to 19m13s and 16m15s with zero OOM event
deltas, all four ABIs, and the same verified artifact. A measured three-worker candidate regressed to
19m07s and is rejected. Release continues to promote the already verified producer artifact.

The TV application does not import Reanimated or Worklets and therefore must not declare either as
a direct dependency. Expo autolinking treats a direct declaration as native application authority:
the otherwise-unused modules added 9m39s and 3m29s respectively to a measured warm four-ABI build,
and 832 precompiled-header compiler calls bypassed ccache. The workspace compatibility overrides
still pin their exact Expo-supported versions for transitive development tooling. The Android gate
verifies the generated TV graph and complete artifact rather than shipping unused native modules as
a compatibility precaution.

The mobile application likewise does not import Reanimated or Worklets. Expo Router retains them as
transitive optional peers, and SDK 54 or newer searches transitive React Native dependencies, so
removing only the direct manifest entries does not remove them from the generated native graph. The
mobile manifest must both omit the unused direct dependencies and exclude the two package names from
**Android** autolinking. The exclusion is application-scoped and platform-scoped rather than a
workspace package removal: exact workspace overrides keep the supported versions available to
Storybook and other transitive tooling. Apple excludes only Reanimated, while Gesture Handler can
still satisfy its conditional `RNWorklets` dependency. The standalone embedded mobile APK remains
the Android native acceptance artifact.

The producer may additionally use the §14-pinned ccache executable through the generated Expo/CMake
plugin. CI requires an absolute verified launcher, content-based compiler identity, a checkout-relative
base directory, no permissive sloppiness, and a bounded dedicated cache directory. Local release tests
acquire the same exact macOS or Linux pin into the worktree artifacts by default, reuse compiler results
on later builds, and retain an explicit or acquisition-failure cold path. Only compiler results are
restored across source identities. Generated Android projects, `.cxx` trees, bundles, keys, and
promotion evidence are never cached. The profiler retains exact version/configuration, zeroed pre/post
JSON statistics, and every primary generated Ninja rules file used to prove launcher propagation
across app and library projects. Nested compiler-capability probes are not application/library rules
and do not participate in that proof. Pull-request and merge-queue refs restore compiler objects from
the default-branch cache but cannot publish into that shared scope. After a successful Android
merge-queue build lands, that producer transfers only its bounded ccache directory plus a
commit/run/workflow/key/tree-digest manifest. A trusted `push` workflow on the exact admitted main
commit validates the successful merge-group run, immutable transfer, and manifest before publishing
one rolling default-branch cache generation. It performs no Gradle, CMake, Expo, Node, or product
build, deletes the one-day transfer, and retires superseded Android main-cache generations.

On Linux, the observer records its inherited cgroup v2 memory scope, limits, lifetime peak and
OOM/limit event counters before and after the build, plus sampled current usage and host available
memory. The lifetime peak is an upper bound for that scope, not a reset or isolated phase peak;
sampled peaks can miss short spikes. Unavailable metrics remain explicit and cannot qualify a
memory-safety claim. A single process's RSS is not aggregate compiler/Gradle memory. Observation does
not change cgroup limits, build concurrency, JVM heap, caches, ABI scope or artifact checks.

The accepted replacement is installed on the maintainer's Shield by removing the Kotlin application,
sideloading the React Native APK, and pairing again. That physical journey has been accepted. The
same permanent package now also has an Internal-testing-only Google Play path: Google manages the
app-signing key, Loomarr protects a durable upload key in the reviewed GitHub environment, the first
bundle may be uploaded manually for Console bootstrap, and later uploads use a package-scoped service
account with no Production permission. The workflow has no open, closed, staged, or Production track
choice. Because the accepted sideload used an intentionally ephemeral key, a Play install may require
one more uninstall and fresh pairing; cross-channel signature continuity is not promised.

Kotlin/Compose source, Gradle build files, generated Kotlin tokens, JVM screenshot references, and
their dedicated CI lane are deleted only after the React Native sideload acceptance and React Native
Play bundle verification exist in the same ancestry. Distribution-neutral store descriptions and
artwork remain generated from the shared brand contract outside the retired Kotlin tree. Preserving
installed credentials, public Play distribution, staged rollout, cross-channel in-place updates, and
rollback machinery remain outside this program.

V58 ships as three checkpoints: worktree runtime isolation plus this contract; the three-engine
controller matrix; then the real composition-root/media gate and its documented soak procedure.

### 9.2 Restarting in place (V13)

Moved to [`design/deployment.md`](design/deployment.md#restarting-in-place), with decision
[0010](design/decisions/0010-restart-in-process.md).
---

## 10. Commercials & filler

Commercials are core to the "feels like real TV" goal, not a garnish — this is a first-class capability with its own **sourcing pipeline** (deliberately *not* the *arr acquisition path) and its own **matching logic**. The scheduler (§9) inserts the results; this section defines where filler comes from, how it's described, and how pods are built.

### Why filler is a separate pipeline
Titles come from TMDB via Seerr/Sonarr/Radarr. Commercials, bumpers, and station IDs are **not** in TMDB and aren't "titles," so the provisioning loop (§3–§7) does not apply to them. Filler gets its own ingestion — designed so the **core stays a static binary** (no Python, no ffprobe; see §16):
- **Sources:** Internet Archive collections; curated YouTube playlists (the dizqueTV-wiki-style filler repos); user-created bumpers / station IDs / "we'll be right back" cards.
- **Ingestion path (v1):** clips land in a **drop-folder** — placed manually, via an existing tool like MeTube, or via loomarr's own **ingest job** (yt-dlp for YouTube/playlists, plain `net/http` for Archive.org), which writes files + info-JSON sidecars into that folder.
- **Ingest runs in the core, in the single image (revised twice — see the history below).** Ingest is a normal job on the same job bus as every other long operation: it reports progress over SSE, is cancellable, and needs no service discovery, no compose profile, and no proxy hop from the API. The tooling it shells out to (`yt-dlp` + `ffmpeg`) **ships in the one published image** (§16), so ingest is always available. The `FeatureIngest` gate (config-design §7) remains — it now resolves from the binaries being *runnable* rather than from the image variant, and still drives the 409 and the Filler tab's empty state, so a broken vendored binary degrades honestly instead of erroring at the point of use.
  - *History, because this question keeps being re-decided:* **(1)** a `loomarr-ingest` sidecar, removed because its only justification was keeping media tooling out of the core image, bought at the price of a second image, a compose profile, a distributed seam on the Filler page's primary action, and progress that couldn't ride the SSE job bus. **(2)** an opt-in `loomarr:filler` tag, which bought the same slimness without the seam. **(3)** the single image (§9.1, §16) — because `ffmpeg` became load-bearing for *playout*, so a variant without it is not a slimmer Loomarr but a broken one. Each step followed a change in what the tooling was **for**.
- **Filler is Loomarr-owned. A media-server library may be one SOURCE it pulls from (revised twice).** The clip folder is registered in **Tunarr as a `local` media source** (Tunarr scans a plain folder directly) that Loomarr sets up idempotently at first filler sync (same enumerate-first pattern as the Live TV wiring, §6). Loomarr owns the clips: they live in its clip folder, identified by content hash, and program content stays cleanly separated from filler.

  ⚠ **What changed (V38c).** This bullet previously said the media server was out of the filler path *entirely* and that the operator "never creates or manages a commercials library in their media server". That is now too strong: an operator who ALREADY keeps commercials in an Emby/Jellyfin library can register it as a filler source, and Loomarr scans it (§10, "the media-server library row is scanned again"). What has not changed is the ownership model — a library scan is an **acquisition** path feeding the same intake as every other source, so clips are copied into the clip folder rather than played out of the library in place, and Loomarr never modifies the library. The original rationale still holds for the DEFAULT: commercials aren't "library titles", so nothing requires an operator to curate one. It is now an option rather than a prohibition.

  ⚠ **The dependency §9.1 removed stays removed.** A library is never the catalog's only route: an install with no media server, or one whose media server is down, still gets a full catalog from its folders and remotes. "No media server ⇒ no commercials" must not come back.
- **Catalog sync (core) — revised by §9.1, then V38c.** Loomarr scans **`FILLER_DIR` itself** and probes each clip's duration with `ffprobe`. ⚠ **Clip identity is the content HASH** (V38c, "Clip identity is a content hash" below) — the path relative to `FILLER_DIR` (`14/36/<hash>.mp4`) is a disk *location*, not identity. The store keyed on hash since V38c; V45a completes it by making the **API wire identity the hash too** (ClipDTO carries `hash`, mutation routes take `hash`, byte routes take `{hash}` and resolve the path server-side) — the path never crosses the wire, which is what removed the slash-in-URL 404 class.

  *This reverses the previous design, in which Tunarr scanned the folder, assigned each clip a program id, probed its duration, and Loomarr synced that back — clip identity being the Tunarr program id.* Two things forced the change, both traceable to §9.1:

  1. **Internal playout needs a playable input, and a Tunarr program uuid is not one.** Loomarr's own encoder takes a file path or a URL. A channel on the internal backend could assemble a pod and then have nothing to hand ffmpeg.
  2. **The dependency ran the wrong way.** Discovering clips *by asking Tunarr* meant an install running internal playout with **no Tunarr at all** had an empty catalog and therefore no commercials — a hard requirement on a service §9.1 makes optional. The files are on Loomarr's own disk the whole time; routing their discovery through Tunarr was a detour.

  The premise for the old arrangement is also simply gone: it existed so *"probing stays out of loomarr entirely… the core never needs ffprobe"*, and §14 now bundles both `ffmpeg` and `ffprobe` as core runtime dependencies **because internal playout owns duration and cut points**. Scanning locally spends a dependency we already have.

  **Tunarr-backed channels are unaffected.** A clip row keeps a nullable `tunarr_program_id`, populated by the same local-source sync as before, so `attachFillerList` still builds filler-lists from real Tunarr program ids. Internal playout reads `path`; Tunarr reads `tunarr_program_id`; one catalog, one assembler, one seed (below). An install with no Tunarr simply leaves that column empty.

  `/v1/filler/sync` triggers the scan; a periodic sync runs alongside the reconciler. **Identity change ⇒ forward-only migration that drops and recreates the catalog empty**, exactly as `00006` did for the same reason: filler is a synced cache, not source-of-truth data, so the next sync repopulates it.

### Filler catalog (metadata is what enables matching)
Each clip carries metadata so the scheduler can place it well, persisted in the store (§5):
- `kind`: unclassified | commercial | bumper | station_id | psa | trailer | interstitial.
  `unclassified` records that no exact role has been established. It is descriptive rather than a
  lifecycle state: Enrollment may still ground a break-body Placement without relabelling the Clip
  as a commercial. Filename inference may retain an explicit concrete token as diagnostic metadata,
  but an unknown filename defaults to `unclassified`, never `commercial`.
- `era`: decade / year (e.g., 1994)
- `audience`: kids | family | general | late_night
- `category`: toys | cereal | cars | tech | fast_food | movie_trailer | …
- `brand`: the advertiser, when it appears in a text or visual signal (e.g. `Kellogg's`) — free text, grounded (V44)
- `duration` (from Loomarr's own `ffprobe` scan — see §9.1; the "from the media server" note was true only under the pre-§9.1 identity, when Tunarr probed), `rating`, `source`
- `transcript`: the clip's spoken text, when transcribed (V44). Persisted, not transient — it is both a searchable metadata field and the richest input to tagging

Tagging options form a **grounding ladder** — cheapest, most-trusted signal first, each tier running only where the ones above left a gap (V44). Every tier obeys the same rule: **a tag is a fact only when the signal literally contains it** — the anti-fabrication discipline the era grounding rule (below) generalises to brand and to visual tags.

| Tier | Signal | Cost | Catches |
| --- | --- | --- | --- |
| 0 | filename / folder convention | free | eras and kinds encoded in names |
| 1 | source sidecar text (yt-dlp/Archive title, description, uploader) | free | archive.org clips that describe themselves |
| 2 | **LLM over the text signals** → era / audience / category / **brand** | cheap | text-described adverts |
| 3 | **Whisper transcript** → fed into tier 2 | moderate | adverts that *speak* their brand but carry no source description |
| 4 | frame heuristics (black-and-white, aspect ratio) | free | era hints for clips with no usable text |
| 5 | **vision LLM over keyframes** → brand / category / on-screen text | expensive | **silent visual adverts and on-screen logos a transcript never hears** |

Tiers 0–2 are pre-V44 (text-only classification via the configured LLM — filename, the source title/description that yt-dlp/Archive preserve as info-JSON sidecars, and, for split segments, the segment transcript, V34; whisper.cpp, §14). **V44 adds tiers 3–5**: persisting transcripts and running them on demand (`transcribe` job below), a grounded `brand` field, cheap frame heuristics, and vision-based tagging. Vision is no longer future work — it is the only tier that reads a *wordless* clip, and it fits the same grounding discipline (a model reading `KELLOGG'S` off an on-screen box is grounded exactly as an era is grounded when its year appears in the filename). Even text-only tagging is what makes thousands of clips practical; the visual tier closes the silent-advert gap that no amount of text ever could.

⚠ **Vision JSON is field-tolerant and evidence-strict.** A local model can return a valid object
with one wrong-typed optional field (measured live: `category: 0`). That field is discarded without
discarding the independently readable fields in the same answer; a numeric-string era is likewise
accepted as an integer. This does not weaken grounding: brand and era still need literal visible
evidence, and category still has to resolve through the taxonomy. A syntactically invalid or
non-object answer remains a retryable provider failure. The Ollama vision request also enables its
native JSON output mode; prompt-only JSON produced malformed syntax often enough to consume real
retries. Hosted providers retain their existing portable prompt-and-parse path.

**Selective transcription (V44).** Transcribing every clip inline is not affordable — a clip costs ~3s natively but ~341s under QEMU (§10 language gate), so a 100-clip folder would become a ~9.5-hour intake on arm64. The background pipeline therefore listens only when a source description is *thin OR the clip remains unidentified after a text-only pass* — a clip with rich Archive.org details never pays for Whisper. The selective local path is enabled by default; choosing a connected service is the explicit egress/cost decision, and operators may turn the detail pass off independently. Transcripts persist to the store **and** the sidecar (like `originalName`/`normalizedLufs`), so they survive a catalog rebuild.

**Vision tagging (V44) is hosted-provider-first, with a local path.** The hosted implementation uses a separate `AskAboutImages` method building `image_url` content parts with `data:image/jpeg;base64,…` URIs, **not** a widening of `Message.Content` (that string is on the hot path of every text request). A **local** path wires Ollama's per-message `images` field so a fully-local install (llava / llama-vision) also gets visual tagging — the one V44 change that touches the shared `Chat` path, and therefore the one guarded by tests proving the existing text path is unchanged. Keyframes come from `ffmpeg` stills (the `FFmpegArtwork` renderer already produces viewable 320px JPEGs; the `GrayFrames` dHash path is 9×8 grayscale and unusable for vision). Vision is a new external capability, recorded in §14 with its cost rationale.

**Era must be grounded in the source text — a measured §8 hole, closed by V34 (maintainer's call: both halves, not one).** Running the real tagging prompt over real transcripts invented an era on 2 of 10 clips — `1980` and `1970` with no year anywhere in the text, inferred from tone — and the validator had no way to tell an inferred year from a read one (plan §6.4). So: an `era` tag is accepted **only when that year appears literally in the clip's text signals** (filename, sidecar text, or transcript); otherwise it is **not persisted as fact** and is instead recorded as a **suggestion** the operator confirms (`PATCH /v1/filler/tags` setting `era` confirms and clears it). This applies to **every** tagging path, not just transcripts — the sidecar path has always been able to hit it; transcripts merely made it frequent enough to measure.

### Sources fetch on their own (V38b)

A registered, enabled source is **checked on its effective automatic-download policy** and new
items download without anyone asking. This supersedes §15's "there is no unattended crawler":
clips arrive because you added a source, not because you pasted a URL each time.

`filler.fetch.every` is the **only user-controlled cadence authority**. There is no second filler
fetch cron setting. The scheduler wakes a cheap internal due-source planner once per minute; that
wake-up makes no provider request unless at least one source is due. A source is due when its
effective interval has elapsed since its durable `last_checked_at`. A successful provider listing
records the check even when it finds nothing new, so an empty or fully catalogued source is not
polled on every internal wake. A failed listing increments durable failure state and retries after
1 minute, then 5 minutes, then 15 minutes, capped at 1 hour; a successful check clears that backoff.
The source row projects the next eligible automatic check from its interval, retry, and active
claim rather than making the browser repeat that arithmetic.

Before making a provider request, the planner atomically leases that exact source for at most 30
minutes using the last-check fact it observed. A second scheduled or manual pass cannot enumerate
the same source while that lease is live, and a stale worker cannot complete a newer worker's
claim. The scheduler's job lease prevents duplicate full passes; the source lease closes the
smaller race between a scheduled pass and **Look for new clips**. The manual command bypasses the
selected cadence and retry delay, but not an active source claim, source/provider enablement,
geography, deduplication, the effective count, or capacity protection. The fixed internal wake is
implementation timing, not an operator setting and not a second product promise.

⚠ **The superseded rule's concern was legitimate — unattended fetching can fill a stranger's disk
— so it survives as LIMITS rather than as a prohibition.** All are settings, all have defaults, and
all fail toward doing less:

| Bound | Default | Why |
| --- | --- | --- |
| `filler.fetch.every` | `6h` | Default interval for enabled sources. Off (`0`) stops sources that inherit it; positive custom values are bounded from one minute through seven days, and a source may still carry an explicit interval |
| `filler.fetch.max_per_run` | `10` | Items one source may pull per poll — a collection of thousands trickles in rather than arriving at once |
| `filler.fetch.max_catalog_clips` | `2000` | A **ceiling on the whole catalog**. At the limit, auto-fetch stops; manual queueing and approved pulls still work |
| `filler.storage.library_budget_gb` | `0` (automatic) | A soft allowance for Loomarr-managed filler media. Automatic is `min(10% of filesystem capacity, 20 GiB)`; a positive value overrides that allowance but never the hard host reserve below |

These user-facing limits compose with two internal provider protections. One fetch pass may
queue at most **50 items from any one provider**, even when many enabled Sources for that provider
are due. The limit is deliberately not another setting: it keeps one upstream service and one
background pass bounded while the per-Source control remains the useful household choice. Sources
left due by this aggregate limit are considered by the next scheduler pass; a provider cannot turn
the limit into starvation by repeatedly restarting from its newest items.

YouTube enumeration is newest-first and checkpointed. A newly registered Source examines at most
the newest **100 entries** before yielding, and each successful check durably records both the last
stable item identity it examined and the newest identity observed at the start of that sweep. A
later check finds that item in the bounded listing and resumes after it; entries inserted above it
are re-observed and removed by exact identity deduplication rather than making a numeric offset
ambiguous. When it reaches the prior newest-item watermark, provider
exhaustion, or the 100-entry lookback, it commits the new watermark and clears the in-progress
cursor for the next refresh. A missing cursor safely restarts the bounded sweep and relies on durable
provider/source/item deduplication rather than guessing where to continue. A failed listing or queue
operation does not advance the checkpoint.

Automatic YouTube acquisition rejects entries before download when their duration is unknown,
shorter than `filler.min_duration`, longer than `filler.autosplit.max_duration`, or when yt-dlp
identifies them as live, upcoming, private, unavailable, or otherwise too incomplete to fetch.
Across one bounded checkpointed YouTube sweep, an explicit duration suffix in otherwise-identical
titles (for example, the 15- and 30-second cuts of one campaign) is a diversity hint: Loomarr picks
the fuller declared cut from the complete bounded listing before advancing its cursor, and continues
for a different campaign. Unsuffixed titles never collapse on text alone, and this selection hint is
not persisted as duplicate evidence. This prevents variants separated across consecutive checks
from filling the Library without pretending that similar names prove identical media.
Archive.org keeps its existing metadata-tolerant behavior because its bounded collection listing
does not reliably expose the same fields. Successful Source checks persist a closed summary of
queued, already-known, too-short, too-long, live, upcoming, private, unavailable, and incomplete
outcomes. The ordinary UI may explain those counts in plain language; extractor logs, cursor
coordinates, and provider implementation details remain diagnostics rather than controls.

Incoming shows Ready clips only as recent activity. `filler.incoming.ready_window` defaults to
`24h`, hot-applies on the next Incoming read, and accepts one hour through 30 days inclusive. It
uses the Ready pipeline row's canonical update time for the same predicate that drives rows, totals,
and pagination. Aging out changes only this view: the clip remains in the Library and remains
eligible for playback. Incoming returns the resolved window alongside the projection so its
plain-language explanation cannot drift from the cutoff the server actually applied.

**Three properties that are not negotiable:**

1. **Only registered, ENABLED sources are polled.** The Sources switch already claims Loomarr
   "stops scanning, searching and downloading" from a source that is off; auto-fetch is bound by
   the same switch or that copy becomes false.
2. **Everything fetched arrives HELD and converges automatically.** Registration or an explicit
   one-item queue supplies durable household Enrollment authority. A downloaded clip is still
   probed, conditioned, and checked before it becomes Ready, but missing optional classification
   does not create a second approval gate.
3. **A limit that is reached is REPORTED, never silent.** An operator whose catalog stopped
   growing must be able to see which ceiling stopped it. A crawler that quietly does nothing is
   indistinguishable from one that is broken.

**Archive.org and YouTube are peer acquisition partners, not a primary source and a fallback.**
Every registered remote is enumerated by its explicit `kind`: Archive uses the bounded collection
API, while YouTube uses yt-dlp's listing-only flat-playlist mode. The fetcher never guesses a
registered source's provider from a returned URL. The Sources page may perform a bounded,
user-initiated YouTube channel search through yt-dlp's listing-only mode so a person need not know
a channel URL; it does not follow recommendations or crawl beyond the typed query. Selecting a
suggestion is still not authorization to enumerate that channel on a schedule: only the separate
registration command grants that authority. Enumeration downloads no media and
is capped before its per-item URLs enter the ordinary ingest path. Both partners then share the
same per-source and catalog bounds, shared storage governor, acquisition record, held lifecycle, provenance
sidecar, cleanup pipeline, and admission authority. Provider-declared licence metadata is passive
provenance: Loomarr records it when supplied and treats absence as “not provided,” but it never
ranks, filters, holds, rejects, or admits an item. The source URL, uploader metadata, acquisition id,
sidecar, and content hash remain the evidence trail.

⚠ **The explicit kind is load-bearing at both boundaries.** A registered YouTube source once
passed through the Archive collection enumerator because the fetcher accepted only one untyped
discoverer; approved pull targets later re-inferred their downloader from URL. Both make the row's
stored `kind` decorative and allow discovery and download to disagree. Registered work now carries
the kind through enumeration and acquisition. URL inference remains only for a one-off URL an admin
typed directly, where no registered source policy exists.

The report is a **live measurement**, not a remembered fetch result: the filler status read counts
the tracked catalog (including held clips already on disk), while the shared governor measures the
managed roots and real filesystem against the current soft allowance and hard host reserve. The UI
receives that complete server-owned projection and its typed pause reason. This makes “nothing new
arrived” answerable after restart and makes the warning disappear as soon as curation, free space,
or a settings change creates room.

### Storage is reserved before Loomarr writes

The old drop-folder-size ceiling did not protect the host. A nearly full 32 GiB appliance could
start ten large downloads whenever Loomarr itself had written less than 20 GiB, and each downloader
made that decision before the pass rather than before each item. It is replaced, not layered, by one
deep `storagegovernor.Governor` module used at the seam before every Loomarr-managed media write.
Callers supply a conservative work estimate; the governor owns filesystem identity, live capacity,
managed-root usage, amplification headroom, atomic in-flight reservations, and the operator-facing
decision. No fetcher, worker, readiness projection, or browser re-derives this arithmetic.

For the filesystem containing a managed destination:

- The **hard free-space reserve** is `clamp(10% of filesystem capacity, 2 GiB, 20 GiB)`.
- The automatic filler allowance is `min(10% of filesystem capacity, 20 GiB)`. A positive
  `filler.storage.library_budget_gb` value replaces that soft allowance; zero selects automatic.
- Effective automatic availability is the smaller of (a) the remaining soft allowance and (b)
  live free bytes above the hard reserve, after subtracting all active reservations on that real
  filesystem. Existing managed media above a newly lowered allowance remains in place; new
  automatic work pauses and deletes nothing.
- An explicitly confirmed manual import may exceed the soft allowance, but never the hard reserve.
  Unknown estimate or capacity is never permission to write.

`Reserve(estimate)` is atomic across paths on the same filesystem/device. It returns a lease or one
typed pause reason: `library_limit`, `host_reserve`, `estimate_unknown`, or
`capacity_unavailable`. A lease is acquired before queue or worker mutation, revalidated immediately
before execution, charged against actual bytes as output grows, and released on success,
cancellation, or failure. The write itself has a byte ceiling: missing or dishonest provider
lengths cannot consume through the reservation. Overflow stops the writer and leaves only private
staging that the existing safe-grace cleanup may later prove disposable.

An acquisition lease covers exactly its staging ceiling: `source + max(source/4, 32 MiB)`, the
same bytes the write guard enforces, so `reservation = ceiling`. Transcode, split, prepared media
and artwork each reserve their own peak when they run, so the download does not pre-reserve their
derivatives (the earlier shared `ceiling×4 + 64 MiB` held about 5× the source and paused
auto-fetch on `library_limit` while the disk was mostly free). Each item's lease is released as its
download ends, not when the whole batch does. Sizing is per item and never a batch-wide storage
pause: an item Download would skip (an Archive item with no video file) is dropped and counted as
skipped without a reservation; an item the provider cannot size is planned from discovery's
duration and height when known, else a fixed 512 MiB cap, and the write guard aborts it if it
outgrows that cap. `estimate_unknown` therefore only reports a download that overran its
reservation, and readiness does not send it to "choose another folder".

Filesystem identity, not path spelling, groups reservations. Filler, its derived media, prepared
output, diagnostics, and staging on the same device cannot each spend the same free bytes. Hard
reserve accounting aggregates every domain on that filesystem; managed usage and soft reservations
remain domain-local, so prepared and diagnostics retain their own existing budgets rather than
silently consuming the filler allowance. A
same-filesystem rename consumes no second copy; a cross-filesystem intake move reserves the full
destination copy before reading, retains the source until the destination is complete, and releases
the lease only after publication or cleanup. A changed managed root is measured as a new destination
before work is accepted and never implies migration of old media.

Restart deliberately forgets in-memory leases but not bytes. Abandoned staging files remain real
managed usage until safe-grace cleanup proves they are disposable. Automatic cleanup may remove
only cancelled/incomplete private staging, superseded temporary output, and unreferenced generated
artifacts already covered by a retention contract. It never selects admitted clips, pinned media,
held review evidence, unique source masters, symlinks, or unknown files. Reclaiming meaningful
media is an explicit operator action with a byte-and-consequence preview, not governor policy.

The server projects one capacity snapshot: managed usage, active reservations, filesystem free and
total bytes, effective soft allowance, hard reserve, available bytes, pause reason, and the actions
`free_disposable_space`, `choose_another_folder`, and `change_storage_limit`. Sources shows a calm
summary while healthy and the exact reason plus one primary action when paused. Advanced contains
the path and soft-budget controls; the browser performs no capacity calculations. The governor also
owns the presentation state (`healthy`, `approaching`, `paused`, or `unknown`). `approaching` begins
when effective availability is at most the smaller of 1 GiB and twenty percent of the active soft
allowance, so small appliances receive a proportional warning without making an empty automatic
library look constrained. The cleanup action revalidates and removes only terminal acquisition
attempt directories older than 24 hours that no staged or repair artifact references, then returns
a fresh preview; admitted media and every unknown path remain outside that authority.

**`Look for new clips` runs acquisition as well as discovery (V56).** A source row invokes one ordinary
bounded fetch pass for that selected source and then scans the configured local sources. It is not
an alias for the local catalog scan: that old wiring returned success on an Archive or YouTube row
while queueing no download at all. Nor is it a global button repeated on every row: clicking the
drop folder cannot unexpectedly start three remote collections. The deliberate admin action may
run before a source is due or while its automatic timing is off, but it retains the source's and
Provider's enabled switches, geography, deduplication, and disk/catalog/per-source ceilings, so it
is not an unbounded bypass. Explicitly queueing one searched item remains a separate action and does
not consume or reset the source's automatic check cadence.

The response preserves the selected source identity and the acquisition result: how many sources
were polled, how many items were queued or skipped, and which capacity ceiling stopped the pass.
The browser therefore reports **Checked Classic TV Commercials — 2 clips queued**, not a generic
success inferred from a refreshed list. Other remote rows retain their own status and never appear
to have participated in the selected-source command.

⚠ **Archive.org collections are the case the limits exist for.** A collection is thousands of
items; `max_per_run` is what stops "add a source" from meaning "download 8,000 files tonight".
A bulk backfill remains the **pull**'s job, where a human sees the plan and approves it.

### Two downloaders, two gates (V38b)

Filler arrives by two mechanisms with **different requirements**, and they are gated separately:

| Source | Fetched by | Needs |
| --- | --- | --- |
| **archive.org** | plain HTTP | **ffmpeg only** (to probe and thumbnail what it fetched) |
| **YouTube** | yt-dlp | yt-dlp **and** ffmpeg |

⚠ **This corrects a real defect, not a preference.** One `ingest` feature flag required *both*
binaries, so a missing yt-dlp switched off archive.org downloads too — despite that path never
invoking it. On a source build with ffmpeg installed and no yt-dlp, downloads were reported
unavailable while being perfectly runnable, and **the starter pull is an archive.org collection**,
so first-run acquisition was blocked by a binary it does not use.

The invariant "every ingest needs yt-dlp" was true when written and became false when the archive
downloader landed beside it; the gate never split. Same shape as V37's `Fetchable()` — an implicit
rule that quietly stops holding when a second case appears.

**So the surface reports per source, never one blanket verdict.** "Downloading isn't available"
was a claim about the whole subsystem made from one binary's absence; a source that can fetch says
so, and one that cannot says which tool it wants.

### The clip lifecycle: held, then Ready (V38/V66; household beta)

Until V38 a clip had no lifecycle. The folder scan catalogued it and the tagger tagged it **in
place**, which meant everything Loomarr downloaded was playable the moment it landed — tagged or
not, right or wrong. V38 gives an arriving clip a **state**:

- **held** — recorded but not playable while required runtime work is incomplete or failed;
- **Ready** — exact playable bytes, Enrollment authority, and Placement were committed together by
  the terminal-ready operation. Only Ready clips may enter a Pod;
- **not usable** — an objective failure, positive non-filler determination, composite container, or
  removal prevents playback.

Enrollment authority comes from choosing and enabling a Filler Source, opting a folder or Library
into Filler, or explicitly queueing one item. It is captured when the Clip enters the conveyor, so a
later Source disable stops future work without rewriting existing outcomes. The enabled Source is
the household's approval; the runtime must not ask for a second per-Clip approval merely because an
optional classifier did not identify an exact role.

Role and Placement are separate. Role describes what the Clip is; Placement says where it may run.
A known commercial, promo, PSA, trailer, or interstitial maps to break body. A bumper or station ID
maps to bookend. Enrollment grounds break body when the exact role remains unclassified; this does
not relabel the Clip as a commercial. Composites and positively identified programme excerpts map to
not playable. An explicit channel kind filter still narrows to known roles; default scheduling may
use enrollment-grounded break-body Clips.

Every new arrival still starts held. The distinction is that the ordinary pipeline now has an
executable terminal operation: after required runtime checks it atomically stores Placement, clears
the hold, settles the conveyor as Ready, and appends the effective activity outcome. Exact retries
are idempotent and stale clip/pipeline identity rolls the transaction back. Composite containers
retain their separate split-repair lifecycle and are never themselves scheduled.

Incoming projects that lifecycle without exposing the state machine as work for the household. Its
three groups are disjoint: machine-owned rows are **preparing**, current split choices backed by the
split-confirm mutation are **needs help**, and Ready rows from the previous seven days are
**recently ready**. A row is counted by the same predicate that can return it, so a capped page never
disagrees with its total. Ready rows age out of Incoming while remaining in Library. Rejected and
completed audit, retries, provider/model failures, and optional enrichment live in Activity or
Diagnostics and cannot become a Needs-help task merely because a browser can describe them.

The calm row carries one server-owned current-state sentence. Its Clip panel may reveal collapsed
**Processing details**: the server's ordered Processing stages, each with a friendly name, outcome,
recorded time, and safe explanation. Successful and active stages remain visible; routine
`not_needed` stages are hidden behind **Show skipped steps**; retrying work remains visible without
turning recovery into a human task. A diagnostics link is present only when the server names an
existing operational owner. The projection never exposes a private path, raw error, transcript,
provider/model response, credential, or browser-reconstructed reason.

**Stage progress belongs only to the active Processing stage.** A measured stage carries an exact
0–100 value and labels it as current-step progress. An active stage without a real measurement is
indeterminate. Finished, waiting, retrying, skipped, and Ready states carry no percentage. Stage
progress may reset between stages and is never Preparation progress, percent Ready, or an ETA; the
server projection, not the browser, owns the stage label, outcome, safe note, measurement, and
order. Every stored stage occurrence is returned in its stored order, including occurrences with
identical content and timestamps. The private-beta `technical` projection is retired rather than
kept as a compatibility alias.

### The quality gate: reject the broken, normalise the quiet (V40)

A clip that arrives is not automatically a clip worth playing. Real downloads from the archive
sources include truncated fragments, audio-only files, and spots recorded a decade apart at wildly
different levels. **Objective failures are handled automatically — there is no badge or operator
decision for a file that cannot play.** A gate an operator has to read is a gate they learn to
click through (§7). Content anomalies that can also be intentional are the narrow exception below:
the machine shows its measurement and asks rather than deleting a plausible creative choice.

**Rejected at the scan boundary.** `ScanDir` already skips unprobeable files; these join it, and
they never become clips at all:

- **Shorter than 10 seconds.** Nothing usable as a break body is shorter. The existing guard was
  `DurationMs <= 0`, which a **2.9KB, 33-millisecond** truncated download passed — it sat `filed`
  and airable in the dev catalog, i.e. a third of a second of nothing in an ad break.
- **No audio stream.** Silence mid-break.
- **No video stream.** An audio-only file in a video catalog.

**The transcode pass also inspects the content it is already decoding.** Stream presence does not
catch a valid MP4 containing thirty seconds of black, a muted audio track, or a decoder repeating
one damaged frame. Adding a second unconditional decode would double the most expensive routine
part of ingest, so `blackdetect`, `silencedetect`, and `freezedetect` ride the mezzanine encode's
video/audio filter chains. Their measured intervals are written to the sidecar: the evidence
survives a catalog rebuild, and an older mezzanine that predates the measurement gets one bounded
inspection pass rather than another lossy encode.

On the first pipeline pass after this ships, already-filed rows whose sidecars carry a mezzanine
marker but no quality report are re-queued at `transcode`. The marker makes that rung inspection-
only: it does not encode those bytes again. The ordinary transcode budget still bounds the work,
so a large existing catalog drains over cycles instead of turning an upgrade into a decode storm.
Rows already rejected or waiting on a person are left alone.

The decision is deliberately asymmetric:

- **At least 90% black or 90% content-silent is a hard reject.** This is dead air with streams
  attached, not the wordless-but-audible advert the language rule below protects. It appears in
  the refusal audit with the measured coverage; no operator control can make it playable.
- **A black or silent span of at least 5 seconds, or a frozen span of at least 10 seconds, goes to
  review unless the hard rule above already decided it.** A long end slate, a radio spot over a
  still, and an intentional dramatic pause are plausible, so freeze alone is never auto-rejected.
  These are the rare judgement calls; clean clips proceed without a badge or a decision.

Any `review` disposition holds the clip out of rotation at the pipeline boundary, including a
hand-dropped or previously-filed clip. Recording "needs a look" while leaving the same bytes
eligible for the next break would make the review state decorative rather than protective. The
hold must succeed before the terminal review row is committed; on a transient store failure the
row stays runnable and the persisted detector report re-emits the verdict without decoding or
encoding the media again.

Intervals are unioned and clamped to the probed duration before coverage is calculated, so
overlapping detector events cannot manufacture more than 100%. A trailing event with no explicit
end is closed at the measured duration rather than dropped.

**Loudness is measured at ingest and applied at playout.** Measured across real fetched clips the
spread was **−21.8 to −32.6 LUFS** — about 11 dB, which is the clip-to-clip volume jump an operator
hears as "some of these are too quiet". The target is **−23 LUFS**, what broadcast uses. The
measurement rides the decode that already happens for artwork (§10 V39), so it costs no extra pass.

⚠ **Normalisation happens in the PLAYOUT chain by default — and that remains the default.** At
playout it is one filter on a stream already being encoded, it is reversible, and changing the
target later simply works. `FILLER_DIR` and the watch folder hold files a person put there, so
rewriting them is never something Loomarr does unasked.

**On-file normalisation is available as an explicit opt-in** (`filler.conditioning.normalize_loudness`,
default **off**, V42 — maintainer decision, originally surfacing the mock's Tune-panel toggle).
When enabled, the conditioning rung creates the playback derivative with ffmpeg `loudnorm` while
the clip remains held. This improves playback evidence; it grants no admission authority.

⚠ **It uses the SAME `filler.target_lufs` (−23) as the playout pass, not a second target.** Two
targets in one system means a clip normalised on file is then corrected again downstream toward a
different number — double processing, and a quieter result than either setting asks for. One
target, whichever stage applies it.

⚠ **It changes the playback derivative and the operator is told so.** V66 retains the immutable
source master, so the acquired bytes remain recoverable; the opt-in still changes what could air
after admission and therefore cannot be implicit. What it must not also be is *repeating*: a
re-scan cannot tell by looking that a playback derivative has already been normalised,
so without a marker every pass would normalise an already-normalised file, walking the loudness
down on each run. The sidecar records `normalizedLufs` beside the clip, and the pass **skips any
file already carrying that marker at the current target**. The marker travels with the clip the
same way `originalName` does, so a restored catalog does not re-do the work either.

⚠ **Playout still normalises, and that is deliberate rather than redundant.** A file normalised on
disk measures at target already, so the playout filter is a no-op for it — while clips that arrived
before the toggle was turned on, or whose derivative has not yet been conditioned, are still corrected. The
guarantee "every break plays at a consistent level" cannot depend on which clips happened to pass
through one optional step.

### Source evidence and playable media are different assets (V66)

The former transcode contract kept one H.264/AAC mezzanine and deleted the bytes it was made from.
Archive acquisition compounded that loss by selecting the smallest video derivative by default.
That is adequate for immediate playback and the wrong foundation for deciding where one commercial
ends, reading small print and logos, distinguishing an upload defect from source content, or
reproducing a later model judgment. A convenient rendition is not source evidence.

Every clip that reaches the transcode rung therefore has three explicit media roles:

| Role | Authority and use | Mutation contract |
| --- | --- | --- |
| **source master** | Exact acquired or operator-supplied bytes; the authority for provenance, reprocessing, and recovery | Immutable. Full-file SHA-256 and byte length identify it. It is never replaced by a derivative. |
| **evidence derivative** | Complete bounded A/V inspection, segmentation, OCR/frame extraction, transcription, and model input | Reproducible from the master under one versioned recipe. No playout loudness rewrite and no cosmetic restoration. |
| **playback derivative** | The file registered with Tunarr or read by internal playout | H.264/yuv420p video, AAC stereo at 48 kHz, fast-start, and bounded GOP; optional measured loudness policy belongs here. |

The source master is copied and fully hashed into the hidden content-addressed tree
`.loomarr-media/masters/<sha[0:2]>/<sha[2:4]>/<sha><original-extension>` before any derivative may
publish. Hidden media is beneath the same filler filesystem so staging and publication remain atomic,
but the ordinary folder scan never treats it as playable. A full digest collision with different
bytes, a non-regular object, an escaping path, or an incomplete copy fails closed. Removing the
visible pre-transcode name after playback publication removes only that name; the master remains.
Application-enforced immutability is the contract—read-only mode bits are defence in depth, not proof,
so every reuse re-opens and verifies the exact bytes.

The playable sidecar carries one closed, versioned media-asset manifest. It binds the source-master
digest/path/length and each derivative's role, input digest, output digest/length/path, recipe id and
recipe digest, exact media-tool identity, decoded duration, stream identities, and completed QC
evidence. A derivative can be reused only when all of those facts match and its current regular-file
bytes have been reverified against the recorded full SHA-256 and byte length; cached QC and sparse
clip identity alone cannot authorize reuse. Paths are filler-root-relative: retained masters and
evidence stay in their hidden role trees, while playback uses its visible catalog path. Paths are
never accepted as identity. A catalog rebuild can
therefore recover the relationship without trusting a database cache or re-running paid analysis.
The hidden source also carries enough portable provenance to find its acquired source and original
name when the playable rendition is absent.

After transcode, the consumed acquisition artifact intentionally spans two roles: its full-file
SHA-256, byte length, and media path identify the immutable source master, while its Clip hash links
to the current playback derivative in the catalog. A catalog scan verifies those roles separately
through the closed media-asset manifest and normalizes an older intake path to the retained master;
it never compares the source-master digest and length to the playback file merely because both
belong to the same acquisition. A historical repair created by that invalid cross-role comparison
may clear automatically only after the exact retained source and current playback bytes both
reverify; any other repair reason or identity drift remains held.

**Archive chooses a source representation, not the cheapest playback file.** Selection is a pure,
stable ordering over Archive's declared file metadata. A recognized video original outranks a
derivative; within the same source class Loomarr prefers complete positive duration and dimensions,
then greater dimensions, a plausible greater encoded bitrate derived from declared bytes/duration,
then positive byte length, with the normalized filename as the final tie-break. Unknown facts never
satisfy a minimum or beat an observed fact, malformed and non-video entries are ineligible, and the
chosen representation plus every observed ranking input travels in acquisition provenance. The
planner's item-level quality floor still applies before download; representation selection cannot
launder an item that failed it. This is a quality/evidence policy, not a claim that Archive's
`original` label proves authenticity or redistribution rights.

**Recipes say exactly what was changed.** The evidence recipe normalizes timestamp origin and selects
one explicit video/audio stream while preserving measured display dimensions, aspect and cadence; it
does not apply loudness, crop, scale, deinterlace, frame-rate conversion, denoise, sharpen, colorize,
logo removal, or grain removal implicitly. An observed defect may justify one of those transforms only
through a different recipe that records the before measurement, decision and exact parameters. The
playback recipe likewise preserves dimensions, display aspect and cadence by default while making the
codec/container changes above. Neither derivative is described as "improved" merely because it is
newer or more compatible.

Both derivatives are built from the retained master, never serially from one another. Publication is
stage-then-verify-then-atomically-name: a candidate must completely decode and match the expected A/V
stream presence and bounded duration before its final hidden or playable name can appear. QC records
container/stream duration, exact cadence, interlace observation, display aspect, audio/video start and
end skew, integrated loudness/true peak, corruption, black, silence and freeze evidence. Seekability,
fast-start and bounded keyframe distance are additional playback checks. Unavailable required evidence,
an unjustified timing/aspect change, failed decode, or mismatched digest holds the clip and preserves
the master; it never promotes a partial derivative.

Confirmed compilation children retain the reviewed parent hash and exact intended interval as today,
and additionally bind the parent media-asset digest and the exact source role from which the cut was
made. A child cut from a confirmed complete-timeline decision also retains that decision's exact
semantic segment role; materialization derives the catalog kind from this role instead of copying the
compilation parent's generic kind. Boundary measurements compare the child with that immutable parent
role. This prevents a future
playback-recipe change from silently changing what the split decision meant. The actual boundary
detection and commercial-versus-scene-change policy remain the following split rung's responsibility.

There is deliberately no automatic master garbage collection in V66. A later recoverable GC may remove
a master only after no playable clip, split lineage, audit record, inference evidence, acquisition
authority, or retention obligation refers to it and after proving every remaining derivative is
regenerable. Until that complete ownership graph exists, storage costs are visible and masters stay.

### Compilation structure is assessed before held children are materialized (V67)

The duration quarantine introduced in V45 answers only **“is this recording too long to air as one
ordinary filler clip?”** It does not answer **“does this recording contain several commercials?”** A
two-hour programme excerpt, a damaged capture, and a commercial compilation all cross the same duration
threshold. The existing `is_composite` column remains the conservative non-airable guard for migration
compatibility, but while structure is unresolved it means **long-source candidate**, not a semantic
compilation verdict. Only the source-structure assessment below may establish that verdict.

One deep structure-assessment module owns the complete evidence-to-plan operation. Its interface accepts
an exact V66 evidence asset plus independently timestamped observations and returns one validated,
content-addressed assessment. Callers do not sequence individual detectors, interpret model confidence,
or assemble cuts themselves. Internally the module may use deterministic reducers and a bounded model
adapter; those are internal seams, not additional application interfaces.

The source verdict uses one closed vocabulary:

| Source structure | Meaning |
| --- | --- |
| `single_unit` | One independently usable item, including a long single-product infomercial. |
| `compilation_break` | Two or more independently bounded filler units recorded back to back. |
| `programme_with_spots` | Programme material containing inserted filler units; programme spans remain explicit. |
| `ambiguous` | Available evidence cannot safely distinguish the structures above. |
| `unusable` | The source cannot support a complete trustworthy assessment (for example, corrupt or materially missing evidence). |

Duration can request assessment and keep a source non-airable; it is never evidence for
`compilation_break`. A source remains `ambiguous` rather than being promoted by plausible 15/30/60-second
unit lengths. A single long span is not split merely because it resembles several standard slots.

Every observation is a durable source-relative fact with a closed kind, exact interval or uncertainty
window, producer identity, evidence digest, and outcome. Container chapters, black intervals, silence
intervals, transcript topic transitions, OCR/logo transitions, audio continuity, and visual continuity
remain independent observations even when they coincide. Coincidence is computed by the reducer; it is
not flattened into a score that discards which detector agreed or contradicted the candidate. An ordinary
scene cut is never a boundary proposal because it describes editing inside commercials as often as joins
between them. Scene change and standard duration may be recorded only as supporting context for a boundary
already proposed by a more specific observation.

Boundary fusion is deterministic and conservative:

1. A declared chapter edge or compatible precise separator observations may propose a bounded cut.
2. Agreement narrows the candidate's uncertainty window; conflicts survive on the candidate and prevent
   unattended materialization.
3. Transcript, OCR/logo, audio-continuity, and visual-continuity changes may support, contradict, or leave
   that candidate unresolved. Absence from a modality that did not run is not negative evidence.
4. The bounded observation model may inspect only unresolved spans. Its strict output cites supplied
   observation IDs and records exact provider, model, prompt/schema, request/response digest, time, token,
   and cost identity. It contributes an observation and cannot override a hard conflict, invent an
   observation, or directly set the source verdict.
5. No scene-only, duration-only, model-only, or round-timestamp candidate becomes an automatic cut.

The assessment contains a **coverage-preserving segment plan** over the complete half-open source timeline
`[0,duration)`. Plan intervals are ordered, non-overlapping, adjacent, and together cover exactly the whole
asset. Every interval has one disposition:

- `keep`: a proposed child with two resolved boundaries and one independently established role;
- `discard`: material intentionally omitted with a closed reason and retained source-relative interval;
- `unresolved`: material requiring review, including uncertain joins or roles.

There are no implicit gaps. A three-second stinger that cannot enter the catalog may be an explained
discard, but its time, evidence, and reason remain in the proposal. Editing or partial confirmation rewrites
the plan while preserving complete coverage; it cannot make omitted time disappear. The existing aggregate
drop tally is compatibility display data only and is not structure authority.

Each `keep` interval is classified independently as `commercial`, `promo`, `bumper`, `station_id`, `psa`,
`trailer`, `interstitial`, `programme_fragment`, `non_filler`, `ambiguous`, or `unusable`. A parent catalog kind is never
copied as the child's role. `programme_fragment`, `non_filler`, `ambiguous`, and `unusable` are never
automatically admitted as filler. Every child binds its role evidence, transcript spans, OCR/logo findings,
frame/audio evidence, exact parent evidence asset, and intended source-relative interval before media is
cut. The portable lineage retains the exact role and the sidecar retains the derived catalog kind so a
catalog rebuild cannot collapse every child back to the parent's generic `commercial` value. The legacy
catalog vocabulary represents `promo` as `interstitial`; the exact `promo` role remains in lineage rather
than being erased by that scheduling projection. Post-screen taxonomy enrichment remains a later operation
and cannot repair an unresolved role.

The existing bounded split-time vision request may return taxonomy grounding and a segment-role judgement
in the same call; role assessment does not introduce an unbounded second request per cut. The role result is
a distinct `segment_role` observation, not a tag projected into a role. It binds the exact source and span,
ordered frame digests, prompt contract and digest, request and response digests, assessment time, requested
and resolved provider/model identity, modalities, attempts, latency, token accounting, generation identity,
and provider-reported charge when available. Missing provider/model identity, malformed evidence, an
unsupported role, or an absent explanation yields no role claim. `ambiguous` and `unusable` remain recorded
outcomes but cannot create a `keep` interval. Rebuilding an assessment replaces role observations for the
same spans from the proposal's retained evidence while preserving independent detector observations and
explicit discard intervals; it never treats a prior tag, generated name, or parent kind as role evidence.

When that sparse-frame request cannot establish a role, the exact unresolved interval may make one
**direct-video escalation** through the certified `filler_video` route. The escalation receives a freshly
rendered, metadata-free MP4 of only that source interval, capped at 60 seconds and 12 MiB; an over-limit span
is held rather than narrowed silently. A valid frame role suppresses the escalation. The resulting role
evidence binds the derivative SHA-256 instead of frame SHA-256s plus the same exact source/span,
prompt/response, route, modalities, accounting, and generation identity. An extraction failure, derivative
identity drift, unavailable route, malformed output, missing attribution, or unsupported role leaves the
interval unresolved. Merely implementing or capability-checking this route does not certify it and cannot
expand unattended materialization; only the locked source/signal-slice result below may do that.

Raw assessors are observations, not competing materialization authorities. A deterministic
**structure-decision reducer** receives complete, content-addressed candidate assessments from at least
two independently locked producer families bound by the current reducer certificate. Two routes serving
the same model family count once. The
reducer first validates exact source identity and complete timeline coverage, then requires the candidates
to agree on the source unit, interval count, per-interval disposition and role, and ordered joins. Joins may
differ only inside the locked boundary tolerance; the reducer selects their deterministic midpoint and
revalidates complete coverage. A hard detector conflict, operational failure, missing interval, different
unit, different role, excessive join distance, or unsupported candidate produces an explicit hold.
Majority voting cannot turn disagreement into a decision, and one valid prohibited observation remains
governed by the separate safety reducer rather than this structure policy.

A complete-timeline candidate assessor is distinct from the bounded observation and per-interval role
escalations above. It receives the same exact deterministic assessment derivative, sees no peer answer,
and must describe every millisecond from zero through the measured duration. One shared, content-addressed
assessment-media contract owns the challenge and runtime normalization recipe: MP4 with H.264/yuv420p at
960-by-720 and 30 frames per second using aspect-preserving padding and square pixels, AAC stereo at 48 kHz,
a 90,000 video timescale, normalized timestamp origin, stripped metadata, bitexact controls, and a 64 MiB
encoded-byte ceiling. The recipe identity includes the exact part and concat argument templates; copying
those arguments into a challenge-only or provider-only adapter is not an equivalent contract. Every
assessment binds both the immutable V66 source (complete SHA-256, byte length, and source timeline) and
the exact submitted derivative (complete SHA-256, byte length, measured duration, and media-profile
SHA-256). The source and derivative are distinct first-class identities in every candidate, reservation,
settled assessment, reducer decision, and materialization-authority check; a transformed-media digest may
never replace or masquerade as the parent source digest. The production preparer likewise receives two
distinct filesystem authorities: the applied filler root used to resolve the retained source path, and a
private assessment-media root used only for snapshots, normalized derivatives, lineage, and operation
indexes. Treating the private output root as the source root makes every catalog path fail before rendering;
deriving either root from the other makes relocation part of semantic identity. Both roots are clean absolute
runtime locators and remain excluded from the path-free evidence. The certification
authority also names the tested source-duration and encoded-byte envelope, and runtime holds before spend
when a source or derivative falls outside it. The certified hosted runtime prepares and validates the complete
media set before it refreshes provider-route metadata, constructs an assessor, reserves spend, or submits HTTP;
an unreadable source, profile mismatch, incomplete decode, or over-ceiling derivative therefore makes no
provider request. Qualification may not extrapolate from shorter cases, silently
truncate a reel, or replace complete-timeline authority with independently judged chunks. If a truthful
full-timeline derivative cannot fit the certified ceiling, long-reel handling requires a separate designed
and certified protocol.

The OpenRouter adapter fixes one fallback-disabled, zero-data-retention route and strict schema, hashes the
exact prompt, schema, request, response, and structured output, and durably reserves the request digest
before sending any bytes. Its configured
worst-case charge must fit inside that reservation. A budget hold sends no provider request; an unknown
transport settlement retains the full reservation; a known charge closes against the provider-reported
amount, and an over-reservation response is retained but unusable. The adapter settles the durable
accounting record before it returns candidate evidence. A crash between reservation and settlement leaves
a discoverable open reservation rather than silently freeing budget. Implementing this adapter grants no
runtime or materialization authority: two independently locked families, persisted raw evidence, the
reducer, and a locked structure-slice certificate remain mandatory. Downstream screening remains mandatory
for broadcast admission but is intentionally not part of this pre-child decision.

The structure reservation participates in the shared filler-inference budget rather than keeping a
provider-specific spend total. One transaction reserves that shared budget and appends a structure journal
entry binding the exact source, assessor profile, expected route, prompt/schema, request, worst-case charge,
and request time. The journal rejects a second reservation for the same request regardless of whether the
first is open, budget-held, or settled. Settlement atomically closes the shared accounting row and stores
the validated content-addressed assessment record. Open and budget-held entries remain listable for
recovery; they are operational holds, never permission to repeat a possibly billed request.

The reducer's immutable artifact retains every candidate identity and the exact reason each case was
confirmed or held. The runtime publishes that content-addressed artifact before attaching it to a split
proposal. A child cut on an exact confirmed `keep` interval carries the artifact SHA-256 in its portable
conditioning lineage; a legacy detector cut or operator-edited interval leaves that field absent rather
than claiming decision provenance it does not have. Certification scores this deterministic policy as the
production candidate: every automatic decision must have zero under-splits, over-splits, unexplained gaps,
wrong filler/programme dispositions, wrong roles, or boundary misses beyond tolerance. Abstentions remain
in the denominator and are reported as coverage, globally and per predeclared slice. Raw assessors may be imperfect; they do not
individually need perfect recall when their disagreement correctly becomes a hold. Each enabled slice still
requires its predeclared minimum number of independently sourced decided cases and zero wrong automatic
decisions. The development certificate that permits shadow evaluation requires at least 30 of 60 decisions
globally, at least 6 decisions in each source-unit category, and at least 6 decisions in every declared
difficult slice. It grants no automatic materialization authority; later slice activation remains a separate decision based
on the locked shadow evidence. This changes what is measured, not the safety bar.

Structure authority and broadcast admission are deliberately separate decisions. A certified
`compilation_break` or explicitly bounded filler portion of `programme_with_spots` may authorize automatic
**materialization** of its complete keep plan: Loomarr cuts every decided filler interval into a held,
non-airable child and enrolls each child at the start of the ordinary ingest ladder. This authority requires
exact evidence bytes, resolved boundaries and allowed filler roles, complete explained coverage, and a
source/signal slice inside the locked structure certificate. It does not claim that a child is safe, lawful,
playable, enriched, or ready to air. An unresolved interval retains one concise structure review, and a human
confirmation is still validated against complete coverage and exact source identity.

The five content screens run on each materialized child only after its final playback derivative exists.
This sequencing is mandatory: a playback-integrity result over the parent span, the stream-copy cut, or a
throwaway preflight rendition cannot prove the bytes that will air. One content-addressed child-screening
subject binds the immutable child lineage, source-master identity, evidence-derivative identity, final
playback-derivative identity, derivative recipes and measurements, and the parent's exact half-open interval.
The subject excludes relocatable filesystem paths: runtime paths only locate artifacts, and each evaluator
must reopen the artifact it uses and reproduce the subject's byte identity before deciding. The aggregate
contains exactly five independent
closed outcomes: `visual_safety` (including explicit imagery), `spoken_safety` (including audibly
prohibited language), `written_safety` (including visibly written prohibited language), `rights`, and
`playback_integrity`. Visual, spoken, and written evaluators inspect the complete evidence derivative;
rights replays acquisition and participation authority; playback integrity inspects the final playback
derivative plus its conditioning evidence. A semantic tagger, a few representative frames, transcript
absence, or visual-model prose cannot satisfy any of those complete-coverage safety axes. Each outcome is
`pass`, `reject`, or `hold`, names an
opaque reason code, and binds the SHA-256 of one immutable axis-evidence record. Raw restricted phrases and
descriptions never enter proposal, catalog, or public admission records.

Airworthiness is audience-policy evaluation over evidence, not a model synonym for `safe`. The three
safety evaluators therefore preserve closed **suitability flags** separately from their axis verdicts:
sexual or nude content; minor presence and age ambiguity; weapons, threats, non-graphic violence,
graphic violence or gore, human death, animal harm, and self-harm; tobacco, alcohol, drugs, and gambling;
hateful targeting, extremist symbols, slurs, profanity, explicit language, and threats; frightening or
disturbing imagery; and regulated-product promotion. Where it changes the policy meaning, evidence also
distinguishes presence or depiction from promotion and instruction, and records a closed severity. Company,
brand, product, logo, and ordinary commercial presence remain enrichment facts unless a policy specifically
consumes them. Religion, politics, war, and historical subject matter are descriptive context, never automatic
prohibitions. `minor_present` is likewise not a prohibited verdict: it may exclude a case from an adult-only
control while remaining acceptable under an audience policy.

The first policy profiles are `all_ages`, `general_audience`, and `restricted_archive`. A deterministic policy
maps complete, certified flags and context to `pass`, `reject`, or `hold`; it never asks a model to choose an
audience verdict. `restricted_archive` permits deliberate retention, not unattended playout. An unknown flag,
unsupported severity/context, missing modality coverage, or conflicting evidence holds. Public records expose
only stable flag and reason identifiers; private raw evidence retains the restricted text or imagery needed for
audit. Certification is policy- and category-specific: evidence that certifies adult-nudity detection cannot
silently authorize violence, hateful-language, substance, or other audience-policy decisions.

The version-one audience policy uses three internal actions: `allow` means the observation does not restrict
that profile, `review` makes Airworthiness `hold`, and `reject` makes Airworthiness `reject`. An observed
`reject` always wins over missing coverage, conflicting evidence elsewhere, or an observed `review`; a valid
positive is never erased by a negative vote or another axis's absence. With no rejecting observation, every
policy-relevant flag must be covered by a complete axis record whose exact evidence-contract, policy,
certification, and implementation profile matches the configured release authority. Missing axis coverage,
an uncertified required flag, profile drift, malformed or unknown evidence, or an upstream conflict holds.
Only complete certified coverage with no `review` or `reject` observation passes.

| Suitability flag or family | `all_ages` | `general_audience` |
| --- | --- | --- |
| adult nudity; minor or age-ambiguous sexual risk | reject | reject |
| sexual activity or sexualized presentation | reject | low severity reviews; moderate/high rejects |
| weapon depiction | promotion/instruction or moderate/high rejects; low reviews | promotion/instruction rejects; high reviews; low/moderate depiction allows |
| threat; non-graphic violence | moderate/high rejects; low reviews | high rejects; moderate reviews; low allows |
| graphic violence or gore | reject | reject |
| human death or corpse | moderate/high rejects; low reviews | high rejects; low/moderate reviews |
| animal harm or death | moderate/high rejects; low reviews | high rejects; moderate reviews; low allows |
| self-harm or suicide | reject | promotion/instruction or high rejects; low/moderate reviews |
| tobacco, alcohol, drugs, or gambling | promotion/instruction or moderate/high rejects; low depiction reviews | promotion/instruction rejects; high depiction reviews; low/moderate depiction allows |
| regulated-product promotion | reject | reject |
| hateful or extremist symbol | promotion/instruction or high rejects; low/moderate depiction reviews | promotion/instruction rejects; high depiction reviews; low/moderate depiction allows |
| hateful targeting; slur or degrading language | reject | reject |
| profanity | moderate/high rejects; low reviews | high rejects; moderate reviews; low allows |
| explicit sexual language | reject | moderate/high rejects; low reviews |
| frightening/disturbing or severe injury/medical imagery | high rejects; low/moderate reviews | high rejects; moderate reviews; low allows |
| ordinary minor presence; age ambiguity alone; religious suffering; war/military; ordinary commercial/brand presence | allow | allow |

`restricted_archive` validates and retains the same factual observations but always returns `hold` for
unattended playout. It is a retention profile, not a permissive audience profile. Each observation carries one
closed modality, severity, context, source-relative half-open interval, and opaque id. The public policy result
may expose those closed values and evidence digests, but never raw restricted language, imagery descriptions,
model prose, or chain-of-thought.

Producer projection is an authenticated translation, not a string switch. A visual policy-match id or spoken
restricted-rule id becomes a suitability flag only through an immutable projection authority that binds the
producer policy, certification and implementation identities, the exact opaque id, and its closed flag,
severity, and context. The projector first reproduces the producer's complete canonical result. A known valid
positive may then publish a source-relative observation and retain positive-wins rejection even when other
coverage is incomplete. An unknown id, missing timing, stale producer identity, irreproducible result, or
unprojectable source/child relationship yields conflicting or incomplete Airworthiness evidence. A negative
producer result supplies complete coverage only for flags explicitly claimed by the matching category-specific
certificate; it cannot clear another flag merely because both originated in one model call. Private raw
producer reports remain digest-bound evidence and public projections contain no matched words or descriptions.

A visual observation with multiple opaque match ids and multiple intervals must not invent an id-to-interval
association. Without an explicit binding, those ambiguous matches supply no timed suitability observations
and leave coverage incomplete. Independently bound positives from other observations remain valid and can
still reject; ambiguity never erases them. A single match id may cover multiple supplied intervals, and
multiple match ids may share one supplied interval. Apple source-level positives retain their documented
whole-source interval because that producer supplies no timestamps.

The rendered-child spoken and visual adapters own the complete bridge from those producer reports into the
four-axis screening operation. Before replay or inference, an adapter reopens the evidence derivative and
reproduces the child's full byte digest, byte count, and sparse catalog identity; missing, unsafe, or drifted
bytes produce no semantic axis authority and therefore remain an operational hold. Once the current bytes are
proved, the adapter replays an already settled subject/profile operation before invoking its producer, so a
retry cannot buy or obtain a second answer. The adapter supplies that same deterministic operation identity to
the producer, whose own durable execution ledger must also replay it when the producer finished but downstream
axis persistence did not. A new producer answer is projected locally through the immutable authority, and the
exact path-free producer report becomes the private raw evidence persisted by the screening repository.
Producer, source bytes, duration, policy, certification, implementation, result, or rule-map drift is an invalid
operation, never a permissive fallback.

For a safety axis, screening `pass` means that authenticated category evidence is complete; it does **not**
mean the observed content is appropriate for every audience. Complete positive evidence therefore remains an
axis `pass` while its closed observations drive the audience-specific Airworthiness `reject` or `hold`.
Incomplete or unprojectable coverage is an axis `hold`. This separation is load-bearing: mapping every positive
fact to an axis `reject` would bypass the audience policy and incorrectly prohibit facts that one profile may
allow or review. The final aggregate still requires all four axis operations to complete and Airworthiness to
pass before a child can advance.

Each axis record binds the complete child subject described above, its outcome, and the evaluator's policy,
certification, implementation, and evidence-contract profile, plus the SHA-256 of its private bounded raw
ledger or measurement bytes. The axis record publishes only after those raw bytes are durable. A child
screening coordinator requires exactly one named evaluator for each axis, makes the path-free subject durable
before the first call, calls them serially without showing one evaluator another's answer, rejects subject or
profile drift, and persists each axis before moving to the next and the validated four-axis aggregate before
returning it. Each evaluator owns repeat-safe settlement of its exact operation;
a retry after a later persistence failure replays the same closed authority-bound result instead of repeating
a possibly billed call. The aggregate's `assessedAt` is the latest of those five immutable axis assessment
times, not a fresh coordinator clock reading, so a crash after aggregate publication but before its caller
records the digest reproduces the same aggregate identity on retry. An operational error means no trustworthy result exists and creates an operational
hold. Reject and hold are durable domain answers, not retryable absence.

The ordinary ingest ladder has one rendered-child screening rung immediately after split. It does
not apply to a top-level source; every child with parent lineage reaches it after transcode has
published the final evidence and playback derivatives. The rung writes a versioned portable pointer
to the exact subject and aggregate only after both are durable. A missing coordinator, malformed
sidecar, operational failure, non-passing result, absent production release authority, or failed
terminal replay resolves to review or rejection and never falls through to metadata enrichment or
the compatibility score gate. The pipeline's exhausted-failure policy parks this rung just as it
parks admission; a panic or store outage cannot turn screening into a skipped stage.
At startup, a data-selected compatibility repair holds and rewinds any pre-rung child that had
already advanced beyond screening and lacks a completed screening stage record. It holds the clip
before rewriting its pipeline row, so an interrupted repair cannot leave the legacy child airable;
already certified children, top-level clips, dismissed rows, and hard file rejections are untouched.
The administrator filing endpoint enforces the same boundary for materialized children: it may
settle a later human metadata decision only when the pipeline row proves that screening completed
and advanced. A review-shaped screening result is recorded as done at its current rung but cannot
be filed, because a person's generic catalog action is not a substitute for the release authority.

Production starts this rung with a **qualification runtime**, rather than leaving the whole coordinator
absent until every independent safety lane is certified. The runtime uses the deterministic playback verifier
immediately and installs one explicit non-authorizing evaluator for each
of visual, spoken, and written safety. Each non-authorizing evaluator reopens the exact evidence derivative,
records its measured artifact identity, and returns a durable `hold` naming the missing axis certification;
it never emits `pass` or `reject` and performs no inference or provider call. Its profile hashes identify
the built-in qualification policy, unavailable-certification marker, and implementation—not a safety
certificate. Consequently the four-axis aggregate and per-axis evidence are exercised on real children,
objective playback failures retain their closed outcomes, and no child can reach enrichment or release while
one of the three safety authorities is absent. An unavailable evidence root or runtime constructor leaves the
existing missing-coordinator hold in place. The private
content-addressed repository lives under the filler root's excluded `.loomarr/segment-screening` tree.

#### Complete-source visual-sensitive-content authority (V68)

The `visual_safety` axis is a complete-source safety question, not another use of the taxonomy vision
rung. Four sampled frames, scene-selected OCR, logo classification, or confident multimodal prose cannot
establish absence of restricted imagery between observations. One deep visual-safety module therefore owns
source validation, the deterministic coverage plan, adapter observations, conservative reduction, and the
canonical result. Its external interface accepts one path-free source authority plus a machine-local path
and returns one content-addressed, non-admitting result. Callers never sequence detectors, interpret their
confidence, or combine votes.

The source authority binds an opaque source id, complete source SHA-256 and byte length, measured duration
and video-stream identity, the private visual-policy SHA-256, measurement time, and the exact probing-tool
identity. The local path is only a locator: the module snapshots a regular non-symlink file and reproduces
the authority before extraction or spend. A source without a complete decodable video stream is a coverage
hold. No filename, provider metadata, catalog tag, generated description, or parent classification enters
the visual verdict.

One versioned coverage profile declares the maximum source duration, target observation interval, maximum
per-frame timestamp drift, maximum observation count, and the shortest restricted display duration it
claims to cover. The deterministic planner always includes the first and last representable milliseconds,
orders every requested timestamp, and rejects a profile whose claimed display floor is not strictly greater
than its maximum planned or observed gap. Extraction evidence binds every requested and observed timestamp,
decoded frame digest and dimensions, decoder identity, complete decode outcome, and the plan digest. A
missing, duplicate, corrupt, out-of-order, excessively drifted, or unobserved frame makes coverage incomplete;
it is never reconstructed as a negative. Certification locks the profile and display floor—production may
use a denser compatible plan but may not claim a shorter floor without a new certificate.

Visual observations use the closed states `prohibited`, `no_signal`, `incomplete`, and `failed`. Every
observation binds the exact source authority, policy, producer family and implementation, certification or
capability identity, immutable private evidence digest, assessment time, and zero or more source-relative
intervals. Public evidence carries only opaque policy-match ids and reason codes; it never repeats a
restricted description. A portable complete-coverage lane is mandatory. A bounded direct-video lane may
run only for a declared escalation, and an optional Apple Sensitive Content Analysis lane may contribute a
source-level observation only when its signed entitlement, enabled analysis policy, framework/OS identity,
input digest, completion, and result are all bound. Apple supplies no timestamp and its negative is neither
portable coverage nor a standalone clear. Unsupported platform, missing entitlement, disabled analysis,
adapter error, stale capability, malformed timing, or identity drift is `incomplete` or `failed`, never
`no_signal`.

Reduction is asymmetric and contains no majority vote. Any valid prohibited observation quarantines the
source and every mapped derivative, even when another lane reports no signal. With no prohibited
observation, missing mandatory portable coverage, an incomplete or failed attempted lane, malformed
evidence, or producer/certification drift holds. Only complete valid negative observations may produce
`no_prohibited_visual_observed`, which remains evidence rather than ingestion, scheduling, training, or
broadcast authority. Source projection is all-or-nothing: an incomplete mapping or a derivative that escapes
one positive source verdict fails the operation closed.

Certification uses rights-cleared, source-family-disjoint positive and clean controls and locks truth before
candidate execution. Positive slices include short exposure, cuts, crop/letterbox, transcode, VFR/CFR,
animation, monochrome, low light, multiple people, compilation placement, and damaged tails. Clean slices
include programme, advertising, animation, historical graphics, and visually busy material. Every labeled
positive at or above the locked display floor must quarantine; one miss fails. With zero misses, at least 59
independent positive source families are required for a one-sided 95% exact lower recall bound of at least
95%. Clean false positives must remain at or below the predeclared per-slice bound. Generated controls and
transformed derivatives exercise implementation behavior but never inflate the independent-source
denominator. One independently acquired, rights-reviewed still work may be carried through the complete-
source evaluator by one deterministic lossless audiovisual wrapper and count as exactly one source family;
the wrapper is transport, not generated semantic content. Every alternate duration, crop, encode, or other
carrier of that work shares the same family and adds no independent sample. Still-work families supplement
rather than replace natural programme, advertising, and animation video in the declared clean slices. Until
that immutable certificate and a separate production release authority exist, the
qualification evaluator continues to return `visual_safety_not_certified` and no catalog behavior changes.
Every visual suitability flag enabled by a production audience policy additionally requires its own
predeclared positive population, severity/context boundary, and confusable hard negatives. A corpus containing
only adult nudity can certify only that narrow flag; historical battles, weapons, accidents, corpses, animal
harm, frightening imagery, substance depiction, and regulated-product promotion are separate measured slices,
not informal examples folded into a generic sensitive-content score.

The file evidence adapter gives each subject/profile pair one deterministic operation identity pointing at
its settled axis record. Once present, a different result cannot replace it; paid evaluators still own their
pre-call reservation journals so a crash before publication cannot repeat an ambiguous charge. The
deterministic playback evaluator reopens the final playback file, verifies its full SHA-256, byte length, and
sparse catalog identity, reprojects the at-most-64-MiB non-symlink sidecar, and then reopens the playback file
before deciding. Its subject requires valid derivative quality evidence and, for a split child, exact agreement
between the playback manifest and post-rewrite conditioning measurement. Existing derivative QC supplies
complete decode, seek, keyframe, A/V, loudness, and fast-start evidence; the conservative media-quality rule
rejects objective dead air and holds long black, silent, or frozen spans. A sparse-hash match alone never
passes because middle-of-file changes must be caught by the full digest.

Provider-declared licence values remain exact provenance metadata on Sources, candidates, sidecars, and clips.
They are not an inference input, acquisition preference, screening axis, admission claim, publication receipt,
or user decision. Missing metadata means only that the Provider supplied none. Private household playback does
not ask an operator to adjudicate copyright, and the runtime contains no current-use rights authority, grant,
withdrawal, or remediation path. Project-owned corpus acquisition and redistribution retain their separate
rights controls because those govern what the Loomarr project itself may copy or distribute.

Screening aggregates, provider-neutral axis records, operation identities, and opaque raw evidence live
separately in a private content-addressed repository. They support measured classification and configured
household protection without becoming a second approval system. A positive objective media failure or a
configured positive safety finding can make a Clip not usable. An unavailable optional model, incomplete
certification corpus, missing classification fact, or absent audit evidence cannot block an otherwise valid
enrolled Clip. Any screening fact used by runtime still binds the exact playable bytes and profile; stale or
identity-drifted evidence is ignored rather than treated as a pass.

**Replacement contract (revised for household beta, 2026-09-13).** The new filler pipeline replaces
the previous pipeline entirely. There is no live compatibility fallback or dormant publication mode.
Missing required bytes, failed media checks, or absent Enrollment authority leaves material held with
an attributable reason; missing optional classifiers, certification, or audit evidence does not.
A structure-assessment failure, including a source outside the reviewed duration envelope, keeps
the source and its pending proposal at the split review rung. It must not exhaust ordinary
non-fatal retries into a terminal Complete disposition. Cancellation preserves resumable work and is
not an assessment result. No such failure may create children or confer publication authority.
No confidence score or detector-only proposal may substitute for the new pipeline's required runtime
checks. Source selection is not inferred trust: it is the explicit Enrollment authority for private
household use. A human is asked only for a genuine unresolved product choice. This contract
supersedes earlier rollout language in this section.

The split stage applies only the complete-plan materialization gate. A proposal without a verified
complete-timeline decision and matching materialization authority remains reviewable and cannot
create children automatically. The compatibility calculation may remain a non-authorizing comparison
for measurement. Its immutable shadow record binds proposal, source, assessment, structure authority,
and policy identities plus exact materialize, hold, and discard spans. Recording agreement never
activates a slice. Failure to persist a required comparison blocks unattended materialization; it
never selects the comparison result as a fallback. Screening identities do not enter this ledger
because no child playback derivative exists yet. Admission comparisons remain development evidence
only. Runtime publication belongs exclusively to the terminal-ready transaction.

Structure-validated children reuse the V66 derivative publisher and are prepared as one replacement generation.
Their playable and evidence derivatives are built from the exact reviewed source intervals, not from an
older playback rendition. The parent, assessment, observations, and prior complete child generation remain
intact until every replacement child and durable lineage record validates and the generation switch commits
atomically. New children remain held through derivative production, the five screens, enrichment, and
terminal readiness; filesystem visibility is not broadcast permission. A crash, partial re-split,
derivative failure, or screening failure cannot replace a complete generation with a partial airable one.

Certification is separate from production assessment. Its rights-cleared corpus is split by source family
and contains declared/chaptered truth, deterministically authored compilations, programme excerpts with
inserted spots, long single units, same-brand joins, wordless units, bumpers/station IDs, slivers, damaged
tails, and adversarial internal scene/music changes and standard-duration non-joins. It scores source
structure, under-splitting, over-splitting, boundary error at predeclared tolerances, segment purity,
timeline coverage, role accuracy, abstention, operational failures, and worst source/signal slices.
Under-splitting and over-splitting remain separate safety results. No automatic slice may contain an
observed cross-unit merge, unexplained timeline gap, or published interval with an unresolved role. New
logic is measured against retained V34/V54 proposals without allowing their compatibility outcome
to create children. Automatic materialization expands only for the exact
predeclared slices whose locked point estimates and confidence bounds pass. Model training is deferred
until this measurement identifies a specific residual error class and a rights-cleared training split.

**Conditioning measurement is evidence, never authority (V64).** The media-tools module can inspect
one bounded local regular-file artifact and, optionally, compare it with one local regular-file
parent artifact plus up to eight intended cut intervals. A request has at most one parent; individual
cuts cannot select different parents. It reports container duration; the closed audio/video stream
kind and index; explicitly available stream start and duration; exact rational video cadence;
explicitly available audio/video start and end skew; measured integrated loudness and true peak;
and the same normalised black/silence/freeze intervals used above. A parent comparison reports
independently observed, signed start and end errors for each presented stream edge and stream index.
A fully silent presented audio stream remains measurable: true peak is represented by a closed
finite/negative-infinity state, never a non-finite number or an unavailable stream. It never
substitutes the requested cut or container duration for an edge it could not match: unavailable is a
first-class result, not zero.

This inspection has **no verdict, threshold, rewrite, admission, filing, or provider authority**. It
does not mutate the operator's media. It opens each input once without following links, copies that
validated regular object into a private per-request snapshot, and runs every container, detector and
packet inspection against those same captured bytes; replacing the caller's pathname after validation
cannot mix evidence or redirect a later invocation to a pipe or device. Every pathname component is
resolved without following a symbolic link or Windows reparse point; rejecting only the basename is
not sufficient. Resource ceilings are part of its interface: each artifact or parent snapshot is at
most **1 GiB**, an artifact has at most eight streams, a request has at most eight intended cuts, and
media is at most 120 seconds. The byte ceiling permits about 71 Mbit/s across the full 120-second
window—well above the compressed filler inputs this seam measures—while bounding a request with both
artifact and parent to 2 GiB of private snapshots. The opened object's size is refused before copying,
and the copy independently caps bytes and observes cancellation so a concurrent size change cannot
evade the limit. URLs, pipes, devices, directories, missing paths, and other non-regular inputs are
refused before any media tool runs. A metadata-only preflight compares the container's exact decimal
duration with 120 seconds before any frame enumeration; exactly 120 seconds is valid and every rational
value above it is refused without millisecond rounding. The decoded-frame probe is independently
bounded to a 120.000001-second read interval so a sparse-frame input cannot turn small output into
unbounded decode work, and caller cancellation remains the returned cancellation identity. Output and
edge searches are bounded too. Conditioning selects the lowest global stream index of each presented
kind, maps those exact video/audio streams into
ffmpeg, and treats each selected stream's decoded-frame EOF (timestamp plus frame duration, or decoded
audio sample count at its sample rate) as its detector timeline; that decoded EOF is checked exactly
against 120 seconds before conversion to the returned integer milliseconds:
black/freeze end at video EOF and silence ends at audio EOF, never at a longer container or sibling-
stream duration. Each selected stream is probed independently under the same byte cap. The compact
frame parser owns only the ordered timestamp/duration-or-sample-count scalar prefix requested from
ffprobe; codec tags and side-data that ffprobe appends despite that projection are bounded by the
same cap and ignored because they cannot contribute timing evidence. A selected stream without an
independently available positive duration fails closed
before detector execution. Detector events must match the anchored ffmpeg blackdetect, silencedetect,
or freezedetect line grammar and fall inside that selected stream's timeline. Every detector kind is
bound to one exact filter instance, address, and modern/legacy grammar for the measurement; distinct or
interleaved identities cannot contribute parts of one event. Loudness likewise requires exactly one
anchored integrated-LUFS summary and one anchored true-peak summary; duplicate, conflicting, or
suffixed summaries fail closed, including duplicate digital-silence peaks. Every completed event
carries the detector's duration token, which must equal end minus start within **exactly 1 millisecond**,
compared as decimal rational values rather than binary floating point, to allow only ffmpeg's decimal
rendering tolerance. Malformed, oversized, incomplete, duplicate, repeated, inverted, or suffixed tool
output fails closed rather than producing guessed precision. Conditioning appends exactly one black
frame and one white frame before the video detectors, guaranteeing a changed frame even when the
artifact ends frozen on either color.
The parser bounds their possible timestamp extension from the exact final decoded-video timestamp
step (falling back to the declared cadence when fewer than two frame timestamps exist), because ffmpeg
uses that terminal step for a sparse-VFR input rather than inventing a nominal cadence. A detector end
may differ from the independently decoded EOF by at most one millisecond of decimal rendering before
that bound is applied. Detector timestamps contributed only by those two terminator frames are clamped
away at the selected video stream's measured EOF and can never become evidence. These are execution limits, not content-quality
policy. V64 ends at this returned evidence: it does not invoke the filler pipeline, persist to the
store or sidecar, expose an API, or decide what may be reviewed, certified, filed, admitted, rewritten,
selected, or played. Those application-journey contracts remain tracked by issue #634 rather than
being implied by this measurement seam.

**Conditioning evidence joins the durable filler pipeline at its existing boundaries (V65).** A
confirmed split writes the composite's content identity and the operator-reviewed intended start and
end beside every child. That lineage survives proposal consumption and a catalog rebuild; a child is
never matched to a parent by filename, ordering, or duration. At the child's transcode rung, Loomarr
measures the cut bytes before rewriting and the hidden staged mezzanine after rewriting, comparing
both artifacts independently with that one preserved parent interval. The replacement sidecar carries
the lineage and both immutable measurements with the transformed bytes. Catalog reconstruction restores
the child's parent identity from that same sidecar instead of flattening it into a top-level clip.
Top-level clips are outside this compilation-conditioning slice. `normalizedLufs` remains only an
idempotency/target marker: measured integrated LUFS and true peak come from conditioning evidence and
are never inferred from that marker.

Split confirmation first snapshots the catalogued composite into its private hidden staging area and
requires that snapshot and the still-owned source name have the catalogued content identity. Every cut
therefore reads one validated immutable snapshot; source replacement fails closed before any child is
published or durable review state is consumed. Split confirmation publishes lineage before media. Each
reviewed child is cut into that hidden staging area on the filler filesystem, hashed there, and given its
final-bound lineage sidecar before one atomic media publication makes it visible to scanning. Existing
content-addressed child bytes are reused only after their measured identity matches the staged child.
A sidecar or identity failure leaves no visible child media. A concurrent
scan can therefore observe either no child or a child with valid parent identity and a positive intended
interval, never an unbound top-level clip. Durable `conditioningLineage.childHash` is the reviewed
stream-copy child's content identity; `beforeRewriteHash` and `afterRewriteHash` bind the two persisted
measurements to the exact bytes they describe after the replacement is re-keyed.

The catalog identity remains V38c's bounded sparse `ClipID`; V65 does not silently redefine it as a
full-file digest. At every conditioning ownership or reuse boundary, however, Loomarr compares the
complete bounded byte streams under one cancellation-aware open: composite source against its private
snapshot, existing child against the staged cut, child and retained parent against their measurement
snapshots and still-owned names, and an interrupted transformed output against the current hidden
output. Matching sparse identities alone never proves those byte pairs equal.

Complete confirmation cuts and sidecars for the entire reviewed generation before publishing any child. It
then durably writes every child as held and tombstoned, persists reviewed tags, and enrolls every
child in the non-runnable review disposition. The final-bound sidecars and media links are published
as a reversible prerequisite: a concurrent scan may see only bound held children, and any later
publication or completion failure removes every new media link and final sidecar from that attempt.
One store transaction is the all-or-none durable completion boundary: it marks the retained parent
composite, files and unholds that parent's clip and pipeline state, activates every replacement child
pipeline at the probe rung, consumes the reviewed proposal, and selects the replacement generation
while preserving channel-pinned children. Until that transaction commits, the proposal, parent
review disposition, and prior selected generation remain unchanged, and replacement pipelines cannot
run against reversible media. Completion compare-and-swaps a still-held parent and exactly one parent
pipeline from `review` to `filed`; a missing, already-filed, or otherwise changed parent state rolls the
whole transaction back rather than treating a zero-row transition as success. The outer application adapter performs no fallible parent-filing write
after inner confirmation, so a successful response cannot be followed by an ambiguous post-commit
error.

Confirmation is serialized by a durable, expiring proposal claim acquired before any final sidecar
or media name is published. The opaque token fences partial proposal updates and the complete store
transaction; ordinary proposal updates, rejection, and replacement cannot bypass an active claim.
A clean exit releases only its own token, while a crashed owner is recoverable after the durable
deadline. The live owner renews immediately before sidecar and media visibility. Every child sidecar
also records the current publication token solely as filesystem rollback ownership: a recovered
confirmer takes that ownership after exact byte and lineage validation, and a stale predecessor may
remove only media whose sidecar still carries its own token. Partial confirmation publishes only
reversible held/tombstoned children whose pipelines remain in `review`, then one claimed store
transaction shrinks the proposal, activates exactly those child pipelines, and releases the claim.
A failed claimed update or child activation rolls those media links back and leaves every replacement
pipeline non-runnable. Catalog and sidecar preparation for the complete batch precedes the first media
link; later cut, metadata, catalog, pipeline, or publication failure therefore exposes no ordinary
child, and any briefly visible bound child remains held/tombstoned and non-runnable through concurrent
Sync. Rollback removes media before its owner-marked sidecar. If media removal fails, the lineage and
publication token remain durable quarantine and recovery evidence; rollback never launders surviving
bytes by deleting that evidence.

Until conditioning evidence is complete, catalog reconstruction also restores a lineage-only child
as held rather than inferring that operator review was conditioning approval.

This wiring does not grant conditioning policy authority. Measurement errors, missing required stream
facts, unavailable cut edges, or evidence that cannot be associated with the preserved parent and
stream identities hold the child for review; they do not manufacture zeroes, silently admit it, or
rewrite the source in place. Existing media-quality and admission policies consume their own facts and
retain their existing authority. Transcoding publishes only a fully measured staged replacement, keeps
the source bytes untouched until the content-addressed replacement and its evidence are durable, and
never places filler in the prepared-program rendition cache.

A conditioned replacement sidecar carries a closed `pending` publication record binding source and
target content identities before the target media name becomes visible. Catalog reconstruction treats
that record as a conditioning hold regardless of otherwise complete evidence. The pending record stays
through `ReplaceClipIdentity`; only after the durable re-key succeeds may the stage atomically clear it
and return the measured media-quality disposition. A crash before re-key leaves the source row as the
only identity owner and retry validates/reuses the pending target; a crash after re-key may clear the
record only when the source identity is absent and the target row/evidence still match exactly.
Concurrent Sync can therefore discover a pending target only as held, never as an ordinary duplicate
or playable clip. Missing, malformed, mismatched, or ambiguously owned pending state remains review.

Before the catalog re-key, the pre-rewrite sidecar durably records the exact replacement identity as
`supersededByHash`. Successful cleanup removes those obsolete bytes and sidecar. If cleanup fails, the
source remains preserved but catalog reconstruction treats that explicit saga marker as quarantine and
holds it out of rotation; a cleanup error can report continuation only after that non-airability fact is
durable.

The application boundary validates the inspector's returned shape before trusting it. A conditioned
child requires a positive bounded container duration; exactly one globally indexed audio stream and
one globally indexed video stream; available start and positive duration for every stream; a positive
exact cadence for every video stream; available start/end A/V skew; one measured loudness summary with
a closed finite or digital-silence true-peak state; and exactly one cut comparison containing available
start/end errors for every presented stream identity. Missing, duplicate, unknown, non-finite, or
out-of-shape facts are unavailable evidence, not a content-quality threshold. Cancellation follows the
same review path before publish rather than consuming generic transcode retries.

A re-encode necessarily changes encoded packet hashes, so post-rewrite packet hashes cannot be matched
directly to the parent without pretending codec output is content identity. Loomarr preserves the raw
`afterRewrite.cuts` measurement unchanged, including unavailable direct packet edges, and persists a
separate derived post-rewrite parent-edge projection as `derivedParentEdgesAfterRewrite`: the
independently matched pre-rewrite parent error plus the measured start/end timeline delta between the
same closed stream kind/index before and after the owned whole-clip transcode. This field is derived,
not directly packet-matched evidence. The projection uses checked integer arithmetic, not content
similarity or a tolerance. Missing stream identity, timing, pre-rewrite edge, or arithmetic safety
makes the projected edge unavailable and holds the child for review.

The sidecar is also the restart record. A child whose current mezzanine sidecar contains valid lineage,
complete before/after conditioning, and media-quality evidence reuses those facts without decoding or
rewriting again. The rung revalidates the persisted shape and still resolves the parent by its preserved
content identity; missing or malformed restart evidence holds for review instead of being treated as a
completed transcode or paying for another generation of loss.

Restart and interrupted-publication recovery are identity-bound and fail closed. Lineage requires
64-character lowercase content hashes and `0 <= intendedStartMs < intendedEndMs`; the parent hash must
resolve to a retained composite. Conditioning measurements use the mediatools validation contract: one
globally unique video stream index and one globally unique audio stream index, exact equality of the
before/after stream identity sets, complete timing/cadence/skew/loudness provenance, and sorted, non-overlapping,
duplicate-free detector intervals bounded by the measured duration. Media-quality evidence carries its
closed `evidenceVersion: 1` and `provenance: ffmpeg_detectors` rather than relying on field presence. A
transformed artifact
already present at the target content identity is recoverable only when its sidecar contains that exact
child lineage, complete media-quality and conditioning evidence, and evidence equal to the measurements
built by the current attempt. A top-level artifact, a different parent or interval, corrupt/incomplete
metadata, or a failed catalog replacement preserves the source and holds the child for review.

Catalog reconstruction treats valid child lineage as authority that the retained parent is a composite
and therefore non-airable, regardless of scan order. Blank or malformed lineage holds the discovered
media out of top-level airability; it never launders a damaged child sidecar into an ordinary clip.
Sidecar reading is tri-state at this boundary: absence is the ordinary hand-dropped case, valid JSON
with no conditioning keys is ordinary top-level metadata, and present unreadable JSON, wrong-typed
Loomarr conditioning state, or a conditioning-lineage object that omits, nulls, or gives the wrong
JSON primitive type to any required identity or interval member is damage that remains held without
being overwritten. Required conditioning-evidence hash scalars follow the same strict raw-JSON check
before typed decoding. An explicit numeric zero remains present and type-valid, distinct from omission
or null, and later lineage semantics decide whether the interval is valid. A conditioned child clears its reconstruction hold only after the completed scan
generation contains its exact parent and that parent is a valid retained top-level clip: its sidecar is
readable, it is not itself conditioned lineage or held by damaged conditioning state, and the child
derives its composite marker from that clean parent. The ordinary parent starts with
`is_composite=false`; valid child lineage is the authority that changes it to `is_composite=true`.
A corrupt, conditioned, missing, or self-referential parent never becomes authority merely because a
child names its sparse hash; child-first and parent-first traversal therefore cannot change the decision.

Conditioning cancellation is checked after every inspector, probe, transcode, and complete-byte
comparison boundary, including after the comparison loop itself, and immediately before sidecar or
media publication and every durable write. A cancelled child returns to the
existing review/hold path with cancellation recognized by `errors.Is`; no replacement sidecar, media,
re-key, source cleanup, or later durable operation may follow cancellation.

Derived post-rewrite parent edges are a media-tools operation beside conditioning validation, not
filler-owned arithmetic. Its one narrow interface validates the measurement pair, preserves raw
after-rewrite edge unavailability, and returns either checked derived edges or an unavailable-evidence
error; callers do not duplicate stream matching or millisecond overflow rules.

When loudness normalisation is enabled, the existing transcode path applies the configured target to the
hidden staged output and conditioning independently measures integrated LUFS and true peak both before
and after that rewrite. The target-valued `normalizedLufs` marker alone is never completion evidence;
restart still requires the versioned measurements and every other conditioning invariant above.

Acceptance follows the same seams as production. A hermetic application journey proves parent probe
and the existing composite-transcode skip, reviewed split confirmation, child enrolment, child
probe/transcode/quality, durable lineage, pod reconstruction, and the scheduler's exact
`Airing.Identity`. Tagged ffmpeg acceptance
separately proves that the selected child is decodable, begins at the requested mid-break offset, and
returns to decodable program content at the scheduled boundary.
That tagged proof resolves each segment through the production scheduler/Airing path, encodes finite
blocks with the production `ProgramArgs`, and joins them through the internal playout block mux. A test
authored `trim`/`concat` graph is not evidence for the behavior of Loomarr's playout assembly.
Those playout observations are acceptance evidence, not new conditioning thresholds or admission
rules. Store-schema and API exposure are deliberately outside V65.

For each reviewed split child, the pre-rewrite conditioning measurement must contain one complete
packet-matched `Cuts` record for the exact interval carried by that child's immutable lineage. The
record stores actual-minus-intended start and end errors for every presented stream; unavailable or
mismatched edge evidence holds the child for review. This is evidence wiring and persistence only:
V65 defines no acceptable error threshold and performs no automatic correction.

**Language is a job, not an inline check**, and it REJECTS rather than holding (maintainer,
2026-08-03) — consistent with the other two gates, which drop a file at the boundary and leave a
log line rather than a queue entry.

⚠ **It runs in the BACKGROUND, after the clip is already catalogued.** A clip enters the catalog on
the scan and the job fills in its language afterwards, acting on the answer then. Inline was the
simpler code path and is not viable: on the local backend a 100-clip folder becomes a ~9.5-hour
scan on arm64 (see the timings below), and the sync timer would overlap itself. This is the same
shape the AI tagger already has, for the same reason.

⚠ **Only where there is audible speech to judge, and SILENCE NEVER REJECTS.** A wordless visual
spot has no language, and those are often the best filler — so "no speech detected" means keep,
never drop. Only confident non-target *speech* rejects.

⚠ **A model handed silence does not decline — it GUESSES, and arbitrarily.** This is the failure
the first live run produced, and it deleted two real clips. Two recorded ad breaks (867s and 978s)
were sampled at their first ten seconds, which on a long recording is leader: tape run-up and dead
air measuring **−70 LUFS**. Asked what language that was, the model answered `ar` for one and `es`
for the other, and the gate tombstoned both. Re-asked about the identical span later it said `en` —
so the answer is not reliably *wrong*, it is reliably *unpredictable*, which no amount of prompt
tuning makes safe to act on.

Two defences, and they are deliberately independent:

- **An ordinary commercial is inspected in full; longer recordings use the middle 30 seconds.**
  The bounded middle is past opening leader while a complete spot carries enough speech for a
  stable decision. This fixes *where* we look: long recordings measured −25 and −28 LUFS in the
  middle, and nearby ten-second cuts of a live Spanish Jersey Mike's spot alternated between
  English and Spanish while its complete 30-second speech was correctly identified as Spanish.
- **A span below a loudness floor is never asked about at all.** `−50 LUFS`, measured with the
  same `ebur128` the loudness half of V40 uses. This holds *wherever* we land, including on a clip
  that is genuinely silent throughout. The floor leaves wide room above the quietest real clip
  measured in this catalog (−32.6 LUFS), because treating a quiet advert as silent would be the
  same bug in the other direction.

Both backends also require at least one non-empty transcribed segment before accepting a detected
language. Silence or music therefore returns `none` even when a recognizer reports a guessed
language code. The loudness floor remains an independent guard that avoids sending obvious silence
to either engine.

**Two backends behind one speech-recognition choice.** Language detection and optional timed
transcripts use the same engine; asking users to configure a second audio-capable chat model for
language made one clip depend on two unrelated services and excluded dedicated ASR servers.

| | `asr.provider = whisper` (default) | `= hosted` |
| --- | --- | --- |
| Engine | vendored `whisper-cli` + `ggml-small.en.bin` | a timed speech-to-text model through a dedicated or inherited OpenAI-compatible endpoint |
| Per clip | bounded background inference over at most 30s of audio | one bounded network inference over at most 30s of audio |
| Cost | free | provider-dependent speech-to-text cost |
| Offline | yes | no |

The connected option uses the standard OpenAI-compatible multipart `/audio/transcriptions` route
with `verbose_json` segment timing and the response's detected `language`. `ASR_URL`, `ASR_MODEL`,
and `ASR_API_KEY[_FILE]` may point it at a dedicated service; a blank URL reuses the selected hosted
AI service for providers such as OpenRouter. An explicit speech URL never inherits the main AI key.

⚠ **An unavailable backend is a skip, not a retry.** If the selected detector has no model,
executable, media tool, or hosted client configured, the language rung records why it did not apply
and the clip advances immediately. Backoff is reserved for a backend that was ready enough to run
and then failed, or one that ran but returned no trustworthy answer. Retrying a missing model at
5- and 30-minute intervals cannot make it appear; it only turns an optional quality gate into a
35-minute ingest delay. A later configuration change can use the ordinary rewind action to re-run
the rung for clips that already passed it. The Filler settings surface shows the same unavailable
reason beside a disabled expected-language control while retaining its desired value; a default of
`en` must never look active when the multilingual model or another selected-backend prerequisite is
missing.

⚠ **NOT Ollama, and this is the trap worth naming.** "We already run a local LLM, so we do not need
whisper" is the reasonable inference and it is wrong: Ollama has no audio input path at all. Probed
against the live dev instance (2026-08-03), its models report `completion`, `vision`, `tools`,
`thinking` — there is no `audio` capability, and `vision` is images only. Local audio means
whisper; the hosted option is what Ollama cannot be.

⚠ **The consequence for arm64, stated so nobody has to discover it:** 341s per clip is not usable,
so an arm64 install effectively has only the hosted path. The feature is off by default rather than
degrading silently there.

⚠ **Local is the default, and a connected service is an opt-in that may cost money and leaves the house.** Sending
clip audio to a third party is a change in posture, not a performance tweak — the same reason §8.1
defaults to local Ollama and makes hosted a deliberate choice with a key. It is also the first
feature that spends money per clip.

The seam is `MediaTools.Transcribe(ctx, file, startMs, endMs)`, which already exists for
compilation splitting. A hosted detector is a second implementation behind it, so the job, the
reject rule and every test are identical whichever backend answered.

⚠ **Brightness is deliberately measured by nothing and fixed by nothing.** Sample clips ranged
YAVG 64–127 against a mid-grey of 128, and the dim end is what an eighties VHS transfer genuinely
looks like. Auto-brightening would invent a picture the source never had — and unlike loudness it
is not recoverable at playout, because the correction would be baked into what the viewer sees with
no original to fall back to.

### Richer metadata: transcripts, brand, and vision (V44)

Three jobs join the background family above (language, tagging, normalisation), all sharing its
shape — opt-in, timer-driven, batched, running **after** the clip is catalogued so a slow pass never
blocks the scan, and recording **every** outcome so a clip is never re-processed forever.

**`transcribe` — persist what a clip says.** The splitter already runs Whisper to rescue over-long
segments (`MediaTools.Transcribe`, §10 V34), then discards the result. V44 stops discarding it: a
confirmed segment carries its transcript onto the clip row, and the `transcribe` job backfills the
rest. It is **selective by design** (the on-demand decision, §10 above): it transcribes a clip only
when its source text is *thin* (an empty or near-empty sidecar description — the archive.org common
case) **or** it is still untagged after a text-only pass. A clip whose source already describes it
never pays for Whisper. The transcript is a first-class metadata field: searchable, and the richest
input the tagger gets — a cereal advert with no description still *says* "Kellogg's".

⚠ **Same backend seam and same arm64 reality as the language gate.** Transcription is
`MediaTools.Transcribe`, whisper local (~341s under QEMU) or the §8.1 hosted audio model — so an
arm64 install effectively has only the hosted path here too, and the job is off by default.

**`brand` — grounded, like era.** The tagger gains a `brand` field (the advertiser: `Kellogg's`,
`Ford`). It obeys the era rule generalised: **a brand is accepted only when it appears literally in
a text signal** (filename, sidecar, or the now-persisted transcript). A brand the model proposes but
cannot ground is dropped, never persisted — the same anti-fabrication asymmetry (§8) that keeps an
inferred era out of the catalog. No brand is the honest common case, not a failure.

**`vision` — read the clip nobody narrates.** A wordless spot (a car on a coast road, a station
ident) gives Whisper nothing to hear and the tagger nothing to read, yet the *image* is full of
signal: an on-screen logo, visible text, a black-and-white transfer that dates it. V44 adds a visual
tier that samples a few keyframes and asks a vision model for `brand`, `category`, and the
`visibleText` it can read off the frame.

⚠ **The grounding rule holds for pixels too — for BRAND and ERA.** A model reading `KELLOGG'S` off
a box on screen is *grounded* — the text is literally in the frame — exactly as an era is grounded
when its year is in the filename. A brand or era the model asserts without the frame showing it is
dropped. The `visibleText` field is what makes this auditable: it is the on-screen text the model
claims to have read, and a brand not supported by it does not persist.

⚠ **CATEGORY is grounded differently, and V54b corrects it (this paragraph used to include
`category` in the rule above).** A category is checked against the TAXONOMY — the model must name a
taxon that exists, resolved through `forest.Resolve` — but it is **not** required to appear in
`visibleText`.

The distinction is what kind of claim each field makes. A brand and a year are **specific facts**,
and a model that emits one it did not see has fabricated a fact that will be wrong in a definite,
checkable way — `KELLOGG'S` on a Ford advert. A category is a **judgement about imagery**:
classifying a toy advert as `toys` by *seeing toys* is not fabrication, it is the entire reason a
vision tier exists. Requiring the word on screen does not make that judgement more honest; it
restricts it to adverts that happen to print their own genre.

⚠ **Measured, because the old rule was close to unsatisfiable.** On a real 37-segment reel
(2026-08-13) with a vision model answering correctly every time, the on-screen-text condition
admitted **0 categories**. `era` grounded once — `1995`, from the visible text
`"WAGA-5/Fox Commercial Breaks (2/5/1995)"` — while `psa`, correctly judged, was dropped for not
being spelled out on the frame. Since `segmentVerdict` refuses any segment with no audience and no
category (`RejectUntagged`) *before* the boundary-confidence check, the rule made split auto-confirm
structurally impossible and the whole confidence ladder unreachable. The unit test hid it by
choosing `{"category":"toys","visibleText":"TOYS R US MEGA SALE"}`, the rare case where the genre
IS printed.

⚠ **The taxonomy is what keeps this grounded rather than open.** The model cannot invent a
category: an unresolvable one is still dropped, so the vocabulary — not the frame's text — is the
constraint. Brand and era are unchanged and still require the frame.

⚠ **Keyframes come from `ffmpeg` stills, not the dHash frames.** `FFmpegArtwork` already produces a
viewable 320px JPEG; the `GrayFrames` path is 9×8 grayscale for perceptual-hash dedup and is
useless for vision. A new `MediaTools.Keyframes` seam returns JPEG bytes for several frames across
the clip — the same seam pattern as `Transcribe`.

⚠ **Hosted-first, with a local path — and the local path is the one careful change.** The hosted
implementation uses a separate `AskAboutImages` method building `image_url` content parts with
`data:image/jpeg;base64,…`, **not** a widening of `Message.Content` (that string
is on the hot path of every text request, §8 provider abstraction). The **local** path wires
Ollama's per-message `images` field — Ollama *does* report a `vision` capability (probed live
2026-08-03, §10 quality gate; it is images-only, which is exactly what this needs), so a fully-local
install gets visual tagging too. That is the single V44 change that touches the shared `Chat` path,
so it is guarded by tests proving an image-free text request is byte-for-byte unchanged.

⚠ **A frame-heuristic tier sits below vision and costs nothing.** Black-and-white detection and
aspect ratio (4:3 vs 16:9) are era *hints* an LLM never needs to see — computed from the same
frames, deterministic, no API call. They never override a grounded tag; they seed `suggestedEra`
for a clip that has no other era signal, which a human confirms exactly like an AI suggestion.

⚠ **Vision spends money per clip and can leave the house — off by default.** Like hosted audio, the
hosted vision path is a deliberate opt-in with a key; the local Ollama path keeps it in the house
for installs that want visual tagging without the cost or the egress. Recorded in §14.

Everything the three jobs learn — transcript, brand, visible text — persists to the **sidecar** as
well as the store (§10 V38c, "metadata travels with the clip"), so a catalog rebuild does not
re-run Whisper or the vision model over the whole folder.

### Composites, lineage, and curation-grade metadata (V45 — design)

A clip like **"KCPQ/Fox commercials, 5/28/1996 part 1"** (16 minutes, ~30 adverts) is not one
commercial, but V44 catalogues it as one — it lands `kind=commercial, category=movie_trailer`, a
single wrong tag over a whole break. V45 fixes this at the root and turns the split segments into
clips a channel can be **automatically, confidently curated** to match. Four parts, each building on
V44's signals.

#### 1. Long-source quarantine is first-class at intake (semantic structure moves to V67)

A **confirmed composite** is a recorded break — many independently bounded units in one file. Intake
cannot establish that from duration. It conservatively quarantines a **long-source candidate** (not left
to be mis-tagged or aired as a single ordinary advert) from the cheap deterministic signal the catalog
already has: duration past `OverlongSegmentMs` (§10 V34, 120s). V67 structure assessment determines
whether that source is a compilation, a programme with spots, one long unit, ambiguous, or unusable.
The earlier intake detector also required multiple black/silence
boundaries, but that made `probe` fully decode every long recording and then made `split` fully
decode it again. On hour-scale captures the first pass could consume the whole job deadline before
the owning stage began. Boundary detection therefore belongs only to `split`; a long single-product
infomercial may ultimately be established as `single_unit`, but it is still correctly quarantined
from playout while Loomarr decides. A long-source candidate or confirmed composite is **not airable**:
neither is matched into a pod, exactly like a `held` clip, because airing a 16-minute block as one
"commercial" is the bug this section removes.

⚠ **`IsComposite` is a distinct axis from `Kind`, deliberately, and its persisted name is now broader
than its unresolved meaning.** A confirmed composite's *segments* are commercials/bumpers/PSAs; the
composite itself is a container. Overloading `Kind` with a `composite`
value would make every `filterKinds` call site have to special-case it, which is how a container
leaks into a pod. A boolean the pod filter excludes once (like `held`/`removed_at`) is the safe
polarity. The V67 assessment, rather than this compatibility-named boolean, is semantic authority.

#### 2. Auto-detect, auto-split, review to confirm — and KEEP THE PARENT

Intake flags a composite and **immediately** runs the existing split detection (§10 V34/V43:
chapters → black/silence → transcript rescue → classify → dedup). Segments arrive as a
`SplitProposal` in the review queue — **nothing unreviewed airs** (detection quality is a property of
the source, measured 69–100%, §10 V34), but the operator gets instant proposals rather than a 6-hour
wait.

⚠ **Validated by prototype on the KCPQ clip (scratchpad, 2026-08-07).** Black-frame detection over
the full 16 minutes found 45 fades, yielding **41 advert-shaped segments (mostly clean 30s/45s
durations — the real TV ad grid) with ZERO over-long segments needing transcript rescue.** The
failure modes were all conservative: a handful of 3–8s slivers that squeaked past the `MinSegmentMs`
floor (inter-ad gaps/bumpers) and one or two missed boundaries (a 57s span the prototype confirmed
was two ads — A&W root beer + a Paramount "Phantom" trailer). ~90% correct.

#### Multi-signal boundary fusion (V45 — prototype-driven upgrade of the V34 detector)

The prototype's diagnosis of the imperfections led to a better detector than "black/silence →
rescue-only-if-over-long". Each signal has a DISTINCT, complementary failure mode — measured on the
KCPQ clip:

| Signal | Precision | Recall | Failure mode |
| --- | --- | --- | --- |
| **black-fade + silence** | high | misses soft cuts | broadcaster-inserted separators, but not every ad has a fade → **under-cuts** |
| **scene-cut** (`scdet`) | ~11% (88 cuts / ~10 boundaries in 5 min) | high | fires on every internal camera cut → **over-cuts** |
| **transcript topic-shift** | high (semantic) | catches what A/V misses | fuzzy timing (±2–3s), blind to wordless ads |
| **duration ≈ 30/60s** | — | — | a corroborating prior, not a detector |

⚠ **Scene detection is a corroborating VOTE, never a boundary source — this is the load-bearing
insight.** Its precision is terrible *alone* (a KFC→Snapple cut is pixel-identical to a cut inside
the KFC ad; `scdet` sees pixels changing, not "the advert changed"). Black-fade and silence have high
precision because they are *deliberately inserted by the broadcaster to separate spots* — signal by
design; scene-cuts are an *editing byproduct* — noise by design. So the fusion is:

1. **Black-fade + silence PROPOSE** candidate boundaries at precise times.
2. ~~**Scene-cut co-location VOTES**~~ — ⚠ **NOT BUILT, and it cannot be (V54).** This proposed that
   a strong scene-cut near a candidate raise its confidence. Scene-cut detection was subsequently
   **measured and rejected** for this pipeline — `scdet` fires on camera cuts *inside* an advert,
   the wrong granularity, which is a property of the signal and not a tuning problem (see V34 step 2)
   — and `MediaTools` exposes no such call. The measurement this bullet rests on survives and is
   still load-bearing, with the scene-cut half struck: **9 of 12 black-fades in the first 5 min were
   corroborated by silence.** That 9-of-12 is what sets the "black + silence agreed" ceiling in V34's
   confidence ladder; the remaining 3 are the single-detector ceiling. Left struck rather than
   deleted, because two paragraphs describing four fusion inputs when only three exist is how the
   contradiction with V34 survived this long.
3. **Transcript topic-shift ADDS** the boundaries all three A/V signals missed (the soft cut) AND
   labels each segment with its product. Measured: a 105s span the A/V pass split weakly actually
   held 6 distinct ads (Fannie Mae, a car ad, US Navy, Dove, Subway, blue M&M's) — the transcript
   found the ones the fades did not.
4. **Duration ≈ a standard ad length** (30/60s ±2s) is the final confidence prior.

⚠ **The transcript-rescue trigger drops from "over-long (>120s)" to "longer than a typical spot".**
The V34 detector only ran the LLM boundary check on >120s segments, so the 57s A&W+Phantom merge was
never rescued — the A/V missed the cut AND the segment was not "over-long enough" to trigger the
transcript pass. A segment materially longer than one spot (~>45–60s) now gets the transcript check
too. The prototype proved the LLM cleanly splits it (`{A&W root beer @0s, Paramount @11s}`).

**The slivers are dropped, not reviewed.** A fragment under the detection floor
(`max(MinSegmentMs, filler.min_duration)`, §10 V34 step 2) whose transcript is only `[MUSIC]`/empty
is an inter-ad stinger, never a real advert — measured on the 3–8s fragments. No transcript ⇒ drop.

⚠ **Dropping discards TIME, and on a real reel that is not a rounding error.** `segmentsFromBoundaries`
advances past a dropped span rather than merging it into either neighbour, so the recording goes with
it: on the measured 82-segment reel, 39 sub-10s fragments is roughly three minutes of source. That is
the right trade — none of it could become a catalog row, since `filler.min_duration` is a hard reject
at the scan boundary — but it must be **reported, not silent**. `Propose` carries the dropped count
and duration onto the ladder note, so the operator reads *"cut into 43 adverts; 39 fragments under 10s
discarded"* rather than wondering where the other three minutes went.

**This produces a per-segment CONFIDENCE from signal agreement**, which is what makes
confidence-gated auto-confirm principled rather than a guess. It replaces "review all 41" with
"file the obvious spots, review the uncertain ones" — the concrete mechanism behind "automatic
curation with high confidence".

⚠ **Built in V54, and NOT as sketched here — see the ladder in §10 V34, which is authoritative.**
Two corrections this paragraph originally got wrong, both recorded there in full: **scene-cut is not
an input** (measured and rejected, bullet 2 above), and **duration is not a confidence input**
either. The sketch's "at a ~30s duration" reads as a corroborating prior, and a real reel refutes
it: 39 of 82 segments on the measured archive.org compilation were sub-10s bumpers, every one
correctly cut. Scoring a cut down for not landing on a standard slot length would flag half a good
reel. Duration survives only as the over-long cap.

⚠ It also gates **per segment**, not per reel: the confident cuts are filed and the doubtful ones
stay behind in a shrunken proposal.

⚠ **Confirm no longer deletes the parent — this reverses V34's delete-on-confirm.** V34 removed the
compilation's row and file on confirm ("its identity is a path that now means twenty clips");
V45 keeps it as a **composite entity** and gives each segment a `parent_hash` pointing back. This is
the load-bearing change, and it buys three things V34 threw away:

1. **Provenance** — "which break did this Coca-Cola ad air in?" is answerable, which channel theming
   ("a real 1996 Fox evening") depends on.
2. **Re-splitting** — detection improves (a better rescue model, a new boundary heuristic); with the
   parent gone that is impossible, and with it kept it is a re-run.
3. **Segments inherit the parent's broadcast context** (below) for free — they came from the same
   tape, so the same network/market/date applies to all of them.

The parent stays on disk and in the store, marked composite (not airable); its segments are the
airable clips. `parent_hash` is nullable — a hand-dropped single advert has none.

⚠ **A terminal parent cannot remain held after its proposal is gone.** Full confirmation now files
the composite parent and releases its hold as one operation. On upgrade, the pipeline performs a
data-selected compatibility pass for parents written by the older confirm path: it releases a hold
only when the pipeline row is already `filed`, the clip is still a composite, and no split proposal
survives for that hash. A review row or a proposal with leftovers remains held. The repaired parent
does not become airable — `is_composite` is the independent playout gate — so this repair removes
stale operator friction without weakening split review.

**A completed re-split replaces the prior generation; it does not stack another overlapping set
beside it.** Partial auto-confirm remembers every child it produced on the proposal. When the last
cut is confirmed, the store atomically restores every child in that remembered generation and
tombstones prior children of the same `parent_hash` that were not reproduced. The bytes are never
deleted, so a retired cut remains recoverable through the normal restore path. A content-identical
cut is kept by hash, and a clip explicitly pinned by any channel is also kept: improving detection
must not silently break an operator override. Until the proposal is fully resolved, the prior
generation remains airable; a half-finished re-split may not replace a complete catalog with half
a reel.

⚠ **"Stays on disk" is now "stays until the sweep retires it" (V54), and one of the three things
above is genuinely given up.** Partial confirm leaves a residue by design — every reel files its
confident cuts and holds the doubtful ones — so proposals accumulate and each pins a 1–2 GB
recording. The `filler-split-sweep` job retires a reel whose leftovers nobody reviewed inside
`filler.split.review_window` (default 30 days, `0s` = never): it drops the proposal and **deletes
the recording**.

Of the three things keeping the parent bought: **provenance survives** and **inherited broadcast
context survives**, because the catalog ROW survives — the sweep sets `clips.reaped_at` and only
the bytes go. **Re-splitting does not.** That is the cost, and it is the reason the sweep is bounded
by three rules rather than one: a recording is eligible only after the window, only if it has
ALREADY produced clips (a reel Loomarr could not use is the operator's only copy of that content),
and never if `review_window` is `0s`.

⚠ **The row must survive, and this is the cascade that makes it non-optional.** Every segment
carries `parent_hash` pointing at the composite. Delete the file without the tombstone and the next
`filler-sync` finds the clip gone from its source and prunes the row — dangling `parent_hash` on all
of its children at once. `DeleteClipsNotIn` therefore skips a reaped row, and the sweep writes the
tombstone BEFORE the unlink so no sync can land in between.

⚠ **The sweep must also take the reel off the belt**, or it is worse than no sweep: the composite is
still `is_composite`, so the split rung re-detects it next pass — propose → partly confirm →
leftovers → sweep → re-propose, burning a boundary scan every cycle forever. Setting the pipeline
row to `filed` is the existing one-word way to say finished, since `ListPipelineWork` claims only
`running`.

⚠ **This is the ONLY thing in Loomarr that deletes an operator's media**, reversing the blanket rule
`fillerbulk.go` states. Everything else still tombstones: removing a clip from the catalog keeps the
file, and so does disabling or deleting a source. `docs/help/filler.md` says so in the operator's
own words, because that file ships inside the binary and is where they will look.

#### 3. Structured broadcast context — a parser over text we already have

The archive.org title/filename encodes **network, station, market, and exact air date** in plain
text that V44 discards after pulling the year for era grounding: "KCPQ/Fox commercials, **5/28/1996**"
and "**CLE**-B23… CBS-**WJW-8**…**1993-01-10**". A structured parser (Go `regexp` + a
station-callsign→market table, e.g. `KCPQ`→Seattle/Fox, `WJW`→Cleveland/CBS) extracts:

- `network` (Fox, CBS, NBC…) — from the callsign table or the title
- `station` (KCPQ, WJW-8) — the callsign
- `market` (Seattle, Cleveland) — from the callsign table
- `air_date` (full date, not just the year `era` grounds) — parsed from the date in the text

⚠ **Grounded like every other tag: parsed only when it literally appears in the text.** A callsign
not in the table, or a title with no date, yields empty — never a guess. This is the cheapest tier
on the whole ladder (no model, no network) and the one that makes period/regional authenticity
possible. The parser runs on the **composite** and its output propagates to every segment.

#### 4. Semantic metadata, without a vector column

The end goal — "automatically curate clips with high confidence they match a channel" — needs a
signal the enum tags cannot give: **thematic/tonal matching**. The shape of this was decided by a
**throwaway prototype on real KCPQ segments** (scratchpad, 2026-08-07), not by assumption — and the
prototype changed the plan, so its findings are recorded here:

- ⚠ **LLM-extracted closed-vocabulary tags are the thematic engine — NOT embeddings.** The tagger
  extracts `mood`/`tone` (nostalgic, energetic, calm), a `topic` hierarchy above the flat 12-value
  `category`, `channelFit` theme labels, and `sensitivity` flags (political, alcohol) from the
  per-segment transcript + vision text. All grounded, all from a closed vocabulary the operator's
  channel rules can match and AUDIT. This is the primary "does this clip fit channel X" signal.
⚠ **Vision model + frame extraction, decided by prototype (scratchpad, 2026-08-07).** Two findings
from testing llava:7b vs qwen2.5vl:7b on real KCPQ frames with known on-screen text ("Deferred
Payment Offer Ends 6/2", six red cars):

- **`qwen2.5vl:7b` is the default vision model, not llava.** On the Dodge frame llava CONFIDENTLY
  FABRICATED a date ("ENDS 2/9/03" — not on screen); qwen read it correctly ("...Payment Ends 6/2")
  and, when it could not read cleanly, returned honest FRAGMENTS ("Del Pay / Effer / End") rather
  than inventing. Honest-partial is far safer than confident-wrong for the grounding gate: the gate
  catches a brand invented out of nowhere, but it CANNOT catch a model that misreads text and then
  grounds a tag against its own misreading (llava's "2/9/03" would self-consistently ground a wrong
  air date). qwen's failure mode drops cleanly; llava's launders fiction into a grounded fact.
- **Vision keyframes must be FULL/near-full resolution, NOT the 320px `FFmpegArtwork` thumbnail
  path.** The SAME model on the SAME frame went from unreadable ("Del Pay / Effer / End") at 640px to
  correct ("Payment Ends 6/2") at full resolution. Resolution was a bigger lever than the model for
  OCR. The V44 vision tier reused the 320px still (fine for thumbnails); V45 vision extracts at full
  res for text reading.
- **Sample MULTIPLE frames per segment, timed toward the end-card.** A single mid-ad keyframe lands on
  narrative (the KFC frame at 4:05 was two people talking — no logo), because an ad shows its brand
  for only ~3–5s (the closing logo card). `MediaTools.Keyframes(n)` already takes a count; V45 uses
  several frames biased toward the segment's end, where branding lives, rather than one.

- ⚠ **Embeddings were measured and rejected.** The prototype embedded 10 real ad transcripts with
  `nomic-embed-text` and failed on the abstract-theme case structured tags must solve. Duplicate
  detection instead uses persisted, versioned whole-catalog dHash evidence: deterministic across
  providers, reusable between split jobs, and supported identically by SQLite and Postgres. A vector
  extension is also incompatible with the current portability boundary: the SQLite build is pure Go
  (`CGO_ENABLED=0`) and the Postgres conformance image does not carry pgvector. Reconsidering fuzzy
  search would require a new measured use case and a §14 dependency decision; it is not latent work.

⚠ **Structured filters stay structured and deterministic — the thematic layer only RANKS.** The pod
assembler's core queries — era, audience, category, no-repeat-brand, network/market — are exact
`WHERE` clauses and must stay deterministic (pod assembly is seeded so the guide can promise what
airs, §10/§19). The grounded LLM theme tags are a *ranking* signal over an
already-eligible structured candidate set — they order the pool toward the channel's theme, they
never decide *which* clips are eligible.

#### The clip taxonomy (V45a — replaces the flat category enum)

⚠ **The flat 12-value `category` string cannot express curation.** A rule like "one **food** ad per
break" cannot ask "is `cereal` a kind of food?", and a free-text set drifts (`drinks` vs
`beverages`, the model emitting `soda`). V45a replaces it with a **multi-tag model over an
operator-editable taxonomy graph** (full design: `design/TAXONOMY-DESIGN-2026-08-07.md`). This is
its own sub-phase, landing BEFORE the composites UI so the UI renders tags from the start.

**A clip carries a SET of tags, each a `taxon`** — `{slug, label, parent, synonyms, kind}` — where
`parent` forms a **forest by axis**, not one tree, because a clip is tagged on independent axes at
once: **product** (`beer` → `alcohol` → `drinks`), **format** (`commercial`, `psa`, `ident`),
**seasonal** (`christmas`, reusing the §10 holiday keyword IDs), **audience-cue** (hints, kept
separate from the `audience` enum). A Christmas beer ad is `{beer, alcohol, christmas}`.

Four properties make it robust, curation-ready, and LLM-friendly:

- **The parent graph makes rollups QUERYABLE.** "One food ad per break" is `count(tags ∋
  descendants(food)) ≤ 1` — impossible on a flat set.
- **The model emits only LEAF tags; rollups are DERIVED from the graph.** The LLM's one job is "this
  is a beer ad"; "therefore a drink/alcohol" is the graph's job — fewer ways for the model to be
  wrong.
- **Grounding = resolve-or-drop with synonym rescue.** Each returned tag resolves: exact slug → keep;
  a `synonym`/retired alias → map to canonical (`brew` → `beer`); anything else → DROPPED, never a
  new taxon. Same anti-fabrication discipline as era/brand (§8); only an OPERATOR adds a taxon. The
  vocabulary is SERVED to the model (BE the single source, mirroring `schedule.BuildVocabulary()`),
  so it never guesses a slug blind.
- **Operator-editable, DB-backed.** New tables `taxa` (the graph) + `clip_tags` (many-to-many), seeded
  with a default forest, forward-only migration. An operator adds `energy-drink` under `drinks`
  without a code change.

Both text and vision classifiers receive this live vocabulary and return a tag set, never a
hard-coded legacy category. A vision pass commits its grounded frame facts and additive asserted
tags in one transaction; compilation-segment vision carries those assertions through confirmation
onto the content-addressed child clips. The `category` column is only the most-specific product-axis
compatibility shadow and is never a classifier-owned source of truth.

⚠ **An asserted tag is not the same thing as a graph leaf.** `clip_tags.leaf` is the historical
column name for “asserted by the classifier/operator”; it is not a promise that the taxon currently
has no children. A broad but honest assertion such as `food` is valid when the evidence cannot
support `cereal`, and it stays asserted if an operator later adds children beneath it. API reads
therefore expose asserted tags separately from the full asserted-plus-rollup set. Editors write only
the asserted set; feeding rollups back through an editor would promote derived ancestors into facts
that survive later graph changes.

⚠ **The graph is valid or the edit does not happen.** A taxon slug is stable, normalised, and unique
across canonical slugs, synonyms, and retired aliases. Its parent must exist on the same axis; cycles,
self-parenting, dangling parents, cross-axis edges, and resolver collisions are rejected. A graph edit,
its closure rebuild, the set-based clip-rollup rebuild, and the derived `category` shadow refresh
commit in one store transaction. Deleting a
taxon with directly asserted clips is refused until those clips are retagged; deleting an unused
intermediate node reparents its children to the grandparent. This keeps an operator mistake from
silently erasing classification or leaving the catalog between graph generations.

⚠ **The taxonomy surface is an accounting view first, not a vocabulary console.** Daily Filler
navigation leads with overall and per-axis classification coverage, plain-language axis names and
examples, and direct catalog links for gaps. A clip with a seasonal cue but no product/topic
assertion is therefore visible in both dimensions rather than flattened into one “classified” bit.
Axis absence is neutral—seasonal and audience cues are intentionally sparse—not an automatic cleanup
task. The raw forest, stable slugs, parent relationships, synonyms, retired aliases, and mutation
controls live under Filler → Advanced → Classification vocabulary in a collapsed **Manage
vocabulary** disclosure. Members may inspect that hierarchy but only admins may edit it. The clip
editor writes only observed/asserted facts and shows inherited matches as read-only context, so a
derived ancestor can never be accidentally promoted into a permanent assertion.

⚠ **An advanced graph edit is previewed through the same prospective-graph contract it commits.**
Before create, update, or delete, the server validates the whole proposed forest and reports directly
asserted stored clips, distinct descendant assertions, affected playable clips, descendants whose
lineage changes, saved channel selections referencing the node/subtree, and resolver terms added or
removed. The browser renders that projection; it does not reconstruct impact. A commit re-runs
validation under the store's graph lock, applies the atomic closure/rollup/category rebuild below,
then sends the affected clips' before-and-after snapshots through the same targeted channel
reconciliation path as a clip-classification edit. Reconciliation remains best-effort after the
semantic commit, with the ordinary channel sweep as the crash-safe retry.

Every taxon count and axis population links back to the matching catalog. Synonyms and retired aliases
remain progressive detail because they help the classifier but are not another set of tags the
operator must manage day to day.
Brand, clip kind, era, and audience remain adjacent
structured facts rather than being folded into the taxonomy: the forest answers thematic/type
membership, while those fields keep their existing grounded or structural semantics. The clip
editor exposes brand correction and explicit clearing because an additive classifier will not
overwrite an existing grounded value; an operator is the recovery authority for a bad guess.
Deletion safety uses a separate all-stored assignment count, including held, removed, and composite
records, so the UI never offers a deletion the backend must refuse to preserve hidden knowledge.

⚠ **Rollups are stored DENORMALISED** (a tag of `beer` writes `beer`+`alcohol`+`drinks` rows, each
flagged leaf-vs-rollup), so `WHERE taxon = 'food'` is one index hit — pod assembly runs it per break
per reconcile, so the read must be cheap. The cost accepted: the owning write recomputes rollup rows
when a clip is re-tagged or the graph changes; the graph is source of truth, the denormalised rows a
derived cache (the same "synced cache" shape `clips` already is).

⚠ **A graph-edit rebuild is SET-BASED, not a per-clip loop, because the catalog is not bounded to a
few thousand.** The `filler.fetch.max_catalog_clips` ceiling (§14, default 2000) only throttles
*auto-fetch* — an operator who raises it or bulk-imports a large archive can reach tens of thousands
of clips, at which point a Go loop issuing one write transaction per clip is an N+1 that holds a job
worker for the whole pass. So the rollup rebuild is **one bulk `INSERT … SELECT`** per graph change,
and it must be dialect-neutral (one statement on SQLite *and* Postgres — the store does not fork it).
The obstacle to plain-SQL is that `taxa` is an **adjacency list** (`parent` points one level up), so
computing a clip's ancestors would need a recursive walk — the exact dual-dialect divergence §7.2's
search decision refused. **A `taxa_closure` table dissolves it:** one row per (ancestor, descendant)
pair — the graph's transitive closure, ~55 taxa so a few hundred rows — rebuilt from the Go `Forest`
whenever the *graph* edits (rare; `Forest.Ancestors` stays the single owner of the walk). With the
closure materialised, the clip rebuild is a flat join —
`INSERT INTO clip_tags SELECT clip_hash, ancestor, (ancestor = leaf) FROM (asserted leaves) JOIN
taxa_closure ON descendant = leaf` — identical SQL on both dialects, O(closure ⋈ leaves) in the
engine, no per-clip round trip. The graph-walk logic therefore lives in Go *once* (to fill the
closure) and never in two SQL dialects; the hot rebuild path touches no recursion.

⚠ **A graph edit is synchronous and atomic, not a background repair promise.** Its semantic store
operation updates the node, closure, denormalised rollups, and the `category` compatibility shadow in
one transaction. The heavy catalog operations are set-based SQL, so the API never acknowledges a
vocabulary generation while matching still sees the previous one. Clip re-tagging remains a
per-clip transaction because it changes one assertion set, not the graph. It expands against the
store's current closure and refreshes the category shadow before commit; callers cannot supply a
stale forest snapshot. Startup performs the same locked, set-based projection rebuild as an upgrade
backstop, healing any drift from pre-atomic releases or restored/manual data without an operator job.

⚠ **The taxonomy owns thematic matching.** The embedding prototype failed at the topic/season/theme
case this graph makes explicit, while persisted dHash owns the separate near-duplicate problem. The
graph-edit rollup maintenance remains a set-based SQL rebuild over `taxa_closure`; there is no
re-embed job or vector column.
Full interaction: `design/TAXONOMY-DESIGN-2026-08-07.md`.

#### The curation confidence this produces

With brand (V44), broadcast context (#3), and grounded taxonomy tags (#4), the assembler gains new match
and variety axes: **no repeat advertiser in a break** (Brand, today carried but unused), **network/
market/era-window** filters (a "1996 Seattle Fox" channel), **loudness-aware ordering** (the LUFS
V42 measures, used to pace a break instead of only to normalise), and a **thematic match score** (the
grounded tag set). "High confidence this segment fits channel X" comes from auditable grounded
signals rather than an opaque vector distance. The operator
rules layer (`internal/schedule`'s `SchedulingRule` WHEN/WHAT/HOW, borrowed rather than reinvented)
expresses the exclusions and quotas.

⚠ **No vector dependencies.** `sqlite-vec`, pgvector and `nomic-embed-text` are not part of the
runtime or §14. Persisted dHash is ordinary store data and keeps the dual-dialect conformance suite
over one behavior.

#### Frontend implications (V45 — governed by §12/§13 and `docs/frontend-design.md`)

The keep-parent model **inverts a user-facing flow**, so V45's FE work is not all additive — two
things it makes actively *wrong* must be corrected in the SAME change as the backend, not deferred:

1. ⚠ **Resolved:** split review says confirming files segments beneath the preserved compilation,
   and completion files the parent's pipeline row so it leaves Incoming without deleting lineage.
2. ⚠ **Composite is a separate boolean, not a seventh clip kind.** `isComposite` marks a non-airable
   container while the underlying recording retains its measured content kind. Catalog navigation
   opts into composites with `includeComposites+topLevel` and loads children with `parentHash`; the
   six-value airable-kind filter remains closed and unchanged.

**The FE already anticipated the composite problem, which is why these additions complete an existing
design rather than fight it.** `pool-health` warns in-tree that "a catalog of five hundred
fifteen-minute compilations reads as healthy by clip count and can fill nothing" — the non-airable
composite kind is the fix for exactly that. The reusable pieces exist: a `ConfidenceMeter` (used in
Incoming) for a per-clip/per-pod curation-confidence score; the `ClipCard` badge/chip row for a
"composite / not airable" badge and for broadcast-context (network · market · air date); the
`incoming-panel` reel rows, which already model "a compilation → its segments", as the home for the
kept-parent lineage view; and `GuideDetailCard`'s per-clip pod lines (already era/quality) for
broadcast context and the match-level ("why this clip") explanation.

**Additive surfaces**, each with an identified home: a compilation container in the catalog;
a lineage view (segment ↔ parent break) in the review editor header and the clip card; broadcast
context (network/market/air-date) on the card, guide hover, and as new catalog filters; a
curation-confidence indicator (reusing `ConfidenceMeter`); and mood/topic/sensitivity chips on the
card and filter bar. All require the new fields on `ClipDTO`/`SplitSegment` first (so the OpenAPI
regen is the gate for each), and all fit the stable three-tab filler IA (Catalog · Incoming ·
Sources) without new top-level navigation.

**Cleanup the audit surfaced** (do in a V45 PR, not silently): `ClipDTO.source` and
`tunarrProgramId` exist on the FE type but are rendered nowhere — either surface them or drop them
from the display path, rather than carrying dead fields that read as capability.

#### Settings + AI-page implications (V45 — governed by `docs/config-design.md`)

⚠ **V61/V62 supersede the manual three-role picker with automatic routes and one speech
connection.** Text and vision may share one hosted OpenAI-compatible provider (including
OpenRouter). Speech recognition may reuse it or declare a dedicated compatible endpoint. Local
speech remains the bundled whisper path; there is no embedding role.

⚠ **Text and inherited vision resolve one active provider selection, including its branded credential.** A
hosted selection stores the wire kind as `llm.provider=openai`, its brand as
`llm.hosted_provider` (`openrouter`, `custom`, …), and its secret as
`llm.api_key.<brand>`. Text tagging and split classification, inherited vision, hosted timed
transcription with a blank speech URL, and the setup connection check resolve that selection rather
than reading the base `llm.api_key` row directly. Speech may instead declare its own
`ASR_URL`/`ASR_MODEL`/`ASR_API_KEY`; that explicit endpoint never inherits the main provider's
credential. The wire remains OpenAI-compatible. Custom endpoints may have an empty key, so endpoint
presence — not credential presence — is the runtime availability boundary.

- **The AI page exposes the lineup model, an advanced vision override, and one Speech recognition
  section.** The ordinary speech choice is simply built-in or connected. A connected service's
  URL, model, and key stay behind Advanced.
- **The organising principle: provider connections live on the AI page; feature toggles and behavior
  live on the Filler page.** So `filler.vision.model` and the speech connection are exposed on AI;
  `filler.vision.enabled` / `filler.transcribe.enabled` and
  the split/curation behavior knobs stay on Filler.

**Setting to change immediately, independent of the phase:** `filler.vision.model` default becomes
**`qwen2.5vl:7b`** — the prototype proved llava:7b's confident-fabrication (a misread date grounded as
fact) is unsafe for the grounding gate, while qwen reads OCR accurately and fails honestly.

The split confidence threshold is `filler.autosplit.min_confidence`; the duration ceiling is
`filler.autosplit.max_duration`. Split-boundary confidence and classification confidence answer
different questions and share no publication control. There are no `filler.embed.*` settings.

### Tagging confidence (V38; confidence publication retired)

The tagger records a **confidence score** (0–100) alongside the tags. It is diagnostic evidence for
classification review and prioritisation. It never decides whether a clip is playable: terminal
admission replays exact four-axis safety, playback, and Airworthiness evidence.

⚠ **V38 originally used this score as an auto-file threshold; that publication use is retired.**
A grounded taxonomy result is not a safety verdict or proof of complete playback.
Keeping the score is useful; keeping a shortcut from score to airability would recreate a second,
weaker admission authority.

⚠ **The score is grounding-gated, and that is the whole safety property.** It is NOT the model's
own self-assessment, because this tagger has a measured history of confident fabrication — the
paragraph above records it inventing an era on 2 of 10 real clips, inferred from tone. A
self-reported number would be the same failure one level up: the model that fabricated the era
also grades how sure it is about the era.

So the score is built in two layers, and only the first can *raise* it:

1. **Grounding facts CAP it.** Everything `validateTags` can verify sets a ceiling — was the era
   found **literally** in the text or merely inferred; did audience and category match the known
   enums; was there any source text to check at all. ⚠ **An ungrounded era can never reach the
   fully-grounded score**, no matter what the model claims. That is a hard ceiling, not a
   subtraction, and it is the property to sabotage-test.
2. **The model refines within the cap.** The model reports its own confidence and it may only
   *lower* the grounded ceiling, never lift it. A model that is unsure about a clip whose tags all
   verify is still worth surfacing; a model that is certain about an era it invented is not.

**The consequence, stated plainly: confidence never publishes content.** It prioritises review
and explains classification strength; a perfect score is still only metadata evidence. The former
confidence threshold, `auto_filed` application state, legacy audit list, and direct withdrawal
route are retired rather than maintained as a parallel lifecycle. Historical database columns may
remain until a future schema rebuild, but no domain or API contract assigns them meaning.

#### Evidence-based classification and terminal readiness (V61; household beta)

V38's grounding cap remains useful for descriptive classification, but it is not household-use
authority. A literal token can support a role, era, or product fact without deciding whether the
person who selected a Source must approve the same Clip again. Confidence therefore remains
versioned diagnostic metadata and never controls readiness.

The production terminal decision belongs to one Go-owned **terminal-ready module** after required
runtime processing. Its small interface receives the exact Clip, durable Enrollment authority, and
completed conveyor state. It returns one of three effective outcomes: Ready, not usable, or a real
Needs-help task whose action changes durable state. Retries, provider failures, exhausted budgets,
and unavailable optional enrichment are operational states in Diagnostics; they cannot become
semantic chores by changing labels.

Evidence is claim-specific and carries provenance. Decoder measurements own media usability;
source-owned dates and recording sidecars outrank a year merely spoken in a clip; readable end
cards, packaging, and spoken advertiser claims support brand/product. Provider-declared licence data
remains passive provenance outside the evaluator. Filename, uploader metadata, transcript, OCR, frames, audio, and video are all
untrusted data with no instruction authority. **Contradiction is a first-class evidence result**:
the evaluator either invokes one bounded additional rung or abstains with a specific question. More
conflicting tokens never increase confidence.

The evaluator accepts one closed, versioned evidence document rather than a prompt-shaped bag of
strings. Its claim roles are exactly media usability, recording date/era, brand, product, content
role, and sensitive-policy flags. Each fact names its extractor kind, source
identity, bounded location, and inference evaluation when one produced it. Authority is assigned by
the Go policy from the claim and provenance kind; evidence cannot declare its own rank. Decoder
measurements alone can prove unusable media. For conflict-prone semantic claims, independent corroboration means distinct extractor
kinds over distinct derivatives or source records — repeated tokens from one transcript, OCR frame,
or model generation still count once. A filename year and a different spoken historical year are a
conflict, not two votes; a source-owned recording date may resolve them because the policy, not
literal presence, grants that source authority.

Content-role and product corroboration also require at least one in-clip signal (transcript, OCR,
frame, audio, or video). A filename plus an uploader description are two fields controlled by the
same uploader, not proof that the bytes contain the claimed advert; metadata-only agreement may
support a suggestion but cannot become a high-confidence descriptive classification.

A high-confidence commercial classification requires a corroborated product from the closed
taxonomy. Brand is retained as useful evidence and may expose a conflict, but the advertiser-name
field is open text and cannot substitute for that closed product classification; two copies of
instruction-looking OCR/transcript text therefore cannot become a commercial identity merely by
agreeing with each other. A Clip may still become Ready with an enrollment-grounded break-body
Placement while its exact role remains unclassified.

`filleradmission.Evaluator.Evaluate` remains deterministic development and certification tooling; it
has no provider, decoder, store, clock, or network dependency and is not a runtime publication gate.
Its measurements carry a stable sorted set of reason codes, the exact evidence references that
support them, all material conflicts, at most one review question, and the inference
attribution/usage supplied with the evidence. Every semantic inference attribution is referenced by
at least one fact, and every fact's inference reference must resolve; unrelated or dangling model
calls cannot be smuggled into a decision's audit. The sole exception is an explicit semantic
abstention: one bounded reason, no evidence, and no operational failure. It remains in the step
ledger so named escalation and cost accounting are truthful, but it cannot support a decision.
Invalid schema/taxonomy, failed extraction, unavailable
provider, retryable error, or exhausted budget returns a separate operational hold and no semantic
verdict. Model confidence is retained for diagnostics but is never read by admission policy.
Untrusted evidence values are compared only as data; instruction-looking metadata, OCR, or transcript
text cannot select a reason, change precedence, or authorize a verdict.

Runtime has one effective path and no `shadow` / `applied` mode. Audit-only human answers and
certification-only release bindings are not runtime capabilities. The terminal-ready module owns
the complete transition before any publication write: it validates the exact Clip identity,
requires durable Enrollment authority, derives Placement without inventing a Role, and checks that
the required pipeline work completed. One store transaction then records the Ready event, stores
Placement, clears `clips.held`, and settles the matching conveyor row as Ready. A missing or changed
Clip, stale pipeline row, absent authority, composite, or objective failure rolls the whole write
back. Repeating the same completed transition is an idempotent success.

The storage interface exposes no generic `held=false` writer. Ordinary pipeline and operator code
may hold or tombstone a Clip, while only the terminal-ready transaction may publish a non-composite.
Composite containers retain a separate constrained operation and remain excluded from Pods
independently of their hold. This concentrates atomicity and stale-state handling behind one deep
module instead of asking each pipeline rung or source adapter to reproduce the rules.

The ingest ladder performs enrichment and diagnostic scoring before terminal readiness. A skipped
optional classifier remains a visible stage fact but cannot stop an otherwise valid enrolled Clip.
Positive measured media failures still reject automatically. Provider-declared licence metadata is
persisted unchanged and never enters the readiness decision. Development/certification evaluations
continue to record exact evidence, model/provider attribution, cost, and disagreement for improving
classification policy; those records grant no runtime action and never appear as Needs-help work.

Certification artifact schema v5 requires the per-inference-step ledger and the corpus-diversity
identity used by the statistical contract. Earlier schemas are rejected: no completed bakeoff
artifact depends on them, and preserving speculative compatibility would create an untested path
that can hide multiple calls behind one terminal attribution or bypass the diversity gate. The v5
prediction wire therefore has no scalar inference fields: every attempted call is a `steps` entry;
only deterministic outcomes and holds reached before a provider attempt may have none.

The inference-spending bakeoff reads a **raw evidence packet**, never the manifest's reviewed
`Evidence` or terminal labels. The packet is a closed, content-addressed input containing only
deterministic decoder/source-policy facts and bounded untrusted source text, transcript, OCR, frame,
audio, or video derivative references. Every referenced external derivative carries its hash and
measured byte/pixel/duration bounds. Packet identity must match the case's `evidenceSha256`; the
runner re-opens every derivative beneath one declared corpus root, refuses symlink escapes, and
verifies its exact bytes and hash. It refuses missing, extra, changed, or label-bearing fields before
calling a provider. The internal case ID remains a local ledger join and is omitted from provider
prompt content. This separation keeps the scorer's answer key and identity-correlated shortcuts out
of prompts and makes the exact provider input replayable.

One bakeoff run accepts a locked manifest, an exact packet set, a versioned admission policy, an
ordered role/rung route, and positive request/spend/concurrency ceilings; it emits an immutable
prediction ledger. Command boundaries that read a complete immutable JSON evidence or attestation
artifact use exact, case-sensitive member names and reject unknown or duplicate members, invalid
UTF-8, and trailing values; `null` and empty collections retain their distinct wire meanings. Writers
remain on the established canonical encoding so strengthening a reader cannot change an artifact's
bytes or SHA-256. The runner is serial by default. Before each call it reserves that rung's
predeclared maximum nanodollar charge and request count, then reconciles the provider-reported exact
charge without binary floating point. A call is refused when its reservation cannot fit. If a failed
call omits or corrupts settlement, its exact charge remains missing while the distinct recorded
reservation stays consumed for that run. Every
attempt is retained as a separate inference step, including route, modalities, derivative bounds,
tokens, charge, latency, attempts, generation id, and operational failure. The terminal prediction
aggregates those steps but never collapses a cascade into one falsely attributed rung.
Certification routes authorize exactly one provider attempt per invocation; an adapter may not hide
internal retries inside an aggregate attribution. A retry is a new runner invocation and ledger step.

An OpenRouter certification run is also bound to one immutable metadata snapshot fetched no more
than 24 hours before its declared run time. The snapshot makes one bounded authenticated model-catalog
request, one bounded ZDR-list request, and one bounded endpoint request per concrete candidate. It
freezes both the requested model ID and catalog canonical revision, model modalities, endpoint
selector slug, distinct returned provider name, strict-output parameters, status, exact price text,
and ZDR membership. Its SHA-256 is both the run's capability and price identity. Every route must bind
its requested and resolved model identities and resolve exactly within that snapshot before media is opened or spend is reserved; a
human-readable snapshot label, catalog-level capability claim, or provider-family name is not proof.

Routing is reason-driven rather than confidence-driven: deterministic and text evidence run first;
frames may run only for a predeclared missing/conflicting claim; direct video may run only for a
named temporal ambiguity; premium escalation may run only for a predeclared unresolved reason whose
measured value justified its marginal ceiling. After each rung the same pure
`filleradmission.Evaluator` evaluates the accumulated document. A terminal semantic result stops the
cascade; a provider, route, schema, extraction, or budget failure becomes an operational hold in the
ledger and can never be converted into `admit`, `reject`, or human review.
Route classes enforce this order: text is first, then frames, direct video, and premium; frame routes
accept only named visual claim gaps/conflicts; direct video accepts only `temporal_ambiguity`; and a
premium route must name and hash the frozen measurement artifact that justified its marginal ceiling.
Each request contains only signals whose modality the route declares; attribution must match that exact
set, so a cheaper route cannot receive or claim a more expensive modality.

Model roles are certified independently: lineup, filler text, filler frames, filler video, and
transcription may have different accuracy/cost frontiers. A normal install follows the last
certified role policy automatically; overrides are Advanced controls. Certification pins concrete
model and provider identities, disables fallback, requires the requested structured-output
parameters, snapshots capabilities/prices/privacy, and uses identical evidence across candidates.
Production resilience may use explicitly allowed fallback, but records the resolved endpoint and is
measured separately. A moving `latest` alias cannot authorize unattended filler decisions.

The evidence cascade is cost- and resource-bounded: deterministic checks, text/transcript/OCR,
near-full-resolution scene/end-biased frames, direct bounded video only for named temporal
ambiguity, premium escalation only where measured value exceeds marginal cost, then review. The
320 px UI preview is never semantic/OCR evidence. Hosted derivatives strip container metadata and
have hard frame, pixel, duration, byte, token, retry, concurrency, per-clip, daily, and evaluation
ceilings. Shared-appliance inference is serial by default. Privacy/provider constraints never relax
silently; an ineligible route leaves the clip held.

The default frame derivative is exactly four ordered JPEGs from bounded representative windows at
5%, one-third, roughly two-thirds, and 90% of the measured clip or segment span. Each window decodes
at most three seconds and the final window is the closing-card bias. Frames preserve their native
size through 1920 px wide (never upscale, preserve aspect ratio) and are encoded sequentially. This
is an extraction ceiling, not permission to send all four frames: the evidence router may send a
strict subset when a cheaper claim is already answered. A missing/invalid duration refuses visual
extraction rather than falling back to an unbounded decode.

The default hosted-video derivative is one metadata- and chapter-stripped MP4 containing H.264 video
and optional AAC audio. It spans at most 60 seconds of the measured clip or segment, fits within
1280×720 without upscaling or changing aspect ratio, and is at most 12 MiB before base64 expansion.
Extraction and upload are sequential, encoding uses one ffmpeg thread, and a size-limited reader
aborts encoding before buffering more than the ceiling. The request caps model output at 512 tokens
and refuses a response body over 256 KiB. The direct-video provider is a separate capability from
text and frame vision. The OpenRouter certification adapter accepts base64 transport only, a
concrete namespaced model whose
live metadata advertises video input, and one explicitly pinned upstream provider; it disables route
fallback, requires supported request parameters, and denies provider data collection while requiring
zero-data-retention routing. URL transport, thin or stale capability metadata, invalid measurements,
and any exceeded bound fail closed before a hosted request. This adapter supplies evidence only and
does not itself decide or change production admission.

Certification uses a versioned, source/similarity-separated development corpus and locked holdout.
The maintained contract requires at least 300 development cases and 1,126 independently clustered
holdout cases: 446 eligible positives, 446 deterministic-invalid controls, 147 semantic-invalid
controls, and 87 genuinely ambiguous, answerable-review cases. The eligible positives include at
least 82 commercials, 82 promos, 59 bumpers, 59 station IDs, 82 trailers, and 82 PSAs. No creator may
supply more than 10% of one eligible role, and no source may supply more than 25% of
eligible holdout cases. A holdout similarity cluster, campaign, and source master each contribute
exactly one case. Preparation derives source-family identity from the source authority and item ID;
alternate segments, encodes, and derivatives of one master therefore cannot pretend to be independent
observations. Campaigns and source families cannot cross the development/holdout split, and
near-duplicates cannot cross it either. It reports action-specific precision and coverage,
worst-slice results, review answerability, conflicts, schema/grounding/security failures, calibration,
latency, and total operating cost with confidence bounds. Unattended behavior expands only after the
predeclared gates in the certification artifact pass: zero observed prohibited admissions,
instruction escapes, or ungrounded taxonomy values; at least 99% observed auto-admit precision; at
least 99% deterministic-reject and 97% semantic-reject precision; then at least 90% valid-filler and
95% invalid-input automation with at most 10% review. Each safety-critical slice has its own gate.
Admission, deterministic-reject, semantic-reject, valid/invalid automation, and review-answerability
gates require both their point estimate and one-sided 95% Wilson lower bound. The precision and
answerability denominators above let one observed error
remain within each 99%, 97%, and 95% lower-bound target; two errors do not. The point estimates alone
do not certify a small corpus.

A certification manifest is itself content-addressed and records its lock time. Every case locks the
source media and captured evidence packet by SHA-256 plus item-level provenance: source authority,
stable item and media URLs, retrieved metadata hash/time, exact rights statement, rights decision
and reviewer, source representation/size, and bounded segment. Redistribution permission is
explicit; media that cannot be redistributed stays outside Git. A collection name or missing rights
field never implies permission. Each semantic reviewer receives an independently shuffled packet
whose random opaque aliases expose content/evidence hashes and bounded segment coordinates but no
internal case ID, split, cluster, creator, campaign, source filename, or labels. An owner-only alias
map binds that batch to the exact draft digest and is required for mechanical unblinding; reviewer
submissions use aliases, never case IDs. Two distinct reviewers in distinct blind-review batches
submit immutable label hashes covering disposition, reject class, content role, taxonomy, policy
flags, evidence spans, and any review question. The original blind submissions remain visible. Matching
submissions become the final labels directly; a disagreement requires a reasoned final adjudication
by a third identity. Rights adjudication and semantic labeling are separate records.

Eligible and semantic-invalid labels require one role from the closed review vocabulary. A
deterministic-invalid or ambiguous label may leave the role empty only when the supplied evidence
cannot establish it; there is no legacy `unknown` role and reviewers never invent a filler type merely
to fill the field. One bound signal may support multiple distinct evidence claims, such as a
transcript supporting role, brand, and product; only an exact repeated evidence record is invalid.

A semantic reviewer may be a person or a model-backed review run, but a model-backed identity is
valid only when a separate immutable attestation binds the exact blind-package manifest, prompt,
provider, concrete model identity, its local digest or hosted capability-snapshot identity,
transcript-set identity when used, completion time, per-case
latency and token accounting, and the completed submission hash. The runner receives only one
reviewer package, re-hashes every supplied signal, joins a transcript by the package audio hash rather
than exposing a case ID, runs serially with one attempt and a per-case timeout, and atomically
publishes the attestation and complete JSONL submission only after all 300 cases validate. Review A
cannot see review B, either submission, an alias map, source identity, or candidate output. The exact
model family used for review or adjudication is excluded from every scored candidate and cascade in
that corpus generation; changing a quantization or provider does not erase that exclusion. A third
model-backed adjudicator receives only the two completed labels and the same blind evidence for a
disputed alias, never candidate predictions. Model agreement is therefore review evidence, not an
automatic claim of truth: the locked development corpus can select candidates but cannot certify
production, and the independently clustered holdout repeats this label protocol before it is opened
to candidate scoring.

Before another full development lock, the 32-case temporal diagnostic factors the previously
conflated model output into two claims. `UnitAssessment` answers whether the bounded span is one
standalone unit, a compilation, a programme excerpt, unusable, or unclear. `RoleAssessment` exists
only for a standalone unit and answers commercial, promo, bumper, PSA, station ID, trailer,
interstitial, or unclear. Each non-operational answer cites only signal IDs from the identity-blind
packet; the validator resolves those IDs to their package-owned timestamps. Provider, schema,
budget, and transport failures remain explicit operational failures rather than semantic labels.
The local runner makes one constrained unit call for every case and a separate constrained role call
only after the unit call returns `standalone`; it records every call's axis, response hash, latency,
tokens, and failure rather than collapsing a two-call cascade into one attempt.
The hosted adapter's provider-facing schema contains only the closed class and one to four package-
owned decisive signal IDs. Free-form explanatory prose is deliberately not accepted over that seam:
routes advertising strict JSON have emitted unescaped titles, HTML fragments, and trailing braces in
otherwise usable explanations. The adapter derives the assessment's required audit sentence from the
closed class and cited IDs after strict decoding. This preserves the decision and its evidence while
preventing non-decision prose from turning a valid classification into a transport failure. Local
diagnostic adapters may retain a model-authored sentence, but comparison and disposition never use
sentence wording as a vote or confidence signal.
Two model families run independently on the exact packet, and the deterministic comparison reports
unit agreement separately from role agreement. More than 15% disputed cases, or one confusion that
accounts for more than half the disputes, stops the development run for contract repair instead of
being hidden behind adjudication. This diagnostic artifact is non-certifying and does not replace the
complete label or holdout contracts above.

The current temporal-structure gate is a private, mechanically constructed 60-case holdout; it does
not require another full blind human viewing pass. Its planner strictly decodes and hashes the exact
48-case selection, evidence manifest and private map, locked human assessment and attestation,
full-decode media-quality report, two-family broadcast-suitability comparison, the exact 300-case
reference audit behind the duplicate-family audit, that audit's exact digest-bound download ledger,
that recomputed duplicate-family graph, a content-bound transition-edge authority, and the
programme-parent inventory. The download ledger is bounded to 16 MiB, decoded only by the
reference-audit hostile-input decoder, and joins every audit case exactly by case ID, item ID,
content SHA-256, and canonical relative local path where present. An audit case's `Source` is a
dataset label, not provenance authority: the digest-bound ledger alone supplies the authority and
canonical HTTPS item URL used for known-filler lineage exclusion. It selects 12
quality-eligible, non-prohibited standalone anchors with two bumpers, three commercials, two promos,
two PSAs, and three trailers. Every anchor has distinct source bytes and duplicate-family identity;
the family authority covers every non-excluded reference case, not merely the later 48-case
selection, and the planner independently recomputes its canonical relationship graph before use.
The replacement certification cohort has 60 cases: 12 standalone units, 12 two-item compilations,
12 three-item compilations, 12 programme excerpts, and 12 programme excerpts with one inserted
filler unit. The combined construction receipt uses schema 3 and contract
`filler-temporal-structure-holdout-plan-v7`: it retains the accepted provenance, transition, and
holdout-exclusion authorities while binding the expanded cohort. Older receipts are not upgraded
by inference and cannot authorize construction under this contract.

Replacement planning receives new source material through one immutable **replacement candidate
pool**, never through the historical programme inventory or a second raw inventory. The leaf
`fillercandidatepool` package owns the strict schema, canonical validation, and digest of
`filler-replacement-candidate-pool-v1`. A separate composition package owns its one build interface
and may depend on `fillercandidatepool`, `fillercorpus`, `fillerquarantine`, `fillerreference`, and
`fillerreview`; the planner depends only on the leaf pool contract. This direction keeps quarantine
and review independent of their consumer and avoids the existing `fillerquarantine` to
`fillerreview` dependency becoming a cycle. The review package exposes one normalized, read-only
authority view from its existing strict loaders so the builder reuses the owning validators instead
of copying their policy.

The builder considers every row in one current schema-v5 corpus inventory and emits exactly one
sorted candidate record for it. Each record has the closed kind `standalone_anchor` or
`programme_parent`, the closed disposition `eligible` or `held`, and sorted unique hold reasons.
Structural corruption or a digest-swapped authority invalidates the entire build. A valid authority
that denies one candidate instead produces a held record, so absence cannot disguise a failed
qualification gate. Eligible records bind the case, source content SHA-256, byte count, duration,
canonical path beneath the declared source root, transport, exact source authority/item/page/media
identity, metadata digest, soundtrack expectation and frozen evidence, duplicate-family identity,
and the raw digest of every authority used to derive the record. Standalone records additionally
bind the locked semantic unit and role plus their measured transition edges. Programme parents have
no standalone role or transition claim. Adapter labels such as `PSA` and `Trailer`, hand-authored
pool fields, and projected worksheet columns are never semantic, rights, quality, soundtrack, or
audience authority.

The authority matrix is closed and transport-aware:

| Candidate | Required authority |
|---|---|
| Remote standalone anchor | Current inventory; applicable development or certification rights lock; exact materialization ledger; exact quarantine-inspection report; inspected source bytes; full-decode audiovisual quality; bound `present_expected` soundtrack claim and decoded audio; duplicate-family and prior-exposure clearance; explicit suitability/audience clearance; locked `standalone` unit and role; measured transition edges |
| Local standalone anchor | The same authorities except that the inventory's direct local representation replaces the materialization ledger and quarantine report; source bytes are still confined, rehashed, and fully decoded |
| Remote programme parent | The remote standalone authorities except semantic role and transition edges; explicit programme-parent kind authority replaces the standalone semantic claim |
| Local programme parent | The local standalone authorities except semantic role and transition edges; explicit programme-parent kind authority replaces the standalone semantic claim |

`present_expected` remains pre-download selection evidence; eligible candidates must also have an
audio stream and a successful full-source decode. `intentionally_silent`, `unknown`, absent audio,
failed or incomplete decode, prohibited suitability evidence, held rights, missing quarantine
evidence for remote bytes, duplicate or related-family collision, and every prior-exposure form have
closed hold reasons. Prior exposure covers exact source bytes, duplicate family, and programme
provenance across the cumulative adjudication chain. A family with any exposed or otherwise eligible
member resolves deterministically to at most one eligible representative; the others remain visible
as held collisions. Local media never receives a fabricated download ledger or quarantine report,
but no downstream authority is waived because of its transport.

Pool construction strictly reopens the generic inventory, rights decision, materialization ledger
where applicable, quarantine report where applicable, source bytes, quality, soundtrack, semantic,
transition, family, suitability, and cumulative prior-adjudication authorities through their owning
decoders. It binds their raw hashes as sorted named inputs and hashes canonical JSON with a fixed
build time. The same inputs and build time produce byte-identical output and digest. Frozen CDC,
Blender, USGS, and LOC fixtures exercise this join without network access. Building the pool creates
no request, download, media mutation, model call, training data, catalog item, schedule, certification
result, or production-admission grant; all corresponding authority flags are present and false.

Genesis planning retains the schema-v3/v7 receipt and its exact historical selection, review,
reference, transition, and programme-inventory inputs so the burned challenge remains reproducible.
Replacement planning uses the next current receipt contract, requires one exact current pool plus
the complete prior-adjudication chain, rejects every historical programme inventory and all raw
candidate-authority paths, and selects both anchors and programme parents only from eligible pool
records. It independently reproduces the pool digest, requires the pool's prior-exposure binding to
equal the reopened cumulative chain, and records the pool hash as `replacement_candidate_pool` in
the receipt. Candidate selection remains seed-deterministic, while the cumulative future-training
exclusion continues to include prior and newly selected source hashes, families, and programme
provenance. Historical pool or receipt schemas remain readable evidence only and are never upgraded
by inference.

**Transition evidence is a prior measurement authority, not a label inferred from the chosen
corpus.** A development-only generator measures all 48 exact evidence cases before role, safety, or
quality eligibility is consulted. It binds the evidence-manifest and private-map hashes, each exact
source SHA-256 and duration, the resolved FFmpeg path/version/binary SHA-256, one fixed generation
time, and the common renderer profile (960-by-720 aspect-preserving padding, 30 fps, yuv420p,
48 kHz stereo). For both the first and final 1,000 ms it retains the PTS-normalised, window-clamped black and
silence intervals plus RMS and peak support over the boundary-adjacent 100 ms. The detectors are
fixed at `blackdetect=d=0.040:pix_th=0.10` and `silencedetect=n=-40dB:d=0.040`; an interval touches a
boundary only when it begins or ends within one rendered frame (34 ms) of that boundary. Missing or
extra cases, repeated case/source identities, changed bytes/duration/tool/profile/policy, malformed
intervals, a missing required stream, or any decode failure invalidates the complete authority.
Generation performs no semantic classification, inference, rendering, training, or admission.

The earlier words *abrupt*, *black-frame*, and *audio-continuous* mixed a construction property with
two overlapping media cues and claimed more than an amplitude detector proves. Every compilation is
already a hard concatenation. Quota selection therefore uses three closed, factual strata derived
from the two independently measured edges, in precedence order: `black_boundary` when either edge
has boundary-touching black; `audible_nonblack_cut` when neither edge has boundary black and neither
has measured boundary silence; and `silence_touched_nonblack_cut` when neither edge has boundary
black and at least one has measured boundary silence. “Audible” here means only “no >=40 ms interval
below -40 dBFS was measured”; it is not a perceptual continuity or airworthiness claim. A pair whose
edge record is absent or failed is unresolved and ineligible.

Anchor and pair selection is one deterministic constraint search over the complete otherwise-
eligible pool, not role selection followed by a best-effort transition bolt-on. Anchor feasibility
uses stable private case-id order so a secret used for blinding cannot make planning time unbounded;
qualifying pair choice, case identity, parent choice, and public alias/order remain seed-ranked. The chosen anchors
construct 12 two-item compilations with four actual joins in each of the first 40%, middle 20%, and
final 40% of playback. Within every band, all eight source uses are distinct, exactly two pairs are
same-role and two cross-role, and at least one pair occupies each transition stratum; the fourth
stratum is seed-ranked rather than silently used to weaken another quota. If all role, family,
source, timing, role-pair, and transition constraints cannot be satisfied simultaneously, planning
fails.

Six distinct programme parents have unique bytes and unique `(provenance authority, reference)`
pairs. Every submitted parent, including an unselected parent, is rejected if its content hash,
canonical relative local path, authority/item-ID pair, or canonical reference URL repeats
an entry in the complete 300-row digest-bound filler download ledger (including unselected, held,
and excluded cases). A URL-identity collision is rejected even when bytes, local path, and item ID
differ, and does not depend on matching authority labels. A programme parent is bound to an immutable
local metadata cache and a local `fillercorpus` inventory-v4 source record under the configured source
root. The loader rejects missing, escaping, symlinked, or over-16-MiB metadata caches and source
records, and rejects malformed source records. It hashes the raw metadata cache bytes and requires
that digest to agree with both the programme inventory and the matched source record; this binds
the cache's byte integrity, not its source-specific semantic format. Source records use only the
canonical `fillercorpus` decoder, never an
arbitrary JSON self-description. A parent names the source record path and the exact source
`(authority, itemId)`; its public provenance `reference` is the canonical HTTPS item URL (no
fragment, lower-case host), and must equal that record's normalized `itemUrl`. The record's
`metadataUrl`, retrieval time, metadata digest/cache, and local representation path, SHA-256,
byte size, and duration must all bind the parent to the actual source-root bytes. Authority and
item ID compare exactly after trimming; URLs compare by this canonical form; local source-root
paths compare as slash-normalized relative paths. Unsupported source-record formats or ambiguous
identity matches are rejected. This is origin binding and known-filler lineage exclusion only: it
does not certify that a programme is globally non-filler.

Every anchor appears exactly three times across the three-item set: six cases contain an adjacent
same-role join and six contain only mixed-role joins. Each of the six seed-ranked programme parents
supplies a 30-second near-start excerpt at `[10s,40s)` and a 45-second near-end excerpt ending ten
seconds before the parent ends. Each parent is at least 120 seconds, and both excerpts retain at
least ten seconds of omitted context on either side. These are construction positions, not a
semantic scene-boundary claim. Each parent also supplies one early and one late inserted-spot
case, with the source's exact programme spans and one independently selected filler anchor. Every
anchor appears exactly once as an inserted spot. The receipt binds these additional constructions
and their exact spans alongside the original two-item transition constraints.

The planner emits only coordinator-private construction authoring and a receipt binding all input
and output digests, deterministic seed ranks, selected families, role quotas, two- and three-item compilation constructions,
their measured transition stratum and join positions, programme cuts, and inserted-spot constructions. The receipt is also the
future split authority: it declares `split=holdout` and enumerates every selected source SHA-256,
duplicate-family id, and programme provenance pair that later development/training corpus builders
must exclude. Missing or broader inferred exclusions are not accepted; a future split change needs
a new independently reviewed artifact. It performs no rendering or inference. The structure
challenge preparer requires the authoring and its structurally validated current-contract receipt,
then records both digests and the plan contract in private challenge authority; authoring alone is
not render authority. It renders and freshly blinds the bound plan before two distinct direct-video
model families assess it serially. The direct-video request contract reserves 4,096 completion tokens so provider-required hidden
reasoning cannot consume the output budget needed for the strict JSON answer. The hosted-model
interface requests one ordered, coverage-preserving timeline with a role, exclusive end timestamp,
and evidence for each interval. The first interval starts at zero and each later start is the
preceding end; redundant starts and a second whole-file classification are not requested. The
adapter coalesces adjacent programme-fragment observations, never adjacent filler intervals, then
derives the internal whole-file claim from the complete timeline. One filler interval is a
standalone unit, two or more non-programme intervals a compilation, one programme interval a
programme excerpt, and programme intervals surrounding filler a programme with spots. A single
ambiguous or non-filler interval is unclear; a single unusable interval is unusable; other mixed
shapes are unclear. Standalone role evidence comes from its sole interval. The shared strict
decoder retains every semantic and length constraint unsupported by a provider-facing schema.
Schemas use the subset accepted by the exact pinned route and do not use `uniqueItems`. The
untouched raw response remains the inference authority; transport adapters do not reinterpret it.
Every source
must have an audio stream. Before concatenation,
every segment is encoded deterministically to a common 960-by-720, 30-frame-per-second video and
48 kHz stereo AAC audio profile with aspect-preserving padding and a fixed video track time base;
The renderer probes each normalized part and the concatenated output, rejecting any departure
from 960-by-720 H.264/yuv420p at 30 fps with exactly one 48 kHz stereo AAC stream. The builder
independently validates the returned measured profile before publication. Challenge contract v3
binds those stream facts alongside each public video's dimensions and byte hash, and its loader
rejects missing or nonconforming profiles. These uniform technical facts disclose no source or
semantic labels. Historical challenge artifacts are not rewritten or upgraded by inference;
the measured part durations, rather than requested timestamps, remain the join
authority. Coverage-only suitability holds may remain as evaluation material;
prohibited and operational holds cannot be selected. The constructed truth can test unit boundaries
without a second blind full-corpus review, but it cannot establish broadcast suitability, enter
training data, or authorize production admission. Plan contract v4 requires the receipt to contain
`blindHumanAuditRequired=false`, `trainingAllowed=false`, and `productionAdmissionAllowed=false`.
Missing, null, or true dispositions are rejected before media probing or rendering. Older receipts
remain immutable historical evidence, not current-contract rendering authority. This explicit
no-full-blind-audit disposition does not replace targeted human adjudication of challenged anchors.

The single-video protocol is only the short-source slice. It cannot authorize general compilation
reels: a 579.5-second construction from the 12 independently reviewed anchors exceeds the 64 MiB
transport ceiling. The separately versioned long-reel protocol therefore plans complete primary
coverage in two-minute spans with 15 seconds of context on both sides, clamped only at source ends,
for at most 30 minutes and 15 windows. Primary spans partition the source exactly; overlap supplies
context but never creates a second boundary vote. A boundary exactly on a primary seam belongs to the
right-hand span. These limits describe protocol capacity, not production authority: every normalized
window must still fit the media byte ceiling, every window must complete for one assessor family to
produce one source-level candidate, and a real seam-focused certificate must authorize the duration
slice before use. Sparse sampling, chunk-majority voting, and silently lowering the canonical media
quality are not valid long-reel fallbacks.
Production chooses between those protocols once, before media preparation, from the immutable
source duration and an explicitly configured short-source ceiling that must equal the activated
short certificate's duration envelope. Sources at or below that ceiling use the complete-video
runtime; longer sources use the window runtime only through its declared 30-minute capacity. A
selected runtime's preparation, provider, evidence, or reduction failure is final for that attempt:
the router never falls through to another representation, because doing so would silently change
the prompt, media authority, accounting operation, and certification slice. A source outside both
duration envelopes holds before either runtime can prepare media or reserve provider spend.

One content-addressed media-set artifact embeds the complete plan and the ordinal, normalized media
identity, and lineage of every window; its ordered membership is the common input authority for all
assessor families. The preparer takes one full-hash- and sparse-hash-verified immutable source snapshot,
renders every declared media interval through the same canonical 960-by-720 profile as the short path,
fully decodes each output, and publishes media and lineage only by content address. Reuse re-hashes,
re-probes, and fully decodes the retained bytes; no partial set is returned after a failed window.
Each accepted window answer covers its complete media interval in source-relative
coordinates and binds that exact media set, its own ordinal, and the assessor profile. An attributable
transport or provider failure is retained as an operational-failure answer with no semantic timeline;
it holds that assessor family's complete source result rather than letting the remaining windows vote.
The deterministic stitcher requires exactly one answer for every planned window from one assessor
profile. For each adjacent pair it compares the complete ordered boundary sequence and the role on
both sides throughout their shared context. Matching observations no more than 2,000 ms apart become
one boundary at their deterministic mean; a missing, extra, differently typed, or farther-apart
observation holds the complete family result. A boundary observed only in non-owning context is not
projected, while a matched observation that straddles a primary seam is retained. Projection must
reproduce one ordered timeline covering `[0,duration)` and preserves adjacent same-role intervals:
two consecutive commercials are two units even though both carry `commercial`. The stitch artifact
retains the plan and every window answer and replays byte-for-byte; it is one assessor-family
candidate, not independent agreement and not split authority.

The direct-video runner's per-request nanodollar value is an **accounting reservation**, not a
provider-enforced total-price cap. OpenRouter does not expose such a cap for token-priced video.
The transport therefore preserves every syntactically valid provider-reported charge before it
checks the reservation. A charge above the reservation closes that attempt as
`over_reservation`, records the exact decimal and nanodollars, request/response digests, generation,
route, tokens, and reserved amount, and makes the structured answer semantically unusable. The
actual charge, even when it exceeds the run's authorized spend, is the consumed-spend truth; the
overrun is explicit and no later request may start. Missing or malformed settlement remains the
separate unknown-charge state and consumes the reservation. The runner must never describe either
the reservation or the run authorization as a hard provider billing limit. Before media is opened,
the runner also multiplies the pinned route's snapshot prompt/completion prices by a declared
worst-case input-token allowance and the fixed completion-token ceiling using exact decimal
arithmetic; a reservation smaller than that bound is refused before HTTP.

The production-domain hosted structure assessor profile separates durable semantic identity from one
live metadata capture. `modelDigest` hashes the requested model id, its exact catalog canonical revision, and the
provider creation identity. `capabilitySha256` hashes that model identity together with the selected
upstream provider name and selector slug, input/output modalities, quantization, context and token
limits, supported parameters, ZDR membership, implicit-cache behavior, and the selected reasoning
mode. Both projections exclude
the snapshot retrieval time, request/response accounting, endpoint liveness status, display names,
and prices: those are capture facts, not a different model or route capability. The complete
metadata snapshot SHA-256 remains separately bound to every certification family result and supplies
freshness, current liveness, exact price, and historical audit evidence. A live or calibration runner
must still validate a fresh under-24-hour snapshot and exact ZDR route before media is opened or spend
is reserved. Every provider reservation and settled call record also binds that complete snapshot
digest, so a production decision artifact's retained evidence can replay which live metadata capture
authorized its route and price. It may reproduce a certified profile from a later fresh snapshot only when both stable
digests match; a capability or canonical-model change holds for recertification, while a price change
changes the reservation evidence without pretending that the assessor itself changed.

The immutable structure-certification report reproduces the comparison from at least two locked,
distinct model-family assessment sets and binds its digest to the exact public manifest, private
construction authority, holdout authoring, and source-family receipt. Certification requires all 60
cases and at least six cases from each predeclared difficult slice: two-item compilations, three-item
compilations, adjacent same-role joins, mixed-role joins, programme cuts near either parent edge, and
inserted spots in either the early or late position. Every assessor must have zero operational
failures, under-splits, over-splits, incomplete timelines, wrong segment roles, or structural
boundary misses beyond 2,000 ms, both globally and in every slice. A passing report still grants no
training or production-admission permission; its only next action is the locked shadow comparison.

Long-reel certification is a separate content-addressed authority over the window protocol rather
than an extrapolation from that complete-video report. Its private suite binds every case to one real,
replay-valid window media set, a complete known-truth source timeline, and the complete set of derived
seam traits. Non-geometric traits such as a wordless join or high-motion window additionally name an
independent measured-evidence digest; a coordinator declaration alone does not prove them. The suite
must contain at least six cases in each predeclared slice: a boundary in shared overlap, immediately
left and immediately right of primary ownership seams, an adjacent same-role join, one unit crossing
a seam, a programme/filler join, a wordless join, a high-motion window, and the six largest
successfully encoded windows in the fixed corpus. The suite records and reproduces the resulting
minimum high-byte threshold; separately constructed over-ceiling cases must hold before inference
rather than being mislabeled as successful stress cases. Exactly two locked, distinct assessor families
supply complete persisted
stitches for every case. The certification judge replays each stitch, scores each family independently
against private truth, then feeds the same two source-level candidates through `fillerstructure.Reduce`
and scores that confirmed decision too. Any missing case or stitch, held family, operational failure,
under-split, over-split, wrong role, incomplete coverage, or boundary error beyond 2,000 ms fails the
affected slice and the whole certificate. The immutable report names the suite, assessor profiles,
slice counts, errors, and reducer contract. It always sets training and automatic-materialization
permission false: a pass is evidence from which a separate window-specific materialization authority
may later be issued after the locked short-versus-long shadow comparison.

That shadow comparison is a provider-neutral replay over the complete 28-case long-reel corpus, not
a second truth-scoring pass. For every opaque case it receives the two immutable reducer artifacts
produced from the same exact source: one through `complete_video` and one through
`window_media_set`. It revalidates both artifacts, requires the same reducer version, boundary
tolerance, source identity, and ordered pair of underlying model families, and requires each paired
family to retain the same provider, declared model, model digest, and capability snapshot across the
two representations. Prompt and evidence-contract identities remain representation-specific and are
therefore bound in the report rather than required to be equal. Both decisions must be confirmed and
must agree on whole-source unit and standalone role, interval count, every interval role and
disposition, and every internal boundary within the certified tolerance. Missing, duplicate, held,
wrong-kind, differently sourced, differently profiled, or semantically divergent cases fail the
whole shadow report; there is no majority or partial-slice pass. The content-addressed report binds
both artifact digests for every case and always leaves training and automatic materialization false.
A repository publication wrapper additionally binds the content and file digests of the passing
window certificate and both complete decision sets, so the family-result lineage checked before the
comparison remains recoverable rather than becoming an unrecorded coordinator precondition. The
wrapper embeds the self-contained replayable report and grants neither training, production
admission, nor automatic materialization authority.
A separately reviewed long-reel authority may consume only a complete passing report together with
the passing window certificate; the shadow report cannot activate production by itself.

Each family run is a separate truth-blind artifact over the complete 28-case public window-set
manifest. The runner receives only opaque aliases, exact public source and media-set authority, and
machine-local window paths; it cannot open case identifiers, construction truth, measured slice
labels, or another family's answers. It evaluates cases and windows serially through the same
production family runtime, returns no partial result after an error, and retains one replay-valid
stitch per alias plus every ordered call record and completed-operation publication that produced
it. The result reproduces provider-request count, known charge, conservative accounted spend, and
unknown-charge reservations and binds them with the exact assessor profile, manifest digest,
complete metadata-snapshot digest, completion time, and self-digest.
Publication reopens the complete public manifest and creates the private result file immutably. The
private certification join starts only after both complete family artifacts exist, attaches case
identifiers by exact media-set identity from the suite, and binds its wrapper digest to the public
manifest, suite content and file digests, and both family content and file digests. Neither the family
artifact nor its certification wrapper grants training or automatic-materialization permission.
The calibration command requires its declared request ceiling to equal the manifest's complete
window count before opening the provider transport, rejects an existing result path before spending,
and uses the production SQLite call ledger for aggregate spend reservation and settlement. Replaying
settled content-addressed evidence consumes no request; a crash-open request conflicts in that ledger
before transport rather than being retried speculatively.

The complete-video half of the shadow uses the same truth-blind, one-family-at-a-time discipline over
the public 28-case manifest. Its production preparer creates or revalidates one exact canonical
derivative per source before the family call. A completed-operation publication keyed by source,
derivative, assessor profile, current prompt digest, and current schema digest points to the full
settled call record only after response, structured output, and record bytes are durable. Restart
therefore replays accepted or closed failure evidence without another request; an interrupted call
whose reservation has no completed publication remains held by the durable inference ledger rather
than being guessed or repeated. Each complete-video family result retains those records and cost
totals and grants no authority. Only two complete family results may be reduced into the complete-
video shadow decision set, and both decision sets must descend from their exact family artifacts
before the representation comparison begins.

The first long-reel corpus plan reuses only the twelve family-distinct bounded anchors and six
programme parents already locked by the 60-case holdout. One private deterministic planner binds
that authoring and receipt and emits 28 programme-with-spots constructions: six place a same-role
filler join ten seconds inside shared seam context, six place it one second left of primary ownership,
six place it one second right, and six place one complete filler unit across a primary seam. Programme
material brackets every construction, filler sources are always used whole, same-role pairs never
share a source or source family, and exact source-relative truth is derived from requested parts rather
than entered separately. The programme prefix begins one third into its parent. For the four
duration-edge constructions, the suffix begins two thirds into that parent rather than being packed
against EOF; even the shortest locked parent therefore retains more than fifteen seconds between the
longest possible prefix and suffix and more than fifteen seconds after that suffix. The seam
constructions retain their separate ten-second end margin. Packet measurement found ten-second
timestamp holes at 100 seconds in one otherwise useful parent and near 650 seconds in another, so
neither an arbitrary ten-second prefix start nor a long EOF-relative edge suffix is continuous
evidence. Before publishing a plan, the planner verifies every requested part against its locked
source bounds and the applicable seam or duration-edge context margins; insufficient programme
parents fail planning without publishing a plan. The plan performs no rendering or model call. A later renderer measures the
canonical encoded parts and may publish a certification suite only when wordless, motion, and largest-
byte evidence also satisfy the fixed slice counts; insufficiency fails the corpus rather than causing
model-outcome-guided case substitution.
The remaining four constructions establish the first intended continuous production-duration
slice without changing those seam cohorts after seeing model output. Two place one whole bounded
filler between programme excerpts in a 121-second source, immediately above the 120-second short
slice. Two use the same real programme/filler/programme shape at 301 seconds, within the fixed
tolerance below the first 302-second windowed ceiling. The pairs use distinct filler families and
programme parents. These edge cases participate in window certification and the locked
complete-video comparison; later expansion beyond the complete-video transport envelope requires
a separate window-only duration certificate rather than pretending this first slice proves the
protocol's full 30-minute capacity.

Rendering that plan is one atomic, non-authorizing operation. It reopens and hashes every declared
source before any output is created, renders each construction through the canonical structure-media
recipe, completely decodes both output streams, and publishes only opaque case names plus exact
full-file, sparse-file, duration, profile, tool, and complete-coverage window-plan identities. Its
private authority retains the construction plan, source provenance, requested parts, measured encoded
part durations, and the resulting complete source-relative truth. Interior truth boundaries come from
the measured encoded part joins rather than requested timestamps; only a bounded final container-
duration reconciliation may extend or trim the final programme interval. A rendered case that leaves
its predeclared seam slice, exceeds the ordinary retained-source byte ceiling, lacks audio or video,
or fails complete decode aborts the entire publication. The resulting files remain corpus inputs, not
training examples or production-admission authority.

One subsequent atomic packager reopens that public/private join, snapshots every exact constructed
source into its own staging root, and invokes the production structure-window media preparer over the
already-bound complete-coverage plan. Its public manifest exposes only opaque aliases, exact source
identity, path-free media sets, and the local paths of their content-addressed windows; its private
authority binds those aliases back to the rendered truth. A missing window, byte or lineage drift,
profile mismatch, over-ceiling window, incomplete decode, or partial source snapshot aborts the whole
package. This packager does not measure semantic traits, call an assessor, assign a certificate, or
grant materialization authority.

The suite assembler accepts only those locked media and pre-model authorities; model responses are
not an input. A wordless join is proven when at least one independently bounded filler source beside
the planned join has a retained transcript artifact containing one or more closed non-speech markers
and no lexical segment. The source-to-evidence alias and transcript digest must replay through the
holdout receipt. Motion is measured over every prepared window as the mean absolute luma delta between
each pair of adjacent decoded frames; the evidence also retains frame count, sum, 95th percentile,
maximum, exact media identity, and ffmpeg identity. The highest-scoring window in each case competes
for the fixed six-case high-motion cohort, with stable case/ordinal tie-breaking, and the sixth score
becomes the reproduced cohort threshold. Selecting one window per case prevents several energetic
windows from one construction from satisfying the slice by themselves. Both measured slices are fixed
before inference, and insufficiency aborts suite publication rather than substituting cases after model
outcomes are known.

Before either paid representation run, one provider-free preflight reopens the complete public
window set and private certification suite, rehashes every retained source and window, and proves
that the suite contains exactly the same media sets. The operator must declare the already-certified
short-source ceiling and the intended first long-source ceiling. The preflight reports the observed
minimum and maximum source durations, window and byte envelope, exact per-family window and
complete-video request counts, and the four-run total. It is ready only when at least two sealed
cases occur within the fixed timeline tolerance above the short ceiling and at least two occur within
that tolerance below the intended long ceiling; any case beyond the intended ceiling also fails
readiness. The content-addressed report grants no authority and performs no provider or metadata
request. A failed report's only next action is to extend and rerender the sealed corpus before paid
assessment, preventing cost approval or a passing semantic score from silently standing in for an
unrepresented production duration slice.

The agreement policy itself is one provider-neutral production-domain module, not a scorer-owned
copy: both the challenge adapter and the eventual production runtime supply source-bound immutable
candidate assessments to the same pure reducer. Challenge aliases, private truth, provider clients,
and persistence stay outside that module. Its interface retains exact source identity, complete
timelines, assessor/model-family identity, immutable assessment identity, and every disagreement.
The shared assessment-input manifest represents either one complete normalized video or one complete
ordered window media set; it binds the source, media profile, every derivative identity and lineage,
and the window plan digest when applicable. Candidates name that manifest's digest. The reducer never
models a window set as one synthetic video and never needs provider or filesystem knowledge;
this makes implementation drift between the passing certificate and the deployed decision path a
compile-time architecture error rather than a rollout convention.
Production persists that reducer input and output as one content-addressed decision artifact before
the split proposal may consume it. The artifact identifies the reducer contract and boundary
tolerance, retains every independent complete-timeline candidate or operational failure, and
reproduces the decision byte-for-byte when validated. The proposal document binds the artifact's
media digest and duration to its exact retained source. An invalid, drifted, missing, or held artifact
cannot certify the heuristic assessment; a later certification authority must verify its declared
assessor slices rather than replacing those durable identities with a boolean callback.
The complete-plan gate independently checks that a confirmed artifact and the proposal assessment
name the same source unit and the same exhaustive ordered spans, roles, and filler/non-filler
dispositions. A certification callback therefore cannot turn a detector-only plan into model
agreement or approve a projection whose programme spans became filler children.
Structure release authority is itself a content-addressed document, not an injected predicate. It
locks the external certificate digest, reducer contract and tolerance, exact assessor/model-family/
provider/model/capability/prompt/evidence-contract profiles, and the allowed source-unit and segment-
role slices. Verification requires an exact profile set and a confirmed artifact; unknown profiles,
units, roles, held decisions, or an authority without explicit production permission fail closed.
The current short-source authority admits only the complete-video input kind. Window-set activation
requires its separately measured certificate and receives a distinct authority contract; a short-
source certificate cannot authorize a long reel merely because both use the same encode profile.
That long-reel authority binds the passing window certificate and short-versus-long shadow digests,
the canonical window-profile identity, the exact window assessor profiles, the observed source-
duration and per-window byte envelope, and only the source units and segment roles present in every
passing shadow case. Issuance records one bounded reviewer identity and canonical review time and
requires an explicit materialization-permission flag. Verification reconstructs the canonical plan
from the artifact's source, matches its plan digest and complete ordered item count, and checks every
window's measured duration, media profile, and byte ceiling. It can create held child work only;
training and broadcast admission remain separate authorities.
A second content-addressed deployment document turns that reviewed authority into an executable but
still fail-closed production configuration. It binds the authority digest; explicit automatic-
assessment permission; exactly the authority's ordered assessor ids, model families, and requested
models; each exact upstream provider name and selector slug; reasoning mode; worst-case input-token
allowance; per-request accounting reservation; and positive per-source and per-day spend ceilings.
The per-source ceiling must reserve every certified window for both families before activation. The
document contains no credential, filesystem path, current canonical model revision, mutable price, or
metadata capture. Production reads the OpenRouter credential from its existing provider secret,
fetches and caches a fresh canonical metadata snapshot for less than half its 24-hour validity,
recomputes every stable assessor profile, and requires exact equality with the reviewed authority
before preparing media. It then price-checks every route against the deployment reservation and uses
the complete fresh snapshot digest in every durable call reservation and settlement. Missing or
invalid authority, deployment, credential, route, snapshot, evidence store, media preparer, or budget
leaves independent assessment and certified materialization disabled; it never restores the heuristic
gate under a certified policy label.
The runtime assessment coordinator calls each configured complete-timeline assessor serially with
the same immutable conditioned-media identity and path. Its port exposes no prior answers. Each
adapter must return either a complete source-bound candidate or an attributable operational-failure
candidate; ordinary provider failures are domain evidence, while an error means the adapter could
not produce trustworthy evidence and aborts reduction. The coordinator rejects declared-profile or
source drift before creating the durable artifact.
The long-reel coordinator preserves those semantics in family-major order. It prepares one complete
window media set, then calls every window for the first assessor serially, durably commits each
validated window answer before starting the next, deterministically stitches and commits that family,
and only then repeats for the next assessor. An assessor receives exactly one path, its source-relative
window geometry, and the common media-set identity; the interface exposes no peer or earlier-family
answers. A normal provider failure returns an attributable operational-failure window and still closes
the family as a held stitch. An error, profile/media/ordinal drift, or failure to persist any answer or
stitch aborts without reduction. Only persisted, replay-valid family stitches become whole-source
candidates, and the final decision artifact is persisted before return.
Each window call crosses the persistence seam as one separately versioned recorded assessment, not
as a bare semantic answer. Its reservation and settlement embed the complete media-set authority and
bind the exact ordinal, assessor profile, prompt and schema digests, requested and resolved route,
generation, tokens, requested/reserved/charged/accounted nanodollars, closed state, raw-response and
structured-output digests, and the resulting semantic window-assessment digest. The request is
durably reserved before transport. Response and structured-output blobs plus the semantic assessment
publish before the settlement record, and the coordinator reloads that complete record before the
answer may enter stitching. Accepted output is parsed in window-local coordinates and projected
deterministically onto the plan's source-relative media interval; the exact planned interval, rather
than encoder-duration drift, remains the coverage authority. Budget holds, route drift, provider or
schema failure, unknown settlement, and reservation overrun each produce one closed operational-
failure assessment with no segments. Cancellation does not bypass settlement: once a reservation
exists, the adapter settles through a bounded context detached from caller cancellation. A paid or
reserved call therefore cannot influence a boundary from memory alone, disappear from accounting,
or be mistaken for semantic evidence.
The completed-call lookup is itself immutable and deterministic. An operation identity hashes the
exact media-set digest, ordinal, and complete assessor profile; its publication binds that operation
to exactly one call-record digest and is written only after all referenced evidence and the durable
settlement exist. On restart, the coordinator resolves this identity before invoking an assessor,
strictly reloads every referenced byte, and reuses the answer only when the complete authority
revalidates. A second, different record for one operation is a conflict, not a retry. A missing
publication permits a new call only when no durable reservation for that exact request exists; an
open crash reservation remains an explicit unknown-charge hold and must never cause an automatic
duplicate provider call. Completed earlier windows therefore resume without repayment while an
interrupted in-flight window fails closed instead of guessing whether the provider charged it.
The conditioned-media identity also binds a content-addressed, path-free lineage document. That
document names the original source identity, the complete canonical assessment-media profile, the
exact ffmpeg version and executable digest, and the normalized derivative's digest, byte count, and
measured duration. Production renders the derivative into staging, validates its streams and profile,
then atomically publishes both derivative and lineage beneath the hidden media tree. A retry may reuse
that derivative only after strictly decoding and re-hashing the lineage, re-hashing and re-probing the
media, and reproducing the operation key from source, profile, and tool identities. Filesystem paths
are locations, never authority. A missing, drifted, oversized, incomplete-stream, or out-of-profile
derivative fails before either assessor or provider is called.
The direct-video prompt version, identity-blind system instructions, dynamic duration message, JSON
schema, strict decoder, programme-only coalescing, whole-source unit derivation, evidence bounds,
and reducer-candidate projection are one `fillerstructure` contract used by evaluation and runtime.
Provider adapters supply transport and durable attribution only; they may not carry a private parser
or reinterpret interval roles.
Each runtime assessor returns one content-addressed assessment record rather than an unaudited
candidate. The record binds the exact source bytes and duration, declared assessor profile, request
and raw-response digests, requested and resolved route, generation, tokens, accounting reservation,
provider charge, closed operational state, and either one complete parsed timeline or no semantic
claim. The coordinator verifies the supplied raw bytes against that record and commits both through
its evidence-repository seam before projecting a reducer candidate. A missing response for a closed
transport failure remains explicit; an unsettled, budget-held, over-reservation, route-drifted, or
invalid-response record becomes attributable operational-hold evidence. Persistence failure aborts
the reduction, so a paid call can never influence a split from memory alone.
Only one confirmed artifact may project model-decided spans into V67. The projection validates and
replays the artifact, binds it to the proposal's exact source, and constructs the V67 assessment
from the artifact's exhaustive ordered intervals. Previous
chapter, black, silence, transcript, and sparse-frame observations remain content-addressed context;
their boundary effects are neutralized in the projected assessment so they cannot impersonate a
third assessor, add an interval, or override the certified reducer. The detector-authored proposal
remains intact for compatibility comparison; a later certified split projection joins metadata only
onto exact decided spans and does not carry stale detector failure or hold decisions. Programme and
non-filler intervals remain explicit discards and never enter the child-confirm list.

The certified gate does not translate complete-timeline agreement into the legacy detector's
boundary-confidence percentage or require that percentage to authorize the same cut a verified
artifact already establishes. It still applies deterministic duplicate and duration refusals,
grounded taxonomy requirements, the four exact-span screens, and the immutable structure authority.
The compatibility calculation is non-authorizing measurement only. Missing complete-timeline
evidence or materialization authority holds the proposal; it never selects the older automatic gate.

Opening that comparison can invalidate inherited anchor truth without authorizing a post-hoc score
repair. A targeted anchor-adjudication module consumes the exact public and private challenge,
plan authoring and receipt, all locked assessment sets named by the comparison, the immutable
comparison itself, and one reviewer submission. It first reproduces the comparison byte-for-byte.
The review target is then exactly every construction-authority `standalone` case named by the
comparison's diagnostic candidates: it cannot omit an inconvenient target or expand into a new
full-corpus audit. Each target records complete-span audiovisual coverage, explicit bounded
observations of the opening, ordered internal joins, and closing, one reviewer identity and fixed
review time, sorted unique decisive timestamps, a bounded rationale, the original closed unit/role,
and exactly one disposition: `confirmed_original`, `structural_disqualification`, or
`role_correction`. A structural disqualification must replace `standalone` with a non-standalone
unit and no role; a role correction must retain `standalone` and select a different valid role.
Model agreement selects what receives review but never becomes truth authority by itself.

The publisher preserves every original input and emits a new owner-only authority rather than
editing the human lock, plan, challenge, model responses, locks, or comparison. It binds every input
file hash, the plan's human-assessment and evidence-manifest hashes, the exact challenged source
bytes and duplicate family, the review decisions, the prior receipt's complete future-training
exclusion, and every rendered video hash exposed in a model request. Its output always declares the
evaluated challenge burned, training false, and production
admission false. A later holdout-plan contract must consume that authority as prior exposure: no
source bytes, duplicate family, or programme provenance from the burned challenge may appear in a
replacement challenge, and its future-training exclusion must be the exact cumulative union. Until
that replacement machinery and new source inventory exist, adjudication can explain and quarantine
the bad truth but cannot manufacture a corrected certification score. The planner makes lineage
explicit: an invocation is either a genesis plan with no prior adjudication, or a replacement plan
with one or more immutable prior adjudication authorities; omitting both or mixing the modes fails.
A replacement validates every prior authority, rejects any candidate whose source bytes, duplicate
family, or programme provenance appears in their cumulative exposure, binds their file hashes in the
new receipt, and publishes the sorted de-duplicated union of prior and newly selected exposure. The
genesis/replacement mode and prior-exposure set are part of the plan contract, so a caller cannot
silently forget burned evidence while requesting a replacement.

Suitability screening is repeated over every freshly rendered structure case because concatenation
and excerpt construction create new viewing contexts. Its prompt identity binds the system prompt,
sentinel dynamic content and schema, request title, and 4,096-token completion ceiling so mandatory
provider reasoning cannot consume the structured answer budget. A prohibited signal in any derivative is a
conservative source-level quarantine: the private construction authority projects the observation
back to every overlapping source segment, and every case derived from that source remains held even
when another model reports no signal. Repetition at the same source-relative interval strengthens
the quarantine evidence; it is not counted as independent corroboration. A model non-flag is never a
safety certificate, and majority voting cannot clear a prohibited observation. Quarantined media may
remain in the immutable evaluation package so the miss is measurable, but cannot enter training,
catalog ingestion, scheduling, or production. Operational and incomplete-modality outcomes also
remain held. Only an independently specified suitability-recall certification can turn complete
no-signal observations into an admission claim.

The source projection is one separate deterministic private seam. It consumes the exact public
structure manifest, private construction authority, two-family comparison, and both immutable
result files named by that comparison; the result files retain the observation ranges that the
summary comparison deliberately does not duplicate. It first reproduces the comparison from those
results, rejects a comparison timestamp earlier than either completed result, and then intersects
every observation with measured output segments. Encoder-duration drift expands the projected
source interval conservatively by the bounded render drift rather than narrowing it. Overlapping
same-kind, same-modality source intervals merge into one observation with ordered assessor/case
witnesses. The private report lists every source and derivative case, propagates a source quarantine
to derivatives that had no direct flag, and keeps `trainingAllowed`, `ingestionAllowed`,
`schedulingAllowed`, and `productionAdmissionAllowed` false. It performs no inference, catalog
write, media mutation, or admission decision.

The private projection also retains the verified immutable challenge-v1 evidence contract required
by #903 and the retained complete-source spoken diagnostic in #912. This is an archived diagnostic
input, not a current challenge or certification. Only the source-suitability and spoken-safety
projection seams may load it through the same private strict decoder; public challenge, assessment,
and rendering interfaces remain on
the current challenge and plan contracts. Separate strict archived input shapes reject fields
introduced by later contracts even when empty or null. They require the original explicit negative
production disposition, complete media/authority/alias/segment bindings, and the exact immutable
result/comparison bindings. The private validator requires the canonical assessment-media profile
for current challenges and its absence for challenge-v1 diagnostics, whose schema predates that
field. Decode and hash the same raw bytes; reject unknown, duplicate, trailing,
mismatched, ambiguous, or incomplete evidence before publication. Archived inputs cannot substitute
for current measured profiles, plan receipts, source provenance, or ledger evidence. No version
string, original artifact, or provider result is rewritten to make historical evidence appear current.
The projected report preserves the original input digests and all four false permissions, including
when reproducing an archived no-signal observation. This narrowly retained external evidence
contract does not provide a general legacy-loader option or an admission fallback.

Spoken-language safety is a separate complete-source evidence seam; neither the production catalog
transcript nor a direct-video model's spoken-language answer can satisfy it alone. The catalog
transcript is deliberately selective and normally samples only the `LanguageSpan` window, while the safety
seam transcribes `[0, measured source duration)` for every source in the exact corpus manifest and
every additional source named only by the construction authority. It validates the label-blind
packet set and its external media bytes against that corpus manifest before evaluating transcripts,
then binds the source, packet, extracted audio, ffmpeg, whisper executable, model, implementation,
timing, and completion identities in an immutable private transcript artifact. The smaller review
evidence set is a projection target, not the scanner's population. Wordless is a completed outcome
only after that full span runs successfully. Missing audio, an engine error, unordered or
out-of-range timing, identity drift, or an incomplete source set is a coverage hold, never a clean
observation.

One deep `PublishTemporalSpokenSafety` module interface owns strict authority loading, complete-span
transcript-artifact validation, private policy evaluation, source projection, canonical validation,
and atomic publication. Transcript production remains behind the existing digest-pinned
`BuildTranscripts` engine seam rather than being duplicated. Its versioned policy file is private
and uses opaque rule identifiers; report artifacts retain only the policy digest, rule identifier,
match class, and time range, never the raw
restricted phrase or transcript text. Exact policy variants may quarantine; deliberately ambiguous
variants hold coverage. A source-level quarantine propagates to every derivative through the same
construction authority as the visual projection, with no majority-vote clear. Certification uses a
separate source-disjoint positive/clean challenge and counts source families rather than derivatives;
it requires zero positive misses and a one-sided 95% exact Clopper-Pearson lower bound of at least
95% for source recall (59 independent positive sources with zero misses). Until that challenge
passes, the report keeps training, ingestion, scheduling, and production admission false even when
the measured sources contain no match.

Spoken-safety certification consumes that exact private projection plus a separate locked private
challenge authority authored no later than the projection under test. The authority binds the
corpus and policy rather than a post-run report, and uses opaque aliases and family identifiers to
label source-disjoint positive intervals and clean locale/slice controls; it contains no model output
and cannot be derived from the transcript under test. Every positive interval must overlap a
prohibited match from the projected source, and any missing/ambiguous transcript remains an
operational hold.
Recall is counted by independent source family, requires zero misses, and reports the one-sided 95%
exact Clopper-Pearson lower bound. Clean false-positive rates are reproduced independently for each
locked locale/slice and may not exceed 1%. The canonical certification report retains only opaque
case identities, counts, rates, authority digests, and outcomes. It never reproduces source identity,
transcript text, or policy phrases and grants no production permission even when the diagnostic
challenge passes. Generated/TTS controls are permanently marked `development` and can produce only
a diagnostic result; they cannot satisfy or be relabeled as the independent-source certification.

The maintained transcript projection above remains deterministic diagnostic history: it loads and
projects already-produced evidence and owns no network, process, retry, budget, or production-ingest
behavior. Measured development controls supersede the assumption that it can become the production
scanner by adding a decoder. Whisper-family transcripts missed prohibited positives; a stronger local
acoustic proposer retained the known positive but generated an impractical candidate queue. A hosted
native-audio adjudicator reduced that queue without earning negative authority, and a distinct
complete-video/audio route corroborated the reduced set. Those observations establish the next
production boundary, not a clean-source label or an admission certificate.

The initial spoken-safety stage delivers a private cascade core and an ephemeral in-memory attempt
record. Its evaluator and adapters remain unexported and unwired; this stage does not expose the
external evaluation operation or claim an immutable durable ledger. The next stacked integration must
deliver that operation and ledger before compatibility scoring or ingest wiring can consume the core.

The completed production **spoken-safety evaluator** is one deep Go module with one external evaluation operation
over an immutable source-authority document. It owns complete-source validation, bounded media planning,
the ordered cascade, deterministic reduction, and canonical result validation. Local acoustic proposal,
native-audio adjudication, complete-video/audio corroboration, and media extraction are private adapters;
their provider, process, and tool details do not leak into callers. The module emits claim-specific
evidence plus an immutable per-step ledger. It does not return or imply a filler-admission verdict, and
`filleradmission.Evaluator` remains the only terminal semantic authority.

The operation request supplies a stable run id and start time, the certification-authority digest, and the
source authority plus machine-local path. The source authority identifies only the immutable source,
measurement, policy, implementation, and tool facts that exist before a certification challenge is locked;
it does not contain the certification-authority digest. Keeping those identities separate is mandatory:
placing the certification digest in the source authority while the certification authority stores the source-
authority digest creates an unconstructable hash cycle. The request joins the two independently immutable
documents, and the durable run header binds both. The path remains excluded from every returned or durable
value. A first invocation atomically creates
the immutable run header before appending source-plan and proposal events; a repeated invocation of the same
completed run returns its already-canonical terminal evidence without repeating local or hosted work. An
existing incomplete run is never resumed in place or silently reissued: recovery closes it conservatively and
a caller starts a new bounded run id. The operation returns only the path-free run identity, closed evidence,
reducer result, and terminal event id/digest needed to reproduce certification input. Audio assessments retain
only sorted opaque matched-rule ids beside their closed state; they never retain a phrase, transcript, or quote.
It exposes neither adapters nor a provider response and cannot be used as an admission decision.

The cascade validates exact source bytes, measured duration, transformations, tool identities, and
complete modality coverage before inference. A certified local candidate proposer emits source-relative
candidate intervals. Each candidate is adjudicated by a pinned native-audio route. Only when every
candidate receives a valid absent result may a distinct pinned route inspect the complete source video
and audio. Calls are serial by default and retain the exact requested and canonical model, upstream
route, snapshot, modalities, media bounds, response, cost, reservation, settlement, and failure identity
required by the OpenRouter certification contract above.

Concrete runtime construction consumes the exact private certification-authority bytes, a reproduced
passing certification report, the private restricted-language policy, and the public spoken projection
authority as one joined deployment input. Callers do not select independent model, prompt, schema,
proposer, or capability strings: the factory reproduces their identities from those authorities and
refuses any disagreement before opening media, reserving spend, or contacting a provider. It refreshes
the bounded OpenRouter route snapshot before a new run, requires the same canonical model and ZDR,
fallback-disabled upstream capabilities used by certification, and refuses a current price ceiling above
the deployment reservation. Credentials, the configured ffmpeg location, storage adapters, and current
time remain runtime-only inputs and do not enter the portable authority.

The concrete factory does not by itself activate production execution. No spoken-safety boot setting or
default deployment exists until a real category corpus produces a passing report and an operator seals its
exact execution envelope. The activation change must declare those artifact paths in the settings registry,
load them once at boot with private-file checks, and fail closed to the existing qualification hold when any
artifact is absent or stale. Shipping a constructor against synthetic fixtures is never permission to spend,
project a complete axis, or admit a rendered child.

The rendered-child producer builds one source authority from the already content-verified evidence
derivative. It remeasures complete audio/video presence and duration with the ffprobe executable adjacent
to the configured ffmpeg, identifies both executable byte digests and banners, and requires the derivative
manifest's ffmpeg identity to match before the cascade can run. The first durable run time is also the
source measurement time. If screening crashes after the cascade settles but before the axis projection is
written, retry reuses that durable first-run time, reopens and remeasures the current exact bytes, and
replays the existing terminal run without another hosted request. A changed source, toolchain, policy,
certification, route, or operation identity conflicts or holds; a fresh timestamp may never turn the same
screening operation into a second billable run.

Reduction is deliberately asymmetric. One valid prohibited-presence observation quarantines the source;
a negative observation never votes it away. An unclear or disagreeing result, incomplete modality,
provider or local-runtime failure, stale identity, exceeded budget, or invalid schema holds
operationally. A presence response with malformed timing is sufficient to hold but cannot be projected
as a bounded fact. Two valid negative model observations produce only `candidate_rejected`: they never
mean clean, suitable, ingestible, schedulable, or admitted. A proposer that emits no candidates likewise
cannot establish clean coverage before the complete cascade is independently certified.

The reusable OpenRouter multimedia transport is a separate concrete infrastructure module rather than
part of the identity-blind review package or the generic text/tool LLM client. It hides capability-
snapshot validation, canonical model and upstream binding, zero-data-retention and fallback-disabled
routing, strict structured output, media ceilings, durable reservations, exact settlement, response
metadata, and raw-response binding. Review, certification, and production safety modules may consume
that behavior through private interfaces; none may weaken its route or accounting contract.

Before reservation or HTTP, the transport requires validated route authority derived from the exact
capability-snapshot bytes and expected digest. Validation binds freshness, requested and canonical
model identities, upstream/provider route, required input modalities, structured-output support, and
zero-data-retention eligibility. Unchecked strings or a syntactically valid digest cannot construct
that authority; a missing or zero authority fails before any reservation or request.

Subsequent production integration must be shadow-only. Its durable ledger is written before the compatibility score may
run; failure to persist leaves the ingest parked and recoverable. The result grants no catalog filing,
training, scheduling, or production-admission permission. An applied projection from certified
spoken-safety evidence into the admission document is a later design and rollout change. Its identity
must match the exact certification artifact; model, route, prompt/schema, media planner, policy, local
runtime, weight, or implementation drift returns a hold until recertified.

The spoken-safety ledger is not an admission-decision record. `filler_admission_decisions` stores the
terminal policy projection described above; a spoken-safety run instead records how one evidence attempt
executed, including work that crashed or never reached a semantic result. Persistence therefore owns an
immutable run header and append-only, ordinal events. The header binds the clip, complete-source authority,
source bytes, certification, policy, implementation, and start time. Events use a closed kind and payload
schema for source planning, local proposal, hosted-call reservation, hosted-call settlement, and the
terminal reduced result. Stable run/event identities and exact-payload conflict checks make a repeated
write idempotent without permitting history to be replaced.

A hosted-call reservation event and its V62 inference-budget reservation commit atomically before the
HTTP request starts. Settlement is a later append-only event bound to that reservation; it records the
request and response digests, requested and resolved model/provider identities, upstream route, modality
coverage, generation id, closed outcome or failure code, exact charged decimal/nanodollars, and reservation
disposition. A terminal event contains the canonical closed evidence and reducer result and references every
attempt event in order. The compatibility score may advance only after that terminal event commits. An
interrupted run remains visibly incomplete: startup/retry appends an operational terminal hold, marks any
unsettled reservation failed with unknown settlement, and starts a new bounded attempt rather than mutating
or silently replaying the old history. Old reservations continue to count against budget, so repeated
crashes cannot create unbounded spend.

`fillersafety` owns the narrow persistence port for those lifecycle facts; the SQL store is an adapter that
maps its closed reservation and settlement commands onto the generic V62 inference row and the spoken-safety
event in one transaction. The evaluator supplies deterministic per-run event/evaluation ids, exact media and
version identities, and the closed outcome or failure class. The store supplies only budget disposition and
persisted accounting facts. A budget-held reservation is itself durable and prevents the HTTP request; it
needs no settlement event. Once an accepted reservation exists, inability to persist its settlement or the
terminal event returns an operational error and leaves the run incomplete for startup recovery rather than
returning unrecorded semantic evidence.

The ledger stores only bounded public identities, closed states, interval coordinates, digests, accounting,
and opaque rule ids. Machine-local paths, source names, restricted variants, transcripts, quotes, private
policy JSON, prompts, media bytes, raw request bodies, raw provider responses, and free-form provider errors
are forbidden. Request/response SHA-256 values plus the provider generation id bind those private artifacts
without copying them into an ordinary application database or operator projection. No ledger read model is
ordinary-user-facing in this slice.

Ledger-owned run, event, evaluation, candidate, reservation, generation, clip, and implementation
identifiers are bounded opaque ASCII tokens using letters, digits, underscores, and hyphens. They
reject filesystem separators, dotted filename forms, URLs, whitespace, and control characters. A clip identity
remains the existing opaque clip key; this boundary does not invent a different digest format for it.
Public model/provider identities use a separate bounded representation that preserves legitimate route
slashes, revision dots, and provider display-name spaces. These values must come from the validated
route and response identities recorded by the atomic V62 helpers, never from source metadata or free-form
provider output. Their syntax is not evidence of route authority: the completed runtime integration must
retain the validated capability and response binding before writing those fields. The generic event
append operation cannot create reservation or settlement authority.

Restricted spoken language and broad visual suitability remain separate claims. Complete-video
suitability must ultimately cover every source, including a source for which the candidate proposer emits
no interval; a video corroboration performed only on spoken candidates cannot satisfy that obligation.
The admission evaluator combines independently certified claim evidence and never treats one no-signal
lane as authority for another.

The measured sherpa keyword proposer is not a production dependency until its runtime and exact model-weight
artifacts have explicit redistribution and use authority recorded in §14. The selected 2024 GigaSpeech archive's model-card
metadata says Apache License 2.0, but the archive contains no `LICENSE` or `NOTICE`, and upstream issue #3802
explicitly asks which terms apply to this exact model and remains unanswered. That is enough evidence for the
maintainer's private development measurement, not enough authority for Loomarr to bundle, fetch, recommend,
or imply commercial use of the weights. The engine pin and companion-file inventory are now known; the weight
authority and both-platform certification remain separate gates. A lower-recall decoder is not silently
substituted.

The legally unblocked baseline proposer is instead deterministic and weight-free. It partitions the verified
complete soundtrack into contiguous, non-overlapping 28-second source intervals, with the final interval ending
exactly at the source duration. The existing native-audio extractor supplies one second of clamped context on
each side, so every request remains at most 30 seconds of 16 kHz mono WAV and below the existing 2 MiB ceiling.
The union of candidate intervals is exactly `[0, duration)`; a boundary cannot become an uncovered negative.
The 4,096-candidate ceiling rejects rather than truncates a source beyond 31 hours. This strategy reads no
source bytes, starts no process, carries no model/runtime identity, and makes no safety classification: it
trades more serial hosted calls for auditable complete audio coverage. Its proposer identity explicitly names
the deterministic strategy and hashes its window configuration; the model-backed sherpa identity remains a
separate strategy with exact platform, runtime, and model hashes. Certification, route budgets, and private
source-family truth decide whether the weight-free baseline is good enough before any shadow wiring.

The development adapter consumes one private, mode-`0600` **acoustic keyword authority**, not raw policy
phrases on its interface. That versioned JSON binds the canonical policy digest, exact model manifest digest,
and an ordered set of opaque rule ids to their pre-tokenized BPE variants. The adapter verifies every token
against the pinned model vocabulary and writes a private ephemeral sherpa keyword file whose `@` label is only
the opaque rule id. It never derives tokens inside the server, places a phrase or token sequence in argv or an
environment variable, or accepts sherpa's keyword text as a result identity. The authority is prepared
offline with the same pinned `bpe.model`; adding SentencePiece or another tokenizer to the server is a separate
§14 decision, not hidden inside this adapter.

The adapter stages the exact runtime executable, ONNX Runtime library, model members, vocabulary, and private
keyword authority into one private workspace before use. A canonical manifest digest, rather than an archive
name, identifies the bytes that can affect inference. It extracts the complete verified source to 16 kHz mono
WAV, invokes the worker through the process-tree supervisor with bounded time and output, captures both streams
in memory, and discards stderr without logging it because sherpa prints source paths there. Stdout is strict
one-JSON-object-per-hit: unknown fields, an unknown/non-opaque keyword label, non-finite or unordered timing,
token/timestamp cardinality drift, output beyond the ceiling, or a non-zero/partial run fails the complete
proposal. Candidate intervals run from the first token timestamp through one 40 ms subsampled frame after the
last and are clamped only at the verified source endpoint. Native-audio adjudication extracts each of those
evidence intervals with the calibrated one second of context on both sides, clamped to the complete source;
the candidate identity and ledger interval remain the unexpanded acoustic evidence rather than pretending the
context was detected speech. These implementation details stay behind the private candidate-proposer seam;
the external evaluator interface remains one evaluation operation.

Production certification uses a locked, source-family-disjoint challenge of real speech, not generated
or transformed copies pretending to be independent observations. Positive coverage includes distinct
speakers and source families across the predeclared accents/locales, music and overlap, noise, speed and
pitch, codec, clipping, and placement slices. With zero misses it still requires at least 59 independent
positive source families for the one-sided 95% exact Clopper-Pearson recall lower bound to reach 95%.
Clean and near-match controls report their observed false-positive rate for every locked locale/slice;
any stronger confidence-bound target declares its sample population before the challenge is opened.
Known-script, consented real-speaker recordings may supply positive truth when licensed pre-labeled media
is unavailable, but two independent blind reviewer identities still verify audibility and timing and a
third adjudicates disagreement. A model-backed reviewer requires the immutable attestation and candidate-
family exclusion already specified above. The maintainer is not a required blind reviewer.

Cascade certification is a separate deterministic module over two private immutable documents. The authority is
authored before evaluation and binds the policy, evaluator/proposer and hosted-route identities, truth
provenance and rights/consent digests, opaque case and source-family ids, locale/slice coverage, positive rule
intervals, and two agreeing reviewer attestations (or a third adjudicator). A model-backed reviewer declares
its model family, which must be absent from every proposer/adjudicator/corroborator family in the same
authority. Each case binds the digest of its certification-independent source authority. Evaluation supplies
the completed certification-authority digest alongside that source authority and binds both separately in the
run header; neither digest is defined in terms of the other. The label-blind result manifest contains every authority alias exactly once with the complete
path-free run header and ordered ledger events; it contains no truth label. Every run must start after the
authority was authored, bind that exact authority digest as its certification identity, and end in the named
terminal event/digest. Missing, extra, duplicate, incomplete, identity-drifted, or non-canonical runs make the
whole score operationally invalid rather than silently reducing its denominator.

The scorer counts a positive source only when a valid audio detection carrying the expected opaque rule id
overlaps every declared positive interval. Another prohibited rule, a video-only flag, an unprojectable
presence, a hold, or a candidate interval without rule attribution is not a hit. Recall is by the authority's
unique source families. Clean false positives are any audio prohibited detection and are reported for both
locale and declared clean slices. A development authority can produce only `diagnostic_passed`; a
certification authority still requires at least 59 positive families, zero misses, a one-sided exact 95%
source-recall lower bound of at least 95%, at least 100 independent clean families so the declared 1%
observed false-positive ceiling has one-source granularity, zero coverage holds, and no declared clean slice
above that ceiling. This is explicitly an observed-rate gate, not a 95% upper confidence claim; the latter
would require at least 299 clean families with zero false positives. Every output permission remains false
regardless of status.

Authority locking is a separate offline operation over a private path-bearing draft, two independent complete
review bundles, an optional disagreement-only adjudication bundle, a private alias seed, and the exact source
and evidence bytes. It verifies source-authority and media identity, policy and implementation identity,
review order and draft binding, draft-pinned rights and truth-provenance digests against their current bytes,
family uniqueness, and all corpus minima
before emitting a path-free authority. Opaque case, family, and reviewer ids are keyed derivations from the
private seed. Reviewers are blind to evaluation output and one another; known-script reviewers may see the
claim they must verify. Model-backed primary/adjudicating reviewers are permitted only when their model
families differ from one another and from every evaluated proposer/adjudicator/corroborator family. This lock
does not download a dataset, create consent, transform media, run inference, or begin certification.

Matching a rights digest is necessary but is not a current rights decision, and a currently valid document is
not proof that it governs the source beside it. Before planning or decoding any source, the authority locker
asks the corpus owner to validate every case's exact rights bytes together with its exact truth-provenance bytes
and source authority at the fixed authority-authored time. Shared rights documents may be decoded and cached
once, but case binding is never deduplicated. The production command hard-wires that validator; callers cannot
omit it. The closed initial vocabulary is the complete VCTK release-authority v1 envelope and the canonical
known-script rights v1 envelope. VCTK validation rechecks the release identity, rights-review chronology,
complete member authority, hosted-evaluation contract and current term, then requires the provenance's named
member and original evidence authorities to occur in that release and its wrapped output identity to equal the
case source authority. Known-script validation rechecks the participant binding, every grant,
expiry/withdrawal state, time-sensitive music/noise rights, authority and transformation binding, then requires
the packaged output identity to equal the case source authority. Unknown, malformed, unsupported,
noncanonical, expired, withdrawn, unbound, or mismatched evidence invalidates the complete lock before media
work or publication. The exact hosted route was
already authorized immediately before each model request; the later lock establishes that the participant and
asset rights remain current, not that an expired grant retroactively authorized a call. A path-free immutable
authority is not a live withdrawal registry: any future production admission must recheck then-current rights
through a separately designed live-use boundary, and all output permissions remain false meanwhile.

A review verdict is `verified` or `rejected`; it is not another copy of the draft's proposed `positive` or
`clean` label. A verified positive retains the draft's exact proposed intervals, while every rejected verdict
and every clean verification carries no interval. This distinction is required so a reviewer can reject a
supposed clean control after hearing prohibited speech without first manufacturing positive timing truth, and
can reject a supposed positive whose intended phrase is inaudible. Two agreeing rejections cannot lock the
draft. A primary disagreement requires the existing independent adjudicator, whose `verified` verdict is the
only outcome that can establish the draft's proposed truth. A rejected adjudication also prevents publication.

Public clean-speech preparation is upstream and non-authorizing. The first pinned adapter accepts an already-
acquired VCTK 0.92 tree, an exact release/member manifest, a completed certification-rights contract, and a
private seed. It does not crawl or download the release. It verifies the archive/release, licence, README,
rights-review, transcript, speaker, microphone, and audio identities; p315 is excluded because the release does
not supply its transcript. From owner-screened eligible utterances it deterministically selects exactly one
utterance from each of 100 distinct speakers. Alternate microphones, takes, encodes, and later transformations
retain that speaker family and cannot increase the denominator.

Because the evaluated cascade requires complete audio and video, the adapter wraps each selected real utterance
in a deterministic neutral-video MP4 using exact ffmpeg/ffprobe executable identities and a fixed recipe. The
speech remains real but its encoding is a declared derivative; input/output hashes, decoded duration, recipe,
tool versions, and source-relative complete span are retained. Both output streams must also survive a complete
bounded decode; a header-valid derivative with a corrupt tail is refused. The adapter emits a private review-ready
cohort and owner map, not a certification authority or clean label. A later full-draft assembler combines it with the
positive and other clean-slice cohorts before any review bundle binds that exact draft. Reviewing the VCTK
cohort before assembly would bind the wrong digest and is forbidden. Raw media, transcripts, speaker ids,
paths, seed, and maps stay outside Git. Missing rights, release drift, an unsafe path, duplicate family/content,
tool drift, an incomplete output, or fewer than 100 eligible speakers fails before atomic publication. This
preparation performs no provider call, policy classification, certification run, or spend.

Consented known-script positive preparation is a second upstream, non-authorizing adapter over recordings
that already exist. Its one external interface accepts a private owner-authored cohort authority, a private
source root and alias seed, exact ffmpeg/ffprobe executables, a fixed preparation time, exact case and resource
ceilings, and a new output directory. It never discovers participants, solicits consent, records or synthesizes
speech, authors a restricted script, downloads media, or sends content to a provider. A file's presence is not
consent: every real participant has a separate bound consent document, signer-authority evidence, processor
schedule, withdrawal instructions, and owner-reviewed consent contract. That contract explicitly covers
collection, private storage, deterministic modification, evidence extraction, independent review, hosted model
evaluation, the participant's redistribution choice, retention and withdrawal, and no endorsement. An expired,
withdrawn, incomplete, ambiguous, or mismatched contract fails before any media tool runs.

The owner authority binds exactly one selected take or derivative for each private participant id and source
family. It also binds recording session/take identity, locale/accent, exact versioned script bytes, private
policy digest and policy-mapping evidence, dry master and selected audio bytes, source-relative intended
positive intervals, and a deterministic transformation record. That record names its recipe and digest,
rendering tool identity and time, master and output authorities, and every music/noise/mix asset with separately
reviewed rights covering the same transformations and hosted processors. Retakes, cuts, re-encodes, mixes,
clipping, placement derivatives, and alternate representations of one participant remain one source family and
cannot inflate the denominator. The selected cases must contain at least 59 unique participants, use only the
locked positive-slice vocabulary, and cover every required positive slice; a music-overlap declaration requires
a rights-cleared music asset.

Preparation reopens and hashes every authority-bound byte, derives opaque case/family ids from the private seed,
copies the selected script only as the private transcript, and wraps the selected real audio in the existing
deterministic neutral audiovisual recipe. Exact tool identity is checked before and after processing, both final
streams must fully decode, and every proposed interval must remain ordered, non-overlapping, rule-valid, and
bounded by the measured final source. Atomic private output contains one positive-candidate cohort, an owner map,
and per-case source, transcript, provenance, and rights documents. Restricted script text, participant identity,
input paths, and seed never enter public output, Git, logs, or errors. The script and preparation establish only
an intended claim: two independent reviews over the fully assembled draft, plus disagreement-only adjudication,
still establish certification truth.

Prepared spoken-safety cohorts use one private candidate contract. Every case binds its complete audiovisual
source, optional exact transcript, source family, rights and truth-provenance evidence, proposed `clean` or
`positive` claim, locale, slices, and any proposed positive intervals. A preparation adapter may propose those
facts from source-owned evidence, but the proposal is not certification truth. Clean candidates carry no positive
intervals; positive candidates carry sorted, non-overlapping, bounded intervals with the exact restricted-rule id.
The cohort document binds transcript bytes separately even when provenance also names the original transcript,
so a model or human reviewer cannot unknowingly assess a changed convenience copy.

One deep offline assembly module owns the transition from separately prepared cohorts to the single review seam.
Its interface is one operation over a private assembly plan, one private input root, and one new output directory.
The plan binds the exact private policy bytes; draft envelope and route identities; every cohort document/root,
kind, dataset, digest, and exact case count; the expected combined case count; and aggregate input, output, and
wall-time ceilings. The module revalidates current source, transcript, rights, provenance, policy, and cohort bytes;
requires one policy and implementation; rejects cross-cohort case, source-content, or source-family collisions;
and enforces the certification minima and complete positive/clean slice vocabulary before publication.

Assembly snapshots only referenced verified bytes into one self-contained `0700` tree. It writes private `0600`
case media, transcripts, provenance, and rights evidence; the canonical #929 path-bearing `draft.json`; the exact
policy; and two byte-identical primary-review worklists bound to the draft digest, policy, evidence, and case order.
Each worklist may
show the proposed claim and intervals needed to verify known-script evidence but contains no evaluation result,
other review, reviewer identity, or completed decision. Outputs are deterministic for the same plan and inputs,
created atomically without overwrite, and remain non-authorizing. Reviewers submit separate #929 review documents;
two complete independent agreements, or disagreement-only independent adjudication, are still required before the
authority locker can establish truth. Assembly runs no model, reviewer, certifier, downloader, or production ingest.

Independent routine review is one separate deep module, not a human playback loop hidden in the assembler.
Its interface is one operation over a private review plan, the assembled private root, an API credential, the
exact local ffmpeg executable, a private checkpoint directory, and a new output path. The plan binds the exact
draft, worklist, private policy, fresh OpenRouter capability/price/ZDR snapshot, reviewer and model-family
identities, one exact requested/resolved model and upstream endpoint, expected case count, and request, charge,
spend, input-byte, audio-byte, per-case-time, and wall-time ceilings. The reviewer model family must differ from
the draft's proposer, native-audio adjudicator, and complete-video corroborator. A second primary review uses a
separate plan, reviewer identity, checkpoint, and model family; the operation never sees the sibling review.

For each case the module first reopens and hashes the source, draft, worklist, policy, and evidence bindings,
then asks the rights owner to authorize the exact hosted processor before any media tool, checkpoint creation,
HTTP request, or spend reservation. A rights document bearing the known-script rights-envelope contract is strictly
decoded inside the corpus module; its participant binding, grants, expiry/withdrawal state, processor schedule,
and time-sensitive asset rights are revalidated at review time. Authorization requires an exact match on the
OpenRouter HTTPS base URL, requested and resolved model, upstream provider name and slug, and ZDR. A malformed
recognized envelope or unmatched route fails closed without logging consent contents, participant identity, or
private paths. Other rights contracts retain their existing validators and are not reinterpreted as participant
consent. After preflight, the reviewer extracts the complete soundtrack from the verified source snapshot into
bounded 16 kHz mono WAV. Calls
are serial, fallback-disabled, ZDR-only, and strict-schema. The prompt exposes the private policy, proposed
claim, opaque rule ids, and proposed intervals but no evaluation output or other review. The model may return
only `verified | rejected | unclear`, `clear | degraded | no_speech`, sorted opaque matched-rule ids, and the
indexes of proposed intervals it heard; it is instructed never to transcribe or quote speech. A positive is
verified only when every proposed interval is confirmed with its expected rule. A clean candidate is verified
only with no matched rule. A contrary decisive observation becomes `rejected`; unclear or degraded evidence is
an operational stop and cannot become a review verdict.

The maximum per-call charge is durably reserved before HTTP and exact usage is settled afterward. The private
checkpoint binds all input, prompt, schema, route, tool, and budget identities; accepted cases form a canonical
prefix and are not called again. A process interruption after reservation remains an unsettled hold rather than
an automatic replay. Source or identity drift, ambiguous output, provider failure, a stale route snapshot, or
exhausted resource limits leaves no review bundle. Only an exhaustive decisive pass publishes the canonical
model-backed authority-review document at mode `0600`; it performs no authority locking, certification scoring,
training, ingestion, scheduling, or production admission.

The model review embeds a bounded, path-free evidence record rather than asking the authority locker to trust a
free-form model-family string. That record binds the plan, worklist, policy and snapshot digests; exact requested
and resolved model and upstream endpoint; model family; prompt and schema digests; ffmpeg identity; fixed resource
ceilings; aggregate usage and settled charge; and every attempt's case id, request/response digest, generation id,
state, observation digest, token counts, and settled charge. It contains neither the response body nor source
content. The review envelope carries the canonical evidence digest, and every final reviewer attestation retains
that digest. Human reviews instead bind a separately authored evidence digest and cannot claim model evidence.

No model is trained or fine-tuned for this lane until governed source-disjoint labels exist and the
certified stock cascade demonstrably misses a locked gate. Existing unknown commercials and agreement
between candidate models remain development observations and cannot be promoted into training truth.

The temporal `unusable` answer is diagnostic history, not media-integrity truth. Media integrity,
presentation/source defects, broadcast suitability, semantic unit/role, and rights are five
independent claims with independent policy owners. The maintained media-integrity challenge consumes
the exact full-decode report produced through `filler.EvaluateMediaQuality`; it never copies or tunes
the production black, silence, or freeze thresholds. Its closed integrity slices are `no_video`,
`no_audio`, `decode_failure`, `near_total_black`, `near_total_silence`, `stuck_or_low_motion`, and
`clean_assessable`. Its separate presentation vocabulary is `screen_recapture`, `player_or_browser_chrome`,
`timecode_or_recording_overlay`, and `third_party_stock_watermark`. A presentation observation cannot
become an integrity failure merely because both occur in one clip.

Challenge preparation takes a private authority manifest, the exact schema-v2 full-decode report,
and a fresh secret seed. It emits a label-free public package containing only fresh opaque aliases
and content/measurement digests plus an owner-only map binding those aliases to source identities,
closed labels, expected `reject | review | continue | hold` outcomes, presentation observations, and
an explicit low-motion disposition. Public bytes are scanned for every private case id, evidence
alias, label, and seed before atomic publication. Missing or extra cases, duplicated aliases,
invalid media or measurement digests, changed tool/policy/report identity, unknown vocabulary, and
any label-bearing public field fail closed.

The locked scorer rejoins the package, private map, and exact full-decode report by digest and scores
each expected outcome against the production-policy result. A missing measurement or operational
failure observes `hold`, never `continue`. Each integrity slice reports cases, exact confusion counts,
accuracy, and the one-sided 95% Wilson lower bound. Presentation observations are counted separately
and never alter integrity correctness. The known noisy/static case may carry
`measured_gap` only when the current detector did not hold it; that gap remains visible and cannot be
converted into a passing detection or a threshold change. Every output remains private,
content-addressed, and `productionAdmissionAllowed: false` until a representative locked challenge
and the independent certification holdout pass their own predeclared gates.

When that stop fires, a stronger hosted model may inspect only the comparison's deterministic,
stratified calibration selection before another development-scale run. The selection is a separate
content-addressed artifact bound to the package, both local assessment hashes, and the comparison
hash; it contains only opaque aliases, reasons, and strata. It cannot silently expand to all disputed
cases or the 300-case corpus. The maintained calibration is serial and makes at most one unit call and,
only for `standalone`, one role call per selected case, so a 15-case selection has a hard 30-request
ceiling. It uses the same less-than-24-hour OpenRouter capability snapshot, exact requested/canonical
model and upstream route, ZDR, fallback-disabled strict-output contract as hosted review. Every call
reserves its declared maximum nano-USD charge in a private checkpoint before HTTP and settles exact
provider cost when returned. A normally returned failure with missing settlement keeps the distinct
reservation consumed; a crash-stale `reserved` call blocks automatic resume. Neither state is retried
or converted to a semantic label. The public result binds the ordinary provider-neutral assessment set
to the selection, capability snapshot, prompt, route, request ledger, known charge, and still-consumed
unknown-charge reservations. This diagnostic remains non-certifying and cannot authorize unattended
admission or the 300-case relabel by itself.

The shared `openroutermedia` capability snapshot is the sole owner of model, endpoint, and pricing
validation for runtime and certification. Schema 3 adds at most four canonical prompt-token pricing
tiers per endpoint. Thresholds are positive, strictly increasing integers; each tier contains only
its changed price fields and inherits omitted fields from the validated base pricing. Previously
captured schema-2 snapshots remain valid only without tiers; a schema-2 artifact carrying tiers is
invalid. The adapter preserves the recorded schema and never invents a tier, upgrades old evidence,
or bypasses the shared validator. Exact route, privacy, freshness, capability, and positive
reservation checks remain mandatory, and known over-reservation charges remain accountable holds.

A hosted review run additionally binds one exact upstream route from a capability snapshot no more
than 24 hours old. It requires image and text input plus strict structured output, zero-data-retention
eligibility, one provider attempt with fallback disabled, and response metadata proving the selected
route and catalog canonical model revision. Before every serial request it atomically records a
private `0700` checkpoint reservation, including the package-manifest, transcript-set, capability-
snapshot, prompt, requested and resolved model, upstream provider/route, reviewer, batch, request-body
SHA-256, and the unchanged request, per-request charge, and total nano-USD ceilings. Only then may the
request leave the process. A settled attempt records its generation, tokens, latency, exact charge,
state, and, when accepted, the hash of its normalized per-case submission; accepted submissions and
their matching calls are stored in the same `0600` checkpoint. An interrupted request whose exact
charge cannot be settled remains reserved and blocks automatic recovery rather than being treated as
free.

Exactly one process owns a hosted-review checkpoint at a time. Before loading checkpoint state, the
runner creates `<output>.private/active-run.lock` with `O_EXCL`, mode `0600`, and a record binding the
checkpoint-identity SHA-256, start time, and diagnostic process ID. It holds that lock across every
load, reservation, persistence write, HTTP request, validation, and return. A competing process fails
before HTTP. Every normal return verifies that the lock bytes still match the owner's digest, removes
the lock, and syncs the private directory; a failed release suppresses otherwise completed output.
A crash deliberately leaves the lock behind. Neither PID existence nor lock age may infer staleness.
After independently establishing that no owner remains, an operator must run the hosted-review CLI
with the same `--out` and `--recover-lock-sha256 <exact digest reported by the blocked runner>`; this
mode makes no provider request, atomically renames the lock to a digest-named recovery audit, and
exits. A missing or changed digest fails closed.

The same CLI has a strictly offline inspection mode only for Reviewer B's exact 300-case hosted-review
checkpoint. It accepts the exact package, transcript set, historical capability snapshot, route
identity, prompt identity, and original ceilings; re-hashes and validates all of them plus the exact
`0700` checkpoint directory, exact regular `0600` checkpoint, complete settled attempt accounting,
package order, and any bounded exact regular `0600` active lock; and rejects symlinks, other types, and
all other modes, including setuid, setgid, and sticky bits. The package and checkpoint directories are
opened without following their final symlink and retained as descriptor roots; package descendants are
opened component-by-component relative to that root without following symlinks. The package tree is an
exact closed set of `manifest.json`, its declared instructions and label template, every declared
signal, and only their required ancestor directories. The checkpoint tree contains exactly
`checkpoint.json` and, when present, `active-run.lock`. Any other file, directory, symlink, device, or
special object fails inspection regardless of its mode. The checkpoint,
optional lock, transcript set, and snapshot are likewise mode-checked and read from the same opened
descriptors, so a pathname replacement cannot redirect validation to one object and reading to
another. Historical inspection validates the snapshot's immutable schema, source, model, route,
capability, privacy, and digest identity but does not apply the live run's 24-hour freshness window.
The live runner still rejects a snapshot older than 24 hours before constructing or invoking its
request path. The offline CLI control path has neither credential lookup nor the provider-capable run
function available to its inspection branch. It never reads a credential, creates a lock, mutates any
input directory, or constructs a provider client.

Its content-addressed attestation is a sanitized inspection projection only: permitted artifact,
prompt, checkpoint, identity, and optional active-lock hashes; closed inspection status; aggregate
case and historical request counts; and historically recorded immutable request/monetary ceilings and
remaining allowance. Its SHA-256 is independently recomputable from the canonical emitted fields with
the digest field omitted. It emits no raw batch, reviewer, model, provider, route, or prompt-version value.
An incomplete checkpoint without a lock is `awaiting_explicit_maintainer_approval`. A present valid
lock is only `active_run_lock_present`: it proves neither stale ownership nor recovery authority.
Presence is tracked separately from lock bytes, so an empty `0600` lock is invalid rather than absent.
Every state reports provider execution unauthorized; inspection authorizes no provider call, recovery
run, or spend reservation. Missing inputs, identity or ceiling drift, unsafe modes, duplicate or
out-of-order state, an invalid lock, an unsettled reservation, or unverifiable accounting emit no
attestation and fail closed.

A resumed hosted review re-hashes every package, transcript, snapshot, prompt, request, and accepted
submission and requires the checkpoint identity and ceilings to match exactly. Identity drift,
unknown fields, permissive or symlinked state, duplicate aliases, duplicate accepted attempts,
unbound calls, and altered hashes fail before HTTP. Previously accepted aliases are never requested
again. Only the failed alias is retried, as a new separately reserved attempt, and every prior failed
or accepted request and exact charge continues to consume the original ceilings. The maintained
300-case route defaults to a hard 301-request ceiling so one failure can be retried; a second failure
exhausts that fixed recovery allowance, and configuration rejects a ceiling above 301. A missing or
mismatched route, usage charge, schema, or case result still aborts the batch.
The private checkpoint is not a label submission: public output appears only when exactly 300 unique
accepted aliases and their complete settled attempt ledger validate. Published attempts must follow
submission/package order: all failed attempts for one alias are contiguous immediately before that
alias's one final accepted attempt, and no later alias may interleave. `labels.jsonl` plus the
schema-2 `review-run.json` are then published together by one atomic directory rename. There is no
schema-1 resume adapter: runs created before private request reservation have no trustworthy accepted-
case or cumulative-charge ledger and must not be treated as resumable evidence.

The maintained review packager turns one such opaque packet into inspectable reviewer evidence; a
hash-only packet is not a completed review handoff. It consumes the exact draft, reviewer packet,
owner-only alias map, provider evidence-packet JSONL, and external derivative root. Before writing
anything it verifies every draft, alias, content, evidence-packet, and derivative hash. Its
reviewer-visible manifest retains only opaque aliases, bounded segment coordinates, sanitized decoder
facts, and alias-relative audio/frame/video paths; it omits source text, source-policy facts, case IDs,
split/cluster assignments, titles, filenames, creator, campaign, source family, provenance URLs, and
the private map. Media is materialized by an explicitly selected hard-link or copy mode, never by a
symlink, and the output is published atomically only after every materialized file re-hashes correctly.
The package includes an intentionally invalid empty label JSONL template in packet order so an
unfilled or partially filled handoff cannot be mistaken for a completed submission. Packaging does
not call a reviewer, infer labels, or make the two review batches independent; those remain separately
executed and attested steps.

Certification scores exactly one named split. Development examples cannot inflate locked-holdout
metrics, and exact or near-duplicate content cannot cross their source/similarity cluster boundary.
The replay report is deterministic for the same manifest, captured predictions, and explicit run
identity: generation time is an input, never the scorer's wall clock. The run predeclares positive
request, spend, and concurrency ceilings; captured attempts or charged cost beyond either ceiling
fail closed. Reports carry the exact manifest digest and one-sided Wilson bounds for admission,
rejection, automation, review, and slice accuracy, including an upper bound for review rate. Each
certification slice gate predeclares both a point threshold and a confidence lower bound.

An open-weight candidate is certified through the same label-blind packet and replay contract, not
through a separate favorable prompt. A local Ollama route is loopback-only, pins the exact model tag
and registry digest before inference, disables model thinking for bounded structured extraction,
uses one attempt at concurrency one, and records prompt/completion tokens plus wall latency. Local
execution has no provider charge, but still reserves request and execution ceilings; a mutable tag,
missing digest, non-loopback endpoint, malformed structured response, or exhausted output budget is
an operational failure. Hosted evidence does not establish quantization quality, resident memory,
throughput, or appliance suitability, and a successful local load does not establish accuracy.

The locked 300-case `development_seed` uses that same packet, policy, route, ceiling, and immutable
prediction contract for model selection. It may run only after both blind submissions and any needed
adjudications have been locked into every selected case. Its replay report is always non-certifying:
development evidence can select a candidate and justify a cascade, but cannot satisfy a holdout gate
or authorize unattended admission. Rejecting an unlocked or partially reviewed development manifest
is a pre-provider failure. This is not a compatibility path around certification; it is the explicit
non-scoring half of the development/holdout experiment.

The production-reference audit composes that immutable development manifest with one separate,
versioned, negative-only content-review artifact. The review binds the exact manifest digest and
keys every finding by unique content SHA-256, never by case ID, filename, collection, uploader text,
or source title. Each finding cites at least two distinct in-clip frame, transcript, OCR, audio, or
video evidence rows already bound into that exact manifest case. The audit rejects an unknown or
duplicate content identity, a missing or mismatched evidence row, an unsupported evidence kind or
closed reason, a future review time, or any artifact/input digest mismatch before screening. A valid
finding may only exclude the exact bytes; it cannot admit, relabel, add taxonomy, mutate the locked
manifest, or satisfy later human playback acceptance. The ordinary policy remains general for every
unlisted case. Positive source and segment duration are also required: zero-duration media is
unusable even when an upstream fact calls it usable. The audit owns the exact raw manifest, packet,
mapping, acquisition-ledger, and content-review bytes: it strictly decodes them, rejects duplicate
object keys at every depth plus unknown fields and trailing values, and derives their recorded
SHA-256 identities itself rather than accepting caller assertions. A usable decoder fact requires
`no_video=false`, `no_audio=false`, segment bounds wholly inside the positive source duration, and
exactly one valid hashed, positive-byte, positive-duration, positive-dimension `video/mp4` evidence
presentation. Missing streams or a missing/malformed presentation are unusable; impossible segment
bounds invalidate the audit rather than being reinterpreted as a hold.

The one retained 300-case development cohort crosses the household licensing retirement through a
single, offline, fail-closed refresh, not through a legacy reader in the audit or runtime. The refresh
owns the exact locked manifest, packet JSONL, product mapping, and negative content-review bytes from
the v3/v1 reference contract. Every legacy packet must be schema 1, bind its manifest case and content
hash, contain exactly one deterministic decoder fact and exactly one eligible source-policy fact with
the legacy evidence ID `source-license`, and otherwise match the closed packet shape. The refresh removes only that retired
licensing fact, advances the packet to schema 2 / `filler-evidence-v2`, recomputes the case evidence
digest, and advances the corpus lock. Case, content, source, provenance, semantic truth, role, slices,
taxonomy, policy flags, independent label attestations, and adjudication remain byte-equivalent under
their typed representation; a change to any of them invalidates the refresh. Because label
attestations bind the semantic label projection rather than the transport packet digest, this
declared removal does not manufacture or repeat semantic review.

The product mapping and negative review are re-bound only after the refreshed manifest bytes exist.
The negative finding must still name the same unique content SHA-256 and the same in-clip evidence
references; it cannot be added, removed, or edited during refresh. One path-free machine-readable
report records all old/new artifact hashes, the exact contract transition, 300/300 transformed cases,
300 removed licensing facts, and preserved label/adjudication denominators. Outputs are current-only,
created atomically in a new directory, and must pass the ordinary v4 audit; the application retains no
v3/v1 adapter, feature flag, or compatibility branch. The refresh performs no media mutation, network
request, provider call, spend, semantic relabel, admission, or maintainer acceptance.

The production-reference duplicate inventory is a separate deterministic derivative of one exact
production-reference audit artifact. It fingerprints every non-excluded case and no other case,
re-opens each acquired source only beneath the declared source root, and verifies the full source
bytes against the audit's content SHA-256 before decoding. The inventory records the source-audit
SHA-256, algorithm version, case ID, content identity, local-file identity, complete ordered visual
fingerprint, and complete audio envelope. Missing, extra, repeated, excluded, mutated, unbound, or
symlink-escaped sources invalidate the whole inventory; a diagnostic subset cannot be published as
the cohort inventory. Visual comparison uses a fixed two-frame-per-second centre-crop dHash sequence,
ignores near-flat frames as positive evidence, and requires sustained fixed-offset agreement across
at least 70% of the shorter useful sequence. Audio comparison uses fixed 100 ms RMS bins and requires
at least 90% normalized correlation across the same minimum coverage. Either modality may relate a
pair, but neither assigns semantic truth, editorial grade, scheduling use, admission, or a preferred
rendition. Related pairs form deterministic connected components. A transitive component that is not
a complete clique remains explicitly unresolved for full playback rather than being silently
collapsed. Once a family is recorded, every later development/holdout or inspection split treats it
as one indivisible similarity cluster: exact or near-duplicate members cannot cross a split boundary.

Gate B begins from one immutable inspection seed bound to the exact production-reference audit and
duplicate-inventory bytes. The seed names exactly 50 non-excluded, positive-duration sources within
the two-minute conditioning ceiling, records its four-frame triage method and limitations, preserves
the 32-clip proposal target and any role/source coverage shortfall, and requires full playback as its
next gate. Selection is a bounded inspection proposal, not a preferred-rendition decision: duplicate
membership is visible, a non-clique family stays unresolved, and selecting renditions for comparison
does not collapse or grade them. Archive and operator-authorized YouTube are independent first-class
partner lanes under the same provenance and inspection rules; absence or scarcity in either lane is
reported rather than making one conditional on failure of the other.

The Gate B selector owns and strictly decodes the exact raw Gate A, duplicate-inventory, and seed
bytes, derives all three SHA-256 identities itself, and recomputes the duplicate inventory from its
stored identity and relationship graph before returning any case; it does not reopen media or repeat
the quadratic fingerprint comparison at this later gate. Unknown or duplicate JSON members, trailing values,
identity/summary drift, a missing, extra, repeated, excluded, over-duration, content-mismatched, or
family-unbound case, an unreported family collision, or an empty/full-playback gate invalidates the
whole selection. The measurement command then re-opens each selected source beneath the declared
root without following an escaping symlink, verifies its full content SHA-256, and runs the bounded
conditioning contract above with one two-minute process deadline per source. Decoder/probe failures
become explicit technical holds; identity, containment, input-contract, time, or publication failures
abort the artifact instead. Output is immutable and deterministic for fixed inputs and time, records
every selected case exactly once, and reports only measured versus technical-hold counts and errors.
It neither edits media nor assigns filler identity, taxonomy, editorial grade, scheduling use,
acceptance, admission, or provider authority. At least 48 measured sources may proceed to complete
playback; otherwise Gate B records an honest technical shortfall and stops.

The one retained v3 inspection seed crosses the licensing retirement only through an offline,
fail-closed rebind after the current Gate A and duplicate-inventory bytes exist. The rebind strictly
decodes all three artifacts, requires the legacy seed identity, validates the current audit and full
duplicate graph, and changes only the contract version and the two artifact SHA-256 bindings. It then
passes the result through the ordinary Gate B selector before publication. The ordered 50 case IDs,
selection policy, triage method and limitations, explicit findings, status, and required next gate
remain byte-semantically unchanged. A path-free report records the input and output identities and
the preserved selection count. The rebind performs no media access, selection, backfill, editorial
decision, acceptance, or admission and creates no general v3 compatibility path.

Shared transcription is a separately locked provider artifact, not text pasted into a mutable packet.
For each case it binds the exact raw packet digest, audio signal identity/hash/bytes/duration, transcript
schema and prompt versions, whisper implementation identity, executable SHA-256, model filename and
SHA-256, generation time, wall latency, timed utterances, canonical joined text, and text SHA-256.
Generation revalidates the packet and WAV beneath the declared corpus root before invoking the engine,
runs serially with an explicit per-case timeout, refuses an existing output, and atomically publishes
only a complete JSONL set. A final spoken decoding window that overlaps the measured WAV tail is clipped
to that measured duration; a wholly out-of-range window, an extreme tail, or any earlier overlap remains
invalid. The engine's exact non-speech `[BLANK_AUDIO]` sentinel is discarded rather than treated as
semantic evidence. A zero-width timed segment is likewise discarded because it has no supported
location; reversed or overlapping timing remains invalid. A transcript set must revalidate all bindings before any classifier call and
contains one artifact for every selected
packet with exactly one certified WAV and no artifact for a packet without audio; ambiguous or
uncertified audio is invalid. This keeps deterministic unusable/wordless cases in the corpus without
inventing an audio binding or calling the speech engine. One set is generated once per speech-model candidate and reused
unchanged across text and vision candidates; classifiers never rerun speech-to-text privately.

The local Ollama adapter supports both text-only and ordered-frame routes through the same evidence
schema. A frame route maps the already verified JPEG bytes to Ollama's per-message `images` field in
packet order and exposes only the matching frame signal ids in the untrusted payload. It neither opens
paths itself nor changes the text-only wire shape. Local vision therefore receives the same four-frame
ceiling as hosted vision, while a model that cannot accept that unchanged route records an operational
failure rather than receiving a smaller favorable prompt.

Source inventory is a separate, non-certifying preflight. Its only live contract is strict
source-neutral schema v5: one snapshot may combine multiple captures, and every case carries the
authority, authority-qualified stable case ID, all capture IDs that discovered it, role hints, frozen metadata evidence, exact selected
representation, acquisition-time campaign and source-family identity when known, and the adapter's
explicit media-host allowlist. Capture-level request,
response-byte, predicted-media-byte, and wall-clock ceilings remain visible beside actual usage.
An adapter's configured snapshot time is the latest permitted source observation, not the time it
pretends the capture occurred. The published inventory and capture snapshot equal the latest search,
metadata, or representation-header observation actually consumed; crossing the configured ceiling
fails before publication. Live requests and cache hits are reported separately so a cache-backed
replay does not masquerade as either a fresh network capture or an unexplained zero-request result.
The combiner strictly decodes every capture artifact, sorts captures and cases by their stable IDs,
rejects duplicate capture identity, and folds a repeated case only when its frozen metadata and
representation are identical; the merged case retains the sorted union of capture IDs and discovery
role hints. Any conflicting repeated case fails closed before producing the single rights-review input.
The former schema v1 put `source` and `collection` at the document root, so it could represent only
one Archive.org collection. Schema v2 let a later split plan invent campaign and source-family
identity instead of freezing that provenance during acquisition. Schema v3 assigned each case to
only one capture, so legitimate role-specific discovery either duplicated the case or discarded part
of its provenance. No certified artifact consumes an older shape; all are rejected rather than
adapted or preserved.

An Archive capture uses the exact `archive.org/<collection>` authority that bounded its search; it
never attributes a non-Prelinger item to the Prelinger collection merely because both use the same
Archive transport. The closed host policy still permits only Archive's HTTPS media hosts for every
such collection-qualified authority. It uses one identified serial client, cached raw search/item responses, a minimum
inter-request delay, and explicit request, item, per-item byte, and total predicted-byte ceilings.
Search-level and item-level licences must agree and NC/ND candidates are excluded, but an allowlisted
uploader field remains only a candidate: independent rights adjudication is still required. Every
adapter freezes retrieval times, response hashes, selected representation identity/checksums, and
predicted bytes before any media download or model call. A partial bounded capture reports exactly
what it saw; it never widens the ceiling or treats a truncated search response as complete.

The non-scoring 300-case development set has one narrower, maintainer-authorized Archive policy. An
independent reviewer may accept the frozen exact-item assertion as operational acquisition authority
when search and item metadata agree on CC0, Public Domain Mark, the legacy Creative Commons public-
domain assertion, CC BY, or CC BY-SA and the capture has already excluded NC/ND. The locked rationale
must say that this is not a chain-of-title warranty, preserve attribution and ShareAlike obligations,
and bind the exact representation and metadata hashes. Media stays in the external corpus store.
This policy cannot qualify a scored holdout case, satisfy source-diversity gates, or authorize Loomarr
to ship the media; holdout rights continue to require the normal evidence threshold above.

Before a new reusable source adapter is built, a source-neutral **rights-yield pilot** locks exactly
ten metadata-only candidates from each qualified lane: Prelinger, Library of Congress, NASA, CDC,
and Wikimedia Commons. Blender is retired as a pilot lane because its live first-party surface could
not supply ten distinct trailer candidates without duplicate encodes, full films, or dead media
links; individually cleared Blender works may still enter the direct/static cohort
(`filler-corpus-blender`; `retired-ok`). Each lane records positive request, response-byte,
predicted-media-byte, and wall-clock ceilings plus actual usage; every candidate freezes its source
identity, role hints, metadata hash/time, source rights assertions, and one predicted media
representation; category-traversed sources also freeze their exact discovery path. The strict
decoder rejects semantic labels and rights decisions because this pilot
is discovery evidence only. A lane earns an adapter only after an independent reviewer approves at
least five product-relevant items without lowering the common rights threshold. No compatibility
format exists: no completed pilot predates this contract, so the unpopulated six-lane shape is
removed rather than decoded or migrated.

Pilot qualification uses a separate deterministic worksheet bound to the exact locked-pilot
SHA-256. It presents all fifty frozen rows and neutralizes spreadsheet formula prefixes while
leaving reviewer identity, review time, independence attestation, rights decision, product
relevance, rationale, redistribution assessment, credit, and restrictions blank. Locking requires
one named reviewer, an explicit independence attestation, complete decisions for every row, and no
immutable-cell changes. A lane qualifies only when at least five of its ten rows are both rights
approved and product relevant. The resulting yield report says `downloadAuthority: false` at both
report and decision level; it cannot be supplied to the media downloader and never substitutes for
the corpus acquisition ledger.

A new corpus-source adapter requires a product-relevance review before implementation. The review
must show useful coverage of the filler roles the certification corpus needs, such as commercials,
promos, bumpers, station IDs, trailers, and PSAs. Rights clarity, API convenience, and inventory
scale do not by themselves make a source representative.

The LOC, NASA, CDC first-party-page, and Commons commands share one promotion seam: the bounded lane
remains the ten-case qualification artifact, while an explicitly requested schema-v5 output carries
the same frozen evidence into full-corpus rights review. Full capture counts stay positive and
bounded but are not hard-coded to ten; request ceilings must cover the declared item count. The CDC
adapter still consumes authored first-party page/media pairs and therefore cannot manufacture extra
cases to meet a quota. Multiple role-specific captures combine only through the strict inventory
combiner, never by concatenating JSON or discarding their individual ceilings.

The generic first-party-page seam also recognizes one closed USGS video authority,
`usgs.gov/media/videos`. Every case binds the matching item slug to an exact
`https://www.usgs.gov/media/videos/<slug>` item and metadata URL. Its representation uses only the
exact `usgs-ocapsv2-public-output-media.s3.us-west-2.amazonaws.com` host, stays beneath the canonical
`/assets/palladium/production/s3fs-public/` output namespace, and ends in an `MP4` directory with an
`.mp4` object. The contract rejects other USGS paths, wildcard or lookalike hosts, sibling or input
S3 buckets, credentials, ports, query strings, fragments, encoded path separators or traversal, and
redirects outside the exact page and media hosts. Recognizing this authority permits only inventory
publication: page assertions, soundtrack evidence, rights review, quality review, suitability, media
acquisition, provider processing, certification, and production admission remain independent gates.

The same seam recognizes the closed National Park Service video authority,
`nps.gov/media/video`. Every case uses the exact `www.nps.gov` host and binds one
canonical uppercase NPS 8-4-4-16 item ID to the exact
`https://www.nps.gov/media/video/view.htm?id=<UUID>` item and metadata URL. Its
representation stays beneath
`/nps-audiovideo/legacy/articles/<matching-UUID>/` on that same exact host. The
contract rejects other NPS paths, UUIDs that do not match the item, lookalike or
wildcard hosts, credentials, ports, query strings, fragments, encoded path
separators or traversal, and redirects outside the exact host. Required page text,
metadata digest, bounded response and predicted-media ceilings, and HEAD evidence
remain mandatory. This authority permits only inventory publication: NPS credit and
the copyright-symbol exception remain rights-review evidence, while soundtrack
evidence, complete-span fire or unsafe-act suitability evidence, media acquisition,
provider processing, certification, and production admission remain independent
gates.

The Blender Open Movie authority is narrower still. `blender.org/open-movies` currently recognizes
only item `sintel-trailer-720p`: the exact `https://durian.blender.org/download/` project download
page and exact `https://download.blender.org/durian/trailer/sintel_trailer-720p.mp4` representation.
The inventory must retain the representation name and `video/mp4` type. Other Blender projects,
subdomains, download paths, mirrors, directory listings, and generic project or licence pages are
outside this authority. The generic page adapter still requires authored page text, bounded page
and HEAD capture, immutable metadata identity, exact HTTPS hosts, and closed redirect handling.
This source rule grants only inventory publication. Soundtrack evidence, exact CC BY 3.0 scope and
attribution, music and embedded-work review, suitability screening, acquisition, provider
processing, certification, and production admission remain independent gates.

Direct/static acquisition is an authored local capture, not a source adapter and not a licence
shortcut. Its schema-v3 manifest predeclares an exact item count and positive quotas for the known
corpus roles and identifies one contracting owner or first-party origin; separate owners use
separate manifests so the emitted source authority is not collapsed into a generic direct bucket.
Those acquisition quotas may describe any bounded lane and do not duplicate the final truth-denominator
and holdout-role gates owned by certification. Every case names a non-empty regular media file plus
separate rights and provenance evidence files beneath one declared root, along with non-empty creator,
campaign, and source-family identity that survives rights review unchanged.
`filler-corpus-direct` resolves symlinks, rejects root escapes and quota drift, streams SHA-256 over
every file under aggregate byte and wall-time ceilings, and emits local transport records into the
same strict schema-v5 inventory. It never creates media, infers a grant from a directory or
collection, or turns authored assertions into approval. The combined public-plus-direct inventory
still goes through one independent rights review; local media is already acquired and is therefore
skipped by the network downloader. No direct-manifest schema-v1 reader or fixed 100-item compatibility
path exists because no certified artifact consumes one.

Corpus preparation is one fail-closed bridge from the fully reviewed inventory to blind review and
provider evaluation. An authored schema-v4 plan explicitly identifies either `development_seed` or
`certification` and may otherwise choose only the development/holdout split, similarity cluster,
source segment, direct-video window, corpus version, evidence version, and predeclared slice gates.
The caller must name the matching preparation profile; a plan cannot silently select it. The
development profile prepares every and only rights-approved row from the reviewed inventory, requires
at least 300 cases all in the development split, and leaves held rows inert. Campaign, source-family,
and creator identities are retained when the source knows them but are not invented merely to prepare
development evidence. The certification profile requires all 1,426–1,600 inventory rows to be approved
and covered exactly once, including at least 300 development and 1,126 independently clustered holdout
cases; it retains the complete acquisition-provenance and confidence-bound requirements. Both profiles
reopen media beneath separate local/direct and download roots; recheck size and all available
SHA-256/SHA-1/MD5 identities; measure an at-most-five-minute bounded segment; and emit source-policy
and decoder facts plus source text, four near-full-resolution frames, one 16 kHz mono WAV of that
segment for direct-audio and shared transcription lanes, and one at-most-60-second 1280×720
direct-video derivative. It stages derivatives before
publication and enforces aggregate source bytes, derivative bytes, and wall time. Preparation also
computes a 64-bit difference hash for each of the four semantic frames. Cases for which at least
three corresponding frames are within eight bits must share one similarity cluster; later holdout
validation permits only one case from that cluster. This catches re-encodes and close derivatives at
the only seam that still has media bytes, while source-family and campaign identity catch shared
masters that frame sampling misses. The resulting
draft copies creator, campaign, and source-family provenance from the acquisition inventory rather
than allowing the split plan to author it. It contains no semantic truth, evidence labels, or review
answer. Each packet is
validated against its draft digest before either artifact is written; provider input therefore
cannot acquire a hidden answer key through corpus preparation. No hand-authored packet or older
preparation shape is accepted.

`filler-corpus-review` derives one reviewer-visible randomized packet and one owner-only alias map
from that draft. A fresh batch and map are generated independently for each reviewer. Development
drafts remain `development_seed` through blind review and label lock and cannot satisfy the
certification contract; certification drafts alone receive the certification sampling and composition
checks. The label lock
accepts only that still-unlocked, wholly unlabeled draft, both exact alias maps, and strict
current-schema JSON/JSONL; unknown or trailing fields are errors, not compatibility data. It validates both blind
submissions as complete labels before comparing their canonical hashes, including the submission
that an adjudicator does not select. A third reviewer therefore resolves a real semantic
disagreement; adjudication cannot turn an incomplete or malformed second review into evidence of
independent labeling.

Schema-v5 inventory gives every exact representation a required **soundtrack expectation**. Its
closed status is `present_expected`, `intentionally_silent`, or `unknown`; the claim also binds a
closed evidence kind, the SHA-256 of already-frozen first-party descriptive, encoded-stream, or
reviewed source-manifest
metadata, a bounded evidence locator, and the SHA-256 of the representation identity. The
representation identity includes transport, name, URL/path, media type, origin, byte and source-hash
facts, duration, and dimensions. Changing any of those facts therefore invalidates the soundtrack
claim instead of carrying it onto different bytes. Publication year, title, a URL, or an unbound
free-text assertion is never soundtrack authority. A newly captured lane must state the claim
explicitly; omission does not become `unknown`. Historical schema-v4 inventories and discovery lanes
remain immutable evidence, but cannot authorize a new download or be silently upgraded to v5.

Soundtrack expectation is pre-download selection evidence, not decoded-audio proof. The generic
inventory accepts all three statuses and production does not globally reject intentionally silent
works. The versioned `temporal_structure_replacement_v1` quarantine-acquisition purpose is narrower:
only `present_expected` may enter its default download plan. `intentionally_silent` and `unknown`
remain visible with the closed `soundtrack_intentionally_silent` and `soundtrack_unknown` hold
reasons, and the rights locker and downloader reproduce the representation and
evidence binding before it creates an output directory or sends the first request. The rights
worksheet exposes the exact soundtrack fields and binds the acquisition purpose; its locked decision
must carry the same purpose. The ordinary `local_quarantine_inspection` purpose preserves the generic
inspection lane. A decoded file can still fail the independent full-source audio, silence, and
quality gates. Restoration, synthesized music, or separately licensed soundtracks require a new
designed purpose with their own rights, lineage, inspection, suitability, and admission authority.

Media acquisition consumes a separate rights-review ledger; discovery output is never download
authority. The caller names one of three non-interchangeable profiles: `quarantine`, `development`,
or `certification`. Every `approved` row binds the inventory digest, authority-qualified case ID,
source metadata hash, reviewer, review time, rationale, purpose-specific authority, attribution, and
restrictions; `held` rows remain inert. The downloader preflights aggregate item and byte ceilings
before its first request, stays serial and identified, checks the initial URL and every redirect
against both the case's frozen allowlist and the built-in policy for that authority, bounds each body
by the inventoried size, verifies source checksums when present, and adds SHA-256. Query strings may
remain when they are part of the exact frozen representation URL; credentials and fragments never
may. The request ceiling counts the initial request and every redirect hop before that hop is sent.
For Met original images, the inventory freezes a metadata-digest cache key, and each bounded
GET attempt may add its deterministic run/case/attempt cache key to avoid inconsistent CDN cache
entries. These keys cannot change the selected host or image path, relax exact byte counts or
available source checksums, or expand the existing request and byte ceilings. Media and its download
ledger remain external to Git.
An incomplete, stale, oversized, or checksum-mismatched plan fails without producing a completed
ledger and cannot flow into blind semantic review.

`quarantine` is the narrow pre-review acquisition profile. Its schema-v6 worksheet locks a
schema-v1 quarantine contract that must grant only local copying/storage and local technical
inspection. Provider transfer, redistribution,
development/certification corpus preparation, training, catalog ingestion, scheduling, and
production admission are all present and false rather than inferred from omission. An approved
quarantine decision may therefore retain `redistributable=false`; it is authority to obtain and
measure an exact source locally, not a finding that the source may be published or used. The
download ledger records the profile, and every downstream corpus-preparation profile rejects a
quarantine contract even if its item identity and content hashes are otherwise valid. Promoting a
surviving source requires a new development or certification rights decision against the same
frozen inventory; a quarantine decision is never upgraded in place.

`filler-corpus-quarantine-inspect` is the sole post-download quarantine gate. It consumes the exact
schema-v5 inventory, schema-v2 quarantine download ledger, and the complete public/private authority
pair for the named prior holdout. It strictly re-establishes those identities before opening media,
resolves each ledger path beneath its declared root without symlink escape, and rechecks byte count,
ledger SHA-256, and every available inventory checksum. One full-source probe and decode then records duration,
dimensions, audio/video presence, and normalized black, silence, and freeze spans. The same complete
decode cadence used by the production-reference duplicate audit produces visual dHash and audio-RMS
sequences. New candidates are compared to one another and to every distinct source in the prior
holdout authority; exact source or rendered-case hash collisions and perceptual relationships are
reported separately. Missing prior source bytes make perceptual exposure `incomplete`, never `clear`.
The caller supplies a positive media-processing wall-time ceiling, which is recorded in the report
and covers candidate and prior-source hashing, probing, decoding, fingerprinting, and in-process
fingerprint alignment. Expiration publishes no partial report.
Missing audio or video, inventoried duration/dimension drift, unusable fingerprints, or black,
silent, or frozen coverage at or above 95% is a technical hold; lower coverage remains measured
evidence for later content review rather than an invented quality score.
The immutable report binds every raw input digest, tool identity, algorithm version, observation, and
comparison. A technically intact and exposure-clear result means only `eligible_for_rights_review`:
provider transfer, redistribution, corpus preparation, training, catalog ingestion, scheduling, and
production admission remain explicitly false. The command never repairs, transcodes, uploads, labels,
or promotes media, and any failed mechanical check leaves the case held in quarantine.

Development and certification rights review consume that quarantine disposition as a fail-closed
authority input. Schema-v6 `quarantine` review remains the pre-download local-copy/inspection path
and cannot consume a report that does not exist yet. A development or certification worksheet that
can select any non-local case instead requires one strict schema-v1 quarantine-inspection report.
The worksheet freezes the raw report SHA-256 and all four report input identities. Report binding
advanced development worksheets from schema v3 to v6 and certification worksheets from schema v4
to v7; the soundtrack authority advances their current schemas to v7 and v8 respectively.
Historical worksheets and their decisions remain readable evidence, but cannot authorize new preparation.

Selection is transport-aware and deterministic: it is the union of valid direct
`transport=local` cases and non-local cases that occur exactly once in the bound report as
`eligible_for_rights_review` with no hold reasons. A held or absent non-local inventory case cannot
enter the worksheet. A local-only inventory does not invent a download-ledger or inspection-report
requirement. The rights lock independently reopens and validates the exact report, reproduces that
selection, and attaches the report binding plus inspected content SHA-256 to every locked non-local
decision; worksheet or CSV projections are never trusted as inspection authority.

Preparation independently reopens the report before it creates a derivative staging directory or
provider-visible packet. Every non-local approval must carry the reproduced report binding, and the
actual local source SHA-256 must equal the inspected content SHA-256. A held or missing case, report
drift, swapped worksheet, legacy unbound decision, or source-byte mismatch fails before output.
Direct local media retains its inventory-bound path and is still rehashed against every available
inventory checksum. One deep `fillerquarantine` module owns strict report decoding and validation,
inventory/report identity, case uniqueness and disposition, transport-aware selection, and
content-hash requirements; review, lock, and preparation commands do not reimplement that policy.

A rights worksheet is a deterministic review aid, not authority. It records the digest of the exact
frozen inventory, presents every selected source assertion and representation fact, and leaves the
reviewer, time, decision, rationale, redistribution, attribution, and restriction fields inert. Its
spreadsheet view neutralizes formula-leading source text and keeps those authority columns blank.
Locking a completed spreadsheet re-reads the original inventory and inert JSON worksheet, rejects a
changed header, immutable cell, missing or duplicate row, incomplete decision, invalid time,
unattributed BY/BY-SA approval, or inconsistent redistribution claim, and only then atomically emits
the downloader's JSONL ledger. The completed ledger binds each row to both that inventory digest and
the item's metadata digest; copying an approval between inventory snapshots fails closed.

The development-only Met lane may reduce repetitive spreadsheet entry without reducing that item-level
lock. Its batch-completion aid accepts only the exact current inventory and inert worksheet, a complete
zero-hold Met metadata pre-screen bound to the pinned Open Access policy evidence, and one separately
authored maintainer attestation bound to all three artifact digests. Met search and object metadata
must have unambiguous field identities: duplicate JSON keys, including alternate casing that would
bind the same decoded field, fail discovery or hold pre-screening. Legitimate additional provider
fields remain allowed; they do not become rights assertions. The attestation names the reviewer
and review time, explicitly accepts the recorded non-copyright limitations, and authorizes only private
development-corpus copying, technical transformation, and evidence extraction. The aid reconstructs
every immutable worksheet row and fills the same decision columns for each independently identified
item; the ordinary rights locker remains the sole producer of downloader authority and revalidates all
rows. Current report-bound development worksheets retain their exact quarantine report and per-case
content bindings through batch proposal and completion; the batch aid must not strip those bindings,
synthesize eligibility, or reinterpret quarantine-only permission as development or redistribution
permission. Missing, malformed, or inconsistent bindings refuse the batch. The ordinary rights locker
independently reopens and validates the exact inspection report before producing any downloader
authority. The attestation time must be canonical UTC and no earlier than its bound pre-screen; this
offline development artifact has no rolling production-certificate lifetime. A held pre-screen case,
mixed authority, incomplete coverage, stale time, changed artifact,
unknown field, blank attestation, attribution requirement, or non-empty restriction refuses batch
completion and returns the reviewer to the ordinary item-level exception path. The batch attestation
does not grant certification, provider transfer, training, production, ingestion, scheduling, or
broadcast authority and is not a model decision or chain-of-title warranty.

The visual-corpus nomination lock accepts an explicit `exclude` disposition so a completed review
does not have to mislabel an unsuitable or uncertain downloaded work as positive or clean. An
excluded row makes no subject, generation, or diagnostic-slice assertion and publishes no candidate
or rights file. Blank and unknown dispositions still fail the complete review. The nomination set
binds the exact canonical completed-review digest, total reviewed count, excluded count, and every
published candidate; the locker reopens the original inventory, materialization ledger, worksheet,
and media before publication. This is a workflow exclusion only: it creates no truth, training,
provider-transfer, certification, ingestion, scheduling, production, or broadcast authority.

Nomination preparation also emits a private, non-blind keyboard review board beside the inert JSON
and CSV. It shows the institution-authored identity and the worksheet-bound source image, and exports
the ordinary completed CSV consumed by the locker. A reviewer may load a separate local model-
assistance manifest only when its worksheet, case, rank, and exact content identities reproduce. The
browser may then prioritize proposed positives and, after one explicit confirmation, mark every
non-proposal as excluded; it never authors a positive or clean decision. Those decisions require an
individual reviewer action except in a worksheet whose every row carries the sole canonical
`policy-clean-nomination` role. For that clean-control worksheet, the board may render a bounded page
of exact source images and let the reviewer explicitly confirm all eligible, previously undecided
images on that page as clean only after a fully bound clean-assistance manifest covers every exact
case with two distinct local vision-model families plus a local OCR text-safety screen, every image
has loaded, and the reviewer attests that they
checked the whole page for adult nudity, minors, sexual or graphic content, hate symbols or text, and
other broadcast-unsuitable content. A version-two assistance manifest may add one complete, source-bound
frontier audience-review ledger with a closed suitability-flag vocabulary. It records observations and routes
cases only: every record binds the worksheet case, rank, exact source SHA-256, review method, and non-authority
flags; absence is never truth. The board accepts the version-two manifest only when its ledger digest, reviewer,
vocabulary, all per-case record digests, and complete worksheet coverage reproduce. Any observed suitability
flag makes the case ineligible for page confirmation and is rendered as a concise factual candidate signal for
individual inspection. Those digests are consistency seals, not reviewer-authentication signatures; the reviewer
still explicitly selects the private manifest. The page attestation explicitly covers sexual content; minor or
age-ambiguous sexual risk; violence, gore, death, self-harm, animal harm, weapons, and frightening imagery;
tobacco, alcohol, drugs,
gambling, and regulated-product promotion; hateful or extremist material; and prohibited visible written
language. It does not call ordinary minor presence, religion, politics, war, historical context, or brand
presence automatically prohibited.

A model-positive, age-risk, overlap-hold, or targeted-review row
is never eligible for that page action and must receive an individual decision; loading assistance
also clears any earlier convenience clean decision on such a row. Positive decisions always remain
individual. Every unresolved row remains blank so locking fails. The assistance bytes are neither
copied into the worksheet nor accepted by the locker. The board is development ergonomics, not the
candidate-blind certification review, and the locker still reopens every source byte after the
review.

The certification holdout uses a distinct schema-v8 worksheet and schema-v1 rights contract, while
quarantine uses its schema-v6 worksheet and schema-v1 acquisition contract. Historical schema-v3
development and schema-v4 certification artifacts remain readable evidence but cannot authorize new
preparation. The caller must name
the `quarantine`, `development`, or `certification` profile before either locking rights decisions or
downloading media. Certification binds one maintainer/counsel-approved agreement identifier and
SHA-256 plus one exact processor and terms-snapshot SHA-256 into the inert worksheet. Each completed
per-master schedule then binds its own identifier and SHA-256, confirmed signer-authority evidence,
and separate grants for commercial evaluation, copying/storage, technical transcoding, bounded
frame/audio/transcript/OCR extraction, the named processor transfer, and the permitted redistribution
scope. Provider transfer is never inferred from redistribution, a public licence, or zero-data-
retention routing.

The per-master schedule records a closed status for embedded music, performers and voices, stock
footage and artwork, trademarks, privacy/publicity, and locations; worldwide territory; the granted
term and any expiry; attribution and restrictions; and, when ambiguity was escalated, the distinct
adjudicator and disposition. Approval requires every required grant, confirmed signer authority,
each embedded-rights category cleared or proven absent, the exact worksheet processor and terms
snapshot, a non-expired worldwide term, and digest-valid agreement, schedule, authority, and evidence
records. Blank, unknown, conflicting, expired, or mismatched facts emit stable hold reasons and no
certification authority. Executed agreements, schedules, reviewer identities, and media remain
private external artifacts; repository fixtures are synthetic and public evidence records only
content digests.

This mechanical contract does not approve its legal form. The maintainer or counsel separately names
the agreement, rights reviewer and ambiguity adjudicator, outreach identity/channel and compensation
ceiling, processor boundary, and first recruitment batch before any contact, signature, acquisition,
download, provider upload, or spend. Changing any of those approved identities invalidates the
affected schedule rather than being treated as a compatible metadata update.

Certification rollout is comparison-first on a bounded development workload. These comparisons
measure classifiers and preserve disagreement evidence; they do not run in the household conveyor
and cannot create, delay, or reverse a Ready transition. Any model, provider, prompt, schema,
taxonomy, extractor, or policy change invalidates the affected certification measurement until it
is replayed.

The human surface follows the same ownership boundary. **Needs help contains exceptional choices
only**, each asking one plain question whose answer changes durable product state and showing the
decisive evidence or conflict. Ready/not-usable outcomes belong to Activity; queued, running, retry,
provider, and budget state belongs to Diagnostics. Overview answers whether the whole Filler
workspace is working from the readiness projection and offers its one ranked action only when one
exists. The header watch pill reports source activity only. Ordinary maintenance never asks a
person to interpret confidence thresholds or audit classifier output.

Incoming is a calm progress surface for first-time and ordinary use: a compact summary of preparing,
Ready, not-usable, and genuinely blocked items, followed by the recent items people are most likely
to inspect. It does not render one review form per Clip. A Needs-help card appears only when the
server provides a current task and actions; playback is required before a media-content answer, while
navigation and diagnostics never masquerade as semantic actions. Classification detail and
certification comparisons are Advanced/development views rather than a household inbox.

#### Certified role routing and evaluation accounting (V62)

V61's five roles are one versioned Go-owned **inference policy**, not five independent settings:
`lineup`, `filler_text`, `filler_frames`, `filler_video`, and `transcription`. Its small interface
resolves a role to one concrete model/provider route plus capability, privacy, fallback, and budget
constraints. The compatibility snapshot proves that a route accepts the required modality and
structured-output parameters; the admission certification artifact separately proves measured
quality. A compatibility result alone never authorizes unattended admission.

The last fully certified policy is the automatic default. Until one exists for a role, Loomarr
retains the currently configured compatible route but labels it unverified and cannot expand
unattended admission through that role. Manual model/provider selection moves under **Advanced
overrides**. An override is explicit operator intent, is recorded in every evaluation, and does not
inherit the certified label. A missing or incompatible route is an operational hold with a recovery
action, never a semantic verdict. The live model catalog remains useful for override discovery but
does not rank unknown families by a stale global quality tier; only an exact model named by the
versioned role policy can be recommended.

Every inference adapter returns one provider-neutral attribution envelope with its semantic output.
The envelope records requested and resolved model/provider identities, modalities, prompt,
completion, reasoning, cached/cache-write, and media token categories when returned, exact
provider-reported charge and currency, latency, attempts, and provider generation id. OpenRouter
requests opt into routing metadata and record the selected upstream endpoint; `usage.cost` is the
billing fact. Missing provider facts remain missing rather than being estimated. Local price
estimation is a separate value tied to the run's immutable price snapshot. Aggregate Prometheus
token counters continue unchanged and never encode a price.

Evaluation accounting is durable state with one atomic reservation-to-settlement transition; after
settlement the row is immutable. One row owns the clip/evidence identity, role
and cascade rung, derivative byte/duration/pixel bounds, the attribution envelope, prompt/schema/
extractor/taxonomy/admission-policy and role-policy versions, price snapshot and estimated charge,
retry reason, and terminal operational outcome. Exact decimal charge text and an integer
nanodollar projection are retained together so historical records neither drift with current prices
nor accumulate binary floating-point error. Per-clip, UTC-day, and certification-run ledgers reserve
budget before a call and settle it from the returned charge; shared-appliance inference is serial by
default so concurrent reservations cannot overspend. Exhaustion leaves the evaluation held and
recoverable.

The deterministic evaluation cache identity includes the clip/evidence, extractor, prompt/schema,
concrete model/provider and role/capability policy, taxonomy, admission policy, modality, and bounded
derivative dimensions. A change to any semantic input cannot reuse an older answer accidentally.

#### Durable readiness audit and operator projections (V63; household beta revision)

The terminal-ready module is the single runtime lifecycle owner between conveyor completion and the
playable catalog. It consumes persisted Clip/pipeline state, commits the effective outcome before it
can affect scheduling, and appends a durable event. Classification evaluation stays outside this
authority. A provider, schema, extraction, or budget failure cannot enter Needs help merely by
changing labels in an API handler.

The effective event is immutable and stores the exact Clip identity, Enrollment reference,
Placement, pipeline identity, outcome, and creation time. Genuine human state changes remain
append-only actions. Development classification decisions retain their evidence and cost lineage in
their own measurement store but are never joined into runtime readiness by inference.

Skipping a diagnostic or optional detail is navigation, not an action. The server records only an
answer that changes durable product state; it does not manufacture review-friction events.

The store exposes one conformance contract over SQLite and Postgres. Ready-event insertion is
idempotent by event id and rejects a different payload under the same id. Event insertion, Placement,
hold release, and pipeline settlement are one transaction. Reads use bounded keyset pagination with
a stable `(created_at, id)` order, and counts are computed by the same predicates as their rows.
Forward migrations add the tables and indexes; applied migrations remain immutable.

Four runtime projections are server-owned, not client-derived:

- **Overview** reports the latest effective outcome per Clip, operational counts, and at most one
  server-ranked next action. The broader readiness projection owns the page's single workspace
  verdict. A later Ready event clears an older Operational hold from current health without erasing
  it from Activity.
- **Needs help** contains only current exceptional choices with one non-empty question, decisive
  evidence, and the closed actions currently allowed. Queued work, retries, optional classification,
  provider/budget holds, and audit sampling are structurally ineligible. The browser never invents
  task kinds or actions from reason text.
- **Activity** is the bounded audit of Ready/not-usable events and genuine human actions.
- **Diagnostics** contains each clip's latest operational hold and one server-authored recovery plan.
  The plan is exactly one of: an automatic retry with its next attempt time, a currently allowed
  manual retry, a precise configuration destination, or inspection of the exact held media. Provider,
  budget, extraction, media-inspection, and policy holds map to those closed modes in
  the server; the browser neither derives a destination from the hold code nor invents a
  command. Provider response text, local paths, raw prompts, and evidence locations are never
  projected.

Manual diagnostic retries use a separate append-only recovery-action ledger rather than semantic
review actions. The request carries a caller id, the authenticated actor is recorded by the server,
and the store rechecks that the named decision is still the latest retryable operational hold before
committing it. Repeating the same request id is a no-op; reusing it for another decision conflicts.
The executor may move first and the ledger second: if a process stops between them, retrying the same
request recognizes already-scheduled machine work and safely completes the audit. A failed executor
records no success. Until a newer admission result exists, the hold remains visible with its actual
automatic/manual state; the UI never removes it optimistically.

Overview and Activity follow the existing member-readable filler contract. Needs help,
Diagnostics, and every action require an admin; member attempts return 403 and create no action.
The API never returns raw evidence sources or locations, provider response bodies, or secrets. A
Needs-help action requires the current unresolved task and is idempotent under its action id; stale
or duplicate conflicting actions fail closed. Automatic publication requires the terminal-ready
transaction and Enrollment authority. Classification certification never grants or withholds it.

**Loudness normalisation is playback conditioning, not admission.** The transcode rung may build a
normalised playback derivative when `filler.conditioning.normalize_loudness` is enabled, using the
shared `filler.target_lufs` target. It retains the exact source master, records before/after
measurements and recipe identity in the sidecar, and never mutates an operator's original. The
former auto-file-named setting is retired because conditioning does not grant publication authority.

#### Beta release readiness is one fail-closed report

The household workspace and the operational `/v1/filler/readiness` projection answer whether an
installed Loomarr can use filler now. They do not certify a release candidate. A beta candidate is
releasable only when one local, versioned manifest binds the exact server, Web, and Android TV candidate
identities to the immutable evidence accepted for that candidate. The release process evaluates that
manifest into one canonical, privacy-safe **GO** or **HOLD** report; issue state and prose comments are
coordination aids, never release authority.

`internal/fillerrelease` owns that evaluation as one deep module. Its external interface accepts the
manifest bytes, a caller-supplied filesystem rooted at the evidence bundle, and an explicit report
time, then returns the complete report. The command adapter owns only flags, file I/O, JSON output,
and exit status. Tests substitute an in-memory filesystem through the same interface used for the OS
filesystem; the evaluator has no GitHub, provider, search, download, publication, deployment, or
runtime-admission authority.

The beta.6 manifest is strict and closed. It names schema version 2, an assembly/expiry window,
release tag, git commit, server image digest, Web build, Android TV artifact/version, and
configuration-profile identity. Its cohort binds intentional source identities and exactly 32
unique source-master, lineage, playback-derivative, and sidecar hashes selected in frozen seed order
from clips the shipping pipeline actually marked Ready. Every row records successful Range playback,
and at most one row may name a duplicate-family identity. This exact household cohort is release
evidence, not general filler certification or a second admission path.

The manifest references seven bounded pipeline results: media preparation, duplicate control,
configured-language handling, suitability, readiness, playback, and descriptive enrichment. Each
result declares its schema, policy, model, prompt, profile, and build identities plus an explicit
denominator, abstentions, holds, and prohibited admissions. A deterministic result uses `none` for
model and prompt; the enrichment result identifies the model and prompt without granting either
readiness authority. Separate Archive.org and operator-authorized YouTube journeys each bind their
exact source and playback hashes and prove acquisition, preparation, Library readiness, and Range
playback through the shipping path.

The same manifest binds Web and Android TV emulator installed journeys that exercise channel
selection, pod selection, and Range playback, plus deployment and rollback evidence and the candidate's SBOM,
signature, provenance, and notice artifacts. Unknown fields, duplicate identities or families,
unsafe paths, missing or changed artifacts, inconsistent denominators, non-Ready clips, stale
candidate bindings, incomplete source or client journeys, unverified release artifacts, prohibited
admissions, or any residual decision fail closed. Artifact contents are not copied into the public
report. General structure, role, visual, spoken, written, and unattended-admission certification is
beta.7 work and must not appear as passing beta.6 release evidence.

The Android TV beta.6 journey runs on one explicitly named Android TV emulator and binds that
emulator identity, installed candidate identity, and playback evidence. The release harness refuses
a physical device for this gate, which keeps the acceptance run repeatable and unattended. Physical
Shield review remains useful post-beta confidence work, but it is not beta.6 release authority.

The evaluator always returns a report for readable manifest bytes, including malformed manifests.
Every HOLD uses stable machine-readable reason codes plus bounded public-safe context. A GO report is
possible only when every closed requirement is present and passing for the same candidate and there
are zero residual human decisions and zero operational failures. Report fields and reason ordering
are deterministic; a self-digest is calculated over the canonical report with its digest field
empty. The command writes the report even on HOLD, exits zero only for GO, uses a distinct non-zero
status for HOLD, and treats inability to read the manifest or write the report as an execution error.

**A new install has an empty drop-folder, so the first channel has nothing to break to.** The fix is a **starter pack**: `GET /v1/filler/discover?collection=<id>` lists a curated archive.org collection, the operator keeps or excludes rows, and only what survives is fetched through the ordinary ingest path. Three properties are load-bearing:

- **It is a listing, not an acquisition.** Nothing downloads until the operator chooses, so a suggested pack cannot fill a stranger's disk with clips they never asked for. This is the same rule the approval gate states for titles (§7): the machine proposes, a human commits.
- **It is the discovery path with a different argument**, not a parallel one. A starter pack that acquired through its own route would be a second implementation of ingest — the shape §10 already rejected for filler search, and the shape that let `filler_sources` ship with no reader.
- **The pack is a default, never a requirement.** "Start from scratch" is always offered, and a collection that has gone away degrades to an empty list with the reason shown — not a blocked channel. An operator with their own clips must never have to walk through someone else's taste to reach their own.

⚠ **The starter collections are product-curated seeds, not operator configuration.** They may
change with a release and degrade to an empty list when unavailable. V55 retired the unconsumed
setting that previously implied an operator could redirect this flow.

### Pulls — the approval gate arrives for filler (V35)

The three properties above are right and had **no object to hang them on**: "the machine proposes, a human commits" described an intention, while the only thing that existed was a listing endpoint and a download button. A **pull** is that object.

A pull is a **plan Loomarr composed across sources** — *"fill the 1990s kids gap"* resolving to several collections, each with a reason and an estimate. It is persisted, it appears in the approval queue beside title proposals, and **nothing downloads until it is approved**. A starter pack may seed a pull on a fresh install instead of driving a parallel path, but is not an operator setting.

**What a pull carries:** a title, who or what proposed it, a rationale, plan rows (each a source with a reason, an estimate, and the ability to drop it before approving), an aggregate estimate, and an optional operator note for the decision record. The note is an annotation only: it does not select, add, remove, or filter downloads. The review UI labels this limitation explicitly; operators change the reviewed set through the source or candidate exclusion controls.

Three rules, each of which is a safety property rather than a feature:

- **Approval enqueues through the existing ingest path.** A pull that downloaded through its own route would be a second implementation of ingest — the shape this section already rejects for filler search, and the shape that let `filler_sources` ship with no reader. Approval writes work items; the ordinary job does the fetching.
- **A pull whose sources are all switched off is refused, not silently empty.** Disabled sources are a real precondition; the operator is told which switch to flip rather than watching an approved pull do nothing.
- **Dropping a plan row before approval is part of the gate, not an edit afterwards.** The committed set is what the human agreed to.

⚠ **The gate binds bulk composition, not an admin's own hands.** An admin searching one source and queueing one clip stays direct — the §7 shape, where an admin may `POST /v1/titles` because the admin *is* the gate. Requiring a proposal for a single deliberate click would make the gate ceremony, and ceremony is what teaches people to click through it. What the gate exists for is what happens when *nobody is looking*: a composed multi-source plan, which is exactly what a pull is.

**A pull decision commits once (#955).** Approval compares-and-sets the persisted pull from
pending, writes the exact reviewed plan (including dropped rows), note, actor and decision time,
and creates one durable queued acquisition run in a single short transaction. Only after commit
may the ordinary manifest-backed ingest job start. A losing approval or dismissal returns a
conflict and cannot overwrite the winning audit or enqueue another run. The transaction never
spans downloader execution; validation or persistence failure before commit leaves the pull pending
and creates no run.

The uniquely pull-bound decision record constrains new approvals without deleting historical
acquisition audit. The acquisition snapshot writer can update an existing pull-bound run but cannot
create one outside the approval commit or change an execution's trigger/source/pull ownership. Approval of a pending pull that already has a historical run is refused for review, rather
than silently starting another download. An operator may dismiss that still-pending proposal
without changing or canceling any historical acquisition; dismissal cannot overwrite an approval. Historical source-level plans remain executable through
the same approval boundary. In the single-replica beta, startup settles any queued/running run left
by the previous process as a visible acquisition error, including a crash after the approval
transaction but before launch. The approved decision remains historical truth; retrying that
approval cannot create another run. An operator may propose a fresh pull after reviewing the
interruption. Recovery does not claim automatic replay or successful download.

### Acquisition intent chooses exact remote items (V66)

V35 supplied the approval object but its first implementation stopped one level too early: it
made one row for every enabled source, described each as *“a source you added and left switched
on”*, and approval downloaded the entire collection. That is an approval over **where** bytes come
from, not a decision about **which bytes are worth acquiring**. On a compilation-heavy source it
also hands the splitter an arbitrary prefix rather than material chosen to close a catalog gap.

A pull now begins with a versioned **acquisition intent**. Its vocabulary is closed and
inspectable: desired content roles, era observation range, audience, target geography, maximum
remote duration, missing taxonomy axes, rights preference, source allow-list, representation
quality floor, requested item count, and the catalog reason for the pull. An omitted constraint
means *not requested*; an unknown candidate field means **unknown**, never a match. In particular,
an upload or publication date is only a weak remote observation and never becomes the clip's era.
The admitted clip still earns its semantic facts from the ordinary evidence pipeline.

The application derives a default intent from the same `PoolReport`/per-channel coverage read used
by the Filler overview, so *“why this pull?”* cannot drift from what the operator sees. Explicit
operator constraints may narrow that draft. Deterministic policy owns selection: a future LLM may
propose evidence or search terms, but may not silently relax a constraint, invent missing metadata,
select an unregistered source, or cross the approval boundary.

Planning is metadata-only and has one fixed 90-second wall-time budget, at most 12 enumerated
sources, and at most 100 candidate observations across the whole proposal. Candidate quota is
distributed deterministically across the eligible source prefix; sources beyond the bound remain
visible as `source_limit`, rather than disappearing from the audit. Each enabled, fetchable,
geographically eligible, allowed source is enumerated through its registered provider adapter with
bounded pagination. The normalized remote
identity is `(provider, registered source id, provider item id)`; URLs are payload, not identity.
Archive and YouTube are peers behind this seam. The planner filters items already catalogued,
staged, queued by another pending/approved pull, previously declined in the same intent family,
or repeated within the proposal. It then applies geography, duration, era-observation and
representation-quality constraints. Provider-declared licence metadata remains attached to the
candidate for provenance but is not a constraint or ranking signal.

Every registered source receives its own durable disposition (`enumerated`, `disabled`,
`not_fetchable`, `not_allowed`, `geography_mismatch`, `source_limit`, or
`enumeration_failed`) because a source that yields no candidate cannot honestly attach its result
to an item. Every considered item likewise receives a stable disposition code and measured observations. Selected rows
retain their exact item URL; excluded rows retain why they lost (`already_catalogued`,
`already_queued`, `previously_declined`, `duplicate_remote`, `source_not_allowed`,
`geography_mismatch`,
`era_unknown`, `era_mismatch`, `duration_unknown`, `duration_exceeded`, `quality_unknown`,
`quality_below_floor`, `role_unknown`, `role_mismatch`, `audience_unknown`,
`audience_mismatch`, `taxonomy_unknown`, `taxonomy_mismatch`, or
`ranked_below_limit`). Ranking is deterministic: constraint fitness, representation quality,
useful diversity across source/era observations, then normalized remote
identity as the final tie-break. The same inputs therefore produce the same proposal and rejected
explanations.

The persisted pull binds the intent version, selected exact candidates, rejected explanations, and
source snapshot used to decide. Approval may drop individual candidates and add an annotation-only
note. The same note semantics apply to retained source-level pulls. Notes never change the approved
identities or imply that free-text acquisition restrictions have been enforced.
At the commit point Loomarr revalidates that every selected candidate still belongs to the same
enabled registered source, remains inside its geographic/source policy, and is not already
catalogued or queued. It then hands **the candidate URLs**, never the collection URL, to the
existing manifest-backed ingest path. Nothing is downloaded during enumeration or proposal.

Scheduled acquisition adopts this selector only after parity tests prove its existing disk/catalog
ceilings, per-source bounds, held lifecycle, and failure reporting remain intact. Until that cutover,
the explicit pull is the proving surface; there must not be two ranking algorithms. Safety
classification, media conditioning, compilation segmentation, post-screen taxonomy enrichment,
and final admission remain downstream stages. Acquisition intent chooses promising evidence; it
does not certify that evidence as safe or airable.

### Removing a clip from the catalog is a tombstone (V35)

The catalog's bulk actions include **Remove from catalog**. It marks the clip removed; it does **not** delete the row, and it does **not** touch the file.

⚠ **A row delete cannot work here.** The catalog is a synced *cache* of `FILLER_DIR`, so the next scan finds the file still on disk and re-creates the row: the operator removes a clip and watches it come back minutes later. The tombstone is what survives a re-scan, and it survives because the scan's upsert is not allowed to write it — the same protection the play counters have.

⚠ **Deleting the file is not the alternative.** Nothing in Loomarr deletes an operator's media: disabling a source keeps its clips, and deleting a source keeps its clips because they are real files that may already be tagged and pinned into a channel. The action names the catalog, and stops at the catalog.

A removed clip is excluded from the catalog listing and from pod assembly **by default**. That polarity is load-bearing rather than cosmetic — assembly loads the catalog with an unfiltered read, so an opt-in exclusion would leave a removed clip airing until some caller remembered a flag. Restoring is the same write with the timestamp cleared.

### What is waiting on a human: the Incoming queue (V35)

Between "a file arrived" and "a clip the scheduler can place" there is work only a person can finish. `GET /v1/filler/incoming` is that queue, in one read:

- **Clips whose tags need a human** — an era the tagger proposed but could not ground in the clip's text (the rule above), or a commercial with no match tags at all. These are **two different questions** and stay separate: the first has a proposed answer to confirm, the second has nothing to confirm. Bumpers and station IDs never appear — they do their bookend job untagged, so queueing them would be work that changes nothing. ⚠ **Nor does a COMPILATION** (V54): it is `kind=commercial` and permanently untagged — the pipeline deliberately skips tag and vision for a composite, "a compilation is cut up rather than filed" — so read literally it satisfies the second bullet above, and for a while it did. It is not an advert with missing tags; it is a container of adverts, and its handoff to a human is the reel below. Asking an operator to "Add tags" to it would tag twenty unrelated products as one clip.
- **Compilations mid-split** — persisted proposals only after bounded detection and classification have examined every usable segment. Known duplicates and below-floor fragments have already been discarded; the attention count is limited to genuinely doubtful boundaries, unsplittable spans, and examined segments the model could not classify.

⚠ **The response is a window, not the database.** Each conveyor/audit array is capped at 100;
the full totals are counted independently in the store and drive headings and the badge. Loading
all held clips or every split-proposal JSON document merely to render the first screen makes a
large import a memory and latency spike, while using `len(rows)` as the total hides work beyond
the cap. The store therefore counts the conveyor union and review-ready reels separately from
their bounded row reads.

⚠ **Historical V35 state, retired in V38:** no confidence score was initially reported because
nothing measured one. The tagger now persists a grounding-capped diagnostic score, while the queue
still reports *why* an item is waiting from real state. The number does not grant admission.

### Sources can be switched off

The **drop-folder** and each **remote collection** can be **disabled**. A disabled source is **not scanned**, and **cannot enter a pull** — neither when one is composed nor when one is approved, which is re-checked at the commit point because a source can be switched off while a pull sits in the queue.

⚠ **What the switch does NOT bind, deliberately: an admin's own hands.** The Sources tab only offers search on an enabled collection and scopes it to that collection; the discover route can also perform a global keyword search, and `POST /v1/filler/ingest` takes a URL the operator typed. Those latter two are the *single-item direct* path this section already carves out — an admin acting deliberately, one item at a time, is the person the gate exists to defer to (§7). The server does not pretend an arbitrary collection reference came from a registered row; the row UI is where enabled-source scope is known.

⚠ **Clips already in the catalog stay.** Disabling a source withdraws it from *future* work; it is not a delete, and it must never look like one. This is enforced at the scan and fetch sites rather than by hiding the source in the UI — a toggle that only dims a row is a claim the system does not honour.

⚠ **The media-server library row is SCANNED again (V38c — this reverses V35).** A `library` source names a media-server library that Loomarr scans for clips, exactly as it scans a watched folder, and it carries a working on/off switch like any other source.

*What this reverses, and why it is not a quiet flip.* V35 recorded, at length, that the row had no switch **because nothing scanned a library** — the media server had been taken out of the filler path by §9.1 and a toggle would have been "a control that dims a row and changes nothing". That reasoning was sound about the code as it stood. The maintainer's decision (2026-08-02) is to give the kind real work instead of removing it from the UI: the mock's "Add a source" dialog offers Media server library, Watched folder, and Playlist/collection URL, and an operator who already keeps their commercials in an Emby library should be able to point Loomarr at it rather than being told to copy files.

⚠ **What §9.1 forbade is still forbidden.** The dependency this restores is NARROW and must stay that way:

- A library source is **one source among several**, never the catalog's only route. An install with no media server, or one whose media server is down, still gets a full catalog from its folders and remotes — the failure §9.1 removed was *"no media server ⇒ no commercials"*, and that must not come back. A library scan that fails is logged and skipped, exactly like an unreachable archive collection.
- **Clip identity is still the content hash**, never a media-server item id. This is the third identity change's whole point (below): identity comes from the bytes Loomarr can see, so a library that is re-indexed, moved, or removed cannot orphan a catalogued clip.
- **Clips still live in the clip folder.** A library scan is an *acquisition* path, not a second storage model: what it finds goes through the same intake as everything else (watch → hash → file → sidecar), so there are still no divergent paths. Loomarr does not play clips out of the operator's library in place, and never modifies it.
- **Program content stays separate.** The reason commercials were moved out of the media server was that filler could otherwise leak into a programming lineup. A library registered as a *filler source* is read for clips only; it is never offered to the suggester as programme material.

*(An earlier draft specified `FILLER_SOURCE_LIBRARY_ENABLED` as a setting. It stays deleted — V37 made sources one flat list, so a library's switch is a column on its own row like every other source, not a key in §15.)*

### Sources are one flat list (V37 — supersedes the derived/registered split)

**Every source is one row in one list**, whatever backs it: the drop-folder, the media-server
library, each Internet Archive collection, each YouTube playlist. An operator adding a source
picks a **kind** and gives a target; the list is the whole answer to *"where does filler come
from?"*

#### A fresh install ships with sources (V38c.8, maintainer)

`folder` and `library` were the only seeded rows; **Internet Archive and YouTube now seed too**,
so a new install can fetch without the operator first having to know what to add.

⚠ **The two seed differently, and the difference is a rule rather than an accident.** Three
archive collections seed with real targets; the YouTube row seeds **empty**. §10 says Loomarr
*"never recommends YouTube content itself"* — the operator brings their own playlist — so a
seeded YouTube target would be Loomarr making exactly the recommendation that sentence forbids.
The mock draws the same split: a YouTube row present but reading *"Bring your own playlist"* with
the stat *"no playlist yet"*. An empty row is an invitation; a filled one is an endorsement.

⚠ **Seeding a source downloads NOTHING.** A row records that a source exists and is allowed;
fetching is the pull's approval gate or a deliberate per-result queue. This is the same promise
the Add-a-source copy makes, and it is what makes seeding safe to do on the operator's behalf.

**The three collections, each VERIFIED against the live archive.org API rather than guessed**
(2026-08-03) — identifier, human-readable label, and item count at capture:

| identifier | label | movies |
| --- | --- | --- |
| `classic_tv_commercials` | Classic TV Commercials | 7,985 |
| `vhscommercials` | Commercials From The Vault | 17,953 |
| `tv_ads` | TV Ads | 2,951 |

⚠ **Every row carries a human-readable `label`**, not the bare identifier. `vhscommercials` is not
a name an operator recognises, and the Sources row renders the label with the target beneath it.

⚠ **The count is THREE, not the mock's "11 curated collections".** That number is fixture prose in
the prototype — there is no list behind it in code or in this doc. Five plausible identifiers were
checked and returned **zero items** (`classic_tv_ads`, `televisionads`, `vintage_tv_commercials`,
`tvcommercials`, `commercialsandbumpers`); the large collections that DO exist — `mirrortube`
(1.2M mirrored YouTube videos), `television` (735K), `vhsvault` (116K) — are general video, and
seeding them would point auto-fetch at arbitrary long-form content for a catalog meant for
30-second breaks. Three verified beats eleven invented. Expanding the list means verifying more,
not padding to a number.

⚠ **All three declare NO licence, and the seed records that honestly** — `license` stays empty,
which this section already defines as UNKNOWN and never "public domain". ~92% of archive items
declare nothing (667 of 8362 measured in `classic_tv_commercials`), so absence is the norm and
carries no permission. The row renders no licence chip rather than a reassuring one.

This **supersedes the read-model/registry asymmetry** V28 and V33 established — recorded here
rather than quietly replaced, because the superseded rule was load-bearing and its reasoning
still applies to the thing that replaces it. The old rule said: the folder is *derived from
configuration* so its switch is a setting, remote collections are *rows* so theirs is a column,
"and that asymmetry is deliberate and is why one source never appears twice."

⚠ **Two properties that asymmetry protected are NOT optional, and the flat list must carry them
itself:**

1. **"Not configured" stays expressible.** The derived rows could say *"you could set up a
   drop-folder but have not"* — a table of things-that-exist cannot say that, and it is §10's
   own answer to *"why is my catalog empty?"*, the most important question this tab answers. So
   the flat list keeps **`configured`** as a per-row fact, and the config-backed kinds
   (`folder`, `library`) are **always present as rows even when unset**. They are not created by
   an operator and cannot be removed; their target is the setting's value, blank when unset.
2. ~~**No source appears twice.** The two config-backed kinds are **singletons**.~~
   ⚠ **REVERSED in V38c — see "Many folders, many libraries" below.** The concern was real and
   survives; the singleton was the wrong instrument for it.

**Where the switch lives, now that both are rows.** A source's `enabled` is a column on every
row. For `folder` the column is the projection of `filler.source.folder.enabled` — the setting
remains the source of truth, so an operator flipping it in Settings and flipping it here are the
same act, and there is still no precedence rule to write. ⚠ **`library` remains switchless** for
the reason above: nothing scans a library, so its toggle would change nothing. Flattening the
list does not create work for a switch to do.

**Kinds.** `folder` · `library` · `archive` · `youtube`. Each declares whether it is
*searchable* (archive today), *fetchable*, and whether an operator may add or remove it (`folder`
and `library` are neither). ⚠ **`packs` — the dizqueTV/Tunarr-wiki bumper packs — is deliberately
NOT a kind yet.** There is no pack index to read: no URL, no manifest, no fetcher. A row for it
would be a control that dims and changes nothing, which this section forbids two paragraphs up.
It returns when there is something real behind it.

⚠ **A source's licence stays OFF the wire** (V37 decision). It is stored per source, but §6.3
measured ~92% of archive.org items declaring none, and a badge that is absent for nine rows in
ten teaches an operator to read absence as "fine" rather than "unknown". The stored value is
audit, not UI.

### Two folders, one pipeline (V38c)

**`FILLER_WATCH_DIR` (`filler.watch_dir`) — the watch folder.** Where clips arrive: downloads land
here, and an operator drops files in here. Loomarr **drains** it. Defaults to `<clip folder>/_watch`
so a zero-config install has one without the operator mounting a second volume.

**`FILLER_DIR` (`filler.dir`) — the clip folder.** Loomarr's own store. Every clip lives as
`<hash>.<ext>` with `<hash>.info.json` beside it, sharded two levels (`a3/f9/<hash>.mp4`).

**The two paths are one immutable generation layout, not independently hot-applied strings.** A
catalog row stores a path relative to the clip folder, so changing the root changes the meaning of
every row. The watch folder is coupled to it because every sync drains arrivals from watch into
root. The composition root therefore resolves both once per application generation and every
filesystem consumer receives that same applied layout. Saving a different value persists the
desired layout and marks the key restart-required; it does not make media serving jump ahead of the
pipeline, nor make a newly-saved watch folder drain into the old clip root.

The layout retains two clip-root spellings deliberately: Loomarr resolves filesystem aliases for
safe local traversal, while Tunarr registration receives the operator-configured shared-volume
spelling. Resolving a symlink before registration can manufacture a path that exists only in
Loomarr's container namespace. Every registered folder source is identity-checked against the
local traversal root in both directions before intake; aliases, ancestors, and shard descendants
are skipped rather than allowed to re-file or delete the live catalog.

**Changing the layout does not move data.** Loomarr neither copies the old clip library to the new
root nor removes the old files. The operator makes the desired mount/path available, deliberately
moves or copies data if wanted, uses the Filler connection Test to prove both desired directories
are readable and writable, and restarts. The new generation reconciles the catalog against that
root. A missing root can be materialized; a later create/scan failure is visible and never becomes
a successful empty scan that prunes the last known catalog. If an existing path ancestor cannot be
inspected far enough to prove the watch and clip trees do not alias, the new generation fails
closed before filler starts — running destructive intake on an unverifiable topology is unsafe.
An accessible empty root is an intentional empty library selection and reconciles as such.

⚠ **The clip folder keeps the EXISTING key rather than gaining a `filler.clip_dir` twin.**
`filler.dir` has always meant "where the clips are", which is exactly what the clip folder is —
its meaning did not change, only its layout did. Minting a new key would mean migrating every
reader, retiring an identifier, and leaving two settings whose difference an operator would have
to learn. The new concept is the watch folder, so the watch folder is what gets a new key.

**Every source uses the same plumbing — there are no divergent paths.** YouTube, Internet Archive
and a hand-dropped file all take one route:

1. **Arrive** in the watch folder (downloader writes there; operator copies there).
2. **Hash** the file — the sparse content hash below.
3. **Move** it into the clip folder as `<hash>.<ext>`. Already present ⇒ it is a duplicate; the
   arriving copy is discarded rather than catalogued twice.
4. **Write the sidecar** beside it, carrying tags and provenance.
5. **Catalogue** the clip.

**A completed download closes that whole loop (V56).** The ingest job does not stop after writing
the watch folder and wait up to fifteen minutes for an unrelated scan. Before its terminal success
event, it runs the ordinary catalog sync and nudges the durable ingest pipeline once, so the new
held clips are already visible in Incoming. The terminal event therefore invalidates the filler
status, Incoming queue and catalog reads. A sync failure is reported as an ingest failure even when
the bytes downloaded successfully: "on disk but invisible to Loomarr" is not a successful
acquisition from the operator's point of view, and the scheduled sync/pipeline remain the retry.

⚠ **The original filename is preserved IN THE SIDECAR before the rename**, and this is
load-bearing rather than sentimental. §10's grounding rule accepts an era only when the year
appears literally in the clip's text signals — and the filename is one of them
(`Frosted Flakes 1993.mp4`). Renaming to a hash without capturing the name first would destroy
that signal permanently, so every clip whose era came from its filename would become ungrounded.
The tagger reads `originalName` from the sidecar instead of from the path.

⚠ **Loomarr rearranges only its OWN clip folder.** The watch folder is drained, never
reorganised; nothing outside those two directories is touched. §10's promise — Loomarr never
deletes an operator's media — is unchanged: a file moves from watch to clip, it is not destroyed.

⚠ **Why moved rather than copied.** A watch folder that never empties is a second copy of the
whole library, and the operator would have to tidy it by hand forever. Draining is what makes it
a *watch* folder.

### Clip identity is a content hash (V38c — the third identity change)

**A clip is identified by a hash of its bytes; its path is data, not identity.** Both are stored,
each doing what it is good at:

- **`id` = sparse content hash** — the first 64 KB, the last 64 KB, and the file size. Two seeks
  rather than a full read, so hashing a 200 MB compilation costs about what a 2 MB clip does.
- **`path` (+ its folder) = where the file lives** — human-readable in logs and the UI, the thing
  playout opens, and the **filename the tagger reads for era and brand**. Identity moving off it
  changes nothing about that signal.

**Why identity had to move.** V38c allows many watched folders (below), and a path is only unique
*within* its folder — two folders each holding `ads/coke.mp4` produced the same identity, and one
silently overwrote the other. Prefixing with a folder id would have fixed the collision; hashing
fixes it **and** answers the question a prefix cannot: *is this the same advert?*

**Duplicates are not saved.** The same file found in two folders is catalogued **once**, first
scan wins; the second is skipped. ⚠ **This is only safe because identity is the hash.** If the
operator deletes the winning copy, the next scan finds the survivor, computes the *same* id, and
re-catalogues it — the clip returns **with its tags intact**. Under path identity that would have
been a different clip and the tags would be gone.

⚠ **Three caveats, written down rather than discovered:**

1. **A sparse hash is a heuristic, not a cryptographic guarantee.** Two files could share a size,
   a head and a tail while differing in the middle — in practice a truncated duplicate or a
   deliberately constructed file. The consequence is one clip shadowing another in the catalog,
   which a re-scan does not fix by itself. Tolerable for filler (clips are a synced cache, and
   the operator can delete the shadowing file), and stated here because a silent shadow is worse
   than a documented one.
2. **A re-encoded or trimmed file is a DIFFERENT content identity.** A file changed outside
   Loomarr and re-entered through the watch folder is a new clip and is tagged again. A transform
   Loomarr performs inside the ingest pipeline is different: it computes the transformed bytes'
   hash, files them at that hash's canonical path, and atomically re-keys the catalog row plus its
   tags, pipeline state and lineage references. Display metadata and provenance travel with that
   replacement; the old hash must never remain as the name of bytes it no longer identifies.
   This distinction is load-bearing: rewriting a hash-addressed path in place makes the next scan
   discover a new identity with only the hash-shaped filename left as its title.

   **Legacy mismatches repair themselves at sync.** V51b briefly rewrote a clip in place while
   leaving the old hash in its path, so a later scan could quite correctly identify hash B at a
   filename whose stem was hash A. The legacy name may be the old 40-character digest or the
   current 64-character content id; both are recognisably machine-generated. When the new value is
   a full content identity and the old stem is either hash shape, sync files
   the bytes at B's canonical shard path without overwriting anything, carries the sidecar, and
   removes A's stale link only after B exists. It also seeds a missing sidecar from the durable
   catalog name before moving, so a human title that still survives in the database does not turn
   into a hash on the following scan. If that name was already lost too, sync may replace the hash
   only with a conservative display name assembled from durable grounded facts (brand/category and
   era). With no such facts it uses the explicitly neutral **“Untitled commercial”** (or the
   corresponding known kind), because displaying an implementation hash as if it were a title is
   not more truthful. This display-name repair also runs when the file is already at its canonical
   path: a stale sidecar name must not keep resurrecting the hash on every sync. An arbitrary
   operator filename is never "repaired" merely for being non-canonical; this is a narrowly
   recognisable old Loomarr state, not a folder tidy.
3. **The migration DROPS the catalog** (`00033`), because ids cannot be recomputed without
   reading every file, and a migration that does I/O over an operator's whole media library is
   not a migration. Clips are a synced cache and the next scan repopulates paths.

   ⚠ **Tags are recoverable, play counts and pins are not.** Sidecars (below) carry era, audience,
   category and confidence back into the catalog on the next scan — which is most of what an
   operator typed. Play counts and channel pins live only in the database and do not survive.
   ⚠ **The loss is surfaced in the app**, not left to be noticed: an operator whose pins vanished
   must be told why, for the same reason §10 requires an auto-fetch limit to report itself rather
   than silently doing nothing.

   ⚠ **Sidecar recovery only helps installs that HAVE sidecars** — i.e. clips Loomarr downloaded,
   plus anything tagged after V38c ships. A pre-V38c install that hand-dropped and hand-tagged its
   whole library has none, and loses those tags outright. That is the case the warning is for.

### Sidecars: metadata travels with the clip (V38c)

A **sidecar** is a small JSON file beside a clip — `ads/coke-1985.mp4` and
`ads/coke-1985.info.json`. `clipfetch` has always written one at download (yt-dlp's
`--write-info-json` shape: title, description, upload date, source URL, licence) and Loomarr has
only ever READ it.

**Loomarr now writes to it too**, recording what it worked out: era, audience, category, kind and
the confidence score. So the metadata **travels with the file**. Reset the database, move the
folder to another install, or take migration `00033` above, and the tagging comes back on the next
scan instead of being retyped.

⚠ **JSON, not `.nfo`** (maintainer, 2026-08-02) — considered, because `.nfo` is the convention in
exactly this ecosystem (Kodi/Emby/Jellyfin/*arr) and an operator would recognise it. Three things
decided it: we would be *extending a file that already exists* rather than adding a format;
`.nfo`'s schema is `<movie>`/`<episode>` and has no element for era-as-decade, audience or a
confidence score, so we would be inventing custom tags inside someone else's schema — compliance
in appearance only; and **nothing consumes it**, because §10 took the media server out of the
filler path, so the interoperability argument that justifies `.nfo` elsewhere does not apply here.
If filler ever lives in a scraped media library, that changes and `.nfo` becomes the better bet.

⚠ **This is the first time Loomarr writes into the operator's media folder, and the promise it
must not break is narrower than "never write".** §10's existing rule is that Loomarr never
*deletes* an operator's media. A sidecar honours that — it adds a file, never removes or rewrites
one the operator authored. **Loomarr may only ever create or update `*.info.json`**; the media
files themselves stay byte-for-byte untouched.

⚠ **The sidecar's `"fetchedBy": "loomarr"` field is portable provenance, not admission
authority (V65).** V38b decided "downloaded vs hand-dropped" from sidecar existence; V38c moved
that inference to an explicit field. Both still fail open when a completed download loses, cannot
write, or cannot parse its sidecar: the catalog can mistake Loomarr's bytes for an operator's
decision and file them immediately. V65 therefore moves the held/filed authority to the durable
acquisition manifest below. The sidecar continues to carry source and run identity when files move
between installations, but its absence or corruption can never authorize playback.

### Download manifests are the quarantine authority (V65)

Every Loomarr acquisition publishes through one deep manifest boundary shared by the Archive and
yt-dlp adapters. A downloader returns an exact bounded set of completed media outputs; it does not
decide catalog ownership. For each output the manifest records the acquisition and registered-source
identities, provider, source URL, media and sidecar paths, byte length, full SHA-256 digest,
completion time, and lifecycle state. The application persists that set before making the files
eligible for ordinary intake. Counts are a projection of these records, never a substitute for
them.

V66 additionally carries the registered provider's exact remote item id from candidate selection
through the target and manifest. A consumed manifest is therefore the acquisition planner's
`already_catalogued` high-water mark; staged, published, and repair manifests are `already_queued`.
This avoids reconstructing identity from a mutable URL or downloaded filename.

Downloaded bytes remain under a run-owned hidden staging directory until their manifest is durable.
Publication records the intended watch-folder path before the rename, so recovery can distinguish
"not published", "published but not acknowledged", and "consumed by intake" without sweeping or
guessing. A manifest row is resolved by its exact path and digest; a symlink, path escape, size or
digest substitution is a held repair condition, never a match. The row is retained after intake and
binds the resulting clip hash, so a crash between moving the media, writing portable provenance,
catalog sync, and pipeline enrolment cannot turn the next scan into an operator drop.

The catalog's ownership question is therefore: **does durable acquisition state claim these exact
bytes?** If yes, the clip is held until the normal semantic admission gate or an explicit operator
decision releases it. Missing, malformed, replaced, truncated, symlinked, or unwritable sidecar
state cannot weaken that answer. If durable state cannot yet prove the bytes, the run reports a
bounded repair reason and the media remains quarantined; Loomarr never deletes it merely because
publication or recovery failed.

Operator drops preserve their existing deliberate policy. Intake never claims unmanifested files
by scanning a directory, and a copied third-party sidecar cannot create a durable acquisition row.
Pre-V65 downloads with only the old portable marker remain held as before; pre-V65 and manual files
with neither signal remain honest legacy/operator content rather than being retroactively claimed.
Startup recovery is idempotent: it validates each nonterminal manifest, completes a safe pending
publication or reports the exact repair reason, and leaves terminal rows alone. Archive and yt-dlp
use this same contract; provider-specific code ends at producing the bounded output set.

Provider deduplication advances only after the exact output manifests are durable. A manifest
retains the provider archive entry needed to replay that commit; startup recovery must make that
entry durable before publishing the corresponding staged media. An archive failure retains the
owned staged bytes and a bounded error. Recovery never treats a failed or missing archive commit
as permission to publish, and a successful restart leaves later acquisition attempts deduplicated.
Only entries bound to reported, persisted outputs may advance the shared provider archive.

Recovery counts each manifest at most once per scan even when processing changes its update time.
Outstanding repair counts and a bounded most-recent reason remain available to readiness
independently of the recent-run history limit; newer successful runs do not hide unresolved repairs.

### Many folders, many libraries (V38c — reverses V37's singleton rule)

An operator may add **any number** of watched folders and media-server libraries, not one of each.
Commercials living in two places is an ordinary situation, and V37 gave it no expression at all.

⚠ **This reverses a rule V37 added one phase earlier, so the reasoning is recorded rather than
edited away.** V37 made `folder`/`library` singletons because V28/V33's superseded asymmetry had
protected "no source appears twice", and a flat list that let someone add a second folder beside
the derived one looked like exactly that double-listing.

**The concern was right; the instrument was wrong.** What must not happen is ONE folder appearing
as TWO rows — a stale row disagreeing with the setting about the same directory. Forbidding a
second *distinct* folder does not prevent that, and it forbids something legitimate. So:

- **Uniqueness is on the TARGET, not the kind.** One row per distinct path or library id. Adding
  a folder already listed is refused as a duplicate, which is the actual invariant.
- **`filler.dir` is the FIRST folder, not the folder.** It seeds a row on a fresh install so a
  zero-config install still has somewhere to drop files, and it remains the default the ingest
  path downloads into. It stops being the only one anything scans.
- **The scan walks every enabled folder row**, not just `filler.dir`. A folder that is switched
  off is not scanned — the same promise the switch already makes.

⚠ **"Not configured" still has to be expressible** (property 1 above, unchanged). A fresh install
shows its seeded folder row with a blank target and a `not configured` badge, because "you could
set up a drop-folder but have not" is §10's own answer to *"why is my catalog empty?"*.

### Per-source fetch overrides (V38c)

`filler.fetch.every` and `filler.fetch.max_per_run` gain **per-source overrides**. A busy archive
collection and a small playlist genuinely want different numbers, and one global figure serves
neither well.

The effective policy is server-owned and returned with every Source: stored override state,
effective interval, effective per-check count, and last successful check. The ordinary state is
**Use automatic-download defaults**. Advanced Source settings offer exactly three choices:
**Use automatic-download defaults**, **Use a different schedule**, and **Never download
automatically**. The custom choice exposes an interval and per-check count; resetting clears both
nullable overrides rather than copying the current global values. The sheet always summarizes the
effective result in plain language, including when the global default is Never but this Source has
its own schedule.

⚠ **Unset must be NULL, never 0.** `0` already means something for `fetch.every` — *never
auto-fetch this source* — so "inherit the global" cannot share that encoding. A column defaulting
to `0` would read as "every existing source is switched off", silently, on upgrade. That is the
`00026` mistake (a default chosen for new rows applied to old ones) and it is the thing to
sabotage-test here.

⚠ **The catalog and disk ceilings stay GLOBAL.** They bound the whole install — what the operator
is protecting is one disk, not one source — and a per-source disk cap would let four sources each
stay under their limit while together filling the volume.

Filler → Manage presents the global policy as **Automatic downloads**. The common control offers
Never, Every 6 hours, Every 12 hours, Daily, Weekly, and Custom; Custom reveals the duration editor.
The per-source count is visible beside it rather than hidden as an expert-only limit. The section
states the bounded consequence using the current number of enabled, configured remote Sources — for
example, “3 sources means at most 30 new clips per check,” while never promising more than the
internal 50-item provider-pass ceiling. Catalog/storage protection remains under Advanced and is
explained as a household-wide backstop. Environment-pinned values stay visibly locked through the
ordinary settings contract.

### What the Sources tab shows (V38c — the mock, read properly)

⚠ **V37 built this tab from the delta doc's SUMMARY rather than the mock's markup and JS**, which
is how it ended up structurally right and detailed wrong. The summary described the shape (flat
list, toggles, a search expander) and that shipped faithfully; the things only the source shows —
the kind badge, the greying, the combined stat line, the three-kind picker, the per-kind copy —
were never read. **A summary is not the source**, which is the same lesson the truncated-fetch
correction records one section up.

Per row: an on/off switch · a **kind badge** (fixed-width, colour-coded) · name + description ·
a lifecycle stat reading *"6 ready · 12 being checked · checked 2m ago"* · an optional Search
expander · an optional remove. Ready clips and the active Incoming conveyor are separate counts:
Incoming clips remain unable to play, but they must not disappear from the source that brought them in. A source with zero ready
clips and a non-empty Incoming queue therefore says *"0 ready · 12 being checked"*, never merely
*"0 clips"*. Being checked uses the same conveyor definition as the Incoming page: it includes a
compilation while Loomarr is preparing it, excludes a completed split proposal that has its own reel
row, and excludes retained terminal compilation containers that will never play. The count links to
Incoming from the source workspace. ⚠ **A disabled row is
GREYED** (the mock's `sv.opacity`), not merely badged — the switch's effect has to be visible at a
glance down a list.

**Source setup inherits the Installation geography (V68).** The ordinary Sources flow never asks
the operator to repeat their country and market for every row. A source with no explicit coverage
uses the current Installation geography when Loomarr decides whether it is ready; changing the
Installation geography therefore updates every inheriting source immediately rather than copying a
value into each row. A source whose real coverage differs may carry an explicit country and optional
market through an Advanced disclosure. The read model reports the effective geography, whether it
is inherited or overridden, and one closed readiness state (`ready`, `off`, `needs_location`,
`not_configured`, or `out_of_area`); counts and available actions use that server-owned state rather
than browser inference. Candidate-level geography remains a separate hard acquisition constraint,
and an inherited source value does not turn missing or conflicting candidate evidence into a match.
The same separation continues after admission: Installation geography and inherited effective source
coverage are not Clip evidence and are never copied into a Clip's location. Explicit item evidence,
an explicit registered-source coverage override, an operator correction, or the narrowly bounded cited
country projection defined below may populate Clip geography; the catalog leaves Location absent when
none of those facts exists.

**A config disclosure per row** (V38c), on the same shelf as the search and URL expanders the mock
already draws. It shows the source's target **read-only** and makes its *behaviour* editable —
enabled, and the fetch overrides above. ⚠ Re-pointing a source is deliberately NOT offered:
changing a folder's path orphans every clip it brought in, which stay attributed to a source that
no longer means the same thing. Remove and re-add makes that explicit rather than silent.

**Two status lines, and they are different.** `svcOnLine` on the tab header reads *"4 of 5 on"*.
The page header's pill — on **every** tab, with a live pulse — reads *"4 of 5 sources on · 9 clips
· last scan 2m ago"*. ⚠ The scan time is the load-bearing part: with auto-fetch running
unattended, *"is this thing actually running?"* is the question the header exists to answer, and
counting rows does not answer it.

⚠ **No Sync or AI-tag buttons in the header** (V38c). Both jobs run on schedules, so a manual
trigger asks the operator to do work the appliance is already doing; the mock draws the status
pill in that space instead. The capability is not lost — the Tasks page has Run-now for every
scheduled job — and the pill's "last scan" is what those buttons were really being used to check.

### Compilation splitting (V34)
V67 supersedes V34's assumption that every duration-quarantined source is a compilation and its
lossy drop tally. The V34 detector remains a shadow/proposal input while the V67 structure assessment
adds semantic source proof, complete timeline coverage, independent interval roles, and certified-slice
admission. The measured detector behavior below remains evidence; where it conflicts, V67 governs.

Discovery (V33) surfaces a source; ingest downloads it. But a large share of what discovery finds is a **compilation** — one file holding twenty or more commercials back to back. Ingested whole it is a single 15-minute "clip" the pod assembler can never place (`durationEligible` rejects anything far longer than a break); split blindly it is twenty files named `compilation_seg07` with no era, audience or category, which the ladder cannot place either. **Splitting and metadata are one phase because either alone produces unplaceable clips.** The pipeline, designed from measurement on six real compilations rather than reasoning (plan §6.4 — every number below names its method there):

1. **Triage.** A source with chapters splits for free (chapters are exposed without downloading the file). Rare in practice — 6 of 8 sampled sources had none — so this is an optimisation, not the mechanism.
2. **Coarse split.** ffmpeg's `blackdetect` + `silencedetect`, parsed in Go; segments under the **detection floor** become explicit discard intervals in the V67 plan rather than disappearing. That floor is `max(MinSegmentMs, filler.min_duration)` — the 3s sliver floor and the catalog floor, whichever binds (10s on a default install). ⚠ `max()`, never replacement: `filler.min_duration` is settable to `0s`, and a 400ms fade artefact must still be omitted there. The two numbers are **one floor on purpose** — a segment the auto-confirm gate would admit and the scan boundary would then reject (§10 V40) is a clip cut out of a compilation and thrown away, work done to produce nothing. Scene-cut detectors (`scdet`, PySceneDetect) were measured and rejected: they fire on camera cuts *inside* an advert — the wrong granularity, not a tuning problem. Detection quality is a property of the **source**, not of any threshold (69–100% across the six compilations; two had genuinely absent boundaries no setting fixes).
3. **Rescue.** A segment far longer than a plausible advert means boundaries the A/V pass could not see; it goes to **transcript (whisper) + LLM** for cut points. ⚠ The LLM must return **exactly one entry when the transcript is a single advert** — without that instruction it invented cuts at suspiciously round 30/61/92s marks inside one 121s infomercial. With no runnable whisper (`INGEST_WHISPER_PATH`, §15) an over-long segment is not guessed at: it surfaces in the review as **unsplittable**.
4. **Independent role before enrichment (V67).** Each planned interval first receives its own closed
content role; a reel-wide `commercial` kind is never inherited. Only after the role and structure are
resolved may ordinary text/vision taxonomy enrichment add product, era, and audience facts. Era follows
the grounding rule above: persisted only when the year appears in the evidence, else carried as an
unconfirmed suggestion.
5. **Deterministic discard.** The same advert recurs across compilations. A dHash over frames sampled at 1/3fps — ~30 lines of pure Go over `ffmpeg -pix_fmt gray` output, no library, no cgo — separates a re-encoded duplicate from a different advert by a measured 25× margin (mean per-frame Hamming 1.1 vs 27.6–32.2), so any threshold in the teens works. A matched segment is discarded before classification: keeping another copy cannot improve the catalog, and asking a person to make that decision is queue-shaped busywork. A segment shorter than the existing `filler.min_duration` floor is discarded at the same seam — the scan boundary would reject the resulting clip immediately, so cutting and reviewing it can produce no usable outcome. **Neither discard deletes source media:** the composite row and file remain (§10 V45), so re-splitting with improved detection is the recovery path. Detection ambiguity is never a deterministic discard: an `unsplittable` or over-long span remains with the whole reel for review.
   **Installation language is also applied per detected segment before review.** A compilation is
   not one language: each span samples its own bounded audio window using the same detector and
   single installation choice as the ordinary language rung. A confident mismatch is omitted and
   counted in the proposal receipt; wordless, unknown, unavailable-backend and failed-backend
   outcomes remain reviewable. An unresolved span carries a stable reason — `unavailable`,
   `failed`, `inconclusive`, or `paused` — plus optional technical detail, so review can distinguish
   speech recognition that is not set up from a real attempt that failed or could not identify the
   language confidently. The work is cursor-persisted and bounded by the existing whisper
   allowance, and a proposal is not reviewable until every span has a terminal outcome. Every
   finished proposal also records the installation language it used, so review can identify a
   later preference change and offer an explicit recheck. Every confident mismatch remains in the
   proposal receipt with its exact interval, detected and
   expected languages, name, and a stable mismatch reason; it is not placed back in the editable
   candidate list. When every candidate is a confident mismatch, the split resolves automatically
   with a plain pipeline receipt instead of creating an empty human-review task. Confirmed
   children inherit known and wordless results so the post-confirm rung does not spend twice, but
   only when the confirmed interval exactly matches the persisted proposal. The server ignores
   language fields supplied by the confirm client; edited or merged intervals and
   checked-but-unknown children leave the field empty so the post-confirm rung remains a
   defence-in-depth retry.
6. **Review — required unless the result is unambiguous (V43).** Because detection quality is a property of the source, an uncertain result is confirmed by a human before anything enters the catalog; auto-accepting a 69% result puts 3-minute "commercials" into 30-second breaks. Detection runs as a **job** (minutes per file) producing a **persisted split proposal** (§5) — review can happen long after detection, and a restart must not lose it — and an unconfirmed proposal writes nothing until `POST /v1/filler/splits/{id}/confirm` (§7) commits a cut list.

   ⚠ **The review PLAYS each proposed cut, in place (V54).** It did not, for as long as it existed: measured 2026-08-12 on a 52-segment reel, the screen offered a name field, two mm:ss fields, Merge, Drop and Confirm, and **no media element at all** — an operator was asked whether a cut at 04:17 was right with nothing to see or hear. V54 A7 had already deleted the mock's "click to preview" caption for being false; this is the other half.

   A proposed segment has **no bytes of its own** until confirm writes them, so the preview is a **byte-range window of the parent composite** (`GET /v1/filler/media/{clipHash}`, range-served by `http.ServeContent`). That is the operational reason V45's keep-the-parent rule matters to an operator and not only to lineage: without the retained reel there is nothing to play. The route is `RoleMember`, the page is admin-gated, and the browser authenticates with the session cookie it already holds — **no new authorization surface**.

   **The review starts with the clips Loomarr found, not detector diagnostics (V68).** Every
   proposed segment receives one representative still extracted from its exact source-bound span.
   The still is ingested through the shared image service as member-visible, proposal-owned
   artwork; the proposal document retains only the content hash and exact span binding. Confirm,
   replacement, rewind, and expiry release the proposal's image references, after which the normal
   image garbage collector owns the bytes. A failed extraction is a terminal presentation outcome
   for that proposal and renders as **Preview unavailable** rather than a broken image or an
   invented frame. Re-detection is the retry path. No browser-visible filesystem path and no new
   image store exist.

   The proportional timeline is the primary overview. It remains one ordered list, scrolls rather
   than compressing a long reel into untappable slivers, and gives every segment its still, name,
   duration, time range, useful tags, language, and any plain-language reason it needs attention.
   Hover and keyboard focus expose the same lightweight summary without starting media. Click or
   Enter selects the segment, reveals its compact row, and opens the existing exact bounded player;
   at most one player is mounted, and changing selection, collapsing it, or leaving the page tears
   that player down. Fifty segments therefore mean fifty lazy still images and **one** possible
   range stream, never fifty video elements.

   The ordinary row vocabulary is **Rename**, **Adjust timing**, **Join next**, **Remove**, and
   **Keep clips**. Boundary confidence, detector evidence, transcripts, recognition diagnostics,
   and other pipeline terms remain available under **Details** instead of competing with the
   decision. The page explains once that keeping clips creates individual filler items while
   retaining the original recording for recovery. Editing, sub-second boundary preservation,
   exact-span playback, language recheck invalidation, and the confirmation authority remain
   unchanged; this is a review presentation and evidence-lifecycle change, not a new admission
   path.

   **Proposed names describe the exact segment instead of its storage order (V69).** Chapter titles
   remain source-authored names. Otherwise the already-budgeted split vision response may propose
   one short household-readable name from text it read in that segment's bounded frames; transcript
   rescue may do the same from the exact subsegment transcript. This adds no second model request.
   A proposed identity is accepted only when its identifying phrase occurs in that exact signal,
   after case and punctuation normalisation. Generic, unsupported, malformed, overlong, wordless,
   and abstaining answers are rejected independently of valid tags or role evidence. They fall back
   to **Clip N from &lt;readable recording name&gt;**, or **Clip N from this recording** when the parent
   name is opaque. Provider failure follows the same non-blocking fallback path.

   The proposal records `source-authored`, `model-proposed`, `fallback`, or `operator-edited` plus a
   bounded evidence excerpt for a model proposal. That provenance stays inside the existing
   collapsed name-edit disclosure (and the selected-clip Details area once the planned shared
   right-side sheet owns inspection), never as another always-visible row. The server—not the confirm
   body—is authoritative: an unchanged name on the same exact span recovers
   its persisted provenance; a rename, boundary edit, or merge keeps the operator's chosen display
   name, clears machine evidence, and becomes operator-edited. The chosen name reaches the confirmed
   child. Naming is descriptive enrichment only and is deliberately absent from role, language,
   suitability, boundary-confidence, and admission inputs. Split review performs no web search and
   does not require a model to finish.

   **One selected clip uses the shared inspection sheet (V70).** The split page keeps the
   proportional filmstrip, one compact ordered clip list, reel-level explanation, and **Keep clips**
   action stable while a clip is inspected. Activating either a filmstrip item or its compact row
   opens the existing application Sheet from the right on desktop and full width on a narrow screen.
   That sheet owns the one exact bounded autoplay preview, name and timing edits, era decision,
   **Join next**, **Remove**, and progressive naming, language, transcript, recognition, and cut
   details. Previous/next controls move through a long reel without closing the sheet. Selection is
   transient local state—not a route or query parameter—and closing restores focus to the exact
   filmstrip item or row that opened it. Changing selection or closing the sheet unmounts the prior
   player, so a 50-clip reel still mounts at most one video and editing never expands or reflows the
   overview. Confirmation remains a reel-level action outside the sheet and sends the same server
   payload; this changes presentation, not split, language, naming, or admission authority.

   ⚠ **The player is clamped to `[startMs, endMs]` and reports the SEGMENT's length, never the reel's.** A 30-second cut of a 22-minute recording reads `0:04 / 0:30`. Handing the readout the reel's own numbers would present the whole recording as if it were the clip, which is precisely what makes a preview useless for judging one cut. One preview is open at a time and collapsing **unmounts** the element — otherwise every row the operator has ever clicked holds a range request open against a 20-minute file.

   ⚠ **This was "not optional, ever" until V43, and the blanket rule was over-applied.** Boundary
   confidence can safely automate the mechanical creation of well-supported child clips while
   uncertain cuts remain for review. That automation does not publish the children: each child
   still traverses conditioning, classification, configured safety/playback checks, and terminal readiness.

   **`filler-split` is a scheduled job** (on by default). It proposes splits for over-long catalog clips rather than waiting for a click, so proposals are ready when the operator looks instead of costing minutes of waiting once they do.

   **Auto-confirm is a separate switch** (`filler.autosplit.enabled`, default **ON** — maintainer decision, V51b: the gate exists to admit *confident* reels, and off-by-default meant every compilation waited for a click the design says should be unnecessary), gated on `filler.autosplit.min_confidence` — deliberately NOT reusing the auto-file threshold. The failure modes differ in kind: a mis-*tagged* clip plays in the wrong break, a mis-*cut* clip plays half an advert. One dial would force the stricter case to govern both.

   ⚠ **The gate is PER SEGMENT (V54). All-or-nothing is retired.** A segment is cut and filed when it passes every refusal *and* its boundary confidence clears `filler.autosplit.min_confidence`; the rest stay behind in a shrunken proposal. A reel of 52 becomes 47 clips and 5 cuts to review, rather than 52 cuts to review.

   ⚠ **Order matters, and it is the whole safety argument: REFUSALS FIRST, ABSOLUTELY — then the threshold.** A segment carrying `SuggestedEra > 0`, `unsplittable`, a duplicate flag, an over-long span or a sub-floor span is refused **at any score**. Confidence chooses only among segments that already pass every refusal: it can hold a qualifying segment back, never let a refused one through. `boundaryScore` cannot even see `SuggestedEra`, `Era`, `Looked`, `Category` or `Tags` — a tag fact is not in its scope, so it cannot move the number. That is what keeps `autosplit.go`'s objection true: nothing here launders a refusal into a score.

   ⚠ **This was ALL-OR-NOTHING until V54, and the old rationale is worth stating rather than deleting.** It ran: *"a badly-split reel is not uniformly slightly-wrong; it has obvious tells… confirming the good segments and surfacing the rest would split one reel's decision across two places and hand the operator fragments to judge without the picture."* That was correct **while there was no per-segment evidence**. Splitting a decision arbitrarily is indeed worse than making it once. But the rule's cost was total: one doubtful segment in 52 sent all 52 back, so the operator's work never shrank and — measured 2026-08-11 — **~50 reels sat parked with none ever auto-confirmed**. With boundary evidence per cut, keeping five back is not splitting a decision arbitrarily; it is **routing by evidence**, which is what every other rung in this pipeline already does. The filmstrip still shows the whole picture, and the parent recording is still there to play (V45), so the operator judging those five has more context than the old rule assumed, not less.

   ⚠ **A confirmed segment is not airable.** Confirm writes the rendered child and lineage, then the
   child traverses the complete pipeline while held. Boundary confidence decides whether a cut may
   be created unattended; it is not evidence that the resulting content is technically playable or
   eligible for a pod. Only the terminal-ready transaction may release it.

   ⚠ **The `filler.min_duration` floor is no longer one of those conditions, because it is enforced earlier (V54).** It used to be, and that is the reason auto-split could never fire: a real commercial compilation is *made of* sub-floor material. Measured 2026-08-11 on an 82-segment archive.org reel, **39 segments sat under the 10s floor**, the shortest 3.1s — station IDs and inter-ad bumpers. `AutoConfirmable` returns on the first failing segment, so `RejectTooShort` sank the reel before the grounding checks at the bottom of the loop were ever reached, and the V54 grounder below could not have changed the outcome no matter how well it worked. Those fragments are now dropped at **detection** (step 2 above), where a fragment the scan boundary would refuse anyway costs nothing to discard. `RejectTooShort` stays in the gate as defence-in-depth for hand-edited proposals and for those detected before V54; it is no longer a reason a freshly-detected reel sinks.

   **Boundary confidence — what routes a cut (V54).** A score of 0–100 per segment, answering *did we cut in the right place?* ⚠ **Distinct from `Clip.Confidence`**, which answers *do we know what this is?* Different question, different evidence, different field; the split gate reads the first and never writes the second. `CONTEXT.md` carries both terms precisely because "confidence" unqualified is now ambiguous.

   It is a **ceiling ladder**, in the shape §10's tagger already uses — the best evidence a boundary has sets its ceiling, and segment-level facts may only lower it:

   | Evidence for one boundary | Ceiling | Basis |
   | --- | --- | --- |
   | a chapter marker, or the reel's own start/end | 100 | declared, not inferred |
   | black **and** silence agreed on it | 90 | **measured** — 9 of 12 fades corroborated |
   | one detector only | 65 | **measured** — the other 3 of 12 |
   | the transcript rescue alone | 50 | **measured** — ±2–3s timing |
   | truncated (an overlap moved this edge) | 40 | asserted |

   **Within one boundary the best evidence wins. Across a segment's two boundaries the WORST wins** — a cut is only as trustworthy as its weaker end. Segment facts then cap it: over-long 50, `unsplittable` 20.

   ⚠ **Black is not ranked above silence.** Nothing measures that, and ranking them would invent exactly the number this design exists to avoid. Which detector fired survives in the evidence token, so the UI can still say "silence only".

   ⚠ **The duration prior is DEMOTED, and this deviates from V45's original sketch.** That sketch made "30/60s ±2s" a confidence input. Under a ceiling ladder a corroborating prior has no legal move, and capping on "not a standard slot" would flag half a good reel: measured 2026-08-11, **39 of 82 segments were sub-10s bumpers and every one was correctly cut**. Duration survives only as the over-long cap.

   ⚠ **The rescue's single-span confirmation REMOVES a cap rather than adding points** — the only legal move a corroboration has here. When the LLM returns exactly one span it is saying "this is one advert" (the measured 121s infomercial), which is the fact that defeats "over-long means a missed boundary".

   **Not scored, deliberately:** the dHash distance and `dupOf` (catalog membership is a different question); vision's `looked`/`category` (tag grounding — their exclusion is the clearest illustration of the boundary/tag split).

### Splitting keys on identity, not location (V51a)

⚠ **Compilation splitting could not commit a single reel between V38c and V51a, and nothing said
so.** The persisted proposal stored `clip.Path` (the sharded location `a3/f9/<hash>.mp4`) in a
field called `clipPath`, and `Confirm` handed that string to `GetClip`, which is keyed
`WHERE hash = ?`. The lookup never matched, so every confirm returned *"compilation … no longer in
the catalog"* for a clip sitting in the catalog. An operator could run detection, open a
41-segment reel, edit the cut list — and never commit it. The Review-cuts button was a dead end.

Underneath it sat a second defect that would have been hit the moment the first was fixed: each
confirmed segment was upserted with **no `Hash` at all**, and `UpsertClip` is
`ON CONFLICT(hash) DO UPDATE`, so every segment overwrote the last and a whole reel collapsed into
**one** catalog row.

Three rules come out of this, and they are the reason the section exists rather than just a
changelog line:

1. **The proposal carries the compilation's HASH (`clipHash` on the wire), and its file location is
   DERIVED** — `Propose` already resolves the file as `join(dropDir, clip.Path)` from the row, and
   `Confirm` now does the same instead of rebuilding a path from an identity. One identity, one
   derivation, nothing to disagree.
2. **A segment is hashed the moment it is cut.** Cuts are written to a temp file outside the clip
   folder, hashed with `ClipID`, and filed at `ClipRelPath(hash, ext)` — the same shape intake
   uses. This retires `uniqueClipPath`/`sanitizeClipName`: under content addressing there is no
   display name to make filesystem-safe and no collision to break, because two cuts with identical
   bytes *are* one clip.
3. **`dedup`'s self-exclusion compares identities.** It took a parameter named `clipPath`, tested
   it against `c.Path`, and was called with the hash — so the guard never fired and every segment
   was compared against the file it was cut from. Segments resembling their parent came back
   flagged as duplicates, which is noise in the review and enough to make `AutoConfirmable` reject
   a sound reel.

⚠ **The reason all three survived is one fixture.** The splitter's test store keyed clips by
`Path` while the real store keys by `hash`, and the app-level fixture set `Hash` and `Path` to the
*same string*. A double that indexes differently from the thing it stands in for cannot see key
confusion — it answers a question production never asks. Both fixtures are now hash-keyed with
identity and location deliberately distinct, `seedCompilation` returns the hash so no test can
re-derive it, and the confirm round trip is exercised against a real store for the first time.
This is the same lesson `internal/store/conformance_filler.go` already records; it had simply
never been applied to the split path.

### The tagging score had no writer (V51a; publication use retired)

⚠ **`clips.confidence` was 0 for every clip in every catalog that has ever existed.**
`TagSuggestion.Score` computed the grounding-capped number and the historical tagger used it to
decide filing — then discarded it. `UpsertClip` inserts a
literal 0 and correctly omits the column from its `DO UPDATE`, so nothing ever persisted a score.
The old publication shortcut worked; the diagnostic number an operator needed was absent, and the
Incoming meter (which correctly renders nothing at 0, because 0 means *never scored*) never
appeared.

`SetClipConfidence` is now that writer — path-keyed, beside `SetClipLanguage` and
`SetClipTranscript`, and the score written is always `Score`'s output, never the model's own
self-assessment, so the grounding cap keeps its teeth.

⚠ **The store's conformance case passed throughout**, because it seeded a value through
`UpsertClip`'s INSERT and asserted the round trip and the `DO UPDATE` omission — all true, and all
true of a column with no producer. A column can round-trip perfectly and still be dead. The case
now exercises the writer itself.

### Ingest is a pipeline, and it is watchable (V51b)

Filler grew one cron job per capability: sync, fetch, language, split, transcribe, vision, and
taxonomy repair. Each **sweeps the whole
catalog looking for its own kind of work**, on its own schedule, knowing nothing about what the
others have done to a clip.

Two consequences, and the second is the one an operator feels:

1. **A clip's journey is invisible.** Download forty commercials and the queue says "Downloaded
   and waiting to be checked" for up to an hour, because language runs at :30, vision at :50, and
   tagging on its own cron. Nothing reports which stage a clip is in, or that it is being worked
   on at all. The system is working and looks broken.
2. **Nothing owns the order.** Tagging can run before transcription, so it grounds against a
   transcript that does not exist yet and scores the clip low; the transcribe job fills it in
   later and nothing re-tags. Cheap work and expensive work compete for the same hour with no
   budget between them.

V51b replaces the seven sweeps with **one ordered per-clip pipeline** and one driver job.

**The readiness stages, in order:** `probe → transcode → split → screen → language → transcribe →
vision → score`. Each stage answers two questions separately — *does this stage apply to this Clip,
in this install?* (no exec, re-evaluated while the Clip is on the conveyor) and *do the work*.
Missing optional capability records a skipped rung and does not block Ready. A later capability
change enriches already-Ready Clips through the post-ready progressive-enrichment phase; it never
rewinds readiness or holds playable media (#1251). Preparation and post-ready enrichment share the
one `filler-pipeline` driver and task row, but retain different authority: preparation may establish
Ready, while enrichment may only improve descriptive facts and suggestions.

**Progressive enrichment is a separate-authority, per-axis loop inside the one pipeline driver
(#1251).** Readiness answers whether exact
bytes may play; enrichment incrementally improves their descriptive matching metadata. The axes are
kind, era, brand, target audience, geography, language, and the controlled taxonomy dimensions product,
format, seasonal, audience cue, and presentation. A missing descriptive answer never holds,
unpublishes, duplicates, or removes a Ready Clip, and enrichment never creates an Incoming
Needs-help task.

One provider-neutral enrichment module owns the small interface: given a Clip's current axis states,
available capabilities, and current producer/taxonomy versions, it returns only the bounded work
still needed; given one grounded result, it applies the evidence-rank and idempotency rules and
returns the accepted projection. Provider selection, retries, budget deferral, evidence precedence,
and version invalidation stay behind that interface. The readiness conveyor, catalog UI, and
scheduler do not reimplement them.

Each axis persists `missing`, `complete`, `unsupported`, or `stale` independently, together with the
accepted value and its exact evidence kind/reference, Evidence rank, confidence, producer/version,
taxonomy version where applicable, and observation time. `complete` may carry no value after an
applicable pass found none; this prevents an honest absence from becoming endless work. Geography is
the exception: machine-produced `complete` requires a non-empty location because an empty terminal
geography falsely looks resolved while the hard household boundary still excludes the Clip. An
unsuccessful geography pass leaves that axis absent rather than persisting a terminal empty value.
An operator may still deliberately clear geography; that higher-ranked empty correction remains
terminal and is never reopened by research.
`unsupported` wakes when a capability becomes available, while `stale` is re-evaluated after a
relevant producer or vocabulary version changes. Backoff, budget waits, and provider failures are
queue state rather than invented semantic states.

Evidence precedence is closed and deterministic: an operator correction outranks exact item
metadata or content evidence, which outranks a trusted provider/network mapping, which outranks a
registered-source default, which outranks a weak inference. Confidence breaks ties only inside one
rank. An explicit broadcast date therefore survives a later guess; an upload date is never an era;
item evidence beats source geography; and a model cannot replace a stronger fact merely by reporting
high confidence. Target audience and presentation remain descriptions, not Airworthiness: a
child-directed or animated Clip with unknown Airworthiness remains ineligible for kids/family Pods.

The first pass is always local and free. It reads the preserved original title, provider
description, original filename, durable source/item provenance, explicit dates, registered-source
geography, and a small versioned set of trusted provider/network and controlled brand/product
mappings. Only unresolved axes are eligible for text, transcript, or vision work, using the
household's already-enabled AI capability and existing egress/cost controls. There is no second
Filler-specific enable switch. Ready Clips wake automatically when a relevant capability or
taxonomy version appears; the manual whole-catalog tagging action and the clip-wide `ai_tagged` /
`vision_tagged` authority retire once this loop owns catch-up.

The text-model pass is automatic when the household has a configured text provider. It groups at
most eight eligible Clips into one bounded JSON-mode call, keys every returned item to the supplied
Clip hash, names only axes whose accepted value is still empty, and serves the current
operator-editable taxonomy once in the batch prompt. Each returned item is validated and persisted
independently, so one malformed item cannot discard valid siblings; a whole-request failure leaves
the batch unstamped for a later retry. Returned taxonomy terms are
resolve-or-dropped against that same vocabulary, a brand is accepted only when it occurs literally
in the supplied item/transcript/visible-text signals, and the pass never asks a text model to infer
era or geography. Its durable pass identity binds the prompt version, branded provider, selected
model, and a digest of the live taxonomy. Changing any of those wakes the relevant Clips; replaying
the same identity does not pay for another call. An absent provider does not query, stamp, fail, or
create a visible problem. Operator evidence, including an intentional empty correction, is never
reopened by automation.

**Context research is a cited-context path, not another evidence rank.** When a publicly acquired
Clip still lacks verified era or geography after the ordinary passes, Loomarr may use its public
item title and description to build a deterministic bounded search plan. One retrieval module fans
that plan out to documented fixed-host knowledge adapters, merges partial successes, de-duplicates
URLs, and caps the final evidence packet. The default adapters search English Wikipedia for
campaign/background context and Archive.org's metadata index for related historical items; exact
public source metadata may also contribute an attributable citation without another fetch. Each
adapter owns its query syntax, host validation, response limits, version and provider etiquette.
A configured text model may interpret only the merged packet; it receives no general web tool,
returns no fetch targets, and cannot introduce a citation Loomarr did not retrieve. Local paths,
private-library metadata, transcripts, and household data never become public lookup queries.
Arbitrary pages and consumer search interfaces are not fetched. YouTube-wide search is not a
default dependency because its official API requires a separately configured Google project/key;
the acquisition sidecar remains the exact YouTube-item evidence available without that credential.

A context report persists separately and records the Clip/input revision, retrieval adapter/version,
interpreting provider/model/prompt, completion time, confidence, explanation, and exact titled citation
URLs. Campaign-level context may render as, for example, `Likely 1970s · United States`. Era remains
descriptive and never becomes a scheduling fact from this report. Country has one deliberately narrow
automatic projection: an uppercase ISO alpha-2 country, confidence of at least 60, and at least one
citation from Loomarr's retrieved packet may populate an otherwise-empty Clip geography as `national`,
with inference-ranked evidence that names the report's adapter and cited IDs. It never invents a market,
overrides stronger item/source/operator evidence, or uses the household/source location as proof. Saving
a new report and its accepted country projection is atomic; a bounded catch-up pass applies already-
stored qualifying reports without re-searching or manual requeue. This changes only descriptive
scheduling metadata for an already-admitted Clip: it never changes readiness, Placement, Airworthiness,
or admission, and unknown/conflicting geography remains excluded by the hard boundary. The ordinary UI
stays quiet when no model, network, or useful result exists; evidence detail is progressive disclosure
in the exact-Clip panel. Commercial discovery adapters remain behind the same interface and require a
fresh terms/retention review before selection.

**General web search is an optional last resort, not a parallel research task (#1349).** The
ordinary **Clip details** settings task keeps credential-free structured research on by default and
offers one **Add web search** action. Brave Search is the recommended hosted provider; an
operator-supplied SearXNG endpoint is the self-hosted Advanced choice. Provider credentials are
installation-wide write-only settings, and the normal screen exposes only readiness, the current
month's request count and the last successful check. Provider replacement, the conservative monthly
ceiling, connection testing and removal stay under Advanced. Removing web search deletes its stored
credential but leaves existing cited reports intact; turning context research off suppresses future
lookups without changing Ready media or accepted metadata.

The plain-language **Where Loomarr looks** disclosure names the four structured sources Loomarr
searches: Wikidata, Wikipedia, Archive.org, and the Library of Congress. Its resting summary reports
how many are selected without repeating pipeline status. Each source is enabled by default and may
be disabled independently; the ordinary view does not expose adapter ordering, versions, or other
retrieval knobs. The main **Find missing details automatically** switch remains the master control.
Exact metadata from the Clip's already-selected source is not a search source and cannot be disabled
here. Disabling all structured sources is valid: configured web search may act as the only lookup,
while an installation without a web provider performs no external lookup and does not report a
background failure. An unconfigured installation shows web search as an optional fallback instead
of claiming that generic "Clip details are ready" status.

The one pipeline driver first uses exact public source metadata, then fixed-host Wikidata,
Wikipedia, Archive.org, and Library of Congress structured adapters. Library of Congress is the
initial credential-free public catalog; additional country-specific catalogs require a separately
reviewed official API contract rather than guessed endpoints or user/model-selected hosts. It
invokes general web search only when the interpreter still lacks one of the requested
era/geography answers, and at most once for one Clip input revision. A durable atomic monthly ledger
and per-revision attempt record reserve each request before dispatch, so concurrent workers cannot
cross the configured ceiling and a failed provider cannot create a later unattended cost loop;
the status projection reports the same count and last success/failure. Structured citations retain
priority, while bounded round-robin merging prevents one peer adapter from consuming the whole
citation packet and reserves room for fallback snippets when the fallback actually runs. Provider
failure or exhaustion keeps the structured report, records degraded status, and never fails
preparation, playback, or admission.

Brave uses only its fixed documented API host. SearXNG configuration accepts one credential-free
HTTPS endpoint; its JSON search response is bounded and no public instance is selected by Loomarr.
The first increment stores attributable result titles, public URLs and bounded snippets only: it
does not fetch result pages. Search content is untrusted data, never model instructions, and the
model receives citation identifiers rather than browsing or fetch authority. General-web evidence
may feed only the same bounded cited-context report and country projection above; it cannot write era,
market, Airworthiness, language, readiness, Placement or admission facts.

Transcript, frame and context catch-up use that same post-ready coordinator rather than putting a
Ready Clip back on the readiness conveyor. The `filler-pipeline` driver always advances bounded
preparation first. When runnable or active preparation remains, it yields the lease without starting
optional enrichment so downloaded Clips keep moving toward playable readiness; scheduled retries do
not block unrelated detail work. A remote enrichment failure cannot change the preparation result.
Each enabled capability has its own provider/model/prompt identity and the
existing `MaxWhisper` or `MaxVision` per-pass bound. A Clip is eligible only while at least one axis
that capability can inform remains unresolved, and an operator answer (including an intentional
empty answer) closes that axis to automation. A successful media pass records its completion even
when it found no usable signal, while a provider failure records no completion and may retry later;
neither outcome changes Placement, `held`, or the terminal-ready event. When transcription or frame
inspection changes the persisted transcript or visible-text signals, the free and text passes become
eligible once more for that Clip. Their ordinary evidence precedence then decides whether the new
signal improves an axis. The same capability identity is never paid for twice, while changing the
selected provider/model or prompt version wakes only the bounded eligible work.

**The pipeline is sequential and budget-bounded, and that is not a limitation.** Whisper is ~341s
per clip under QEMU and ffmpeg competes with playout for the GPU, so one clip at a time is what
keeps a catalog import from starving live channels. It is also the answer to SSE volume: at most
one clip is running, so "forty clips × eight stages" never arrives at once. The per-run budget
(`MaxClips`, `MaxTranscodes`, `MaxWhisper`, `MaxVision`, `MaxSplits`) carries forward the existing
batch constants, so the "backlog drains over cycles" property those constants defend is preserved
exactly.

**Retry with backoff is new.** The cron jobs had none — `Work` always returned nil, so a failure
simply waited for the next tick and a permanently-broken clip was retried at full cost forever.
Stages now retry 5m / 30m / 2h and then resolve: a `probe` failure is a **reject** (a file we
cannot measure is not a clip), while `transcribe`/`vision`/`language` **skip and advance** — a
missing transcript must never strand a clip.

**Recovery rewinds the dependency suffix, not the whole clip.** Once an operator fixes the cause
(for example a model or provider setting), an admin may restart at the failed stage. The rewind
removes that stage and each downstream rung from the ladder, clears only their safely replaceable
derived state, resets retry attempts/backoff, persists an explicit force-run marker, and returns the
existing row to `queued`. The worker bypasses that selected rung's ordinary applicability shortcut
even after a restart, then clears the marker when the rung resolves; upstream measurements and
operator-authored tags survive. A transcode rewind is exceptional and requires an explicit force
flag because it replaces derived playable bytes, although V66 retains the source master. This is deliberately a pipeline operation rather than a
delete-and-rescan workaround: recovery must preserve identity, provenance, and operator decisions.

**Lifecycle and recovery are domain answers, not UI guesses (V56).** The persisted row remains the
fact record, but `filler.Pipeline` projects it into one bounded lifecycle vocabulary shared by the
runner, Incoming, and telemetry: `runnable`, `in_progress`, `scheduled`, `needs_decision`, `ready`,
`complete`, `rejected`, or `dismissed`. `ready` is playable; `complete` is reserved for a processed
Composite container that is structurally not playable. A failed rung additionally carries a stable failure code,
the exact rung that can be retried, and one sanctioned recovery action. This prevents three callers
from independently interpreting combinations of disposition, status, backoff, and reject reason;
the store aggregates facts, while the filler domain owns their meaning.

Retry and override remain different authorities. An exhausted `probe` or `transcode` execution
failure may retry the failed rung after its dependency suffix is invalidated; a soft content
decision may be restored to human review; measured hard content failures remain non-overridable.
For a terminal execution failure, returning the row to `running` and clearing the catalog tombstone
is one store transaction, and the clip is held before it becomes present again. Derived artifacts
are invalidated first; if that preparation fails, the old rejected row and tombstone remain. Thus
no partial recovery can make an unprepared or rejected clip airable, while a successful retry
preserves completed upstream measurements and operator-authored tags.

#### Stage state is persisted, in a sibling table

⚠ **`filler_clip_pipeline` is a durable table beside `clips`, NOT columns on it**, and the reason
is the same one every ⚠ in `UpsertClip`'s DO UPDATE block states. `clips` is a synced **cache** of
the drop-folder and has been dropped and recreated twice (00006, 00033). Pipeline state records
that we *already spent* ~341s of Whisper and a paid vision call on a clip; in the cache, the next
identity change silently re-runs all of it and re-spends the money. In a sibling table, a rebuilt
`clips` re-syncs from disk and the pipeline correctly sees the work as done.

It also makes the single-writer story **structural rather than by convention**: the folder scan
cannot touch a table it does not know about, so there is no omission list to forget.

The finished ladder is one JSON document (`stages_json`), the same call `filler_split_proposals`
makes for `segments_json` and for the same stated reason — it is authored and read as a unit and
never queried relationally. The columns above it (`stage`, `status`, `disposition`, `next_run`) are
exactly the ones the work-list query and the Incoming tab filter on.

#### Watching it happen

⚠ **Stage state is PERSISTED and served by `GET /v1/filler/incoming`; the SSE frame is a latency
optimisation.** That is §8's standing rule, and it is load-bearing here rather than ceremonial: the
bus drops frames for a slow subscriber by design, so a client that assembled the ladder from frames
would show a queue that is silently missing steps. Every `filler_clip` frame is therefore a
**self-sufficient snapshot** — any single frame fully describes where the clip is now — and the GET
is the truth on reconnect.

⚠ **Only one stage reports a real percentage.** The transcode stage reads ffmpeg's
`-progress pipe:1` and the clip's duration is already known, so `outTime / duration` is exact and
free. Whisper and an LLM turn are single opaque calls: interpolating a bar over them would be the
fabricated-progress failure the database-migration frame already warns about. Other stages report
**step boundaries** where genuine sub-steps exist and 0-then-100 where they do not. **A running
stage with no measurement shows no bar**, for the same reason a confidence of 0 renders nothing:
absence of measurement and a measurement of zero are different claims.

Intra-stage progress is throttled in the emitter (≥1s and ≥5 points since the last frame for that
clip) and in the database; status *transitions* always publish and always write. The percentage is
decoration — what has to survive a reload is which stage, and whether it is running.

**Preparation progress is a separate server projection (V68).** A Preparation attempt begins at
Enrollment and receives a durable generation, start time, and start reason. Automatic stage retries
and pass deferrals remain in that attempt; an explicit recovery rewind increments the generation,
records `restart`, and visibly resets its progress. The bounded ladder stores each rung's latest
start and finish time, plus the current rung's queued and started times. It remains one record per
stage rather than becoming an event log.

The server owns the applicable-work plan. Every rung in `StageOrder` is one possible unit until the
pipeline proves it done or not needed; a completed skip remains in the denominator and contributes
exactly once. A measured current rung contributes only its measured fraction. The persisted maximum
within one attempt prevents retries, setting changes, and concurrent snapshots from moving the
number backward; only a new attempt may reset it, and Ready is exactly 100. Rows predating this
evidence expose unknown progress rather than reconstructing a plausible-looking value.

**A Ready estimate is local evidence, not a countdown.** Loomarr may return a structured lower and
upper duration only after this installation has enough recent Ready attempts with complete timing,
no retries, the same applicable-stage signature, and a comparable coarse clip-duration bucket. The
range is derived from those observed stage durations plus bounded older work ahead in the
oldest-first queue. It disappears when evidence is stale or insufficient, while a retry is
scheduled, or while the Clip waits for a person. The browser formats only the server's range (for
example, “About 2–4 minutes”); it never counts stages, extrapolates elapsed time, or invents a
deadline.

⚠ **The database throttle is `percent >= lastWritten + 10` OR `>= 2s since the last write`, and
`lastWritten` is the last value actually PERSISTED (V54).** Both halves of that sentence are load-
bearing, because the obvious reading of "≥2s / ≥10 points" produced a throttle that could never
fire:

- **OR, not AND.** A stage that crawls needs the time half to ever reach disk; a stage that jumps
  needs the points half. Requiring both means a slow stage writes nothing for minutes.
- **The baseline is the persisted value, not the last reported one.** If the skip branch advances
  the baseline, it moves with every sample — ffmpeg emits about once a second, so `percent` is
  perpetually `lastWritten + 1` and the `+ 10` test is never satisfied. A long transcode then
  persists **nothing at all** between 0 and 100, which is precisely the case the write exists for.
  The skip branch must publish and leave the baseline where it is.

⚠ **`NoMeasurement` (-1) is a state, not a percentage, so it always writes and is never throttled.**
It fires once when a stage that cannot measure itself starts, and it is what makes "a running stage
with no measurement shows no bar" true across a reload — dropping it leaves the row at the 0 the
stage was initialised with, which renders as a bar frozen at zero, i.e. the exact fabricated claim
this section forbids.

#### Three outcomes, not two

The lifecycle gains a third terminal state. `held` and `filed` were decided in different files by
different jobs with no shared vocabulary: the language job tombstoned, the tagger filed, and the
scan silently skipped. A single `Verdict` — **continue / review / reject** — is now what every
rule answers, so adding a criterion means adding a rule rather than editing three jobs.

⚠ **V54 adds a fourth, `defer`, and it is not a terminal state — that is the point.** The three
above all END a rung's involvement with a clip. `defer` says *the rung made progress and is not
finished*: no attempt is spent, the clip stays `running`, and it resumes next pass. It exists
because a per-pass budget over a per-reel job had no way to say "60 of 142 done, ask me again", so
a budget meant to bound COST silently behaved as a ceiling on capability. A rung may only return it
having actually advanced something; deferring on a pass that achieved nothing is an infinite loop.

⚠ Do not confuse this with **"The operator's decision is the fourth outcome (V54)"** below. That
one is a fourth *disposition* — where a clip sits. This is a fourth *verdict* — what a rung
concluded. Two different enumerations, both of which gained a fourth member in V54.

⚠ **A reject is visible and reversible.** It carries a stable reason CODE (never generated prose)
plus the measured detail behind it — `"8.2s; floor is 10s"` — and appears in Incoming under *"we
didn't use N clips"* with a one-click restore for the soft cases. A hard reject (no audio, no
video) offers no override, because that is a control that could not work. The rule §10 has held
since V35 applies unchanged: **an unattended decision that cannot be found is not one an appliance
gets to make.**

⚠ `clips.removed_at` stays the *airability* gate and pod assembly is untouched. Two places, one
truth: `removed_at` is **whether**, the pipeline row is **why**.

#### Operator outcomes (V54; direct publication actions retired)

⚠ **V54 added direct “Use it” and “Looks right” publication actions; those actions and their route
are retired.** Confirming taxonomy is not equivalent to certifying content safety and rights. The
only positive terminal transition is now an applied admission action whose transaction replays the
exact current evidence before changing `held` and the pipeline disposition together.

**`dismissed` is a fourth disposition, not a reuse of `rejected`.** A person saying *no* and the
quality gate refusing are different facts, and `rejected`'s shape is built for the second: it
carries a stable `RejectReason` CODE plus the measured detail behind it, and whether it may be
undone is decided per reason by `Soft()`. An operator dismissal has no code — the reason is *a
person said so* — no measurement, and is always reversible. Folding it in would mean inventing a
reason that means "no reason" and a `Soft()` case that is unconditionally true: two exceptions so
one enumeration can carry two subjects. That is the argument `reject.go` already makes for keeping
`RejectReason` and `AutoSplitReject` apart, applied one level up.

**Current transitions:**

| Operator action | Route | Pipeline row |
| --- | --- | --- |
| Review or correct tags | tag editor | classification changes; remains held |
| Required work completes | terminal-ready operation | atomic `running → ready` with Placement |
| *Don't use it* | `POST /v1/filler/bulk/remove` | `review → dismissed` |
| Restore | `bulk/remove` with `restore: true` | `dismissed`/`rejected` `→ review` |

⚠ **Positive publication is never a best-effort pair of writes.** The Ready event, Placement, clip
release, and pipeline settlement commit in one transaction or none do. Tag correction remains
deliberately non-terminal: it improves descriptive metadata but cannot make media playable.

⚠ **`dismissed` is off the conveyor AND off the refusals list, and the second is not an
oversight.** *"Loomarr didn't use N clips"* is the audit of what the appliance decided **without**
the operator. A dismissal is what the operator decided themselves, so listing it there would
re-merge the two questions that section exists to hold apart. The restore endpoint already accepts
a dismissed row and returns it to `review` — but **no surface lists dismissed clips today, so that
undo is currently unreachable from the UI.** Recorded as a known gap rather than described as a
feature: the endpoint is right, the affordance is missing.

### Sources roll up by provider (V51c)

Three archive.org collections sat as three sibling rows with no indication they are one service,
and adding YouTube channels would have made it worse. The Sources tab now shows one **Archive.org**
row and one **YouTube** row, each twirling down to the targets an operator added beneath it.

⚠ **The grouping is DERIVED from `kind` at read time. There is no `parent_id`.** *The grouping
being asked for is already a column*: every `archive` row belongs under Archive.org, and there is
no representable case where it belongs anywhere else. A stored parent would be a second encoding
of a fact `kind` already carries, and second encodings make illegal states representable
(`kind='archive', parent_id='provider:youtube'`). Provider policy is different: Archive.org and
YouTube now have persisted master switches in `filler_providers`, keyed by that same closed `kind`
vocabulary. The table stores policy about a provider, never hierarchy about a source.

Three concrete costs a stored parent would have added, each measured against code that exists:

- Migration `00034` already seeds a blank-URI `youtube` row shaped exactly like a provider root.
  Inserting `provider:youtube` beside it produces **two** blank-URI YouTube rows, both invisible to
  `idx_filler_sources_uri` (whose `WHERE uri <> ''` predicate excludes them) and both eligible for
  the read model — "one source appears twice", which `00023` and `00029` both exist to prevent.
- It creates a **three-tier inherit problem** for `fetch_every_seconds`/`fetch_max_per_run`, whose
  nil/0/N encoding already carries a ⚠ saying both callers "must not re-derive this three-state
  logic separately". A parent tier makes `child=nil, parent=0` mean *never* while
  `child=nil, parent=nil` means *global* — four states, re-derived in three places.
- `filler_pulls.plan_json` stores `SourceID` strings looked up at approve time, so rewriting the
  seeded `youtube` row's id would 409 any pending pull.

`filler_providers` seeds `archive` and `youtube` enabled on upgrade. Its switch is independent of
every child switch: pausing Archive.org leaves each collection's `enabled` value untouched, and
resuming it restores the exact mix the operator chose. Existing clips remain catalogued and
playable; pause governs future provider work only.

**Wire shape: flat, pre-ordered, `group` + `parentId`.** Not nested — a recursive `children: []`
generates badly through orval and the frontend has no tree primitive, while a flat pre-order array
is exactly what a twirl-down renders from (hide rows whose parent is collapsed). It is purely
additive, so a client that knows neither field renders the flat list it always did.

**What does NOT inherit, and why each one is deliberate:**

- **`enabled`: provider policy composes with, and never rewrites, child policy.** The store's source
  projection owns `effectiveEnabled = provider.enabled && source.enabled`; acquisition callers use
  that projection rather than re-reading either flag. Provider-off blocks scheduled and manual
  enumeration/search/fetch, pull proposal, the approval-time recheck, and direct provider-adapter
  execution. Editing an existing target's local policy remains allowed while paused. Adding a new
  remote target now repeats exact provider resolution at registration (V67), so it is paused with
  search and resolution rather than creating an unverified row; resume the provider to add it.
  The read model exposes both the remembered child choice
  and effective state so the UI can say “Paused with Archive.org” without pretending the child was
  switched off.
- **Fetch overrides: leaf only** — see the three-tier argument above.
- **`lastCheckedAt`: a read-only `MAX` over children**, computed in the API so no column can
  disagree. Absent when no child has been checked, so the row reads "never" rather than an epoch date.

⚠ **`folder` and `library` do not group.** A twirl-down exists because ONE SERVICE offers many
targets; two watched folders are unrelated directories with no service in common, so a "Folders"
container would be a row that dims and changes nothing — the shape §10 forbids.

### Finding a source is not adding it (V67)

Remote setup has three deliberately separate steps: a **source suggestion** is one transient
provider result, **source resolution** verifies typed input and returns one canonical provider
target, and **registration** persists that target as a filler source. Suggestion and resolution
create no database row, download no media, enqueue no work, and grant no background-acquisition
authority. Only `POST /v1/filler/sources` registers a source. Search responses are short-lived
cache material, not embedded catalog data, so they do not belong in the database.

One provider-neutral finder owns that contract while provider adapters hide their query syntax,
canonicalisation, timeouts, result caps, and response parsing. Archive.org uses its public Advanced
Search endpoint restricted to `mediatype:collection`, then resolves an exact identifier through
`/metadata/{id}` and refuses anything whose metadata is not a collection. YouTube uses the shipped
yt-dlp executable in flat, listing-only mode; it needs no Google project or operator credential.
Both paths are bounded and fail closed before external work when their provider master switch is
off. The browser sends text and renders server results; it does not reproduce provider rules.
YouTube video-search hits are collapsed by stable channel identity, and a channel resolves to its
canonical `/channel/{id}/videos` target rather than the channel root: the root lists channel tabs,
not the bounded stream of videos the registered-source enumerator needs. A video-only URL is
rejected here rather than silently turning one video into an unattended recurring source; one-off
ingest remains a separate deliberate action.

The Sources page renders one calm section for Archive.org and one for YouTube. Each has its master
switch, a single accessible **search-or-paste** field, a stable keyboard-operable suggestion list,
and quiet rows for sources already added. Selecting a suggestion first shows the exact target that
will be added; registration remains an explicit action with duplicate and failure states beside
that target. **Cancel** discards that transient selection and returns focus to the finder; a slow
resolution response cannot restore a target after the operator has cancelled or entered newer
text. Provider/type chips, repeated card borders, and always-visible tuning controls are
absent: provider headings already supply the context.
Turning a provider's master switch off disables its child source controls and folds the provider
body closed, including its add flow and registered rows. It changes no child's saved switch value:
turning the provider back on unfolds the body with those choices intact. The fold begins with the
pending off action and reopens if that action fails, rather than leaving apparently usable children
on screen while the parent change is in flight.
Provider summaries reserve **needs attention** for an actionable problem; a child the operator
intentionally switched off remains counted as a saved source but is not labelled a problem.

Exact resolution also returns at most three provider-native example items from that target. The
selection preview links to the source and names those examples so the operator can understand what
Loomarr will check before registration. Each example has an explicit **Preview** action that loads
the provider's own player in the shared dialog only after the operator asks, and asks that player to
begin immediately because opening the dialog is the playback gesture; **Open original** is always
available as the fallback. Examples stay text-only until then: no thumbnail wall, eager
embed, local download, database row, or acquisition authority. An unavailable example preview does
not make an otherwise valid source unregistrable, and the UI describes examples as a bounded look
at the source rather than a promise that every shown item will be accepted or played.
The example section and its heading occupy their final position while those at-most-three items are
being resolved, with Loomarr's indeterminate loader in place of the rows. The loader is replaced by
the examples without shifting an otherwise unlabelled loading message into a new section.
Archive.org examples are selected separately from acquisition order: one bounded Advanced Search
request restricts the sample to movie items with a video representation Loomarr can consume and
orders those items by Archive.org download count. This produces recognizable, playable examples
without claiming that popularity changes what acquisition is allowed to inspect; the returned count
describes the video items eligible for that preview, not every metadata record attached to the
collection.

A registered source row is an index entry, not a miniature settings page. It shows the switch,
name, one truthful status, and one affordance to open the source workspace. That workspace reuses
the application Sheet: it slides from the right on desktop, fills a narrow screen, traps and
restores focus, and keeps the Sources list stationary behind it. The index is `/filler/sources` and
an open workspace is `/filler/sources/{sourceId}`: direct links and reload restore the same source,
browser Back and Forward follow the open workspace, and closing returns to the index without
leaving selection in component-only or query-string state. A stale source bookmark returns to the
index, and the route rejects members before reading the admin-only source list. The selected source's exact check
outcome, provider link, same three-item source preview used before registration, clip browser,
location exception, cadence/limits, and removal live together there. Opening a registered remote
source resolves its saved target directly; the operator never has to search for a source they have
already added. The primary manual action is labelled **Look for new clips** because it may queue
downloads; “check” is reserved for status and must not hide that consequence. Repeated prose around
the three-item sample is removed once the source status already explains review. Archive's optional
catalog tool is labelled **Find a specific clip** and starts collapsed: it is an intentional escape
hatch, not a required setup step or a peer of unattended acquisition. Its result rows reuse the same
provider-native Preview dialog as the three-item sample before offering **Queue download**. Queueing
an item keeps the registered parent Source and provider item identity; it never turns the item into
another recurring Source. The row moves through **Queueing…**, **Downloading…**, then the truthful
terminal **Added — being checked** or **Couldn’t add** state. The browser retains only the small
item-to-job correlation needed to restore that row after navigation; the acquisition GET is the
authoritative state after reconnect, while SSE only shortens the delay. An accepted request is never
rendered as permanently queued after its background job has failed.
The full visible row for this tool and **Source settings** is the disclosure trigger; a small
far-edge chevron alone is not an adequate or discoverable hit target.
Clip search initially reveals eight results and progressively reveals the rest of the bounded 25-result
page inside the sheet's own scroll area; it never lengthens the Sources page. A local-source sheet
retains the concrete folder path or library name and latest-check time; a wide workspace that repeats
only “Ready” and one button does not give the operator enough context to justify opening it. At ten registered
sources in a provider or local section, the section adds a registered-source filter. Without a
filter it shows every paused, failed, or attention-needed row first, then five healthy rows, with
the remaining healthy count behind **Show N more**; **Show fewer** restores that calm view. An
active filter searches only registered rows, shows every match, and never changes the provider
catalog finder above it. Local folders and media-server libraries use the same source workspace where their
capabilities apply. **Your files** is the first source group, ahead of remote providers, and owns its
compact folder/library add form just as each remote provider owns its catalog finder; that form is
never detached at the bottom of the page and the group remains visible as an invitation when it has
no sources yet. It has no remote catalog to search. A location exception reuses the same single searchable location picker as setup and
Settings; it never exposes separate country and market fields. Automatic detection belongs to the
installation location. In a source workspace, **Use my location** instead clears the exception and
returns the source to installation-location inheritance.

⚠ **An honest gap this exposes rather than creates:** `sync.go` writes `Source = "filler-dir"` for
every clip the folder scan finds, and the sidecar records only *whether* Loomarr downloaded a clip,
never *from which source*. So `bySource["archive"]` is 0 on essentially every install. A group
reports the **sum of its children's counts** — honest arithmetic over whatever the children claim,
never an invented number. Per-source attribution is an **intake** change, tracked separately.

### The catalog is paged, sorted, and searched wider (V51d)

`GET /v1/filler` returned **every clip in the install** on every call, and four clients depended
on that. On a catalog of a few hundred that is merely wasteful; at scale it is a hard failure —
`attachTags` builds one bind parameter per clip in a single `IN (…)`, and Postgres caps a
statement at 65535 parameters, so an unpaginated read stops working north of ~65k clips.

**Offset pagination, not a cursor.** The catalog is a *filterable, sortable grid* that has to say
"showing 61–120 of 1,204" and offer a page jump; a cursor expresses neither. A keyset cursor would
also have to encode the full sort tuple — five sort keys × two directions is ten encoders, each an
independent chance to get the tie-break wrong — while `total` here is free and correct **by
construction**, because `CountClips` already shares `clipWhere` with `ListClips` (that sharing
exists precisely so a second hand-written predicate cannot drift). A search can never return rows
whose count disagrees with them.

⚠ **`limit` defaults to 100 and caps at 500 — a deliberate behavioural break.** Three of the four
clients were unbounded catalog reads that paging should *delete* rather than paginate, and all four
are fixed in the same change: the dashboard's clip count reads `GET /v1/filler/watch`'s SQL-counted
total; the channel-filler pin/exclude resolver asks for **the N hashes it actually holds**
(`ClipFilter.Hashes`, a batch read) instead of loading the catalog to build a map; the ⌘K palette
renders at most eight results and now asks for eight; and the Filler page wires the pager.

⚠ **`ClipFilter.Limit == 0` means no `LIMIT` clause, and the default lives in the API, never in the
store.** This is the single most important sequencing rule in the change: **pod assembly loads the
catalog through the zero filter**, so a store-side default of 100 would silently cut every
channel's break pool to the first hundred clips — a scheduling bug with no error and no log line.
The same polarity argument the `IncludeHeld`/`IncludeComposites` flags carry, applied to a number.

**Sort:** `name | duration | added | plays | confidence`, ascending or descending.

- ⚠ **Every ordering appends `hash` as a tie-break.** Without a total order, `ORDER BY duration_ms`
  under `LIMIT`/`OFFSET` may return one row on two pages and skip another — Postgres makes no
  promise about the relative order of tied rows between statements. The tie-break is what makes
  paging *correct*, not merely pretty.
- ⚠ **`name` sorts as `LOWER(name), hash` on both dialects.** SQLite's default `BINARY` collation
  puts `'Z' < 'a'`; Postgres's locale collation typically does not. Without the `LOWER()` this is
  one suite producing two different orders, which is exactly the per-dialect fork §5's store rules
  forbid — and the conformance fixture carries case-mixed names so that a regression fails on
  exactly one backend, which is what `make test-pg` exists to catch.
- ⚠ The sort column comes from a **fixed `switch`**, never concatenated from client input, and an
  unknown value is an error rather than a silent fall-back to a default. A silent fall-back turns
  "the sort control does nothing" into a bug nobody can see.

**`added` needs a column — `clips.created_at` (migration `00046`).** `updated_at` cannot stand in:
a re-sync bumps it on every clip it touches, so "recently added" would reshuffle the entire catalog
after a routine folder scan. Existing rows backfill from `updated_at` as a **stated estimate** —
the honest answer for clips that predate the column. ⚠ It is INSERTed but **omitted from
`UpsertClip`'s `DO UPDATE`**, the same rule `held`, `removed_at`, `confidence` and the play counters
already carry, and for the same reason: the scan supplies a fresh timestamp on every pass, so
letting it ride the update list would make every clip "just added" after each sync — the precise
failure the column exists to avoid.

**Search widens** from `name` alone to `name | brand | visible_text | tags` (the last via an
`EXISTS` over `clip_tags`, which is indexed both ways). `transcript` sits behind an explicit
`QueryTranscript` flag rather than joining the default set: it is the one genuinely long column (a
few KB per clip, so a 500-row page scans megabytes) and the one noisy one — "ford" matches "afford"
with no ranking available to explain the hit.

⚠ **No full-text search.** SQLite FTS5 and Postgres `tsvector` are two engines with different
tokenizers and different ranking, so adopting them would force `ListClips` to branch on dialect and
the conformance suite to assert *equivalent-but-not-identical* results per backend — one suite, two
behaviours, which §5 forbids in as many words. A `LIKE` over four columns is slower in theory and
indistinguishable at household scale. Because the predicate lives in the shared `clipWhere`, this
widens `ListClips` and `CountClips` **together**, so `total` under a search cannot disagree with the
rows returned.

**Composites paginate as containers.** A new `ClipFilter.TopLevelOnly` (`parent_hash = ''`) is
**opt-in**, set only by the catalog listing, and its opt-in argument is sharper than its siblings':
**segments are the airable clips**, and pod assembly loads through the zero filter — so an opt-*out*
would remove every advert split out of a recorded break from every channel's breaks, the exact
inverse of what V45 exists to achieve. `ClipFilter.ParentHash` is exposed on the route so expanding
a break loads its segments.

⚠ **A shape defect this exposes:** `listFiller` passed **neither** `IncludeComposites` nor
`TopLevelOnly`, so composites were invisible while their segments appeared as flat rows — the
inverse of the composites design. The catalog listing now asks for composites *as containers* and
hides the segments beneath them.

**`ClipDTO` gains** `brand`, `confidence`, `held` (with `includeHeld` — the field and the parameter
ship together, never the parameter alone, or a client can ask for held clips and not be told which
ones they are), `language` (all three states documented), `visionTagged`, `license` (carrying
`FillerSourceDTO.License`'s warning verbatim — **empty means UNKNOWN, never public domain**), and
`hasTranscript`. ⚠ **Not `transcript` itself** — kilobytes per clip that no grid renders; at 100
rows it would be roughly ten times the rest of the payload. ⚠ Not `visibleText`, which is the audit
trail behind a vision-grounded tag and therefore a detail-surface concern.

### The pipeline becomes visible (V51e)

V51b made ingest an ordered, watchable pipeline and served every fact about it. **Nothing
rendered any of it.** `GET /v1/filler/incoming` carried `pipeline` and `rejected`, the bus
published a `filler_clip` frame per transition, and the frontend subscribed to neither — so the
operator-visible symptom V51b was built to remove ("I downloaded forty commercials and nothing is
happening") survived V51b intact. V51e is the rendering half, and it is the phase that makes the
previous one true.

**Incoming is ONE conveyor, not a queue beside a progress list.** A clip is somewhere on a single
belt: the machine is still working on it, or the machine has finished and wants a person. One row
per clip, and the row says which.

⚠ **"One row per clip" covers the REELS half too, and that half was missed (V54).** The rule was
written against the asks-vs-pipeline duplication (the 84-of-85 incident below) and left implicit
for compilations, so the same reel rendered twice: once as a taggable ask, once as a reel. Stated
explicitly: **a compilation with a pending proposal is represented by its reel and is off the belt;
while it is still being DETECTED it is on the belt as a preparing row, and never as a decision.**

The reason no test caught it is worth keeping. Neither rule was wrong. `conveyorDTO` read
`disposition == review`, which is correct — review IS the handoff. `askReasonFor` reported an
untagged commercial as unidentified, which is also correct, because the pipeline deliberately never
tags a composite. The defect existed only in their intersection, which is a shape a per-rule test
cannot reach and only a fixture holding a composite could produce.

- **Still being prepared** — thumbnail, name, duration, an eight-pip strip, and the active-voice
  sentence for the rung it is on ("Working out what it is"). Expanding gives the named ladder with
  skip reasons and, where one exists, a percentage. Nothing here is work the operator owes.
- **Needs a decision** — the same row, once the pipeline hands the clip over: its tags, its
  grounding-capped confidence, the reason it could not be settled automatically, and the file /
  retag / discard controls.

⚠ **The two were separate lists and it was a mistake, caught by looking at 85 real clips rather
than by any test.** `asks` was V38-era logic — *held and untagged* — and `pipeline` was V51b's
non-terminal rows. Nothing joined them, and the runner enrols **every** clip at `probe/queued` on
scan, so the two sets were identical by construction on a fresh catalog: **84 of 85 clips appeared
in both**, one row demanding a decision while a row below it said *"nothing here needs you — it's
just working"*. The page contradicted itself about the same clip.

⚠ **The state that resolves it already existed and was never consulted.** V51b's `Disposition` is
`running → review | filed | rejected`, and `review` means precisely *"the machine is finished and a
human is needed"* — the population `asks` was trying to describe. `asks` predated the enum and
computed membership from tag-shape instead. So this is not two features disagreeing; it is one
query answering a question the pipeline had taken ownership of. `needsDecision` now comes from the
disposition, with the old held-and-untagged test surviving **only** as the fallback for a clip
catalogued before V51b, which has no pipeline row at all and must not become invisible.

⚠ Ordering is decisions first, in-flight after: work the operator owes outranks work they merely
watch. `total` still counts only what is waiting on a human — which now actually matches the rows
beneath it, where before it disagreed with its own list.

**Refusals stay their own section** — *"Loomarr didn't use N clips"*, the audit half V51b's text
already promised, carrying the reason in the operator's words, the measured detail, and a one-click
restore for the soft cases only. It is deliberately NOT on the conveyor: a refused clip has left
the belt, and mixing it back in would make "what is Loomarr doing" and "what did Loomarr decide
without me" the same list again.

⚠ **The ladder is served, not hardcoded — and `IncomingPipelineDTO.stages` is the wrong source for
it.** That field is the *visited* ladder, so a clip at `split` carries three records; a strip drawn
from it would grow as the clip advanced instead of filling, and the operator could never see how
much was left. The response therefore carries **`stageOrder`**, the whole sequence in run order,
derived from `filler.StageOrder` — the same list the runner walks. A rung added to the pipeline
appears in the UI without a second edit, and a guard test compares the served list against
`StageOrder` itself rather than a literal, so the obvious way to "fix" a failure is not also the
way to hide the bug.

⚠ **A disabled stage is a `skipped` rung, not an absent one.** An install with vision off still has
a ten-rung pipeline; the rung renders greyed with its reason inline ("Listen — skipped (the
description already says enough)"). A stage that silently does not happen reads as broken, and the
sentence is what turns a bug report into an answer.

⚠ **`stageOrder` and the per-clip status are the only things the frame is allowed to move.** SSE
frames merge onto the cached row and never assemble it: the bus drops frames for a slow subscriber
by design, a frame for an unknown clip triggers a refetch rather than inserting a half-built row,
and a terminal frame invalidates `/v1/filler` outright — a filed clip changes the catalog, which
nobody watching the catalog tab has a pipeline listener for. Only running frames merge, which is
what keeps forty clips × ten rungs from becoming 400 refetches.

⚠ **The ordering rule is derived from the ladder, because there is no sequence number.** A frame
carries no `seq` and no timestamp, so "is this newer than what is shown" is answered by the
pipeline's own shape: a stage or status CHANGE is always applied, and only the percentage *within*
one rung is guarded against going backwards. Strict advance-only was rejected — `Rewind`, the
sanctioned re-tag/re-split path, moves a clip backward on purpose, and a guard that refused it
would blank the whole re-run until something forced a refetch. A stale repaint lasts until the
next frame; a suppressed re-run looks like the machine has stopped.

⚠ **This does not contradict V40's "no badge, no review step", and the boundary is worth stating
because the next reader will otherwise take V40 as forbidding this section.** V40 refuses files at
the **scan** boundary, before they are catalogued, where listing every skipped file in an
operator's media folder would be noise about files Loomarr never took responsibility for. Runtime
refusals after cataloguing must be objective and remain visible with their measured reason; missing
descriptive classification is not one of them.

### A rung may not spend per SEGMENT what the budget allows per CLIP (V51g)

**Found on a live catalog, not by a test.** `WAGA-5/Fox Commercial Breaks(2/5/1995)` — a 16m47s
recording — sat at *"Finding the ads inside"* through **twelve** consecutive pipeline passes,
failing every two minutes with `context deadline exceeded` and starting again from the beginning.
Roughly 25 minutes of GPU spent re-doing the first third of one clip, while the row animated as
though it were making progress.

**Measured, on the real file** (the numbers are the point — the first three diagnoses were wrong
without them):

| Step of the `split` rung | Cost | Fits the pass? |
| --- | --- | --- |
| `blackdetect` + `silencedetect` | **4s** (319× realtime; 44 + 53 hits) | ✅ |
| `dedup` — `GrayFrames` × 51 | **33s** (662ms/segment) | ✅ |
| cut — ffmpeg stream copy × 51 | **3s** (59ms/segment) | ✅ |
| **`classify` — one LLM turn × 51** | **≈377s** (7.4s/call, `qwen3:8b`) | ❌ **6× the whole budget** |

⚠ **The "120s pass" this table originally compared against was WRONG, and the real ceiling was
half of it (V54).** 120s is the CRON INTERVAL (`0 */2 * * * *`) — how often the job *starts*, not
how long it may run. The actual ceiling was River's `JobTimeoutDefault`, **60 seconds**, because
`river.Config.JobTimeout` was never set and `riverWorker` did not implement `Timeout()`. So every
job on every install ran under a deadline inherited from a dependency, which nothing here chose
and nothing recorded.

⚠ **It does not surface as a timeout, which is why it survived.** `exec.CommandContext` SIGKILLs
its child, so the operator sees `ffmpeg …: signal: killed` / `whisper-cli: signal: killed` — a
corrupt file or a broken binary, not a clock. Measured on the maintainer's catalog 2026-08-11: a
`blackdetect` pass reported as "killed" inside the job completed in **40s** run by hand, and the
20-minute reel it belonged to was left `Unsplittable` for want of time it should have had.

The row above still holds — 377s does not fit a 60s pass either, and the rule below is unchanged —
but the margin was 6×, not 3×, and the numbers under it were being judged against a budget twice
the real one. Jobs now declare their own ceiling (`scheduler.Job.Timeout`); media jobs take
`scheduler.LongJobTimeout`, tied to the lease horizon so a job cannot outlive its own claim.

⚠ **The rule this establishes.** The scheduler's unit of work is a CLIP: `Cost()`, the per-run
budgets (`FILLER_PIPELINE_MAX_CLIPS`, `…MAX_WHISPER`) and the retry policy are all sized per clip.
A rung whose cost scales with a clip's CONTENT breaks that, and no retry helps — attempt two is
exactly as impossible as attempt one. Cheap per-segment work is fine (the fingerprint pass is 51
segments in 33 seconds). **What a rung may not do is spend a model call per segment.**

⚠ **`classify` inside `Propose` was that, and it was strictly-worse duplicate work.** It calls the
same `Classify` the `tag` rung calls, but with `SplitSegment.Transcript` — which is EMPTY unless
`rescue` ran, and `rescue` only transcribes segments over ~120s. On this reel **none** qualified
(longest 60s), so all 51 calls classified on nothing but a generated name: `"… part 7"`, identical
across segments apart from the number. `Confirm` writes those results onto each spawned clip, but
`Tagged()` needs `Era > 0 && Audience != "" && Category != ""` — and a bare part-number grounds no
category — so the `tag` rung re-runs anyway, this time after `transcribe`, with a real transcript.
The pipeline paid twice and kept the worse answer's cost.

**The fix is removal, not rescheduling.** Split CUTS; it does not describe. Every segment is
spawned as its own clip and runs the whole ladder for itself, so the classification already happens
downstream — one clip at a time, budget-bounded, resumable, and individually visible in Incoming.
Without `classify` the rung completes in **~40s** and all 51 children are enrolled in a single
pass, after which they progress **independently and out of order**: a 16-second silent advert
reaches `score` while its sibling is still being transcribed.

⚠ **The atomic confirm STAYS.** Cutting all 51 costs 3 seconds, so streaming enrolment buys nothing
here and would cost a deliberate safety property: a proposal is editable, so a partially-applied
confirm leaves orphan cuts of a plan that no longer exists. `Confirm`'s own comment states the
invariant — the proposal is consumed only once every segment exists on disk and in the catalog.

**Two mechanical rules, independent of split and true of every rung:**

⚠ **Running out of time is not failing.** A `context deadline exceeded` is a DEFERRAL: status back
to `queued`, attempts unchanged, resume next pass. V51b already treats budget exhaustion exactly
this way; the deadline path never got the same treatment, so a timeout burned an attempt and took a
backoff it had not earned.

⚠ **Failure bookkeeping must outlive the failure.** `onFailure` computes the record, the backoff
and the `MaxAttempts` resolution, then persists them through `ctx` — *the context whose expiry
caused the failure*. The save fails and all of it is discarded; only the pre-work write
(`status=running`, `attempts++`) survives, because that one happened while the context was alive.
That is why attempts reached 12 against a `MaxAttempts` of 3, and why the row never left `running`.
**Any rung that ever times out loops forever**; `split` is simply the one that timed out first.
Failure and deferral both persist through a detached context.

**Long recordings resume instead of restarting.** The split proposal is also the detector's durable
checkpoint before it becomes operator-visible. When chapters are absent, boundary detection scans
one fixed ten-minute span per pipeline pass, persists `scannedThroughMs` plus the accumulated
black/silence gaps, and yields without spending an attempt. The next pass resumes at that exact
offset. Once the final span is scanned, the module fuses the gaps into segments, performs rescue and
dedup, removes the private checkpoint, and only then exposes the proposal in the review queue. The
ten-minute span is an implementation budget, not a setting: it is deliberately well inside the
two-minute job deadline on the measured hardware, and adding another operator control would make
reliability depend on tuning Loomarr should own.

The checkpoint rides inside `filler_split_proposals.segments_json` as a versioned document rather
than adding coordination columns to the table. Readers remain backward-compatible with the former
bare segment array. A draft has no reviewable segments and is filtered from both the Incoming reels
list and `GET /v1/filler/splits/{id}`; the pipeline and rewind path still see it. Persist after every
successful span, before starting the next: a crash or deadline can repeat at most one span, never
the whole recording. The media-tools seam takes a time span and returns black and silence gaps
separately; preserving the source across checkpoints is what lets detector agreement remain the
strongest boundary-confidence evidence. Whether production uses ffmpeg filters or a test adapter
supplies captured intervals stays inside that module.

**Every segment operation is duration-bounded.** The ffmpeg adapters seek with `-ss` and limit work
with `-t (end-start)`. They never pair an input seek with absolute `-to end`: `-to` is a position,
not a duration, and made a late 30-second segment decode/cut an ever-growing portion of the reel.
That error made fingerprinting super-linear and could produce oversized cuts. Timeline checkpointing
still stands independently—the fixed span makes each invocation bounded; the durable proposal makes
the sequence resumable.

**Catalog fingerprints are a persisted derived cache, not work a reel repeats (V54).** Duplicate
detection compares each proposed span with every other catalog clip. Decoding those whole catalog
clips again for every compilation makes the rung scale with *catalog size × reels attempted*, and a
deadline or restart throws away every completed decode. Loomarr therefore persists the successful,
non-empty dHash sequence for a catalog clip in a sibling `filler_clip_fingerprints` table and reuses
it across reels, retries and process restarts. Proposed spans remain proposal-local work: only
whole catalog clips are reusable across compilations.

The cache key is **`(clip_hash, algorithm)`**. `clip_hash` is the SHA-256 identity of the actual
bytes, so replacing or transcoding a file produces a hard miss instead of reusing evidence from the
old media. `algorithm` versions every sampling and hashing choice (`fps`, dimensions, pixel format,
hash construction and comparison alignment), so a future detector change can coexist with old rows
without a lock-step rewrite. Identity replacement deletes the old hash's entries rather than
re-keying them: the new bytes must earn new evidence. Catalog pruning removes orphan cache rows; the
table deliberately has no foreign key to the rebuildable `clips` cache, matching the pipeline-state
ownership rule above.

Cache population is incremental. After one catalog clip decodes, its fingerprint is upserted through
a short context detached from the expiring pipeline pass; a later timeout can repeat at most the
currently-decoding clip, not the entire catalog prefix. Concurrent misses may compute the same clip
and converge through the same upsert. Each reel loads the current algorithm's cache in **one read**,
not one query per catalog clip; a malformed row is omitted without discarding valid siblings. A
read/write/cache-document failure is **fail-open**: Loomarr
computes from media when it can, logs a cache write failure, and never calls a clip duplicate from
missing or corrupt cached evidence. Empty frame sequences and failed decodes are not cached, because
an undecodable file is not proof that two adverts are the same. This is derived state and adds no
operator setting or review control.

#### The rule V51g established, restated as its principle (V54)

V51g's rule was written as *"what a rung may not do is spend a model call per segment"*. That
sentence is the **letter** of a measurement taken on one provider; the **principle** it protects is
narrower and is what actually binds:

> ⚠ **A rung's cost must stay inside its pass budget, and must not scale unboundedly with a clip's
> content.** A per-segment model call is forbidden *when it cannot satisfy that*.

The distinction matters because V51g's 377s was **51 × 7.4s of SERIAL inference on one local GPU**.
That is a property of `qwen3:8b` on a single 3080 Ti, not of the algorithm: a hosted provider
answers concurrently, and the same 51 calls complete inside the pass. Restating the rule as an
absolute would forbid a shape that no longer costs what it cost when it was measured.

**What does NOT change, and is not reopened:**

- ⚠ **The text `classify` inside `Propose` stays deleted.** Its second defect was never about
  speed: `SplitSegment.Transcript` is empty at split time (`StageTranscribe` runs *after*
  `StageSplit`), so all 51 calls classified nothing but a generated name — `"… part 7"`, identical
  across segments but for the number — and grounding correctly refused to invent tags from that.
  **A faster model classifying `"part 7"` grounds exactly as much as a slow one.** The tripwire in
  `splitjob.go` stands: it must not come back to that file.
- The atomic confirm stays atomic. Deferral-not-failure stays. The per-pass segment budget is now
  a persisted checkpoint: each examined segment is marked on the proposal, a deliberate yield
  spends no retry, and the next pass resumes the unchecked tail even for ~500-segment captures.

**What a per-segment model call must satisfy to be permitted:**

1. **An explicit budget setting**, sized per pass, in the same family as `filler.pipeline.max_whisper`
   and `…max_vision`. No budget ⇒ not permitted, whatever the provider.
2. **Data the rung genuinely needs and cannot obtain later.** The bar V51g set: the classifier it
   removed produced results the `tag` rung recomputed downstream anyway — *"the pipeline paid twice
   and kept the worse answer's cost."* A rung may not buy a second, earlier, worse copy of something
   the ladder already produces.
3. **A signal that actually exists at that point in the ladder.** This is what disqualified the text
   classifier and is the test any replacement must pass.

**The worked example: grounding a segment from PIXELS, not from its name.** The auto-confirm gate
(§10 V34) refuses a segment with neither `Audience` nor `Category`, because such a clip can only
ever be a fallback-ladder pick and is not something to create unattended. `classify` used to supply
those fields; removing it left the gate with **no data source at all**, so `filler.autosplit.enabled`
has been default-ON and structurally unable to fire ever since — measured on the maintainer's
catalog as **45 compilations parked at `split`, none auto-confirmed**. A default-ON feature that
cannot fire is worse than one that is off, because the operator believes it works.

A keyframe drawn from the segment's own span satisfies all three conditions where the transcript
could not: the frames exist at split time (`MediaTools` already extracts keyframes, and already
takes a span for `Transcribe`), a category grounded from what is on screen is a real answer rather
than a re-reading of `"part 7"`, and the vision tier's grounding (`groundVisionTags`) is the same
one the `vision` rung uses — so the gate is fed by the same vocabulary that judges it.

⚠ **This does not make the gate's data authoritative for the CLIP.** Each spawned segment still runs
the whole ladder for itself and is tagged downstream with its own transcript; the split-time
grounding exists to answer the gate, not to replace `tag`. Where the two disagree, the child's own
pass wins — it is later, better-informed, and per-clip.

#### The budget is a RATE, not a ceiling (V54)

⚠ **`filler.pipeline.max_split_vision` bounded one pass but was read as a limit on the reel, and
that made the grounder above unable to finish on any real compilation.** `ground` indexed the
budget absolutely, so a reel with more segments than the budget ground its first N on every pass
and never advanced past them; the tail stayed ungrounded and `AutoConfirmable` returned
`RejectUntagged` forever. Measured on the maintainer's catalog against a default budget of **60**,
live proposals hold **82, 133, 142, 222, 235 and 303 segments** — so the budget silently meant
"reels this size can never auto-confirm", which is the same class of default-ON-but-cannot-fire
failure V54 exists to end.

Three changes make the budget behave as the per-pass cost bound it was always described as:

1. **Grounding PERSISTS on the proposal.** `SplitSegment` already carried `Category`/`Era` and
   `segments_json` is a plain JSON blob, so this is additive — no migration. One new field,
   `Looked`, records that the grounder examined a segment *whether or not it came back with
   anything*. ⚠ Inference cannot replace it: `Category != ""` conflates *never looked at* with
   *looked at and grounded nothing*, and treating those alike is exactly what makes a resumable
   budget never converge.
2. **A partly-grounded reel DEFERS** (the fourth verdict, above) rather than being judged. It
   returns to the belt with its progress recorded — *"looked at 60 of 142 cuts"* — and the next
   pass resumes at segment 61. A 303-segment reel completes in six passes.
3. **Termination is progress, not a counter.** A defer requires that the pass looked at something,
   and every look marks a segment, so the pending count strictly decreases. A pass that achieves
   nothing (vision off, no vocabulary, the provider down at the first segment) does not defer: it
   falls to the gate and the reel parks with a real reason. No new column, no timestamp, no
   retry budget.

⚠ **The write must never insert.** Grounding is a read-modify-write spanning minutes of vision
calls, so it races `Confirm`. `UpdateSplitProposalSegments` is an `UPDATE` returning `ErrNotFound`
on zero rows, deliberately NOT the `INSERT … ON CONFLICT` upsert — a grounding write landing after
a confirm would otherwise **resurrect** the proposal: a pending review for a reel already cut,
pointing at a composite whose segments are in the catalog. It also leaves `created_at` alone, since
the Incoming queue orders by it and a reel must not jump the queue for having been grounded.

⚠ **An existing proposal is RE-GROUNDED, never re-detected.** The split rung used to return
immediately when a proposal existed, which read as "leave the operator's cut list alone" and was in
fact a dead end: the row went to `review`, `ListPipelineWork` only claims `running`, and nothing
reached that reel again. Every compilation detected before the grounder existed was therefore
permanently ungroundable. Re-detection stays the operator's call (`POST /v1/filler/split`) because
a rung must not redraw a cut list a human may have open; grounding is additive and touches no
boundary.

⚠ **…and a re-detect returns the reel to the belt (V54a).** The paragraph above was only half a
remedy. `POST /v1/filler/split` does replace the proposal — `filler_split_proposals.clip_hash` is
UNIQUE and the upsert conflicts on it, so a fresh scored cut list lands in place of the stale one —
but detection writes no pipeline row. A reel parked at `split`/`review` therefore stayed parked,
now holding a proposal nothing would ever read, and the documented remedy could not work. The
operator path un-parks the row itself: `disposition='running'`, `status='queued'`, `attempts=0`,
`next_run=0` — the same four columns migration 00050 set, for the same reasons, including giving
back attempts spent losing to a gate that could not be won.

⚠ **After detection, never before.** An un-parked row is claimable, and claiming it mid-detection
would let the split rung ground the OLD segment list — or call `Propose` a second time on the same
reel — while the new list is still being written. So the un-park is the last step of a successful
detection, and a detection that fails leaves the row exactly as it found it: parked, with its
original proposal, which is the honest state.

⚠ **Scope is migration 00050's `WHERE` clause**, for the migration's reason: `stage='split'` AND
`disposition='review'` is the unreachable state. A `rejected` row keeps its own restore path
(`Soft()`) — a re-detect must not quietly overturn a refusal an operator can see and argue with —
and a row already `running` has nothing to un-park.

⚠ **Why this cannot be a migration.** 00050 performed exactly this un-park, once, and it is the
worked example of why the mechanism was wrong: it ran at 07:37 on 2026-08-12 under the binary that
preceded per-segment confirm, the old all-or-nothing gate re-parked all 17 reels within nine
minutes, and goose recorded it applied forever. A data migration cannot be re-run and cannot know
which binary it is firing under. **A state transition whose correctness depends on the running code
belongs on an operator path or a job, never in goose.**

⚠ **Side effect worth having: the gate's inputs became observable.** `GET
/v1/filler/splits/{proposalId}` returned only `endMs/index/name/startMs`, so a grounded and an
ungrounded proposal read identically and the only way to tell whether the grounder had run was to
watch for an `ffmpeg … thumbnail=n=` process. That is why V54's own shipping went unverified.

### Break & pod policy (per channel)

Break and pod policy and break placement moved to
[`design/scheduling.md`](design/scheduling.md#breaks-and-pods). Filler acquisition runs and
readiness stay here until the filler docs move.

- **Durable acquisition and filler readiness (V59).** Every accepted download creates a durable
  acquisition run before background work begins. The run records its trigger, registered source or
  approved pull attribution, requested/downloaded/skipped/failed/empty counts, and queued/running/
  terminal state. Its id travels beside the downloaded bytes in the Loomarr sidecar, through
  compilation splitting, and into each resulting pipeline row. Reconnecting clients therefore read
  the current run and its preparing/needs-decision/ready/complete/rejected/dismissed outcomes from the
  store; SSE remains a latency hint and is never the only history. A job whose initial run record
  cannot be persisted does not start. On single-replica startup, queued/running rows left by the
  previous process become terminal interrupted errors rather than appearing active forever.
  Manual and pre-V59 files honestly retain no acquisition id.

  The acquisition work is an application-generation-owned interactive operation: request
  disconnect does not stop an accepted download, but generation shutdown cancels and awaits it.
  Its bounded terminal update cannot outlive shutdown and overwrite the replacement generation's
  interrupted recovery. Compilation detection follows the same ownership rule and records a
  durable operation keyed by the `jobId` returned from `POST /v1/filler/split`; that operation maps
  success to the separately persisted Proposal id and records cancellation, timeout, or failure.

  The Filler entry point presents one server-owned readiness projection built from the live fetch
  limits, the pipeline lifecycle overview, the playable pool, per-channel coverage (including
  usable duration and grounded category/brand variety), and recent acquisition runs. It returns
  one typed highest-impact next action in this order: repair a stopped
  acquisition path, retry failed machine work, make pending operator decisions, add an empty
  airable pool, improve the weakest live channel, or no action when ready. A terminal rejection
  remains audit; only rows carrying the domain's explicit retry/restore action count as recoverable
  work. Clients render that decision; they do not reconstruct health from raw counters. A latest
  failed acquisition is an
  actionable machine failure rather than a transient toast. Detailed execution history, the
  catalog, sources, and taxonomy remain available through progressive disclosure.

  A channel's Filler section renders its saved coverage before matching controls, making inherited
  behavior the ordinary path. Per-clip overrides stay collapsed under explicit `Prefer on this
  channel` and `Exclude from this channel` language; they are never presented as required setup.
  Every real Clip segment in the assembled break preview is individually playable through the same
  content-hash media route and shared player as the catalog. The embedded fallback card is not
  playable, and preview does not create a separately stitched Pod asset.

  Exact Clip details resolve the registered source identity to its human label and keep the canonical
  source id as transport/debug identity rather than primary UI. The detail response may also project
  dimensions, cadence, codecs, container, channel/rate, and byte size from the already-validated
  durable playback lineage in the sidecar. It does not run ffprobe during a read. These file facts sit
  under progressive disclosure; the ordinary card keeps the short quality label.

  Acquisition is not readiness. These records and summaries do not weaken registered-source
  enablement, disk/catalog limits, grounding, required checks, or the held-to-Ready transition.
  Machine work, genuine operator decisions, completed Composite containers, and Ready catalog content remain distinct even
  when the simple overview brings them onto one page.

### AI assist (optional, opt-in)
Two jobs the suggester (§8) can do here, both under the same grounding rule (can only reference clips that actually exist in the filler catalog):
1. **Classify/tag** ingested filler so matching works without manual tagging.
2. **Assemble pods** matched to a block's vibe, and flag gaps — "the Saturday-morning channel has no 80s toy ads" — so you can point the `FillerSource` at a playlist to fill them.

---

## 11. Users, authentication & permissions

Moved to [`design/auth.md`](design/auth.md), with decisions 0007, 0011 and 0012. The auth negatives in
§19 stay there until §19 is split.

---

## 12. Web UI

Moved to [`design/web-ui.md`](design/web-ui.md). Per-view descriptions were deleted; the mocks and
components own them.

---

## 13. Onboarding & documentation

The settings decision, operator wizard and member first run moved to
[`design/web-ui.md`](design/web-ui.md), with decision
[0008](design/decisions/0008-settings-live-in-the-app.md). The documentation set is described by
`docs/README.md` and the contributor docs, not here.

---

## 14. Technology stack (decided)

Moved to [`design/dependencies.md`](design/dependencies.md); §14.1's structure rules moved to
[`design/overview.md`](design/overview.md#backend-structure-rules). The suggester certification rules
that followed §14.1 moved to [`design/suggester.md`](design/suggester.md#evaluation) (discovery
feedback and evaluation) and the archive
([`suggester-certification.md`](engineering/archive/design-2026-09/suggester-certification.md)).

### 14.2 The package map

Moved to [`design/package-map.md`](design/package-map.md). The hand-kept role table that stood here
was a stale second copy of the generated map and was deleted in #779.

## 15. Configuration — layered settings

**Full subsystem design — registry schema, resolution semantics, secrets lifecycle, Settings IA, wizard integration — lives in `config-design.md`.** Every setting resolves **`env > database > default`**, per key, through one **typed settings registry** (backed by the §5 settings store; all subsystems read via the settings service, never `os.Getenv` directly). An env var that is set wins and **locks its UI field** ("set via environment"); unset, the setting is managed in the app (§13). Runtime application is per key: connection operations snapshot live values, policies resolve per run, and generation-scoped resources wait for restart. `filler.dir` + `filler.watch_dir` are the first app-managed generation-scoped pair; bootstrap keys below remain restart-scoped for their pre-database reason.

### Bootstrap (needed before/independent of the DB) — `env > file > default`

*Revised by V5: these were env-**only**. The wizard's Database step must persist which
database to use and cannot write into the database it is choosing, so a narrow file tier
sits beneath env — `bootstrap.json` in the data directory, bootstrap keys only, env still
wins. See `config-design.md` §1. Every app-managed setting is unchanged at
`env > database > default`.*

| Env var | Required | Example / default |
| --- | --- | --- |
| `DATABASE_URL` | no | **default `sqlite:///data/loomarr.db`** / `postgres://…` |
| `AUTO_MIGRATE` | no | `true` |
| `LISTEN_ADDR` / `LOG_LEVEL` | no | `:8080` / `info` |
| `TZ` | no | container time zone; time-slot schedules computed here (§9) |
| `LOOMARR_ENCRYPTION_KEY` / `LOOMARR_ENCRYPTION_KEY_FILE` | no | Base64url 32-byte installation key, or a file containing it. Both set is an error. Neither set atomically creates `/data/encryption.key` at `0600`. Every PostgreSQL replica must share the same key; the database retains only its fingerprint. |
| `LOOMARR_ENCRYPTION_KEY_PREVIOUS` / `LOOMARR_ENCRYPTION_KEY_PREVIOUS_FILE` | no | One-boot installation-key replacement input. Supply the old key here and the new key through the ordinary current-key variable; Loomarr atomically rewraps all DEKs, then this input must be removed. Both previous forms set is an error. |

**Zero required env** for a SQLite first run: `docker run -v loomarr-data:/data loomarr` → wizard.

### Database secret encryption

Every reversibly stored database secret is sealed through one Grafana-style envelope-encryption
module before persistence. Versioned AES-256-GCM ciphertext is authenticated against its record
kind, stable identity, and field name. Wrapped data-encryption keys live in the database; the
installation key that wraps them does not. Database-only backups therefore contain ciphertext but
require the separately preserved installation key to restore credentials. A wrong or missing key,
unknown envelope, or failed authentication prevents readiness and never clears the value.

Data-key rotation activates a fresh key for new writes before existing ciphertext is migrated.
Installation-key replacement rewraps the data keys while old and new material are explicitly
available for that maintenance operation. Access and devices exposes encryption status, a non-secret
fingerprint, and data-key rotation only; provider setup never exposes key management. Full host or
volume compromise containing both database and installation key is outside this protection boundary.

### Generated secrets (created at first migration; view/regenerate in Settings; env override optional)

| Setting (env override) | Notes |
| --- | --- |
| `API_TOKEN` | Machine access + break-glass admin (§11) — the Sonarr API-key model. |
| `PLAYOUT_TOKEN` | Read-only device credential for tuner and segment URLs (§9.1). |

### Application settings registry (UI-managed; each key's env name pins it)

| Setting (env name) | Default / example |
| --- | --- |
| `LIBRARY_FLAVOR` / `LIBRARY_URL` / `LIBRARY_TOKEN` | `emby` \| `jellyfin` / `http://emby:8096` / *(secret)* |
| `REQUESTER_PROVIDER` | `seerr` \| `arr` — selects the acquisition backend (load-bearing, like `LLM_PROVIDER`); gates which fields show. |
| `SEERR_URL` / `SEERR_API_KEY` | `http://seerr:5055` / *(secret)* — when `REQUESTER_PROVIDER=seerr` |
| `SONARR_URL` / `SONARR_API_KEY` / `RADARR_URL` / `RADARR_API_KEY` | direct requester (`REQUESTER_PROVIDER=arr`): TV→Sonarr, movies→Radarr / *(secrets)*. Optional `SONARR_QUALITY_PROFILE`/`SONARR_ROOT_FOLDER` (+ `RADARR_*`) pin the profile/root; blank = the arr's first. |
| `TUNARR_URL` | `http://tunarr:8000` (Tunarr has no auth; no key config) |
| `TUNARR_TRANSCODE_CONFIG_ID` | Tunarr transcode-config uuid created channels reference (Phase-0: channel create requires a valid `transcodeConfigId`; empty → resolve the instance `Default` via `GET /api/transcode_configs`, §9) |
| `SERVER_PUBLIC_URL` | **Re-scoped by §9.1 — no longer icon-only, and no longer Advanced.** Loomarr's own address as your media server *and* Tunarr reach it (e.g. `http://loomarr:8080`). **The rule (#1447): external consumers use the public URL; Loomarr's own process uses the listen address.** The stream URLs handed to a media server or Tunarr are built from this base, so a wrong value means channels appear in the guide *there* and never play. Loomarr's **in-app playback does not use it**: the playout session's parent process fetches its own programme blocks from the bound `LISTEN_ADDR` over loopback (a wildcard bind dials `127.0.0.1`), so a stale or hairpin-blocked public address cannot stall a tune. Only a build with no listener (embedded/test) falls back to this value. Current Health carries a non-required **Public address** check (`public_url`): it requests `/v1/healthz` at this address and, when unreachable, says "This server can't reach its public address … — check SERVER_PUBLIC_URL" with remediation `/settings/system/playback`. It is skipped at boot (the listener does not exist yet during assembly) and runs from the first Current Health refresh. A tune that fails to start returns a 502 problem whose `detail` names the cause (programme source unreachable or refused, encoder exited, no stream); the player shows that reason with Retry instead of retrying, while a 404/empty playlist stays the ordinary warm-up retry. Once three consecutive block opens for the first block fail, the tune fails at once rather than waiting the 45s first-segment deadline. Still also used for **uploaded** channel icons — the stored icon URL is built from this, never from request headers (Host-injection-safe). Deliberately ONE key rather than a second `playout.public_url`: it is genuinely the server's own address, both callers need the same value, and two keys could drift. Empty → a relative `/v1/channels/{id}/icon` URL for icons (works when Tunarr shares Loomarr's origin); a media server or Tunarr consuming internal playout requires it set (in-app playback does not). |
| `ACCESS_PUBLIC_URL` | Empty by default. The global `access.public_url` setting is the absolute `http`/`https` browser origin recipients can reach (for example `https://loomarr.example.test`), used to construct Invitation and local-recovery links (§11). Its General editor is labelled **Recipient-facing Loomarr address** and, when the setting is empty, stages the current browser origin as the default. The operator saves that value or changes it when recipients reach Loomarr elsewhere. The backend never infers the value from request headers, so async email delivery always uses explicit persisted configuration. It is intentionally distinct from `SERVER_PUBLIC_URL`, which may be a container-only machine-client address. Empty suppresses email and shareable-link generation with an actionable Settings → General link; it does not prevent direct account creation/import or storing an Invitation reservation. Notifications consumes its readiness state but does not own its editor. |
| `JOB_NOTIFICATION_DELIVERY_SCHEDULE` | `*/15 * * * * *`. How often the provider-neutral worker claims queued account-message Delivery attempts. The worker drains a bounded batch per run; retry availability remains the fixed policy above rather than another setting. |

**One-time SMTP upgrade input (not application settings).** The legacy environment variables
`NOTIFICATIONS_EMAIL_ENABLED`, `NOTIFICATIONS_SMTP_HOST`, `NOTIFICATIONS_SMTP_PORT`,
`NOTIFICATIONS_SMTP_SECURITY`, `NOTIFICATIONS_SMTP_USERNAME`, `NOTIFICATIONS_SMTP_PASSWORD`,
`NOTIFICATIONS_EMAIL_FROM_ADDRESS`, and `NOTIFICATIONS_EMAIL_FROM_NAME` retain their former parsing
rules only for the one upgrade inspection described in §11. They do not pin, recreate, or update a
provider after that inspection. New installations configure SMTP only through **Settings →
Notifications → Add provider**.

**Playout (§9.1 — added with internal playout).**

| Env | Meaning / default |
| --- | --- |
| `PLAYOUT_BACKEND` | `internal` (default) or `tunarr` — who streams a channel. **Overridable per channel** via `policy.playout.backend`, which rides `policy_json` (no schema change, like `rules`/`filler`/`window`/`autoCurate`). Nil per-channel means **follow the live global value**; changing the default moves every inherited channel, while an explicit per-channel value pins that channel. |
| `PLAYOUT_ENCODER` | ffmpeg encoder (e.g. `libx264`, `h264_vaapi`, `h264_nvenc`). Empty ⇒ the best one the transcode check found. |
| `PLAYOUT_AUDIO_LANGUAGE` | `eng` (default) — ISO 639-2 code for the preferred audio track. A **preference**: an optional map plus a first-track fallback, so a file with no track in that language still gets audio rather than failing to encode. Empty ⇒ ffmpeg's own choice, which picks the track with the **most channels** and ignores language entirely — that is how a 5.1 Russian dub beats a 2.0 English track (§9.1). |
| `PLAYOUT_FFMPEG_PATH` | `ffmpeg` — the binary playout executes. Deliberately **separate from `INGEST_FFMPEG_PATH`**, though ⚠ **not for the reason this row used to give** (it cited the filler sidecar bundling its own ffmpeg in a different image — there is one image now, §16, so that rationale died with the sidecar). The live reason is that the two fail differently: playout's ffmpeg is a runtime dependency of a channel that is **on air**, ingest's is a dependency of a download nobody is watching, so repointing one must not be able to break the other. Advanced; the default is right whenever ffmpeg is on `PATH`. |
| `PLAYOUT_QUALITY_TIER` | `balanced` (default) / `efficient` / `quality` — the picture-vs-channel-count target. Resolved at each program boundary against measured capacity and current load, so quality adapts as channels come and go rather than being fixed per channel (§9.1). |
| `PLAYOUT_PREPARED_DIR` | **Retired (#1512 phase 4)** with prepared media; ignored if still set. |
| `PLAYOUT_PREPARED_BUDGET_GB` | **Retired (#1512 phase 4)** with prepared media; ignored if still set. |
| `PLAYOUT_MAX_CHANNELS` | `0` (automatic) — use the encoder trial's measured concurrent-transcode capacity. A positive value is an optional safety cap and may only lower that measurement, never raise it. A test pattern encodes cheaper than film grain, so lower the cap if real content cannot sustain the measured budget. |
| `PLAYOUT_TOKEN` | **Generated secret** (§11 device auth), viewable because it must be pasted into a tuner/listings URL by hand. Signs every segment request so only your media server can pull a stream. Distinct from `API_TOKEN`: that is break-glass **admin** with full authority; this grants nothing beyond reading streams. |

**Backup (§16 — added with the Backup UI).**

| Env | Meaning / default |
| --- | --- |
| `BACKUP_SCHEDULE` | `0 30 3 * * *` — nightly database backup. It contains settings, channels, people, encrypted secrets, and wrapped DEKs, but not the external installation key. It does not contain filler, prepared media, cached artwork, or operator image uploads; protect `/data` separately when those files matter. |
| `BACKUP_RETAIN` | `7` — how many to keep before pruning the oldest. |
| `BACKUP_DIR` | `/data/backups` — inside the documented volume by default; point elsewhere to keep backups off the database's disk. |
| `LLM_PROVIDER` / `LLM_URL` / `LLM_MODEL` | `ollama` \| `openai` / base URL / model id. **`LLM_PROVIDER` is load-bearing** (selects the client). For `openai`, `LLM_URL` is the OpenAI-compatible **base URL** (a hosted `…/v1`, or Ollama's own `http://ollama:11434/v1`). Local default: `ollama` + `qwen3:8b` (or `qwen3:14b` at **Q6_K** — stock Q4 degrades tool-calling/JSON). **Initial defaults only:** an in-app selection (§8.1) persisted to the settings store (`llm.provider`/`llm.url`/`llm.model` + per-provider secret `llm.api_key.<provider>`) **overrides** them and hot-swaps the running suggester, so a UI choice survives a reboot without editing env. |
| `LLM_API_KEY` | *(secret; read for `LLM_PROVIDER=openai`. An in-app hosted selection stores its own per-provider key in the settings store, overriding this — §8.1; **never echoed** by any API.)* |
| `LLM_KEEP_ALIVE` | `30m` — how long a **local** Ollama model stays resident between calls (§8.2). Loading an 8B model costs ~9s vs ~0.5s warm, and Ollama unloads after 5m idle, so the stock behavior makes a describe→read→refine cycle re-pay the load every time. `0` disables (stock unload) for a memory-tight host. Ignored by hosted providers, which have no residency to manage. |
| `TMDB_API_KEY` | *(secret; enables TMDB search, channel icon suggestions, and AI grounding — required if the suggester is enabled)* |
| `IMAGES_DIR` | `/data/images` — where the image service (§22) stores originals and derivatives, inside the documented volume. ⚠ **Not covered by the application backup**, which is a database backup: `/data` is one volume and the volume is what to back up. Everything here is regenerable or re-fetchable **except** operator uploads. |
| `IMAGES_MAX_UPLOAD_BYTES` | `8388608` (8 MiB) — the ceiling on an uploaded image, enforced on the read as well as the declared size. |
| `IMAGES_REMOTE_FETCH_ENABLED` | `true` — whether to ingest remote artwork (TMDB, media-server) at all. `false` keeps the service to locally-produced images only; no outbound image requests are made. |
| `IMAGES_CACHE_BUDGET_MB` | `2048` — soft cap on the derivative cache before the GC job evicts least-recently-used renditions. Derivatives are always regenerable, so eviction costs latency, never data. |
| Image module policy (not settings) | AVIF/WebP/JPEG are always emitted; remote fetch concurrency is capped below the provider limit; fetched artwork never outlives the six-month compliance ceiling. These are compatibility, service-protection, and compliance invariants rather than user preferences. |
| `REQUEST_TTL` / `DOWNLOADING_TTL` | `48h` / `12h` |
| `CHANNEL_RECONCILE_EVERY` | `10m` — minimum delay after a successful channel rebuild before it is eligible for another scheduled sweep. The normal cadence control is the **Maintain live channels** task under System → Tasks, so this cooldown remains an advanced setting. |
| `JOB_SYSTEM_HEALTH_SCHEDULE` | `*/30 * * * * *` — every 30 seconds, refresh Current Health through the named System task. Probes run concurrently under bounded deadlines; changing this cadence also changes their freshness deadline rather than allowing an older observation to remain green indefinitely (§17). |
| `SESSION_TTL` / `COOKIE_SECURE` | `720h` / `auto` (§11) |
| `TRUST_PROXY` | `false` (§11) — trust `X-Forwarded-For`/`X-Forwarded-Proto`. Default `false`: the login rate-limit key and `cookie.secure=auto` use the socket peer address, so forwarding headers can't be forged by a direct client. Set `true` only when a reverse proxy in front sets these headers. |
| `LOOMARR_METRICS_TOKEN` / `LOOMARR_METRICS_TOKEN_FILE` | *(unset)* — the bearer credential Prometheus presents to `GET /v1/metrics` and `/metrics` (§7). **Unset ⇒ the endpoint is refused (`403`) — fail closed**, and boot WARNs once naming this variable. `_FILE` follows the Docker-secrets idiom (contents trimmed; both set, or an unreadable file, stops the boot; errors never echo the value). Bootstrap-tier like `LOOMARR_PPROF`: it gates a surface that is unauthenticated by nature, and a registry key would be editable by any admin session. Deliberately separate from `API_TOKEN` — a scraper credential must not act as an admin. Generate one with `openssl rand -hex 32`. |
| `LOOMARR_PPROF` | *(unset)* — **development only.** `1` mounts `/debug/pprof/*` (§7). Unset ⇒ the routes do not exist. Bootstrap-tier for the same reason as `LOOMARR_DEV_LOGIN`: it decides which routes are mounted, and a profiling surface an admin session could switch on at runtime would be a worse hole than the one it opens. Boot WARNs while it is on. |
| `LOOMARR_DEV_LOGIN` | *(unset)* — **development only.** `1` registers `POST /v1/auth/dev-login`, a credential-free admin sign-in (§11), and makes the login screen offer it. Unset ⇒ the route does not exist. Bootstrap-tier (read at boot, not hot-appliable): it decides which routes are mounted, and a bypass that could be switched on at runtime through the settings API would be a worse hole than the one it opens. Boot WARNs on every startup while it is on. |
| `JOB_WORKERS` / `JOB_TIMEOUT` | `1` / `10m` (§8). One suggestion at a time is the appliance-safe default because a local model may share CPU, memory, and GPU with playback or transcode. A larger or hosted-model deployment may deliberately raise it. |
| `JOBS_RETENTION` / `PROPOSALS_RETENTION` | `720h` / `2160h` (§5 housekeeping). |
| `ACTIVITY_RETENTION` | `720h` — how long Dashboard activity rows are kept before `housekeeping` removes them (§5, §18.1, V32). |
| `DIAGNOSTICS_DIR` | `/data/diagnostics` — persistent, diagnostics-owned Process-output files. The path is generation-scoped and applies after restart; changing it cannot split one generation's active Process runs across roots (§17). |
| `DIAGNOSTICS_RETENTION` | `168h` — how long Diagnostic events and completed Process runs remain available. Active Process runs are exempt regardless of age (§5, §17). |
| `DIAGNOSTICS_MAX_STORAGE_MB` | `512` — soft global budget for normalized Diagnostic-event payload plus bounded Process-output files. Housekeeping removes the oldest completed evidence until under budget; active Process runs remain protected even when that temporarily leaves the install over budget (§5, §17). |
| `episodes.max_age` | `24h` — how stale a cached series episode list may be before resolution and `channel-maintenance` re-enumerate it (§5). A miss or aged row attempts the live library call. On an aged refresh failure, a non-empty valid cache preserves playable/safety facts but disables editorial subset selection; an empty cache remains unavailable. |
| `SUGGEST_MAX_ACQUISITIONS` | `10` |
| `SCHED_WINDOW_HOURS` | `24h` (rolling-window horizon a channel materializes; per-channel/-rule overridable, `0` = the whole run — `programming-design.md` §6.5) |
| `FILLER_DIR` / `FILLER_SYNC_EVERY` | **`/data/filler`** / `15m` (§10). ⚠ **V38c: this is the CLIP FOLDER** — Loomarr's own store, holding `a3/f9/<hash>.mp4` plus sidecars, scanned directly by Loomarr and the only directory Loomarr rearranges. *(It briefly meant "the first watched folder" in V38c's intermediate model, before "Two folders, one pipeline" split arrival from storage. The key kept its name because its meaning — where the clips are — did not change; only the layout did.)* Tunarr-backed channels also receive this folder as a `local` source; that playout integration is not how the catalog discovers files. ⚠ **Defaults inside `/data`, like `DATABASE_URL` and `BACKUP_DIR`** — it was previously empty for no recorded reason, which made filler opt-in by accident: a zero-env install opened the Filler page on a single "no folder configured" empty state, hiding every shipped filler capability behind a config step. Created at generation build if missing (the scanner treats a missing root as fatal by design, so a default that did not exist would swap an honest empty state for a scan error). **Generation-scoped:** a saved replacement is desired immediately but every filesystem consumer keeps the applied root until restart. Changing it selects another library; it never moves the old library implicitly |
| `FILLER_WATCH_DIR` | **`""` ⇒ `<FILLER_DIR>/_watch`** (§10 V38c, "Two folders, one pipeline"). Where clips ARRIVE — downloads land here, operators drop files here — and Loomarr drains it into the clip folder on every sync. ⚠ **The default is derived rather than a literal**, so pointing `FILLER_DIR` at an existing library moves the watch folder with it instead of leaving it orphaned under `/data`. ⚠ **Underscore-prefixed and INSIDE the clip folder on purpose**: a sibling default would need a second mounted volume to survive a restart, and an unmounted watch folder loses anything not yet filed on the next restart — silently, because an empty folder is also what success looks like. The scan skips it by name, so a file waiting there is never catalogued from its arrival path. **Generation-scoped with `FILLER_DIR`:** saving both can never apply the new watch against the old root; one immutable pair takes effect after restart |
| `FILLER_BREAKS_PER_HOUR` / `FILLER_BREAK_DURATION` / `FILLER_POD_MAX` | `4` / `30s` / `4`. Break frequency is the inherited channel default (`policy.breaksPerHour`: absent = follow it, `0` = no breaks, positive = custom). Break length is also inherited (`policy.breakDuration`: absent = follow it, minimum `30s`) and never uses zero as off. Pod size is a global preferred clip count, automatically raised when the matching catalog's median clip duration needs more clips to fill the resolved break length. |
| `FILLER_INCOMING_READY_WINDOW` | `24h` — how long Ready clips remain visible as recent activity in Filler → Incoming. Bounded from `1h` through `720h` (30 days), hot-applied on the next read, and view-only: aging out never removes a Library clip or changes playback eligibility. |
| `FILLER_COOLDOWN_SECONDS` / `FILLER_WEIGHT` | `30` / `1` (Tunarr filler-list attach: min seconds before a clip repeats; relative draw weight across multiple filler-lists) |
| `FILLER_MIN_QUALITY` | `0` — minimum clip height in px for a commercial to be eligible (`480` excludes 240p rips). **`0` disables the floor, and that is the default**: quality is display-only unless an operator opts in, because a blanket "prefer HD" starves the era-accurate 4:3 commercials §10 exists to play (V17c) |
| `FILLER_MIN_DURATION` | `10s` — the quality gate's floor (§10 V40). A clip shorter than this is **rejected at the scan boundary** and never becomes a catalog row at all. ⚠ Distinct from `FILLER_MIN_QUALITY`, which is an opt-in *eligibility* filter over clips that already exist: this one rejects, and its default is ON. It exists because `DurationMs <= 0` was the only guard, and a 2.9KB / 33ms truncated download passed it and sat airable in the catalog. ⚠ **It has a SECOND job since V54**: composed with `MinSegmentMs` as `max()`, it is also the splitter's detection floor (§10 V34 step 2). One number, two enforcement points, deliberately — the alternative is a segment the auto-confirm gate admits and the scan boundary then rejects, which is the shape `FILLER_AUTOSPLIT_MAX_DURATION` one row down also serves two jobs to avoid |
| `FILLER_SPLIT_REVIEW_WINDOW` | **`720h`** (30 days) — how long a split proposal's leftover cuts wait for review before `filler-split-sweep` gives up on them (§10 V54). ⚠ **The ONLY setting in Loomarr that deletes an operator's media**: when it expires, the leftover cuts are dropped AND the original recording is removed to reclaim the space (reels are commonly 1–2 GB). Bounded by three rules, all enforced rather than documented: only past the window; only for a recording that has ALREADY produced clips (a reel Loomarr could not use is the operator's only copy, and is never touched); and `0s` = never, which is the same off-by-explicit-zero encoding `FILLER_MIN_CLIP_DURATION` uses. The clips cut from a reel are never affected, and the catalog ROW survives as a tombstone so `parent_hash` lineage keeps resolving — only the bytes go. Told to the operator in `docs/help/filler.md`, which ships inside the binary |
| `FILLER_MIN_CLIP_DURATION` / `FILLER_MAX_CLIP_DURATION` | **`0s` / `0s`** — both OFF (§10 V51f). Pod-assembly *eligibility* bounds: a commercial outside them is not drawn into breaks automatically, but stays in the catalog, searchable and pinnable. ⚠ **Distinct from `FILLER_MIN_DURATION` above, on the other side of the catalog boundary**: that one refuses a file *entry*; these decide what an existing clip may fill. ⚠ **`Policy.MinClipMs`/`MaxClipMs` existed for several phases with no way to set them** — assigned in tests and nowhere else — so `durationEligible` always returned true and `PoolReport.Eligible`, which §10 headlines as "the number that surprises operators", was arithmetically identical to `Commercials` on every install ever run. The pool strip printed one number twice and presented the pair as a diagnosis. The max is the one worth setting: it is the guard against a three-minute infomercial filling a thirty-second gap |
| `FILLER_TARGET_LUFS` | `-23` — the broadcast loudness target filler is normalised to (§10 V40, §9.1). Measured spread across real fetched clips was −21.8 to −32.6 LUFS, about 11 dB of clip-to-clip jump. ⚠ **Applied at PLAYOUT by default** — the drop-folder holds the operator's own files, so Loomarr does not rewrite them unasked. ⚠ **ONE target for both stages**: `FILLER_CONDITIONING_NORMALIZE_LOUDNESS` reuses this value rather than declaring its own, or a clip normalised on file would be corrected again at playout toward a different number. Set empty to disable |
| `FILLER_CONDITIONING_NORMALIZE_LOUDNESS` | `false` — when on, the transcode rung rewrites the playback derivative with ffmpeg `loudnorm` at `FILLER_TARGET_LUFS`. It is independent of admission and never mutates the retained source master. The sidecar records `normalizedLufs` so restart replay does not repeatedly process the same derivative. The former auto-file-named key is retired rather than silently retaining a false relationship to publication. |
| `FILLER_VISION_ENABLED` / `FILLER_VISION_MODEL` | **`true` / empty** (§10 V44) — whether a clip's own frames are read, and by which model. Empty model ⇒ reuse `LLM_MODEL`, for an install whose main model already sees images. ⚠ **Neither row existed in this table until V54a**, though both settings shipped in V44; the omission is why the gap one row below went unnoticed. |
| `FILLER_VISION_PROVIDER` / `FILLER_VISION_URL` / `FILLER_VISION_API_KEY` | **all empty ⇒ vision uses the main LLM's provider, URL and key** — unchanged behaviour for every existing install (§10 V54a). Set them to point vision at a *different service* from the one that writes text. ⚠ **This gap was load-bearing.** `FILLER_VISION_MODEL` promised a vision model independent of `LLM_MODEL`, but the provider was built from `LLM_URL`/`LLM_API_KEY`, so the model name was the ONLY independent part: naming a local `llava:7b` while `LLM_URL` was a hosted endpoint sent an Ollama tag to that endpoint. Measured on the maintainer's stack — `llava:7b` → `https://openrouter.ai/api/v1` → **HTTP 401** on every segment, so split grounding had never once run and the gate refused every reel with *"a segment could not be classified"*. ⚠ **The key is NEVER inherited when `FILLER_VISION_PROVIDER` is set.** Declaring a separate vision service means declaring its own credentials: inheriting would send the operator's hosted key to whatever host they named, including `localhost`. ⚠ `FILLER_VISION_URL` empty with provider `ollama` resolves to the conventional `http://localhost:11434`, the same rule `ollamaBase` already applies to probes and pulls. |
| `FILLER_LANGUAGE` | `en` — the installation-level commercial language, chosen beside Location during setup or under Settings → Access and devices (§10 V40). It is searchable by friendly language name; provider/model details stay Advanced. A clip whose SPEECH is confidently something else is rejected; a clip with no speech at all is always kept, because a wordless visual spot has no language and those are often the best filler. Empty selects any language and disables the gate. The preference remains editable even when the detector is unavailable so setup records the household answer once; the backend reports and skips unavailable work without discarding that choice. |
| `ASR_PROVIDER` / `ASR_URL` / `ASR_MODEL` / `ASR_API_KEY` | **`whisper` / empty / `openai/whisper-large-v3` / empty** (§10). One speech-recognition choice supplies both language detection and optional timed clip transcripts. `whisper` uses the bundled local `INGEST_WHISPER_*` paths. `hosted` uses standard multipart `/audio/transcriptions` with `verbose_json`; empty URL reuses the selected §8.1 hosted provider, while an explicit URL uses only its own optional API key. The model is separate from `LLM_MODEL` because chat/vision and STT have different modalities. Hosted spans are split into sub-minute requests and timestamps are reassembled. The detected language is trusted only when at least one non-empty segment proves speech. A service without timed segments fails visibly and retries; it never falls back to invented cuts. Connected speech sends clip audio off the box and may incur cost; local remains the default. |
| `INGEST_YTDLP_PATH` / `INGEST_FFMPEG_PATH` | vendored paths in the image; **unset ⇒ looked up on `PATH`** (V38b), so a source build with the tools installed works without configuring anything. Overridable so an operator can run a newer yt-dlp than the image ships. `ffmpeg` is also the internal-playout encoder (§9.1), so pointing this at a broken binary degrades playout too. ⚠ **They gate DIFFERENT things** — see §10's "Two downloaders, two gates": ffmpeg alone enables archive.org; yt-dlp adds YouTube |
| `INGEST_WHISPER_PATH` / `INGEST_WHISPER_MODEL` | vendored paths in the image — the whisper.cpp binary and its model file (§10, §14, V34). Unset/unrunnable ⇒ compilation splitting's transcript-rescue step is unavailable: over-long segments surface to the operator as **unsplittable** in the review UI rather than being guessed at (coarse splitting still works — it needs only ffmpeg). Overridable like the other tool paths |
| `INGEST_TIMEOUT` | `30m` — per-item wall-clock ceiling so one wedged fetch cannot hold the pipeline forever. Ingest concurrency is pipeline-owned policy. |
| `FILLER_PIPELINE_MAX_CLIPS` / `FILLER_TRANSCODE_MAX_PER_RUN` / `FILLER_PIPELINE_MAX_WHISPER` / `FILLER_PIPELINE_MAX_VISION` / `FILLER_PIPELINE_MAX_SPLITS` | **`25` / `3` / `10` / `5` / `3`** (§10 V51b). The ingest pipeline's per-run budget. Each bounds ONE PASS, not the catalog, so a backlog drains over cycles — the property the per-job batch constants they replace were chosen to defend, with the numbers carried forward unchanged. ⚠ **Zero means NONE, a distinct state from the default**: it is the only way to say "never do this kind of work on this box", which matters most for the transcode budget — the rung that creates V66's evidence and playback derivatives while retaining the source master. (⚠ `FILLER_SPLIT_EVERY` is retired: splitting is a rung every long recording reaches as it is ingested, so "how often do we go looking" stopped being a question with an answer.) |
| `FILLER_AUTOSPLIT_ENABLED` / `FILLER_AUTOSPLIT_MIN_CONFIDENCE` | **`true` / `85`** (§10 V43, default flipped in V51b). Whether an unambiguous split is confirmed without a human, and the score every remaining segment must reach. Known duplicates and below-`FILLER_MIN_DURATION` fragments are discarded first; they are deterministic non-clips, not review decisions, and the preserved composite is the recovery path. ⚠ **This was OFF, and the note here argued for it**: cutting is destructive in a way tagging is not — a mis-cut clip plays half an advert. That risk has not changed; the evidence has. The gate remains strict (the remaining reel qualifies as a whole or none of it does, an ungrounded era disqualifies at every threshold, and a segment the detector admits it could not resolve sends the reel to a human) and its measured failure mode is refusing GOOD reels, not admitting bad ones. Off by default meant every compilation waited for a click the design says should be unnecessary. Its confidence threshold governs cut acceptance only and cannot make any child playable. |
| `FILLER_AUTOSPLIT_MAX_DURATION` | `120s` (§10 V43). The longest a segment may be and still count as advert-shaped. ⚠ Serves TWO jobs and that is why it is one key: it selects which catalog clips the split job even looks at (longer than this ⇒ a compilation worth detecting), and it is the ceiling every segment must clear for auto-confirm. A single number keeps those two answers from disagreeing — a clip the job considers too long to be an advert must not then auto-confirm as one |
| `FILLER_STRUCTURE_WINDOW_AUTHORITY_PATH` | **empty** (§10 V67). Optional absolute path to the separately reviewed long-reel materialization-authority JSON. Empty, missing, malformed, drifted, or non-authorizing evidence enables no certified slice. The file is loaded at generation start and therefore requires restart after replacement. A valid authority permits independently assessed long-reel proposals to use the certified complete-plan gate. Without matching authority and a verified decision, automatic materialization holds; no compatibility fallback exists. Materialization can create held children but grants no training or broadcast admission. |
| `FILLER_STRUCTURE_WINDOW_DEPLOYMENT_PATH` | **empty** (§10 V67). Optional absolute path to the content-addressed long-reel deployment JSON that binds the reviewed authority to two exact OpenRouter routes, reasoning modes, token bounds, reservations, and aggregate spend ceilings. It contains no credential; production uses the existing OpenRouter provider secret. Authority and deployment must both validate at generation start, and replacement requires restart. Empty, malformed, drifted, under-budgeted, or non-authorizing configuration performs no structure inference and enables no certified materialization. |
| `FILLER_FETCH_EVERY` | `6h` (§10 V38b). How often each registered source is polled for new items. ⚠ **`0` disables auto-fetch entirely** — the escape hatch for an operator who wants acquisition to stay manual, and the value to reach for before disabling sources one by one. ⚠ **V38c: this is now the DEFAULT, not the only value** — a source may override it, and `0` on one row means *that* source never auto-fetches. Inherit is NULL, never 0 |
| `FILLER_FETCH_MAX_PER_RUN` | `10` (§10 V38b). Items ONE source may pull per poll. ⚠ The bound that stops "add a source" meaning "download 8,000 files tonight" — an archive.org collection is thousands of items, and this is what makes it trickle rather than flood |
| `FILLER_FETCH_MAX_CATALOG_CLIPS` | `2000` (§10 V38b). Auto-fetch stops when the catalog reaches this. ⚠ Manual queueing and approved pulls still work at the limit: a ceiling on what happens UNATTENDED is not a ceiling on what an operator may deliberately do |
| `FILLER_STORAGE_LIBRARY_BUDGET_GB` | `0` (automatic) (§10, "Storage is reserved before Loomarr writes"). Soft allowance for Loomarr-managed filler media on its real filesystem: automatic means `min(10% of filesystem capacity, 20 GiB)`; a positive value is the operator's allowance. Hot-applies to new reservations. It never weakens the hard host reserve. The upgrade migration moves a verified stored value from the retired fetch-only disk ceiling to `filler.storage.library_budget_gb` and removes that obsolete key; its old env name and runtime path are not retained. |
| `FILLER_RESEARCH_ENABLED` | `true` (§10 context research). Find missing descriptive clip details from exact public source metadata and credential-free structured sources when a text model is configured. Turning it off stops future context lookups; it does not remove reports or affect playback. |
| `FILLER_RESEARCH_WEB_PROVIDER` / `FILLER_RESEARCH_BRAVE_API_KEY` / `FILLER_RESEARCH_SEARXNG_URL` | `none` / *(secret)* / empty (§10 #1349). Optional last-resort search after structured evidence cannot supply the requested context. Provider is `none`, `brave`, or `searxng`; Brave uses the fixed API host and SearXNG uses the operator's HTTPS endpoint. Secrets are masked and replace-only. |
| `FILLER_RESEARCH_MONTHLY_LIMIT` | `100` (§10 #1349). Maximum general-web search requests reserved in one UTC calendar month. Structured sources do not consume it; connection tests and pipeline fallbacks do. The server enforces the limit atomically before dispatch. |
| `FILLER_SOURCE_FOLDER_ENABLED` | `true` (§10 V35). The drop-folder's on/off switch. It is a setting rather than a row because the folder is **derived from configuration** — a remote collection's switch is a column on its own row. Disabling stops the catalog scan; ⚠ **it never removes clips already in the catalog**, and the enforcement lives in the syncer, not in the UI. ⚠ There is deliberately **no library equivalent**: nothing scans a media-server library for filler (§10), so the key would gate nothing |

**Secrets handling:** stored in the DB following ecosystem practice (Sonarr, Seerr); masked after save (replace-only in the UI), never logged, excluded from `/v1/setup/status`; env-supplied secrets may come from env or mounted files (`<VAR>_FILE`), never baked into the image. This table mirrors the code registry — a setting that isn't here doesn't exist (AGENTS.md do-nots). Full mechanics: `config-design.md`.

---

## 16. Deployment (Docker)

The image, Compose and upgrades moved to [`design/deployment.md`](design/deployment.md); backup and
restore to [`design/storage.md`](design/storage.md#backup-and-restore). The operator runbook and
Compose listing were deleted: the wizard, `docs/install/` and `docker/compose.yaml` own them.
---

## 17. Observability

Moved to [`design/observability.md`](design/observability.md).
---

## 18. Concurrency & correctness

Moved to [`design/overview.md`](design/overview.md#concurrency-and-correctness).

### 18.1 The job scheduler — named, tunable, on-demand background work

Moved to [`design/deployment.md`](design/deployment.md#the-job-scheduler).
---

## 19. Testing strategy
- **Reference-backed Intent:** hermetic generic-web fixtures cover arbitrary public hosts, visible-text
  and title-anchor extraction, bounded bodies/excerpts/anchors, malformed and missing pages,
  cancellation, redirects, content types, and private-address/port/userinfo rejection. Suggester regressions
  use fictional programming concepts and titles to prove exact-title grounding, reference-data
  prompt isolation, zero-evidence rejection, request-scaffolding normalization, rationale-independent
  scoring, and no generic fallback when reference resolution fails. Unit and CI tests never contact
  the public web; an operator's household URL, prompt, Library, and resolved article bytes never enter a
  tracked fixture or training corpus.
- **Invitation and contact store conformance:** one shared suite runs unchanged over SQLite and
  Postgres. It covers normalized contact uniqueness, reserved local/Library identity collisions,
  lifecycle transitions, regeneration/revocation, expiry, verified-contact replacement, grant hashes
  never yielding a usable bearer, and two concurrent redemptions producing exactly one user/session.
  Migrations are forward-only and upgrade populated users without inventing contact verification.
- **Metrics scrape-token negatives (§7):** with a token configured, no credential, a wrong token,
  a prefix or superset of the token, the admin `API_TOKEN`, a member token and a session cookie
  alone each get `401` on both `/v1/metrics` and `/metrics`; the correct bearer gets the
  exposition. With no token configured every request — including one carrying a Loomarr
  credential — gets `403` naming `LOOMARR_METRICS_TOKEN` and no series. Neither the token nor a
  guess at it appears in logs, config errors or refusals. The composition-root test proves the
  same through `app.Build`.
- **Access security negatives:** members receive 403 for every Invitation/contact/delivery admin
  mutation; anonymous callers cannot inspect reservations or delivery status; disabled users lose
  sessions; unlisted Library accounts remain indistinguishable from bad credentials; imported
  recovery never sends or resets; public recovery responses do not enumerate eligibility; provider
  rejection during imported redemption never falls back; and no password, provider/session token,
  SMTP credential, or plaintext grant appears in SQL fixtures, logs, metrics, diagnostics, activity,
  RFC 7807 bodies, browser storage, or generated examples.
- **Notification certification:** a deterministic Delivery-means adapter pins intent idempotency,
  retry timing/classification, suppression, retention, and ambiguous-acceptance behavior without a
  network. SMTP integration runs against an in-process protocol server and covers unauthenticated and
  authenticated submission, required STARTTLS, implicit TLS, certificate rejection, plain-text plus
  HTML MIME, permanent recipient rejection, pre-acceptance transient retry, ambiguous post-`DATA`
  disconnect, cancellation, and redaction. Unit tests never contact a real SMTP provider.
- **Contract and frontend certification:** OpenAPI generation and orval types cover every new route
  and lifecycle shape. Stories use synthetic non-secret grants and cover configured/unconfigured
  email, delivery states, local/imported Invitations, invalid public grants, recovery, and QR/copy at
  desktop and mobile widths. Vitest pins URL cleanup and zero browser persistence; Playwright pins
  explicit-consent redemption, keyboard/focus return, the shared polite live region, forced colors,
  axe, and deterministic visual baselines.
- **Gate composition:** `make verify` is the single local verification interface. Its default is
  affected evidence; `make verify SCOPE=all` is the complete explicit local Go/Rust audit and is not
  the default edit-loop, task-start, or pre-publication ritual. Normal local and agent work uses
  focused tests while editing, then classifies the changed paths once and runs the affected evidence
  through the tool-neutral `make verify BASE=<base>`; `make agent-verify` remains a compatibility
  alias. The command reports the complete CI impact separately from the gates it executes locally
  and the specialized or platform-dependent gates left to protected CI; its final success line names
  only completed local gates. A newly selected impact key without an explicit disposition fails
  closed. Go changes run golangci-lint and tests over the same affected reverse-dependency package
  closure, so scoped verification catches static-analysis failures without expanding lint to the
  whole repository. `go_full` is an executable local scope modifier and reports a complete Go package set;
  shared-client changes run `make clients` locally, while Postgres, browser-container, native-client,
  and release-image gates remain explicit specialized or protected evidence. The pull-request fast
  lane and authoritative merge queue then provide the protected remote evidence.
  A maintainer requests `make verify SCOPE=all` deliberately when auditing the complete repository, changing
  the gate machinery itself, or diagnosing a classifier boundary. CI may run its
  `check-static` contract half and its race-policy-aware `test` half as parallel jobs, and may shard
  the latter or run independent runtime certification beside both, but the required aggregate
  succeeds only when every constituent succeeds. Splitting execution must not delete, skip, or
  weaken an assertion. The Go test module exposes one `make test` interface over four protected
  internal lanes: two measured ordinary-package shards run with bounded `-p=4` package parallelism,
  while two reviewed certification lanes run the latency-sensitive synthetic playout packages with
  `-p=1`. The lanes execute concurrently, so protecting media latency does not serialize unrelated
  repository packages. Each certification lane remains serial so independent synthetic targets
  never compete for the same worker while asserting latency and capacity; the two balanced groups
  use separate runners and therefore overlap without sharing worker resources. Ordinary shards are
  emitted in descending measured-cost order and the race-policy split must preserve that order, so
  the bounded Go scheduler starts critical packages before cheap work. Fixture-isolated top-level
  tests in `fillerreview`, `recurate`, `binder`, and non-integration `backendtransition` may use
  bounded `t.Parallel`; environment-mutating integration tests remain serial. Race instrumentation
  and the complete package partition remain unchanged.
  The Postgres Make target pins `-p=1` directly; its
  release-verifier contract requires that exact recipe and still rejects workflow environment
  overrides. Go runtime workers use the production Dockerfile's retained FFmpeg build and
  architecture-specific SHA-256 pins for both `ffmpeg` and `ffprobe`, rather than the runner's
  distribution package. The download archive is cached by its exact digest, verified on every
  use before extraction or execution, and its installed pair must report the declared build.
  Installer, Go workflow, and Dockerfile pin-source changes select the complete Go runtime gate. The
  Go workflow admits only the four literal lane identities; `scripts/go-test-lane.sh` owns their
  exact `-p=4` ordinary and `-p=1` certification execution policy and rejects lane-scoped `GOFLAGS`
  overrides. Go package shards use
  longest-processing-time assignment over the measured package seconds in
  `scripts/go-race-weights.tsv`, regenerated from hosted merge-group runs by
  `make go-race-weights`; unlisted packages receive a conservative one-second planning
  floor, so additions remain covered before their first hosted measurement. Every budget derives
  from #1570's ten-minute Go-only merge-queue target minus the measured queue overhead outside a
  lane's test step (`go-shard.sh --budgets`). The two ordinary-shard plan rejects aggregate
  package-seconds above four `-p=4` workers' share of that test step and a slowest lane more than
  25% above the lightest. A second model mirrors the runner's sequential race/non-race groups and
  bounded `-p=4` package workers; its makespan must fit the test step, with the same 25% balance
  bound. Each serial certification lane must fit the test step too, and the two certification
  groups may not differ by more than 25%. A per-package cap bounds any single package; it sits
  temporarily at `internal/store`'s measured time plus 10% because that package alone exceeds a
  lane's test step (#1570), and only a lane holding such a package is judged against the cap. The
  workflow independently caps every lane job at 15 wall-clock minutes,
  preserving six minutes for setup, compilation, and cache variance while making latency
  regressions fail loud. `go-shard-verify` proves the two ordinary shards plus both certification
  lanes remain an exact partition of `go list ./...` and enforces both modeled budgets; the
  release-verification suite pins the weighted assignment and executable descending-cost order,
  race-policy order preservation, certification package grouping, lane parallelism, workflow lanes
  and timeout. Lane-scoped CI invocations omit `make test`'s eager Rust
  worker and evaluation prerequisites: the repository-contract job runs `eval-contract` exactly
  once, and composition packages that exercise the production image protocol acquire the real
  debug worker through `internal/testkit` and pass it at the explicit application override seam.
  Unsharded local `make test` retains both prerequisites. The release
  verifier also requires every top-level job in `ci.yml` to appear in
  `ci-ok.needs`; adding a job without aggregating its result fails closed.
  SQLite store conformance builds one fully migrated, boot-seeded, clean template database per
  suite run, closes it, and gives every assertion a private file copy opened without replaying
  migrations. The copied stores remain physically isolated and exercise the same production SQLite
  driver, WAL configuration, schema, seeds, and Store implementation. Dedicated startup, migration,
  downgrade, historical-data, and restart tests continue to create and migrate their own databases;
  the template is a conformance-fixture optimization, never a production migration shortcut.
  PostgreSQL conformance follows the same isolation contract through its native database seam: one
  closed, fully migrated and boot-seeded template database is cloned into a distinct database for
  every assertion, and each clone is opened through the production PostgreSQL adapter without
  replaying migrations. Clone cleanup closes the Store before force-dropping only that disposable
  database from a separate maintenance connection. The template is never mutated, and dedicated
  PostgreSQL migration, preflight, and data-migration tests continue to create and migrate fresh
  databases. Real database cloning preserves PostgreSQL locking, independent pools, constraints,
  schema, and seeds; it removes repeated fixture construction, not backend fidelity.
  The captured-private-fixture regression guard remains one repository contract behind the stable
  `privacy-verify` Make interface. It enumerates the tracked tree once, extracts only the candidate
  shapes represented by the original capture audit, case-folds and SHA-256 fingerprints candidates
  in process, and reports only the stored label plus digest on a match. The plaintext private values
  are never checked in, emitted, or passed through one host process per candidate. Fixture-only
  household names and profile PINs remain scoped to the four audited response fixtures; widening
  those shapes to ordinary prose would turn a precise regression guard into a generic secret scan.
- **Proportional CI selection:** pull-request jobs may consume dedicated path-impact decisions only
  after their classifier has run in shadow, its complete path-to-gate sets are pinned by exact
  fixtures, and an activation verifier rejects falling back to the replaced broad selector. A
  missing or unresolvable diff base, classifier error, or unknown path selects every gate. Gates
  activate one job at a time so each change is independently reversible; an activated job remains
  a constituent of the single required aggregate, and activation must neither broaden nor shrink
  the explicit manual release-candidate scope. The combined Playwright job is selected by the union
  of its visual and end-to-end decisions. Before that selector activates, every shipping Web runtime
  source and browser build configuration is conservatively visual-sensitive; shared API/core/fixture
  inputs and the OpenAPI generator contract select both browser suites. Unit-test-only Web sources
  may skip Playwright, while visual/e2e tests and their committed baselines select their owning suite.
  The tuner job consumes its dedicated tuner decision. Every shipping Web runtime source remains
  tuner-sensitive until a committed dependency closure proves a narrower boundary; unit, spec, and
  story-only sources may skip tuner, while tuner e2e inputs, browser configuration, shared
  API/core/fixtures, runtime tokens, and OpenAPI select it directly.
  Apple mobile and Apple TV are separate required jobs with app-specific native build, install, and
  launch commands, and each consumes its dedicated decision. Splitting a matrix must preserve
  compatible cache-key identities and must preserve each native result as a separate aggregate
  dependency. Expo Android mobile is an independently selected required job consuming only
  `impact_expo_android_mobile`. Its reusable workflow generates and assembles only `CLIENT_APP=mobile`
  through the serialized native Make target, retains the standalone debug APK for seven days, and
  feeds the required aggregate. This build gate grants no mobile release or distribution authority;
  Android TV keeps its independently governed build and release path.
  Pull requests are the fast-feedback lane: they retain affected policy, repository,
  static-analysis, compile/type, unit, documentation, shared-client, and Android feedback, but do
  not repeat race-policy shards, Postgres conformance, Playwright, release-image builds, runtime
  image certification, or scarce macOS jobs. The required merge queue admits at most two cumulative
  builds concurrently and is the authoritative integration lane. The bounded second build removes
  serialized head-of-line delay while limiting the speculative rerun cost if an earlier entry fails.
  The same fail-closed impact decisions run against the generated
  `merge_group` commit, and the aggregate cannot pass unless every selected full gate succeeds.
  Explicit manual CI retains release-candidate and full recovery scopes. A normal queue-produced
  push to main runs publication workflows only; it does not launch product validation for a third
  time. The workflow must keep the `merge_group` trigger, repository protection must keep the queue
  active and apply to administrators, and normal changes may not bypass that admission boundary.
  Removing any of those coupled protections is a fail-closed delivery-policy change, not a
  performance tweak.
  CI orchestration and harness inputs select a dedicated policy gate rather than product builds.
  Rust, Android, Apple, browser, image, frontend, Postgres, and application-Go jobs run only when
  their classifier decision names a consumed input; unknown paths and classifier failures still
  select all gates. The policy gate actionlints workflows, exercises release verification and the
  agent harness, shellchecks scripts, and runs the Go-owned documentation contracts.
  Physical orchestration files follow the same ownership graph. The root `Makefile` contains only
  shared variables, the help interface, and ordered `mk/*.mk` includes; each included module owns
  one command family, and the command-reference generator follows those includes as the
  authoritative Make interface. The triggering CI workflow owns classification, admission, manual
  scopes, and the required aggregate, while product implementations live in family-named reusable
  workflows. A family workflow change selects its owning product gate plus policy. Changing the
  root Make interface selects every gate because it can alter every included command; changing the
  root CI orchestrator selects policy, whose structural verifier rejects changed admission,
  aggregation, or family wiring without rebuilding unchanged products. Unknown paths still select
  everything. Generated docs, action pinning, impact fixtures, and the release verifier reject an
  orphaned module or a caller whose reusable implementation is not covered by its owning decision.
- **State machine:** every transition + the five invariants.
- **Store conformance:** one suite vs **both** SQLite (private temp-file clones) and Postgres (**testcontainers**, private database clones), incl. `ClaimDue` concurrency (no record claimed twice). Both template factories migrate and boot-seed once, then open each isolated clone through its production adapter without migration replay; dedicated migration and lifecycle tests retain fresh databases. The race-enabled integration target carries an explicit 20-minute Go package timeout because the complete store package has exceeded Go's implicit 10-minute default on a two-core hosted runner, while a finite doubled ceiling still fails closed on genuine hangs.
- **Database lifecycle certification:** `make test-db-lifecycle` first runs that complete Postgres
  gate, then builds the shipped Loomarr image and drives isolated Compose projects through the real
  Traefik/HTTP boundary. It proves a fresh PostgreSQL install can write and restart; a populated
  SQLite install can preflight, back up, drain, copy, verify, persist its bootstrap target, restart,
  preserve authentication and application reads, accept PostgreSQL writes, and roll back; target
  failures recover on SQLite; a killed mid-copy process leaves SQLite usable and can retry after the
  disposable target is cleared; and an in-flight write drains into the snapshot while new admission
  closes for the maintenance window. Direct SQL is restricted to fixture setup and independent
  row-count, schema-version, and foreign-key fidelity checks.
- **Library conformance:** Emby vs Jellyfin flavors w/ mock transport; correct auth header each.
- **Webhook idempotency/replay:** duplicate/out-of-order events converge.
- **Scheduler reconcile:** desired-vs-actual against a **mock Tunarr** — idempotent (second reconcile = no-op), minimal-diff, and **backfill** (pending slot filled with filler → real title on `available` → re-push; `unavailable` → substitute). **Event-loss recovery:** drop the availability event entirely and assert the periodic sweep still backfills. Per-channel single-leader claim under concurrency.
- **Proposal workflow:** interface-level tests cover every legal Journey milestone and permitted action without reading raw tables. SQLite/Postgres conformance proves atomic claim, expired-running recovery, monotonically increasing Attempt tokens, stale-worker rejection, success/failure rollback, caller-owned cache cloning, and bounded history. Crash tests stop after claim, after model return, after Proposal insert, and after approval commit; restart converges to exactly one visible outcome. Previous-version fixtures remain readable; unknown versions and impossible approved-without-Channel combinations fail closed. Dropping every SSE frame does not change the authoritative result.
- **Lifecycle:** the downgrade guard refuses to start on a newer-schema DB; the janitor purges expired sessions/old jobs on schedule; `GET /v1/backup` (SQLite) yields a snapshot that restores to a working instance; deleting a scheduled item from the mock library → the sweep flags drift and substitutes.
- **Search:** `/v1/search` fans out to mock media server + mock TMDB + clip store; `in_library` flags correct; a member can search (read-only) but adding a missing title still routes through submit→approve; scope filters honored.
- **Suggestion grounding (critical):** mock LLM returns fabricated titles → **zero** unresolvable items reach a proposal, **nothing** unapproved reaches `/v1/titles`; already-present acquisitions filtered; `auto_approve` respects quota; output validates against schema.
- **Filler & pods:** catalog sync from a **mock media server's** filler library lands clips with duration + metadata; pod assembly is **seeded-deterministic** (seed = channel + window, so tests reproduce exactly) and respects era/audience matching, category variety, density, and no-repeat-in-window; the fallback ladder degrades gracefully to a bumper card; filler never appears as a lineup "program". Grounding applies to AI tagging and pod assembly (only real catalog clips).
- **Auth & roles:** bootstrap creates the first local admin and succeeds **exactly once** (a second call 409s while an admin exists); a **local** user logs in against its Argon2id verifier (or upgrades a successfully verified legacy bcrypt row to Argon2id in the same login); an **imported** media-server user prefers provider auth, refreshes its verifier after success, falls back only during provider unavailability, and never falls back after provider rejection; an **un-imported** media-server user is **rejected even with valid credentials** (the allowlist — no lazy self-provision); plaintext passwords/media-server tokens are never persisted; import is admin-only and creates rows, sync refreshes but **never adds**; `member` cannot hit approve/admin routes **or `POST /v1/titles`** (403 — the approval bypass is closed); disabling a user (directly or via sync of a server-disabled user) revokes their sessions immediately; `API_TOKEN` grants break-glass admin; ⚠ **an SSO identity with no allowlist row is rejected even with a valid provider token** (§11 V8 — the direct analogue of the un-imported media-server case), and no SSO login path creates a row.
- **Onboarding:** `GET /v1/setup/status` reports each integration pass/fail correctly against mocks (including the Tunarr media-source-matches-library check); a Sonarr/Radarr `Test` webhook with minimal payload is acked and flips the handshake check; a failing check carries an actionable hint + doc link.
- **API contract:** `/openapi.json` valid 3.1; served spec == committed `api/openapi.yaml` (fail CI on drift); spec `State` enum == code enum; `/docs` renders offline.
- **Frontend:** typed-client generation compiles; e2e smoke of approve flow vs mocked backend; SSE board updates on simulated `available`.

---

## 20. Open questions & follow-ons

Moved to #1576. Plans live in issues, not in this document.

## 21. Build plan for coding agents (phased, verifiable)

Deleted in #779: the phase plan shipped, and git history keeps it.

---

## 22. Image service — one pipeline for every image

Moved to [`design/images.md`](design/images.md), with decision
[0020](design/decisions/0020-one-image-service.md). The data model, width ladders and job list were
deleted: the migrations, the code and the scheduler registry own them. The frontend contract (bound
for `frontend-design.md`) and runtime certification (bound for `docs/dev/releasing.md`, #1572) stay
here for now.

### Frontend contract

One Layer-1 `Image` primitive consumes this service; no surface hand-writes an `<img>` against it.
Beyond `<picture>`/`srcset`, three properties are required rather than optional:

- **Explicit `width`/`height`**, from which browsers derive `aspect-ratio` — so cumulative layout shift
  is zero. This is free here: the API returns real dimensions and the roles have fixed aspects.
- **A `priority` mode.** ⚠ Lazy-loading the LCP image is the most common self-inflicted image
  regression on the web, and a blanket "lazy-load everything" rule walks straight into it. `priority`
  means eager loading with high fetch priority **and no async decoding** (async decode can defer the
  very paint being measured); the default is lazy, async, low priority. The first row of any poster
  grid is `priority`.
- **A built-in error fallback**, because `logo` values can be operator-pasted arbitrary URLs.

⚠ **Ship explicit `sizes`; do not use `sizes="auto"`.** Chrome and Firefox support it; **Safari does
not, in any version**. It is an Interop 2026 focus, so revisit — but not yet.

**Reaching the primitive from a resource that stores a URL.** `Image` takes the whole image record,
not a hash — real `width`/`height`, the ThumbHash and both srcsets are exactly what a URL cannot
carry. A resource whose field is a URL therefore carries the record ALONGSIDE it: `ChannelDTO` has
`logo` (the URL, unchanged) and an optional `logoImage` (the record, present only when the logo
resolves to one of this instance's images).

⚠ **Enrichment, never replacement.** Substituting a hash for the URL would permanently break the
external case, and the external case is not a legacy state — pasting an arbitrary image URL is a
supported way to set a channel icon. An external logo simply has no `logoImage`, and the surface
falls back to a plain `<img>`, which is the only honest rendering for bytes this instance does not
own and knows no dimensions for.

⚠ **The URL→record lookup VALIDATES, it does not merely parse.** The field is operator-writable
(`PATCH /v1/channels/{id}` accepts any string), so whatever is extracted is attacker-influenced and
is handed to the image store as a lookup key. Require a full 64-character lowercase hex hash;
anything else is treated as an external URL. Extracting "the path segment after `/v1/images/`"
without validating forwards traversal.

⚠ **Resolve the batch before the loop, never inside the per-item mapper.** A list endpoint maps once
per row, so a lookup inside the mapper is an N+1 — pre-resolve the distinct hashes for the whole
page. A failed lookup is an absent record, never an error: an image row lost with `/data/images` (see
*Durability*) must still let its channel render.

### Runtime certification

The required worker is release infrastructure, so its gate is broader than unit codec coverage.
`make image-cert` drives the installed `loomarr-image` executable through the same bounded protocol
and manifest validation used by the application, then writes a machine-readable report under this
worktree's `.artifacts/<instance>/` directory. It has two corpus modes:

- With no arguments, it uses the repository's deterministic certification corpus. That corpus
  covers opaque JPEG, transparent PNG, static WebP, animated GIF, APNG and WebP, finite and infinite
  loops, fractional and zero frame delays, a one-frame animated container, corrupt input, and every
  resource ceiling whose refusal is observable without allocating the forbidden resource.
- `make image-cert IMAGE_CERT_CORPUS=/absolute/read-only/path` scans an operator's existing raster
  corpus. It never modifies source files, follows no symlinks, performs no network I/O, and treats a
  supported-looking file that cannot complete inspection plus the requested ladder as a failure.
  Unsupported files are reported as skipped; stable budget refusals are reported separately from
  crashes, malformed manifests, and I/O failures.

Every accepted case produces an inspection plus a 320-pixel JPEG/WebP/AVIF ladder. Motion-preserving
WebP is required for an animated source; JPEG and AVIF must be the first composited presentation
frame. The certifier independently verifies the source hash, output signatures, dimensions, hashes,
motion flag, and that staging is empty after each case. A run fails on a worker crash, protocol or
manifest violation, unexpected refusal, leaked staging file, incorrect visible timeline, or a
resource ceiling breach.

The deterministic gate is intentionally generous enough to survive shared CI hardware while still
catching runaway work: each static case must complete within 10 seconds, each animated case within
30 seconds, and a worker process must remain below 768 MiB peak resident memory. The report records
per-case wall time, source and output bytes, peak RSS where the host exposes it, plus p50/p95/max
summaries. These are certification ceilings, not product SLOs; lowering them requires corpus evidence
and raising them is a design change.

Production exposes the same boundary at `/metrics`: worker operations and stable outcomes, wall
time, input/output bytes, peak RSS, queue wait, and in-flight count. Labels are bounded vocabulary
(`inspect`/`render` and stable result classes), never an image hash, path, URL, MIME supplied by a
caller, or free-form error text.

Those measurements are also the admission gate for another Rust capability. A proposal must name
the production operation that dominates a captured worker-duration, queue-wait, peak-RSS, byte, or
failure distribution, then reproduce it through the certification or benchmark seam. An operation
does not move merely because it handles media or because a Rust implementation is possible. The
Image service itself imports no Go `image` package; its deterministic corpus generator lives only in
`cmd/image-cert`, and a source architecture test keeps that boundary closed. Filler-era frame hints
remain in `internal/mediatools`: they sample at most 1,024 pixels per keyframe in background work and
no captured Image-worker evidence identifies them as a bottleneck. Therefore V59b selects no next
capability. A future measured case extends this worker protocol and resource boundary rather than
creating a second Rust service.

`make image-bench` is the performance companion to certification. It drives the installed release
worker through `internal/images/rustgen` with deterministic poster, backdrop, and icon sources and
the complete AVIF width ladder for each role. Source creation and the capabilities self-test happen
outside the timed region. After one warm-up per role, three runs record the recipe, host architecture
and logical CPU count, process/Rendition counts, output bytes, complete-ladder throughput,
p50/p95/max worker time, median ladder time, and maximum child peak RSS in a machine-readable report
under the worktree artifact directory. The current baseline deliberately performs one worker request
per missing AVIF Rendition, matching the background job shape that later batching will replace.

The benchmark is opt-in locally and manually dispatched on native amd64 and arm64 CI runners. It is
not part of comprehensive verification and has no wall-clock pass/fail threshold: shared-runner timing is comparative
evidence, not a product SLO or correctness gate. Comparisons are valid only for the same corpus,
recipe, release profile, architecture, and CPU profile. `make image-cert` remains the authority for
protocol, output, and resource-ceiling correctness.

The consumer half of the gate is observable through public seams. Guide programme art, Watch
timeline art, and Filler still/hover art must carry a real Image record through their HTTP DTO and
render through the shared frontend `Image` primitive. An animated filler hover must offer an
animated WebP rendition while its still fallback remains non-animated. Tests exercise those HTTP
responses and rendered elements; querying image tables or asserting private renderer calls is not
certification evidence.
