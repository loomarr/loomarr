import { describe, expect, it } from "vitest";

import {
  isTvLongSelect,
  tvFavouriteToggle,
  tvGuideFavouriteChannel,
  tvSurfFavouriteChannel,
} from "../index";

describe("TV long-press Select", () => {
  it("is react-native-tvos's longSelect, counted once, on release", () => {
    expect(isTvLongSelect("longSelect", 1)).toBe(true);
    // Platforms that don't report a key action send one event per hold.
    expect(isTvLongSelect("longSelect")).toBe(true);
    // The recognised-but-still-held event must not toggle a second time.
    expect(isTvLongSelect("longSelect", 0)).toBe(false);
  });

  it("leaves a short Select and every other key to the focus engine", () => {
    for (const eventType of ["select", "up", "down", "left", "right", "menu", "longPlayPause", "7"]) {
      expect(isTvLongSelect(eventType, 1)).toBe(false);
    }
  });
});

describe("TV favourite toggle", () => {
  it("stars a channel that is not a favourite yet, and unstars one that is", () => {
    expect(tvFavouriteToggle("ch-scifi", ["ch-cartoons"])).toEqual({ channelId: "ch-scifi", starred: true });
    expect(tvFavouriteToggle("ch-scifi", ["ch-cartoons", "ch-scifi"])).toEqual({
      channelId: "ch-scifi",
      starred: false,
    });
    expect(tvFavouriteToggle("ch-scifi", [])).toEqual({ channelId: "ch-scifi", starred: true });
  });

  it("acts on the focused Guide programme's channel, never on a filter chip", () => {
    const selection = { anchorMs: 1_000, channelId: "ch-scifi", scheduleBlockId: "block-1" };
    expect(tvGuideFavouriteChannel({ kind: "airing", selection })).toBe("ch-scifi");
    expect(tvGuideFavouriteChannel({ filter: "favourites", kind: "filter" })).toBeUndefined();
    expect(tvGuideFavouriteChannel(undefined)).toBeUndefined();
  });

  it("acts on the focused Surf card's channel, in whichever group it was focused", () => {
    const recent = { channelId: "ch-cartoons", group: "recent" } as const;
    expect(tvSurfFavouriteChannel(recent)).toBe("ch-cartoons");
    expect(tvSurfFavouriteChannel({ ...recent, group: "all" })).toBe("ch-cartoons");
    expect(tvSurfFavouriteChannel(undefined)).toBeUndefined();
  });
});
