import { defineConfig } from "@playwright/test";
import { DESKTOP, DETERMINISM } from "./playwright.shared";

// The route-timing gate (#3399): the shipping SPA driven through the main nav in Chromium, with
// the browser's own route marks asserted against warm and cold budgets. It sits beside the tuner
// matrix in CI (same macOS runner, same host-installed browser, same `make fe-build` output) and
// is kept out of the wizard e2e suite for the same reason the tuner is: it is a timing gate, so it
// runs alone, serially, with no retries to hide a slow run.
const PORT = 6010;

export default defineConfig({
  ...DETERMINISM,
  testDir: "./tests/e2e",
  testMatch: ["route-timing.spec.ts"],
  fullyParallel: false,
  workers: 1,
  retries: 0,
  forbidOnly: !!process.env.CI,
  // `list` as well as `github`: the spec prints its table, and `github` alone drops stdout.
  reporter: process.env.CI ? [["github"], ["list"]] : "list",
  // Playwright's actions wait forever by default; here a control that cannot be pressed must fail
  // the step in seconds, not run out the whole test's clock.
  use: { ...DETERMINISM.use, baseURL: `http://127.0.0.1:${PORT}`, actionTimeout: 10_000 },
  projects: [{ name: "chromium", use: DESKTOP }],
  webServer: {
    // The real SPA build over the pure-JS static server, history fallback included, exactly as the
    // e2e suite serves it (playwright.e2e.config.ts). The backend is the spec's own route mock.
    command: `node_modules/.bin/http-server ../../../internal/platform/web/dist -p ${PORT} -s -c-1 --proxy http://127.0.0.1:${PORT}?`,
    url: `http://127.0.0.1:${PORT}/`,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
});
