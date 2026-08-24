import type { GuideOutputBody } from "@loomarr/api/models/guideOutputBody";
import { describe, expect, it } from "vitest";
import {
  DEFAULT_WINDOW_MINUTES,
  defaultGuideWindow,
  GUIDE_BUCKET_MS,
  guideWindow,
  LOOKBACK_MINUTES,
  layoutGuide,
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
});
