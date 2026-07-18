import { expect, type Page, test } from "@playwright/test";
import { installMockBackend } from "./mock-backend";

// The Phase 13.3 gate (frontend-build-plan §5): a wizard e2e smoke against a mocked
// backend, plus a page-level snapshot per step. This is the first thing to exercise the
// whole first-run flow as a user meets it — the unit tests each prove one step, this
// proves they compose into a path an operator can actually walk from a fresh install to
// a finished setup.

// Regions whose content legitimately changes between runs. Masked rather than frozen:
// the relative timestamp is real behaviour we want rendering, just not diffing.
const mask = (page: Page) => [page.getByText(/last received/i)];

// Each step's snapshot waits for the heading that identifies it, so a shot is never
// taken mid-transition.
const shot = async (page: Page, name: string, heading: RegExp) => {
  await expect(page.getByRole("heading", { name: heading })).toBeVisible();
  await page.evaluate(() => document.fonts.ready);
  await expect(page).toHaveScreenshot(`${name}.png`, { mask: mask(page), fullPage: true });
};

test.describe("operator first-run wizard", () => {
  test("walks a fresh install from bootstrap to a finished setup", async ({ page }) => {
    const backend = await installMockBackend(page);

    // --- step 1: bootstrap ------------------------------------------------------
    // A true first run: no admin exists, so the wizard is reachable unauthenticated.
    await page.goto("/wizard");
    await shot(page, "step-1-bootstrap", /create the owning admin/i);

    await page.getByLabel("Username").fill("ada");
    await page.getByLabel("Password", { exact: true }).fill("hunter2!");
    await page.getByLabel("Confirm password").fill("hunter2!");
    await page.getByRole("button", { name: /create admin/i }).click();

    // Bootstrap issues no session, so the step signs the new admin in for them.
    await expect(page.getByRole("heading", { name: /connect your services/i })).toBeVisible();
    expect(backend.state.authed).toBe(true);

    // --- step 2: connection checklist -------------------------------------------
    await shot(page, "step-2-checklist", /connect your services/i);
    await page.getByRole("button", { name: "Continue" }).click();

    // --- step 3: live TV --------------------------------------------------------
    await shot(page, "step-3-livetv", /put your channels in the tv guide/i);
    await page.getByRole("button", { name: /connect tunarr to the guide/i }).click();
    // The CHECK reports success, not the click (§6 "never silent").
    await expect(page.getByRole("button", { name: /run again/i })).toBeVisible();
    expect(backend.state.checks.livetv).toBe(true);
    await page.getByRole("button", { name: "Continue" }).click();

    // --- step 4: webhook handshake ----------------------------------------------
    await expect(page.getByRole("heading", { name: /tell sonarr and radarr/i })).toBeVisible();
    // The URL is built from the REVEALED secret — what the operator pastes, unrotated.
    await expect(page.getByText(/\/hooks\/arr\?token=s3cr3t/)).toBeVisible();
    // Neither app has reported yet: both listen, neither reads as failed.
    await expect(page.getByText(/press Test in this app/i)).toHaveCount(2);
    await shot(page, "step-4-webhooks", /tell sonarr and radarr/i);
    // No *arr apps configured is a legitimate install — skipping must be possible.
    await page.getByRole("button", { name: /skip for now/i }).click();

    // --- step 5: tunarr library --------------------------------------------------
    await shot(page, "step-5-library", /give tunarr your library/i);
    await page.getByRole("button", { name: /wire tunarr to your library/i }).click();
    await expect(page.getByRole("button", { name: /run again/i })).toBeVisible();
    expect(backend.state.checks.tunarr_library).toBe(true);
    await page.getByRole("button", { name: "Continue" }).click();

    // --- step 6: import users -----------------------------------------------------
    await expect(page.getByRole("heading", { name: /import media-server users/i })).toBeVisible();
    await page.getByLabel("Ada").check();
    await shot(page, "step-6-users", /import media-server users/i);
    await page.getByRole("button", { name: /^import/i }).click();
    // Only the picked account is allowlisted — signing in is not self-provisioning (§11).
    await expect(page.getByText("imported")).toBeVisible();
    expect(backend.state.imported).toEqual(["u-ada"]);
    await page.getByRole("button", { name: "Continue" }).click();

    // --- step 7: guided first channel ---------------------------------------------
    await shot(page, "step-7-first-channel", /your first channel/i);
    await page.getByText("90s Saturday Morning Cartoons").click();

    // Finishing the wizard flips setup.completed (so `/` stops routing here) and hands
    // off to Suggest with the template's intent prefilled.
    await expect(page).toHaveURL(/\/suggest\?intent=/);
    expect(backend.state.edits["setup.completed"]).toBe("true");
  });

  test("a completed setup no longer routes / to the wizard", async ({ page }) => {
    // The other half of first-run detection: once the flag is set, the app opens normally.
    const backend = await installMockBackend(page, { authed: true });
    backend.state.edits["setup.completed"] = "true";

    await page.goto("/");
    await expect(page.getByRole("heading", { name: "Channels" })).toBeVisible();
  });

  test("an unfinished setup sends the operator back into the wizard", async ({ page }) => {
    await installMockBackend(page, { authed: true, checks: { media_server: false, tunarr: false } });

    await page.goto("/");
    await expect(page.getByText(/first-run setup/i)).toBeVisible();
  });
});
