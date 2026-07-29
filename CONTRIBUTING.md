# Contributing to Loomarr

Thanks for your interest! Loomarr is a self-hosted app for turning a
natural-language channel intent into a live, self-maintaining Tunarr channel.
This guide covers how to get set up and the conventions that keep the tree
green.

## Ground rules (the ones that matter most)

- **`docs/design.md` is the single source of truth.** If a change needs to
  deviate from it, update the doc in the *same* PR, *before* the code. Doc and
  code must never disagree silently.
- **Gates are hard.** `make check` (and CI) must be green. Never weaken, skip,
  or stub a test to make a gate pass — if a gate can't pass, the design or the
  code is wrong; fix one of them.
- **No new dependencies** without a one-line rationale added to `docs/design.md`
  §14 in the same PR.
- **Generated files are never hand-edited**: `api/openapi.yaml` (`make openapi`),
  the orval FE client, goose-applied migrations. Migrations are **forward-only** —
  add a new one, never edit an applied one.
- **All application code is Go.** The only non-Go pieces are the embedded
  frontend (compiles to static assets) and the vendored
  `yt-dlp`/`ffmpeg`/`ffprobe`/`deno` binaries the image invokes via `exec`.

## Prerequisites

- Go **1.26+**
- Node **22.5+** and `pnpm` (via `corepack enable`) — 22.5 is a hard floor, not a
  preference: pnpm 11.13 uses the built-in `node:sqlite` for its store index, and
  anything older fails with `ERR_UNKNOWN_BUILTIN_MODULE`
- **`ffmpeg` and `ffprobe` on PATH** — internal playout is the default backend
  (§9.1), so without them channels appear in the guide and fail at tune time. The
  image ships its own; a host run needs them installed:
  - macOS: `brew install ffmpeg`
  - Debian/Ubuntu: `sudo apt install ffmpeg`
  - Arch: `sudo pacman -S ffmpeg`

  Only `make test-ffmpeg` executes them from the test suite; `make check` stays
  hermetic and needs neither. **Builds differ in what they carry** — Loomarr probes
  the binary rather than assuming, so a build without `drawtext` (Homebrew's, for
  one) renders an unlabelled card instead of killing the channel.
- **Docker** — required for the Postgres conformance suite and the Playwright
  visual/e2e suites (from Phase 4 onward)

## Getting started

```bash
git clone https://github.com/mantonx/loomarr && cd loomarr
make check          # the default gate: fmt + vet + lint + unit tests
make build          # static binary → bin/loomarr

# Frontend
make fe-install     # pnpm install (web/)
make fe             # biome + codegen + typecheck + unit tests + SPA + storybook
```

### Running the app while you work

Copy `.env.example` to `.env` first, then run both halves with live reload:

```bash
make dev-be                        # backend  :8080 — Air rebuilds on any Go change
pnpm --filter @loomarr/web dev     # frontend :5173 — Vite HMR (from web/)
```

Develop against **:5173**, which proxies `/v1` to :8080.

⚠ Do not start the backend with a bare `go run ./cmd/loomarr`: it does not reload, and
because `go run` supervises rather than execs, closing the terminal can leave an orphan
serving pre-change code indefinitely. `GET /v1/system/version` reports the commit a running
binary was built from — the fastest way to confirm you are not talking to a stale process.

## Before you open a PR

Run the gates your change touches:

| Change | Run |
| --- | --- |
| Go code | `make check` |
| API surface | `make openapi-verify` (regenerate with `make openapi`) |
| Settings/config | `make config-docs-verify` |
| Store (SQLite/Postgres) | `make test-pg` (needs Docker) |
| Frontend | `make fe` and, for UI, `make fe-visual` + `make e2e` |

CI runs all of these; a PR that's green locally should be green in CI.

## Conventions

- **Commits:** conventional-commit style (`feat(api): …`, `fix(store): …`,
  `docs: …`) with a body explaining the *why*.
- **Frontend:** folder-per-module, arrow functions, barrel exports, a story and
  a unit test per component. Request types are generated (orval) — never
  hand-written.
- **Tests never touch the network.** External services are mocked through
  `internal/testkit`; extend it rather than inventing a private mock.

## Reporting bugs / proposing features

Open an issue using the templates. For anything security-sensitive, follow
[`SECURITY.md`](SECURITY.md) instead of filing a public issue.

By contributing, you agree that your contributions are licensed under the
project's [MIT License](LICENSE).
