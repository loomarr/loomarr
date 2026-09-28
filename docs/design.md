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
[`design/`](design/README.md). Gate composition and CI selection moved to
[`contributing/testing.md`](contributing/testing.md) (`make verify`, the layers) and
[`contributing/ci.md`](contributing/ci.md) (selection, the merge queue, sharding, `ci-ok`).

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
