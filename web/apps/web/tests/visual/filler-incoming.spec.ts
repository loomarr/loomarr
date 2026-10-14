import { expect, test } from "@playwright/test";

test("large Incoming groups stay bounded and focused at mobile width", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "mobile", "mobile Incoming layout contract");

  await page.goto("/iframe.html?id=filler-incoming--twenty-plus-clips&viewMode=story");
  await page.locator("#storybook-root > *").first().waitFor({ state: "visible" });
  await expect(page.getByRole("button", { name: /^View details for / })).toHaveCount(20);
  await expect(page.getByRole("button", { name: "Show more clips being prepared" })).toBeVisible();

  const metrics = await page.evaluate(() => ({
    viewportWidth: window.innerWidth,
    documentWidth: document.documentElement.scrollWidth,
    rootWidth: document.querySelector<HTMLElement>("#storybook-root")?.scrollWidth ?? 0,
  }));
  expect(metrics.documentWidth).toBeLessThanOrEqual(metrics.viewportWidth);
  expect(metrics.rootWidth).toBeLessThanOrEqual(metrics.viewportWidth);

  await page.goto("/iframe.html?id=filler-incoming--needs-help&viewMode=story");
  await page.locator("#storybook-root > *").first().waitFor({ state: "visible" });
  await expect(page.getByRole("link", { name: "Review clips" })).toHaveCount(1);
  await expect(page.getByText("1 of 3")).toBeVisible();
  await expect(page.getByRole("button", { name: "Next" })).toBeVisible();
});
