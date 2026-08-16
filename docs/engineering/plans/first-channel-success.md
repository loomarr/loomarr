# First-channel success

**Status:** proposed. Planning worktree `../loomarr-first-channel-success`, branch
`first-channel-success-plan`, based on `origin/main` `4c6f08c2`. `make agent-baseline` was green on
2026-08-15.

## Outcome

A new operator can choose a preset or write one sentence and reach one watchable linear channel.
The path either produces a grounded, reviewable proposal or stops in a durable, actionable failure
state. Reloading the page or losing SSE must never erase the outcome.

This is the product proof for Loomarr's premise: an old-fashioned channel is simple to ask for, while
LLM curation, acquisition, scheduling, filler, and playout stay sophisticated behind that request.

## Current truth

The architecture has already achieved most of the premise:

- Natural-language intents enter a grounded tool loop and produce typed proposals.
- Approval is the single gate into acquisitions and channel materialization. The local approval,
  title, and channel writes are atomic.
- Approved channels reconcile into schedules, Guide rows, Tunarr or internal playout, and Watch.
- Filler V54 is complete. Empty filler is a supported program-only channel, while a populated pool is
  inherited and assembled through the same scheduler seam.
- A real-model semantic harness already exists under `internal/eval`.

The missing proof is concentrated before and immediately after approval:

1. A generation job is durable in the store, but there is no authoritative job read interface.
   `GET /v1/proposals/{id}` currently reads a proposal id even though design §7/§8 promises job status
   plus an optional proposal.
2. The frontend treats SSE as the only source of terminal failure and exposes only the initial POST
   error. A background failure therefore drops back to a blank form with no reason.
3. Successful outcomes can disappear too: the hook searches only submitted proposals, so an
   auto-approved, already-decided, cached, or missed-final-event result is not recoverable.
4. The successful-job cache reuses lifecycle identity across requesters. Equal intent text can return
   another request's old job instead of giving the new requester a fresh auditable request.
5. The four presets carry typed `era` and `tone` fields, but wizard and Guide handoff submit only their
   descriptions. The semantic corpus is separately authored, so it does not prove the exact presets
   that ship.
6. Existing browser coverage stops before the complete journey: wizard coverage ends at Guide and
   approval coverage begins later in Queue.

The resulting assessment is: **the channel appliance exists, but the first-channel acceptance
contract is not yet release-grade.** This plan closes that contract without redesigning the scheduler,
approval gate, or filler engine.

## Non-goals

- No second channel-creation surface and no dense first-screen constraint editor.
- No automatic intent rewriting, fabricated fallback titles, weakened grounding, or weakened
  approval/authorization.
- No new LLM provider, application runtime, scheduler, manual “Rebuild now” control, or automatic
  paid semantic call in health polling.
- No requirement to ingest filler before a channel can be approved or watched.
- No repetition of V54, and no taxonomy, retry, storage-probe, or filler-pipeline work owned by V55.
- No decoded-frame browser matrix here; V58 tuner certification owns that evidence.

## Interface decision: a durable proposal job

Add one deep module whose interface represents a proposal job independently from the proposal it may
produce. Its implementation owns job/proposal joining, visibility, lifecycle transitions, safe
failure classification, and cache materialization. Callers must not reconstruct those rules by
listing proposals and matching `jobId`.

The HTTP seam is:

```text
GET /v1/proposal-jobs/{jobId}
GET /v1/proposal-jobs?mine=true&status=queued|running|done|failed
```

The single read returns:

```text
jobId, status, intent, attempts, createdAt, updatedAt,
failure? { code, message }, proposal?
```

Rules:

- `GET` is authoritative. SSE carries transient phase/round data and only accelerates a refetch.
- A member may read only their own jobs; an admin may read any job. There is no caller-supplied user
  id. Cross-user reads return 403.
- `intent` is the complete typed intent so reload and retry do not depend on browser memory.
- `failure.code` is a bounded enum, initially `no_grounded_titles`, `timed_out`,
  `provider_unavailable`, and `generation_failed`. `message` is operator-safe. Raw provider text
  remains in logs and the internal diagnostic field, never in a member response or metric label.
- `proposal` is the newest proposal for the job in any decision state, not only `submitted`.
- Job completion is one store operation: create the proposal and transition the job to `done`
  atomically. Failure is a checked transition that persists both safe code and diagnostic detail.
- A cache hit saves inference, not request identity. Every submit creates a fresh caller-owned job and
  proposal/audit lifecycle. Cached proposal content is copied into that lifecycle atomically.
- Refine/re-curate may continue to reuse the channel's stable intent reference. The job read describes
  the current execution; proposal audit history remains the durable record of earlier decisions.

Do not overload `GET /v1/proposals/{id}` with a job id while approve/deny use proposal ids. Amend the
false design claim and retain that route as the proposal-resource read.

## Delivery slices

Each slice is an independently reviewable commit or very small PR. Design changes precede the code
that relies on them.

### 1. Lock the contract and acceptance states

- Amend `docs/design.md` §§7, 8, 13 and `docs/programming-design.md` to distinguish a proposal job
  from its optional proposal, define ownership, cache behavior, durable failure, retry, and the
  building-to-live handoff.
- Amend `docs/dev/testing.md` with the deterministic browser journey and real semantic certification.
- Add failing interface-level tests for the four job outcomes: queued/running, done with a submitted
  proposal, done with an already-decided proposal, and failed without a proposal.
- Record filler as optional: no eligible clips means programs play back-to-back, not that channel
  creation is blocked.

### 2. Make proposal-job outcomes atomic, typed, and caller-owned

- Add paired SQLite/Postgres migration fields for the stable failure code. Select the migration
  number only after rebasing; `migrations` is currently claimed by V55.
- Add store operations for checked failure and atomic successful completion, plus newest proposal by
  job irrespective of decision state and bounded job listing by creator/status.
- Change cache reuse to create a fresh job/proposal lifecycle for every submit. Prove that equal
  intents from two users never share ids, ownership, or decision state.
- Stop ignoring the final job update error. Preserve the existing atomic approval/materialization
  boundary unchanged.
- Cover SQLite and Postgres with the one conformance suite, including concurrency and rollback.

Likely files: `internal/store/{store.go,jobs.go,conformance_jobs_test.go,approval.go}`,
`internal/store/migrations/`, and `internal/suggest/{worker.go,worker_test.go}`.

### 3. Expose the authoritative proposal-job reads

- Implement the deep job projection and the two owner-scoped endpoints.
- Return the full typed intent, safe failure, and newest proposal when one exists.
- Add member-own, member-cross-user 403, admin, token, missing, auto-approved, and failed cases at the
  HTTP interface.
- Regenerate OpenAPI and the frontend client through the repository generators.
- Update SSE comments and invalidation so no code claims that a proposal list is the reconnect source
  of truth.

Likely files: `internal/api/proposal_jobs.go`, `internal/api/proposal_jobs_test.go`, registration in
the existing Huma module, `api/openapi.yaml`, and generated frontend client inputs.

### 4. Recover generation in Guide and Queue

- Make `useSuggestionRun` query the job endpoint while queued/running and once at every terminal
  transition. Polling is the correctness backstop; SSE invalidation is the latency path.
- Persist the active job id in Guide route search so reload resumes progress, failure, or review.
- Preserve the complete intent through failure. Offer bounded actions:
  - no grounded titles: edit the description or retry;
  - timeout/provider failure: retry or open Settings → AI;
  - generic failure: retry and keep the diagnostic link for admins.
- Retry creates a fresh job with the same complete payload. Edit returns to the populated form.
- Replace proposal-only “My requests” aggregation with caller-owned proposal jobs so failed and
  in-flight requests do not vanish. Keep approval history proposal-based because it is an audit of
  decisions, not executions.
- Apply the same authoritative hook to Refine; do not keep two near-copy state machines.
- Announce progress with `aria-live`/`aria-busy`; terminal failure uses `role=alert` and receives
  focus.

Likely files: `web/apps/web/src/suggest/use-suggestion-run/`,
`web/apps/web/src/suggest/use-channel-refine/`, `channel-suggest-panel/`, `refine-panel/`, Guide route
search, `queue/my-requests/`, and the shared events invalidation module.

### 5. Submit the presets that the interface promises

- Use one canonical preset data source containing the complete typed intents. Prefer a neutral JSON
  product-data file consumed by `@loomarr/core`; the Go eval harness reads that same file rather than
  hand-copying the four descriptions.
- Hand off a stable preset id from wizard to Guide, resolve it to the full `Intent`, and preserve the
  legacy free-text `?intent=` deep link.
- Make Guide preset chips and wizard presets call the same typed submission path.
- Pin the Saturday preset's `description`, `era: "1990s"`, and `tone: "playful"` at the HTTP request
  seam. Do the equivalent identity check for all four presets.

Likely files: `web/packages/core/src/templates/`, `wizard/first-channel-step/`, Guide route search,
`suggest/intent-form/`, and `internal/eval`.

### 6. Make review and creation truthful

- Keep the first view compact, but expose the proposal facts that affect trust: audience/scope,
  ready-versus-acquire counts, refused titles/reasons, and extracted policy. Put item-level rationale
  behind disclosure.
- Members see “Waiting for admin approval”; they never receive inert approval controls. Direct member
  approval remains 403.
- While approval/materialization is in flight, label the action “Creating channel…” and disable
  duplicate submission.
- A `building` channel does not initiate HLS. Watch shows an automatic-retry state and transitions on
  channel SSE plus authoritative refetch when the channel becomes live.
- Repeated approval remains idempotent and creates one channel.

Likely files: proposal review primitives, `channel-suggest-panel`, channel detail/watch, and their
tests/stories. Backend approval logic changes only if an interface-level test exposes a real gap.

### 7. Make model and curation readiness honest

- Separate configured, reachable, tool-capable, and semantically certified states. A model known not
  to support tools cannot be selected for grounded curation; unknown hosted capability remains
  explicitly unverified rather than falsely green.
- Add an explicit operator-triggered curation verification. Never perform paid inference from
  periodic setup/health reads.
- Run the exact four canonical presets through the existing real-model harness. Add
  `make eval-templates` only through the documented command generator.
- Require non-empty grounded output, no fabricated ids, and each preset's safety/era expectations.
  Keep this manual and non-hermetic; `make check` remains network-free.
- Add bounded job outcome counters and duration histograms. Labels may include stable result/failure
  code, never intent, job id, model error, or raw provider text.

### 8. Close filler and end-to-end acceptance without reopening filler

- Reuse `GET /v1/filler/pool` in the proposal panel for one compact, non-blocking line:
  - eligible clips exist: filler is available and will be tuned after creation;
  - none exist: the channel will play programs back-to-back and filler can be added later.
- Approval is enabled in both states. Do not claim exact proposed-channel coverage before a channel
  and saved policy exist.
- Extend the real integration journey with two variants: zero filler yields real program slots and
  no break writes; seeded filler yields attached filler. Reconcile twice to prove idempotence.
- Add a rendered browser journey: fresh setup → full preset → durable progress → review → approve →
  building → live Watch → Guide row.
- Add rendered failure journeys for dropped SSE/reload, no-grounded retry, provider timeout, member
  waiting, and building Watch making no HLS request.
- Update the real smoke scenario from the retired standalone Suggest route to Guide. Agents do not
  run `make smoke*`; the maintainer performs the final real-stack playout proof.

## Acceptance contract

The phase is complete only when all of these are true:

1. All four presets submit their exact full canonical intent from both wizard and Guide.
2. Every accepted job is durably readable as queued, running, done, or failed after reload and with
   SSE disabled.
3. A failed job shows a stable safe reason, preserves intent, and supports retry/edit without
   leaking provider text.
4. Equal intents from different users create distinct caller-owned audit lifecycles.
5. Review counts, refused items, policy, and proposal state match the persisted proposal. Nothing is
   acquired before approval.
6. One approval creates one channel. A building channel makes no HLS request; live transition starts
   playback without a page reload.
7. A no-filler install creates and plays a program-only channel. A seeded install attaches filler
   through the existing assembler.
8. Members can submit and follow their own jobs but cannot read another member's job or approve.
9. The exact shipped presets pass the real semantic evaluation on the declared certification
   provider/catalog.

## Coordination and claims

Re-run `make agent-status`, rebase, and check migration-version uniqueness before every implementation
slice. At plan time these scarce outputs are owned elsewhere:

- `migrations` and `filler-operations`/`taxonomy`: V55 filler operations.
- `openapi-client` and `visual-baselines`: Dashboard/People operations.
- `e2e-baselines`: tuner browser certification.

Do not begin an overlapping slice until its claim is free. Use a dedicated `proposal-jobs` claim for
the store/interface work, then acquire only the scarce generated-output claim needed by that slice.
The filler UI/test portion begins after V55 merges or rebases cleanly; it must not edit V55's filler
pipeline, taxonomy, settings, migration, or operations surfaces.

## Gates

Focused inner loop:

```sh
go test -race ./internal/store ./internal/suggest ./internal/api
make test-pg
make openapi-verify
make fe
make e2e
make docs-lint
make retired-verify
make agent-verify BASE=origin/main
```

Rendered frontend work is verified against the Vite URL from `make dev-fe`, with desktop and mobile
states and accessibility checks. The backend port's embedded SPA is not evidence.

Final delivery runs the complete gates required by the touched areas, including `make check`, then
publishes the implementation PR with auto-merge per the agent contract. `make eval-templates` is
recorded as real-model semantic evidence. The maintainer-owned smoke records the real Tunarr/internal
playout result; agents never run `make smoke*`.
