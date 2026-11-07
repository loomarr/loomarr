// @ts-check
// Deep-module enforcement for the TypeScript workspace.
//
// Root files under `packages/<name>/` are entry points; anything nested below
// them is private implementation. Loomarr keeps implementation in `src/` and
// tests in `tests/`. A package may expose several small entry points instead of
// funnelling its whole interface through one barrel.

const fs = require("node:fs");
const path = require("node:path");

/** One immediate child directory per package (flat, no nested packages). */
const PACKAGES_ROOT = "packages";

const R = PACKAGES_ROOT;
const PACKAGE_INTERNALS = `^${R}/[^/]+/[^/]+/`;
const PACKAGE_TESTS = `^${R}/[^/]+/tests/`;

// The layers, lowest first. A package imports only packages in a LOWER layer: the wire and the
// design system at the bottom, the domain over the wire, playback and native adapters over the
// domain, product modules over those, and the TV's remote focus over the product modules. Apps sit
// above every package and are not listed.
const LAYERS = [
  ["api", "design-system"],
  ["core"],
  ["player", "crash-reports-native", "lan-discovery-native", "fixtures"],
  ["ui", "native-app"],
  ["ui-tv"],
];

// A package with no layer would be unchecked, so a new one must be placed before the gate runs.
const unlayered = fs
  .readdirSync(path.join(__dirname, R), { withFileTypes: true })
  .filter((entry) => entry.isDirectory() && fs.existsSync(path.join(__dirname, R, entry.name, "package.json")))
  .map((entry) => entry.name)
  .filter((name) => !LAYERS.flat().includes(name));
if (unlayered.length > 0) {
  throw new Error(`.dependency-cruiser.cjs: place ${unlayered.join(", ")} in LAYERS`);
}

const layerRules = LAYERS.map((layer, rank) => ({
  name: `layer-${rank}-${layer.join("-")}`,
  comment: `${layer.join(", ")} may import only packages in a lower layer.`,
  severity: /** @type {const} */ ("error"),
  from: { path: `^${R}/(${layer.join("|")})/` },
  to: { path: `^${R}/(${LAYERS.slice(rank).flat().join("|")})/`, pathNot: `^${R}/$1/` },
}));

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
    {
      name: "not-to-unresolvable",
      comment:
        "Every import resolves. The rules above only see edges the resolver follows, so an unresolved import is an unchecked one. Generated files (route tree, API client) must exist: run `pnpm codegen` first, as every gate does.",
      severity: "error",
      from: {},
      to: { couldNotResolve: true },
    },

    // Layering controls WHICH packages may depend on which; interface hiding
    // above controls HOW callers reach them.
    ...layerRules,
    {
      name: "packages-never-import-apps",
      comment: "A package serves the apps; it never reaches back into one.",
      severity: "error",
      from: { path: `^${R}/` },
      to: { path: "^apps/" },
    },
  ],
  options: {
    // dependency-cruiser parses TypeScript with the `typescript` package only below 7, and the
    // workspace is on 7. Without a parser it scans no .ts/.tsx file in a folder and checks only
    // what the root entry points import, so the app folders went unchecked. swc reads both.
    parser: "swc",
    doNotFollow: { path: "node_modules|^packages/[^/]+/generated/" },
    tsConfig: { fileName: "tsconfig.depcruise.json" },
    enhancedResolveOptions: {
      extensions: [".ts", ".tsx", ".js", ".jsx", ".json"],
      // Workspace packages publish subpaths (`@loomarr/api/endpoints/*`) through package.json
      // `exports`; without this the resolver gives up on them.
      exportsFields: ["exports"],
      conditionNames: ["import", "require", "types", "default"],
    },
  },
};
