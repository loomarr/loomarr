import type { GuideOutputBody } from "@loomarr/api/models/guideOutputBody";
import { expect, type Page, test } from "@playwright/test";
import { installMockBackend } from "./mock-backend";

const installGuide = async (page: Page) => {
  await installMockBackend(page, { authed: true, role: "admin", guideChannels: 100 });
  await page.route("**/v1/guide?*", async (route) => {
    const url = new URL(route.request().url());
    const fromMs = Number(url.searchParams.get("from"));
    const toMs = Number(url.searchParams.get("to"));
    const minute = 60_000;
    const body: GuideOutputBody = {
      fromMs,
      toMs,
      channels: Array.from({ length: 100 }, (_, i) => ({
        channelId: `ch-${i + 1}`,
        name: `Guide channel ${i + 1}`,
        number: i + 1,
        status: "live",
        pendingCount: 0,
        airings: [
          {
            kind: "program",
            scheduleBlockId: `first-${i}`,
            title: `First programme ${i + 1}`,
            description: "An invented programme for interaction evidence.",
            startMs: fromMs,
            stopMs: fromMs + 30 * minute,
          },
          {
            kind: "filler",
            scheduleBlockId: `break-${i}`,
            title: "Break",
            startMs: fromMs + 30 * minute,
            stopMs: fromMs + 32 * minute,
            pod: {
              entries: [
                { name: "Moonlight station ident", kind: "station_id", durationMs: 30_000 },
                { name: "Garden safety message", kind: "psa", durationMs: 90_000 },
              ],
            },
          },
          {
            kind: "program",
            scheduleBlockId: `middle-${i}`,
            title: `Middle programme ${i + 1}`,
            startMs: fromMs + 32 * minute,
            stopMs: toMs - 10 * minute,
          },
          {
            kind: "program",
            scheduleBlockId: `last-${i}`,
            title: `Final programme ${i + 1}`,
            startMs: toMs - 10 * minute,
            stopMs: toMs,
          },
        ],
      })),
    };
    await route.fulfill({ json: body });
  });
};

for (const width of [900, 1280, 1920]) {
  test(`desktop Guide previews use the full ${width}px frame and fit viewport edges`, async ({
    page,
  }, info) => {
    await page.setViewportSize({ width, height: 900 });
    await installGuide(page);
    await page.goto("/guide");
    const first = page.getByRole("button", { name: /^First programme 1,/ });
    await expect(first).toBeVisible();
    await expect(page.getByRole("tooltip")).toHaveCount(0);
    // No permanent detail column: the last timeline block reaches the content frame's right edge.
    const last = page.getByRole("button", { name: /^Final programme 1,/ });
    await last.scrollIntoViewIfNeeded();
    const lastBox = await last.boundingBox();
    expect(lastBox!.x + lastBox!.width).toBeGreaterThan(width - 35);
    await last.hover();
    const preview = page.getByRole("tooltip");
    await expect(preview).toContainText("Final programme 1");
    const box = await preview.boundingBox();
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width).toBeLessThanOrEqual(width);
    expect(box!.y).toBeGreaterThanOrEqual(0);
    expect(box!.y + box!.height).toBeLessThanOrEqual(900);
    await preview.hover();
    await page.waitForTimeout(350);
    await expect(preview).toBeVisible();
    await page.screenshot({ path: info.outputPath(`guide-${width}.png`) });
    await page.keyboard.press("Escape");
    await expect(preview).toHaveCount(0);
    await page.waitForTimeout(350);
    await expect(preview).toHaveCount(0);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  });
}

test("keyboard and short filler previews preserve navigation, supplied identity, and actions", async ({
  page,
}, info) => {
  await page.setViewportSize({ width: 1280, height: 900 });
  await installGuide(page);
  await page.goto("/guide");
  const first = page.getByRole("button", { name: /^First programme 1,/ });
  await first.focus();
  const preview = page.getByRole("tooltip");
  await expect(preview).toContainText("First programme 1");
  await expect(first).toBeFocused();
  await page.keyboard.press("ArrowRight");
  await expect(preview).toContainText("Moonlight station ident");
  await expect(preview).toContainText("Garden safety message");
  const block = page.getByRole("button", { name: /^Break,/ }).first();
  await expect(block).toBeFocused();
  expect((await block.boundingBox())!.width).toBeLessThan(20);
  await page.screenshot({ path: info.outputPath("guide-short-filler.png") });
  await page.keyboard.press("Escape");
  await expect(preview).toHaveCount(0);
  await expect(block).toBeFocused();
  // Typeahead still reaches virtualised rows.
  await page.keyboard.type("100");
  await expect(page.getByText("Guide channel 100", { exact: true }).first()).toBeVisible();
  await expect(preview).toContainText("Guide channel 100");
  const box = await preview.boundingBox();
  expect(box!.y + box!.height).toBeLessThanOrEqual(900);
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/\/channels\/ch-100\/watch$/);
});
