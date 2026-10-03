import AxeBuilder from "@axe-core/playwright";
import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import type { GuideHighlightsOutputBody } from "@loomarr/api/models/guideHighlightsOutputBody";
import type { GuideOutputBody } from "@loomarr/api/models/guideOutputBody";
import type { HouseholdViewingOutputBody } from "@loomarr/api/models/householdViewingOutputBody";
import type { TitleDTO } from "@loomarr/api/models/titleDTO";
import { expect, type Page, test } from "@playwright/test";
import { findClippedContent } from "./clipped-content";
import { installMockBackend } from "./mock-backend";

// Home at phone width (#1822 evidence, #1815): the approved proposal's geometry and state
// contract, driven against the real embedded SPA rather than inspected as class strings.
// phone-layout.spec.ts already gates every shell route against ancestor clipping generically;
// this file adds the proposal's own numbers — New this week's poster geometry at 390 and 320 px —
// and an axe pass, because neither is covered by the component tests in home/*.test.tsx (jsdom
// has no real CSS layout, and no axe harness exists at that test layer).
//
// Cached-refresh-failure is NOT covered here: `project/evidence/home-design-2026-10-01.md`
// explicitly marks it "for eventual implementation" — it names the future behaviour but the
// approved build does not implement it, so there is nothing running to assert against.
//
// The rail is a fixed 56 px below `md` (page-consistency.spec.ts pins this for every shell
// route), and Home's own content area takes 16 px of padding at this width (the geometry
// table's "16px padding; about 302px available at 390px") — so content width is
// `viewport - 56 - 32`. New this week's posters are two columns down to the 371px viewport the
// proposal's own prototype breaks at, one below it.
const RAIL_PX = 56;
const CONTENT_PADDING_PX = 32; // 16px each side
const GRID_GAP_PX = 12;
const WIDTH_TOLERANCE_PX = 4;

const contentWidth = (viewportWidth: number) => viewportWidth - RAIL_PX - CONTENT_PADDING_PX;
const expectedPosterWidth = (viewportWidth: number) => {
  const available = contentWidth(viewportWidth);
  return viewportWidth <= 371 ? available : (available - GRID_GAP_PX) / 2;
};
const closeTo = (actual: number, expected: number, label: string) =>
  expect(Math.abs(actual - expected), `${label}: ${actual} vs expected ${expected}`).toBeLessThanOrEqual(
    WIDTH_TOLERANCE_PX,
  );

const WIDTHS = [390, 320] as const;

const guideChannel = (
  channelId: string,
  number: number,
  name: string,
  programmeTitle: string,
  over: Partial<GuideOutputBody["channels"][number]> = {},
): GuideOutputBody["channels"][number] => ({
  airings: [
    {
      kind: "program",
      scheduleBlockId: `${channelId}-now`,
      series: "An invented series",
      title: programmeTitle,
      startMs: Date.now() - 5 * 60_000,
      stopMs: Date.now() + 25 * 60_000,
    },
  ],
  channelId,
  name,
  number,
  pendingCount: 0,
  status: "live",
  ...over,
});

const title = (
  key: string,
  name: string,
  channel?: { id: string; name: string; number: number },
): TitleDTO => ({
  key,
  mediaType: "movie",
  name,
  state: "available",
  channels: channel ? [channel] : undefined,
});

// A very long title, stressing wrap-not-truncate (the geometry table never allows a sliver or a
// document-edge overflow). Distinct strings for On now and New this week so each can be asserted
// unambiguously by text.
const LONG_ON_NOW_TITLE =
  "An unusually long programme title about a lighthouse keeper's evening above the city";
const LONG_POSTER_TITLE =
  "The remarkably long story of a lighthouse keeper and the extraordinary journey home and on";

interface HomeFixture {
  guide: GuideOutputBody;
  viewing: HouseholdViewingOutputBody;
  highlights: GuideHighlightsOutputBody;
  titles: TitleDTO[];
  channels: ChannelDTO[];
}

const POPULATED: HomeFixture = (() => {
  const fromMs = Date.now() - 60 * 60_000;
  const toMs = Date.now() + 3 * 60 * 60_000;
  return {
    guide: {
      fromMs,
      toMs,
      timezone: "UTC",
      channels: [
        guideChannel("ch-1", 7, "Night Signal", LONG_ON_NOW_TITLE),
        guideChannel("ch-2", 12, "Sunday Stories", "The Garden Next Door"),
        guideChannel("ch-3", 19, "Field Notes", "Along the Quiet Coast"),
      ],
    },
    viewing: {
      channels: [{ channelId: "ch-1", viewers: 1 }],
      // The caller is watching ch-1, so Recently tuned must prefer ch-2 (not already an active
      // own-viewing card) over duplicating ch-1.
      continueWatching: { channelId: "ch-2", tunedAt: new Date().toISOString() },
      scope: "household",
      viewers: [
        {
          channelId: "ch-1",
          device: "Living room TV",
          name: "Ada",
          since: new Date().toISOString(),
          userId: "u1",
          you: true,
        },
      ],
      watching: 1,
    },
    highlights: {
      fromMs,
      toMs,
      highlights: [
        {
          airing: {
            kind: "program",
            scheduleBlockId: "ch-3-tonight",
            series: "A sci-fi anthology",
            season: 4,
            title: "The pilot",
            startMs: toMs - 30 * 60_000,
            stopMs: toMs,
          },
          channelId: "ch-3",
          channelName: "Field Notes",
          channelNumber: 19,
          reason: "season_premiere",
        },
      ],
    },
    titles: [
      title("t1", "A Light in the Window", { id: "ch-2", name: "Sunday Stories", number: 12 }),
      title("t2", LONG_POSTER_TITLE),
      title("t3", "The River Returns"),
      title("t4", "Small Worlds"),
      title("t5", "Before the Morning"),
    ],
    channels: [
      {
        breakCount: 0,
        id: "ch-ren",
        inAppPlayable: true,
        lineup: [],
        name: "Weekend Rerun",
        number: 24,
        pendingCount: 0,
        policy: {},
        programCount: 0,
        revision: 1,
        slotCount: 0,
        status: "live",
        strategy: "sequential",
        createdAtMs: Date.now(),
      },
    ],
  };
})();

const QUIET: HomeFixture = {
  guide: {
    fromMs: Date.now() - 60 * 60_000,
    toMs: Date.now() + 3 * 60 * 60_000,
    timezone: "UTC",
    channels: [
      { airings: [], channelId: "ch-1", name: "Night Signal", number: 7, pendingCount: 0, status: "paused" },
      { airings: [], channelId: "ch-2", name: "Sunday Stories", number: 12, pendingCount: 0, status: "live" },
    ],
  },
  viewing: { channels: [], scope: "household", viewers: [], watching: 0 },
  highlights: { fromMs: 0, toMs: 0, highlights: [] },
  titles: [],
  channels: [],
};

const EMPTY: HomeFixture = {
  guide: { fromMs: 0, toMs: 0, channels: [] },
  viewing: { channels: [], scope: "household", viewers: [], watching: 0 },
  highlights: { fromMs: 0, toMs: 0, highlights: [] },
  titles: [],
  channels: [],
};

// Layers Home's own endpoints over installMockBackend's baseline (the established per-spec
// pattern — see guide-preview.spec.ts), rather than growing the shared mock for one page.
const mockHome = async (page: Page, fixture: HomeFixture) => {
  await page.route("**/v1/guide?*", (route) => route.fulfill({ json: fixture.guide }));
  await page.route("**/v1/household/viewing", (route) => route.fulfill({ json: fixture.viewing }));
  await page.route("**/v1/guide/highlights", (route) => route.fulfill({ json: fixture.highlights }));
  await page.route("**/v1/titles?*", (route) => {
    const url = new URL(route.request().url());
    if (!url.searchParams.has("since")) return route.fallback();
    return route.fulfill({ json: { titles: fixture.titles } });
  });
  await page.route("**/v1/channels", (route) => {
    if (route.request().method() !== "GET") return route.fallback();
    return route.fulfill({ json: { channels: fixture.channels } });
  });
};

const clipSummary = (clipped: Awaited<ReturnType<typeof findClippedContent>>) =>
  clipped.map((c) => `${c.element} "${c.text}" cut ${c.px}px on the ${c.side} by ${c.clippedBy}`);

const STATES = [
  { name: "populated", fixture: POPULATED },
  { name: "quiet household", fixture: QUIET },
  { name: "zero channels", fixture: EMPTY },
] as const;

for (const width of WIDTHS) {
  test.describe(`Home at ${width}px`, () => {
    test.use({ viewport: { width, height: 844 }, isMobile: true, hasTouch: true });

    for (const { name, fixture } of STATES) {
      test(`${name}: no section clips its children, and axe finds nothing serious`, async ({ page }) => {
        await installMockBackend(page, { authed: true, role: "admin" });
        await mockHome(page, fixture);
        await page.goto("/dashboard");
        await expect(page.getByRole("heading", { level: 1, name: "Home", exact: true })).toBeVisible();

        if (fixture === EMPTY) {
          await expect(page.getByText("Your first channel starts here")).toBeVisible();
        } else {
          await expect(page.getByText(`${fixture.guide.channels.length} channels`)).toBeVisible();
        }

        const clipped = await findClippedContent(page);
        expect(clipSummary(clipped), `Home (${name}) should not clip content at ${width}px`).toEqual([]);

        const results = await new AxeBuilder({ page }).analyze();
        const blocking = results.violations.filter(
          (violation) => violation.impact === "serious" || violation.impact === "critical",
        );
        expect(
          blocking,
          `Home (${name}) at ${width}px axe: ${blocking.map((violation) => violation.id).join(", ")}`,
        ).toEqual([]);
      });
    }

    test("New this week's posters hold the proposal's geometry, missing-art monogram, and a wrapped long title", async ({
      page,
    }) => {
      await installMockBackend(page, { authed: true, role: "admin" });
      await mockHome(page, POPULATED);
      await page.goto("/dashboard");
      await expect(page.getByRole("heading", { name: "New this week" })).toBeVisible();

      const arts = page.locator("[data-home-poster-art]");
      await expect(arts).toHaveCount(POPULATED.titles.length);

      const expectedWidth = expectedPosterWidth(width);
      for (let i = 0; i < POPULATED.titles.length; i++) {
        const box = await arts.nth(i).boundingBox();
        expect(box, `poster ${i} has a box`).not.toBeNull();
        closeTo(box?.width ?? 0, expectedWidth, `poster ${i} width at ${width}px`);
        // 2:3 artwork (the geometry table): height follows width exactly.
        closeTo(box?.height ?? 0, expectedWidth * 1.5, `poster ${i} height at ${width}px`);
        // No poster is a sliver: the defect the proposal names outright.
        expect(box?.width ?? 0, `poster ${i} is not a sliver`).toBeGreaterThan(80);
      }

      // t1 (index 0) names a channel: its monogram shows inside ITS OWN poster — scoped, since
      // the same channel's monogram also legitimately appears in On now and Recently tuned.
      // t2 (index 1, the long title) names none: the fallback's restrained line motif alone, no
      // invented monogram ("a channel monogram where available").
      await expect(arts.nth(0).getByText("SS", { exact: true })).toBeVisible();
      await expect(arts.nth(1).getByText(/^[A-Z]{2}$/)).toHaveCount(0);
      await expect(page.locator("[data-home-poster-art] img")).toHaveCount(0);
      await expect(page.getByText(LONG_POSTER_TITLE)).toBeVisible();
      // The on-now stress title wraps too, in its own section, at the same width.
      await expect(page.getByText(LONG_ON_NOW_TITLE)).toBeVisible();

      const clipped = await findClippedContent(page);
      expect(clipSummary(clipped), `New this week should not clip its posters at ${width}px`).toEqual([]);
    });
  });
}
