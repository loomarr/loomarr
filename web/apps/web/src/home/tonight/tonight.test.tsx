import type { GuideAiring } from "@loomarr/api/models/guideAiring";
import type { GuideChannelTimeline } from "@loomarr/api/models/guideChannelTimeline";
import type { GuideHighlightDTO } from "@loomarr/api/models/guideHighlightDTO";
import { describe, expect, it } from "vitest";
import { highlightReason, nextProgrammes } from "./tonight";

const airing = { kind: "program", scheduleBlockId: "b1", startMs: 0, stopMs: 1, title: "Pilot" } as const;
const highlight = (over: Partial<GuideHighlightDTO>): GuideHighlightDTO => ({
  airing,
  channelId: "c1",
  channelName: "Test",
  channelNumber: 7,
  reason: "series_premiere",
  ...over,
});

describe("highlightReason — the client words the server's typed reason (#1664)", () => {
  it("names a season premiere by its season", () => {
    expect(highlightReason(highlight({ reason: "season_premiere", airing: { ...airing, season: 4 } }))).toBe(
      "Season 4 premiere",
    );
  });

  it("gives a marathon's end time in the guide's timezone", () => {
    const untilMs = Date.UTC(2026, 8, 28, 3, 0);
    expect(highlightReason(highlight({ reason: "marathon", untilMs }), "UTC")).toBe(
      "Back-to-back until 3:00 AM",
    );
  });

  it("says series premiere without a number", () => {
    expect(highlightReason(highlight({}))).toBe("Series premiere");
  });
});

describe("nextProgrammes — one row per channel (#2763)", () => {
  const channel = (id: string, number: number, airings: GuideAiring[]): GuideChannelTimeline => ({
    airings,
    channelId: id,
    name: id,
    number,
    pendingCount: 0,
    status: "live",
  });
  const at = (id: string, kind: "program" | "filler", startMs: number): GuideAiring => ({
    kind,
    scheduleBlockId: id,
    startMs,
    stopMs: startMs + 10,
    title: id,
  });

  it("takes each channel's next programme after now, soonest first", () => {
    const rows = nextProgrammes(
      [
        channel("a", 1, [at("a-past", "program", 50), at("a-1", "program", 300), at("a-2", "program", 400)]),
        channel("b", 2, [at("b-1", "program", 200)]),
      ],
      100,
    );
    expect(rows.map((r) => r.airing.scheduleBlockId)).toEqual(["b-1", "a-1"]);
  });

  it("skips filler, and gives a channel with only filler ahead no row", () => {
    const rows = nextProgrammes(
      [
        channel("a", 1, [at("a-ad", "filler", 150), at("a-1", "program", 300)]),
        channel("b", 2, [at("b-ad", "filler", 150)]),
      ],
      100,
    );
    expect(rows.map((r) => r.channelId)).toEqual(["a"]);
    expect(rows[0]?.airing.scheduleBlockId).toBe("a-1");
  });
});
