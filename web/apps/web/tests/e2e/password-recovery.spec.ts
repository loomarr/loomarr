import { expect, test } from "@playwright/test";
import { installMockBackend } from "./mock-backend";

const GRANT = "f".repeat(64);

test("a local person requests and explicitly redeems a cleaned memory-only reset grant", async ({ page }) => {
  await installMockBackend(page, { authed: false });
  const requests: unknown[] = [];
  const previews: unknown[] = [];
  const redemptions: unknown[] = [];

  await page.route("**/v1/auth/password-recovery/request", async (route) => {
    requests.push(route.request().postDataJSON());
    await route.fulfill({
      status: 202,
      contentType: "application/json",
      body: JSON.stringify({
        message: "If that account can be recovered here, Loomarr will send a password reset email.",
      }),
    });
  });
  await page.goto("/login");
  await page.getByRole("link", { name: "Forgot password?" }).click();
  await expect(page).toHaveURL("/forgot-password");
  await page.getByLabel("Username").fill("Robin");
  await page.getByRole("button", { name: "Email reset link" }).click();
  await expect(page.getByRole("status")).toContainText("If that account can be recovered here");
  expect(requests).toEqual([{ username: "Robin" }]);

  await page.route("**/v1/auth/password-recovery/preview", async (route) => {
    previews.push(route.request().postDataJSON());
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ expiresAt: Date.UTC(2030, 2, 24, 13, 30) }),
    });
  });
  await page.route("**/v1/auth/password-recovery/redeem", async (route) => {
    redemptions.push(route.request().postDataJSON());
    await route.fulfill({ status: 204 });
  });

  await page.goto(`/reset-password#grant=${GRANT}`);
  await expect(page.getByLabel("New password", { exact: true })).toBeVisible();
  await expect(page).toHaveURL("/reset-password");
  expect(previews).toEqual([{ grant: GRANT }]);
  expect(redemptions).toEqual([]);
  expect(await page.evaluate(() => ({ local: { ...localStorage }, session: { ...sessionStorage } }))).toEqual(
    { local: {}, session: {} },
  );

  await page.getByLabel("New password", { exact: true }).fill("replacement-password");
  await page.getByLabel("Confirm new password").fill("replacement-password");
  await page.getByRole("button", { name: "Reset password" }).click();

  await expect(page.getByRole("status")).toContainText("Every existing session has been signed out");
  expect(redemptions).toEqual([{ grant: GRANT, password: "replacement-password" }]);
  expect(page.url()).not.toContain(GRANT);
});
