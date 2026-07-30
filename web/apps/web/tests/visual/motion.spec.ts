import { expect, test } from "@playwright/test";

// ⚠ THE ONE SUITE THAT RUNS WITH MOTION ON.
//
// `playwright.shared` pins `reducedMotion: "reduce"` for every other spec, and rightly so:
// baselines must never rasterize animating, GPU-dependent pixels (§5.2). But that pin makes
// the whole visual suite structurally BLIND to animation — `motion-safe:` utilities compile
// to nothing under it, and a `hidden motion-safe:block` layer is simply display:none.
//
// So a fully green fe-visual run says nothing about whether idle-surface motion works, and
// "zero baseline churn" is equally consistent with the effect never rendering at all. That
// gap cost real time twice while building the guide's empty state: an over-dimmed TvStatic
// (0.11 × 0.6 ≈ 7% opacity) and a scanline sized for the wrong axis both shipped green.
//
// These tests assert BEHAVIOUR, never pixels — computed animation names and frame-to-frame
// difference. Nothing here writes a snapshot, so it cannot flake a diff.
test.use({ reducedMotion: "no-preference" });

const story = (id: string) => `/iframe.html?id=${id}&viewMode=story`;

test.describe("idle-surface motion", () => {
  // The guide's "Dead air" test card (§1). Each segment breathes out of phase, so the shimmer
  // travels across the card rather than the strip pulsing as one block.
  test("ColorBars breathe, and the segments are staggered", async ({ page }) => {
    await page.goto(story("shell-colorbars--breathing"));
    await page.locator("#storybook-root > *").first().waitFor({ state: "visible" });

    const segments = await page.evaluate(() =>
      Array.from(document.querySelectorAll("#storybook-root [aria-hidden] > span")).map((el) => {
        const cs = getComputedStyle(el as HTMLElement);
        return { name: cs.animationName, delay: cs.animationDelay };
      }),
    );

    expect(segments.length).toBeGreaterThan(1);
    // Every segment animates…
    expect(segments.every((s) => s.name === "bar-breathe")).toBe(true);
    // …and no two share a delay. A single shared delay would make this a pulse, which reads
    // as a loading spinner rather than a live signal — the whole point of the stagger.
    expect(new Set(segments.map((s) => s.delay)).size).toBe(segments.length);
  });

  // The `breathe` prop is opt-in, and staying opt-in is the design decision worth pinning: a
  // brand mark that pulses on every screen is noise, and beside real data it reads as a status.
  test("ColorBars are still unless asked to breathe", async ({ page }) => {
    await page.goto(story("shell-colorbars--hero"));
    await page.locator("#storybook-root > *").first().waitFor({ state: "visible" });

    const names = await page.evaluate(() =>
      Array.from(document.querySelectorAll("#storybook-root [aria-hidden] > span")).map(
        (el) => getComputedStyle(el as HTMLElement).animationName,
      ),
    );
    expect(names.length).toBeGreaterThan(1);
    expect(names.every((n) => n === "none")).toBe(true);
  });

  // TvStatic is `hidden motion-safe:block`, so under the pinned suite it does not exist at
  // all. This is the only place its snow is ever proven to paint.
  test("TvStatic paints and its grain actually moves", async ({ page }) => {
    await page.goto(story("shell-tvstatic--default"));
    await page.locator("#storybook-root > *").first().waitFor({ state: "visible" });

    const layer = page.locator("#storybook-root [aria-hidden]").first();
    await expect(layer).toBeVisible();

    // Frame-to-frame difference, not a snapshot: the grain is deliberately random, so the
    // only stable assertion is that it CHANGES.
    //
    // ⚠ **Sampled repeatedly rather than twice, because the animation is DISCRETE.**
    // `--animate-tv-snow` is `tv-snow 0.45s steps(1, end)` over five keyframe stops, so the
    // transform changes every ~112ms and holds still in between. Two shots 700ms apart
    // normally straddle several steps — but a loaded CI runner can stall both inside ONE,
    // and then identical frames mean "the sampler was starved", not "the animation stopped".
    //
    // That is exactly how this failed on a React 19 bump that had nothing to do with it:
    // all three retries red in CI, 574/574 green locally on the same commit and image.
    // Comparing against several later samples makes a pass require only that SOME step
    // boundary was observed, which is the property actually being claimed. It cannot pass a
    // genuinely frozen animation — every sample would be identical.
    const first = await layer.screenshot();
    let moved = false;
    for (let i = 0; i < 6 && !moved; i++) {
      await page.waitForTimeout(200);
      moved = !first.equals(await layer.screenshot());
    }
    expect(moved, "the snow never changed across ~1.2s — the animation is not running").toBe(true);
  });
});
