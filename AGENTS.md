# Loomarr agent contract

This file is the canonical contract for every coding agent and human-operated harness. Agent-specific
files may add interface conveniences, but they do not override this file.

## Authority

- `docs/design.md` is the source of truth for behaviour. Amend it in the same PR before code that
  deviates from it.
- Companion design documents in `docs/` own their named domains; `CONTEXT.md` owns vocabulary.
- `PROGRESS.md` records active and shipped work. Read its **Active work** table first; do not load the
  historical tables unless the task needs them.
- Generated artifacts are changed through their generators, never by hand.
- Frontend packages are deep modules — read [`web/packages/README.md`](web/packages/README.md) before
  adding one or importing from one.

## Session lifecycle

From the worktree that will own the change:

```sh
make agent-status
make agent-worktree TOPIC=<short-name> CLAIMS=<comma-separated-shared-outputs>
# cd to the printed worktree; it is already registered
make agent-baseline
```

`agent-status` is the cross-harness roster. A product's own agent-list command is supplementary; it
cannot see agents running in other products. Before starting, resolve any overlapping task or claim.

Use claims for scarce outputs whose conflicts are expensive:

- `openapi-client` — `api/openapi.yaml` and the generated orval client
- `visual-baselines` and `e2e-baselines`
- `tokens`
- `migrations`
- `agent-contract` and `dev-runtime`

Add a domain-specific claim when two changes would edit the same interface or DTO. A worktree isolates
files; the claim identifies the real seam where concurrent work would collide.

During implementation, use `make agent-verify BASE=<base>` for a focused, explicitly non-final check.
Before pushing, run the complete required gates for the touched areas; `make check` is always the Go
gate. Renew a long-running claim with `make agent-renew`; clean abandoned expired entries with
`make agent-prune`. When finished, run `make agent-stop`.

## Delivery

Completed, validated implementation work is published as a pull request and set to auto-merge by
default. Leave it as a draft only while required gates or requested work remain. Do not publish or
enable auto-merge when the task explicitly asks for local-only changes, a review checkpoint, or a
different delivery path.

## Prime directives

1. Gates are hard. Never stub, skip, delete, or weaken a test to make a gate green.
2. Never weaken grounding, the approval gate, authorization, or forward-only migrations, including in
   tests and seed data.
3. New dependencies require a design §14 amendment with a one-line rationale in the same PR.
4. Application code is Go except for the frontend build, the vendored `yt-dlp` executable, and the
   required `loomarr-image` Rust worker documented in design §14 and §22. Do not introduce another
   application runtime.
5. Unit tests never touch the network. Extend `internal/testkit`; do not create private service mocks.
6. Store conformance remains one suite over SQLite and Postgres.
7. Never run `make smoke*` from an agent session. It drives the maintainer's live stack.

## Commands

`make check` is the default gate. One focused test:

```sh
go test -race -run TestName ./internal/<pkg>/
```

The complete, generated target reference is `docs/dev/commands.md`. Fix a Makefile `##` description
and run `make dev-docs`; never copy target lists into prose.

Useful local interfaces:

```sh
make doctor                 # toolchain, worktrees, ports, caches, artifacts
make bootstrap              # pnpm install + codegen + local directories
make agent-env              # this worktree's runtime addresses
make dev-be                 # isolated backend with Air
make dev-fe                 # isolated Vite frontend pointed at that backend
```

Go 1.26+, the Rust toolchain pinned by `rust-toolchain.toml`, Node 22.x (22.5 minimum), pnpm 11.13.1,
and Docker are required. ffmpeg/ffprobe are required for playout tests. Lint tools and Air run at
pinned versions from the harness.

## Generated files

- `api/openapi.yaml` → `make openapi`; verify with `make openapi-verify`
- `docs/configuration.md` → `make config-docs`
- `docs/dev/commands.md` → `make dev-docs`
- `docs/design.md` §2 map → `make arch-docs`
- `web/packages/tokens/generated/` → `make fe-tokens`
- `web/packages/api/generated/` and `web/apps/web/src/routeTree.gen.ts` → `make fe-codegen`; both are
  gitignored and absent from a fresh worktree
- applied migrations in `internal/store/migrations/` are immutable; add the next migration

## Repository map

- `cmd/loomarr` is the entrypoint; `internal/app` is the composition root; `internal/api` owns Huma
  routes.
- Domain packages under `internal/` map to the ports documented in design §2.
- `internal/testkit` is the shared mock and pinned-fixture module.
- `web/` is a pnpm workspace: `apps/web` plus `packages/{api,core,tokens,fixtures}`. Frontend request
  types come from orval and are never handwritten.

## Local runtime and worktrees

Create a sibling worktree through the harness:

```sh
make agent-worktree TOPIC=<branch>
```

It branches off a freshly fetched `origin/main` — not whatever branch the primary worktree is parked
on — so a new worktree always starts from current main. To stack deliberately on the current branch (or
any other base), pass `BASE=HEAD` (or `BASE=<ref>`). It installs frontend dependencies and runs codegen. Credentials are not copied by default; use
`COPY_ENV=1` only when the task genuinely needs the maintainer's configured integrations. Secondary
worktrees receive deterministic, distinct backend/frontend/Storybook/Tunarr ports, a Compose project,
an SQLite database, a prepared-publication library, a filler drop folder, and
`.artifacts/<instance>/`.

Do not park a secondary worktree on `main`. Never remove a worktree containing uncommitted or untracked
work. Use `git worktree list` and `make agent-status` before cleanup.

Develop against the URL printed by `make dev-fe`; the backend URL serves the last embedded SPA build and
can look stale by design. A bare `go run ./cmd/loomarr` can orphan a stale child; use `make dev-be`.

## Hard rules

- Auth changes include the design §19 negatives: member 403s and sessions dying on disable.
- Retiring a capability adds its identifier to `scripts/check-retired.sh` in the same PR.
- Adding a setting changes design §15 first.
- Adding a build tag changes the guarded `TAGS` list in the Makefile.
- Adding a CI build input changes the per-job filter in `.github/workflows/ci.yml`; never add a
  workflow-level `paths:` filter.
- Frontend work uses the Vite server, not the stale SPA embedded on the backend port.

## Stop points

Stop and ask the maintainer for a Phase-0 contract deviation, an authorization/safety change, a gate
that appears to require weakening, or a new authority beyond the requested task. Preserve unrelated
dirty files and worktrees.

## Agent adapters and skills

Durable workflows live in `.agents/workflows/`; installed skill bodies live in `.agents/skills/`.
Agent-specific directories such as `.claude/` contain adapters or symlinks only. A required workflow
must be usable without a proprietary slash command, home-directory plan, or product-specific worktree
feature.

One worktree owns implementation and delivery. Use subagents for independent research, competing
designs, and fresh-context review; they report to the owner unless a separate editing worktree has a
clear file seam, interface seam, claim set, and merge order. For dependent branches, record
`DEPENDS_ON` and stack with `BASE=<dependency-branch>`. See `docs/dev/agents.md` and the curated
catalog in `docs/dev/skills.md`.
