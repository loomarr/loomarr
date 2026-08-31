# Setup

## Toolchain

| Tool | Version | Pinned in |
| --- | --- | --- |
| Go | 1.27+ | `go.mod` |
| Rust | 1.93.x | `rust-toolchain.toml` |
| Node | 22.x (22.5 minimum) | `.node-version` + `web/package.json` → `engines.node` |
| pnpm | 11.13.1, via Corepack from the pinned Node installation | `web/package.json` → `packageManager` |
| GNU Make | 4.x | Make parser policy and the repository harness |
| ffmpeg + ffprobe | on `PATH` | — |
| Docker | for Postgres and browser suites | — |
| shellcheck | on `PATH` | comprehensive verification validates every `scripts/*.sh` file |

Node 22.5 is a hard floor: pnpm 11.13 uses the built-in `node:sqlite`, and older versions fail
with `ERR_UNKNOWN_BUILTIN_MODULE`.

Go 1.27's native macOS toolchain requires macOS 13 or newer. Older macOS releases can still build
the Linux container through Docker, but they cannot run the supported native contributor toolchain.

Everything else — golangci-lint, actionlint, Air — runs via `go run` at a pinned version, so
there's nothing to install globally.

## macOS with Homebrew

Homebrew's current `node` formula may be newer than Loomarr's CI version. Install the versioned
formula and put its keg first on `PATH`; leaving an unversioned Node keg installed is harmless.
Rustup is also keg-only, so it needs the same treatment.

```bash
brew install go node@22 rustup make ffmpeg shellcheck
brew install --cask docker-desktop

printf '%s\n' 'export PATH="$(brew --prefix make)/libexec/gnubin:$(brew --prefix node@22)/bin:$(brew --prefix rustup)/bin:$PATH"' >> ~/.zprofile
source ~/.zprofile

corepack enable
corepack prepare pnpm@11.13.1 --activate
open -a Docker
```

Run `make doctor` after Docker Desktop reports that its engine is running. The first Rust command
in this repository installs the toolchain and components pinned by `rust-toolchain.toml`; the first
Go, Rust and frontend gates also populate their dependency caches.

ffmpeg is needed because Loomarr streams its own channels by default. Comprehensive verification stays
hermetic and needs neither binary; only `make test-ffmpeg` runs them.

```bash
brew install ffmpeg          # macOS
sudo apt install ffmpeg      # Debian/Ubuntu
sudo pacman -S ffmpeg        # Arch
```

## From a clean clone

```bash
git clone https://github.com/loomarr/loomarr && cd loomarr
make doctor    # exact toolchain + local-state diagnostics
make bootstrap # pnpm install + codegen
```

Setup prepares the workspace; it does not audit the entire repository. Use focused tests for the
area you change and `make verify BASE=origin/main` before pushing. `make agent-baseline` remains an
opt-in, shared `make verify SCOPE=all` proof when a complete audit is deliberately required.

What each target does is in the [command reference](commands.md), generated from the Makefile.

`make bootstrap` is idempotent. It builds the required Rust image worker and closes the fresh-clone
gap where `make fe` fails first on missing dependencies and then on missing generated files.

The frontend also needs codegen: `web/packages/api/generated/` is gitignored, so `@loomarr/api`
imports don't resolve until `make fe-codegen` (or `make fe`) has run once.

## Worktrees

```bash
make agent-worktree TOPIC=<topic>
```

The harness creates a sibling worktree, installs the web workspace, runs codegen, and assigns isolated
runtime ports, Compose state, SQLite storage, prepared-publication storage, and an artifact directory.
It provisions a `developer` admin in that isolated SQLite database, marks setup complete, and enables
automatic dev login so the advertised Vite URL opens ready to use. Set `AGENT_DEV_IDENTITY=0` when a
worktree must exercise a genuine first run. `BOOTSTRAP_SKIP_FE=1` skips the web preparation for a known
Go-only task.

Credentials are not copied. `COPY_ENV=1` is an explicit opt-in for integration work; runtime and
database isolation still override the copied values.

Register shared outputs with `make agent-start`; see [Agent development](agents.md). Never remove a
worktree containing uncommitted or untracked work.
