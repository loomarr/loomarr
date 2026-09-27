# Loomarr system design

**Status:** Pointer file. This document was split into [`docs/design/`](design/README.md) (#779),
which is now the source of truth for how the system works and why. Each numbered section below
keeps its heading so old links and `design.md §N` citations still resolve, and says where its
content went. The last full version is
[`design.md` at `8031a52f`](https://github.com/loomarr/loomarr/blob/8031a52fe007992666c1dfcea28e8adc294f2620/docs/design.md).

Three release and testing passages (browser certification layering and Android TV distribution
under §9.1, the gate composition and CI selection under §19, and image runtime certification under
§22) are still here until they move to `docs/dev/` under #1572. Until then they remain current;
amend them in the same PR as the behaviour they describe. Add nothing else to this file.

**Audience:** Builders and coding agents. User-facing instructions live under
[Get started](get-started.md) and [`docs/guides/`](guides/install-docker.md). `CONTEXT.md` owns
vocabulary and `PROGRESS.md` owns work status.

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

Moved to [`config-design.md`](config-design.md#12-rules-carried-from-the-former-designmd-15), with
decision [0008](design/decisions/0008-settings-live-in-the-app.md). The settings table was deleted:
the generated [settings reference](reference/settings.md) is the key reference.

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

Each subsystem's test obligations moved to a "Tests that pin this" section in its doc under
[`design/`](design/README.md). The gate composition and CI selection text below is bound for the
testing docs under #1572 and stays here until then.

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

---

## 20. Open questions & follow-ons

Moved to #1576. Plans live in issues, not in this document.

## 21. Build plan for coding agents (phased, verifiable)

Deleted in #779: the phase plan shipped, and git history keeps it.

---

## 22. Image service — one pipeline for every image

Moved to [`design/images.md`](design/images.md), with decision
[0020](design/decisions/0020-one-image-service.md). The data model, width ladders and job list were
deleted: the migrations, the code and the scheduler registry own them. The frontend contract moved
to [`frontend-design.md`](frontend-design.md#8-the-image-primitive-formerly-designmd-22s-frontend-contract).
Runtime certification (bound for `docs/dev/releasing.md`, #1572) stays here for now.

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
