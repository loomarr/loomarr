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

The catalog, sources, fetching, storage, pulls, the clip lifecycle, the media gates, media roles,
conditioning evidence, language detection and acquisition runs moved to
[`design/filler.md`](design/filler.md), with decisions 0006, 0013–0016 and 0028. Compilation
structure, detection, split review, auto-split and composites moved to
[`design/filler-structure.md`](design/filler-structure.md), with decisions 0017 and 0029. Child
screening and tagging stay here until the filler-classification doc moves.

### Materialized children are screened (V67)

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

### Curation-grade metadata (V45)

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

### Ingest is a pipeline, and it is watchable (V51b)

Moved to [`design/filler-pipeline.md`](design/filler-pipeline.md), with V51d (the catalog listing),
V51e (the visible pipeline) and V51g (per-clip budgets). Decisions 0018 and 0019.

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
