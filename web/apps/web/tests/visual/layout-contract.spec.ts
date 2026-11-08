import { expect, test } from "@playwright/test";

test("adaptive layout and disclosure retain browser semantics", async ({ page }) => {
  await page.goto("/iframe.html?id=loomarr-foundations-layout--pointer&viewMode=story");

  const adaptiveLayout = page.getByTestId("adaptive-layout");
  const disclosure = page.getByRole("button", { name: /Episode information/ });
  await expect(adaptiveLayout).toBeVisible();
  await expect(disclosure).toHaveAttribute("aria-expanded", "false");
  // Collapsed, the body stays in the page for find-in-page but is hidden until opened.
  await expect(page.getByText(/Season 4 · Episode 12/)).toBeHidden();

  await disclosure.click();
  await expect(disclosure).toHaveAttribute("aria-expanded", "true");
  await expect(page.getByText(/Season 4 · Episode 12/)).toBeVisible();

  const direction = await adaptiveLayout.evaluate((element) => getComputedStyle(element).flexDirection);
  expect(direction).toBe(test.info().project.name === "mobile" ? "column" : "row");
});
