import { expect, test } from "@playwright/test";
import { installMockBackend } from "./mock-backend";

test("Filler keeps its workspace stable and settings focused without losing edits", async ({ page }) => {
  const backend = await installMockBackend(page, { authed: true, role: "admin", fillerEnabled: true });
  await page.goto("/filler/manage");
  await page.getByRole("link", { name: "Automatic download settings" }).click();
  await expect(page).toHaveURL(/\/filler\/settings\/downloads$/);
  await expect(page.getByRole("heading", { level: 1, name: "Filler", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Automatic downloads", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Clip folders", exact: true })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "Processing tools", exact: true })).toHaveCount(0);
  await page.getByRole("spinbutton", { name: "Add up to", exact: true }).fill("4");
  await page.getByRole("link", { name: "Storage limits", exact: true }).click();
  await expect(page).toHaveURL(/\/filler\/settings\/storage$/);
  await expect(page.getByRole("spinbutton", { name: "Automatic-download catalog limit" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Automatic downloads", exact: true })).toHaveCount(0);
  await page.getByRole("spinbutton", { name: "Automatic-download catalog limit" }).fill("1000");
  await expect(page.getByRole("button", { name: "Save changes", exact: true })).toHaveCount(1);
  await page.getByRole("button", { name: /More settings/ }).click();
  await page
    .getByRole("navigation", { name: "Filler settings tasks" })
    .getByRole("link", { name: "Automatic downloads", exact: true })
    .click();
  await expect(page.getByRole("spinbutton", { name: "Add up to", exact: true })).toHaveValue("4");
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  await expect
    .poll(() => backend.state.edits)
    .toMatchObject({
      "filler.fetch.max_per_run": "4",
      "filler.fetch.max_catalog_clips": "1000",
    });

  for (const viewport of [
    { width: 1440, height: 1000 },
    { width: 390, height: 844 },
  ]) {
    await page.setViewportSize(viewport);
    const positions: number[] = [];
    for (const path of [
      "/filler",
      "/filler/sources",
      "/filler/incoming",
      "/filler/library",
      "/filler/manage",
      "/filler/settings",
    ]) {
      await page.goto(path);
      const navigation = page.getByRole("navigation", { name: "Filler sections" });
      await expect(navigation).toBeVisible();
      positions.push((await navigation.boundingBox())?.y ?? -1);
      await expect(page.getByRole("heading", { level: 1, name: "Filler", exact: true })).toHaveCount(1);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      if (path === "/filler/library") {
        // Document overflow alone misses a control spilling out of its own card.
        const viewFits = await page.getByRole("radiogroup", { name: "Clip view" }).evaluate((element) => {
          const card = element.parentElement?.getBoundingClientRect();
          if (!card) return false;
          return Array.from(element.children).every((child) => {
            const bounds = child.getBoundingClientRect();
            return bounds.left >= card.left && bounds.right <= card.right;
          });
        });
        expect(viewFits).toBe(true);
        await expect(page.getByRole("button", { name: "Propose a pull" })).toHaveCount(0);
        const health = page.getByRole("region", { name: "Catalog health" });
        await expect(health).toBeVisible();
        expect((await health.boundingBox())?.y).toBeGreaterThan((await navigation.boundingBox())?.y ?? 0);
      }
    }
    expect(Math.max(...positions) - Math.min(...positions)).toBeLessThanOrEqual(1);
  }
});

test("empty Library leads to Sources instead of creating an acquisition", async ({ page }) => {
  await installMockBackend(page, { authed: true, role: "admin", fillerEnabled: true });
  await page.route("**/v1/filler?*", (route) => route.fulfill({ json: { clips: [], total: 0 } }));
  await page.goto("/filler/library");
  await page.getByRole("button", { name: "Open sources", exact: true }).click();
  await expect(page).toHaveURL(/\/filler\/sources$/);
  await expect(page.getByRole("heading", { name: "Where filler comes from" })).toBeVisible();
});
