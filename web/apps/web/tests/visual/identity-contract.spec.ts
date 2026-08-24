import { expect, test } from "@playwright/test";

test("media identity keeps complete and fallback content inside the viewport", async ({ page }) => {
  await page.goto("/iframe.html?id=loomarr-components-media-identity--missing-logo&viewMode=story");

  const story = page.locator("#storybook-root");
  await expect(story.getByText("Classic Animation", { exact: true })).toBeVisible();
  await expect(story.getByText("CA", { exact: true })).toBeVisible();
  await expect(story.getByText("Marge vs. the Monorail", { exact: true })).toBeVisible();
  await expect(story.getByText("7:00–7:30 PM · S04E12", { exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(
    true,
  );
});
