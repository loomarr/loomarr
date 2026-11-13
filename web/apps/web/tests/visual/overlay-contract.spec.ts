import { expect, test } from "@playwright/test";

test("modal overlay traps focus, dismisses with Escape, and returns focus", async ({ page }) => {
  await page.goto("/iframe.html?id=loomarr-components-overlay--interactive-confirmation&viewMode=story");

  const trigger = page.getByRole("button", { name: "Open overlay" });
  await trigger.focus();
  await trigger.press("Enter");

  const dialog = page.getByRole("dialog", { name: "Return to the device home screen?" });
  await expect(dialog).toBeVisible();
  await expect(page.getByRole("button", { name: "Keep watching" })).toBeFocused();
  await page.keyboard.press("Shift+Tab");
  await expect(page.getByRole("button", { name: "Leave playback" })).toBeFocused();

  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(trigger).toBeFocused();

  await trigger.press("Enter");
  await expect(dialog).toBeVisible();
  await page.mouse.click(5, 5);
  await expect(dialog).toHaveCount(0);
  await expect(trigger).toBeFocused();
});

test("transient overlay auto-dismisses after its bounded visibility period", async ({ page }) => {
  await page.goto("/iframe.html?id=loomarr-components-overlay--auto-dismiss&viewMode=story");
  const title = page.getByText("Now playing", { exact: true });
  await expect(title).toBeVisible();
  await expect(title).toHaveCount(0, { timeout: 2_000 });
});

test("transient overlay pauses auto-dismiss while its controls have focus", async ({ page }) => {
  await page.goto("/iframe.html?id=loomarr-components-overlay--auto-dismiss&viewMode=story");
  const title = page.getByText("Now playing", { exact: true });
  const action = page.getByRole("button", { name: "View Guide" });
  await action.focus();
  await page.waitForTimeout(700);
  await expect(title).toBeVisible();

  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
  await expect(title).toHaveCount(0, { timeout: 2_000 });
});
