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
  [`configuration.md`](../configuration.md), and the package inventory is
  [`package-map.md`](package-map.md). Do not restate them in prose.
- Vocabulary is defined in [`CONTEXT.md`](../../CONTEXT.md). Work status lives in GitHub issues.

## Subsystems

The **Old §** column maps the `§N` references in code comments to the doc that now owns them.

| Doc | Owns | Old § | Now in |
| --- | --- | --- | --- |
| [overview](overview.md) | scope, design envelope, ports, package-name traps, backend structure rules, HTTP API rules, concurrency rules | 1, 2, 7, 7.1, 14.1, 18 | here |
| [dependencies](dependencies.md) | every dependency and bundled tool, with its reason | 14 | here |
| [package-map](package-map.md) | generated package inventory | 2 (map), 14.2 | here |
| acquisition | title identity, provisioning state machine, requester, adapter resilience | 3, 4, 6 | `design.md` |
| library | media inventory, series episode cache, media-server contract, search | 5 (inventory), 6, 7.2 | `design.md` |
| storage | one store with two backends, retention, Postgres concurrency, backup | 5, 16 (backup) | `design.md` |
| suggester | grounding, model identity, reference-backed Intent, providers, model selection and residency | 8, 8.1, 8.2 | `design.md` |
| proposal-workflow | Proposal Job execution, decision traces | 8 (execution) | `design.md` |
| scheduling | channels, lineups, strategies, airing history, breaks and pods | 9, 10 (breaks) | `design.md` |
| playout | the channel packager, backends, what is served, admission, playout status, Live TV wiring | 9.1 | `design.md` |
| playback-clients | tuning, pause, Android TV | 9.1 (clients) | `design.md` |
| auth | identity, credentials, invitations, SSO, roles, route authorization | 11 | `design.md` |
| web-ui | navigation, first-run wizard, cross-view rules | 12, 13 | `design.md` |
| deployment | image, Compose, restart in place, the job scheduler | 9.2, 16, 18.1 | `design.md` |
| observability | diagnostics, correlation, health, metrics | 17 | `design.md` |
| images | the image service | 22 | `design.md` |
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
| [0002](decisions/0002-code-first-openapi.md) | 2026-07-13 | Code-first OpenAPI with Huma v2 |
| [0024](decisions/0024-react-native-clients.md) | 2026-08-23 | React Native and Expo for client binaries |

Future work that used to live in `design.md` §20 is tracked in #1576.
