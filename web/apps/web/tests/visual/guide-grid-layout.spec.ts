import { expect, test } from "@playwright/test";

test("the default four-channel Guide fits a 1440 by 900 desktop", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/iframe.html?id=guide-guidegrid--desktop-four-channels&viewMode=story");

  const grid = page.getByTestId("guide-grid");
  await expect(grid).toBeVisible();
  await expect(page.getByText("1980s Action Heroes", { exact: true })).toBeVisible();
  await expect(page.getByText("Saturday Night Comedy", { exact: true })).toBeVisible();

  const widths = await grid.evaluate((element) => ({
    client: element.clientWidth,
    scroll: element.scrollWidth,
  }));
  expect(widths.scroll).toBeLessThanOrEqual(widths.client);

  await expect(grid).toHaveScreenshot("guide-four-channel-default-1440x900.png");
});
