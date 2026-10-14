# Proposal workflow

Formerly `design.md` §8's execution model, human-in-the-loop rule and decision traces. This doc
covers how an Intent is executed durably, what the requester sees, how dates in a request are
interpreted, and the traces that explain a decision. What the Suggester does inside one run is in
[`suggester.md`](suggester.md).

## Approval

Proposals are never auto-executed (decision [0005](decisions/0005-human-approval.md)). Every
approval for one requester, manual or automatic, enters one store-owned, requester-scoped ordering:
an in-process keyed semaphore on SQLite, and a transaction-scoped advisory lock on PostgreSQL whose
quota reads and approval commit share that transaction and connection, so a lost session rolls back
instead of releasing the ordering early. Ordering ends at the commit or rollback, before best-effort
channel reconciliation. Any ordering or quota-read failure leaves the Proposal `submitted`.

## Proposal Jobs and the Journey

A **Proposal Job** is the caller-owned durable execution of an Intent. `internal/proposalworkflow`
owns its lifecycle, attempt recovery, caller visibility and the First-channel **Journey** projection.
Its interface is small: submit or re-run an Intent, claim bounded work, complete or fail one claimed
Attempt, and read or list Journeys. The Store and the model runner are private ports; API, worker
and frontend never rebuild lifecycle rules from raw rows.

The Job id is the correlation spine, not a state machine that takes over domain ownership. The
Proposal stays the grounded artifact and approval audit; the Channel owns `building`/`live`. The
Journey composes them into one milestone:

```text
queued/running                              -> generating
done + Proposal submitted                   -> awaiting_approval
done + Proposal denied                      -> denied
done + Proposal approved + Channel building -> building
done + Proposal approved + Channel live     -> live
failed                                      -> failed
```

A later pause does not erase that the Journey reached `live`; current Channel status is returned
separately. An impossible combination fails the read closed and is recorded for operators.

## Attempts and recovery

- Each lease creates a numbered **Attempt** with its own times and outcome (`succeeded`, `failed`,
  `interrupted`). Claiming is one transaction that takes due `queued` Jobs and expired `running`
  ones, marks the expired Attempt `interrupted`, bumps the attempt token and renews the lease
  (`FOR UPDATE SKIP LOCKED` on PostgreSQL). A crash causes bounded re-execution, never a stranded row.
- Completion and failure compare-and-swap on `(job_id, attempt)`, so a stale worker cannot publish.
  Success inserts the Proposal and marks Attempt and Job terminal in one transaction; a failed
  execution materialises neither a Proposal nor a Channel.
- Retrying a failed first request creates a fresh Job. Refine and re-curate keep the Channel's intent
  reference and add an Attempt. A cache hit (hash of normalised Intent and constraints, 24 h TTL)
  copies grounded content into a fresh caller-owned Job and Proposal; it never shares another
  caller's ids, approval or history. Only a successful job is cached, and zero grounded titles fails
  the job rather than persisting an empty Proposal.
- Persisted Jobs and Attempts carry a workflow-schema version. Readers accept every version an
  upgrade still supports and fail closed on unknown, future or corrupt state; a release may add a
  transition path but never reinterpret stored state. Upgrade fixtures prove it.

Proposal Jobs keep their own leased executor because their history is business state. River runs
named recurring operator tasks ([`deployment.md`](deployment.md#the-job-scheduler)).

| Activity | Contract |
| --- | --- |
| Catalog tool | Read-only, bounded, safe to retry within the tool-round ceiling. |
| Model turn | Bounded by the Job timeout; may re-run after worker loss; only the current Attempt token commits. |
| Proposal completion | One local transaction; no Proposal is visible for a Job that did not become `done`. |
| Approval | The atomic `submitted -> approved` decision; authorization and quota ordering stay outside model control. |
| Acquisition and publication | Idempotent reconciliation after approval; repair by converging again. |

## What the requester sees

`GET /v1/proposal-jobs/{jobId}` is authoritative after reload, restart, replica change or event loss.
It returns the Intent, bounded Attempt history, safe failure code and message, the newest Proposal,
the intent-bound Channel, the milestone and the server-derived permitted actions (`wait`, `review`,
`retry`, `edit`, `check_ai`, `open_channel`). The frontend never reconstructs authorization or retry
policy. `/v1/events` streams `suggestion` frames (`{jobId, phase, round}`) that only trigger a
refetch; a dropped frame is a latency defect, not a correctness one. Phases are emitted inside the
tool loop at the transition they describe and may repeat.

The builder shows one calm status message: no rounds, timers, percentages or checklists. A failure
replaces progress with one plain explanation and the useful authorised action, and keeps the saved
Intent. Revisions keep the current list visible and read-only until the replacement arrives. The
active Job and its approval edits are restored after reload until an explicit discard.

**Failures.** A failed Journey keeps its stable outer code and adds a closed `reason` and
`recoveryAction` with fixed server-owned copy:

| Reason | Recovery | Selected by |
| --- | --- | --- |
| `reference_unreadable` | `edit_reference` | a typed failure from the reference-read stage only |
| `retrieval_unavailable` | `retry_later` | any other reference or catalog retrieval failure |
| `no_catalog_match` | `broaden_request` | no grounded titles |
| `named_set_unproven` | `provide_examples` | selection empty only because members lacked membership evidence |
| `constraints_conflict` | `resolve_constraints` | identical include and exclude values, or an empty same-axis date intersection |
| `date_semantics_unclear` | `clarify_dates` | validated, anchored date ambiguity |
| `invalid_tool_calls` | `retry_later` | every consumed tool round was invalid |
| `provider_response_invalid` | `retry_later` | malformed final JSON, or exhaustion after candidates were surfaced |
| `discovery_budget_exhausted` | `retry_later` | exhaustion with nothing surfaced; copy stays neutral |
| `provider_timeout`, `provider_unavailable` | `retry_later` | an actual provider-turn deadline or outage |
| `generation_failed` | `retry_later` | anything else |

The mapping and its copy live in `internal/proposalworkflow/workflow.go`. Only allowlisted typed
evidence captured at the rejecting branch selects a specific reason. `check_ai` is offered only for an AI-stage failure, and a member is
never told to change an admin-only setting. No projection exposes raw provider errors, prompts,
responses, fetched text, credentials, private titles or candidate identities.

`/metrics` reports Jobs by status, the oldest queued or running Job's age, Attempts by outcome, and
failed Jobs by safe code. Unknown values collapse to `other`; ids and text are never labels.

## Dates in a request

Every `catalog_search` call and final response carries `dateMeaning` (#1034): `kind` is `none`,
`constraints` or `ambiguous`; `anchors` point at submitted text (field, optional array index,
half-open rune offsets); `axes` are one to three of `movie_release`, `series_premiere` and
`series_airing`, each with `combine: any|all` and one to four 1900–2099 intervals.

- Canonicalisation intersects `all`, coalesces `any`, and only an empty same-axis intersection is a
  conflict. "90s and 2000s" is a union. Different axes stay independent.
- `kind: none` is malformed when the request has unmistakable date syntax (an `Era` field, a decade,
  a year range, or `from`/`before`/`since`… a year); it goes through the bounded repair path. A year
  inside a title is not a filter.
- Tool meanings and the final meaning must canonicalise identically. Sources are initialised once per
  invocation, only after a valid non-ambiguous interpretation, so ambiguity, conflict and exhausted
  malformed output dispatch nothing.
- `scope.dates` stores `movieRelease`, `seriesPremiere` and `seriesAiring` ranges: union within a
  list, conjunction across axes. Airing is checked per episode, so an older series can supply
  in-window episodes. Nothing widens explicit windows. `scope.dates` is the only stored date
  scope: an `era` is read as an alias for the same range on all three axes and never written
  (#1877, [programming design](../programming-design.md)).
- Filler follows through `filler.eraWindows` (at most eight ranges, exclusive with `filler.era`),
  inherited from the airing or release windows when neither is set. Unknown filler years do not
  satisfy a selected era.
- A union is one retrieval: each window is one scalar query, results are deduplicated and truncated
  once to 24 candidates, and a failed window fails the whole union.
- **Network epochs** ("like a documentary network in the 1990s") are editorial references, not
  episode filters. The network is resolved through Catalog discovery, the older title pool comes from
  a premiere upper bound at the end of the decade, and the network-only tool interface is exposed.
  Title or generic genre lookups without a network identity are corrected before dispatch.

**Credits.** A generation has six work credits; one Suggest invocation has
`ProductionBounds().MaxToolCalls` in total (currently 24), shared across repairs. A discovery that
expands into N queries costs N credits, reserved before dispatch.

## Model requests

- **Prefix caching.** On a provider with a prompt-prefix cache (`llm.PrefixCacher`, such as
  llama.cpp), every turn of one suggestion sends the same tools array and finalisation uses
  `tool_choice: "none"`, because removing tools changes the prompt from its first tokens and forces a
  full re-prefill. Static content leads. Hosted providers still drop tools at finalisation.
- **Reference requests** start with no tools: the model interprets the Intent and returns empty
  picks, then Loomarr resolves the reference and supplies its evidence within the same six-turn
  generation. Source lookups are cached across repairs.
- A request centred on a named franchise stays within its members unless the Intent invites
  adjacent discoveries; fewer supported members beat padding.
- Grounded selection uses the LLM adapter's request profile (`grounded-selection-v3`), which adapts
  optional parameters per exact model and enforces strict routing and data-collection controls on
  hosted aggregators. It never selects a model or qualifies one.
- Structured filler calls use JSON mode, low temperature and an explicit `max_tokens`; self-hosted
  endpoints also get `enable_thinking=false`.
- Prompt, tool-schema and certification manifests are versioned and immutable; a model or route
  change needs fresh qualification.

## Decision traces

**Proposal trace (#496).** The ranking emits one immutable, Proposal-scoped `DecisionTrace`:
original evidence that edits, availability changes, cache clones and re-curation never recompute.
Each surfaced candidate has one closed disposition and reason (selected, alternate, not-selected,
policy-refused, validation-dropped, acquisition-cap, plus terminal outcomes). The published rank is
the integer tuple (relevance, preference, novelty, canonical key). Constraint evidence is the
relevance count and closed booleans for request, tone, era, include, exclude and refine.

**Scheduler trace (#496).** Every `schedule.ComputeDesiredAt` run emits its own trace of current
cycle evidence, exposed by the cycle and programming-preview endpoints from the same computation.
Stages are `hard_filter`, `availability`, `episode_selection` and `placement`; metadata records the
ordering, decimal-string shuffle seed and window. 256 of the 1,024 facts are reserved for placement.

Both traces are versioned, stably ordered, bounded to 1,024 facts with `factTotal`, `recordedTotal`
and `truncated` counting past the bound, and carry only canonical identity, safe display names and
closed codes. Prompts, rationale, credentials, provider payloads, paths and locations are redacted.
Evaluators consume the same typed evidence and fail closed on mismatch.

## Tests that pin this

Formerly `design.md` §19.

- Interface-level tests cover every legal Journey milestone and permitted action without reading
  raw tables. SQLite and Postgres conformance proves atomic claim, expired-running recovery,
  monotonic Attempt tokens, stale-worker rejection, success and failure rollback, cache cloning and
  bounded history. Crash tests stop after claim, model return, Proposal insert and approval commit;
  restart converges to exactly one visible outcome. Old-version fixtures stay readable; unknown
  versions and approved-without-Channel fail closed. Dropping every SSE frame does not change the
  result.
