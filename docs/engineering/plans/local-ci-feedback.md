# Proportional local and CI feedback

## Goal

Reduce Loomarr's local-development and CI feedback time without weakening the assurance required
to merge or release. Changed paths are classified once, unfamiliar inputs fail closed, fast checks
exercise the affected dependency closure, and a protected final boundary retains the complete
required gates.

## Measured baseline

On 2026-08-22, recent successful pull requests completed in roughly 10-12 minutes. A change limited
to `internal/suggest` and `docs/programming-design.md` still ran three `make check` shards, Postgres
conformance, Windows playout verification, and native amd64 and arm64 release-image builds. In one
representative run the three `make check` steps took 456, 537, and 648 seconds. Each shard repeated
Rust, formatting, vet, lint, tagged compilation, Windows compilation, harness, and release-contract
work; only the Go test package list was partitioned.

The repository requires one aggregate `CI` status. On 2026-08-23 its branch protection was changed
to strict mode while preserving the GitHub Actions app binding, so a pull request must be tested
against the current `main` before merging. GitHub does not offer merge queues to personally owned
public repositories; queue activation remains ready at the workflow level and becomes available
after an organization owns the repository.

## Assurance tiers

| Tier | Scope | Budget |
| --- | --- | --- |
| Edit | Direct package or frontend test in watch mode | seconds |
| Pre-push | Affected Go dependency closure and relevant frontend/static checks | 90 seconds p95 |
| Pull request | Fail-closed, impact-scoped gates running in parallel | 5 minutes p95 for leaf changes |
| Merge group | Full affected-domain gate against current `main` | 12 minutes p95 |
| Main, nightly, release | Complete race, database, browser, architecture, and packaging matrices | comprehensive |

These are feedback budgets, not test timeouts. Exceeding one produces evidence for the next
optimization; it never skips or kills a correctness gate.

## Policy

One deep module owns path classification. Its interface accepts changed repository paths and
returns stable gate decisions. Local tooling and CI are adapters at that seam; neither carries a
second set of path regular expressions. Unknown paths, missing bases, classifier errors, and new
source families select every gate.

For Go changes, the fast tier runs race tests for changed packages plus their reverse-dependent
closure. Repository-wide compilation and contract checks remain cheap, parallel checks. These
seams always force the complete Go suite: composition root, shared testkit, store interfaces and
migrations, module files, build tags, and generators.

The final protected tier retains all applicable assertions from `make check`, Postgres conformance,
the three-browser tuner suite, visual and accessibility coverage, native release architectures,
and Android. The work changes when assurance runs, not whether it exists.

## Delivery

1. Add the classifier and exhaustive table/known-path tests without changing CI behavior.
2. Make `agent-verify` consume it and calculate reverse Go dependencies.
3. Split global Go contracts from sharded race tests so global work runs once.
4. Add specialized Postgres, Windows, Rust, visual, e2e, tuner, image, and Android decisions in
   shadow mode while the old jobs still run.
5. Compare shadow selections with full outcomes and add a regression fixture for every mismatch.
6. Enable strict merge protection. Enable and prove the already-supported `merge_group` path after
   an organization owns the repository; GitHub does not expose merge queues to personal repos.
7. Activate proportional pull-request gates and retain complete merge/main/nightly/release audits.
8. Publish selected gates, setup/cache/test timings, critical path, and runner-minutes in summaries;
   then profile genuinely slow packages after orchestration waste is gone.

Each activation is a separate reversible pull request. `docs/design.md` section 19 and the developer
gate documentation are amended before the first change that alters required behavior.

## Evidence

- PR #462 merged the fail-closed classifier with the full required CI matrix green. Its critical
  path was 11m18s; the other Go shards took 11m06s and 9m37s.
- The reverse-dependency selector resolves the repository graph in about 0.3 seconds on the
  development host. A representative `internal/suggest` leaf selects 10 of 59 packages, including
  its command, API, composition, integration, and workflow consumers while excluding the unrelated
  store package. Cross-cutting and unknown paths select all 59.
- With the selector wired into `agent-verify`, that representative leaf completed its 10-package
  race-policy-aware local check in 34.2 seconds on a warm development host. The command still states
  that it is focused evidence and that the complete gate remains required before publication.
- PR #464 merged that local activation with the full matrix green. Its legacy CI critical path was
  still 11m15s: every specialized job finished within 3m31s, while the three Go shards took 11m15s,
  9m14s, and 9m28s because each repeated the same repository contracts before its test partition.
- The next slice preserves `make check` as `check-static` plus `test`, runs the static/repository
  contracts once beside three test-only shards, verifies the shard partition independently, and
  requires both halves in the `CI` aggregate.
- PR #465 merged that split. Its clean-run test shards completed in 3m48s, 4m36s, and 5m56s,
  versus 11m15s, 9m14s, and 9m28s before. The one-time contract job took 8m50s because a 1m59s
  runtime certification still followed 6m26s of independent static contracts; the next slice runs
  those two required results in parallel.
- PR #466 merged the certification split. On its clean run, repository contracts became the
  5m39s critical path while release-worker certification completed in 2m50s and the cached Go
  shards completed in 2m21s, 3m09s, and 2m24s. This establishes the pre-activation baseline for
  observing specialized decisions without allowing them to skip any current job.
- The specialized classifier now publishes shadow `impact_*` decisions and a comparison table in
  the `changes` job summary. Legacy broad outputs remain the only selectors until shadow evidence
  and regression fixtures establish that every affected gate is retained.
- PR #468 merged the first live shadow report with every legacy-selected job green and a 3m32s
  critical path. Auditing those full outcomes exposed that the native Windows job was absent from
  `ci-ok.needs`; its failure could therefore have been invisible to the protected `CI` result. The
  next regression fixture makes aggregate completeness structural rather than hand-maintained.
- PR #469 restored Windows to the required aggregate and added a parser-backed completeness guard;
  its full matrix passed with the now-load-bearing Windows result green in 1m32s. Exact path-set
  fixtures now pin both selected and intentionally unselected specialized gates across every gate
  family, turning subsequent shadow mismatches into explicit regression cases.
- PR #470 merged those exact fixtures. Its script-and-docs change proposed only contracts and docs,
  while the authoritative legacy family also ran three Go shards (up to 3m21s), Postgres (2m04s),
  Windows (1m59s), and image certification (2m30s), all green. This is measured safe
  over-selection rather than a false negative.
- Live branch-protection verification on 2026-08-23 reports `strict: true` for the sole required
  `CI` check, still bound to the GitHub Actions app (`app_id: 15368`). Merge-queue activation is
  blocked by GitHub's organization-ownership requirement, not by workflow readiness:
  `merge_group` and its merge-base handling are already present.
- The pre-activation input audit found non-code files read by Go tests that neither the legacy nor
  shadow maps fully represented: design/configuration/generated-command docs, install docs and
  README, committed OpenAPI, and production Compose. It also found release-contract consumers for
  Dockerfile and packaged notices. Each now selects its consuming gate and has an exact fixture;
  activation waits for this correction to pass the legacy matrix.
