/// <reference types="vitest/config" />
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { basename, join } from "node:path";
import { fileURLToPath } from "node:url";
import tailwindcss from "@tailwindcss/vite";
import { tanstackRouter } from "@tanstack/router-plugin/vite";
import react from "@vitejs/plugin-react";
import { defineConfig, type Plugin } from "vite";

// The SPA is embedded in the Go binary and served same-origin at / (main doc §12).
// Build output lands in internal/web/dist, which internal/web/embed.go embeds.
// In dev, Vite proxies the API surface to the Go server on :8080 so the app runs
// against the real backend with cookie auth intact.
const BACKEND_PORT = process.env.LOOMARR_DEV_PORT ?? "8080";
const DEV_PORT = Number(process.env.LOOMARR_FE_PORT ?? "5173");
const API_TARGET = process.env.LOOMARR_API ?? `http://localhost:${BACKEND_PORT}`;
const webReact = fileURLToPath(new URL("./node_modules/react", import.meta.url));
const webReactDOM = fileURLToPath(new URL("./node_modules/react-dom", import.meta.url));
const reactNativeWeb = fileURLToPath(new URL("./node_modules/react-native-web", import.meta.url));
const reactNativeSvgWeb = fileURLToPath(
  new URL("./node_modules/react-native-svg/lib/module/elements.web.js", import.meta.url),
);
// react-native-svg's web build needs two React Native modules react-native-web lacks: the asset
// registry package React Native 0.88 no longer installs, and an Android-only TurboModule.
const reactNativeWebAssetRegistry = fileURLToPath(
  new URL("./node_modules/react-native-web/dist/modules/AssetRegistry/index.js", import.meta.url),
);
const reactNativeSvgSoundModule = fileURLToPath(
  new URL("../../react-native-svg-sound-module.web.js", import.meta.url),
);
const proxied = [
  // /v1 covers the whole versioned surface INCLUDING /v1/playout (§9.1 V47: the playout streaming
  // routes moved under /v1, so the in-app HLS player's same-origin /v1/playout/hls/... URLs are
  // proxied to the Go server automatically — no separate entry needed).
  "/v1",
  "/hooks",
  "/docs",
  "/openapi.json",
  "/openapi.yaml",
  "/healthz",
  "/readyz",
  "/metrics",
];

// internal/web/embed.go declares `//go:embed all:dist`, which does not compile unless the
// directory exists — so .gitkeep is the one committed file in there (.gitignore un-ignores
// exactly that path). But `emptyOutDir: true` deletes it on every build, and a routine
// `git add -A` then stages the deletion. That has now broken the clean-clone build twice.
//
// Rewriting it after the bundle closes the loop at the cause, rather than re-adding the
// file and waiting for the next build to remove it again.
const keepEmbedDir = (): Plugin => ({
  name: "loomarr-keep-embed-dir",
  apply: "build",
  closeBundle() {
    const dir = fileURLToPath(new URL("../../../internal/web/dist", import.meta.url));
    mkdirSync(dir, { recursive: true });
    writeFileSync(join(dir, ".gitkeep"), "");
  },
});

// The document's first frame. Before the stylesheet arrives, a fresh browsing context — the
// phone's pairing sheet, a new tab — paints the browser's default canvas, which is WHITE, for as
// long as the CSS and module graph take on a phone. An inline canvas colour paints the app's own
// background from the first parsed byte, and theme-color tints the browser chrome to match.
// Injected here from the token rather than written into index.html, so the value cannot drift.
// It is read from the generated token stylesheet, the package's public CSS entry: the config runs
// in plain Node, which cannot load the package's TypeScript entries.
// Only the app's index.html: Storybook's iframe.html goes through this config too, and its
// baselines are framed on their own backgrounds.
const firstPaintCanvas = (): Plugin => {
  const theme = readFileSync(fileURLToPath(import.meta.resolve("@loomarr/tokens/theme.css")), "utf8");
  const canvas = /--color-static-950:\s*(#[0-9A-Fa-f]{6});/.exec(theme)?.[1];
  if (!canvas) throw new Error("vite.config: --color-static-950 is missing from @loomarr/tokens/theme.css");
  return {
    name: "loomarr-first-paint-canvas",
    transformIndexHtml: (_html, ctx) =>
      basename(ctx.filename) === "index.html"
        ? [
            { tag: "meta", attrs: { name: "theme-color", content: canvas }, injectTo: "head" },
            {
              tag: "style",
              children: `html{background-color:${canvas};color-scheme:dark}`,
              injectTo: "head",
            },
          ]
        : undefined,
  };
};

export default defineConfig({
  // tanstackRouter must precede react() — it generates src/routeTree.gen.ts from the
  // file-based routes in src/routes before React compiles (design §14).
  plugins: [
    keepEmbedDir(),
    firstPaintCanvas(),
    tanstackRouter({ target: "react", autoCodeSplitting: true }),
    react(),
    tailwindcss(),
  ],
  resolve: {
    // The production Web root now mounts the same universal design-system provider as native
    // clients. Resolve its host primitives through the browser adapters and one React runtime;
    // otherwise Vite reaches react-native-tvos' untranspiled Flow entrypoint.
    alias: [
      { find: "@", replacement: fileURLToPath(new URL("./src", import.meta.url)) },
      { find: /^react$/, replacement: webReact },
      { find: /^react-dom$/, replacement: webReactDOM },
      { find: /^react-native$/, replacement: reactNativeWeb },
      { find: /^react-native-svg$/, replacement: reactNativeSvgWeb },
      { find: /^@react-native\/assets-registry\/registry$/, replacement: reactNativeWebAssetRegistry },
      { find: /^\.\.\/fabric\/NativeSvgSoundModule$/, replacement: reactNativeSvgSoundModule },
    ],
    dedupe: ["react", "react-dom", "react-native"],
    extensions: [".web.mjs", ".web.js", ".web.ts", ".web.tsx", ".mjs", ".js", ".ts", ".tsx", ".json"],
  },
  // This directory also contains client-platform-proof.html, whose entrypoint deliberately imports
  // React Native modules. Dev dependency discovery must stay rooted at the browser app; otherwise
  // Vite tries to parse react-native-tvos' Flow sources before the web aliases can apply.
  optimizeDeps: {
    entries: ["index.html"],
  },
  server: {
    port: DEV_PORT,
    // Multiple worktrees receive distinct deterministic ports. If one is unexpectedly occupied,
    // fail with the advertised URL instead of Vite silently incrementing to an unknown port.
    strictPort: true,
    proxy: Object.fromEntries(
      proxied.map((path) => [path, { target: API_TARGET, changeOrigin: true, ws: path === "/v1" }]),
    ),
  },
  build: {
    outDir: fileURLToPath(new URL("../../../internal/web/dist", import.meta.url)),
    // Wipes the directory each build, which is what deletes .gitkeep — see keepEmbedDir.
    emptyOutDir: true,
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
    // The shared browser-player adapter loads hls.js dynamically from its owning package. Alias the
    // transport explicitly so production-wrapper tests exercise the public adapter with a bounded
    // MediaSource controller even when pnpm resolves the dependency from that package.
    alias: {
      "hls.js": fileURLToPath(new URL("./src/test/hls.mock.ts", import.meta.url)),
    },
    // The Guide renders ui/guide, whose design-system icons come from lucide-react-native. Left
    // external, Node requires its `react-native` directly and meets the Flow source; inlined, the
    // react-native-web alias above applies, as in vitest.universal.config.ts.
    server: { deps: { inline: ["lucide-react-native"] } },
    css: false,
    // jsdom units only — Playwright visual specs (tests/visual/*.spec.ts) run under
    // Playwright, not vitest, and Storybook stories are exercised by the visual suite.
    include: ["src/**/*.test.{ts,tsx}"],
    exclude: ["node_modules", "dist", "storybook-static", "tests/visual/**"],
    passWithNoTests: true,
  },
});
