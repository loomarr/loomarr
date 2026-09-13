import { expect, type Page, test } from "@playwright/test";

const expectNoHorizontalOverflow = async (page: Page) => {
  const metrics = await page.evaluate(() => ({
    viewportWidth: window.innerWidth,
    documentWidth: document.documentElement.scrollWidth,
    rootWidth: document.querySelector<HTMLElement>("#storybook-root")?.scrollWidth ?? 0,
  }));
  expect(metrics.documentWidth).toBeLessThanOrEqual(metrics.viewportWidth);
  expect(metrics.rootWidth).toBeLessThanOrEqual(metrics.viewportWidth);
};

test("filler recovery actions stay complete and usable at desktop and mobile widths", async ({ page }) => {
  await page.goto("/iframe.html?id=filler-manage--recoverable-diagnostics&viewMode=story");
  await page.locator("#storybook-root > *").first().waitFor({ state: "visible" });

  await expect(page.getByRole("link", { name: "Check AI connection" })).toHaveAttribute(
    "href",
    "/settings/ai",
  );
  await expect(page.getByRole("link", { name: "Review limits" })).toHaveAttribute("href", "/filler/settings");
  await expect(page.getByRole("link", { name: "Open clip" })).toHaveAttribute(
    "href",
    `/v1/filler/media/${"a".repeat(64)}`,
  );
  await expect(page.getByRole("button", { name: "Try again" })).toBeVisible();
  await expect(page.getByText(/bounded|per-pass|dependency suffix|operational holds/i)).toHaveCount(0);

  const automatic = page.locator('[aria-labelledby="diagnostic-hold-3"]');
  await expect(automatic.getByText("Trying again", { exact: true })).toBeVisible();
  await expect(automatic.getByRole("button", { name: "Try again" })).toHaveCount(0);

  await expectNoHorizontalOverflow(page);
});

test("a failed manual retry stays held and explains the failure", async ({ page }) => {
  await page.goto("/iframe.html?id=filler-manage--manual-retry-failure&viewMode=story");
  await expect(page.getByRole("alert")).toContainText("This clip is still on hold");
  await expect(page.getByRole("button", { name: "Try again" })).toBeVisible();
  await expectNoHorizontalOverflow(page);
});

test("a completed retry refreshes to the server-owned recovered state", async ({ page }) => {
  await page.goto("/iframe.html?id=filler-manage--recovered-after-retry&viewMode=story");
  await expect(page.getByText("Everything is working. Nothing needs your attention.")).toBeVisible();
  await expect(page.getByRole("button", { name: "Try again" })).toHaveCount(0);
  await expectNoHorizontalOverflow(page);
});
