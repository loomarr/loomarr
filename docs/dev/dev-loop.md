# The dev loop

Two processes, each with live reload. Run them in separate terminals.

```bash
cp .env.example .env    # Air sources it

make dev-be                                # backend :8080, rebuilds on any Go change
cd web && pnpm --filter @loomarr/web dev   # frontend :5173, Vite HMR
```

## Develop against :5173

```mermaid
graph LR
  B["<b>Your browser</b><br/>localhost:5173"]
  V["<b>Vite dev server</b> :5173<br/><i>your working copy</i>"]
  A["<b>Air</b> :8080<br/><i>rebuilds on .go change</i>"]
  E["embedded SPA<br/><i>from last</i> <code>make fe</code>"]

  B --> V
  V -->|"proxies /v1, /docs, /metrics…"| A
  A -.->|"also served at :8080"| E

  classDef good fill:#1f6f4a,stroke:#134a31,color:#fff
  classDef warn fill:#7a2d2d,stroke:#4a1b1b,color:#fff
  classDef norm fill:#2b3b52,stroke:#1b2736,color:#dbe4ef
  class V good
  class E warn
  class B,A norm
```

**:8080 serves the SPA compiled into the binary at your last `make fe`, not your working copy.**
Frontend changes won't appear there.

Vite proxies `/v1`, `/hooks`, `/docs`, `/openapi.*`, `/healthz`, `/readyz` and `/metrics` to
:8080. Point it elsewhere with `LOOMARR_API=http://otherbox:8080`.

## Don't use `go run ./cmd/loomarr`

It doesn't reload, and because `go run` supervises rather than execs, closing the terminal can
leave an orphan serving old code with no sign anything is stale.

If an API change isn't showing up:

```bash
curl -s localhost:8080/v1/system/version
```

That reports the commit the running binary was built from.

`make dev-be` prevents this. It refuses to start a second instance, and a watchdog detects "Air
alive but not rebuilding" by comparing the binary's mtime against the newest `.go` file.
`DEV_BE_REPLACE=1` replaces a running instance; `DEV_BE_NO_WATCHDOG=1` skips the watchdog.

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

Populates a store through the real domain paths, honouring the approval gate.
