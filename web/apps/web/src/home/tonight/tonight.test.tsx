import type { GuideHighlightDTO } from "@loomarr/api/models/guideHighlightDTO";
import { describe, expect, it } from "vitest";
import { highlightReason } from "./tonight";

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
