import { expect, test } from "@playwright/test";

test("client destinations remain keyboard reachable and publish selection", async ({ page }) => {
  await page.goto("/iframe.html?id=loomarr-components-client-navigation--pointer&viewMode=story");

  const watching = page.getByRole("button", { name: "Watching" });
  const guide = page.getByRole("button", { name: "Guide" });
  const surf = page.getByRole("button", { name: "Surf" });
  await expect(page.getByRole("navigation", { name: "Primary navigation" })).toBeVisible();
  await expect(guide).toHaveAttribute("aria-pressed", "true");

  await watching.focus();
  await page.keyboard.press("Tab");
  await expect(guide).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(surf).toBeFocused();
  await surf.press("Enter");

  await expect(surf).toHaveAttribute("aria-pressed", "true");
  await expect(guide).not.toHaveAttribute("aria-pressed", "true");
  await expect(page.getByText("Surf", { exact: true }).first()).toBeVisible();
});
