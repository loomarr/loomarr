# Agent development

Loomarr's local harness is agent-agnostic. Codex, Claude Code, a terminal-driven agent, and a human all
use the same Make targets and the same registry stored under Git's common directory.

## Lifecycle

```sh
make agent-status
make agent-start TASK=filler-refresh CLAIMS=openapi-client,visual-baselines
make agent-baseline

# edit and iterate
make agent-verify BASE=origin/main

# run the complete gates for the touched areas, then
make agent-stop
```

`agent-baseline` caches a successful `make check` by clean commit, Go toolchain, operating system, and
architecture. Worktrees at the same commit wait for one proof and reuse it. Dirty trees always run the
gate and never populate the cache.

`agent-verify` is an inner-loop aid, not a gate. It reports the changed-file scope, runs harness and
format guards, tests directly changed Go packages, and checks frontend codegen/lint/types when `web/`
moved. Its first output line deliberately says that `make check` remains required.

## Claims

The registry lives in the common Git directory, so every worktree sees it. A claim names a shared
output or interface that cannot be merged safely after two agents regenerate it independently.

| Claim | Covers |
| --- | --- |
| `openapi-client` | Huma definitions, `api/openapi.yaml`, orval output, shared DTOs |
| `visual-baselines` | Storybook snapshots |
| `e2e-baselines` | Full-page snapshots |
| `tokens` | Generated design tokens |
| `migrations` | The next forward-only migration number |
| `agent-contract` | `AGENTS.md`, adapters, agent workflows |
| `dev-runtime` | Make targets, local ports, Air, Compose |

Claims expire after 12 hours by default so a dead terminal does not block the machine permanently. Set
`AGENT_LEASE_HOURS` when a longer session is intentional. `make agent-renew` extends the current lease
and `make agent-prune` removes expired registry entries without touching any worktree or its files.
`make agent-stop` releases the current worktree immediately.

## Worktrees and bootstrap

```sh
make agent-worktree TOPIC=fix/example
```

The command creates a sibling worktree, runs `pnpm install --frozen-lockfile`, runs codegen, and creates
the ignored `.agent-data/` and `.artifacts/` directories. It also prepares the isolated database with
a `developer` admin and completed setup, then enables automatic dev login. Opening the worktree's Vite
URL therefore lands directly in the app. Both provisioning and login reuse the production domain
paths; no shipped server gains a bootstrap shortcut. Set `AGENT_DEV_IDENTITY=0` when the task is the
first-run wizard itself. `BOOTSTRAP_SKIP_FE=1` is available for a known Go-only task.

The harness does not copy `.env` by default. `COPY_ENV=1` is an explicit opt-in for integration work;
the copy is mode `0600`. Even then, secondary worktrees override the local SQLite path, runtime
ports, and `server.public_url` after sourcing `.env`, preventing two agents from sharing a database
or listener and preventing playout's parent ffmpeg from calling the primary backend accidentally.
The automatic developer exists only in that isolated database; the primary database and its
authentication policy are never changed.

## Runtime isolation

`make agent-env` prints the resolved environment. The primary worktree keeps the familiar ports. Every
secondary worktree derives stable ports and names from its absolute path:

- backend and Vite frontend
- Storybook and Tunarr
- Compose project and volumes
- SQLite database and filler drop folder
- the server public URL used by internal playout
- diagnostic artifact directory

`make dev-be`, `make dev-fe`, `make storybook`, `make dev`, and `make dev-gpu` consume that environment.
Vite uses `strictPort`, so an unexpected collision fails at the advertised address instead of silently
moving to a different port.

Air and its watchdog match processes by both command name and worktree cwd. `DEV_BE_REPLACE=1` can
therefore replace only this worktree's processes; a listener owned by another worktree is reported and
left untouched. CI exercises this ownership contract on both Linux (`/proc`) and macOS (`lsof`).

## Doctor and cleanup

`make doctor` verifies the required toolchain and reports worktrees, per-worktree addresses, the Go
cache size, smoke-artifact size, a secondary worktree parked on `main`, and image artifacts placed in
the repository root. It never deletes anything.

Put screenshots, traces, and generated diagnostics under the path printed by `make agent-env`. Cleanup
remains explicit because databases, smoke data, dirty worktrees, and credentials are not disposable
merely because they are ignored by Git.
