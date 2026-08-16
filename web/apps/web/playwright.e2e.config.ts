import { defineConfig } from "@playwright/test";
import { DESKTOP, DETERMINISM, E2E_WORKERS } from "./playwright.shared";

// Wizard flow suite + page-level snapshots — the Phase 13.3 gate (frontend-build-plan §5).
// Distinct from the visual suite: that one snapshots isolated components out of
// storybook-static; this drives the REAL embedded SPA build through the whole first-run
// flow. The backend is mocked with Playwright route interception rather than MSW or a
// stub server — no new dependency, and no second implementation of the API to drift.
// Determinism comes from the shared kit; baselines are likewise Linux-only, generated via
// `make e2e-update` in the pinned Docker image.
const PORT = 6008;

export default defineConfig({
  ...DETERMINISM,
  testDir: "./tests/e2e",
  fullyParallel: true,
  // ⚠ LOCAL ONLY — `E2E_WORKERS` is undefined under CI, which leaves Playwright's own default
  // in place. This suite had no worker cap at all, so a local run took `cpus()/2` and booted a
  // browser per worker against the real SPA, the heavier of the two suites. Capping locally is
  // safe; RAISING it in CI is not, because tuner-surf measures latency against a threshold and
  // extra parallel browsers change the thing under test. See playwright.shared.
  workers: E2E_WORKERS,
  forbidOnly: !!process.env.CI,
  reporter: process.env.CI ? "github" : "list",
  use: { ...DETERMINISM.use, baseURL: `http://127.0.0.1:${PORT}` },
  // One viewport: this suite proves the FLOW works and pins each step's layout. Component
  // responsiveness is already covered at both viewports by the visual suite.
  projects: [{ name: "desktop", use: DESKTOP }],
  webServer: {
    // Serves the REAL SPA build (internal/web/dist) via the pure-JS http-server bin, for
    // the same reason the visual suite does: the bind-mounted node_modules carries the
    // HOST's native binaries, so anything with a platform-specific binding (vite/rollup)
    // cannot run inside the Linux image. The `--proxy …?` form is http-server's SPA
    // history fallback, so deep links like /wizard resolve the way they do from the Go
    // binary's embed rather than 404-ing.
    command: `node_modules/.bin/http-server ../../../internal/web/dist -p ${PORT} -s -c-1 --proxy http://127.0.0.1:${PORT}?`,
    url: `http://127.0.0.1:${PORT}/`,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
});
