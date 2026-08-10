# Code generation

**Generated files are never hand-edited.** Each has a command that produces it and, where it's
committed, a `*-verify` target that fails CI when the two disagree.

## Committed — drift fails the build

| Artifact | From | Regenerate | Gate |
| --- | --- | --- | --- |
| `api/openapi.yaml` | the Huma operation definitions | `make openapi` | `make openapi-verify` |
| `docs/configuration.md` | the settings registry | `make config-docs` | `make config-docs-verify` |
| `docs/dev/commands.md` | the Makefile + the CI workflows | `make dev-docs` | `make dev-docs-verify` |
| `web/packages/tokens/generated/` | `packages/tokens` source | `make fe-tokens` | `make fe-tokens-verify` |
| Visual baselines (`*-linux.png`) | Playwright, pinned image | `make fe-visual-update` | `make fe-visual` |
| e2e page snapshots | Playwright, pinned image | `make e2e-update` | `make e2e` |

These are committed so a reviewer can see the effect of a change in the diff — a renamed field
shows up as a spec change in the same PR that renames it.

## Generated, not committed

| Artifact | From | Produced by |
| --- | --- | --- |
| `web/packages/api/generated/` | `api/openapi.yaml` via orval | `make fe-codegen` |
| `web/apps/web/src/routeTree.gen.ts` | TanStack Router file routes | `make fe-codegen` |
| `internal/web/dist/` | Vite | `make fe` / `make fe-build` |
| `web/apps/web/storybook-static/` | Storybook | `make storybook-build` |

The orval client is gitignored deliberately: the spec is the single source of truth, so there is
nothing for the client to drift *against*. That's also why a fresh clone typechecks red until
codegen has run once.

## Migrations are hand-written and forward-only

Goose migrations under `internal/store/migrations/{sqlite,postgres}` are written by hand and
applied in-process at boot. **Never edit an applied migration** — add a new one.

> ⚠ Two long-lived branches will both take the next free number, and goose silently skips the
> second. Check the highest number on `main` before adding one.

## The two traps worth knowing

**Generation carries names and types, not rules.** The generated zod schemas give you the wire
field names; every product rule — bounds, messages, trims — stays hand-authored. Deriving the
names is still what matters: a hand-mirrored schema once said `maxAcquire` where the wire says
`maxAcquisitions`, so a user's acquisition cap serialised into JSON the server ignored and
vanished silently.

**Generated MSW mock data is not trustworthy.** Optional fields emit as
`arrayElement([value, undefined])`, so presence varies per call and nothing is seeded — flaky
rather than merely arbitrary. The *wiring* is what generation buys you: a renamed route is fixed
by regenerating, where a hand-written path would silently stop matching and its test would keep
passing against nothing. Always pass explicit overrides from the shared fixtures.

## A note on `.gitkeep`

`internal/web/dist/.gitkeep` is the one tracked file in that directory. `//go:embed all:dist`
will not compile without the directory existing, and Vite's `emptyOutDir` deletes it — so a
build plugin rewrites it after every bundle. This broke clean-clone builds twice; don't "tidy"
it away.
