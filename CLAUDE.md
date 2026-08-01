# CLAUDE.md — Loomarr build guide

Loomarr turns a natural-language channel intent into a live, self-maintaining Tunarr channel: suggest a lineup (LLM, grounded), acquire what's missing (Seerr → Sonarr/Radarr), schedule + insert commercial pods, push to Tunarr, backfill as content lands.

**`docs/design.md` is the single source of truth.** If code needs to deviate from it, update the doc in the same PR *first*, then implement. Never let the doc and the code disagree silently.

## Prime directives

1. **One phase per session/PR.** The build plan (design doc §21) has phases 0–14. Do not start phase N+1 until phase N's gate is green and recorded in `PROGRESS.md`.
2. **Gates are hard.** A gate is a set of tests. Never stub, skip, or weaken a test to turn a gate green — if a gate can't pass, the design is wrong or the code is; fix one of them, doc-first.
3. **Never weaken safety for convenience.** Specifically: the grounding rules (§8), the approval gate / authorization model (§7, §11), and forward-only migrations (§16) are not negotiable, including in tests and seed data.
4. **Generated files are never hand-edited**: `api/openapi.yaml` (regenerate via `make openapi`), orval output, goose-applied schemas.
5. **No new dependencies** beyond design doc §14 without updating §14 in the same PR, with a one-line rationale.
6. **All application code is Go** (design doc §14 language policy). The only non-Go allowed: FE libraries that compile to embedded static assets, and the vendored `yt-dlp` binary invoked via exec. Never introduce a Python/Node/shell *service* or script as application code — if a task seems to need one, that's a §14 conversation first.

## Session start ritual

1. Read `PROGRESS.md` — find the active phase.
2. Read **only** the design-doc sections for that phase (map below) plus §14 (stack) and §21 (the phase text itself). Don't load the whole doc; it wastes context.
3. Run `make check` to confirm the tree is green before writing anything.

### Phase → design-doc section map

| Phase | Read sections |
| --- | --- |
| 0 Contract spikes | §6, §9, §21 phase 0 |
| 1 Scaffold + harness | §14, §15, §16 (Dockerfile/compose), §21 + `docs/config-design.md` §2–§4 |
| 2 Provisioner domain | §3, §4 |
| 3 Store + SQLite | §5 |
| 4 Postgres | §5 (esp. concurrency), §18 |
| 5 Library adapter | §6 (Library + auth), §11 (flavor login header) |
| 6 Requester + ingest | §6 (Requester, Ingest), §4 |
| 7 Reconciler + janitor | §4, §5 (retention), §18 |
| 8 API + OpenAPI + backup | §7, §7.1, §16 (backup) + `docs/config-design.md` §8 |
| 9 Users & auth | §11, §7 (authorization model) + `docs/config-design.md` §4 (secrets) |
| 10 Scheduler + Tunarr | §9, §6 (Programmer + resilience), §18 + `docs/programming-design.md` §2–§8 |
| 11 Suggester + search | §8, §8.1 (model selection), §7.2 + `docs/programming-design.md` §2, §8 |
| 12 Commercials & filler | §10 |
| 13 Web UI + onboarding | §12, §13, §14 (FE) + `docs/frontend-design.md` (all of it) + `docs/config-design.md` §5–§7 |
| 14 Docs & ship | §13 (docs set), §16 |

## PROGRESS.md format

A table the agent maintains — one row per phase: `phase | status (todo/active/done) | gate evidence (commit SHA + test command that proves it) | notes/deviations`. Phase-0 findings (contract surprises, Tunarr version, API-key answer) go in notes.

## Commands (the harness contract — created in phase 1, used forever)

```
make check          # fmt + vet + golangci-lint + unit tests (the default gate)
make test           # unit tests only
make test-pg        # store conformance vs Postgres (testcontainers; requires Docker)
make openapi        # export api/openapi.yaml from the running definitions
make config-docs    # generate docs/configuration.md from the settings registry (CI diffs must be empty)
make openapi-verify # regenerated spec must match committed (CI red on drift)
make retired-verify # retired identifiers must not appear as live instructions (CI red on drift)
make ci-lint        # actionlint over .github/workflows (a workflow can be valid YAML and still be rejected)
                    # ⚠ needs shellcheck on PATH — without it actionlint SKIPS the shell half and exits 0 locally while CI fails
make fe             # orval typegen + Biome + tsc + vitest (jsdom units + Storybook browser tests)
make fe-tokens      # regenerate token artifacts from packages/tokens (CI diffs must be empty)
make storybook      # Storybook dev workshop (the component gallery/contract)
make storybook-build # offline storybook-static build (what fe-visual snapshots)
make fe-visual      # Playwright visual suite over the Storybook stories (storybook-static)
make fe-visual-update # sanctioned baseline-update path (image diffs reviewed in PR)
make e2e            # wizard flow smoke + page snapshots vs a mocked backend (Docker)
make e2e-update     # sanctioned e2e page-snapshot baseline update (reviewed in PR)
make dev            # dev compose stack
make seed           # populate a dev store (fake users/titles/channels/clips via testkit)
```

CI mirrors `make check` + `openapi-verify` + `test-pg` + `fe` + `e2e`. If a command doesn't exist yet for the active phase, creating it is part of the phase.

**CI runs jobs only when their inputs changed.** A `changes` job diffs against the merge base
and each job gates on it: Go/Postgres on `**/*.go`, `go.mod|sum`, `internal/store/migrations/`,
**`docs/help/`** (embedded in the binary — `retired-verify` reads it), `Makefile`, and the
workflow itself; Frontend/Playwright on `web/`, `Makefile`, and the workflow; **Image on
`Dockerfile` and `.dockerignore` ONLY**. `Makefile` and the workflow deliberately gate Go and
Frontend — they define how those jobs run.

⚠ **The Image job is the deliberate exception to the `Makefile`/workflow rule.** It builds BOTH
release platforms (`linux/amd64,linux/arm64`) under QEMU, so a cold build costs ~30 min of billed
CI; gating it on two frequently-edited files that cannot change what `docker build` produces would
spend that on every workflow tweak. It is also the only job with a `timeout-minutes` — GitHub's
default is six hours, which is a lot of money for a hung emulated build.

It exists because a Dockerfile that could never build for arm64 sat undetected: `apt` exited 100
on `intel-media-va-driver`, which has no arm64 candidate, and since the image was previously built
only by `release.yml` on a `v*` tag, the first symptom would have been a failed release. **Build
both platforms or the job cannot catch the arch-specific class it was added for.**

Two rules if you touch this:

- **`ci-ok` is the single required check**, always runs, and inspects `needs.*.result`
  explicitly. A skipped job does not fail an aggregate by default — and neither does a FAILED
  one under `if: always()` — so a naive shim reports green over a red job.
- **Never use a workflow-level `paths:`.** A run that does not trigger reports no checks at
  all, so a required check sits "expected" forever and the PR cannot merge. Filter per job.

The filter fails SAFE: no usable merge base (first push, force-push, new branch) runs
everything. Adding a new build input means adding it to the filter in the same PR — the same
class of hand-maintained list as `scripts/check-retired.sh`.

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
- **When a PR retires a capability, add its identifier to `scripts/check-retired.sh` in the same PR.** `docs/help/` ships inside the binary and is read as instructions: the deleted `/hooks/arr` webhook kept being documented as a setup step, telling operators to set a secret that was never minted, while `docs/help/troubleshooting.md` described the correct polling behaviour one file over. A prose rule would not have caught that; a grep catches it forever.
- Don't hand-write FE request types; regenerate via orval.
- Don't add config that isn't in §15; if a knob is needed, add it to §15 first.

## Parallel sessions (git worktrees)

Git worktrees, one session each — **for genuinely disjoint work**. One worktree per phase,
never per branch-you-might-need.

**The dependency analysis is what makes this safe, not the worktree.** Two sessions editing
the same domain produce a conflict in *generated* output — `api/openapi.yaml`, orval's
client, the token artifacts — and a conflict in generated files is miserable to resolve
because the merge tool is reconciling output nobody wrote. Without the analysis you are
only creating conflicts faster.

Before splitting, check the candidates against each other:

| Safe together | Not safe together |
| --- | --- |
| Disjoint domains with no shared DTO (Help + Filler) | Anything where both phases add an endpoint — both edit `api/openapi.yaml` |
| One backend phase + one pure-frontend phase on a different surface | Two phases touching one DTO (`ChannelDTO`, `ProposalDTO`) |
| A docs-only pass + a code phase | Two phases that both regenerate baselines |

⚠ **Do not name specific phases here.** This warning used to say *"V25b and V16 are NOT
disjoint — run them sequentially"*; both shipped, and the sentence stayed, telling anyone
reading the session-start ritual to plan around work that was already done. `PROGRESS.md`
is the phase record — read its **Next up** line, which carries the same lesson about itself.

The durable version of the warning is the test above, not a roster: **two phases are unsafe
together when they touch the same generated output.** In practice that means anything adding
an endpoint (both edit `api/openapi.yaml` and regenerate the orval client), sharing a DTO, or
regenerating the same visual baselines.

A single session working through phases in order needs no worktree at all — there is no
concurrency to isolate, and the branch is doing the same job.

**Setting one up.** A worktree carries tracked files only, so everything gitignored is
missing — and the non-obvious one is the generated API client, not `node_modules`:

```
git worktree add ../loomarr-<phase> -b <phase>
cd ../loomarr-<phase>/web
npx pnpm@11.13.1 install --frozen-lockfile   # ~2s; the pnpm store is shared, so it hard-links
npx pnpm@11.13.1 codegen                     # REQUIRED — packages/api/generated/ is gitignored
```

Skip `codegen` and every `@loomarr/api` import fails to resolve, *after* a successful
install — so the setup looks complete and the typecheck says otherwise. Go needs nothing:
the module cache is shared and `go build ./...` works immediately.

`git worktree remove ../loomarr-<phase>` when the phase merges. `node_modules` is ~470MB
per worktree, which is disk rather than download.

### The `using-git-worktrees` skill — where this repo overrides it

The installed `using-git-worktrees` skill (obra/superpowers) is good on the parts this
section does not cover: **detecting** that you are already in a linked worktree
(`git rev-parse --git-dir` vs `--git-common-dir`, with a submodule guard) and asking
consent before creating one. Use it for that.

It disagrees with the above in three places, and **this file wins**:

1. ⚠ **Placement.** The skill defaults to a project-local `.worktrees/`; this repo uses a
   SIBLING directory (`../loomarr-<phase>`). Sibling placement is deliberate — the
   Playwright targets bind-mount the repo root into a container, so a worktree *inside*
   the root would be mounted into every visual run.
2. ⚠ **It will edit and COMMIT `.gitignore`.** Its safety step adds the worktree directory
   and commits that change. `.worktrees/` is not ignored here, so following the skill
   unmodified produces an unrequested commit. Don't; use the sibling path instead.
3. ⚠ **Its Step 2/3 do not fit.** `go mod download` + `go test ./...` is not this repo's
   setup: the load-bearing step is `pnpm codegen` (the generated API client is gitignored,
   so a fresh worktree typechecks red without it), and the baseline gate is `make check`.

Prefer the native `EnterWorktree` tool when one is available — the skill says so itself,
and it owns placement and cleanup that manual `git worktree add` leaves as phantom state.

## Ask the maintainer (stop points)

- Any Phase-0 contract deviation from §6/§9 (Tunarr shape, webhook payloads, auth quirks).
- The Tunarr API-key question (§6) if Phase 0 doesn't settle it.
- Go module path, license, and name availability (§20) before anything is published.
- Any gate that seems to require weakening a prime directive — that's a design conversation, not a workaround.

## Agent skills

Config the `mattpocock-skills` engineering skills read. Edit `docs/agents/*.md` directly.

### Issue tracker

GitHub Issues (`gh` CLI) — for work that is NOT a phase. ⚠ `PROGRESS.md` remains the phase
record; an issue duplicating a phase row is a process bug. See `docs/agents/issue-tracker.md`.

### Triage labels

The five canonical roles, unchanged (`needs-triage`, `needs-info`, `ready-for-agent`,
`ready-for-human`, `wontfix`). Only `wontfix` exists today. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context. **`CONTEXT.md` (repo root) is the glossary — what a word MEANS**; it holds no
behavior. **`docs/design.md` stays the source of truth — what the system DOES**, and wins on
every overlap. ⚠ A `CONTEXT.md` that grows into a spec becomes the second authority the
doc-first directive exists to prevent. No `docs/adr/`. See `docs/agents/domain.md`.

## Companion & seed docs

**Companion design docs — in `docs/`, authoritative for their own domains:**
- **`docs/programming-design.md`** — the ChannelPolicy heuristics: extract-vs-enforce split, scope/audience/separation/ordering/seasonality, the relaxation ladder, the extensibility checklist. Phases 10, 11, and 13 build against it.
- **`docs/config-design.md`** — the settings subsystem mechanics: the typed registry, `env > database > default` resolution, hot-apply, secrets lifecycle, Settings IA, wizard-as-settings. Phases 1, 8, 9, and 13 build against it.
- **`docs/frontend-design.md`** — authoritative for how the frontend looks and is built (tokens, palette, component library, visual testing, mobile-readiness). Incorporated in phase 14 (was `loomarr-frontend-design.md` at the repo root).
- **`docs/integrations/media-server-livetv.md`** — the media-server Live TV wiring summary; its troubleshooting hooks are folded into `docs/help/troubleshooting.md`. Incorporated in phase 14 (was `docs-livetv-integration.md`).

**Remaining seed artifact (not in `docs/`):**
- **`design/loomarr-prototype-{desktop,mobile}.dc.html`** — the Claude Design mock: the authoritative visual reference for phase 13. Recreate pixel-perfectly (per `design/README.md`) with the two deltas noted in `docs/frontend-design.md` §7 (badge `-300` text stops; `static-500` disabled-only). These are `.dc.html` prototypes (a design-tool format + `support.js` runtime), NOT the shippable frontend — phase 13 builds the real Vite/React app to match their rendered output.

Precedence for all of them: `docs/design.md` wins on *behavior* (endpoints, flows, auth, phases); each companion/seed wins on its own domain (programming heuristics, config mechanics, look/onboarding). A companion or seed that contradicts the design doc gets corrected, not followed.

## Definition of done

Two halves (design doc §21): the **automated** DoD runs in CI against the testkit; the **manual smoke** runs on the maintainer's real stack (wizard all-green, a real intent → approved → a channel actually playing in Tunarr with ad pods). The build isn't done until both are.
