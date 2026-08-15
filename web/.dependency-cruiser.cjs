// @ts-check
// Deep-module enforcement for the TypeScript workspace.
//
// Root files under `packages/<name>/` are entry points; anything nested below
// them is private implementation. Loomarr keeps implementation in `src/` and
// tests in `tests/`. A package may expose several small entry points instead of
// funnelling its whole interface through one barrel.

/** One immediate child directory per package (flat, no nested packages). */
const PACKAGES_ROOT = "packages";

const R = PACKAGES_ROOT;
const PACKAGE_INTERNALS = `^${R}/[^/]+/[^/]+/`;
const PACKAGE_TESTS = `^${R}/[^/]+/tests/`;

/** @type {import('dependency-cruiser').IConfiguration} */
module.exports = {
  forbidden: [
    {
      name: "entrypoint-boundary-from-app",
      comment:
        "App code may import a package's root entry points, but nothing nested beneath them.",
      severity: "error",
      from: { pathNot: `^${R}/` },
      to: { path: PACKAGE_INTERNALS },
    },
    {
      name: "entrypoint-boundary-across-packages",
      comment:
        "A package's own files import each other freely, but sibling packages are reached only through root entry points.",
      severity: "error",
      from: { path: `^${R}/([^/]+)/`, pathNot: PACKAGE_TESTS },
      to: {
        path: PACKAGE_INTERNALS,
        pathNot: `^${R}/$1/`,
      },
    },
    {
      name: "tests-through-entrypoints",
      comment:
        "A package's tests exercise package interfaces, while their own tests/ fixtures remain available.",
      severity: "error",
      from: { path: `^${R}/([^/]+)/tests/` },
      to: {
        path: PACKAGE_INTERNALS,
        pathNot: `^${R}/$1/tests/`,
      },
    },
    {
      name: "tests-folder-is-private",
      comment: "Only tests may import files from a package's tests/ folder.",
      severity: "error",
      from: { pathNot: PACKAGE_TESTS },
      to: { path: PACKAGE_TESTS },
    },
    {
      name: "no-circular",
      comment: "No dependency cycles.",
      severity: "error",
      from: {},
      to: { circular: true },
    },

    // Layering controls WHICH packages may depend on which; interface hiding
    // above controls HOW callers reach them. Add Loomarr-specific layers here.
  ],
  options: {
    doNotFollow: { path: "node_modules|^packages/[^/]+/generated/" },
    tsConfig: { fileName: "tsconfig.base.json" },
    enhancedResolveOptions: {
      extensions: [".ts", ".tsx", ".js", ".jsx", ".json"],
    },
  },
};
