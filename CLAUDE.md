# CLAUDE.md — Loomarr build guide

Loomarr turns a natural-language channel intent into a live, self-maintaining Tunarr channel: suggest a lineup (LLM, grounded), acquire what's missing (Seerr → Sonarr/Radarr), schedule + insert commercial pods, push to Tunarr, backfill as content lands.

**`docs/design.md` is the single source of truth.** If code needs to deviate from it, update the doc in the same PR *first*, then implement. Never let the doc and the code disagree silently.

## Prime directives

1. **One phase per session/PR.** The build plan (design doc §21) has phases 0–14. Do not start phase N+1 until phase N's gate is green and recorded in `PROGRESS.md`.
2. **Gates are hard.** A gate is a set of tests. Never stub, skip, or weaken a test to turn a gate green — if a gate can't pass, the design is wrong or the code is; fix one of them, doc-first.
3. **Never weaken safety for convenience.** Specifically: the grounding rules (§8), the approval gate / authorization model (§7, §11), and forward-only migrations (§16) are not negotiable, including in tests and seed data.
4. **Generated files are never hand-edited**: `api/openapi.yaml` (regenerate via `make openapi`), orval output, goose-applied schemas.
5. **New dependencies are fine when they genuinely help** — record them in design doc §14 in the same PR, with a one-line rationale. A library that solves a real problem (accessibility semantics, a hard algorithm, a protocol) beats hand-rolling it and pretending that was free. What §14 exists to prevent is *unconsidered* dependencies, not useful ones: prefer something already in the tree, prefer a focused library over a framework, and say why in the §14 row. Don't stop to ask permission for a dependency that clearly earns its place.
6. **All application code is Go** (design doc §14 language policy). The only non-Go allowed: FE libraries that compile to embedded static assets, and the vendored `yt-dlp` binary invoked via exec. Never introduce a Python/Node/shell *service* or script as application code — if a task seems to need one, that's a §14 conversation first.

## Session start ritual

1. **Run `/list-agents`.** If another session is live, assume it may be working the same
   thing until you know otherwise, and `SendMessage` to say what you are taking before you
   start. ⚠ Address a peer by the name **and** the `[ref]` the listing shows — a bare name
   is rejected whenever a ref is displayed.
2. Read `PROGRESS.md` — find the active phase.
3. Read **only** the design-doc sections for that phase (map below) plus §14 (stack) and §21 (the phase text itself). Don't load the whole doc; it wastes context.
4. Run `make check` to confirm the tree is green before writing anything.

⚠ **Step 1 exists because it was skipped, on 2026-08-10.** Two sessions independently
regenerated the same ten visual baselines for PR #226, and one had already fixed #236
before the other began. Both appeared in `/list-agents` the whole time — the capability was
never missing, only the habit of looking. It costs one command; skipping it cost two
sessions' worth of Docker Playwright runs on identical PNGs.

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
make check          # fmt + vet + vet-tags + golangci-lint + unit tests (the default gate)
make test           # unit tests only
make go-shard-verify # the GO_SHARD split must be a PARTITION of `go list ./...` (CI red on drift)
                    # ⚠ A real gate, not a sanity check: a split that DROPS a package does not fail
                    # — those tests never run, every shard reports success, and CI is green over
                    # code it did not execute. CI runs this BEFORE the suite.
make vet-tags       # go vet over the `//go:build ffmpeg|eval|integration` sources
                    # ⚠ these files are INVISIBLE to plain `go vet ./...` and to golangci-lint —
                    # both ask the build system which files exist. `go vet ./...` exited 0 while
                    # `go vet -tags '…' ./...` exited 1 for MONTHS, and the one test that proves
                    # programs actually sequence had not compiled since V47 (GH #227 §1).
                    # ⚠ A tagged `go build` would NOT catch this: `go build ./...` skips _test.go
                    # entirely and 9 of the 11 tagged files are tests. Only vet typechecks them.
make tags-verify    # the Makefile's hand-maintained TAGS list still covers every tag in the tree
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
                    # runs the WHOLE suite locally (389 stories x 2 viewports, ~780 tests as of
                    # 2026-08-09); CI splits it with PW_SHARD (wall-clock only, never locally)
                    # ⚠ Don't write the shard COUNT here or in the Makefile. It lives in ci.yml's
                    # `matrix.shard`, and the denominator derives from `strategy.job-total`. This
                    # line used to say "624 … --shard=N/2" long after both had changed.
make fe-visual-update # sanctioned baseline-update path (image diffs reviewed in PR)
make e2e            # wizard flow smoke + page snapshots vs a mocked backend (Docker)
make e2e-update     # sanctioned e2e page-snapshot baseline update (reviewed in PR)
make dev            # dev compose stack
make seed           # populate a dev store (fake users/titles/channels/clips via testkit)
```

CI mirrors `make check` + `openapi-verify` + `test-pg` + `fe` + `e2e`. If a command doesn't exist yet for the active phase, creating it is part of the phase.

**CI runs jobs only when their inputs changed.** A `changes` job diffs against the merge base
and each job gates on it: Go/Postgres on `**/*.go`, `go.mod|sum`, `internal/store/migrations/`,
**`docs/help/`** (embedded in the binary — `retired-verify` reads it), **`scripts/`** (the job
RUNS them — `retired-verify` is `check-retired.sh`, `tags-verify` is `check-tags.sh`; without
it a PR editing only a guard skipped the one job that executes it), `Makefile`, and the
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

**Caching.** Every job that compiles or installs caches its work; the two rules that are easy
to get wrong:

- ⚠ **`actions/cache` never overwrites an existing key** — it skips the save when the key
  already hit. So any cache whose CONTENTS track something the KEY does not (`~/.cache/go-build`
  tracks `.go` source; a `go.sum` key does not) is written once and then frozen forever. Found
  in the wild here: one 473MB entry served every run for days while the source moved under it.
  Use the rolling pattern — `${{ github.run_id }}` in the key so the save always happens, plus
  `restore-keys` prefixes so it still restores the newest prior cache.
- ⚠ **…and a rolling key never PRUNES**, which is the same trade seen from the other end. Neither
  cache mode evicts, so the rolling entry grows by accretion: every run restores the previous one,
  adds to it, and saves the union. Measured 2026-08-09 the Go cache had reached **1.36 GB**, costing
  85s to restore (74s of that EXTRACTING) plus 83–122s to save — ~168s before a test ran. The bound
  is an **ISO-week epoch** (`date -u +%GW%V`) in the key *and* in every restore-keys prefix; a prefix
  that outlives the epoch restores the accretion the epoch exists to drop. It rotates itself, so
  there is no number to remember to bump. Use `%G`+`%V`, never `%Y`+`%V` — mixing calendar year with
  ISO week mints a second key inside one week every New Year.
- ⚠ **Only `main` saves.** A PR's cache is scoped to `refs/pull/N/merge` and can never be read again
  once the PR closes, so a PR-side save is pure eviction pressure on the caches that ARE read. PRs
  restore from main's entry (`actions/cache/restore`) and the save (`actions/cache/save`) is gated
  on `github.ref == 'refs/heads/main'`. ⚠ A cache-KEY change therefore reads COLD on its own PR and
  only pays off after main repopulates it — judge such a PR on the run after the merge, not its own.
- ⚠ **The 10GB repo cap evicts LRU across ALL refs**, so caches from closed PRs do not merely
  sit there — they push out live ones. `cache-cleanup.yml` deletes a PR's caches when it closes
  (GitHub's own 7-day expiry is far too slow when one Go cache is ~470MB). Measured 2026-08-01:
  the repo was at **9.94GB of 10GB with ~6GB owned by already-closed PRs**; clearing them and
  adding the workflow took it to **3.9GB**.

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

**Setting one up.** Prefer `claude --worktree <phase>`; `.worktreeinclude` carries `.env`
in automatically. Then, once, in the new worktree:

```sh
cd web && npx pnpm@11.13.1 install --frozen-lockfile && npx pnpm@11.13.1 codegen
```

A worktree carries tracked files only, so two gitignored things are missing, in order of
how confusing they are:

- **`.env`** — handled by `.worktreeinclude` for worktrees Claude Code creates. A hand-run
  `git worktree add` gets nothing, and that is how `loomarr-msw` ended up with no
  credentials: every live-stack call there fails looking like a code bug. Copy it yourself
  if you create a worktree by hand.
- **`packages/api/generated/`** — `codegen` output. Skip it and every `@loomarr/api` import
  fails to resolve *after* a successful install, so setup looks complete and the typecheck
  disagrees.

Go needs nothing: the module cache is shared and `go build ./...` works immediately.

Exiting a `--worktree` session cleans it up, and the built-in cleanup **refuses to remove a
worktree holding uncommitted or untracked work**. For one made by hand,
`git worktree remove ../loomarr-<phase>`.

⚠ **Worktrees are nearly free here, but not for the reason this file used to give.** It
said the pnpm store "hard-links". It does not — `links=1`, verified. `/home` is btrfs and
pnpm uses REFLINKS, so each worktree's `node_modules` measures ~450MB by `du` while holding
**1MB exclusive** (`btrfs filesystem du`, 2026-08-10). Right conclusion, wrong mechanism —
and on a filesystem without reflinks the real cost is ~450MB apiece.

⚠ **Never park a worktree on `main`.** `loomarr-playout-fixes` does, and it makes
`gh pr merge --delete-branch` exit non-zero — `fatal: 'main' is already used by worktree
at …` — *after the merge has already succeeded*. An agent reads that as a failed merge and
retries it.

### The `using-git-worktrees` skill — where this repo overrides it

The installed `using-git-worktrees` skill (obra/superpowers) is good on the parts this
section does not cover: **detecting** that you are already in a linked worktree
(`git rev-parse --git-dir` vs `--git-common-dir`, with a submodule guard) and asking
consent before creating one. Use it for that.

It disagrees with the above in three places, and **this file wins**:

1. ⚠ **Placement.** The skill defaults to a project-local `.worktrees/`. Use Claude Code's
   own `.claude/worktrees/` instead (gitignored), which is where `--worktree`,
   `EnterWorktree` and `isolation: worktree` put things with no configuration.
2. ⚠ **It will edit and COMMIT `.gitignore`.** Its safety step adds the worktree directory
   and commits that change. `.claude/worktrees/` is already ignored here, so that commit is
   both unrequested and unnecessary. Don't.
3. ⚠ **Its Step 2/3 do not fit.** `go mod download` + `go test ./...` is not this repo's
   setup: the load-bearing step is `pnpm codegen` (the generated API client is gitignored,
   so a fresh worktree typechecks red without it), and the baseline gate is `make check`.

**Prefer the native tooling.** It owns placement, `.env` propagation via
`.worktreeinclude`, and cleanup that refuses to delete work — none of which a manual
`git worktree add` gives you.

⚠ **This section used to mandate SIBLING worktrees (`../loomarr-<phase>`) and, two
paragraphs later, tell you to prefer `EnterWorktree` — which defaults to `.claude/worktrees/`
inside the repo. Following either instruction violated the other**, and every worktree here
was built by hand as a result, skipping `.env` with it.

The stated reason for siblings was that `make e2e` / `e2e-update` bind-mount the repo ROOT
(`-v "$(PWD):/work"`, because e2e needs `internal/web/dist`, outside `web/`), so an in-repo
worktree is mounted into the container. ⚠ **That cost has never been measured.** A bind
mount does not copy anything — the container merely *sees* the path — so it may be free, or
it may matter if something in the run walks the tree. If e2e slows noticeably once several
worktrees exist, measure it before reintroducing a placement rule; do not reinstate it on
the strength of this paragraph alone.

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
