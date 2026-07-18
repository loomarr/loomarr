import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

// Drive every story from the built Storybook index: a pixel snapshot + an axe sweep, at
// each project's viewport (frontend-design §5). Build storybook-static first —
// `make fe-visual` does `build-storybook` → `playwright test`.
const here = dirname(fileURLToPath(import.meta.url));
const indexPath = join(here, "..", "..", "storybook-static", "index.json");

interface StoryEntry {
  id: string;
  title: string;
  name: string;
  type: string;
}

const index = JSON.parse(readFileSync(indexPath, "utf8")) as { entries: Record<string, StoryEntry> };
const stories = Object.values(index.entries).filter((entry) => entry.type === "story");

for (const story of stories) {
  test(`${story.title} — ${story.name}`, async ({ page }) => {
    await page.goto(`/iframe.html?id=${story.id}&viewMode=story`);
    // Wait for the story to actually render content (not just the root to attach), then
    // for self-hosted Geist to finish loading — otherwise a screenshot can race a
    // fallback-font paint and drift at the strict 0.001 threshold (§5.2).
    await page.locator("#storybook-root > *").first().waitFor({ state: "visible" });
    await page.evaluate(async () => {
      await document.fonts.ready;
    });

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
    await expect(page.locator("#storybook-root")).toHaveScreenshot(`${story.id}.png`);

    // Accessibility (§5.3): zero serious/critical violations. The region/landmark rule
    // (moderate) is expected on bare component stories, so only serious+ is blocking.
    const results = await new AxeBuilder({ page }).include("#storybook-root").analyze();
    const blocking = results.violations.filter(
      (violation) => violation.impact === "serious" || violation.impact === "critical",
    );
    expect(blocking, `axe: ${blocking.map((violation) => violation.id).join(", ")}`).toEqual([]);
  });
}
