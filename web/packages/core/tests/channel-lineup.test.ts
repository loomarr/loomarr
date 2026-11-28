import { describe, expect, it } from "vitest";
import { createChannelLineup, type LineupChannel, lastChannelId, RECENT_LIMIT } from "../channel-lineup";

const channel = (number: number, over: Partial<LineupChannel> = {}): LineupChannel => ({
  id: `ch-${number}`,
  name: `Channel ${number}`,
  number,
  playable: true,
  ...over,
});

const numbers = (channels: readonly LineupChannel[]) => channels.map((each) => each.number);

// Thirty channels, handed over out of order.
const thirty = Array.from({ length: 30 }, (_, index) => channel(index + 1)).reverse();

describe("createChannelLineup order", () => {
  it("lists 30 channels in number order and says where each sits", () => {
    const lineup = createChannelLineup({ channels: thirty });
    expect(numbers(lineup.channels)).toEqual(Array.from({ length: 30 }, (_, index) => index + 1));
    expect(lineup.positionOf("ch-17")).toBe(16);
    expect(lineup.positionOf("missing")).toBeUndefined();
  });

  it("puts the last tuned channel first for the wall, playable or not", () => {
    const channels = [channel(1), channel(2), channel(3, { playable: false })];
    const lineup = createChannelLineup({ channels, lastTunedId: "ch-3" });
    expect(numbers(lineup.lastTunedFirst())).toEqual([3, 1, 2]);
    expect(numbers(createChannelLineup({ channels }).lastTunedFirst())).toEqual([1, 2, 3]);
  });
});

describe("flipping", () => {
  it("wraps from the last playable channel to the first, and back", () => {
    const lineup = createChannelLineup({ channels: thirty });
    expect(lineup.neighbour("ch-30", 1)?.number).toBe(1);
    expect(lineup.neighbour("ch-1", -1)?.number).toBe(30);
  });

  it("skips channels that are not playable", () => {
    const lineup = createChannelLineup({
      channels: [channel(1), channel(2, { playable: false }), channel(3), channel(4, { playable: false })],
    });
    expect(lineup.neighbour("ch-1", 1)?.number).toBe(3);
    expect(lineup.neighbour("ch-3", 1)?.number).toBe(1);
    expect(lineup.neighbour("ch-2", -1)?.number).toBe(1);
  });

  it("follows the active group, entering it from outside", () => {
    const lineup = createChannelLineup({ channels: thirty, favouriteIds: ["ch-9", "ch-4", "ch-20"] });
    expect(lineup.neighbour("ch-9", 1, "favourites")?.number).toBe(4);
    expect(lineup.neighbour("ch-20", 1, "favourites")?.number).toBe(9);
    expect(lineup.neighbour("ch-1", 1, "favourites")?.number).toBe(9);
    expect(lineup.neighbour("ch-1", -1, "favourites")?.number).toBe(20);
  });

  it("has nowhere to go with no other playable channel", () => {
    const lineup = createChannelLineup({ channels: [channel(1), channel(2, { playable: false })] });
    expect(lineup.neighbour("ch-1", 1)).toBeUndefined();
    expect(lineup.neighbour("ch-1", 1, "favourites")).toBeUndefined();
  });
});

describe("groups", () => {
  const channels = [
    channel(1, { decade: 1990 }),
    channel(2),
    channel(3, { decade: 1960 }),
    channel(4, { decade: 1990 }),
  ];

  it("heads the list with Favourites, All, Recent and By decade", () => {
    expect(createChannelLineup({ channels }).groups.map((group) => group.kind)).toEqual([
      "favourites",
      "all",
      "recent",
      "decade",
    ]);
  });

  it("keeps Favourites as starred, All by number and Recent newest first", () => {
    const lineup = createChannelLineup({
      channels,
      favouriteIds: ["ch-4", "gone", "ch-1"],
      recentIds: ["ch-3", "ch-1", "ch-3", "gone"],
    });
    expect(numbers(lineup.group("favourites").channels)).toEqual([4, 1]);
    expect(numbers(lineup.group("all").channels)).toEqual([1, 2, 3, 4]);
    expect(numbers(lineup.group("recent").channels)).toEqual([3, 1]);
  });

  it("holds Recent to the server's limit", () => {
    const recentIds = thirty.map((each) => each.id);
    const lineup = createChannelLineup({ channels: thirty, recentIds });
    expect(lineup.group("recent").channels).toHaveLength(RECENT_LIMIT);
    expect(lineup.group("recent").channels[0]?.number).toBe(30);
  });

  it("heads By decade with ascending decades, leaving a mixed channel to All", () => {
    const lineup = createChannelLineup({ channels });
    const byDecade = lineup.group("decade");
    expect(byDecade.decades.map((section) => [section.decade, numbers(section.channels)])).toEqual([
      [1960, [3]],
      [1990, [1, 4]],
    ]);
    expect(numbers(byDecade.channels)).toEqual([3, 1, 4]);
    expect(lineup.positionOf("ch-2", "decade")).toBeUndefined();
    expect(lineup.positionOf("ch-2")).toBe(1);
  });

  it("reports an empty group", () => {
    const lineup = createChannelLineup({ channels: [channel(1)] });
    expect(lineup.groups.map((group) => group.empty)).toEqual([true, false, true, true]);
  });
});

describe("find", () => {
  const lineup = createChannelLineup({
    channels: [
      channel(12, { name: "Movie Matinee" }),
      channel(9, { name: "Late Movies" }),
      channel(7, { name: "Coast 90s" }),
    ],
  });

  it("filters by name in number order", () => {
    expect(numbers(lineup.find("  mOVie "))).toEqual([9, 12]);
    expect(lineup.find("nothing")).toEqual([]);
    expect(lineup.find("  ")).toEqual([]);
  });

  it("resolves typed digits to that channel, or to none", () => {
    expect(lineup.channelByNumber("12")?.name).toBe("Movie Matinee");
    expect(lineup.channelByNumber("07")?.number).toBe(7);
    expect(lineup.channelByNumber("1")).toBeUndefined();
    expect(lineup.channelByNumber("movie")).toBeUndefined();
    expect(numbers(lineup.find("9"))).toEqual([7, 9]);
  });
});

describe("landing and last channel", () => {
  const channels = [channel(5, { playable: false }), channel(10), channel(20)];

  it("lands on the last tuned channel", () => {
    expect(createChannelLineup({ channels, lastTunedId: "ch-20" }).landing?.number).toBe(20);
  });

  it("lands on the lowest playable channel when the last tuned one cannot be played", () => {
    for (const lastTunedId of [undefined, "gone", "ch-5"]) {
      expect(createChannelLineup({ channels, lastTunedId }).landing?.number).toBe(10);
    }
  });

  it("lands nowhere when nothing is playable", () => {
    const lineup = createChannelLineup({
      channels: [channel(5, { playable: false })],
      lastTunedId: "ch-5",
    });
    expect(lineup.landing).toBeUndefined();
    expect(lineup.neighbour(undefined, 1)).toBeUndefined();
  });

  it("returns to the newest other playable recent", () => {
    const recentIds = ["ch-10", "ch-5", "ch-20"];
    expect(createChannelLineup({ channels, recentIds }).lastChannel("ch-10")?.number).toBe(20);
    expect(createChannelLineup({ channels }).lastChannel("ch-10")).toBeUndefined();
    expect(lastChannelId("ch-20", recentIds, ["ch-10", "ch-20"])).toBe("ch-10");
  });
});
