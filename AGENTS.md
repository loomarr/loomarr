# AGENTS.md

**Read `CLAUDE.md` first** — it is the full build guide (prime directives, phase→doc map,
worktree rules, stop points). **`docs/design.md` is the single source of truth**: if code
must deviate, amend the doc in the same PR *before* implementing. **`PROGRESS.md`** is the
phase record — read it at session start to find what's active and what's already shipped.
This file is only the quick-ramp distillation; the two above win on any conflict.

## Commands

**`make check` is THE gate.** One test: `go test -race -run TestName ./internal/<pkg>/`.

⚠ **The target list is not copied here.** It is generated into
[`docs/dev/commands.md`](docs/dev/commands.md) from the Makefile and the CI workflows, and
gated by `make dev-docs-verify`. This block used to hold a hand-written copy that omitted
`vet-tags` from `make check` — precisely the step the rest of the repo spends the most words
explaining. Fix the Makefile's `##` doc comment and regenerate; never re-add a copy.

Prereqs: Go 1.26+, Node 22.5+, pnpm, Docker (test-pg, fe-visual, e2e). Lint tools run via
`go run <tool>@<pin>` — nothing needs a global install. Setup detail:
[`docs/dev/setup.md`](docs/dev/setup.md).

**Never run `make smoke*` from an agent session** — it drives the maintainer's live stack.

## Generated files — never hand-edit

- `api/openapi.yaml` → `make openapi` (`make openapi-verify` fails CI on drift)
- `docs/configuration.md` → `make config-docs`
- `web/packages/api/generated/` (orval client) — **gitignored**. A fresh clone or worktree
  typechecks red until you run `cd web && pnpm install --frozen-lockfile && pnpm codegen`.
- `web/packages/tokens/generated/` → `make fe-tokens`
- goose-applied migrations in `internal/store/migrations/` — forward-only; add, never edit.

## Layout (what owns what)

- `cmd/loomarr` — entrypoint; `internal/app` — composition root; `internal/api` — Huma routes.
- Domain packages under `internal/` map to design-doc ports: `suggest` (LLM grounding),
  `binder`, `channels`, `schedule` (curation engine), `reconcile`, `library`, `requester`,
  `filler`/`clipfetch`, `scheduler`, `settings`, `store`, `recurate`.
- `internal/testkit` — the ONE shared set of mocks + pinned fixtures. Never invent private
  mocks; extend testkit. Unit tests never touch the network (§19).
- Store conformance is ONE suite over two backends (SQLite + Postgres) — never fork
  assertions per dialect.
- `web/` — pnpm monorepo: `apps/web` (Vite/React SPA embedded into the binary),
  `packages/{api,core,tokens,fixtures}`. FE request types come from orval — never hand-write.

## Dev loop gotchas

- Develop against **:5173** (`pnpm --filter @loomarr/web dev` from `web/`, proxies `/v1` to
  :8080). :8080 serves the SPA baked into the binary — stale-looking by design.
- Backend live-reload is `make dev-be` (Air). A bare `go run ./cmd/loomarr` supervises rather
  than execs — killing the terminal can orphan a stale binary serving pre-change code for
  hours. If an API change "isn't showing up", check
  `curl -s localhost:8080/v1/system/version` for the build commit.

## Hard rules (violating these is a design conversation, not a workaround)

- Never weaken the grounding rules, approval gate, or authorization model — including in
  tests and `make seed` (seed must acquire via the admin path, not write `available` rows).
- No new dependencies without amending design §14 in the same PR. All application code is Go.
- Auth tests must include the §19 negatives (member 403s, sessions die on disable).
- Retiring a capability → add its identifier to `scripts/check-retired.sh` in the same PR.
- Adding a setting → design §15 first; adding a config knob not in §15 is a doc change first.

## CI

- `ci-ok` is the single required check; jobs are filtered per-input by a `changes` job
  (Go on `**/*.go`/`go.mod|sum`/migrations/`docs/help/`/Makefile/workflow; FE on `web/`/
  Makefile/workflow; **Image on `Dockerfile`/`.dockerignore` only**). No usable merge base →
  everything runs. Adding a new build input means adding it to the filter in the same PR.
  Never use a workflow-level `paths:` filter. On a PR the filter diffs against the MERGE
  BASE, not the last push.
- The **Image** job builds both release platforms (`linux/amd64,linux/arm64`) under QEMU. It
  is the deliberate exception to the Makefile/workflow rule — emulation is expensive and
  neither file changes what `docker build` produces — and the only job with a timeout.
- ⚠ **`make ci-lint` is weaker locally than in CI**: actionlint shells out to shellcheck and
  SILENTLY SKIPS that half when shellcheck is not on PATH, so it exits 0 locally and fails
  in CI. Install shellcheck before doubting CI.
- ⚠ **`actions/cache` never overwrites an existing key.** Any cache whose contents track
  something the key does not (`~/.cache/go-build` tracks `.go` source; a `go.sum` key does
  not) is written once and frozen. Use `${{ github.run_id }}` in the key + `restore-keys`
  prefixes. Closed-PR caches evict live ones LRU across the 10GB cap — `cache-cleanup.yml`
  drops them on PR close.

## Git worktrees (parallel sessions)

- Sibling placement: `git worktree add ../loomarr-<phase> -b <phase>` — NOT inside the repo
  (Playwright bind-mounts the repo root into containers).
- After adding: `cd ../loomarr-<phase>/web && pnpm install --frozen-lockfile && pnpm codegen`
  (the gitignored orval client, or every `@loomarr/api` import fails to resolve).
- Only split genuinely disjoint work: two sessions editing the same generated output
  (`api/openapi.yaml`, orval client, visual baselines) will conflict miserably.

## Agent skills

`docs/agents/*.md` is the config the installed mattpocock skills read (issue tracker =
GitHub via `gh`, the five triage labels, `CONTEXT.md` as glossary). Edit those files
directly; `.agents/skills/` + `.claude/skills/` symlinks are the skill bodies (skills.sh
install — update with `npx skills update`).
