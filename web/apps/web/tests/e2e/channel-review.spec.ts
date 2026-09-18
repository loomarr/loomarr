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

  // Finding more is an in-place refresh. It keeps the current choices visible,
  // explains what is happening, and sends the exact selected lineup as context.
  await page.getByText("More suggestions", { exact: true }).click();
  await page.getByRole("button", { name: "Find more suggestions" }).click();
  await expect(page.getByRole("button", { name: "Finding more suggestions…" })).toBeDisabled();
  await expect(
    page.getByText("Finding more suggestions… Your selected titles won’t change."),
  ).toHaveAttribute("role", "status");
  await expect(page.getByRole("textbox", { name: "Channel intent" })).toHaveCount(0);
  await expect
    .poll(() => mock.state.proposalRevisionRequests)
    .toEqual([
      {
        jobId: "proposal-job-1",
        intent: {
          description: "90s action movies",
          currentLineup: [
            { name: "Point Break", year: 1991, key: "movie:tmdb:1089" },
            { name: "Con Air", year: 1997, key: "movie:tmdb:1701" },
            { name: "The Matrix", year: 1999, key: "movie:tmdb:603" },
          ],
          refineText:
            "Find 6–8 additional titles that match this brief. Do not repeat or replace the selected lineup.",
        },
      },
    ]);
  await expect(page.getByText("Sci-Fi Action Mix", { exact: true })).toBeVisible();
  await expect(page.getByRole("checkbox", { name: "Include Heat" })).not.toBeChecked();
  await expect(page.getByText("The Matrix", { exact: true })).toHaveCount(1);

  // A reload restores both the durable revision and the local selection delta.
  await page.reload();
  await expect(page.getByRole("heading", { name: "Review your channel" })).toBeVisible();
  await expect(page.getByText("Sci-Fi Action Mix", { exact: true })).toBeVisible();
  await expect(page.getByRole("checkbox", { name: "Include Heat" })).not.toBeChecked();
  await expect(page.getByText("The Matrix", { exact: true })).toHaveCount(1);

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
        intent: {
          description: "90s action movies",
          currentLineup: [
            { name: "Point Break", year: 1991, key: "movie:tmdb:1089" },
            { name: "Con Air", year: 1997, key: "movie:tmdb:1701" },
            { name: "The Matrix", year: 1999, key: "movie:tmdb:603" },
          ],
          refineText:
            "Find 6–8 additional titles that match this brief. Do not repeat or replace the selected lineup.",
        },
      },
      {
        jobId: "proposal-job-1",
        intent: {
          description: "90s action with more sci-fi variety",
          currentLineup: [
            { name: "Point Break", year: 1991, key: "movie:tmdb:1089" },
            { name: "The Matrix", year: 1999, key: "movie:tmdb:603" },
            { name: "Con Air", year: 1997, key: "movie:tmdb:1701" },
          ],
        },
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

test("the suggestion review remains usable on a phone with reduced motion", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await installMockBackend(page, { authed: true, role: "admin", proposalJourney: true });

  await page.goto("/guide");
  await page.getByRole("button", { name: "Add a channel" }).click();
  await page.getByRole("textbox", { name: "Channel intent" }).fill("90s action movies");
  await page.getByRole("button", { name: "Suggest a lineup" }).click();

  await expect(page.getByRole("heading", { name: "Review your channel" })).toBeVisible();
  await page.getByText("More suggestions", { exact: true }).click();
  await page.getByRole("button", { name: "Find more suggestions" }).click();

  const progressButton = page.getByRole("button", { name: "Finding more suggestions…" });
  await expect(progressButton).toBeDisabled();
  const animationDuration = await progressButton
    .locator("svg")
    .evaluate((icon) => getComputedStyle(icon).animationDuration);
  expect(Number.parseFloat(animationDuration)).toBeLessThanOrEqual(0.001);

  const review = page.getByRole("heading", { name: "Review your channel" }).locator("../..");
  const reviewBox = await review.boundingBox();
  expect(reviewBox).not.toBeNull();
  expect((reviewBox?.x ?? 0) + (reviewBox?.width ?? 0)).toBeLessThanOrEqual(390);
  await expect(page.getByRole("button", { name: "Create channel" })).toBeVisible();
  await expect(page.getByRole("checkbox", { name: "Include Heat" })).toBeVisible();
});
