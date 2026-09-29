import { expect, test } from "@playwright/test";
import { findClippedContent } from "./clipped-content";
import { installMockBackend } from "./mock-backend";
import { shellRoutes } from "./shell-routes";

// Phone layout gate (#1785): every shell route at 390 × 844 on a touch phone, failing on any
// interactive element or text box clipped by an ancestor. The page-shell contract's width check
// passed while the app was broken at this size, because the content was clipped inside
// overflow-hidden boxes, not pushed past the document edge.
test.use({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });

for (const route of shellRoutes) {
  test(`${route.path} shows all its content at phone width`, async ({ page }) => {
    await installMockBackend(page, { authed: true, role: "admin", fillerEnabled: true });
    await page.goto(route.path);
    await expect(page.getByRole("heading", { level: 1, name: route.title, exact: true })).toBeVisible();

    const clipped = await findClippedContent(page);
    expect(
      clipped.map((c) => `${c.element} "${c.text}" cut ${c.px}px on the ${c.side} by ${c.clippedBy}`),
      `${route.path} should not clip content at 390 px`,
    ).toEqual([]);
  });
}

// The phone Guide (#1785, native mocks 5d/5f, decision N6): the compact grid with its filter chips,
// a tapped programme docked with Watch, and nothing clipped.
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

  const clipped = await findClippedContent(page);
  expect(
    clipped.map((c) => `${c.element} "${c.text}" cut ${c.px}px on the ${c.side} by ${c.clippedBy}`),
    "/guide should not clip content at 390 px",
  ).toEqual([]);

  await filters.getByRole("button", { name: "Favorites, 2 channels" }).tap();
  await expect(page.getByRole("button", { name: /^5 Guide channel 5,/ })).toHaveCount(0);
  await expect(page.getByRole("button", { name: /^2 Guide channel 2,/ }).first()).toBeVisible();

  await dock.getByRole("button", { name: "Watch 5 Guide channel 5" }).tap();
  await expect(page).toHaveURL(/\/channels\/ch-5\/watch$/);
});
