/// <reference types="vitest/config" />
import { fileURLToPath } from "node:url";
import tailwindcss from "@tailwindcss/vite";
import { tanstackRouter } from "@tanstack/router-plugin/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

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

export default defineConfig({
  // tanstackRouter must precede react() — it generates src/routeTree.gen.ts from the
  // file-based routes in src/routes before React compiles (design §14).
  plugins: [tanstackRouter({ target: "react", autoCodeSplitting: true }), react(), tailwindcss()],
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
