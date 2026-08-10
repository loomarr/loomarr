# Setup

## Toolchain

| Tool | Version | Pinned in |
| --- | --- | --- |
| Go | **1.26+** | `go.mod` |
| Node | **22.5+** | `web/package.json` → `engines.node` |
| pnpm | 11.13.1, via `corepack enable` | `web/package.json` → `packageManager` |
| ffmpeg + ffprobe | on `PATH` | — |
| Docker | for Postgres and Playwright suites | — |

**Node 22.5 is a hard floor, not a preference.** pnpm 11.13 uses the built-in `node:sqlite`
for its store index; anything older fails with `ERR_UNKNOWN_BUILTIN_MODULE`.

Everything else — golangci-lint, actionlint, Air — runs through `go run` at a pinned version,
so there is nothing to install globally.

**ffmpeg** is needed because internal playout is the default backend: without it, channels
appear in the guide and fail at tune time. `make check` stays hermetic and needs neither binary;
only `make test-ffmpeg` executes them.

```bash
brew install ffmpeg          # macOS
sudo apt install ffmpeg      # Debian/Ubuntu
sudo pacman -S ffmpeg        # Arch
```

Builds differ in what they carry, so Loomarr probes rather than assuming — a build without
`drawtext` (Homebrew's, for one) renders an unlabelled card instead of killing the channel.

## From a clean clone

```bash
git clone https://github.com/mantonx/loomarr && cd loomarr
make check          # the gate: fmt + vet + vet-tags + lint + unit tests
make fe-install     # pnpm install — nothing else does this
make fe             # biome + codegen + typecheck + unit tests + SPA + storybook
```

> ⚠ **`make fe-install` is not implied by `make fe`.** On a fresh clone, running `make fe`
> first fails on missing dependencies. This step used to be absent from the documented path,
> which meant the documented path did not work.

The Go half needs no setup beyond the module cache. The frontend half needs both `fe-install`
*and* codegen: `web/packages/api/generated/` is gitignored, so every `@loomarr/api` import
fails to resolve until `make fe-codegen` (or `make fe`) has run once.

## Working in a worktree

For parallel sessions, use a **sibling** directory — never one inside the repo root, because
the Playwright targets bind-mount the root and would mount the worktree into every visual run.

```bash
git worktree add ../loomarr-<topic> -b <topic>
cd ../loomarr-<topic>/web
npx pnpm@11.13.1 install --frozen-lockfile   # fast; the pnpm store hard-links
npx pnpm@11.13.1 codegen                     # REQUIRED — generated client is gitignored
```

Go needs nothing: the module cache is shared and `go build ./...` works immediately. Skip the
`web/` steps entirely if your change doesn't touch the frontend.

Two topics are only safe to run in parallel if they touch **different generated output**.
Anything that adds an endpoint edits `api/openapi.yaml` and regenerates the orval client;
anything that shares a DTO or regenerates the same visual baselines will conflict in files
nobody wrote, which is miserable to merge.

`git worktree remove ../loomarr-<topic>` when it merges.
