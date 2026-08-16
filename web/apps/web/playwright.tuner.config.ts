import { defineConfig, devices } from "@playwright/test";
import { DETERMINISM } from "./playwright.shared";

// The controller certification is deliberately separate from the page-snapshot suite. It drives
// one behavior-heavy tuner scenario through every browser engine without tripling unrelated
// wizard screenshots or pretending Playwright WebKit is shipping Safari certification.
const PORT = 6009;
const viewport = { width: 1280, height: 800 };

export default defineConfig({
  ...DETERMINISM,
  testDir: "./tests/e2e",
  testMatch: "tuner-surf.spec.ts",
  fullyParallel: false,
  // This is a behavioral/performance gate, not a screenshot suite: engines run alone so their
  // timings are independent, and a black start fails instead of being reclassified as flaky.
  workers: 1,
  retries: 0,
  forbidOnly: !!process.env.CI,
  reporter: process.env.CI ? "github" : "list",
  use: { ...DETERMINISM.use, baseURL: `http://127.0.0.1:${PORT}` },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"], viewport },
    },
    {
      name: "firefox",
      use: { ...devices["Desktop Firefox"], viewport, launchOptions: { args: [] } },
    },
    {
      name: "webkit",
      use: { ...devices["Desktop Safari"], viewport, launchOptions: { args: [] } },
    },
  ],
  webServer: {
    command: "node --experimental-strip-types tests/e2e/tuner-backend.ts",
    url: `http://127.0.0.1:${PORT}/`,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
});
