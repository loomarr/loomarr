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
  await expect(page.getByText("Will be added", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Create channel" })).toBeVisible();

  await page.getByRole("checkbox", { name: "Include Heat" }).click();
  await expect(page.getByText("Not included")).toBeVisible();

  await page.getByRole("button", { name: "Add title" }).click();
  await page.getByRole("combobox").fill("matrix");
  await page.getByText("The Matrix").click();
  await expect(page.getByText("The Matrix", { exact: true })).toBeVisible();

  // Close means "come back later", not "throw this away". The durable Job and
  // its local title delta restore together when the panel is reopened.
  await page.getByRole("button", { name: "Close" }).click();
  await page.getByRole("button", { name: "Add a channel" }).click();
  await expect(page.getByRole("heading", { name: "Review your channel" })).toBeVisible();
  await expect(page.getByRole("checkbox", { name: "Include Heat" })).not.toBeChecked();
  await expect(page.getByText("The Matrix", { exact: true })).toBeVisible();

  // Editing the brief revises this Job in place. The current review remains on
  // screen while the replacement runs; it never falls back to the describe form.
  await page.getByRole("button", { name: "Edit brief" }).click();
  await page.getByRole("textbox", { name: "Channel brief" }).fill("90s action with more sci-fi variety");
  await page.getByRole("button", { name: "Update suggestions" }).click();
  await expect(page.getByRole("heading", { name: "Updating suggestions" })).toBeVisible();
  await expect(page.getByText("Heat", { exact: true })).toBeVisible();
  await expect(page.getByRole("textbox", { name: "Channel intent" })).toHaveCount(0);
  await expect
    .poll(() => mock.state.proposalRevisionRequests)
    .toEqual([
      {
        jobId: "proposal-job-1",
        intent: { description: "90s action with more sci-fi variety" },
      },
    ]);
  await expect(page.getByRole("heading", { name: "Review your channel" })).toBeVisible();
  await expect(page.getByText("Sci-Fi Action Mix", { exact: true })).toBeVisible();
  await expect(page.getByRole("checkbox", { name: "Include Heat" })).not.toBeChecked();
  await expect(page.getByText("The Matrix", { exact: true })).toHaveCount(1);

  await page.getByRole("button", { name: "Create channel" }).click();
  await expect
    .poll(() => mock.state.approvalEdits)
    .toEqual([
      {
        drop: ["movie:tmdb:949"],
      },
    ]);
});
