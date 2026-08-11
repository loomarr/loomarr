# Setup

## Toolchain

| Tool | Version | Pinned in |
| --- | --- | --- |
| Go | 1.26+ | `go.mod` |
| Node | 22.5+ | `web/package.json` → `engines.node` |
| pnpm | 11.13.1, via `corepack enable` | `web/package.json` → `packageManager` |
| ffmpeg + ffprobe | on `PATH` | — |
| Docker | for Postgres and browser suites | — |

Node 22.5 is a hard floor: pnpm 11.13 uses the built-in `node:sqlite`, and older versions fail
with `ERR_UNKNOWN_BUILTIN_MODULE`.

Everything else — golangci-lint, actionlint, Air — runs via `go run` at a pinned version, so
there's nothing to install globally.

ffmpeg is needed because Loomarr streams its own channels by default. `make check` stays
hermetic and needs neither binary; only `make test-ffmpeg` runs them.

```bash
brew install ffmpeg          # macOS
sudo apt install ffmpeg      # Debian/Ubuntu
sudo pacman -S ffmpeg        # Arch
```

## From a clean clone

```bash
git clone https://github.com/mantonx/loomarr && cd loomarr
make check          # the gate — run before every push
make fe-install     # pnpm install
make fe             # the frontend gate
```

What each target does is in the [command reference](commands.md), generated from the Makefile.

`make fe-install` isn't implied by `make fe` — on a fresh clone, running `make fe` first fails
on missing dependencies.

The frontend also needs codegen: `web/packages/api/generated/` is gitignored, so `@loomarr/api`
imports don't resolve until `make fe-codegen` (or `make fe`) has run once.

## Worktrees

Use a sibling directory, not one inside the repo root — the Playwright targets bind-mount the
root.

```bash
git worktree add ../loomarr-<topic> -b <topic>
cd ../loomarr-<topic>/web
npx pnpm@11.13.1 install --frozen-lockfile
npx pnpm@11.13.1 codegen                     # generated client is gitignored
```

Go needs nothing — the module cache is shared. Skip the `web/` steps if your change doesn't
touch the frontend.

Two topics are only safe in parallel if they touch **different generated output**. Anything
adding an endpoint edits `api/openapi.yaml` and regenerates the orval client; two branches doing
that conflict in files nobody wrote.

`git worktree remove ../loomarr-<topic>` when it merges.
