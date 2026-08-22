# Backend consolidation

- Status: active
- Base: `79349941`
- Claims: `backend-readiness`, `dev-runtime`, `proposal-observability`

## Outcome

Loomarr keeps its current Go appliance, domain modules, shared SQLite/Postgres implementation, River
task engine, grounding, authorization, and approval gate. The work makes the existing backend easier
to change and operate: composition is split along real subsystem seams, application-owned work has an
explicit lifecycle, domain persistence dependencies are honest, proposal execution exposes bounded
health signals, and semantic certification becomes a repeatable opt-in assertion.

This is consolidation, not a framework migration. It adds no runtime or dependency and does not
promote multi-replica support without the two-process evidence in `multi-replica-readiness.md`.

## Coordination

`fix-playout-activation-anchor` is actively changing playout/store/design seams. Composition-root and
lifecycle implementation wait until that work lands; the readiness correction, proposal metrics, and
semantic-certification slices can proceed where their files do not overlap. Rebase before touching
`internal/app` or finalizing `docs/design.md`.

The unchanged worktree baseline passed Rust, shell, private-fixture, vet, tagged-vet, and Windows
cross-compilation, then reproduced the pinned golangci/staticcheck analyzer panic in dependency
package `poll`. That inherited failure is not a green gate and must be compared unchanged later.

## Delivery slices

1. Correct stale workflow/readiness records and amend the §14.1 lifecycle, persistence-seam,
   observability, and semantic-certification contracts.
2. Add bounded durable Proposal Job metrics: oldest nonterminal age, Attempts by terminal outcome, and
   failed Jobs by safe code; never label by id, owner, Intent, output, or diagnostic.
3. Turn the existing `internal/eval` harness into two honest modes: exploratory `make eval` may skip;
   `make eval-cert` requires configuration, executes the versioned corpus, and emits a versioned JSON
   scorecard. Cover the exact starter Intents and adversarial negative constraints.
4. After playout churn lands, characterize `BuildHandler` dependencies and extract immutable
   provisioning, channel, playout, suggestion, filler, scheduler, and HTTP assembly results without a
   mutable builder.
5. Return an application value with Handler plus idempotent Shutdown; move generation-owned stop/wait
   ordering behind it and delete test-only mutable production globals.
6. Narrow selected domain constructors from `store.Store` to their declared role interfaces; retain
   the composite Store at composition and conformance.
7. Run focused race tests per slice, shared SQLite/Postgres conformance, generated verifiers, and all
   touched-area gates. Run rather than infer the two-process investigation, recording unsupported
   rows without broadening the beta contract.
8. Publish one reviewable PR (or stacked PRs if the settled composition delta demands it), enable
   auto-merge after required gates, verify the merge, release claims, and remove only a clean task
   worktree.

## Completion evidence

| Requirement | Proof |
| --- | --- |
| Readiness truth | No merged workflow is listed as active/blocked; cluster evidence remains explicitly pending. |
| Composition depth | `BuildHandler` is a short ordered assembly over immutable subsystem results; tests drive those seams. |
| Lifecycle | Application shutdown is idempotent, reverse-ordered, bounded, leak-tested, and precedes Store close. |
| Persistence seams | Selected domains compile against narrow role interfaces; Store conformance remains one suite over both dialects. |
| Workflow observability | Prometheus tests prove closed labels, ages/counts, unknown-to-other mapping, and diagnostic/id absence. |
| Semantic certification | Missing config fails `make eval-cert`; all exact template/adversarial cases execute; scorecard schema/corpus/provider/model/result are recorded without secrets. |
| Architecture safety | Dependency and retired checks prove no new runtime/dependency and no weakened grounding/approval/auth path. |
| Delivery | Focused/full gates, PR CI, merge state, claim release, and clean worktree removal are recorded. |
