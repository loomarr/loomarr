# Durable first-channel workflow

- Status: merged in PR #453
- Merge commit: `7934994185bea5bfb37464ae7ab6a951acc819cd`
- Contract: `docs/design.md` §8, `CONTEXT.md` Proposal Job / Attempt / First-channel Journey

## Outcome

One sentence or preset starts a caller-owned Proposal Job that remains understandable across reload,
restart, worker loss, and replica change. Its authoritative Journey either advances through grounded
Proposal review and approval to the intent-bound Channel, or stops with a durable safe failure and
server-authorized recovery actions. SSE improves latency only.

This is a Loomarr-native workflow module. It keeps the one Go runtime, SQLite/Postgres store
conformance, River's named recurring Tasks, deterministic grounding, authorization, and the approval
gate. It adds no generic DAG engine or external workflow server.

## Original current-state assessment

Before PR #453, `main` persisted Jobs and Proposals, but callers could mutate a whole Job record, generation
success is two writes, a process can strand `running` work, attempts are only a counter, and reconnect
documentation points at a Proposal route even though a failed Job has no Proposal. A preserved,
unpublished `first-channel-success-plan` branch proved atomic completion, expired-lease recovery,
caller-owned cache clones, typed failure reads, and polling UI, but it is 61 `main` commits behind and
mixes unrelated semantic/model work. Recover its verified behaviors selectively; do not merge it.

## Module seam

`internal/proposalworkflow` owns:

- Proposal Job creation, claim, recovery, completion, failure, and re-run commands;
- versioned Attempt history and checked attempt tokens;
- safe failure projection and server-derived next actions;
- the composed Job + newest Proposal + intent-bound Channel Journey;
- caller visibility and bounded history rules exercised at its interface.

The implementation uses private Store and generator ports. `internal/suggest` retains grounded model
reasoning and deterministic proposal construction behind the generator port. Existing approval and
Channel modules retain their state and invariants; the Journey reads them without taking ownership.

## Delivery slices

1. Contract and module-interface tests: vocabulary, legal milestones/actions, dependency seams, and
   fail-closed version/state behavior.
2. Persistence: forward-only SQLite/Postgres migration, versioned Attempt records, atomic claims and
   terminal transitions, expired-running recovery, and caller-owned cache cloning.
3. Execution integration: replace whole-row Job mutation and ignored terminal writes with workflow
   commands; preserve bounded model/tool behavior and automatic-approval ordering.
4. Authoritative reads: owner-scoped Journey endpoints, OpenAPI/client regeneration, bounded safe
   failure copy, newest Proposal decision, and intent-bound Channel state.
5. Frontend recovery: one shared tracker for Guide, Refine, and My requests; polling is correctness,
   SSE is invalidation; retry/edit/diagnostic/review/open-channel actions come from the server.
6. Proof and delivery: SQLite/Postgres conformance, crash/replay/version fixtures, auth/approval/
   grounding negatives, rendered desktop/mobile recovery journeys, full gates, PR, CI, merge, cleanup.

## Completion evidence

| Requirement | Required evidence |
| --- | --- |
| Authoritative Journey | Interface and HTTP tests for generating, awaiting approval, denied, building, live, and failed; reload with SSE disabled. |
| Atomic/recoverable transitions | SQLite/Postgres conformance including concurrent claim, expired-running recovery, stale worker, and rollback. |
| Attempt/failure history | Persisted bounded attempts with safe public failure and private diagnostic; retry/edit actions preserve the Intent. |
| Versioned state | Previous-version fixture recovery and unknown/corrupt/impossible-state fail-closed tests. |
| Safety | Member cross-owner 403, member approval 403, grounding fabrication rejection, and automatic quota tests unchanged. |
| Runtime architecture | Dependency/architecture gates show no new runtime and River remains the recurring Task engine. |
| Product behavior | Vite-rendered desktop/mobile Guide, Queue, and Refine flows recover after reload and dropped SSE. |
| Delivery | Generated artifacts clean, touched-area gates plus `make check` green, PR auto-merge enabled, CI and merge verified. |

## Baseline and coordination

The worktree was created from `origin/main` at `c650d7cb`. The unchanged baseline passed Rust, tagged
vet, Windows cross-build, and package compilation, then the pinned golangci/staticcheck analyzer
panicked internally while building package `poll`; this is inherited baseline evidence, not a green
gate and must be rerun unchanged before final attribution. Claims: `migrations`, `openapi-client`, and
`proposal-workflow`. The bounded failure classifier subsequently landed on `origin/main` in PR #452;
this branch rebased it into the workflow module's atomic failure transition and safe Journey projection.

## Validation

- Focused race tests pass for API, app, proposal workflow, store, and suggestion packages.
- The full frontend suite passes (1,529 app tests plus package suites), along with lint, typecheck,
  production bundle limits, and Storybook build.
- Browser-rendered Vite acceptance passes at 1440×1000 and 390×844 for My requests and Guide failure
  recovery, including zero horizontal overflow. Captures live under the worktree's agent artifacts.
- SQLite conformance passes. Postgres conformance was invoked but this host has no Docker provider;
  CI remains the required Postgres execution environment.
- `make check` reaches the same inherited pinned golangci/staticcheck panic in dependency package
  `poll` recorded at baseline. All later gate targets were run independently; the unrelated
  `cmd/image-bench` synthetic fixture hash failure is also reproducible from an `origin/main` archive.
- PR #453 merged on 2026-08-22. Its replacement CI run completed every required Go, Postgres,
  frontend, visual/a11y/e2e, tuner-browser, image, Android, Windows, docs, and aggregation job with
  no failing required check.
