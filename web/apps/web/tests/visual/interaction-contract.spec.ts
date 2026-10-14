import { expect, test } from "@playwright/test";

test("shared controls publish and update their browser interaction state", async ({ page }) => {
  await page.goto("/iframe.html?id=loomarr-foundations-interaction--pointer&viewMode=story");

  const artwork = page.getByRole("checkbox", { name: "Episode artwork" });
  const systemTheme = page.getByRole("switch", { name: "Follow system theme" });
  const compact = page.getByRole("radio", { name: "Compact" });
  const disabledAction = page.getByRole("button", { name: "Disabled" });
  const invalidField = page.getByRole("textbox", { name: "Invalid address" });

  await expect(artwork).toHaveAttribute("aria-checked", "true");
  await artwork.click();
  await expect(artwork).toHaveAttribute("aria-checked", "false");

  await expect(systemTheme).toHaveAttribute("aria-checked", "false");
  await systemTheme.click();
  await expect(systemTheme).toHaveAttribute("aria-checked", "true");

  await compact.click();
  await expect(compact).toHaveAttribute("aria-checked", "true");
  await expect(disabledAction).toBeDisabled();
  await expect(disabledAction).toHaveAttribute("tabindex", "-1");
  await expect(invalidField).toHaveAttribute("aria-invalid", "true");
});
