import type { GuideHighlightDTO } from "@loomarr/api/models/guideHighlightDTO";
import type { HouseholdViewingOutputBody } from "@loomarr/api/models/householdViewingOutputBody";
import type { ViewerDTO } from "@loomarr/api/models/viewerDTO";
import { layoutGuide } from "@loomarr/core/guide";
import { describe, expect, it } from "vitest";
import { highlightReason } from "./tonight";
import { watchingCards, watchingMeta } from "./watching-now";

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
    expect(highlightReason(highlight({ reason: "marathon", untilMs }), "UTC")).toBe("Back-to-back until 3:00 AM");
  });

  it("says series premiere without a number", () => {
    expect(highlightReason(highlight({}))).toBe("Series premiere");
  });
});

const viewer = (over: Partial<ViewerDTO>): ViewerDTO => ({
  channelId: "c1",
  device: "living room TV",
  name: "Ada",
  since: "2026-09-28T00:00:00Z",
  userId: "u1",
  you: false,
  ...over,
});
const layout = layoutGuide(
  {
    channels: [{ airings: [], channelId: "c1", name: "Test", number: 7, pendingCount: 0, status: "live" }],
    fromMs: 0,
    toMs: 1,
  },
  0,
);
const viewing = (over: Partial<HouseholdViewingOutputBody>): HouseholdViewingOutputBody => ({
  channels: [],
  scope: "household",
  viewers: [],
  watching: 0,
  ...over,
});

describe("Watching now — who gets a card (#1662, Q-H2)", () => {
  it("leads with the caller's own viewing", () => {
    const cards = watchingCards({
      layout,
      viewing: viewing({ viewers: [viewer({}), viewer({ userId: "u2", name: "Bo", you: true, device: "laptop" })] }),
    });
    expect(cards.map((c) => c.viewer.name)).toEqual(["You — continue watching", "Ada"]);
    expect(cards[0]?.viewer.initials).toBe("BO");
  });

  it("stands in the last-tuned channel when the caller isn't watching", () => {
    const cards = watchingCards({
      layout,
      viewing: viewing({ scope: "self", continueWatching: { channelId: "c1", tunedAt: "2026-09-28T00:00:00Z" } }),
    });
    expect(cards).toHaveLength(1);
    expect(cards[0]?.you).toBe(true);
  });

  it("counts people, not devices, for an admin", () => {
    const v = viewing({ viewers: [viewer({}), viewer({ device: "phone" }), viewer({ userId: "u2" })], watching: 3 });
    expect(watchingMeta({ viewing: v })).toBe("2 people");
  });

  it("gives a member the others as a count", () => {
    const v = viewing({ scope: "self", viewers: [viewer({ you: true })], watching: 3 });
    expect(watchingMeta({ viewing: v })).toBe("You and 2 others");
  });
});
