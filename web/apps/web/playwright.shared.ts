import { cpus, freemem } from "node:os";
import { devices } from "@playwright/test";

// How many workers a LOCAL run may use. CI passes its own count and never calls this.
//
// ⚠ Bound by MEMORY, not cores, because memory is what actually runs out. Playwright's
// default is `cpus()/2`, which reads as "leave half the machine for the person using it" —
// true of CPU, false of RAM. Each worker is a full browser (~300-600MB with its renderers),
// so on a 24-core desktop the default asks for 12 of them, ~4-7GB on top of whatever the
// developer already has open. That is a swap-thrash hard-lock, and it gets WORSE on better
// hardware: the same default is 6x more aggressive here than on the 4-core CI runner these
// suites were tuned against.
//
// Ceiling of 4 because past that these suites are disk- and browser-startup-bound, not
// core-bound; floor of 2 so a busy machine still runs them concurrently rather than serially.
const localWorkers = () => Math.max(2, Math.min(4, Math.floor(freemem() / (1.5 * 1024 ** 3))));

// CI gets the full core count (hermetic suites, a dedicated runner, nothing else resident).
const WORKERS = process.env.CI ? cpus().length : localWorkers();

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

export { DESKTOP, DETERMINISM, MOBILE, WORKERS };
