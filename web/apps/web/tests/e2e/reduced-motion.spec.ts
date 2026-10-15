import { expect, test } from "@playwright/test";
import { installMockBackend } from "./mock-backend";

// The shared determinism kit pins reduced motion for this suite (playwright.shared). The setting was
// once dropped without a word, because Playwright reads it only from `contextOptions`, and every page
// snapshot ran with motion on (#2004). Only the browser can say what it received, so ask it.
test("the flow suite runs with reduced motion", async ({ page }) => {
  await installMockBackend(page);
  await page.goto("/wizard");
  await expect(page.getByRole("heading", { name: /create your admin account/i })).toBeVisible();
  expect(await page.evaluate(() => matchMedia("(prefers-reduced-motion: reduce)").matches)).toBe(true);
});
