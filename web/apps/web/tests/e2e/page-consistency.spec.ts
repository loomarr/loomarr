import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { installMockBackend } from "./mock-backend";

// Page-shell contract (frontend-design §6): Playwright drives the running SPA rather than
// inspecting class strings. A route may fill its body however it needs, but its navigation,
// semantic title, page-edge gutter, and mobile overflow behavior stay invariant.
const pages = [
  { path: "/dashboard", title: "Dashboard" },
  { path: "/guide", title: "Channels" },
  { path: "/queue/approval", title: "Queue" },
  { path: "/queue/flight", title: "Queue" },
  { path: "/queue/history", title: "Queue" },
  { path: "/filler", title: "Filler" },
  { path: "/filler/incoming", title: "Filler" },
  { path: "/filler/sources", title: "Filler" },
  { path: "/filler/settings", title: "Filler settings" },
  { path: "/people", title: "People" },
  { path: "/settings/connections", title: "Connections" },
  { path: "/settings/ai", title: "AI" },
  { path: "/settings/defaults", title: "Channel defaults" },
  { path: "/settings/security", title: "Security" },
  { path: "/settings/all", title: "All settings" },
  { path: "/settings/system/tasks", title: "Tasks" },
  { path: "/settings/system/playback", title: "Playback" },
  { path: "/settings/system/database", title: "Database" },
  { path: "/settings/system/backup", title: "Backup" },
  { path: "/settings/system/storage", title: "Storage" },
  { path: "/settings/system/diagnostics", title: "Diagnostics" },
  { path: "/settings/system/about", title: "About" },
  { path: "/help", title: "Help" },
  { path: "/account", title: "Your account" },
] as const;

const viewports = [
  { name: "desktop", width: 1280, height: 800, navWidth: 224 },
  { name: "mobile", width: 390, height: 844, navWidth: 56 },
] as const;

const sectionDestinations: Record<string, Array<{ navigation: string; current: string }>> = {
  "/queue/approval": [{ navigation: "Queue sections", current: "Needs approval" }],
  "/queue/flight": [{ navigation: "Queue sections", current: "In flight" }],
  "/queue/history": [{ navigation: "Queue sections", current: "History" }],
  "/settings/connections": [{ navigation: "Settings", current: "Connections" }],
  "/settings/ai": [{ navigation: "Settings", current: "AI" }],
  "/settings/defaults": [{ navigation: "Settings", current: "Defaults" }],
  "/settings/security": [{ navigation: "Settings", current: "Security" }],
  "/settings/all": [{ navigation: "Settings", current: "All settings" }],
  "/settings/system/tasks": [
    { navigation: "Settings", current: "System" },
    { navigation: "System settings", current: "Tasks" },
  ],
  "/settings/system/playback": [
    { navigation: "Settings", current: "System" },
    { navigation: "System settings", current: "Playback" },
  ],
  "/settings/system/database": [
    { navigation: "Settings", current: "System" },
    { navigation: "System settings", current: "Database" },
  ],
  "/settings/system/backup": [
    { navigation: "Settings", current: "System" },
    { navigation: "System settings", current: "Backup" },
  ],
  "/settings/system/storage": [
    { navigation: "Settings", current: "System" },
    { navigation: "System settings", current: "Storage" },
  ],
  "/settings/system/diagnostics": [
    { navigation: "Settings", current: "System" },
    { navigation: "System settings", current: "Diagnostics" },
  ],
  "/settings/system/about": [
    { navigation: "Settings", current: "System" },
    { navigation: "System settings", current: "About" },
  ],
};

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
      await expect(header, `${entry.path} should use PageHeader`).toHaveCount(1);
      if (entry.path === "/account") {
        // The account identity is a rail footer control, not a section of the product's
        // primary navigation. It still identifies the current page, while none of the
        // authored primary destinations claims to be active.
        await expect(primary.locator('a[data-status="active"]')).toHaveCount(0);
        await expect(page.getByRole("link", { name: "Your account" })).toHaveAttribute(
          "aria-current",
          "page",
        );
      } else {
        await expect(primary.locator('a[data-status="active"]')).toHaveCount(1);
      }

      for (const expected of sectionDestinations[entry.path] ?? []) {
        const navigation = page.getByRole("navigation", { name: expected.navigation, exact: true });
        const current = navigation.locator('a[aria-current="page"]');
        await expect(current).toHaveCount(1);
        await expect(current).toHaveAccessibleName(new RegExp(`^${expected.current}`));
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

      expect(Math.round(navBox?.width ?? 0), `${entry.path} ${viewport.name} nav width`).toBe(
        viewport.navWidth,
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
    { path: "/filler/sources", current: "Sources", title: "Filler" },
    { path: "/filler/incoming", current: "Incoming", title: "Filler" },
    { path: "/filler/library", current: "Library", title: "Filler" },
    { path: "/filler/manage", current: "Manage", title: "Filler" },
    // Compatibility deep link: settings is part of the canonical Manage destination.
    { path: "/filler/settings", current: "Manage", title: "Filler settings" },
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
    const contextualLink = page.getByRole("link", { name: "uses its own grounded selection" });
    await expect(contextualLink).toBeVisible();
    expect(await contextualLink.evaluate((link) => getComputedStyle(link).textDecorationLine)).toContain(
      "underline",
    );

    const results = await new AxeBuilder({ page }).analyze();
    const blocking = results.violations.filter(
      (violation) => violation.impact === "serious" || violation.impact === "critical",
    );
    expect(blocking, `${viewport.name} axe: ${blocking.map((violation) => violation.id).join(", ")}`).toEqual(
      [],
    );
  }

  expect(fillerRequests.filter((path) => path === "/v1/filler/attention").length).toBeGreaterThanOrEqual(
    viewports.length,
  );
  expect(fillerRequests).not.toContain("/v1/filler/incoming");
});
