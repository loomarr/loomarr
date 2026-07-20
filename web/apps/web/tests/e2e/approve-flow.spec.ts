import { expect, test } from "@playwright/test";
import { installMockBackend } from "./mock-backend";

// The §7 approval gate, driven through the REAL embedded SPA build.
//
// This is the 13.4 gate's load-bearing test. The gate is the one place the UI can spend
// real resources — approving turns a proposal into acquisitions that download content —
// so §19 requires the negative case, not just the happy path. Component tests cover the
// button; this covers whether the RUNNING APP honors the rule.
//
// Assertions land on the mock's recorded state, not on the screen: "the button looked
// like it worked" is exactly the failure a gate exists to catch.

test.describe("the approval gate", () => {
  test("an admin approving enqueues the acquisition", async ({ page }) => {
    const mock = await installMockBackend(page, { authed: true, role: "admin", pendingProposal: true });

    await page.goto("/suggest");

    // The queue is where an admin acts on work submitted by others — the only path from
    // a proposal to a real acquisition. Scoped to that section, because the IntentForm
    // above it offers a template chip with the same words, and matching the chip would
    // prove nothing about the queue.
    const queue = page.locator("section").filter({ hasText: /awaiting approval/i });
    await expect(queue.getByRole("heading", { name: /awaiting approval/i })).toBeVisible();
    await expect(queue.getByText(/90s saturday morning cartoons/i)).toBeVisible();
    await expect(queue.getByText(/1 to acquire/i)).toBeVisible();

    // Nothing spent yet: a proposal is a plan.
    expect(mock.state.enqueued).toEqual([]);

    await queue.getByRole("button", { name: /^approve$/i }).click();

    await expect
      .poll(() => mock.state.enqueued, { message: "approving should enqueue the acquisition" })
      .toEqual(["series:tmdb:gargoyles"]);
    expect(mock.state.proposals[0]?.status).toBe("approved");
  });

  // §19's negative. A member browsing the same screen must not be able to spend anything,
  // and the UI must not offer a control the server would refuse.
  test("a member is not offered approval, and nothing is enqueued", async ({ page }) => {
    const mock = await installMockBackend(page, { authed: true, role: "member", pendingProposal: true });

    await page.goto("/suggest");
    await expect(page.getByRole("heading", { name: /^suggest$/i })).toBeVisible();

    // The queue is admin-only, so a member never sees it at all.
    await expect(page.getByRole("heading", { name: /awaiting approval/i })).toHaveCount(0);
    await expect(page.getByRole("button", { name: /^approve$/i })).toHaveCount(0);

    // And the outcome that actually matters: nothing was acquired.
    await expect
      .poll(() => mock.state.enqueued, {
        message: "a member must never cause an acquisition (§19)",
        timeout: 2000,
      })
      .toEqual([]);
    expect(mock.state.proposals[0]?.status).toBe("submitted");
  });
});
