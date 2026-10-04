import type { HouseholdViewingOutputBody } from "@loomarr/api/models/householdViewingOutputBody";
import type { ViewerDTO } from "@loomarr/api/models/viewerDTO";
import { describe, expect, it } from "vitest";
import { watchingCards, watchingMeta } from "./watching-now";

const viewer = (over: Partial<ViewerDTO>): ViewerDTO => ({
  channelId: "c1",
  device: "living room TV",
  name: "Ada",
  since: "2026-09-28T00:00:00Z",
  userId: "u1",
  you: false,
  ...over,
});
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
      viewing: viewing({
        viewers: [viewer({}), viewer({ userId: "u2", name: "Bo", you: true, device: "laptop" })],
      }),
    });
    expect(cards.map((c) => c.viewer.name)).toEqual(["You", "Ada"]);
    expect(cards[0]?.viewer.initials).toBe("BO");
  });

  // The caller's last-tuned channel, when they aren't watching right now, is Home's separate
  // "Recently tuned" return card (#1822 evidence) — not a stand-in Watching now session.
  it("gives the caller no card when they aren't watching", () => {
    const cards = watchingCards({
      viewing: viewing({
        scope: "self",
        continueWatching: { channelId: "c1", tunedAt: "2026-09-28T00:00:00Z" },
      }),
    });
    expect(cards).toHaveLength(0);
  });

  it("reads a body without viewers as nobody watching", () => {
    const body = { scope: "household", watching: 0 } as unknown as HouseholdViewingOutputBody;
    expect(watchingCards({ viewing: body })).toEqual([]);
    expect(watchingMeta({ viewing: body })).toBe("0 people");
  });

  it("counts people, not devices, for an admin", () => {
    const v = viewing({
      viewers: [viewer({}), viewer({ device: "phone" }), viewer({ userId: "u2" })],
      watching: 3,
    });
    expect(watchingMeta({ viewing: v })).toBe("2 people");
  });

  it("gives a member the others as a count", () => {
    const v = viewing({ scope: "self", viewers: [viewer({ you: true })], watching: 3 });
    expect(watchingMeta({ viewing: v })).toBe("You and 2 others");
  });
});
