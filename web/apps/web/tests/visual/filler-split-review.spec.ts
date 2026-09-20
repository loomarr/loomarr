import { expect, test } from "@playwright/test";

const story = (id: string) => `/iframe.html?id=${id}&viewMode=story`;

test("split review opens one exact preview from mouse or keyboard", async ({ page }) => {
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
  await expect(clips.first()).toHaveAttribute("aria-current", "true");

  await page.mouse.move(0, 0);
  // Use a real keyboard transition. Base UI intentionally opens a tooltip only for
  // keyboard-visible focus; locator.focus() is programmatic focus and does not establish
  // keyboard modality in Chromium.
  await page.keyboard.press("Tab");
  await expect(clips.nth(1)).toBeFocused();
  await expect(page.getByRole("tooltip").filter({ hasText: "Rotoscoped tech spot" })).toBeVisible();
  await page.keyboard.press("Enter");
  await expect(page.locator("video")).toHaveCount(1);
  await expect(clips.nth(1)).toHaveAttribute("aria-current", "true");

  await page.getByRole("button", { name: "Preview segment #2" }).click();
  await expect(page.locator("video")).toHaveCount(0);
  await expect(page.getByText("Preview unavailable")).toHaveCount(2);

  const rootOverflow = await page.locator("#storybook-root").evaluate((root) => ({
    client: root.clientWidth,
    scroll: root.scrollWidth,
  }));
  expect(rootOverflow.scroll).toBeLessThanOrEqual(rootOverflow.client);
});

test("split naming evidence stays quiet until requested and clears after an edit", async ({ page }) => {
  await page.goto(story("filler-splitrevieweditor--review"));
  const row = page.getByRole("region", { name: "Segment 1: Sunny D — Dude!" });
  const details = row.locator("details").filter({ hasText: "Rename or adjust timing" });

  await expect(details).not.toHaveAttribute("open", "");
  await row.getByText("Rename or adjust timing", { exact: true }).click();
  await expect(details).toHaveAttribute("open", "");
  await expect(row.getByText(/Name suggested from this clip/)).toContainText("SUNNY D — DUDE!");

  await row.getByLabel("Name").fill("Sunny D family commercial");
  // The section's accessible name intentionally follows the edited clip name. Re-resolve the row
  // rather than keeping a locator whose own selector still asks for the old name.
  const editedRow = page.getByRole("region", { name: "Segment 1: Sunny D family commercial" });
  await expect(editedRow.getByText("Name edited during review.")).toBeVisible();
  await expect(editedRow.getByText(/Name suggested from this clip/)).toHaveCount(0);
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
