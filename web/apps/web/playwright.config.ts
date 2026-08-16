import { defineConfig } from "@playwright/test";
import { DESKTOP, DETERMINISM, MOBILE, WORKERS } from "./playwright.shared";

// Visual + a11y suite (frontend-design §5) over the OFFLINE storybook-static build —
// Chromatic rejected. Deterministic by construction via the shared kit in
// ./playwright.shared (one rasterizer, one font stack, motion frozen). Baselines are
// COMMITTED and MUST be generated in the Docker image via `make fe-visual-update` — a
// macOS run writes darwin-suffixed snapshots CI won't use.
//
// The flow suite lives in playwright.e2e.config.ts; see playwright.shared for why the
// two are separate configs rather than one.
const PORT = 6007;

export default defineConfig({
  ...DETERMINISM,
  testDir: "./tests/visual",
  fullyParallel: true,
  // Playwright defaults to HALF the available cores, which on GitHub's 4-core runner meant 2
  // workers for 504 tests (~5.1 min of a 6.8 min job). The suite is hermetic — a static
  // storybook server, stubbed network, no shared state — so nothing contends and the full
  // core count is safe.
  //
  // Derived from cpus() rather than hardcoded: the runner ALSO reports 4 today, and pinning a
  // literal would silently cap a larger runner later while looking deliberate.
  //
  // ⚠ Local runs are no longer left on Playwright's default. "Half the machine" is a CPU
  // statement, and the resource these suites exhaust is MEMORY — on a 24-core desktop the
  // default asked for 12 browsers and swap-thrashed the box into a hard restart. See WORKERS.
  workers: WORKERS,
  forbidOnly: !!process.env.CI,
  reporter: process.env.CI ? "github" : "list",
  use: { ...DETERMINISM.use, baseURL: `http://127.0.0.1:${PORT}` },
  projects: [
    { name: "desktop", use: DESKTOP },
    { name: "mobile", use: MOBILE },
  ],
  webServer: {
    // Serve the static gallery via the local http-server bin (no pnpm needed — so this
    // runs identically on the host and inside the Playwright Docker image, §5.2).
    command: `node_modules/.bin/http-server storybook-static -p ${PORT} -s -c-1`,
    url: `http://127.0.0.1:${PORT}/index.json`,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
});
