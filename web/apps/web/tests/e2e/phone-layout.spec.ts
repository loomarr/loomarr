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
