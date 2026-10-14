import { expect, test } from "@playwright/test";

test("the loading indicator is horizontally centered in its feedback panel", async ({ page }) => {
  await page.goto("/iframe.html?id=loomarr-components-feedback-and-recovery--loading&viewMode=story");

  const panel = page.getByRole("status");
  const indicator = page.getByRole("progressbar", { name: "Loading channels" });
  await expect(panel).toBeVisible();
  await expect(indicator).toBeVisible();

  const panelBox = await panel.boundingBox();
  const indicatorBox = await indicator.boundingBox();
  expect(panelBox).not.toBeNull();
  expect(indicatorBox).not.toBeNull();

  const panelCenter = panelBox!.x + panelBox!.width / 2;
  const indicatorCenter = indicatorBox!.x + indicatorBox!.width / 2;
  expect(Math.abs(panelCenter - indicatorCenter)).toBeLessThanOrEqual(1);
});
