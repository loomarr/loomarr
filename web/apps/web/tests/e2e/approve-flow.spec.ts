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

    // The approvals surface is the Requests page's "Needs you" tab (#1405), not the origination
    // panel: `/suggest` folded into the Guide header (§12), and what moved there is DESCRIBING a
    // channel, not approving one. The tab is explicit in the URL so landing elsewhere fails for
    // the right reason.
    await page.goto("/requests/needs-you");

    // The approvals section is where an admin acts on work submitted by others — the only path
    // from a proposal to a real acquisition. Asserting its heading here is what keeps the
    // member test's "heading absent" check from passing vacuously if the section is renamed.
    await expect(page.getByRole("heading", { name: "Waiting for your approval" })).toBeVisible();
    // Scoped to the panel so a match cannot come from the tab's own label.
    const queue = page.getByRole("tabpanel");
    await expect(queue.getByText(/90s saturday morning cartoons/i)).toBeVisible();
    await expect(queue.getByText(/1 to acquire/i)).toBeVisible();

    // Nothing spent yet: a proposal is a plan.
    expect(mock.state.enqueued).toEqual([]);

    await queue.getByRole("button", { name: /^approve$/i }).click();

    await expect
      .poll(() => mock.state.enqueued, { message: "approving should enqueue the acquisition" })
      .toEqual(["series:tmdb:gargoyles"]);
    // This is the request-level control for recovery's negative case: the mock records the
    // actual approval mutation, not an inferred proposal status or a rendered button.
    expect(mock.state.approvalRequests).toEqual(["prop-1"]);
    expect(mock.state.proposals[0]?.status).toBe("approved");
  });

  // §19's negative. A member browsing the same screen must not be able to spend anything,
  // and the UI must not offer a control the server would refuse.
  test("a member is not offered approval, and nothing is enqueued", async ({ page }) => {
    const mock = await installMockBackend(page, { authed: true, role: "member", pendingProposal: true });

    // Even asking for the approvals tab directly: a member must not get the gate. The old
    // `/queue?tab=approval` link must land on the same page (#1405 redirects it), so both are
    // exercised.
    await page.goto("/queue?tab=approval");
    await expect(page).toHaveURL(/\/requests(\/|$)/);
    await page.goto("/requests/needs-you");

    // Approvals are an admin-only SECTION of "Needs you" (§11), not a tab: every viewer gets the
    // same three tabs, and a member with no requests of their own gets the empty state instead.
    // So the gate is asserted on the section's exact heading — which the admin test above proves
    // is visible when the gate IS offered, so this absence cannot pass vacuously on a rename —
    // and on the control itself.
    await expect(page.getByRole("heading", { level: 1, name: "Requests" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Waiting for your approval" })).toHaveCount(0);
    await expect(page.getByRole("button", { name: /^approve$/i })).toHaveCount(0);

    // And the outcome that actually matters: nothing was acquired.
    await expect
      .poll(() => mock.state.enqueued, {
        message: "a member must never cause an acquisition (§19)",
        timeout: 2000,
      })
      .toEqual([]);
    expect(mock.state.proposals[0]?.status).toBe("submitted");
    expect(mock.state.approvalRequests).toEqual([]);
  });
});
