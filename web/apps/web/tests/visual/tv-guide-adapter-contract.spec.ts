import { expect, test } from "@playwright/test";

test("TV Guide D-pad crosses grid and enabled filters without stranding focus", async ({ page }) => {
  await page.goto("/iframe.html?id=loomarr-components-tv-guide-adapter--remote&viewMode=story");

  const controller = page.getByRole("application", { name: "TV Guide remote controller" });
  await expect(controller).toBeFocused();

  await page.keyboard.press("ArrowDown");
  await expect(page.getByText("ch-springfield focused")).toBeVisible();
  await page.keyboard.press("ArrowUp");
  await page.keyboard.press("ArrowUp");
  await expect(page.getByText("all filter focused")).toBeVisible();

  await page.keyboard.press("ArrowRight");
  await expect(page.getByText("recent filter focused")).toBeVisible();
  await page.keyboard.press("Enter");
  await expect(page.getByText("recent filter applied")).toBeVisible();
  await expect(page.getByRole("button", { name: "Recent channels" })).toHaveAttribute("aria-pressed", "true");

  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("Enter");
  await expect(page.getByText("ch-action tuned")).toBeVisible();
});
