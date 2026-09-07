import { expect, test } from "@playwright/test";
import { installMockBackend } from "./mock-backend";

test("a failed builder journey preserves the complete intent through authorized recovery", async ({
  page,
}) => {
  const mock = await installMockBackend(page, { authed: true, role: "admin", failedProposalJourney: true });

  await page.goto("/guide");
  await page.getByRole("button", { name: "Add a channel" }).click();
  await page.getByRole("textbox", { name: "Channel intent" }).fill("90s action movies");
  await page.getByRole("button", { name: "Add constraints" }).click();
  await page.getByLabel("Era").fill("1990s");
  await page.getByLabel("Target runtime (minutes)").fill("180");
  await page.getByLabel("Must include").fill("Heat, Point Break");
  await page.getByLabel("Must exclude").fill("clowns");

  await page.getByRole("button", { name: "Suggest a lineup" }).click();
  await expect(page.getByText("No grounded titles matched this request.")).toBeVisible();
  await expect(page.getByText("Broaden the request or add examples from your library.")).toBeVisible();
  await expect(page.getByRole("button", { name: "Try again" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Edit request" })).toBeVisible();
  await expect.poll(() => mock.state.proposalJobRequests).toHaveLength(1);

  // The Journey, rather than a client guess, granted this action. Editing must restore every
  // persisted constraint before the operator sends the next request.
  await page.getByRole("button", { name: "Edit request" }).click();
  await expect(page.getByRole("textbox", { name: "Channel intent" })).toHaveValue("90s action movies");
  await page.getByRole("button", { name: "Add constraints" }).click();
  await expect(page.getByLabel("Era")).toHaveValue("1990s");
  await expect(page.getByLabel("Target runtime (minutes)")).toHaveValue("180");
  await expect(page.getByLabel("Must include")).toHaveValue("Heat, Point Break");
  await expect(page.getByLabel("Must exclude")).toHaveValue("clowns");

  await page.getByRole("button", { name: "Suggest a lineup" }).click();
  await expect.poll(() => mock.state.proposalJobRequests).toHaveLength(2);
  expect(mock.state.proposalJobRequests).toEqual([
    {
      description: "90s action movies",
      era: "1990s",
      runtimeTargetMin: 180,
      mustInclude: ["Heat", "Point Break"],
      mustExclude: ["clowns"],
    },
    {
      description: "90s action movies",
      era: "1990s",
      runtimeTargetMin: 180,
      mustInclude: ["Heat", "Point Break"],
      mustExclude: ["clowns"],
    },
  ]);
  // Request-level telemetry catches an accidental direct create or approval call even if a
  // mock response would otherwise let the screen look recovered. The positive approval flow
  // proves that the approval recorder observes the real mutation route.
  expect(mock.state.channelCreationRequests).toEqual([]);
  expect(mock.state.approvalRequests).toEqual([]);
  expect(mock.state.enqueued).toEqual([]);
});

test("starting over clears a deep-linked Journey so a reload is genuinely fresh", async ({ page }) => {
  const mock = await installMockBackend(page, { authed: true, role: "admin", proposalJourney: true });

  // Seed a real persisted Journey, then enter through the same opaque `?job=` link used by
  // recovery navigation. This rules out a test that only observes in-memory reset state.
  await page.goto("/guide");
  await page.getByRole("button", { name: "Add a channel" }).click();
  await page.getByRole("textbox", { name: "Channel intent" }).fill("90s action movies");
  await page.getByRole("button", { name: "Suggest a lineup" }).click();
  await expect.poll(() => mock.state.proposalJobRequests).toHaveLength(1);

  await page.goto("/guide?job=proposal-job-1&intent=stale%20template");
  await expect(page.getByText("Heat")).toBeVisible();
  await page.getByRole("button", { name: "Start over" }).click();
  await expect(page.getByRole("textbox", { name: "Channel intent" })).toHaveValue("");
  await expect(page).toHaveURL(/\/guide$/);

  await page.reload();
  await page.getByRole("button", { name: "Add a channel" }).click();
  await expect(page.getByRole("textbox", { name: "Channel intent" })).toHaveValue("");
  expect(mock.state.channelCreationRequests).toEqual([]);
  expect(mock.state.approvalRequests).toEqual([]);
  expect(mock.state.enqueued).toEqual([]);
});
