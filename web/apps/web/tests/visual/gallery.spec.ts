import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import AxeBuilder from "@axe-core/playwright";
import { expect, type Page, test } from "@playwright/test";

// Drive every story from the built Storybook index: a pixel snapshot + an axe sweep, at
// each project's viewport (frontend-design §5). Build storybook-static first —
// `make fe-visual` does `build-storybook` → `playwright test`.
const here = dirname(fileURLToPath(import.meta.url));
const indexPath = join(here, "..", "..", "storybook-static", "index.json");

// The slice of Storybook's preview API this file depends on. Typed here rather than pulled
// from a Storybook package: it is one field read through `page.waitForFunction`, and the
// import would have to resolve inside the browser context, not this Node process.
interface StorybookPreviewWindow {
  __STORYBOOK_PREVIEW__?: { storyRenders?: { id: string; phase?: string }[] };
}

interface StoryEntry {
  id: string;
  title: string;
  name: string;
  tags?: string[];
  type: string;
}

const index = JSON.parse(readFileSync(indexPath, "utf8")) as { entries: Record<string, StoryEntry> };
// JS-driven React Native animations cannot be frozen by Playwright's CSS animation switch.
// Their dedicated stories are exercised by motion.spec.ts through frame-to-frame behavior;
// pixel snapshots cover the same markup in its explicit reduced-motion state.
const stories = Object.values(index.entries).filter(
  (entry) => entry.type === "story" && !entry.tags?.includes("motion-only"),
);

// axe-core keeps a module-global "running" flag per frame, and its runPartialRecursive
// walk can re-enter it — so an analyze() occasionally rejects with "Axe is already
// running" even though this test owns its page. It surfaced on roughly one story per run,
// a DIFFERENT story each time, which is how we know it is a race rather than a real
// violation. Playwright's retry masked it as "1 flaky" and the suite stayed green, but a
// gate that cries wolf on every run stops being read.
//
// Retry ONLY on that specific message: a genuine violation still fails on the first pass.
const analyzeWhenFree = async (page: Page) => {
  for (let attempt = 0; ; attempt++) {
    try {
      return await new AxeBuilder({ page }).include("#storybook-root").analyze();
    } catch (err) {
      const busy = err instanceof Error && err.message.includes("Axe is already running");
      if (!busy || attempt >= 3) throw err;
      await page.waitForTimeout(100 * (attempt + 1));
    }
  }
};

// ⚠ Every story sees the same "now". Fixtures carry absolute timestamps and the UI prints them
// relative to the clock ("checked 89d ago"), so on a live clock a baseline goes stale the day a
// count rolls over: FillerSources/WithRegisteredSources flipped to "90d ago" on 2026-10-27 and
// failed every queue run. This is a moment when every committed baseline was green.
//
// The clock STARTS here and keeps running (`setSystemTime`), not `setFixedTime`: a frozen Date
// stops anything that measures elapsed time with it, and the brand-launch intro and the guide's
// keyboard story then render their first frame instead of their settled one.
const GALLERY_NOW = new Date("2026-10-27T12:00:00Z");

// A story whose meta carries `tags: ["route"]` renders a whole page or shell; the phone project
// shoots only those (playwright.config.ts).
//
// Every story is shot under the shared kit's reduced motion, except one tagged `motion`: a story that
// exists to show motion (the idle snow behind the login and setup surfaces), which reduced motion
// would hide. It gets `no-preference` before it loads, because TvStatic and the design system read
// the preference once, at mount. A tag rather than a story parameter, because index.json carries
// tags and a parameter is only readable after the story has rendered under the wrong preference.
// The style tag below still freezes its CSS animation at frame 0, so the story must be still in
// every other way (a seeded tile, no JS-driven loop) — docs/contributing/testing.md.
for (const story of stories) {
  const showsMotion = story.tags?.includes("motion") ?? false;
  const tag = [...(story.tags?.includes("route") ? ["@route"] : []), ...(showsMotion ? ["@motion"] : [])];
  test(`${story.title} — ${story.name}`, { tag }, async ({ page }) => {
    if (showsMotion) await page.emulateMedia({ reducedMotion: "no-preference" });
    await page.clock.setSystemTime(GALLERY_NOW);
    await page.goto(`/iframe.html?id=${story.id}&viewMode=story`);
    // The preference this story is shot under, read in the page itself: the kit's setting was once
    // dropped without a word (#2004), and only the browser can say what it received.
    expect(
      await page.evaluate(() => matchMedia("(prefers-reduced-motion: reduce)").matches),
      showsMotion
        ? "a `motion` story must load with motion allowed"
        : "the gallery must load with reduced motion",
    ).toBe(!showsMotion);
    // Wait for the story to actually render content (not just the root to attach), then
    // for self-hosted Geist to finish loading — otherwise a screenshot can race a
    // fallback-font paint and drift at the strict 0.001 threshold (§5.2).
    await page.locator("#storybook-root > *").first().waitFor({ state: "visible" });

    // ⚠ WAIT FOR THE PLAY FUNCTION, or a story with one is shot MID-INTERACTION.
    //
    // "First child visible" happens on mount, long before an async `play()` has finished
    // typing and clicking, so the screenshot raced it. This is not theoretical: it produced
    // ChannelLineupEditor/AddingATitle at 340px most runs and 416px intermittently, against
    // a 306px baseline, which read like a mystery flake for weeks.
    //
    // `toHaveScreenshot`'s own stability check does NOT save us. It shoots until two
    // consecutive frames match, and `play()` pauses between keystrokes — so two frames
    // taken inside one of those pauses agree, and it settles on an intermediate state.
    //
    // Storybook tracks the phase per render (`playing` → `finished`); a story with no
    // `play()` reaches a terminal phase immediately, so this costs those nothing. Wait for
    // every terminal phase so a broken play function fails with its actual phase rather
    // than an unrelated timeout, then require success before treating pixels as evidence.
    const phase = await page
      .waitForFunction(
        (id) => {
          const renders = (window as unknown as StorybookPreviewWindow).__STORYBOOK_PREVIEW__?.storyRenders;
          const phase = renders?.find((r) => r.id === id)?.phase;
          return phase === "finished" || phase === "errored" || phase === "aborted" ? phase : undefined;
        },
        story.id,
        { timeout: 15_000 },
      )
      .then((handle) => handle.jsonValue());
    expect(phase, `Storybook interaction for ${story.id} must finish successfully`).toBe("finished");

    await page.evaluate(async () => {
      await document.fonts.ready;
    });
    // TvStatic's snow is a tile rendered after the first frame (the content paints first), so a
    // shown snow layer is still `data-snow-pending` for a moment after mount — the same race as
    // fonts. An idle layer hidden by reduced motion never renders one, so only visible ones count;
    // a `motion` story shows its idle snow and waits here.
    await page.waitForFunction(() =>
      Array.from(document.querySelectorAll("[data-snow-pending]")).every((el) => !el.checkVisibility()),
    );

    // Kill animations/transitions/caret OUTRIGHT — reduced-motion only fast-forwards
    // (`duration: 0.001ms`), which leaves an infinite spinner frozen at a random frame
    // and flakes the diff. `animation: none` freezes it at its initial state (§5.2).
    await page.addStyleTag({
      content:
        "*, *::before, *::after { animation: none !important; transition: none !important; caret-color: transparent !important; }",
    });

    // Visual regression (§5.1): snapshot the component element itself, not the full
    // centered page — a fullPage shot re-centers with fractional margins run-to-run,
    // shifting text AA and flaking the 0.001 diff. Committed baseline, 0.001 diff ratio.
    // Portal stories render their actual module beside #storybook-root. React Native Web's Modal
    // gives that portal one dialog root spanning the viewport, so target it directly rather than
    // broadening every story back to the fractional full-page screenshots avoided above.
    const portalStory = story.tags?.includes("portal");
    const snapshotTarget = portalStory ? page.getByRole("dialog") : page.locator("#storybook-root");
    await expect(snapshotTarget).toHaveScreenshot(`${story.id}.png`);

    // Accessibility (§5.3): zero serious/critical violations. The region/landmark rule
    // (moderate) is expected on bare component stories, so only serious+ is blocking.
    const results = portalStory
      ? await new AxeBuilder({ page }).include('[role="dialog"]').analyze()
      : await analyzeWhenFree(page);
    const blocking = results.violations.filter(
      (violation) => violation.impact === "serious" || violation.impact === "critical",
    );
    expect(blocking, `axe: ${blocking.map((violation) => violation.id).join(", ")}`).toEqual([]);
  });
}
