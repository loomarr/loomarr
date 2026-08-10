# Developing Loomarr

**This directory is the single source for how to build, run and test Loomarr.** `README.md`,
`CONTRIBUTING.md`, `CLAUDE.md` and `AGENTS.md` link here rather than restating it — they each
used to carry their own copy, and the copies disagreed about the Go version, the Node version,
what `make fe` runs, and how large the visual suite is.

| Page | Answers |
| --- | --- |
| [Setup](setup.md) | Toolchain versions and getting a clean clone green |
| [The dev loop](dev-loop.md) | Running both halves with live reload |
| [Testing](testing.md) | The test layers, what each proves, and which are gates |
| [CI](ci.md) | The jobs, the path filters, and what's actually required |
| [Codegen](codegen.md) | What's generated, by what, and what's committed |
| [Commands](commands.md) | Every `make` target — **generated**, never hand-edited |
| [AI in this project](ai.md) | Built with coding agents — the conventions that follow from it |

## The five rules that matter

1. **`docs/design.md` is the source of truth.** If code must deviate, update the doc in the
   *same PR, first*. Doc and code never disagree silently.
2. **Gates are hard.** `make check` must be green. Never stub, skip or weaken a test to pass
   one — if a gate can't pass, either the design or the code is wrong. Fix one of them.
3. **Generated files are never hand-edited.** `api/openapi.yaml`, `docs/configuration.md`,
   `docs/dev/commands.md`, the orval client, the token artifacts. Migrations are forward-only:
   add a new one, never edit an applied one.
4. **Tests never touch the network.** External services are mocked through `internal/testkit` —
   extend it rather than inventing a private mock.
5. **All application code is Go.** The exceptions are the frontend (compiles to embedded static
   assets), the vendored binaries invoked via `exec`, and build tooling like Storybook and the
   docs site — none of which ship in the binary.

New dependencies are fine when they earn their place; record them in `docs/design.md` §14 in the
same PR with a one-line rationale.

## Where the code lives

```text
cmd/loomarr/      the binary — config, logging, HTTP server, graceful restart
cmd/openapi/      exports api/openapi.yaml from the Huma definitions
cmd/config-docs/  generates docs/configuration.md from the settings registry
cmd/dev-docs/     generates docs/dev/commands.md from the Makefile + workflows
cmd/seed/         populates a dev store through real domain paths

internal/app/       composition root — wires every subsystem from an open store
internal/api/       inbound HTTP: stdlib ServeMux + Huma v2, code-first OpenAPI
internal/store/     persistence: one interface, SQLite and Postgres behind it
internal/settings/  the typed registry — env > database > default
internal/playout/   Loomarr's own streaming engine
internal/suggest/   intent → grounded proposal
internal/schedule/  the scheduler domain
internal/filler/    clip catalog and pod assembly
internal/testkit/   shared doubles + pinned fixtures — every test uses these

web/apps/web/       the SPA, built into internal/web/dist and embedded
web/packages/       tokens · api (orval) · core · fixtures
docs-site/          the Starlight docs site — renders docs/ in place
```

`internal/` has more packages than this; each carries a doc comment saying what it owns.
`docs/design.md` §14.2 has the full map.
