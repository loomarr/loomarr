import { expect, test } from "@playwright/test";

test("the workshop theme toolbar drives the real shared provider", async ({ page }, testInfo) => {
  test.skip(
    testInfo.project.name !== "desktop",
    "The Storybook manager toolbar is reviewed at desktop width.",
  );

  await page.goto("/?path=/story/loomarr-components-feedback-and-recovery--empty");
  const preview = page.frameLocator('iframe[title="storybook-preview-iframe"]');
  const panel = preview.getByRole("status");
  await expect(panel).toBeVisible();

  await page.getByText("dark", { exact: true }).click();
  await page.getByText("light", { exact: true }).click();

  await expect(page.getByText("light", { exact: true })).toBeVisible();
  await expect(panel).toHaveCSS("background-color", "rgb(255, 255, 255)");
  await expect(preview.locator("body")).toHaveCSS("background-color", "rgb(247, 248, 250)");

  await page.getByText("light", { exact: true }).click();
  await page.emulateMedia({ colorScheme: "light" });
  await page.getByText("system", { exact: true }).click();
  await expect(panel).toHaveCSS("background-color", "rgb(255, 255, 255)");

  await page.emulateMedia({ colorScheme: "dark" });
  await expect(panel).toHaveCSS("background-color", "rgb(19, 21, 25)");
  await expect(preview.locator("body")).toHaveCSS("background-color", "rgb(11, 12, 14)");
});
