# Developing Loomarr

This directory is the single source for how to build, run and test Loomarr. `README.md`,
`README.md`, `CONTRIBUTING.md`, and agent-specific adapters link here. `AGENTS.md` keeps the concise
cross-harness contract and points here for detail.

| Page | Answers |
| --- | --- |
| [Setup](setup.md) | Toolchain versions, getting a clean clone green |
| [Dev loop](dev-loop.md) | Running both halves with live reload |
| [Testing](testing.md) | The test layers and which are gates |
| [CI](ci.md) | The jobs, path filters, and what's required |
| [Android TV beta](android-beta.md) | Play identity, signing, testing tracks, and Shield acceptance |
| [Codegen](codegen.md) | What's generated and what's committed |
| [Commands](commands.md) | Every `make` target — generated, don't hand-edit |
| [AI in this project](ai.md) | Built with coding agents, and what that expects of a PR |
| [Agent development](agents.md) | Cross-harness sessions, claims, worktrees, and runtime isolation |
| [Agent skills](skills.md) | The curated skills, invocation policy, and reasons to delegate |

## The rules that matter

1. **`docs/design.md` is the source of truth.** If code must deviate, update the doc in the same
   PR, first.
2. **Gates are hard.** Never weaken a selected local or CI gate to pass it. Use focused tests while
   editing and `make verify BASE=origin/main` before pushing; reserve `make verify SCOPE=all` for a deliberate
   complete-repository audit.
3. **Generated files are never hand-edited** — `api/openapi.yaml`, `docs/configuration.md`,
   `docs/dev/commands.md`, the orval client, token artifacts. Migrations are forward-only.
4. **Tests never touch the network.** Mock through `internal/testkit`; extend it rather than
   writing a private mock.
5. **Application code is Go.** Exceptions: the frontend, the vendored binaries invoked via
   `exec`, and build tooling like Storybook and the docs site.

New dependencies are fine when they earn their place — add a row to `docs/design.md` §14 in the
same PR.

## Where the code lives

```text
cmd/loomarr/      the binary
cmd/openapi/      exports api/openapi.yaml
cmd/config-docs/  generates docs/configuration.md
cmd/dev-docs/     generates docs/dev/commands.md
cmd/arch-docs/    generates the §2 package map
docs/diagrams/    keeps D2 sources beside generated SVG diagrams
cmd/seed/         populates a dev store

internal/app/       wires every subsystem from an open store
internal/api/       inbound HTTP — ServeMux + Huma v2
internal/store/     one interface, SQLite and Postgres behind it
internal/settings/  the typed registry: env > database > default
internal/playout/   the streaming engine
internal/suggest/   intent → grounded proposal
internal/schedule/  the scheduler domain
internal/filler/    clip catalog and pod assembly
internal/testkit/   shared doubles and pinned fixtures

web/apps/web/       the SPA, built into internal/web/dist and embedded
web/packages/       tokens · api (orval) · core · fixtures
docs-site/          the docs site — renders docs/ in place
```

`docs/design.md` §14.2 has the full package map.
