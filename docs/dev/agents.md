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

## Supervise a task

Use [the supervisor workflow](../../.agents/workflows/supervise.md) when one agent should coordinate
several bounded workers. It defines the task graph, worker brief, evidence report, steering loop, and
integration handoff. The delivery owner remains accountable for the combined diff, final gates, PR,
and cleanup; a worker reporting `complete` closes only its assigned outcome.

Native subagents are the strongest arrangement because the parent can inspect, steer, wait for, and
collect its children directly. Independent agent sessions can still participate through the shared
registry and isolated worktrees, but their conversations are not visible across harnesses. Treat
their branches, diffs, commits, command results, and structured reports as evidence; do not imply the
supervisor can read or control an unrelated session.

### Roles, capability, and reasoning

Assign roles per task rather than making them permanent agent personas. Useful roles include a
bounded investigator, implementer, adversarial reviewer, or integrator, but each role ends with the
worker report. A worker waits for the supervisor to stop it or issue another brief; it does not grow
its own backlog. This keeps ownership with the delivery agent while still providing fresh context.

When the harness supports model and reasoning controls, select them by task shape and record the
choice in the worker brief:

| Task shape | Default execution |
| --- | --- |
| Small task, sequential reasoning, or shared mutable seam | One owning agent; no delegation |
| Bounded search, triage, or repetitive mechanical work | Faster/lower-cost model or effort when acceptance is objective |
| Ambiguous multi-step design or implementation | Strong-capability model with enough reasoning for uncertainty |
| Authorization, safety, integration, or final acceptance | Strong-capability model and high scrutiny |
| External session without trustworthy controls | Record `uncontrolled`; verify through artifacts and evidence |

Inheritance is the default. Override it only when the expected quality, latency, or cost difference
is material, and compare alternatives only when they pass the same acceptance gate. Never treat a
stronger model as broader authority. Do not switch a worker's model during an active checkpoint;
make the change with its next bounded assignment. During crash recovery, preserve the original model
and thread when possible so the checkpoint remains coherent.

These rules follow OpenAI's guidance that subagents work best on independent, bounded tasks and that
model and reasoning settings should be task-dependent. See [Codex subagents](https://learn.chatgpt.com/docs/agent-configuration/subagents)
and [multi-agent workflows](https://developers.openai.com/api/docs/guides/responses-multi-agent).

### tmux

`tmux` is a useful operator interface for independent sessions, not an orchestration protocol. Keep
the supervisor in one pane and give every editing worker its own registered worktree and pane. A pane
is not the worker's identity; its unique task name, branch, worktree, and claims are.

Create worktrees through the harness before starting agent processes, then arrange panes with exact
paths. For example:

```sh
make agent-worktree TOPIC=worker-a CLAIMS=<owned-seam>
make agent-worktree TOPIC=worker-b CLAIMS=<different-seam>

tmux new-session -d -s loomarr-supervisor -c /path/to/owning-worktree
tmux new-window -t loomarr-supervisor -n worker-a -c /path/to/loomarr-worker-a
tmux new-window -t loomarr-supervisor -n worker-b -c /path/to/loomarr-worker-b
tmux attach -t loomarr-supervisor
```

Start the chosen agent interactively in each pane and provide the workflow's worker brief. Use
`make agent-status` plus the worker report for coordination. Do not treat `tmux capture-pane` output
as completion evidence or use blind `send-keys` automation as a substitute for an acknowledged
handoff; prompts, approval overlays, and terminal state make that brittle. Use native subagents when
live programmatic steering is required. When recovering a crashed session, launch or resume it from
the owning worktree and verify that both the worktree and its Git metadata are writable before it
continues an in-progress merge or edit.

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
always run the gate and never populate the cache. The harness rechecks the commit and tracked-file
state after the gate and refuses to cache if implementation began while the baseline was running;
mixed-tree output is not evidence for either version.

Run small affected tests while editing, formatting and `git diff --check` before commit, then one
stabilized complete gate for every touched area. CI owns expensive native and platform matrices.
Never run `make smoke*` from an agent session; those commands drive the maintainer's live stack.

## Finish and clean up

After the PR is merged and its required evidence is complete, release its claims and audit the
machine:

```sh
make agent-stop
make agent-gc
```

`make agent-gc` is read-only by default. It classifies every registered worktree and explains why
each one is protected or eligible. After reviewing that inventory, one explicit command retires
every eligible entry:

```sh
make agent-gc APPLY=1
```

Eligibility is deliberately strict. The worktree must be secondary, unregistered, unlocked, clean
including untracked files, free of a copied `.env`, and still at the exact head of a merged GitHub
PR whose merge commit is present on current `origin/main`. The collector matches PR head OIDs rather
than relying on `git branch --merged`, which cannot recognize squash-merged branch heads. Active,
dependent, running, dirty, credential-bearing, divergent, open, closed-unmerged, detached, and
ambiguous worktrees are reported and retained. Ignored bootstrap/runtime directories are removed
only after the worktree meets every eligibility rule and the maintainer supplies `APPLY=1`.

The audit requires an authenticated GitHub CLI because squash-merge evidence is not recoverable from
local branch ancestry. If GitHub or `origin/main` cannot be verified, the command exits without
removing anything.

Creating another worktree fails when the secondary-worktree count has reached 16. That is a backlog
tripwire, not a concurrency target: run the audit and resolve its findings first. A maintainer may
set `ALLOW_WORKTREE_BACKLOG=1` for an intentional exception.

`make doctor` reports toolchain drift, worktrees, addresses, caches, and misplaced artifacts. It
does not delete anything; `make agent-gc` owns worktree classification and retirement.

## Skills and durable workflows

The curated skill set and when to use it are documented in [Skills](skills.md). Durable audit and
review procedures live in `.agents/workflows/`; adapters may expose them as slash commands, but the
Markdown files remain the cross-harness authority.
