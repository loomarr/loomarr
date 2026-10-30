import { expect, type Page, test } from "@playwright/test";
import { findClippedContent } from "./clipped-content";
import { installMockBackend } from "./mock-backend";
import { shellRoutes } from "./shell-routes";

// Phone layout gate (#1785): every shell route on a touch phone, failing on any interactive
// element or text box clipped by an ancestor. The page-shell contract's width check passed while
// the app was broken at this size, because the content was clipped inside overflow-hidden boxes,
// not pushed past the document edge.
//
// Two phones: 390 × 844 (the common size) and 320 × 640 (the narrowest still sold, and the short
// one). A list that fits the tall phone can be squeezed on the short one: Requests › Done clipped
// its last rows only at 320 × 640.
const PHONES = [
  { width: 390, height: 844 },
  { width: 320, height: 640 },
] as const;

const expectNothingClipped = async (page: Page, where: string, width: number) => {
  const clipped = await findClippedContent(page);
  expect(
    clipped.map((c) => `${c.element} "${c.text}" cut ${c.px}px on the ${c.side} by ${c.clippedBy}`),
    `${where} should not clip content at ${width} px`,
  ).toEqual([]);
};

for (const viewport of PHONES) {
  test.describe(`at ${viewport.width} × ${viewport.height}`, () => {
    test.use({ viewport, isMobile: true, hasTouch: true });

    for (const route of shellRoutes) {
      test(`${route.path} shows all its content`, async ({ page }) => {
        await installMockBackend(page, { authed: true, role: "admin", fillerEnabled: true });
        await page.goto(route.path);
        await expect(page.getByRole("heading", { level: 1, name: route.title, exact: true })).toBeVisible();
        await expectNothingClipped(page, route.path, viewport.width);
      });
    }

    // The shell routes above show Requests empty. Here each tab has rows: approval cards with
    // their actions, the requests on their way, and more built channels than the panel is tall.
    // Each tab waits for its LAST row, so the check runs on the whole list.
    for (const tab of [
      { path: "/requests/needs-you", last: { role: "button", name: "Review & edit picks" } },
      { path: "/requests/in-progress", last: { role: "link", name: /^Open / } },
      {
        path: "/requests/done",
        last: { role: "link", name: "Open channel for Weekend cartoons about inventors" },
      },
    ] as const) {
      test(`${tab.path} shows every row`, async ({ page }) => {
        await installMockBackend(page, {
          authed: true,
          role: "admin",
          pendingProposal: true,
          requests: true,
        });
        await page.goto(tab.path);
        await expect(
          page.getByRole("tabpanel").getByRole(tab.last.role, { name: tab.last.name }).last(),
        ).toBeAttached();
        await expectNothingClipped(page, tab.path, viewport.width);
        // The check skips what a closed <details> hides, so open each one and look again. The first
        // closed one in document order is always showing: an outer disclosure opens before its own.
        const closed = page.locator("details:not([open]) > summary");
        while ((await closed.count()) > 0) await closed.first().tap();
        await expectNothingClipped(page, `${tab.path} with every disclosure open`, viewport.width);
      });
    }

    // A channel's pages as an admin, whose header holds the editable name: 25 characters, which a
    // one-line field cut at both sizes. Its tab bar is one line that scrolls, so the active tab
    // must be wholly in view, not merely reachable.
    for (const section of ["info", "programming", "filler", "danger"]) {
      test(`/channels/ch-1/${section} shows the whole name and its tab`, async ({ page }) => {
        await installMockBackend(page, { authed: true, role: "admin", channelDetail: true });
        await page.goto(`/channels/ch-1/${section}`);
        await expect(page.getByLabel("Channel name")).toHaveValue("Saturday Morning Cartoons");
        await expectNothingClipped(page, `/channels/ch-1/${section}`, viewport.width);

        const bar = page.getByRole("navigation", { name: "Channel sections" });
        const active = await bar.locator("[aria-current=page]").boundingBox();
        const frame = await bar.boundingBox();
        expect(
          active && frame && active.x >= frame.x - 1 && active.x + active.width <= frame.x + frame.width + 1,
        ).toBe(true);
      });
    }
  });
}

// The phone Guide (#1785, native mocks 5d/5f, decision N6): the compact grid with its filter chips,
// a tapped programme docked with Watch, and nothing clipped.
test.describe("phone Guide", () => {
  test.use({ viewport: PHONES[0], isMobile: true, hasTouch: true });

  test("/guide at phone width is the compact grid, and a tapped programme docks with Watch", async ({
    page,
  }) => {
    await installMockBackend(page, { authed: true, role: "admin", guideChannels: 12 });
    await page.goto("/guide");
    const filters = page.getByRole("toolbar", { name: "Guide filters" });
    await expect(filters.getByRole("button", { name: "All, 12 channels" })).toBeVisible();
    await expect(filters.getByRole("button", { name: "Favorites, 2 channels" })).toBeVisible();
    await expect(filters.getByRole("button", { name: "Recent, 1 channels" })).toBeVisible();

    const cell = page.getByRole("button", { name: /^5 Guide channel 5, A series 5 · Episode 2,/ });
    await cell.tap();
    expect((await cell.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    const dock = page.getByRole("region", { name: "Selected programme" });
    await expect(dock).toContainText("A series 5 “Episode 2”");

    await expectNothingClipped(page, "/guide", PHONES[0].width);

    await filters.getByRole("button", { name: "Favorites, 2 channels" }).tap();
    await expect(page.getByRole("button", { name: /^5 Guide channel 5,/ })).toHaveCount(0);
    await expect(page.getByRole("button", { name: /^2 Guide channel 2,/ }).first()).toBeVisible();

    await dock.getByRole("button", { name: "Watch 5 Guide channel 5" }).tap();
    await expect(page).toHaveURL(/\/watch\/ch-5$/);
  });
});
