import type { GuideAiringLayout } from "@loomarr/core/guide";
import { describe, expect, it } from "vitest";

import { playbarFromTimeline } from "../index";

type Airing = GuideAiringLayout["source"];

const MINUTE = 60_000;
const NOW = Date.UTC(2026, 9, 29, 20, 44);
const at = (minutes: number) => NOW + minutes * MINUTE;
const channel = { name: "Coast 90s", number: "7" };
const live = { lagSeconds: 0, mode: "live" as const };

const programme = (id: string, from: number, to: number, fields: Partial<Airing> = {}): Airing => ({
  kind: "program",
  scheduleBlockId: id,
  startMs: at(from),
  stopMs: at(to),
  title: `Programme ${id}`,
  ...fields,
});

const spot = (role: "commercial" | "station_id", name: string, seconds: number, brand?: string) => ({
  brand,
  durationMs: seconds * 1000,
  era: role === "commercial" ? 1996 : undefined,
  hash: `${role}-${name}`,
  isFallbackCard: false,
  name,
  role,
});

const breakAt = (id: string, from: number, entries: ReturnType<typeof spot>[]): Airing => {
  const totalMs = entries.reduce((sum, e) => sum + e.durationMs, 0);
  return {
    kind: "filler",
    pod: { entries, matchLevel: "exact", totalMs },
    scheduleBlockId: id,
    startMs: at(from),
    stopMs: at(from) + totalMs,
    title: "Commercials",
  };
};

describe("playbarFromTimeline", () => {
  it("windows fifteen minutes behind live to two hours ahead, clipping the blocks it cuts", () => {
    const { segments, window } = playbarFromTimeline({
      airings: [
        programme("gone", -60, -20),
        programme("cut", -20, 10),
        programme("ahead", 10, 200),
        programme("beyond", 200, 260),
      ],
      channel,
      live,
      nowMs: NOW,
      playheadMs: NOW,
    });

    expect(window).toEqual({ endMs: at(120), nowMs: NOW, startMs: at(-15) });
    expect(segments.map((s) => s.id)).toEqual(["cut", "ahead"]);
    expect(segments[0]?.startMs).toBe(window.startMs);
    expect(segments[1]?.stopMs).toBe(window.endMs);
  });

  it("names an episode by its series, season, episode and title, with time in and left", () => {
    const { band } = playbarFromTimeline({
      airings: [
        programme("p", -9, 13, {
          episode: 4,
          season: 2,
          series: "An ensemble comedy",
          title: "Opening Night",
        }),
      ],
      channel,
      live,
      nowMs: NOW,
      playheadMs: NOW,
    });

    expect(band).toMatchObject({
      channelName: "Coast 90s",
      channelNumber: "7",
      programmeLine: "An ensemble comedy · Opening Night",
      timeLabel: "9m in · 13m left",
    });
    expect(band.progress).toBeCloseTo(9 / 22);
  });

  it("reads the block under the playhead, not under live, when the viewer is behind", () => {
    const { band } = playbarFromTimeline({
      airings: [programme("before", -30, -5, { title: "What aired then" }), programme("now", -5, 25)],
      channel,
      live: { lagSeconds: 134, mode: "paused" },
      nowMs: NOW,
      playheadMs: at(-10),
    });

    expect(band.programmeLine).toBe("What aired then");
    expect(band.timeLabel).toBe("20m in · 2m 14s behind");
  });

  it("names the spot on air in a break, walking the pod by duration", () => {
    const airings = [
      breakAt("b", -1, [
        spot("commercial", "Breakfast cereal", 30, "A cereal brand"),
        spot("commercial", "Family car", 30, "A car maker"),
        spot("station_id", "", 10),
      ]),
    ];
    const read = (seconds: number) =>
      playbarFromTimeline({ airings, channel, live, nowMs: NOW, playheadMs: at(-1) + seconds * 1000 }).band;

    expect(read(10)).toMatchObject({ inBreak: true, programmeLine: "Commercials · Breakfast cereal" });
    expect(read(45).programmeLine).toBe("Commercials · Family car");
    // A spot with no viewer name leaves the line at the break itself.
    expect(read(65).programmeLine).toBe("Commercials");
    expect(read(10).timeLabel).toBe("Break · 1 min");
  });

  it("lists every spot in a break, a commercial by brand and era and the rest by kind", () => {
    const { segments } = playbarFromTimeline({
      airings: [
        breakAt("b", 5, [spot("commercial", "Toy robot", 30, "A toy maker"), spot("station_id", "", 10)]),
      ],
      channel,
      live,
      nowMs: NOW,
      playheadMs: NOW,
    });

    const [only] = segments;
    expect(only?.kind).toBe("break");
    if (only?.kind !== "break") return;
    expect(only.spots.map(({ detail, durationLabel, name }) => ({ detail, durationLabel, name }))).toEqual([
      { detail: "A toy maker · 1996", durationLabel: "30s", name: "Toy robot" },
      { detail: "", durationLabel: "10s", name: "Station ID" },
    ]);
  });

  it("starts over at the programme under the playhead, only while the window holds its start", () => {
    const read = (from: number, playheadMs = NOW) =>
      playbarFromTimeline({
        airings: [programme("p", from, 30), breakAt("b", 30, [spot("station_id", "", 10)])],
        channel,
        live,
        nowMs: NOW,
        playheadMs,
      }).startOverMs;

    expect(read(-12)).toBe(at(-12));
    // Began before the fifteen minutes the stream holds: nowhere to start over from.
    expect(read(-20)).toBeUndefined();
    // A break has no programme start to go back to.
    expect(read(-12, at(30) + 5_000)).toBeUndefined();
  });

  it("is the channel alone while nothing is known to be airing", () => {
    const { band, segments } = playbarFromTimeline({
      airings: [],
      channel,
      live,
      nowMs: NOW,
      playheadMs: NOW,
    });
    expect(band).toEqual({ channelName: "Coast 90s", channelNumber: "7" });
    expect(segments).toEqual([]);
  });

  it("gives the station-ID card the channel's era and place, saying nothing of an open span", () => {
    const station = (era?: { from?: number; to?: number }, place?: string) =>
      playbarFromTimeline({
        airings: [],
        channel: { ...channel, era, place },
        live,
        nowMs: NOW,
        playheadMs: NOW,
      }).band.station;

    expect(station({ from: 1990, to: 1999 }, "Portland, ME")).toBe("1990s · Portland, ME");
    expect(station({ from: 1996, to: 1996 })).toBe("1996");
    expect(station({ from: 1985, to: 1994 })).toBe("1985–1994");
    expect(station({ from: 1990 }, "Portland, ME")).toBe("Portland, ME");
    expect(station()).toBeUndefined();
  });

  it("describes the programme at the playhead for Info, with the next programme past any break", () => {
    const { info } = playbarFromTimeline({
      airings: [
        programme("now", -9, 13, {
          description: "A storm cuts the island off from the mainland.",
          episode: 4,
          genres: ["Mystery", "Drama"],
          rating: "TV-PG",
          season: 2,
          series: "Coastline Mysteries",
          title: "The Last Lighthouse",
          year: 1994,
        }),
        breakAt("b", 13, [spot("commercial", "Fizz", 30)]),
        programme("next", 14, 40, { episode: 1, season: 1, series: "Night Shift Diner", title: "Pilot" }),
      ],
      channel,
      live,
      nowMs: NOW,
      playheadMs: NOW,
    });

    expect(info).toMatchObject({
      facts: "S02E04 · 1994 · TV-PG · Mystery · Drama",
      progress: 9 / 22,
      startMs: at(-9),
      stopMs: at(13),
      synopsis: "A storm cuts the island off from the mainland.",
      timeLabel: "9m in · 13m left",
      title: "Coastline Mysteries · The Last Lighthouse",
      upNext: { line: "S01E01 “Pilot”", name: "Night Shift Diner", startMs: at(14) },
    });
  });

  it("describes a break for Info by its label alone", () => {
    const { info } = playbarFromTimeline({
      airings: [breakAt("b", -1, [spot("commercial", "Fizz", 120)])],
      channel,
      live,
      nowMs: NOW,
      playheadMs: NOW,
    });

    expect(info?.title).toBe("Commercials · Fizz");
    expect(info?.facts).toBeUndefined();
    expect(info?.synopsis).toBeUndefined();
    expect(info?.upNext).toBeUndefined();
  });
});
