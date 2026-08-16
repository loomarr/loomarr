# Deep TypeScript modules

Copy this shape when adding a package:

```text
web/packages/<name>/
├── index.ts            # entry point: public interface
├── client.ts           # optional additional entry point
├── package.json
├── src/                # private implementation
│   └── lib/
│       ├── index.ts
│       └── lib.ts
└── tests/              # tests and private fixtures
```

**Entry-point seam.** Import only through a package's entry points: the files at its package root. Anything in a subfolder is private implementation. Do not deep-import a sibling package or reach into one from app code.

**Intra-package freedom.** Files inside one package may import that package's `src/lib/` implementation freely. This keeps change knowledge local while the package presents a smaller interface to callers.

**Tests use the interface.** Put new package tests in `tests/` and import behavior through root entry points, including the package under test. Tests may share fixtures from their own `tests/` folder, but never import implementation from another subfolder.

**No cycles.** Package and app dependencies must remain acyclic. Run `pnpm lint:boundaries` from `web/`; the same gate runs with the frontend check.

Avoid giant barrel files that re-export a whole implementation tree. Add several focused root entry points when callers need distinct interfaces. The tree above is the starter template; production workspaces should contain only packages the product uses.

`@loomarr/api` keeps its focused `models/*`, `endpoints/*`, and `zod/*` public subpaths for route-level code splitting. Orval generation creates their ignored `model-*`, `endpoint-*`, and `zod-*` root entry files; change the generator or package exports instead of editing those files. `@loomarr/core/*` follows the same public-subpath shape with committed root entry files.
