import type { GuideChannelTimeline } from "@loomarr/api/models/guideChannelTimeline";
import type { GuideOutputBody } from "@loomarr/api/models/guideOutputBody";
import { describe, expect, it } from "vitest";
import {
  DEFAULT_WINDOW_MINUTES,
  defaultGuideWindow,
  formatGuideEpisode,
  formatGuideTime,
  formatGuideTimeRange,
  GUIDE_BUCKET_MS,
  guideAiringLabel,
  guideChannelState,
  guideSelectionForChannel,
  guideWindow,
  LOOKBACK_MINUTES,
  layoutGuide,
  moveGuideSelection,
} from "./guide";

describe("guideWindow", () => {
  it("quantizes today's lookback so prefetch and render share a cache key", () => {
    const at = new Date(2026, 7, 23, 19, 42, 37, 912).getTime();
    const expectedFrom = Math.floor((at - LOOKBACK_MINUTES * 60_000) / GUIDE_BUCKET_MS) * GUIDE_BUCKET_MS;

    expect(defaultGuideWindow(at)).toEqual({
      from: expectedFrom,
      to: expectedFrom + DEFAULT_WINDOW_MINUTES * 60_000,
    });
    expect(defaultGuideWindow(at + 20_000)).toEqual(defaultGuideWindow(at));
  });

  it("uses local midnight for another day and honors explicit starts and shifts", () => {
    const at = new Date(2026, 7, 23, 19, 42).getTime();
    const tomorrow = new Date(2026, 7, 24, 0, 0).getTime();

    expect(guideWindow({ at, dayOffset: 1, windowMinutes: 120, hourShift: 0, startHour: null })).toEqual({
      from: tomorrow,
      to: tomorrow + 120 * 60_000,
    });
    expect(guideWindow({ at, dayOffset: 1, windowMinutes: 120, hourShift: -1, startHour: 7 })).toEqual({
      from: tomorrow + 6 * 3_600_000,
      to: tomorrow + 8 * 3_600_000,
    });
  });
});

describe("guide time formatting", () => {
  it("uses the household time zone instead of the viewer device time zone", () => {
    const at = Date.parse("2026-08-23T23:00:00Z");

    expect(formatGuideTime(at, "America/New_York")).toBe("7:00 PM");
    expect(formatGuideTime(at, "America/Los_Angeles")).toBe("4:00 PM");
    expect(formatGuideTimeRange(at, at + 30 * 60_000, "America/New_York")).toBe("7:00 PM–7:30 PM");
  });

  it("keeps season zero specials and partial episode identity", () => {
    expect(formatGuideEpisode(7, 2)).toBe("S07E02");
    expect(formatGuideEpisode(0, 1)).toBe("S00E01");
    expect(formatGuideEpisode(7)).toBe("S07");
    expect(formatGuideEpisode(undefined, 2)).toBe("E02");
    expect(formatGuideEpisode()).toBeUndefined();
  });
});

describe("guide presentation facts", () => {
  it("keeps series and episode title together with honest fallbacks", () => {
    expect(
      guideAiringLabel({
        scheduleBlockId: "episode",
        kind: "program",
        title: "Radioactive Man",
        series: "The Simpsons",
        startMs: 0,
        stopMs: 1,
      }),
    ).toBe("The Simpsons · Radioactive Man");
    expect(
      guideAiringLabel({
        scheduleBlockId: "pending",
        kind: "pending",
        title: "   ",
        startMs: 0,
        stopMs: 1,
      }),
    ).toBe("Coming soon");
  });

  it("keeps on-air truth separate from channel health", () => {
    const channel: Omit<GuideChannelTimeline, "status"> = {
      channelId: "classic-animation",
      name: "Classic Animation",
      number: 7,
      pendingCount: 0,
      airings: [],
    };

    expect(guideChannelState({ ...channel, status: "live" })).toEqual({ broadcast: "live", health: null });
    expect(guideChannelState({ ...channel, status: "drifted" })).toEqual({
      broadcast: "live",
      health: "drift",
    });
    expect(guideChannelState({ ...channel, status: "building" })).toEqual({
      broadcast: "reconciling",
      health: "creating",
    });
    expect(guideChannelState({ ...channel, status: "live", pendingCount: 2 })).toEqual({
      broadcast: "live",
      health: "pending-slots",
    });
    expect(guideChannelState({ ...channel, status: "detached" })).toEqual({
      broadcast: "off",
      health: "error",
    });
  });
});

describe("layoutGuide", () => {
  const source = {
    fromMs: 1_000,
    toMs: 5_000,
    timezone: "America/New_York",
    channels: [
      {
        channelId: "classic-animation",
        name: "Classic Animation",
        number: 7,
        pendingCount: 0,
        status: "live",
        airings: [
          {
            scheduleBlockId: "before",
            kind: "program",
            title: "Before",
            startMs: 0,
            stopMs: 1_000,
          },
          {
            scheduleBlockId: "radioactive-man",
            kind: "program",
            title: "Radioactive Man",
            series: "The Simpsons",
            season: 7,
            episode: 2,
            nominal: true,
            startMs: 0,
            stopMs: 3_000,
          },
          {
            scheduleBlockId: "after",
            kind: "program",
            title: "After",
            startMs: 4_000,
            stopMs: 6_000,
          },
          {
            scheduleBlockId: "invalid",
            kind: "flex",
            title: "Invalid",
            startMs: 4_500,
            stopMs: 4_500,
          },
        ],
      },
      {
        channelId: "empty",
        name: "Empty row",
        number: 8,
        pendingCount: 1,
        status: "building",
        airings: [],
      },
    ],
  } satisfies GuideOutputBody;

  it("derives clipped, pixel-independent geometry from the served window", () => {
    const layout = layoutGuide(source, 2_000);

    expect(layout).toMatchObject({
      source,
      fromMs: 1_000,
      toMs: 5_000,
      timezone: "America/New_York",
    });
    expect(layout.channels[0]?.airings).toHaveLength(2);
    expect(layout.channels[0]?.airings[0]).toMatchObject({
      source: source.channels[0]?.airings[1],
      channelId: "classic-animation",
      scheduleBlockId: "radioactive-man",
      startRatio: 0,
      widthRatio: 0.5,
      clippedStartMs: 1_000,
      clippedStopMs: 3_000,
      startsBeforeWindow: true,
      endsAfterWindow: false,
      isOnNow: true,
      progressRatio: 2 / 3,
    });
    expect(layout.channels[0]?.airings[1]).toMatchObject({
      startRatio: 0.75,
      widthRatio: 0.25,
      startsBeforeWindow: false,
      endsAfterWindow: true,
      isOnNow: false,
    });
  });

  it("preserves channel rows and generated source identity without inventing metadata", () => {
    const layout = layoutGuide(source, 6_000);

    expect(layout.channels).toHaveLength(2);
    expect(layout.channels[0]?.source).toBe(source.channels[0]);
    expect(layout.channels[0]?.airings[0]?.source.nominal).toBe(true);
    expect(layout.channels[1]).toEqual({ source: source.channels[1], airings: [] });
    expect(layout.channels[0]?.airings.every((airing) => airing.progressRatio === undefined)).toBe(true);
  });

  it("keeps the same time column when moving between channels", () => {
    const firstChannel = source.channels[0];
    const secondChannel = source.channels[1];
    expect(firstChannel).toBeDefined();
    expect(secondChannel).toBeDefined();
    if (!firstChannel || !secondChannel) return;

    const navigationSource = {
      ...source,
      channels: [
        firstChannel,
        {
          ...secondChannel,
          airings: [
            {
              scheduleBlockId: "adjacent-early",
              kind: "program",
              title: "Early",
              startMs: 1_000,
              stopMs: 1_800,
            },
            {
              scheduleBlockId: "adjacent-overlap",
              kind: "program",
              title: "Overlapping",
              startMs: 1_800,
              stopMs: 3_500,
            },
          ],
        },
      ],
    } satisfies GuideOutputBody;
    const layout = layoutGuide(navigationSource, 2_000);
    const selection = guideSelectionForChannel(layout, "classic-animation", 2_500);

    expect(selection).toEqual({
      channelId: "classic-animation",
      scheduleBlockId: "radioactive-man",
      anchorMs: 2_500,
    });
    expect(selection && moveGuideSelection(layout, selection, "down")).toEqual({
      selection: {
        channelId: "empty",
        scheduleBlockId: "adjacent-overlap",
        anchorMs: 2_500,
      },
    });
  });

  it("moves between adjacent airings and reports boundaries to the platform adapter", () => {
    const layout = layoutGuide(source, 2_000);
    const first = guideSelectionForChannel(layout, "classic-animation", 2_000);
    expect(first).toBeDefined();
    if (!first) return;

    const right = moveGuideSelection(layout, first, "right");
    expect(right.selection).toMatchObject({
      channelId: "classic-animation",
      scheduleBlockId: "after",
      anchorMs: 5_000,
    });
    expect(moveGuideSelection(layout, right.selection, "right")).toEqual({
      selection: right.selection,
      boundary: "right",
    });
    expect(moveGuideSelection(layout, first, "up")).toEqual({ selection: first, boundary: "up" });
  });

  it("keeps an empty channel row focusable during vertical traversal", () => {
    const layout = layoutGuide(source, 2_000);
    const first = guideSelectionForChannel(layout, "classic-animation", 2_000);
    expect(first).toBeDefined();
    if (!first) return;

    expect(moveGuideSelection(layout, first, "down")).toEqual({
      selection: { channelId: "empty", scheduleBlockId: undefined, anchorMs: 2_000 },
    });
  });
});
