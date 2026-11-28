# Loomarr agent contract

This file is the canonical contract for every coding agent and human-operated harness. Agent-specific
files may add interface conveniences, but they do not override this file.

## Authority

- The design docs are the source of truth for behaviour: `docs/design/` (start at its README),
  plus the `docs/design.md` sections not yet moved there (#779). Amend them in the same PR before
  code that deviates from them. Old `§N` references resolve through the index's Old § column.
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
```

`agent-status` is the cross-harness roster. A product's own agent-list command is supplementary; it
cannot see agents running in other products. Before starting, resolve any overlapping task or claim.

### Delegation

One agent owns each deliverable and does its coupled work itself: a sequential reasoning chain,
edits to one seam, or work whose contract is still moving. Delegate an assignment when it is
independent and bounded, and a child saves elapsed time, keeps noisy context out of the owner's
window, or supplies a fresh view: evidence collection, competing designs, a disjoint implementation
seam, or review. Fix the shared contract first, then fan out, sending independent assignments
together so they run concurrently. Codex and Claude follow the same rule.

Non-trivial implementation receives a fresh-context child review of the fixed final diff before
delivery; unresolved findings return to the owner for repair and a fresh bounded review.

Children report to the owner and delegate further only when their brief permits it. A read-only
child is a native subagent of the owner. An editing child needs its own worktree and an explicit
file seam, interface seam, claim set, and merge order; without all of them it stays read-only. In
Orca, the primary harness, an editing child or a child that must be a different agent is a worker
started through the installed `orchestration` skill, in a worktree nested under the owner's. The
owner verifies evidence, integrates changes, runs final gates, and owns delivery and cleanup.
Delegation grants no additional authority: claims, one-writer ownership, and gates still apply.

Give each child a bounded brief: scope, acceptance, and return format. Choose its model by the
[routing policy](.agents/workflows/supervise.md#route-each-assignment) instead of inheriting the
parent's. Every editing child, several concurrent workers, a PR backlog, and any request to
supervise or coordinate agents are supervision: follow
[the supervisor workflow](.agents/workflows/supervise.md), which owns the worker brief, token
budgets, launch checks, and the supervision loop.

### Claims

Use claims for scarce outputs whose conflicts are expensive:

- `openapi-client` — `api/openapi.yaml` and the generated orval client
- `visual-baselines` and `e2e-baselines`
- `tokens`
- `migrations`
- `agent-contract` and `dev-runtime`

Add a domain-specific claim when two changes would edit the same interface or DTO. A worktree isolates
files; the claim identifies the real seam where concurrent work would collide.

### Verification and cleanup

During implementation, run the smallest focused test that covers the edit. When the exact test is not
known, `make test-affected BASE=<base>` runs direct Go packages, related Vitest tests, and directly
owned script/native/Rust unit suites as development feedback. It deliberately skips reverse dependants,
lint, publication prerequisites, and protected integration gates. Before pushing, run the focused
tests for each package and frontend surface the diff touches, once the diff is stable, with `-race`
for a package whose concurrency the diff changes. CI is the gate: pull-request CI runs the fast
affected checks (lint, Go contracts, frontend), and `CI assurance` in the merge queue runs the
complete affected set, including the reverse-dependant Go closure under `-race` and Postgres, with a
failure there reverted first. `make verify BASE=<base>` reproduces that
selection locally at a workstation's cost, so run it only to reproduce a CI failure, when CI is
unavailable, or when the diff changes the gate or classifier machinery, and keep it to one run per
machine at a time. Reserve
`make verify SCOPE=all` and `make agent-baseline` for an explicitly requested complete audit, changes to the
gate/classifier machinery, or diagnosis of a classifier boundary—not merely because a task started.
Renew a long-running claim with `make agent-renew`; clean abandoned expired entries with
`make agent-prune`. When finished, run `make agent-stop`; after merge, audit and retire completed
worktrees with `make agent-gc` and then `make agent-gc APPLY=1`. The maintainer has authorized every
agent to run `APPLY=1` without asking; its eligibility checks are the safeguard.

## Delivery

Completed, validated implementation work is published as a pull request and set to auto-merge by
default. Leave it as a draft only while required gates or requested work remain. Do not publish or
enable auto-merge when the task explicitly asks for local-only changes, a review checkpoint, or a
different delivery path.

## Prime directives

1. Gates are hard. Never stub, skip, delete, or weaken a test to make a gate green.
2. Never weaken grounding, the approval gate, authorization, or forward-only migrations, including in
   tests and seed data.
3. New dependencies require a row in `docs/design/dependencies.md` with a one-line rationale in the
   same PR.
4. Application code is Go except for the frontend build, the vendored `yt-dlp` executable, and the
   required `loomarr-image` Rust worker documented in `docs/design/dependencies.md` and design §22. Do not introduce another
   application runtime.
5. Unit tests never touch the network. Extend `internal/testkit`; do not create private service mocks.
6. Store conformance remains one suite over SQLite and Postgres.
7. Never run `make smoke*` from an agent session. It drives the maintainer's live stack.
8. Do not preserve legacy formats, schemas, adapters, or code paths speculatively. Keep compatibility
   only for verified live data, active users, or an external contract that cannot be migrated in the
   same change; otherwise migrate or remove the obsolete path.
9. Pin every installed third-party dependency to one exact reviewed version. Installed-dependency
   manifest sections must not use semver ranges or floating tags; container images and remote CI
   actions must use immutable digests or commit SHAs. Peer ranges remain compatibility contracts,
   not selected installations. Workspace links and operator-supplied Loomarr release variables are
   the other exceptions. Compatibility authorities such as Expo may require an older exact version;
   record the hold beside the pin instead of widening its range.

## Commands

`make test-affected` is the fallback edit-loop interface when the exact test is unknown. `make verify`
reproduces CI's affected gates locally; its default is affected evidence and `SCOPE=all` is the
explicit complete-repository audit. Use the comprehensive scope only when the task requests it, changes
the gate machinery/classifier, or needs to diagnose a boundary. One exact focused test:

```sh
go test -race -run TestName ./internal/<pkg>/
```

The complete, generated target reference is `docs/reference/make.md`. Fix a Makefile `##` description
and run `make dev-docs`; never copy target lists into prose.

Useful local interfaces:

```sh
make doctor                 # toolchain, worktrees, ports, caches, artifacts
make bootstrap              # pnpm install + codegen + local directories
make test-affected BASE=<base> # direct/related unit feedback while editing
make verify BASE=<base>     # reproduce CI's affected gates locally (CI failure, gate changes)
make agent-env              # this worktree's runtime addresses
make dev-be                 # isolated backend with Air
make dev-fe                 # isolated Vite frontend pointed at that backend
```

Go 1.27+, the Rust toolchain pinned by `rust-toolchain.toml`, Node 22.x (22.5 minimum), pnpm 11.13.1,
GNU Make 4.x, and Docker are required. ffmpeg/ffprobe are required for playout tests. Lint tools and
Air run at pinned versions from the harness.

## Generated files

- `api/openapi.yaml` → `make openapi`; verify with `make openapi-verify`
- `docs/reference/settings.md` → `make config-docs`
- `internal/platform/settings/keys_gen.go` → `make settings-keys` after adding, renaming or retyping a setting
- `docs/reference/make.md` → `make dev-docs`
- `docs/design/package-map.md` and `docs/design/areas/` → `make arch-docs`
- `web/packages/design-system/generated/` → `make fe-tokens`
- `web/packages/api/generated/` and `web/apps/web/src/routeTree.gen.ts` → `make fe-codegen`; both are
  gitignored and absent from a fresh worktree
- applied migrations in `internal/platform/store/migrations/` are immutable; add the next migration

## Repository map

- `cmd/loomarr` is the entrypoint; `internal/app` is the composition root; `internal/api` owns Huma
  routes.
- Domain packages under `internal/` map to the ports documented in design §2.
- `internal/filler/structure` is the provider-neutral complete-timeline reducer shared by filler
  certification and production; challenge truth and provider clients do not belong there.
- `internal/testkit` is the shared mock and pinned-fixture module.
- `web/` is a pnpm workspace: `apps/{web,mobile,tv}` plus the packages under `web/packages/` (shared
  `api`, `core`, `design-system`, `ui`, `ui-tv`, `player`, `fixtures`, `paired-client`, and the native
  modules). Frontend request types come from orval and are never handwritten.

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
work. `make agent-gc` is the canonical cross-worktree audit; its explicit `APPLY=1` mode may remove
only worktrees whose exact head belongs to a merged PR on current `origin/main`. Active, dependent,
dirty, credential-bearing, divergent, open, and ambiguous worktrees fail closed.
For retained worktrees, record an owner, reason, and next review trigger on the tracking issue.
Preservation is not a retirement plan; closing a pane or completing a goal does not clean a worktree.
Retiring a worktree includes stopping every process still working in it: `make agent-reap
WORKTREE=<path>` lists them, `APPLY=1` stops them by PID, and `make agent-reap ORPHANS=1` finds any
left in deleted worktrees.

Supervisors and long-running agents arm the dev watchers (`make dev-watch`) for the whole session and
act on what they report; the watchers only observe. See
[`docs/contributing/dev-watchers.md`](docs/contributing/dev-watchers.md).

Orca creates worktrees itself, under its own workspace directory. The checked-in `orca.yaml` runs the
same registration and bootstrap from Orca's setup hook, so an Orca worktree is registered without
claims: run `make agent-start TASK=<branch> CLAIMS=...` before touching a shared output. See
[`docs/agents/harness.md`](docs/agents/harness.md#orca).

Develop against the URL printed by `make dev-fe`; the backend URL serves the last embedded SPA build and
can look stale by design. A bare `go run ./cmd/loomarr` can orphan a stale child; use `make dev-be`.

## Hard rules

- Tests assert on behaviour, never on the text of Go or TypeScript source. A test that reads a source
  file and matches it with a regexp, `strings.Contains` or `toContain` passes while the behaviour is
  broken and fails on a harmless refactor. Exercise the code; where the intent is structural, parse
  it (`go/parser`, a JS parser) and check the syntax tree. Generated files, workflows, Makefiles,
  JSON and docs are artefacts, not source. `internal/guard_test.go` and
  `web/scripts/source-text-tests.mjs` enforce this with a shrinking allowlist.
- Auth changes include the design §19 negatives: member 403s and sessions dying on disable.
- Retiring a capability adds its identifier to `scripts/check-retired.sh` in the same PR.
- Adding a setting changes design §15 first.
- Adding a build tag changes the guarded `TAGS` list in the Makefile.
- Adding a CI build input changes the per-job filter in `.github/workflows/ci.yml`; never add a
  workflow-level `paths:` filter.
- Pull requests own fast affected feedback. In the merge queue, `merge-gate.yml` reports the required
  `CI` check in minutes, and ci.yml's complete affected gate set runs beside it as `CI assurance`,
  which finishes after the merge and files an issue when a merged commit fails. Revert first: anyone
  may revert that commit, and the fix lands as a new PR. Rerun a flake or outage. Normal queue-produced
  pushes to `main` publish only and must not restore a third product validation run. Keep `CI`
  required for both PR and `merge_group` commits.
- CodeQL uses checked-in advanced setup. Trusted PR/main diffs select only changed languages;
  scheduled/manual runs, unknown paths, and unusable bases scan all six. Keep GitHub default setup
  disabled because it overrides advanced workflows and restores all-language scans on every change.
- Frontend work uses the Vite server, not the stale SPA embedded on the backend port.
- Read a database only through a copy (`make db-view`, or a `VACUUM INTO` snapshot), and never write
  to one directly. To create state, drive the app's API or a test fixture.
- Change source, test, config and doc files through the harness's file-edit tool, one reviewed
  change at a time. Never write them with `sed -i`, `perl -pi`, `awk`, a `python` script, or a shell
  heredoc or redirect. That includes conflict resolution and one-line tweaks. A scripted replace
  fails silently and has mangled Go escapes, merged comments and broken JSX here. An edit tool
  fails loudly on a bad match. For a repetitive rename, use the edit tool's replace-all or ask
  first. Exempt: generators and formatters that a Make target owns (`make fe-codegen`, `make
  openapi`, `biome --write`, `gofmt`), and reading files with any tool.
- Write code comments in the present tense, about the code as it stands. Say what it does and why,
  and state a trap as a fact about today: "the server drops an unknown field silently", not "this
  once said `maxAcquire` and the cap vanished". The story belongs in the commit and the issue, which
  a comment may cite by number. Never narrate a change ("no longer", "used to", "now", "moved
  from"): a comment that describes the past goes stale the day it is written.

## Stop points

Stop and ask the maintainer for a Phase-0 contract deviation, an authorization/safety change, a gate
that appears to require weakening, or a new authority beyond the requested task. Preserve unrelated
dirty files and worktrees.

## Agent adapters and skills

Durable workflows live in `.agents/workflows/`; installed skill bodies live in `.agents/skills/`.
Agent-specific directories such as `.claude/` contain adapters or symlinks only. A required workflow
must be usable without a proprietary slash command, home-directory plan, or product-specific worktree
feature.

For dependent branches, record `DEPENDS_ON` and stack with `BASE=<dependency-branch>`. See
`docs/agents/harness.md` and the curated catalog in `docs/agents/skills.md`.
