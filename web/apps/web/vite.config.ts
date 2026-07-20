/// <reference types="vitest/config" />
import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import tailwindcss from "@tailwindcss/vite";
import { tanstackRouter } from "@tanstack/router-plugin/vite";
import react from "@vitejs/plugin-react";
import { defineConfig, type Plugin } from "vite";

// The SPA is embedded in the Go binary and served same-origin at / (main doc §12).
// Build output lands in internal/web/dist, which internal/web/embed.go embeds.
// In dev, Vite proxies the API surface to the Go server on :8080 so the app runs
// against the real backend with cookie auth intact.
const API_TARGET = process.env.LOOMARR_API ?? "http://localhost:8080";
const proxied = [
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

export default defineConfig({
  // tanstackRouter must precede react() — it generates src/routeTree.gen.ts from the
  // file-based routes in src/routes before React compiles (design §14).
  plugins: [
    keepEmbedDir(),
    tanstackRouter({ target: "react", autoCodeSplitting: true }),
    react(),
    tailwindcss(),
  ],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  server: {
    port: 5173,
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
    css: false,
    // jsdom units only — Playwright visual specs (tests/visual/*.spec.ts) run under
    // Playwright, not vitest, and Storybook stories are exercised by the visual suite.
    include: ["src/**/*.test.{ts,tsx}"],
    exclude: ["node_modules", "dist", "storybook-static", "tests/visual/**"],
    passWithNoTests: true,
  },
});
