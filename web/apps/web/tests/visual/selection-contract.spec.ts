import { expect, test } from "@playwright/test";

test("shared selection controls publish and update their browser state", async ({ page }) => {
  await page.goto("/iframe.html?id=loomarr-foundations-selection--pointer&viewMode=story");

  const guide = page.getByRole("tab", { name: "Guide" });
  const surf = page.getByRole("tab", { name: "Surf" });
  const appearance = page.getByRole("button", { name: "Appearance, Dark" });
  const refresh = page.getByRole("menuitem", { name: "Refresh guide" });
  const favourite = page.getByRole("menuitem", { name: "Favourite channel" });

  await expect(guide).toHaveAttribute("aria-selected", "true");
  await guide.press("ArrowRight");
  await expect(surf).toHaveAttribute("aria-selected", "true");
  await expect(guide).toHaveAttribute("tabindex", "-1");

  await appearance.click();
  await expect(appearance).toHaveAttribute("aria-expanded", "true");
  await page.getByRole("radio", { name: "Light" }).click();
  await expect(page.getByRole("button", { name: "Appearance, Light" })).toHaveAttribute(
    "aria-expanded",
    "false",
  );
  const lightAppearance = page.getByRole("button", { name: "Appearance, Light" });
  await lightAppearance.click();
  await page.getByRole("radio", { name: "Light" }).press("Escape");
  await expect(lightAppearance).toBeFocused();
  await expect(lightAppearance).toHaveAttribute("aria-expanded", "false");

  await expect(favourite).toBeDisabled();
  await refresh.focus();
  await refresh.press("ArrowDown");
  await expect(page.getByRole("menuitem", { name: "Disconnect device" })).toBeFocused();
  await refresh.focus();
  await refresh.press("Escape");
  await expect(page.getByText("dismiss", { exact: true })).toBeVisible();
  await refresh.click();
  await expect(page.getByText("refresh", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Go live" }).focus();
  await expect(page.getByRole("tooltip")).toContainText("live edge");
});
