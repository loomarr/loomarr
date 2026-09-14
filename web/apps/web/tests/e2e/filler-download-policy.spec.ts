import { expect, test } from "@playwright/test";
import { installMockBackend } from "./mock-backend";

test("automatic downloads expose simple defaults with optional per-source control", async ({ page }) => {
  const backend = await installMockBackend(page, {
    authed: true,
    role: "admin",
    fillerEnabled: true,
  });

  await page.goto("/filler/manage");

  await expect(page.getByRole("heading", { name: "Automatic downloads" })).toBeVisible({
    timeout: 10_000,
  });
  await page.getByRole("link", { name: "Automatic download settings" }).click();

  await expect(page.getByRole("heading", { name: "Automatic downloads" })).toBeVisible({
    timeout: 10_000,
  });
  await expect(
    page.getByText(
      "Every 6 hours, each source using these defaults can add up to 10 clips. Across 1 enabled source, one full check can add up to 10 clips. Storage limits are under Advanced.",
    ),
  ).toBeVisible({ timeout: 10_000 });
  const globalSchedule = page.getByRole("combobox", { name: "Look for new clips" });
  const globalLimit = page.getByRole("spinbutton", { name: "Add up to" });
  await expect(globalSchedule).toContainText("Every 6 hours");
  await expect(globalLimit).toHaveValue("10");
  await expect(page.getByRole("button", { name: "Show advanced (2)" })).toBeVisible();

  await globalSchedule.click();
  await page.getByRole("option", { name: "Every 12 hours" }).click();
  await globalLimit.fill("4");
  await expect(
    page.getByText(
      "Every 12 hours, each source using these defaults can add up to 4 clips. Across 1 enabled source, one full check can add up to 4 clips. Storage limits are under Advanced.",
    ),
  ).toBeVisible();
  await page.getByRole("button", { name: "Save changes" }).click();

  await expect
    .poll(() => backend.state.edits)
    .toMatchObject({
      "filler.fetch.every": "12h",
      "filler.fetch.max_per_run": "4",
    });

  await page.goto("/filler/sources");
  await page
    .getByRole("button", {
      name: "Manage Classic television commercials from a deliberately long collection name",
    })
    .click();
  await page.getByRole("button", { name: "Show source settings" }).click();

  await expect(page.getByText("Uses your defaults: every 12 hours, up to 4 clips each check.")).toBeVisible();
  await expect(page.getByText(/^Next automatic check /)).toBeVisible();
  await page.getByLabel("Automatic downloads for this source").click();
  await page.getByRole("option", { name: "Use a different schedule" }).click();
  await page.getByLabel("Source check schedule").click();
  await page.getByRole("option", { name: "Custom" }).click();
  await page.getByRole("spinbutton", { name: "Source check interval" }).fill("2");
  await page.getByRole("spinbutton", { name: "Clips per source check" }).fill("3");
  await page.getByRole("button", { name: "Save automatic downloads" }).click();

  await expect
    .poll(() => backend.state.fillerSourcePatches)
    .toEqual([
      {
        id: "archive:long",
        body: {
          enabled: true,
          automaticDownloads: { mode: "custom", everySeconds: 7200, maxPerCheck: 3 },
        },
      },
    ]);

  await page.getByLabel("Automatic downloads for this source").click();
  await page.getByRole("option", { name: "Use automatic-download defaults" }).click();
  await page.getByRole("button", { name: "Save automatic downloads" }).click();
  await expect.poll(() => backend.state.fillerSourcePolicy).toEqual({ mode: "defaults" });

  await page.getByLabel("Automatic downloads for this source").click();
  await page.getByRole("option", { name: "Never download automatically" }).click();
  await page.getByRole("button", { name: "Save automatic downloads" }).click();
  await expect.poll(() => backend.state.fillerSourcePolicy).toEqual({ mode: "never" });
  await expect(
    page.getByText("Doesn’t download automatically. You can still look for clips yourself."),
  ).toBeVisible();
  await expect(page.getByText(/^Next automatic check /)).toHaveCount(0);

  await page.goto("/filler/settings");
  await page.getByRole("combobox", { name: "Look for new clips" }).click();
  await page.getByRole("option", { name: "Never" }).click();
  await expect(
    page.getByText(
      "Automatic downloads are off for sources using these defaults. No enabled sources are currently downloading automatically. Storage limits are under Advanced.",
    ),
  ).toBeVisible();
});

test("automatic download controls stay usable on a narrow, zoomed screen and from the keyboard", async ({
  page,
}) => {
  await installMockBackend(page, { authed: true, role: "admin", fillerEnabled: true });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/filler/manage");

  const settingsLink = page.getByRole("link", { name: "Automatic download settings" });
  await settingsLink.focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("combobox", { name: "Look for new clips" })).toBeVisible();
  await expect(page.getByRole("spinbutton", { name: "Add up to" })).toBeVisible();

  await page.evaluate(() => {
    document.body.style.zoom = "2";
  });
  await expect(page.getByText(/one full check can add up to 10 clips/i)).toBeVisible();

  await page.goto("/filler/sources");
  await page.evaluate(() => {
    document.body.style.zoom = "2";
  });
  const manage = page.getByRole("button", {
    name: "Manage Classic television commercials from a deliberately long collection name",
  });
  await manage.focus();
  await page.keyboard.press("Enter");
  const workspace = page.getByRole("dialog", {
    name: "Classic television commercials from a deliberately long collection name",
  });
  await expect(workspace).toBeVisible();
  const sourceSettings = workspace.getByRole("button", { name: "Show source settings" });
  await sourceSettings.focus();
  await page.keyboard.press("Enter");
  await expect(
    workspace.getByRole("combobox", { name: "Automatic downloads for this source" }),
  ).toBeVisible();
  await expect(workspace.getByRole("button", { name: "Save automatic downloads" })).toBeVisible();
});
