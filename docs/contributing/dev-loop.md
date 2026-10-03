# The dev loop

**For:** contributors running Loomarr from source.
**You'll get:** a backend and frontend that reload as you edit, and which URL to use.

Two processes, each with live reload. Run them in separate terminals; the harness assigns stable,
worktree-specific ports and prints both URLs.

```bash
make dev-be    # backend, rebuilds the Go server and required Rust image worker
make dev-fe    # Vite HMR, proxying this worktree's backend
```

## Develop against the frontend URL

Your browser talks to Vite, which serves the frontend you're editing and proxies API calls to the
Air-managed backend.

The backend URL serves the SPA compiled into the binary at your last `make fe`, not your working copy.
Frontend changes appear only at the frontend URL.

Vite proxies `/v1`, `/hooks`, `/docs`, `/openapi.*`, `/healthz`, `/readyz` and `/metrics` to this
worktree's backend. Point it elsewhere with `LOOMARR_API=http://otherbox:8080`.

## Don't use `go run ./cmd/loomarr`

It doesn't reload, and because `go run` supervises rather than execs, closing the terminal can
leave an orphan serving old code with no sign anything is stale.

If an API change isn't showing up:

```bash
eval "$(./scripts/dev-env.sh export)"
curl -s "localhost:$LOOMARR_DEV_PORT/v1/system/version"
```

That reports the commit the running binary was built from.

`make dev-be` prevents this. It refuses to start a second instance, and a watchdog detects "Air
alive but not rebuilding" by comparing the binary's mtime against the newest watched Go/Rust input.
`DEV_BE_REPLACE=1` replaces a running instance; `DEV_BE_NO_WATCHDOG=1` skips the watchdog.

Process ownership includes the worktree cwd. Replacement never kills another worktree's Air or
backend, even though their process names are identical.

## Two Air settings that must stay

- **`stop_on_error = false`** — with `true`, Air stops watching after a failed build, so a
  mid-refactor compile error wedges it while it keeps serving the old binary.
- **`poll = true`** — inotify is unreliable on btrfs and with atomic-save editors.

It also sources `.env` rather than inlining variables, so an inlined `DATABASE_URL` can't
silently point at a different database.

## `make dev` is not the app

It starts external dependencies only — a Tunarr container and the filler drop folder. Use it
when working on the Tunarr backend. `make dev-gpu` adds the NVIDIA overlay.

## A dev store

```bash
make seed
```

Populates a store through the real domain paths, honoring the approval gate.

## Agent lanes never publish or retire a Live TV tuner

`scripts/dev-env.sh export` sets `LOOMARR_AGENT_DISABLE_LIVETV_TUNER=1` for every secondary
worktree (an agent lane), unset for the primary worktree. While set, the settings-save Live TV
transition, the channel-maintenance repair publisher, and the `livetv-reconnect` endpoint all skip
every `AddTuner`/`RemoveTuner`/listing-provider call and log one line instead — settings still
save. This is deliberately NOT a declared setting (`internal/settings`): a lane's `library.url`/
`library.token` can drift onto the household media server, and this must hold regardless of what
the Settings UI saves (#1555). `scripts/dev-env.sh show` reports the current state as "livetv
tuner publishing". Override with `LOOMARR_AGENT_DISABLE_LIVETV_TUNER=` (empty) only to reproduce
production behaviour in a lane — never point it at a real household media server.
