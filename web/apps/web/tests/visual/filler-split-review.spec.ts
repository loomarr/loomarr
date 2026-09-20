import { expect, test } from "@playwright/test";

const story = (id: string) => `/iframe.html?id=${id}&viewMode=story`;

test("split review opens one exact Sheet preview from mouse or keyboard", async ({ page }) => {
  await page.goto(story("filler-splitrevieweditor--review"));
  const timeline = page.getByRole("list", { name: "Detected clips, in order" });
  const clips = timeline.getByRole("button");
  await expect(clips).toHaveCount(4);
  await expect(clips.first().getByText("Sunny D — Dude!", { exact: true })).toBeVisible();

  const timelineBounds = await timeline.boundingBox();
  const firstClipBounds = await clips.first().boundingBox();
  expect(timelineBounds).not.toBeNull();
  expect(firstClipBounds).not.toBeNull();
  expect(firstClipBounds?.height).toBeLessThanOrEqual(timelineBounds?.height ?? 0);

  await clips.first().hover();
  await expect(page.getByRole("tooltip").filter({ hasText: "Sunny D — Dude!" })).toBeVisible();

  await clips.first().click();
  await expect(page.locator("video")).toHaveCount(1);
  await expect(page.locator('button[aria-label="00:00 · Sunny D — Dude!"]')).toHaveAttribute(
    "aria-current",
    "true",
  );
  await expect(page.getByRole("dialog", { name: "Sunny D — Dude!" })).toBeVisible();

  await page.getByRole("button", { name: "Close" }).click();
  await expect(page.locator("video")).toHaveCount(0);
  await expect(clips.first()).toBeFocused();

  await page.mouse.move(0, 0);
  // Use a real keyboard transition. Base UI intentionally opens a tooltip only for
  // keyboard-visible focus; locator.focus() is programmatic focus and does not establish
  // keyboard modality in Chromium.
  await page.keyboard.press("Tab");
  await expect(clips.nth(1)).toBeFocused();
  await expect(page.getByRole("tooltip").filter({ hasText: "Rotoscoped tech spot" })).toBeVisible();
  await page.keyboard.press("Enter");
  await expect(page.locator("video")).toHaveCount(1);
  await expect(page.locator('button[aria-label="00:30 · Rotoscoped tech spot"]')).toHaveAttribute(
    "aria-current",
    "true",
  );
  await expect(page.getByRole("dialog", { name: "Rotoscoped tech spot" })).toBeVisible();

  await page.getByRole("button", { name: "Close" }).click();
  await expect(page.locator("video")).toHaveCount(0);

  const rootOverflow = await page.locator("#storybook-root").evaluate((root) => ({
    client: root.clientWidth,
    scroll: root.scrollWidth,
  }));
  expect(rootOverflow.scroll).toBeLessThanOrEqual(rootOverflow.client);
});

test("split naming evidence stays quiet until requested and clears after an edit", async ({ page }) => {
  await page.goto(story("filler-splitrevieweditor--review"));
  await page.getByRole("button", { name: "Open clip 1: Sunny D — Dude!" }).click();
  const row = page.getByRole("region", { name: "Clip 1: Sunny D — Dude!" });
  const editFields = row.locator("details").filter({ hasText: "Rename or adjust timing" });
  const namingEvidence = row.getByText(/Name suggested from this clip/);

  await expect(namingEvidence).toBeHidden();
  await row.getByText("Details", { exact: true }).click();
  await expect(namingEvidence).toContainText("SUNNY D — DUDE!");
  await expect(editFields).not.toHaveAttribute("open", "");
  await row.getByText("Rename or adjust timing", { exact: true }).click();
  await expect(editFields).toHaveAttribute("open", "");

  await row.getByLabel("Name").fill("Sunny D family commercial");
  // The section's accessible name intentionally follows the edited clip name. Re-resolve the row
  // rather than keeping a locator whose own selector still asks for the old name.
  const editedRow = page.getByRole("region", { name: "Clip 1: Sunny D family commercial" });
  await expect(editedRow.getByText("Name edited during review.")).toBeVisible();
  await expect(editedRow.getByText(/Name suggested from this clip/)).toHaveCount(0);
});

test("a 50-clip review stays compact and usable on a phone", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(story("filler-splitrevieweditor--fifty-clips"));

  const list = page.getByRole("list", { name: "Clips in this recording" });
  await expect(list.getByRole("listitem")).toHaveCount(50);
  await expect(page.locator("video")).toHaveCount(0);

  await page.getByRole("button", { name: "Open clip 24: Commercial 24" }).click();
  await expect(page.getByRole("dialog", { name: "Commercial 24" })).toBeVisible();
  await expect(page.locator("video")).toHaveCount(1);
  await page.getByRole("button", { name: "Next clip" }).click();
  await expect(page.getByRole("dialog", { name: "Commercial 25" })).toBeVisible();
  await expect(page.locator("video")).toHaveCount(1);
  await expect(page.locator('button[aria-label="12:00 · Commercial 25"]')).toHaveAttribute(
    "aria-current",
    "true",
  );
  await expect(page.locator('button[aria-label="Open clip 25: Commercial 25"]')).toHaveAttribute(
    "aria-current",
    "true",
  );

  await page.getByRole("button", { name: "Previous clip" }).focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("dialog", { name: "Commercial 24" })).toBeVisible();
  await expect(page.locator('button[aria-label="11:30 · Commercial 24"]')).toHaveAttribute(
    "aria-current",
    "true",
  );

  const viewportOverflow = await page.locator("html").evaluate((root) => ({
    client: root.clientWidth,
    scroll: root.scrollWidth,
  }));
  const overflowSources = await page.locator("body *").evaluateAll((elements) =>
    elements
      .map((element) => {
        const rect = element.getBoundingClientRect();
        return {
          tag: element.tagName,
          className: element.getAttribute("class") ?? "",
          left: Math.round(rect.left),
          right: Math.round(rect.right),
          scrollWidth: element.scrollWidth,
          clientWidth: element.clientWidth,
        };
      })
      .filter((element) => element.right > document.documentElement.clientWidth + 1)
      .slice(0, 8),
  );
  expect(viewportOverflow.scroll, JSON.stringify(overflowSources)).toBeLessThanOrEqual(
    viewportOverflow.client,
  );
});

test("split review reflows at a 200% zoom-equivalent width", async ({ page }) => {
  // Browser zoom halves the CSS viewport. A 320px CSS viewport is the standards-equivalent
  // reflow check for a 640px-wide window at 200%, without relying on browser-specific zoom APIs.
  await page.setViewportSize({ width: 320, height: 800 });
  await page.goto(story("filler-splitrevieweditor--fifty-clips"));
  await page.getByRole("button", { name: "Open clip 13:", exact: false }).click();
  await expect(page.getByRole("dialog")).toBeVisible();

  const viewport = await page.locator("html").evaluate((root) => ({
    client: root.clientWidth,
    scroll: root.scrollWidth,
  }));
  expect(viewport.scroll).toBeLessThanOrEqual(viewport.client);
});

test("a 50-clip timeline scrolls without starting 50 streams", async ({ page }) => {
  await page.goto(story("filler-segmentfilmstrip--long-reel"));
  const timeline = page.getByRole("list", { name: "Detected clips, in order" });
  await expect(timeline.getByRole("button")).toHaveCount(50);
  await expect(page.locator("video")).toHaveCount(0);

  const overflow = await page.getByLabel("Detected clip timeline").evaluate((element) => ({
    client: element.clientWidth,
    scroll: element.scrollWidth,
  }));
  expect(overflow.scroll).toBeGreaterThan(overflow.client);
});

test("a media failure becomes a useful state and releases the player", async ({ page }) => {
  await page.goto(story("filler-segmentpreview--media-unavailable"));
  await expect(page.getByRole("alert")).toContainText("This preview couldn’t be played");
  await expect(page.locator("video")).toHaveCount(0);
});
