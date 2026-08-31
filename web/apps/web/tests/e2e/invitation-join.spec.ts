import { expect, test } from "@playwright/test";
import { installMockBackend } from "./mock-backend";

const GRANT = "e".repeat(64);

test("a local invitee explicitly activates from a cleaned, memory-only grant", async ({ page }) => {
  const mock = await installMockBackend(page, { authed: false });
  const previews: unknown[] = [];
  const redemptions: unknown[] = [];

  await page.route("**/v1/invitations/preview", async (route) => {
    previews.push(route.request().postDataJSON());
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        kind: "local",
        username: "Robin",
        role: "member",
        credentialPath: "local_password",
        expiresAt: Date.UTC(2030, 2, 24, 13, 30),
      }),
    });
  });
  await page.route("**/v1/invitations/redeem", async (route) => {
    redemptions.push(route.request().postDataJSON());
    mock.state.authed = true;
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        id: "invitee-robin",
        name: "Robin",
        role: "member",
        local: true,
        offlineLogin: false,
        autoApprove: false,
        disabled: false,
        quota: 0,
      }),
    });
  });

  await page.goto(`/join#grant=${GRANT}`);

  await expect(page.getByLabel("Create password")).toBeVisible();
  await expect(page).toHaveURL("/join");
  expect(previews).toEqual([{ grant: GRANT }]);
  expect(redemptions).toEqual([]);
  expect(await page.evaluate(() => ({ local: { ...localStorage }, session: { ...sessionStorage } }))).toEqual(
    { local: {}, session: {} },
  );

  await page.getByLabel("Create password").fill("a-safe-password");
  await page.getByLabel("Confirm password").fill("a-safe-password");
  await page.getByRole("button", { name: "Activate account" }).click();

  await expect(page).toHaveURL("/guide");
  expect(redemptions).toEqual([{ grant: GRANT, password: "a-safe-password" }]);
  expect(page.url()).not.toContain(GRANT);
});
