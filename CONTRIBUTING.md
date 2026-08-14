# Contributing to Loomarr

Thanks for your interest! This page covers the conventions and the PR process.

**Setup, the dev loop, testing and CI live in [`docs/dev/`](docs/dev/index.md)** — they used to
be restated here and in three other files, and the copies drifted apart. One home now:

- [Setup](docs/dev/setup.md) — toolchain versions and a clean-clone path that works
- [The dev loop](docs/dev/dev-loop.md) — both halves with live reload
- [Testing](docs/dev/testing.md) — the layers and what each proves
- [CI](docs/dev/ci.md) — jobs, path filters, and what's required
- [Commands](docs/dev/commands.md) — every `make` target (generated)
- [AI in this project](docs/dev/ai.md) — how it's built with coding agents, and what that
  expects of a contribution
- [Agent development](docs/dev/agents.md) — shared sessions, claims, worktrees, and isolated runtimes

## Ground rules

- **`docs/design.md` is the single source of truth.** If a change needs to deviate, update the
  doc in the *same PR, before the code*. Doc and code must never disagree silently.
- **Gates are hard.** `make check` and CI must be green. Never weaken, skip or stub a test to
  make a gate pass — if a gate can't pass, the design or the code is wrong. Fix one of them.
- **New dependencies are fine when they genuinely help** — add a row to `docs/design.md` §14 in
  the same PR with a one-line rationale. Prefer something already in the tree, and a focused
  library over a framework.
- **Generated files are never hand-edited.** See [codegen](docs/dev/codegen.md). Migrations are
  forward-only: add a new one, never edit an applied one.
- **All application code is Go.** The exceptions are the frontend (compiles to embedded static
  assets), the vendored binaries invoked via `exec`, and build tooling that doesn't ship.
- **When a PR retires a capability, add its identifier to `scripts/check-retired.sh`** in the
  same PR. `docs/help/` ships inside the binary and is read as instructions — a retired setting
  still documented sends an operator to configure something nothing reads.

## Before you open a PR

Run the gates your change touches:

| Change | Run |
| --- | --- |
| Go code | `make check` |
| API surface | `make openapi-verify` |
| Settings | `make config-docs-verify` |
| Makefile / workflows | `make dev-docs-verify` and `make ci-lint` |
| Documentation | `make docs-lint` |
| Store | `make test-pg` (Docker) |
| Frontend | `make fe`, plus `make fe-visual` and `make e2e` for UI |

⚠ `make ci-lint` needs `shellcheck` on `PATH`. Without it, actionlint silently skips the shell
half and exits 0 locally while CI fails.

## Conventions

- **Commits:** conventional-commit style (`feat(api):`, `fix(store):`, `docs:`) with a body
  explaining the *why*.
- **Frontend:** folder-per-module, arrow functions, barrel exports, a story and a unit test per
  component. Request types are generated — never hand-written.
- **Tests never touch the network.** Extend `internal/testkit` rather than inventing a private
  mock.
- **Prove a new test works** by sabotaging the code and watching it go red. A test that has
  never failed may not be connected to anything.

## AI-assisted contributions

Welcome, and held to the same bar as everything else — much of this codebase was written that
way. You own the diff, you run the gates, and you check that the comments describe the code you
actually shipped rather than the approach you started with. Mention it in the PR body; it's
useful review context, not a mark against the change.

Details, and the specific failure modes this repo has learned to look for, are in
[AI in this project](docs/dev/ai.md).

## Reporting bugs / proposing features

Open an issue using the templates. For anything security-sensitive, follow
[`SECURITY.md`](SECURITY.md) instead of filing a public issue.

By contributing, you agree that your contributions are licensed under the project's
[MIT License](LICENSE).
