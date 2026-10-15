import { describe, expect, it } from "vitest";

import {
  isTvLongSelect,
  tvFavouriteToggle,
  tvGuideFavouriteChannel,
  tvPressIsLongSelectRelease,
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

describe("the press that ends a long-press", () => {
  // Measured on the Apple TV simulator, 1.5 s hold: on a Guide row longSelect (the toggle) first, then
  // the row's own onPress ~0.7 s later; on Watching longSelect action 1 and the Select press arrive in
  // the same millisecond. Only the long-press may happen.
  it("is not a tune or an OK, once", () => {
    expect(tvPressIsLongSelectRelease(10_000, 10_000)).toBe(true);
    expect(tvPressIsLongSelectRelease(10_000, 10_700)).toBe(true);
    expect(tvPressIsLongSelectRelease(10_000, 11_500)).toBe(true);
  });

  it("lets a real press through when no hold came before it, or a moment ago", () => {
    expect(tvPressIsLongSelectRelease(undefined, 10_700)).toBe(false);
    expect(tvPressIsLongSelectRelease(10_000, 11_501)).toBe(false);
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
