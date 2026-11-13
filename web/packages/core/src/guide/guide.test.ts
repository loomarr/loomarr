import type { GuideChannelTimeline } from "@loomarr/api/models/guideChannelTimeline";
import type { GuideOutputBody } from "@loomarr/api/models/guideOutputBody";
import { describe, expect, it } from "vitest";
import {
  DEFAULT_WINDOW_MINUTES,
  defaultGuideWindow,
  formatGuideEpisode,
  formatGuideTime,
  formatGuideTimeRange,
  formatProgrammeTitle,
  GUIDE_BUCKET_MS,
  guideBreakProgramme,
  guideChannelForTypeahead,
  guideChannelState,
  guideDayOffset,
  guideFillerEntryLabel,
  guideFillerNowLabel,
  guideHourOf,
  guideSelectionForChannel,
  guideWaitingForTitles,
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

  // The axis reads in the household's zone, so its days and hours are that zone's wall clock,
  // whatever zone the device is in, and a day a clock change makes 23 or 25 hours long is one day.
  describe("in the household's time zone", () => {
    const ny = "America/New_York";
    const win = (at: string, dayOffset: number, startHour: number | null) =>
      guideWindow({
        at: Date.parse(at),
        dayOffset,
        windowMinutes: 60,
        hourShift: 0,
        startHour,
        timeZone: ny,
      });

    it("starts at the household's hour, not the device's", () => {
      // Noon in New York; Start at 8 PM is 8 PM there.
      expect(win("2026-08-23T16:00:00Z", 0, 20).from).toBe(Date.parse("2026-08-24T00:00:00Z"));
    });

    it("counts days from the household's date", () => {
      // 10 PM on the 23rd in New York is already the 24th in UTC; tomorrow is the 24th there.
      expect(win("2026-08-24T02:00:00Z", 1, null).from).toBe(Date.parse("2026-08-24T04:00:00Z"));
    });

    it("keeps the wall clock across a clock change", () => {
      // Saturday noon, the day before clocks go back: 8 PM tomorrow is 8 PM EST, and two days on
      // is Monday's midnight, not Sunday 11 PM.
      expect(win("2026-10-31T16:00:00Z", 1, 20).from).toBe(Date.parse("2026-11-02T01:00:00Z"));
      expect(win("2026-10-31T16:00:00Z", 2, null).from).toBe(Date.parse("2026-11-02T05:00:00Z"));
    });

    // As a Date or Temporal ("compatible") reads a wall time a clock change skips or repeats: a
    // skipped hour moves forward by the gap, a repeated hour is its first occurrence.
    it("starts at an hour a clock change skips or repeats as a clock would", () => {
      // 8 March 2026: 2 AM does not exist in New York (EST jumps to EDT); start at 2 AM is 3 AM EDT.
      expect(win("2026-03-07T17:00:00Z", 1, 2).from).toBe(Date.parse("2026-03-08T07:00:00Z"));
      // 1 November 2026: 1 AM happens twice; start at 1 AM is the first, 1 AM EDT.
      expect(win("2026-10-31T16:00:00Z", 1, 1).from).toBe(Date.parse("2026-11-01T05:00:00Z"));
      // The hours either side are untouched.
      expect(win("2026-03-07T17:00:00Z", 1, 1).from).toBe(Date.parse("2026-03-08T06:00:00Z"));
      expect(win("2026-03-07T17:00:00Z", 1, 3).from).toBe(Date.parse("2026-03-08T07:00:00Z"));
    });

    it("reads a moment's day offset and hour on the household's clock", () => {
      const now = Date.parse("2026-10-31T16:00:00Z");
      expect(guideDayOffset(Date.parse("2026-11-02T05:00:00Z"), now, ny)).toBe(2);
      expect(guideDayOffset(Date.parse("2026-11-01T03:30:00Z"), now, ny)).toBe(0);
      expect(guideHourOf(Date.parse("2026-11-02T01:00:00Z"), ny)).toBe(20);
    });
  });
});

describe("guide time formatting", () => {
  it("uses the household time zone instead of the viewer device time zone", () => {
    const at = Date.parse("2026-08-23T23:00:00Z");

    expect(formatGuideTime(at, "America/New_York")).toBe("7:00 PM");
    expect(formatGuideTime(at, "America/Los_Angeles")).toBe("4:00 PM");
    expect(formatGuideTimeRange(at, at + 30 * 60_000, "America/New_York")).toBe("7:00–7:30 PM");
  });

  it("writes AM/PM once when both ends share it, and on both ends when they differ", () => {
    const evening = Date.parse("2026-08-24T00:50:00Z"); // 8:50 PM in New York
    expect(formatGuideTimeRange(evening, evening + 49 * 60_000, "America/New_York")).toBe("8:50–9:39 PM");
    const lateNight = Date.parse("2026-08-24T03:30:00Z"); // 11:30 PM in New York
    expect(formatGuideTimeRange(lateNight, lateNight + 45 * 60_000, "America/New_York")).toBe(
      "11:30 PM–12:15 AM",
    );
    const lateMorning = Date.parse("2026-08-24T15:30:00Z"); // 11:30 AM in New York
    expect(formatGuideTimeRange(lateMorning, lateMorning + 45 * 60_000, "America/New_York")).toBe(
      "11:30 AM–12:15 PM",
    );
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
  it("names an airing one way everywhere: series, a dot, the title (#2748)", () => {
    const airing = { kind: "program" as const, title: " The Mad Scientist ", series: " Superman " };
    expect(formatProgrammeTitle(airing)).toBe("Superman · The Mad Scientist");
    expect(formatGuideEpisode(1, 1)).toBe("S01E01");
  });

  it("keeps series and episode title together with honest fallbacks", () => {
    expect(
      formatProgrammeTitle({ kind: "program", title: "The Seventh Postcard", series: "Kettle Kids" }),
    ).toBe("Kettle Kids · The Seventh Postcard");
    expect(formatProgrammeTitle({ kind: "pending", title: "   " })).toBe("Coming soon");
  });

  it("labels a break from the clip's viewer name, or the break's own label without one (#1980)", () => {
    expect(guideFillerNowLabel({ name: "Fizzbolt" })).toBe("Commercials · Fizzbolt");
    expect(guideFillerNowLabel({ name: "  " })).toBe("Commercials");
    expect(guideFillerNowLabel()).toBe("Commercials");
    expect(guideFillerEntryLabel({ role: "station_id", name: "Fizzbolt" })).toBe("Fizzbolt · station id");
    expect(guideFillerEntryLabel({ role: "commercial", name: "" })).toBe("Commercial");
    // The client never invents a name: what the server withheld stays withheld.
    for (const label of [
      guideFillerNowLabel({ name: "" }),
      guideFillerEntryLabel({ role: "bumper", name: "" }),
    ]) {
      expect(label).not.toMatch(/^[a-z0-9]+(?:[-_.][a-z0-9]+)+$|\.(?:mp4|mkv)$/);
    }
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
            scheduleBlockId: "seventh-postcard",
            kind: "program",
            title: "The Seventh Postcard",
            series: "Kettle Kids",
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
      scheduleBlockId: "seventh-postcard",
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
      scheduleBlockId: "seventh-postcard",
      anchorMs: 2_500,
    });
    expect(selection && moveGuideSelection(layout, selection, "down")).toEqual({
      selection: {
        channelId: "empty",
        scheduleBlockId: "adjacent-overlap",
        anchorMs: 2_500,
      },
    });

    const boundarySelection = guideSelectionForChannel(layout, "empty", 1_800);
    expect(boundarySelection?.scheduleBlockId).toBe("adjacent-overlap");
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

describe("breaks and downloading channels (#2727, #2761)", () => {
  const block = (kind: "program" | "filler" | "pending", startMs: number, stopMs: number) => ({
    kind,
    startMs,
    stopMs,
  });

  it("names the programme a break leads back to", () => {
    const airings = [block("program", 0, 10), block("filler", 10, 12), block("program", 12, 40)];
    expect(guideBreakProgramme(airings, airings[1] as (typeof airings)[number])).toBe(airings[2]);
  });

  it("has no programme for a break nothing follows", () => {
    const airings = [block("filler", 0, 2), block("filler", 2, 4)];
    expect(guideBreakProgramme(airings, airings[0] as (typeof airings)[number])).toBeUndefined();
  });

  it("labels a channel that is all breaks while its titles download", () => {
    const channel = {
      airings: [block("filler", 0, 2), block("filler", 2, 4)] as GuideChannelTimeline["airings"],
      pendingCount: 9,
      programCount: 3,
    };
    expect(guideWaitingForTitles(channel)).toBe("Commercials until titles arrive · 3 of 12 downloaded");
  });

  it("doesn't label a channel with a programme, or one with nothing on the way", () => {
    const withProgramme = [block("program", 0, 30)] as GuideChannelTimeline["airings"];
    expect(
      guideWaitingForTitles({ airings: withProgramme, pendingCount: 2, programCount: 1 }),
    ).toBeUndefined();
    const breaks = [block("filler", 0, 2)] as GuideChannelTimeline["airings"];
    expect(guideWaitingForTitles({ airings: breaks, pendingCount: 0, programCount: 4 })).toBeUndefined();
  });
});

describe("guideChannelForTypeahead", () => {
  const channel = (channelId: string, number: number, name: string): GuideChannelTimeline => ({
    airings: [],
    channelId,
    name,
    number,
    pendingCount: 0,
    status: "live",
  });
  const layout = layoutGuide(
    {
      channels: [
        channel("a", 2, "Sci-Fi Nights"),
        channel("b", 7, "Saturday Cartoons"),
        channel("c", 12, "Cozy Autumn Horror"),
        channel("d", 70, "Sitcom Reruns"),
        channel("e", 120, "Silent Films"),
      ],
      fromMs: 0,
      toMs: 10_000,
    } as GuideOutputBody,
    0,
  );

  it("jumps to the channel with exactly the typed number before any it merely prefixes", () => {
    expect(guideChannelForTypeahead(layout, "12")).toBe("c");
    expect(guideChannelForTypeahead(layout, "7")).toBe("b");
    expect(guideChannelForTypeahead(layout, "1")).toBe("c");
    expect(guideChannelForTypeahead(layout, "9")).toBeUndefined();
  });

  it("matches a name from its start, then from the start of any word, ignoring case", () => {
    expect(guideChannelForTypeahead(layout, "sat")).toBe("b");
    expect(guideChannelForTypeahead(layout, "HORR")).toBe("c");
    expect(guideChannelForTypeahead(layout, "zz")).toBeUndefined();
  });

  it("cycles through the names that share a first letter from the current channel", () => {
    expect(guideChannelForTypeahead(layout, "s", "a")).toBe("b");
    expect(guideChannelForTypeahead(layout, "s", "b")).toBe("d");
    expect(guideChannelForTypeahead(layout, "s", "e")).toBe("a");
    // A longer query keeps the current channel while it still matches.
    expect(guideChannelForTypeahead(layout, "sa", "b")).toBe("b");
  });
});
