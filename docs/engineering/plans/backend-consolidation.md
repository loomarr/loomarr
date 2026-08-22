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

The `fix-playout-activation-anchor` agent released its claim after publishing PR #455 with green CI.
That PR remains draft as of 2026-08-22, so this branch does not depend on its unmerged commit and must
still rebase after it lands. Its changed store/playout-adapter files were not edited here; the
composition extraction proceeded in the unclaimed application assembly files.

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

## Composition extraction map

Measured at `79349941`, `BuildHandler` spans `internal/app/app.go:169–1635`. Extraction follows the
existing dependency order. Each builder is an internal seam: a concrete immutable dependency value
in, a concrete immutable result value out. Neither value is a generic service locator, and no builder
starts work it cannot return a shutdown handle for.

| Builder | Owns | Required earlier inputs | Result consumed later |
| --- | --- | --- | --- |
| `buildFoundation` | readiness, settings generation, secrets/redaction, dynamic library/TMDB clients, event bus, job registry, activity recorder | Store, logger, external-origin Overrides | resolved settings, clients, emitter, registry, activity, secret readers |
| `buildProvisioning` | title reconcile, availability scan, queue poll, episode refresh, retention registrations | Foundation | provisioning runner and episode resolver |
| `buildChannels` | dynamic Programmer, availability, channel engine, Live TV connector, backend-transition view, channel-number source | Foundation + Provisioning | channel HTTP roles, engine, connector, backend checkpoint functions |
| `buildPlayout` | media budget, session/HLS managers, prepared origin, resolver, guide, backend transition controller, playout lifecycle | Foundation + Channels | playout HTTP roles, observers, resolver roles, lifecycle shutdown |
| `buildApproval` | binder and approval coordinator | Foundation + Channels + Playout codec role | binder and approver roles |
| `buildSuggestions` | LLM/catalog, Journey workflow, search, icon/image adapters, re-curation registration, resident-model hooks | Foundation + Approval + Provisioning | suggestion/search/workflow/image/system-LLM roles and VRAM hooks |
| `buildFillerSubsystem` | existing tag/sync/pipeline/split/fetch builders and scheduler registrations | Foundation + Suggestions | filler roles and pod adapter |
| `buildOperations` | backup, auth/device/SSO, settings/restart/database, River start | All prior results | operational HTTP roles and scheduler shutdown |
| `buildHTTP` | one `api.Options` assembly | All immutable results | configured Handler only |

The three deliberate late connections remain visible at their owning seams: `buildChannels` attaches
the channel engine to the event emitter, `buildFillerSubsystem` attaches pods to channel/playout
consumers, and the short root attaches the returned resident-model probe to playout. They occur once
during ordered construction and are not exposed as mutable application state.

The external application interface is deliberately smaller than the internal extraction map:

```go
app, err := Build(parent, store, logger, overrides)
handler := app.Handler()
err = app.Shutdown(ctx)
```

The handler-only compatibility constructor has been deleted; main, tests, and the integration harness
all own an `Application` and call `Shutdown`. Shutdown order is the reverse of successful startup;
partial Build failure uses the same stack. The Store is not on that stack because its caller owns it
and closes it only after application shutdown.

### Replacement tests

- Application-interface tests cover no-store readiness, real route wiring, restart generations, and
  idempotent bounded Shutdown.
- Builder tests cover only their returned roles and failure cleanup. They do not inspect another
  builder's fields.
- Playout dependencies become constructor-required; deleting a ladder input fails construction,
  replacing the `lastPlayoutResolver` global and its post-build nil inspection.
- Filler-layout generation behavior is asserted through the playout builder result before HTTP
  assembly and through the running route behavior after assembly; no process-global observation
  point remains.
- The existing goleak restart test moves to `Application.Shutdown`, then closes the Store, which is
  the production ordering it is intended to certify.

## Completion evidence

| Requirement | Proof |
| --- | --- |
| Readiness truth | No merged workflow is listed as active/blocked; cluster evidence remains explicitly pending. |
| Composition depth | `app.go` is 219 lines and its 73-line ordered assembly consumes immutable subsystem results; tests drive the real `Build` seam. |
| Lifecycle | Application shutdown is idempotent, reverse-ordered, bounded, leak-tested, and precedes Store close. |
| Persistence seams | Selected domains compile against narrow role interfaces; Store conformance remains one suite over both dialects. |
| Workflow observability | Prometheus tests prove closed labels, ages/counts, unknown-to-other mapping, and diagnostic/id absence. |
| Semantic certification | Missing config fails `make eval-cert`; all exact template/adversarial cases execute; scorecard schema/corpus/provider/model/result are recorded without secrets. |
| Architecture safety | Dependency and retired checks prove no new runtime/dependency and no weakened grounding/approval/auth path. |
| Delivery | Focused/full gates, PR CI, merge state, claim release, and clean worktree removal are recorded. |
