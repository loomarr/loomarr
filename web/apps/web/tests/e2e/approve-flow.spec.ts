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

    // The approvals surface is /queue's "Needs approval" tab (V27), not the origination
    // panel: `/suggest` folded into the Guide header (§12), and what moved there is
    // DESCRIBING a channel, not approving one. The tab is explicit in the URL because the
    // queue opens on "In flight" — landing on the wrong tab would fail for the wrong reason.
    await page.goto("/queue?tab=approval");

    // The tabpanel is where an admin acts on work submitted by others — the only path from a
    // proposal to a real acquisition. Scoped to the panel so a match cannot come from the
    // tab's own label.
    const queue = page.getByRole("tabpanel");
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

    // Even asking for the approval tab directly: a member must not get it.
    await page.goto("/queue?tab=approval");

    // The approvals tab is admin-only, so a member never sees it at all (§11).
    //
    // Asserted on the FULL TAB LIST rather than on a name matcher. The count renders inside
    // the tab ("Needs approval 1" once whitespace-normalized), so a name-based
    // `toHaveCount(0)` matches nothing and passes whether or not the tab is present — it was
    // verified to miss a deliberate sabotage that showed the tab to members. Comparing the
    // whole list cannot be fooled that way: it fails if an extra tab appears, whatever its
    // label happens to be.
    //
    // ⚠ Scoped to the Queue's own bar by its accessible NAME, and queried as LINKS. `NavTabs`
    // replaced `CountTabs`: every tab is a real `<Link>` marked with `aria-current`, so
    // `getByRole("tab")` now matches nothing and this assertion would have passed vacuously —
    // the exact failure mode the comment above exists to prevent, arriving by a different
    // route. The nav scope is needed because the app sidebar is also a list of links.
    await expect(page.getByRole("navigation", { name: /queue sections/i }).getByRole("link")).toHaveText([
      /in flight/i,
    ]);
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
