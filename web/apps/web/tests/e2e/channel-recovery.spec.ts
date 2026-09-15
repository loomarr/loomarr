import { expect, test } from "@playwright/test";
import { installMockBackend } from "./mock-backend";

test("missing TMDB keeps connected AI truthful and preserves the channel draft through settings", async ({
  page,
}) => {
  await installMockBackend(page, { authed: true, role: "admin", checks: { tmdb: false } });
  await page.route("**/v1/proposals", async (route) => {
    if (route.request().method() !== "POST") return route.fallback();
    return route.fulfill({
      status: 409,
      contentType: "application/json",
      body: JSON.stringify({
        type: "grounding_not_configured",
        title: "TMDB is needed for channel suggestions",
        detail:
          "Connect TMDB in Settings → Connections so Loomarr can match this channel description to real titles.",
      }),
    });
  });

  await page.goto("/guide");
  await page.getByRole("button", { name: "Add a channel" }).click();
  await page.getByRole("textbox", { name: "Channel intent" }).fill("Saturday morning cartoons");
  await page.getByRole("button", { name: "Add constraints" }).click();
  await page.getByLabel("Era").fill("1990s");
  await page.getByRole("button", { name: "Suggest a lineup" }).click();

  await expect(page.getByText("Connect TMDB to build this channel")).toBeVisible();
  await expect(page.getByText(/AI is connected/)).toBeVisible();
  await expect(page.getByText(/AI isn't set up|Connect AI/)).toHaveCount(0);

  await page.getByRole("link", { name: "Connect TMDB" }).click();
  await expect(page).toHaveURL(/\/settings\/connections\?focus=tmdb$/);
  await expect(page.getByRole("button", { name: /^TMDB/ })).toHaveAttribute("aria-expanded", "true");
  await expect(page.getByText("Your channel draft is saved")).toBeVisible();

  await page.getByRole("link", { name: "Return to channel" }).click();
  await expect(page).toHaveURL(/\/guide$/);
  await expect(page.getByRole("textbox", { name: "Channel intent" })).toHaveValue(
    "Saturday morning cartoons",
  );
  await page.getByRole("button", { name: "Add constraints" }).click();
  await expect(page.getByLabel("Era")).toHaveValue("1990s");

  await page.getByRole("button", { name: "Close" }).click();
  await page.reload();
  await page.getByRole("button", { name: "Add a channel" }).click();
  await expect(page.getByRole("textbox", { name: "Channel intent" })).toHaveValue("");
});

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
  await expect(page.getByText("Adjust your description")).toBeVisible();
  await expect(page.getByText("No grounded titles matched this request.")).toBeVisible();
  await expect(page.getByRole("button", { name: "Try again" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Edit description" })).toBeVisible();
  await expect.poll(() => mock.state.proposalJobRequests).toHaveLength(1);

  // The Journey, rather than a client guess, granted this action. Editing must restore every
  // persisted constraint before the operator sends the next request.
  await page.getByRole("button", { name: "Edit description" }).click();
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

test("a bounded-discovery failure preserves the draft without exposing an internal budget", async ({
  page,
}) => {
  await installMockBackend(page, { authed: true, role: "admin", failedProposalJourney: true });
  await page.route("**/v1/proposal-jobs/**", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        version: 1,
        jobId: "failed-job-1",
        milestone: "failed",
        intent: { description: "Create a TGIF channel" },
        attempts: [],
        failure: {
          code: "budget_exhausted",
          reason: "discovery_budget_exhausted",
          recoveryAction: "simplify_request",
          message: "This request exceeded the bounded discovery budget.",
          guidance: "Simplify the request and try again.",
        },
        actions: ["edit", "retry"],
        createdAt: "2026-09-07T12:00:00Z",
        updatedAt: "2026-09-07T12:00:01Z",
      }),
    }),
  );

  await page.goto("/guide");
  await page.getByRole("button", { name: "Add a channel" }).click();
  await page.getByRole("textbox", { name: "Channel intent" }).fill("Create a TGIF channel");
  await page.getByRole("button", { name: "Suggest a lineup" }).click();

  const recovery = page.getByRole("alert");
  await expect(recovery).toContainText("We couldn't finish the lineup");
  await expect(recovery).toContainText(
    "Your description is still here. Try again, or edit it if you want to.",
  );
  await expect(recovery).not.toContainText(/budget|more specific|too many possible directions/i);
  await page.getByRole("button", { name: "Edit description" }).click();
  await expect(page.getByRole("textbox", { name: "Channel intent" })).toHaveValue("Create a TGIF channel");
});

test("discarding clears a deep-linked Journey so a reload is genuinely fresh", async ({ page }) => {
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
  await page.getByRole("button", { name: "Discard" }).click();
  await expect(page.getByRole("textbox", { name: "Channel intent" })).toHaveValue("");
  await expect(page).toHaveURL(/\/guide$/);

  await page.reload();
  await page.getByRole("button", { name: "Add a channel" }).click();
  await expect(page.getByRole("textbox", { name: "Channel intent" })).toHaveValue("");
  expect(mock.state.channelCreationRequests).toEqual([]);
  expect(mock.state.approvalRequests).toEqual([]);
  expect(mock.state.enqueued).toEqual([]);
});
