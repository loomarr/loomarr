# CLAUDE.md — Loomarr build guide

Loomarr turns a natural-language channel intent into a live, self-maintaining Tunarr channel: suggest a lineup (LLM, grounded), acquire what's missing (Seerr → Sonarr/Radarr), schedule + insert commercial pods, push to Tunarr, backfill as content lands.

**`docs/design.md` is the single source of truth.** If code needs to deviate from it, update the doc in the same PR *first*, then implement. Never let the doc and the code disagree silently.

## Prime directives

1. **One phase per session/PR.** The build plan (design doc §21) has phases 0–14. Do not start phase N+1 until phase N's gate is green and recorded in `PROGRESS.md`.
2. **Gates are hard.** A gate is a set of tests. Never stub, skip, or weaken a test to turn a gate green — if a gate can't pass, the design is wrong or the code is; fix one of them, doc-first.
3. **Never weaken safety for convenience.** Specifically: the grounding rules (§8), the approval gate / authorization model (§7, §11), and forward-only migrations (§16) are not negotiable, including in tests and seed data.
4. **Generated files are never hand-edited**: `api/openapi.yaml` (regenerate via `make openapi`), orval output, goose-applied schemas.
5. **No new dependencies** beyond design doc §14 without updating §14 in the same PR, with a one-line rationale.

## Session start ritual

1. Read `PROGRESS.md` — find the active phase.
2. Read **only** the design-doc sections for that phase (map below) plus §14 (stack) and §21 (the phase text itself). Don't load the whole doc; it wastes context.
3. Run `make check` to confirm the tree is green before writing anything.

### Phase → design-doc section map

| Phase | Read sections |
| --- | --- |
| 0 Contract spikes | §6, §9, §21 phase 0 |
| 1 Scaffold + harness | §14, §15, §16 (Dockerfile/compose), §21 |
| 2 Provisioner domain | §3, §4 |
| 3 Store + SQLite | §5 |
| 4 Postgres | §5 (esp. concurrency), §18 |
| 5 Library adapter | §6 (Library + auth), §11 (flavor login header) |
| 6 Requester + ingest | §6 (Requester, Ingest), §4 |
| 7 Reconciler + janitor | §4, §5 (retention), §18 |
| 8 API + OpenAPI + backup | §7, §7.1, §16 (backup) |
| 9 Users & auth | §11, §7 (authorization model) |
| 10 Scheduler + Tunarr | §9, §6 (Programmer + resilience), §18 |
| 11 Suggester + search | §8, §7.2 |
| 12 Commercials & filler | §10 |
| 13 Web UI + onboarding | §12, §13, §14 (FE) |
| 14 Docs & ship | §13 (docs set), §16 |

## PROGRESS.md format

A table the agent maintains — one row per phase: `phase | status (todo/active/done) | gate evidence (commit SHA + test command that proves it) | notes/deviations`. Phase-0 findings (contract surprises, Tunarr version, API-key answer) go in notes.

## Commands (the harness contract — created in phase 1, used forever)

```
make check          # fmt + vet + golangci-lint + unit tests (the default gate)
make test           # unit tests only
make test-pg        # store conformance vs Postgres (testcontainers; requires Docker)
make openapi        # export api/openapi.yaml from the running definitions
make openapi-verify # regenerated spec must match committed (CI red on drift)
make fe             # orval typegen + tsc + vitest
make e2e            # Playwright smoke vs mocked backend
make dev            # dev compose stack
make seed           # populate a dev store (fake users/titles/channels/clips via testkit)
```

CI mirrors `make check` + `openapi-verify` + `test-pg` + `fe` + `e2e`. If a command doesn't exist yet for the active phase, creating it is part of the phase.

## Environment prerequisites

Go 1.22+, Node 20+. **Docker is required from phase 4 onward** (testcontainers) — verify with `docker info` during phase 1 and record in PROGRESS.md. Playwright browsers install in phase 13. If Docker is unavailable in the current environment, stop and tell the maintainer; do not fake the Postgres conformance suite.

## Testing rules

- **Unit tests never touch the network.** All external services are mocked through `internal/testkit` — one shared implementation per service (media server in both flavors, Tunarr, Seerr, TMDB, LLM, Sonarr/Radarr webhook sender). Phases do not invent private mocks; extend the testkit.
- **Fixtures are pinned truth.** Webhook payloads and Tunarr responses in `internal/testkit/fixtures/` come from Phase 0 captures with source-version comments. Write parsers against the fixtures, not against remembered field names.
- **Determinism:** pod assembly and any shuffling take an explicit seed in tests (§10).
- Store conformance is **one suite, two backends** — never fork the assertions per dialect.
- Auth tests must include the negative cases (§19): member 403s on titles/approve/admin routes; sessions die on disable.

## Do-nots

- Don't call live services from any test (Phase 0 is the only sanctioned live contact, and it's maintainer-supervised).
- Don't invent API fields, endpoints, or enum values not present in the design doc / committed `api/openapi.yaml`.
- Don't bypass the approval gate anywhere — including `make seed`, which must create acquisitions via an admin path, not by writing `available` rows for unapproved titles.
- Don't edit applied migrations; add new ones (forward-only, §16).
- Don't hand-write FE request types; regenerate via orval.
- Don't add config that isn't in §15; if a knob is needed, add it to §15 first.

## Ask the maintainer (stop points)

- Any Phase-0 contract deviation from §6/§9 (Tunarr shape, webhook payloads, auth quirks).
- The Tunarr API-key question (§6) if Phase 0 doesn't settle it.
- Go module path, license, and name availability (§20) before anything is published.
- Any gate that seems to require weakening a prime directive — that's a design conversation, not a workaround.

## Definition of done

Two halves (design doc §21): the **automated** DoD runs in CI against the testkit; the **manual smoke** runs on the maintainer's real stack (wizard all-green, a real intent → approved → a channel actually playing in Tunarr with ad pods). The build isn't done until both are.
