import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// The SPA is embedded in the Go binary and served same-origin at / (main doc §12).
// Build output lands in internal/web/dist, which internal/web/embed.go embeds.
// In dev, Vite proxies the API surface to the Go server on :8080 so the app runs
// against the real backend with cookie auth intact.
const API_TARGET = process.env.LOOMARR_API ?? "http://localhost:8080";
const proxied = ["/v1", "/hooks", "/docs", "/openapi.json", "/openapi.yaml", "/healthz", "/readyz", "/metrics"];

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  server: {
    port: 5173,
    proxy: Object.fromEntries(
      proxied.map((path) => [
        path,
        { target: API_TARGET, changeOrigin: true, ws: path === "/v1" },
      ]),
    ),
  },
  build: {
    outDir: fileURLToPath(new URL("../../../internal/web/dist", import.meta.url)),
    emptyOutDir: true,
  },
});
