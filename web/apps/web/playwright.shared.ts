import { devices } from "@playwright/test";

// The determinism kit both Playwright suites share (frontend-design §5.2), in ONE place.
//
// There are two configs because Playwright boots every configured `webServer` regardless
// of which project you filter to, and the suites have different build prerequisites —
// the gallery needs `storybook-static`, the flow suite needs the real SPA in
// `internal/web/dist`. Merging them would force you to build both to run either.
//
// What must NOT be duplicated is this: a tuning change (diff ratio, launch flags) that
// lands in only one config is a silent divergence between what the gallery gate and the
// flow gate consider "the same pixel".
const DETERMINISM = {
  // Residual sub-pixel text-AA jitter can nudge a rare shot past the strict ratio.
  // Retries de-flake that WITHOUT masking real regressions — a genuine diff reproduces
  // and still fails every attempt.
  retries: 2,
  expect: { toHaveScreenshot: { maxDiffPixelRatio: 0.001, animations: "disabled" as const } },
  use: {
    reducedMotion: "reduce" as const,
    // ⚠ **TIMEZONE, and it is not a nicety.** Fixtures pin instants with `Date.UTC(...)`,
    // but anything rendering a WALL-CLOCK time formats it in the browser's zone — so the
    // same story shot on an EDT laptop and in the UTC container disagree by four hours.
    // The guide's FullDay story renders ~8 hourly labels across a 12-hour window and every
    // one of them moved; its 2-hour siblings show too few labels to notice, which is how a
    // whole class of drift hid behind five passing stories.
    //
    // UTC because the container already runs it, so a baseline shot on a developer's
    // machine matches CI rather than the reverse.
    timezoneId: "UTC",
    // The three sources of PIXEL drift, pinned: software GL, a fixed sRGB profile,
    // and grayscale (non-subpixel) text AA.
    launchOptions: { args: ["--disable-gpu", "--force-color-profile=srgb", "--disable-lcd-text"] },
  },
};

const DESKTOP = { ...devices["Desktop Chrome"], viewport: { width: 1280, height: 800 } };
const MOBILE = { ...devices["Desktop Chrome"], viewport: { width: 390, height: 844 } };

export { DESKTOP, DETERMINISM, MOBILE };
