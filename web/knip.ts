import type { KnipConfig } from "knip";

// knip: unused files, exports and dependencies across the web workspace (`make knip`).
//
// Every entry and ignore below exists because knip cannot see a real consumer, and each one
// names that consumer, so a reviewer can tell an allowance from a hiding place. The first
// answer to a finding is to delete the unused code. Add an allowance here only when something
// outside the TypeScript import graph really uses it (a string path, a native build, a
// generated file), and write down what. docs/dev/code-hygiene.md has the procedure.

const config: KnipConfig = {
  // A stale allowance fails the gate too: knip reports an ignore that no longer matches an
  // issue, and an entry that is redundant, as a configuration hint, and this makes hints errors.
  treatConfigHintsAsErrors: true,
  // A module exports its Props type beside the component (folder-per-module). A type its own
  // file uses is that module's interface; a type nothing uses at all is still reported.
  ignoreExportsUsedInFile: { interface: true, type: true },
  // The same shape has each module's barrel re-export its Props/value types beside the
  // component (`export type { XProps } from "./x.type"`). That is the module's interface
  // whether or not a caller spells the type yet. Unused VALUE exports in barrels are reported.
  ignoreIssues: { "{apps,packages}/*/src/**/index.ts": ["types"] },
  // Folder-per-module (apps/web/src/test/structure.test.ts) REQUIRES an index.ts beside every
  // `<folder>/<folder>.ts(x)` module, even when every caller reaches the module some other
  // way. These are those barrels. The module behind each one is still checked: if nothing
  // uses it, knip reports the implementation file. Because hints are errors, an entry here
  // fails the gate as soon as something starts importing through its barrel.
  ignoreFiles: [
    // client-platform-proof.html loads the .tsx directly.
    "apps/web/src/client-platform-proof/index.ts",
    // Storybook foundation pages: each story imports its sibling implementation.
    "apps/web/src/design/palette/index.ts",
    "apps/web/src/design/spacing/index.ts",
    "apps/web/src/design/tokens/index.ts",
    "apps/web/src/design/typography/index.ts",
    // tasks-page imports its private child module directly.
    "apps/web/src/settings/tasks-page/job-edit-modal/index.ts",
    // The package's root browser.ts entry point re-exports the implementation file.
    "packages/player/src/browser/index.ts",
  ],
  // System tools the node:test suites shell out to, not npm packages.
  ignoreBinaries: ["awk", "java", "javac"],
  workspaces: {
    ".": {
      // CLIs run by package.json scripts, shell scripts (capture-tv-native-references.sh)
      // and the Makefile.
      entry: ["scripts/*.{mjs,cjs}"],
      // Both tests resolve these through Expo's own chain on purpose (createRequire from expo →
      // @expo/metro-config → metro; the app's .bin/expo-modules-autolinking): they assert what
      // the native build loads, so declaring a second copy would test the wrong one.
      ignoreDependencies: ["metro", "expo-modules-autolinking"],
      // Written by `sb-rn-get-stories` (native-storybook:check) and gitignored.
      ignoreUnresolved: ["./storybook.requires"],
    },
    "apps/web": {
      entry: [
        // TanStack file routes. The generated, gitignored routeTree.gen.ts imports every file
        // under routes/ except `-`-prefixed ones, which are ordinary modules the routes import.
        // Listed so the result does not depend on whether codegen has run.
        "src/routes/**/!(-*).tsx",
        // client-platform-proof.html → `pnpm client-proof` (make clients).
        "src/client-platform-proof/client-platform-proof.tsx",
        "tests/client-platform-proof.ssr.test.tsx",
        // Registered by URL: navigator.serviceWorker.register("/push-worker.js").
        "public/push-worker.js",
      ],
      playwright: { config: ["playwright*.config.ts"] },
      vite: { config: ["vite.config.ts", "vite.client-platform.config.ts"] },
      // Run as node_modules/.bin/http-server by the Playwright webServer commands.
      ignoreDependencies: ["http-server"],
    },
    "apps/mobile": {
      // Metro bundles web/native-stories (which import @loomarr/fixtures) into the app when
      // STORYBOOK_ENABLED is set, resolving from the app. expo-updates: knip assumes it unless
      // app.json sets updates.enabled=false; the app ships no OTA updates.
      ignoreDependencies: ["@loomarr/fixtures", "expo-updates"],
    },
    "apps/tv": {
      // Expo's dynamic config, loaded by the Expo CLI.
      entry: ["app.config.cjs"],
      ignoreDependencies: ["@loomarr/fixtures", "expo-updates"],
    },
    "packages/api": {
      // Used by the Orval-generated client (model-*, endpoint-*, zod-*, msw, generated/), which
      // is gitignored and so outside knip's view.
      ignoreDependencies: ["@faker-js/faker", "@tanstack/react-query", "@types/react", "msw", "react", "zod"],
    },
    "packages/design-system": {
      // The Storybook build (apps/web `build-storybook`) resolves `react-native` from this
      // package's own node_modules. Without its react-native-web the build parses
      // react-native-tvos' Flow source and fails; no TypeScript import shows that.
      ignoreDependencies: ["react-native-web"],
      // vitest.config.ts re-exports vitest.universal.config.ts, which has no `test` block, so
      // knip's Vitest plugin adds no test entries; vitest itself runs its default include.
      entry: ["tests/**/*.test.{ts,tsx}"],
    },
    "packages/lan-discovery-native": {
      // React Native CLI autolinking config.
      entry: ["react-native.config.cjs"],
    },
    "packages/ui": {
      entry: ["tests/**/*.test.{ts,tsx}"],
    },
  },
};

export default config;
