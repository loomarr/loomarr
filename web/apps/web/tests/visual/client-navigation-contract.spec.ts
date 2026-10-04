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

// A phone's tab bar (#1659 native mock 5d-5g): tabs in a named list, one selected, keyboard-operable.
for (const story of ["touch", "material"]) {
  test(`the ${story} tab bar selects by keyboard and announces its tab`, async ({ page }) => {
    await page.goto(`/iframe.html?id=loomarr-components-client-navigation--${story}&viewMode=story`);

    const guide = page.getByRole("tab", { name: "Guide" });
    const surf = page.getByRole("tab", { name: "Surf" });
    await expect(page.getByRole("tablist", { name: "Primary navigation" })).toBeVisible();
    await expect(guide).toHaveAttribute("aria-selected", "true");

    // Requests sits between Guide and Surf on a phone (#1816), so Surf is the second Tab stop.
    await guide.focus();
    await page.keyboard.press("Tab");
    await expect(page.getByRole("tab", { name: "Requests" })).toBeFocused();
    await page.keyboard.press("Tab");
    await expect(surf).toBeFocused();
    await surf.press("Enter");

    await expect(surf).toHaveAttribute("aria-selected", "true");
    await expect(guide).toHaveAttribute("aria-selected", "false");
  });
}
