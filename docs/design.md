# Loomarr system design

**Status:** Pointer file. This document was split into [`docs/design/`](design/README.md) (#779),
which is now the source of truth for how the system works and why. Each numbered section below
keeps its heading so old links and `design.md §N` citations still resolve, and says where its
content went. The last full version is
[`design.md` at `8031a52f`](https://github.com/loomarr/loomarr/blob/8031a52fe007992666c1dfcea28e8adc294f2620/docs/design.md).

Three release and testing passages (browser certification layering and Android TV distribution
under §9.1, the gate composition and CI selection under §19, and image runtime certification under
§22) are still here until #1572 moves them: testing and certification to
`docs/contributing/testing.md` and `docs/contributing/releasing.md`, Android TV distribution to
`project/android-release.md`. Until then they remain current;
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
in [`project/design-archive-2026-09/`](../project/design-archive-2026-09/README.md). Approval,
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
[`design/playback-clients.md`](design/playback-clients.md). Playback certification moved to
[`contributing/testing.md`](contributing/testing.md#playback-certification-is-layered), and the
Android TV artifact and build contract to
[`project/android-release.md`](../project/android-release.md#artifact-and-build-contract).

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
([`suggester-certification.md`](../project/design-archive-2026-09/suggester-certification.md)).

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
  everything, including a script under `scripts/` or a workflow that no rule names: neither
  directory has a narrowing catch-all, so a new shared input runs every gate until it is classified. Generated docs, action pinning, impact fixtures, and the release verifier reject an
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
Runtime certification (`make image-cert`, `make image-bench`) moved to
[`contributing/releasing.md`](contributing/releasing.md#image-worker-certification).
