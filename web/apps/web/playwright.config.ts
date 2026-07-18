import { defineConfig, devices } from "@playwright/test";

// Visual + a11y suite (frontend-design §5) over the OFFLINE storybook-static build —
// Chromatic rejected. Deterministic by construction: the official Playwright Docker
// image in CI (one rasterizer, one font stack), reduced-motion forced (trips the global
// CSS gate, §2.4), animations frozen at their end state, self-hosted fonts awaited per
// test. Baselines are COMMITTED and MUST be generated in the Docker image via
// `make fe-visual-update` — a macOS run writes darwin-suffixed snapshots CI won't use.
const PORT = 6007;

export default defineConfig({
  testDir: "./tests/visual",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  reporter: process.env.CI ? "github" : "list",
  // Residual sub-pixel text-AA jitter can nudge a rare story past the strict 0.001 ratio.
  // Retries de-flake that WITHOUT masking real regressions — a genuine diff (or an a11y
  // violation) reproduces and still fails every attempt.
  retries: 2,
  expect: { toHaveScreenshot: { maxDiffPixelRatio: 0.001, animations: "disabled" } },
  use: {
    baseURL: `http://127.0.0.1:${PORT}`,
    reducedMotion: "reduce",
    // Deterministic rasterization inside the Docker image (§5.2): software GL, a fixed
    // sRGB color profile, and grayscale (non-subpixel) text AA — the three sources of
    // screenshot drift, pinned.
    launchOptions: { args: ["--disable-gpu", "--force-color-profile=srgb", "--disable-lcd-text"] },
  },
  projects: [
    { name: "desktop", use: { ...devices["Desktop Chrome"], viewport: { width: 1280, height: 800 } } },
    { name: "mobile", use: { ...devices["Desktop Chrome"], viewport: { width: 390, height: 844 } } },
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
