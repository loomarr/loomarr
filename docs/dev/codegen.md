# Code generation

Generated files are never hand-edited. Each has a command that produces it, and where it's
committed, a `*-verify` target that fails CI on drift.

## Committed

| Artifact | From | Regenerate | Gate |
| --- | --- | --- | --- |
| `api/openapi.yaml` | Huma operation definitions | `make openapi` | `make openapi-verify` |
| `docs/configuration.md` | the settings registry | `make config-docs` | `make config-docs-verify` |
| `docs/dev/commands.md` | the Makefile + CI workflows | `make dev-docs` | `make dev-docs-verify` |
| `docs/design.md` §2 map | package doc comments + imports | `make arch-docs` | `make arch-docs-verify` |
| `web/packages/tokens/generated/` | `packages/tokens` | `make fe-tokens` | `make fe-tokens-verify` |
| Visual baselines | Playwright, pinned image | `make fe-visual-update` | `make fe-visual` |
| e2e snapshots | Playwright, pinned image | `make e2e-update` | `make e2e` |

These are committed so a reviewer sees the effect in the diff — a renamed field shows up as a
spec change in the same PR.

## Not committed

| Artifact | From | Produced by |
| --- | --- | --- |
| `web/packages/api/generated/` | `api/openapi.yaml` via orval | `make fe-codegen` |
| `web/apps/web/src/routeTree.gen.ts` | TanStack Router file routes | `make fe-codegen` |
| `internal/web/dist/` | Vite | `make fe` / `make fe-build` |
| `web/apps/web/storybook-static/` | Storybook | `make storybook-build` |

The orval client is gitignored because the spec is the source of truth — there's nothing for it
to drift against. That's also why a fresh clone typechecks red until codegen has run.

## Migrations

Goose migrations under `internal/store/migrations/{sqlite,postgres}` are hand-written and applied
at boot. Never edit an applied migration; add a new one.

Two long-lived branches will both take the next free number and goose silently skips the second.
Check the highest number on `main` first.

## Two things generation doesn't give you

**Names and types, not rules.** The generated zod schemas give you wire field names; bounds,
messages and trims stay hand-authored. Deriving the names is still what matters — a hand-mirrored
schema once said `maxAcquire` where the wire says `maxAcquisitions`, so a user's setting
serialised into JSON the server ignored.

**Trustworthy wiring, untrustworthy data.** Generated MSW mocks emit optional fields as
`arrayElement([value, undefined])`, so presence varies per call. Pass explicit overrides from the
shared fixtures. What generation buys is the URL and method: a renamed route is fixed by
regenerating, where a hand-written path would silently stop matching and its test keep passing.

## `.gitkeep`

`internal/web/dist/.gitkeep` is the one tracked file there. `//go:embed all:dist` won't compile
without the directory, and Vite's `emptyOutDir` deletes it, so a build plugin rewrites it. Don't
tidy it away.
