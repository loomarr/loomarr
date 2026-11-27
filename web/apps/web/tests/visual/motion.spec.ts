import { expect, type Page, test } from "@playwright/test";

// ⚠ THE ONE SUITE THAT RUNS WITH MOTION ON.
//
// `playwright.shared` pins `reducedMotion: "reduce"` for every other spec, and rightly so:
// baselines must never rasterize animating, GPU-dependent pixels (§5.2). But that pin makes
// the whole visual suite structurally BLIND to animation: motion-gated rules do not apply
// under it, and a layer drawn only with motion does not exist.
//
// So a fully green fe-visual run says nothing about whether idle-surface motion works, and
// "zero baseline churn" is equally consistent with the effect never rendering at all. That
// gap cost real time twice while building the guide's empty state: an over-dimmed snow
// (0.11 × 0.6 ≈ 7% opacity) and a scanline sized for the wrong axis both shipped green.
//
// These tests assert BEHAVIOUR, never pixels — computed animation names and frame-to-frame
// difference. Nothing here writes a snapshot, so it cannot flake a diff.
//
// ⚠ `contextOptions`, because Playwright has no `reducedMotion` fixture: a bare
// `test.use({ reducedMotion })` is dropped without a word, and this file once passed only because the
// kit's own setting was dropped the same way (#2004). The first test proves the override lands.
test.use({ contextOptions: { reducedMotion: "no-preference" } });

const story = (id: string) => `/iframe.html?id=${id}&viewMode=story`;

// The computed animation of each of the tuner loader's moving parts, keyed by its `data-motion` hook.
const tunerAnimations = async (page: Page) => {
  await page.goto(story("shell-tunerloader--switch"));
  await page.locator("#storybook-root > *").first().waitFor({ state: "visible" });
  return page.evaluate(() =>
    Array.from(document.querySelectorAll<HTMLElement>("#storybook-root [data-motion]"), (el) => [
      el.dataset.motion,
      getComputedStyle(el).animationName,
    ]),
  );
};

test.describe("idle-surface motion", () => {
  test("the page is told motion is allowed", async ({ page }) => {
    await page.goto(story("shell-idlesnow--default"));
    expect(await page.evaluate(() => matchMedia("(prefers-reduced-motion: reduce)").matches)).toBe(false);
  });

  test("loading indicators animate when motion is enabled", async ({ page }) => {
    await page.goto(story("loomarr-foundations-loading--animated"));
    await page.locator("#storybook-root > *").first().waitFor({ state: "visible" });

    const indicator = page.getByRole("progressbar", { name: "Saving" });
    const firstTransform = await indicator.evaluate((element) => getComputedStyle(element).transform);
    let moved = false;
    for (let i = 0; i < 6 && !moved; i++) {
      await page.waitForTimeout(200);
      moved = (await indicator.evaluate((element) => getComputedStyle(element).transform)) !== firstTransform;
    }
    expect(moved, "the activity indicator never rotated across ~1.2s").toBe(true);
  });

  test("the tuner loader's bars, wash, snow and cursor all animate", async ({ page }) => {
    const animations = new Map((await tunerAnimations(page)) as [string, string][]);
    expect(Object.fromEntries(animations)).toEqual({
      "held-drain": "held-drain",
      "signal-lock": "signal-lock",
      "wash-thicken": "wash-thicken",
      cursor: "onair-pulse",
    });
  });

  // The idle snow draws nothing under reduced motion, so this is the only place it is ever proven
  // to paint.
  test("the idle snow paints and its grain actually moves", async ({ page }) => {
    await page.goto(story("shell-idlesnow--default"));
    const layer = page.locator("#storybook-root [aria-hidden]").first();
    await expect(layer).toBeVisible();

    // Frame-to-frame difference, not a snapshot: the only stable assertion is that it CHANGES.
    //
    // ⚠ **Sampled repeatedly rather than twice, because the flicker is DISCRETE.** The snow holds
    // each offset for ~112ms and then jumps. Two shots 700ms apart normally straddle several steps,
    // but a loaded CI runner can stall both inside ONE, and then identical frames mean "the sampler
    // was starved", not "the animation stopped". Comparing against several later samples makes a
    // pass require only that SOME step boundary was observed, which is the property being claimed.
    // It cannot pass a genuinely frozen animation: every sample would be identical.
    const first = await layer.screenshot();
    let moved = false;
    for (let i = 0; i < 6 && !moved; i++) {
      await page.waitForTimeout(200);
      moved = !first.equals(await layer.screenshot());
    }
    expect(moved, "the snow never changed across ~1.2s — the animation is not running").toBe(true);
  });
});

test.describe("reduced motion", () => {
  test.use({ contextOptions: { reducedMotion: "reduce" } });

  test("the tuner loader holds still", async ({ page }) => {
    const animations = await tunerAnimations(page);
    expect(animations.length).toBeGreaterThan(0);
    expect(animations.filter(([, name]) => name !== "none")).toEqual([]);
  });
});
