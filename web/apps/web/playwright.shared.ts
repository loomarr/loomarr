import { cpus, freemem } from "node:os";
import { devices } from "@playwright/test";

// How many workers a LOCAL run may use.
//
// ⚠ Bound by MEMORY, not cores, because memory is what actually runs out. Playwright's
// default is `cpus()/2`, which reads as "leave half the machine for the person using it" —
// true of CPU, false of RAM. Each worker is a full browser (~300-600MB with its renderers),
// so on a 24-core desktop the default asks for 12 of them, ~4-7GB on top of whatever the
// developer already has open. That is a swap-thrash hard-lock, and it gets WORSE on better
// hardware: the same default is 6x more aggressive there than on the 4-core CI runner these
// suites were tuned against.
//
// Ceiling of 4 because past that these suites are disk- and browser-startup-bound, not
// core-bound. The floor is deliberately one: forcing two browsers while the machine has less
// than one worker's memory allowance defeats this guard and can freeze a no-swap workstation.
const localWorkerCount = (freeBytes: number) =>
  Math.max(1, Math.min(4, Math.floor(freeBytes / (1.5 * 1024 ** 3))));
const localWorkers = () => localWorkerCount(freemem());

// ⚠ GATE ON `GITHUB_ACTIONS`, NOT ON `CI`. This is the whole point of the constant.
//
// Every sanctioned Playwright entry point is a make target that runs Docker with
// `-e CI=$(PW_CI)` and `PW_CI ?= 1` — fe-visual, fe-visual-update, e2e, e2e-update. So `CI` is
// set on a DEVELOPER'S MACHINE just as it is on the runner: it records the config intent
// (behave like CI), not whose hardware this is. Gating the worker count on it therefore hands
// a 24-core workstation 24 browsers under `make fe-visual-update`, which is exactly the
// swap-thrash hard-lock this constant exists to prevent — measured going 16GB free to 2GB with
// swap fully consumed, in about a minute.
//
// `GITHUB_ACTIONS` is set by the runner itself and is never forwarded by those make targets,
// so it distinguishes the MACHINE. That is the question being asked here.
//
// ⚠ The two suites still do not share a concurrency profile, only a determinism kit. VISUAL is
// hermetic (static storybook server, stubbed network, no shared state), so the real runner can
// take the full core count. E2E's CI concurrency is deliberately left at Playwright's own
// default rather than retuned by us — `undefined` means "do not set this key".
const onRealCI = !!process.env.GITHUB_ACTIONS;
const VISUAL_WORKERS = onRealCI ? cpus().length : localWorkers();
const E2E_WORKERS = onRealCI ? undefined : localWorkers();

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

export { DESKTOP, DETERMINISM, E2E_WORKERS, localWorkerCount, MOBILE, VISUAL_WORKERS };
