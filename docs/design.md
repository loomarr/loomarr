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

Moved to four docs: [`design/filler.md`](design/filler.md) (catalog, sources, fetching, storage,
pulls, lifecycle, media gates, media roles, conditioning, acquisition runs),
[`design/filler-pipeline.md`](design/filler-pipeline.md) (the ingest pipeline and budgets),
[`design/filler-structure.md`](design/filler-structure.md) (structure assessment, splitting,
composites) and [`design/filler-classification.md`](design/filler-classification.md) (tagging,
taxonomy, readiness, screening, the release report). Decisions 0006, 0013–0019 and 0028–0030.

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
