import { layoutGuide } from "@loomarr/core/guide";
import { describe, expect, it } from "vitest";

import {
  restoreSurfSelection,
  surfGroupsFromGuide,
  surfPreviousChannel,
  watchingScheduleFromGuide,
} from "../index";

const layout = layoutGuide(
  {
    channels: [
      {
        airings: [
          {
            episode: 2,
            kind: "program",
            scheduleBlockId: "now",
            season: 7,
            startMs: 1_000,
            stopMs: 3_000,
            thumbUrl: "/v1/images/now.jpg",
            title: "Now",
          },
          { kind: "program", scheduleBlockId: "next", startMs: 3_000, stopMs: 5_000, title: "Next" },
        ],
        channelId: "seven",
        logo: "/v1/images/seven-logo.png",
        name: "Science Fiction",
        number: 7,
        pendingCount: 0,
        status: "live",
      },
      {
        airings: [],
        channelId: "blocked",
        name: "Not playable",
        number: 8,
        pendingCount: 0,
        status: "live",
      },
      {
        airings: [],
        channelId: "nine",
        name: "Drama",
        number: 9,
        pendingCount: 0,
        status: "live",
      },
    ],
    fromMs: 1_000,
    timezone: "America/New_York",
    toMs: 5_000,
  },
  2_000,
);

const groups = (favoriteChannelIds?: readonly string[]) =>
  surfGroupsFromGuide({
    currentChannelId: "seven",
    favoriteChannelIds,
    layout,
    nowMs: 2_000,
    playableChannelIds: ["seven", "nine"],
    recentChannelIds: ["blocked", "nine", "nine", "seven"],
  });

describe("Surf data", () => {
  it("uses authoritative now/next identity and only playable Channels", () => {
    const result = groups();

    expect(result[0]).toEqual({ channels: [], kind: "favourites", label: "Favourites" });
    expect(result[1]?.channels.map(({ id }) => id)).toEqual(["nine"]);
    expect(result[2]?.channels.map(({ id }) => id)).toEqual(["seven", "nine"]);
    expect(result[2]?.channels[0]).toMatchObject({
      channelLogoUri: "/v1/images/seven-logo.png",
      channelName: "Science Fiction",
      next: { title: "Next" },
      now: {
        artworkUri: "/v1/images/now.jpg",
        episodeLabel: "S07E02",
        progressPercent: 50,
        remainingLabel: "1m left",
        title: "Now",
      },
    });
  });

  it("maps the tuned Channel's authoritative Guide identity into Watching", () => {
    expect(watchingScheduleFromGuide(layout, "seven", 2_000)).toMatchObject({
      next: { title: "Next" },
      now: { episodeLabel: "S07E02", progressPercent: 50, title: "Now" },
    });
    expect(watchingScheduleFromGuide(layout, "missing", 2_000)).toBeUndefined();
  });

  it("moves Watching through programme, named filler clips, and the returning programme", () => {
    const transitionLayout = layoutGuide(
      {
        channels: [
          {
            airings: [
              {
                kind: "program",
                scheduleBlockId: "programme-before",
                series: "The Simpsons",
                startMs: 1_000,
                stopMs: 3_000,
                title: "Before the break",
              },
              {
                kind: "filler",
                pod: {
                  entries: [
                    {
                      brand: "First Brand",
                      durationMs: 800,
                      hash: "raw-first-hash",
                      isFallbackCard: false,
                      kind: "commercial",
                      name: "First sponsor spot",
                      path: "/raw/first-source.mp4",
                    },
                    {
                      brand: "Second Brand",
                      durationMs: 1_200,
                      isFallbackCard: false,
                      kind: "commercial",
                      name: "Second sponsor spot",
                    },
                  ],
                  matchLevel: "exact",
                  totalMs: 2_000,
                },
                scheduleBlockId: "commercial-break",
                startMs: 3_000,
                stopMs: 5_000,
                title: "",
              },
              {
                kind: "program",
                scheduleBlockId: "programme-after",
                series: "The Simpsons",
                startMs: 5_000,
                stopMs: 7_000,
                title: "After the break",
              },
            ],
            channelId: "seven",
            name: "Science Fiction",
            number: 7,
            pendingCount: 0,
            status: "live",
          },
        ],
        fromMs: 1_000,
        toMs: 7_000,
      },
      2_000,
    );

    expect(watchingScheduleFromGuide(transitionLayout, "seven", 2_500)).toMatchObject({
      next: { title: "Commercials" },
      now: { progressPercent: 75, title: "The Simpsons · Before the break" },
    });
    const fillerSchedule = watchingScheduleFromGuide(transitionLayout, "seven", 3_500);
    expect(fillerSchedule).toMatchObject({
      next: { title: "The Simpsons · After the break" },
      now: {
        facts: ["First Brand"],
        progressPercent: 25,
        title: "Commercials · First sponsor spot",
      },
    });
    expect(JSON.stringify(fillerSchedule)).not.toContain("raw-first-hash");
    expect(JSON.stringify(fillerSchedule)).not.toContain("/raw/first-source.mp4");
    expect(watchingScheduleFromGuide(transitionLayout, "seven", 4_100)).toMatchObject({
      now: { facts: ["Second Brand"], title: "Commercials · Second sponsor spot" },
    });
    expect(watchingScheduleFromGuide(transitionLayout, "seven", 5_000)).toMatchObject({
      now: { progressPercent: 0, title: "The Simpsons · After the break" },
    });
  });

  it("populates Favourites only from authoritative membership and preserves Channel order", () => {
    expect(groups(["blocked", "nine", "seven"])[0]?.channels.map(({ id }) => id)).toEqual(["seven", "nine"]);
  });

  it("restores by group, then Channel identity, then the first available row", () => {
    const result = groups();
    expect(restoreSurfSelection(result, { channelId: "nine", group: "recent" })).toEqual({
      channelId: "nine",
      group: "recent",
    });
    expect(restoreSurfSelection(result.slice(2), { channelId: "nine", group: "recent" })).toEqual({
      channelId: "nine",
      group: "all",
    });
    expect(restoreSurfSelection(result, { channelId: "removed", group: "all" })).toEqual({
      channelId: "nine",
      group: "recent",
    });
  });

  it("returns the first distinct playable Recent Channel as the previous target", () => {
    expect(surfPreviousChannel("seven", ["seven", "removed", "nine"], ["seven", "nine"])).toBe("nine");
    expect(surfPreviousChannel("seven", ["seven", "removed"], ["seven", "nine"])).toBeUndefined();
  });
});
