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
  await expect(page.getByText("Click to play this exact clip.")).toBeVisible();

  await clips.first().click();
  await expect(page.locator("video")).toHaveCount(1);
  await expect(clips.first()).toHaveAttribute("aria-current", "true");

  await page.mouse.move(0, 0);
  await clips.nth(1).focus();
  await expect(page.getByText("Click to play this exact clip.")).toBeVisible();
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
