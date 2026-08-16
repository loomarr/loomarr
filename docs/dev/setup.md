# Setup

## Toolchain

| Tool | Version | Pinned in |
| --- | --- | --- |
| Go | 1.26+ | `go.mod` |
| Rust | 1.93.x | `rust-toolchain.toml` |
| Node | 22.x (22.5 minimum) | `.node-version` + `web/package.json` → `engines.node` |
| pnpm | 11.13.1, via `corepack enable` | `web/package.json` → `packageManager` |
| ffmpeg + ffprobe | on `PATH` | — |
| Docker | for Postgres and browser suites | — |
| shellcheck | on `PATH` | `make check` validates every `scripts/*.sh` file |

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
make doctor         # exact toolchain + local-state diagnostics
make bootstrap      # pnpm install + codegen
make agent-baseline # make check, shared by clean worktrees at this commit
```

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
