import { expect, test } from "@playwright/test";
import { installMockBackend } from "./mock-backend";

test("a first-time admin can shape the suggested channel before creating it", async ({ page }) => {
  const mock = await installMockBackend(page, { authed: true, role: "admin", proposalJourney: true });

  await page.goto("/guide");
  await page.getByRole("button", { name: "Add a channel" }).click();
  await page.getByRole("textbox", { name: "Channel intent" }).fill("90s action movies");
  await page.getByRole("button", { name: "Suggest a lineup" }).click();

  await expect(page.getByRole("heading", { name: "Review your channel" })).toBeVisible();
  await expect(page.getByText("Friday Night Action", { exact: true })).toBeVisible();
  await expect(page.getByText("Dead air")).toHaveCount(0);
  await expect(page.getByText("In your library").first()).toBeVisible();
  await expect(page.getByText("Will be added")).toBeVisible();
  await expect(page.getByRole("button", { name: "Create channel" })).toBeVisible();

  await page.getByRole("checkbox", { name: "Include Heat" }).click();
  await expect(page.getByText("Not included")).toBeVisible();

  await page.getByRole("button", { name: "Add title" }).click();
  await page.getByRole("combobox").fill("matrix");
  await page.getByText("The Matrix").click();
  await expect(page.getByText("Added by you")).toBeVisible();

  await page.getByRole("button", { name: "Create channel" }).click();
  await expect
    .poll(() => mock.state.approvalEdits)
    .toEqual([
      {
        drop: ["movie:tmdb:949"],
        add: [
          {
            name: "The Matrix",
            year: 1999,
            mediaType: "movie",
            tmdbId: 603,
            inLibrary: false,
          },
        ],
      },
    ]);
});
