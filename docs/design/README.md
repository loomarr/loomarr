# Loomarr design

Loomarr turns "I want a channel that feels like X" into a running virtual TV channel and keeps it
populated as content comes and goes. The loop is **intent → grounded proposal → acquire what is
missing → build the schedule → play it out → keep it filled**, sized for one household: about 100k
library items, up to 50 channels, 20 users, 10k filler clips and one media server.

This directory describes **how the system works now** and **why the lasting decisions were made**.
It is replacing the single `docs/design.md`, one subsystem at a time (#779). Until a subsystem has
moved, its section in [`design.md`](../design.md) is still the source of truth.

## How to use these docs

- **Current behaviour only.** A subsystem doc says what the code does today. Amend it in the same
  PR as the behaviour change, before the code.
- **Decisions are records.** When a change settles a question whose reason will still matter, add
  a short record under `decisions/` (context, decision, consequences, what it supersedes). Records
  are append-only: a later decision supersedes an earlier one, and neither is rewritten.
- **Plans live in issues and milestones**, not here. Link the issue instead of describing the plan.
- **Generated facts stay generated.** The HTTP API reference is the OpenAPI spec
  (`api/openapi.yaml`, served at `/docs`), settings are in
  [`reference/settings.md`](../reference/settings.md), and the package inventory is
  [`package-map.md`](package-map.md). Do not restate them in prose.
- Vocabulary is defined in [`CONTEXT.md`](../../CONTEXT.md). Work status lives in GitHub issues.

## Subsystems

The **Old §** column maps the `§N` references in code comments to the doc that now owns them.

| Doc | Owns | Old § | Now in |
| --- | --- | --- | --- |
| [overview](overview.md) | scope, design envelope, ports, package-name traps, backend structure rules, HTTP API rules, concurrency rules | 1, 2, 7, 7.1, 14.1, 18 | here |
| [dependencies](dependencies.md) | every dependency and bundled tool, with its reason | 14 | here |
| [package-map](package-map.md) | generated package inventory | 2 (map), 14.2 | here |
| [acquisition](acquisition.md) | title identity, provisioning state machine, requesters, outbound client rules | 3, 4, 6 | here |
| [library](library.md) | media-server contract, media inventory, series episode cache, search | 5 (inventory), 6, 7.2 | here |
| [storage](storage.md) | one store with two backends, Postgres concurrency, retention, backup | 5, 16 (backup) | here |
| [suggester](suggester.md) | grounding, reference-backed Intent, explicit policy, providers, model selection and residency | 6 (reference source), 8, 8.1, 8.2 | here |
| [proposal-workflow](proposal-workflow.md) | approval ordering, Proposal Jobs, the Journey, failures, date interpretation, decision traces | 8 (execution, human-in-the-loop, traces) | here |
| [scheduling](scheduling.md) | channels, lineups, strategies, reconcile and backfill, guide freshness, airing history, breaks, pods and mid-programme breaks | 5 (airing history), 9, 10 (breaks) | here |
| [playout](playout.md) | the channel packager, backends, what is served, admission, playout status, Tunarr, Live TV wiring | 6 (Tunarr, Live TV), 9.1 | here |
| [playback-clients](playback-clients.md) | tuning, pause, Android TV | 9.1 (clients) | here |
| [auth](auth.md) | identity, credentials, invitations, notifications, devices, roles, route authorization, SSO | 11 | here |
| web-ui | navigation, first-run wizard, cross-view rules | 12, 13 | `design.md` |
| [deployment](deployment.md) | restart in place, the image, Compose, upgrades, the job scheduler | 9.2, 16, 18.1 | here |
| [observability](observability.md) | activity, diagnostics, correlation, client events, health, reports, metrics, discovery quality | 5 (activity, diagnostics tables), 17 | here |
| [images](images.md) | the image service | 22 | here |
| filler | catalog, lifecycle, gates, sources, pulls, clip identity | 10 | `design.md` |
| filler-pipeline | the ingest pipeline and its budgets | 10 (V51) | `design.md` |
| filler-structure | compilation structure, splitting, composites | 10 (V34, V45, V67) | `design.md` |
| filler-classification | tagging, confidence, readiness | 10 (V38, V44, V61–V63) | `design.md` |

Settings (§15) belong to [`config-design.md`](../config-design.md). Scheduling heuristics belong to
[`programming-design.md`](../programming-design.md), and the client architecture to
[`frontend-design.md`](../frontend-design.md).

## Decisions

Records are added under `decisions/` as each subsystem moves. Numbers were assigned in date order
when the map was approved on #779, so gaps close as the remaining records are written.

| # | Date | Decision |
| --- | --- | --- |
| [0001](decisions/0001-postgres-and-sqlite.md) | 2026-07-13 | PostgreSQL and SQLite are both first-class stores |
| [0002](decisions/0002-code-first-openapi.md) | 2026-07-13 | Code-first OpenAPI with Huma v2 |
| [0003](decisions/0003-federated-search.md) | 2026-07-13 | Search is federated; Loomarr builds no index |
| [0004](decisions/0004-grounding.md) | 2026-07-13 | The LLM never supplies trusted identity |
| [0005](decisions/0005-human-approval.md) | 2026-07-13 | Proposals need human approval by default |
| [0007](decisions/0007-loomarr-owns-identity.md) | 2026-07-13 | Loomarr owns identity; every credential path lands on one users table |
| [0009](decisions/0009-loomarr-plays-out.md) | 2026-07-25 | Loomarr plays out its own streams |
| [0010](decisions/0010-restart-in-process.md) | 2026-07-29 | Restart rebuilds in process, never exits |
| [0011](decisions/0011-sso-is-a-credential-path.md) | 2026-07-30 | SSO is a credential path, not a provisioning path |
| [0012](decisions/0012-routes-declare-their-role.md) | 2026-07-30 | Every route declares its role; anonymous is denied by default |
| [0020](decisions/0020-one-image-service.md) | 2026-08-09 | One content-addressed image service |
| [0021](decisions/0021-tuning-state-machine.md) | 2026-08-15 | Tuning is a latest-request-wins state machine |
| [0022](decisions/0022-pause-is-shared-time-shift.md) | 2026-08-17 | Pause is shared time-shift |
| [0023](decisions/0023-android-tv-watching-first.md) | 2026-08-22 | Android TV is watching-first with one permanent app identity |
| [0024](decisions/0024-react-native-clients.md) | 2026-08-23 | React Native and Expo for client binaries |
| [0025](decisions/0025-three-ai-pillars.md) | 2026-09-02 | Three independently certified AI pillars |
| [0026](decisions/0026-specialized-local-model.md) | 2026-09-02 | A specialized local model is an optional Suggester optimisation |
| [0027](decisions/0027-loomarr-owned-media-inventory.md) | 2026-09-05 | Loomarr owns its media inventory; the library is an importer |
| [0031](decisions/0031-tone-curve.md) | 2026-09-26 | Tone curve: one default plus six selectable curves |
| [0032](decisions/0032-4k-and-dynamic-range.md) | 2026-09-26 | 4K is a given; dynamic range is independent of resolution |
| [0033](decisions/0033-measured-admission.md) | 2026-09-27 | Admission is one measured ledger |
| [0034](decisions/0034-midroll-default-on.md) | 2026-09-27 | Mid-programme breaks on by default, with a per-channel off switch |
| [0035](decisions/0035-cpu-degradation-ladder.md) | 2026-09-27 | CPU degradation ladder: never refuse on a slow CPU |
| [0036](decisions/0036-watermark-active-picture.md) | 2026-09-27 | The watermark is anchored to the measured active picture |
| [0037](decisions/0037-one-packager-per-channel-format.md) | 2026-09-27 | One packager per (channel, format) |
| [0038](decisions/0038-one-second-segments.md) | 2026-09-27 | 1 s segments |
| [0039](decisions/0039-uniform-channel-output.md) | 2026-09-27 | Always transcode to one uniform channel output |
| [0040](decisions/0040-prepared-media-withdrawn.md) | 2026-09-27 | Prepared media is withdrawn; nothing encodes until a viewer tunes |
| [0041](decisions/0041-ship-ffmpeg-9.md) | 2026-09-27 | Ship ffmpeg 9 |

Future work that used to live in `design.md` §20 is tracked in #1576.
