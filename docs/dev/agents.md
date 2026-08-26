# Agent development

Loomarr's harness is agent-agnostic. Codex, Claude Code, terminal-driven agents, and humans use
the same Make targets and the same registry under Git's common directory.

## One owner, selective delegation

One task worktree owns each deliverable from first edit through merge. That owner may delegate
independent reading, competing designs, or a fresh-context review, but delegated agents return
findings to the owner by default. They do not open a second implementation branch for the same
deliverable.

Use another editing agent only when the work has a real merge seam:

| Situation | Use another agent? | Shape |
| --- | --- | --- |
| Search, research, or independent review | Yes | Read-only; report to the owning worktree |
| Two alternative interface designs | Yes | Independent proposals; the owner chooses and implements |
| Disjoint product slices | Yes | Separate worktrees, claims, tests, and PRs |
| Same DTO, generated output, migration number, or visual baseline | No | One owner; delegate read-only analysis |
| One change depends on another unmerged branch | Sequential or stacked | Record `DEPENDS_ON` and create from the dependency branch |
| One implementation split across several agents | Usually no | Coordination cost and partial ownership outweigh parallelism |

Claims prevent known collisions; they do not make overlapping implementations safe. Before
delegating edits, identify the file boundary, interface boundary, delivery owner, and merge order.
If any of those is unclear, keep one editing agent.

## Start a task

Create, register, claim, and bootstrap a fresh sibling worktree in one command:

```sh
make agent-status
make agent-worktree TOPIC=filler-refresh CLAIMS=openapi-client
cd ../loomarr-filler-refresh
make agent-baseline
```

Registration happens before bootstrap, closing the old gap where a worktree existed and generated
files before it appeared in the roster. If the worktree already exists, register from inside it:

```sh
make agent-start TASK=filler-refresh CLAIMS=openapi-client
make agent-baseline
```

During implementation, `make agent-verify BASE=origin/main` is a focused inner-loop check. It
reports the changed-file scope and uses the fail-closed CI classifier. It is explicitly not a final
gate. Run the complete gates for the touched areas once the change stabilizes.

## Dependent work

Do not start two dependent branches independently from `main`; both agents will edit assumptions
the other does not contain. Stack the dependent work and make that edge visible:

```sh
make agent-worktree \
  TOPIC=channel-ui \
  BASE=channel-api \
  DEPENDS_ON=channel-api \
  CLAIMS=openapi-client
```

The harness rejects a dependency that is not active and a branch that is not based on the active
dependency branch. `make agent-status` shows the dependency and remaining lease so the merge order
is visible across harnesses.

Prefer waiting for the first PR to merge when the second task is small or rebase-sensitive. Use a
stack only when the saved wall time is worth carrying the dependency through review and rebase.

## Claims

A claim names a shared output or interface that cannot be merged safely after two agents edit it
independently.

| Claim | Covers |
| --- | --- |
| `openapi-client` | Huma definitions, `api/openapi.yaml`, orval output, shared DTOs |
| `visual-baselines` | Storybook snapshots |
| `e2e-baselines` | Full-page snapshots |
| `tokens` | Generated design tokens |
| `migrations` | The next forward-only migration number |
| `agent-contract` | `AGENTS.md`, adapters, agent workflows, and skills |
| `dev-runtime` | Make targets, local ports, Air, Compose, and the harness |

Add a domain-specific claim when two tasks would edit the same interface even if the files differ.
Keep claims narrow: claiming `*` or an entire broad domain makes safe work wait and hides the actual
seam. Duplicate active task names are rejected because they make ownership ambiguous.

Claims expire after four hours by default. Use `make agent-renew` for work that is still active and
`make agent-prune` for expired entries. A dead registry lock is reclaimed only after its owner is
gone and the lock is old enough that no live writer can be between lock creation and owner
publication.

## Worktree isolation

`make agent-worktree` branches from freshly fetched `origin/main` unless `BASE` is explicit. It runs
the pinned frontend install, code generation, Rust build, and isolated developer bootstrap.
Credentials are not copied unless `COPY_ENV=1` is explicitly supplied.

Every secondary worktree receives deterministic, distinct values for:

- backend, Vite, Storybook, and Tunarr ports;
- Compose project and volumes;
- SQLite database, filler drop, prepared-media, and artifact directories;
- the public URL used by internal Playout; and
- an isolated automatic developer login.

`make agent-env` prints those values. `make dev-be`, `make dev-fe`, `make storybook`, `make dev`, and
`make dev-gpu` consume them. Vite uses `strictPort`; a collision fails at the advertised address
instead of silently moving.

Air and its watchdog match processes by command and worktree directory. `DEV_BE_REPLACE=1` can
replace only this worktree's processes. A listener owned by another worktree is reported and left
alone.

## Baselines and gates

`agent-baseline` caches a successful `make check` by clean commit, Go and Rust toolchains, operating
system, and architecture. Worktrees at the same commit wait for one proof and reuse it. Dirty trees
always run the gate and never populate the cache.

Run small affected tests while editing, formatting and `git diff --check` before commit, then one
stabilized complete gate for every touched area. CI owns expensive native and platform matrices.
Never run `make smoke*` from an agent session; those commands drive the maintainer's live stack.

## Finish and clean up

After the PR is published and its required evidence is complete:

```sh
make agent-stop
```

Then inspect `make agent-status`, `git worktree list`, and `git status` in the task worktree. Remove
only a clean, unused worktree. Never remove a worktree with tracked or untracked work, and never
remove another agent's worktree merely because its registry lease expired.

`make doctor` reports toolchain drift, worktrees, addresses, caches, and misplaced artifacts. It
does not delete anything.

## Skills and durable workflows

The curated skill set and when to use it are documented in [Skills](skills.md). Durable audit and
review procedures live in `.agents/workflows/`; adapters may expose them as slash commands, but the
Markdown files remain the cross-harness authority.
