import { expect, test } from "@playwright/test";
import { installMockBackend } from "./mock-backend";

test("automatic downloads expose simple defaults with optional per-source control", async ({ page }) => {
  const backend = await installMockBackend(page, {
    authed: true,
    role: "admin",
    fillerEnabled: true,
  });

  await page.goto("/filler/manage");

  await expect(page.getByRole("heading", { name: "Automatic downloads" })).toBeVisible();
  await page.getByRole("link", { name: "Automatic download settings" }).click();

  await expect(page.getByRole("heading", { name: "Automatic downloads" })).toBeVisible();
  await expect(
    page.getByText("1 enabled source can add up to 10 clips each time Loomarr checks."),
  ).toBeVisible();
  const globalSchedule = page.getByRole("combobox", { name: "Look for new clips" });
  const globalLimit = page.getByRole("spinbutton", { name: "Add up to" });
  await expect(globalSchedule).toContainText("Every 6 hours");
  await expect(globalLimit).toHaveValue("10");
  await expect(page.getByRole("button", { name: "Show advanced (2)" })).toBeVisible();

  await globalSchedule.click();
  await page.getByRole("option", { name: "Every 12 hours" }).click();
  await globalLimit.fill("4");
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

  await expect(page.getByText("Uses your defaults: every 6 hours, up to 10 clips each check.")).toBeVisible();
  await page.getByLabel("Automatic downloads for this source").click();
  await page.getByRole("option", { name: "Use a different schedule" }).click();
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
});
