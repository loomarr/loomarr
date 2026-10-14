import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { installMockBackend } from "./mock-backend";
import { shellRoutes as pages } from "./shell-routes";

// Page-shell contract (frontend-design §6): Playwright drives the running SPA rather than
// inspecting class strings. A route may fill its body however it needs, but its navigation,
// semantic title, page-edge gutter, and mobile overflow behavior stay invariant. Document width
// alone misses content clipped inside an overflow-hidden box; phone-layout.spec.ts gates that.

// Desktop's rail is a fixed-width vertical column; below `md` it's replaced by PhoneBottomBar,
// a full-width horizontal bar (#1785 Alt A) — so the geometry invariant each holds is its OWN
// fixed dimension, width for the rail and height for the bar, not the same one.
const viewports = [
  { name: "desktop", width: 1280, height: 800, navSize: 224, navSizeTolerance: 0 },
  // 49 (idiom height) + 1 (hairline border), ±5 for sub-pixel line-height rounding: the
  // SELECTED label renders at font-weight 600 vs 500 for the rest, so a page with a lit tab
  // (bold) measures a few px taller than one with none lit (every label regular weight).
  { name: "mobile", width: 390, height: 844, navSize: 52, navSizeTolerance: 5 },
] as const;

const sectionDestinations: Record<string, Array<{ navigation: string; current: string }>> = {};

const settingsPages = new Set(
  pages.map((entry) => entry.path).filter((path) => path.startsWith("/settings/")),
);

// The primary destination each shell route belongs to, by the label both layouts give it. /account
// belongs to none: it's the rail's footer identity, not a section of primary navigation.
const primaryDestinations: ReadonlyArray<{ prefix: string; label: string }> = [
  { prefix: "/dashboard", label: "Home" },
  { prefix: "/guide", label: "Guide" },
  { prefix: "/requests", label: "Requests" },
  { prefix: "/filler", label: "Filler" },
  { prefix: "/people", label: "People" },
  { prefix: "/settings", label: "Settings" },
  { prefix: "/help", label: "Help" },
];

const primaryDestinationFor = (path: string): string | undefined =>
  primaryDestinations.find(({ prefix }) => path === prefix || path.startsWith(`${prefix}/`))?.label;

test("pages share one navigation and header geometry at desktop and mobile widths", async ({ page }) => {
  await installMockBackend(page, { authed: true, role: "admin" });

  for (const viewport of viewports) {
    await page.setViewportSize(viewport);

    for (const entry of pages) {
      await page.goto(entry.path);

      const main = page.getByRole("main");
      const primary = page.getByRole("navigation", { name: "Primary" });
      const header = page.locator("[data-page-header]");
      const title = page.getByRole("heading", { level: 1, name: entry.title, exact: true });

      await expect(title, `${entry.path} should expose its page title`).toBeVisible();
      await expect(page.getByRole("heading", { level: 1 })).toHaveCount(1);
      if (entry.path === "/filler/settings") {
        await expect(
          page.getByRole("heading", { level: 2, name: "Filler settings", exact: true }),
        ).toBeVisible();
      }
      await expect(header, `${entry.path} should use PageHeader`).toHaveCount(1);
      // One invariant for every layout: the Primary landmark lights at most one destination, and
      // it lights the route's own destination exactly when that destination has a slot in it.
      // Desktop's rail is `<Link>`s marked `data-status="active"`; PhoneBottomBar is `TabBar`'s
      // `role="tab"` pattern (shared with native, #1785 Alt A) marked `aria-selected`. The rail
      // has a slot for every destination; the bar only for Home/Watch/Guide/Requests, with the
      // rest behind a More button that by design is never lit.
      const items = primary.getByRole("link").or(primary.getByRole("tab"));
      // PhoneBottomBar is lazy: wait for real items, not the Suspense placeholder's empty row.
      await expect(items.first(), `${entry.path} primary navigation renders`).toBeVisible();
      const active = primary.locator('a[data-status="active"], [role="tab"][aria-selected="true"]');
      const destination = primaryDestinationFor(entry.path);
      const slots = destination
        ? primary
            .getByRole("link", { name: new RegExp(`^${destination}\\b`) })
            .or(primary.getByRole("tab", { name: new RegExp(`^${destination}\\b`) }))
        : undefined;
      const slotCount = slots ? await slots.count() : 0;
      expect(slotCount, `${entry.path} ${destination} has at most one primary slot`).toBeLessThanOrEqual(1);
      await expect(active, `${entry.path} lights only its own destination`).toHaveCount(slotCount);
      if (destination && slotCount === 1) {
        await expect(active).toHaveAccessibleName(new RegExp(`^${destination}\\b`));
      }
      // The account identity is a rail footer control: it identifies the current page wherever
      // the layout shows it, while no primary destination claims /account.
      const account = page.getByRole("link", { name: "Your account" });
      if (entry.path === "/account" && (await account.count()) > 0) {
        await expect(account).toHaveAttribute("aria-current", "page");
      }

      for (const expected of sectionDestinations[entry.path] ?? []) {
        const navigation = page.getByRole("navigation", { name: expected.navigation, exact: true });
        const current = navigation.locator('a[aria-current="page"]');
        await expect(current).toHaveCount(1);
        await expect(current).toHaveAccessibleName(new RegExp(`^${expected.current}`));
      }

      if (settingsPages.has(entry.path)) {
        const location = page.getByRole("navigation", { name: "Settings location" });
        await expect(location.getByRole("link", { name: "Settings", exact: true })).toHaveAttribute(
          "href",
          "/settings",
        );
        await expect(page.getByRole("link", { name: "Find another setting" })).toHaveAttribute(
          "href",
          "/settings",
        );
        if (entry.path.startsWith("/settings/system/")) {
          await expect(location).toContainText("This server");
        }
      }

      const [mainBox, navBox, headerBox, titleBox] = await Promise.all([
        main.boundingBox(),
        primary.boundingBox(),
        header.boundingBox(),
        title.boundingBox(),
      ]);
      expect(mainBox, `${entry.path} main bounds`).not.toBeNull();
      expect(navBox, `${entry.path} nav bounds`).not.toBeNull();
      expect(headerBox, `${entry.path} header bounds`).not.toBeNull();
      expect(titleBox, `${entry.path} title bounds`).not.toBeNull();

      const navDimension = viewport.name === "mobile" ? (navBox?.height ?? 0) : (navBox?.width ?? 0);
      const navLabel = `${entry.path} ${viewport.name} nav ${viewport.name === "mobile" ? "height" : "width"}`;
      expect(Math.round(navDimension), navLabel).toBeGreaterThanOrEqual(
        viewport.navSize - viewport.navSizeTolerance,
      );
      expect(Math.round(navDimension), navLabel).toBeLessThanOrEqual(
        viewport.navSize + viewport.navSizeTolerance,
      );
      expect(Math.round(headerBox?.x ?? 0), `${entry.path} header starts at main edge`).toBe(
        Math.round(mainBox?.x ?? 0),
      );
      expect(Math.round(headerBox?.width ?? 0), `${entry.path} header spans the page`).toBe(
        Math.round(mainBox?.width ?? 0),
      );
      expect(Math.round(titleBox?.x ?? 0), `${entry.path} title uses the 24px gutter`).toBe(
        Math.round((mainBox?.x ?? 0) + 24),
      );

      const overflowsHorizontally = await main.evaluate(
        (element) => element.scrollWidth > element.clientWidth + 1,
      );
      expect(overflowsHorizontally, `${entry.path} should fit the ${viewport.name} viewport`).toBe(false);
    }
  }
});

test("Filler stays simple, discoverable, and accessible at desktop and mobile widths", async ({ page }) => {
  await installMockBackend(page, { authed: true, role: "admin", fillerEnabled: true });
  const fillerRequests: string[] = [];
  page.on("request", (request) => {
    const path = new URL(request.url()).pathname;
    if (path.startsWith("/v1/filler/")) fillerRequests.push(path);
  });
  const destinations = [
    { path: "/filler", current: "Overview", title: "Filler" },
    // Sources and Incoming open from the Manage hub, so they light Manage (#1659).
    { path: "/filler/sources", current: "Manage", title: "Filler" },
    { path: "/filler/incoming", current: "Manage", title: "Filler" },
    { path: "/filler/library", current: "Library", title: "Filler" },
    { path: "/filler/manage", current: "Manage", title: "Filler" },
    // Focused settings retain the same Filler workspace header and Manage destination.
    { path: "/filler/settings", current: "Manage", title: "Filler" },
  ] as const;

  for (const viewport of viewports) {
    await page.setViewportSize(viewport);
    if (page.url() !== "about:blank") await page.evaluate(() => localStorage.clear());

    for (const destination of destinations) {
      await page.goto(destination.path);

      const sections = page.getByRole("navigation", { name: "Filler sections" });
      await expect(
        page.getByRole("heading", { level: 1, name: destination.title, exact: true }),
      ).toBeVisible();
      await expect(page.getByRole("heading", { level: 1 })).toHaveCount(1);
      if (destination.path === "/filler/settings") {
        await expect(page.getByRole("heading", { name: "Filler settings", exact: true })).toBeVisible();
        await expect(page.getByRole("link", { name: /^Automatic downloads/ })).toBeVisible();
      }
      await expect(sections.locator('a[aria-current="page"]')).toHaveCount(1);
      await expect(
        sections.getByRole("link", { name: new RegExp(`^${destination.current}`) }),
      ).toHaveAttribute("aria-current", "page");
      await expect(page.locator("[data-sonner-toast]")).toHaveCount(0);

      const navigationFits = await sections.evaluate((navigation) => {
        const bounds = navigation.getBoundingClientRect();
        return [...navigation.querySelectorAll("a")].every((link) => {
          const linkBounds = link.getBoundingClientRect();
          return linkBounds.left >= bounds.left - 1 && linkBounds.right <= bounds.right + 1;
        });
      });
      expect(navigationFits, `${destination.path} destinations fit at ${viewport.name} width`).toBe(true);
    }

    await page.goto("/filler");
    await expect(page.getByText("Add filler to get started")).toBeVisible();
    await expect(page.getByText("Filler is working on its own")).toHaveCount(0);
    await expect(page.getByText("Each channel picks what fits from the shared library.")).toBeVisible();

    const results = await new AxeBuilder({ page }).analyze();
    const blocking = results.violations.filter(
      (violation) => violation.impact === "serious" || violation.impact === "critical",
    );
    expect(blocking, `${viewport.name} axe: ${blocking.map((violation) => violation.id).join(", ")}`).toEqual(
      [],
    );

    await page.goto("/filler/sources");
    await expect(page.getByRole("heading", { level: 2, name: "Where filler comes from" })).toBeVisible();
    if (viewport.name === "mobile") {
      for (const name of [
        "/data/filler/a-deliberately-long-folder-name",
        "Archive.org",
        "Classic television commercials from a deliberately long collection name",
      ]) {
        const width = await page
          .getByText(name, { exact: true })
          .evaluate((element) => Math.round(element.getBoundingClientRect().width));
        expect(width, `${name} keeps readable width`).toBeGreaterThan(180);
      }
    }
  }

  expect(fillerRequests.filter((path) => path === "/v1/filler/incoming").length).toBeGreaterThanOrEqual(
    viewports.length,
  );
  expect(fillerRequests).not.toContain("/v1/filler/attention");
});

test("Filler keeps member navigation on member-readable routes", async ({ page }) => {
  await installMockBackend(page, { authed: true, role: "member", fillerEnabled: true });
  const privateReads: string[] = [];
  page.on("request", (request) => {
    const path = new URL(request.url()).pathname;
    if (path === "/v1/filler/incoming" || path === "/v1/settings") privateReads.push(path);
  });

  await page.goto("/filler/incoming");
  await expect(page).toHaveURL(/\/filler\/library$/);
  const sections = page.getByRole("navigation", { name: "Filler sections" });
  await expect(sections.locator('a[aria-current="page"]')).toHaveCount(1);
  await expect(sections.getByRole("link", { name: /^Library/ })).toHaveAttribute("aria-current", "page");
  await expect(sections.getByRole("link", { name: "Incoming" })).toHaveCount(0);
  await expect(sections.getByRole("link", { name: "Sources" })).toHaveCount(0);

  await page.goto("/filler/settings/details");
  await expect(page).toHaveURL(/\/filler\/manage$/);
  // The web mock's Manage list (#1659): a member sees the tools, but Settings has no Open.
  const tools = page.getByRole("region", { name: "Filler tools" });
  await expect(tools).toBeVisible();
  await expect(tools.getByRole("link", { name: "Open Settings" })).toHaveCount(0);
  expect(privateReads).toEqual([]);
});
