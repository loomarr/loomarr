import type { GuideLayout, GuideSelection } from "@loomarr/core/guide";
import { describe, expect, it } from "vitest";

import {
  activateTvGuideFocus,
  moveTvGuideFocus,
  restoreTvGuideFocus,
  type TvGuideNavigationState,
  tvGuideRowWindow,
} from "../index";

const channel = (index: number) => ({
  airings: [
    {
      channelId: `channel-${index}`,
      clippedStartMs: 0,
      clippedStopMs: 100,
      endsAfterWindow: false,
      isOnNow: true,
      scheduleBlockId: `block-${index}`,
      source: {
        kind: "program" as const,
        scheduleBlockId: `block-${index}`,
        startMs: 0,
        stopMs: 100,
        title: `Programme ${index}`,
      },
      startRatio: 0,
      startsBeforeWindow: false,
      widthRatio: 1,
    },
  ],
  source: {
    airings: [],
    channelId: `channel-${index}`,
    name: `Channel ${index}`,
    number: index + 1,
    pendingCount: 0,
    status: "live" as const,
  },
});

const layout = {
  channels: [channel(0), channel(1), channel(2)],
  fromMs: 0,
  source: { channels: [], fromMs: 0, toMs: 100 },
  toMs: 100,
} satisfies GuideLayout;

const selection: GuideSelection = { anchorMs: 50, channelId: "channel-0", scheduleBlockId: "block-0" };
const state: TvGuideNavigationState = {
  activeFilter: "all",
  focus: { region: "grid", selection },
  gridSelection: selection,
};
const filters = [{ value: "all" }, { disabled: true, value: "favourites" }, { value: "recent" }] as const;

describe("TV Guide navigation", () => {
  it("moves from the first row into filters, skips disabled filters, and returns to the grid", () => {
    const filtersFocused = moveTvGuideFocus(layout, state, "up", filters).state;
    expect(filtersFocused.focus).toEqual({ filter: "all", region: "filters" });

    const recentFocused = moveTvGuideFocus(layout, filtersFocused, "right", filters).state;
    expect(recentFocused.focus).toEqual({ filter: "recent", region: "filters" });
    expect(activateTvGuideFocus(recentFocused)).toEqual({ filter: "recent", kind: "filter" });

    const gridFocused = moveTvGuideFocus(layout, recentFocused, "down", filters).state;
    expect(gridFocused.focus).toEqual({ region: "grid", selection });
  });

  it("retains the time anchor while moving vertically and exposes tune intent", () => {
    const moved = moveTvGuideFocus(layout, state, "down", filters).state;
    expect(moved.gridSelection).toEqual({
      anchorMs: 50,
      channelId: "channel-1",
      scheduleBlockId: "block-1",
    });
    expect(activateTvGuideFocus(moved)).toEqual({ kind: "tune", selection: moved.gridSelection });
  });

  it("restores focus by channel and falls back deterministically after removal", () => {
    const removed = { ...layout, channels: layout.channels.slice(1) };
    expect(restoreTvGuideFocus(removed, state)?.gridSelection.channelId).toBe("channel-1");
    expect(restoreTvGuideFocus({ ...layout, channels: [] }, state)).toBeUndefined();
  });

  it("bounds a 100-channel render window while keeping row position explicit", () => {
    expect(tvGuideRowWindow(100, 49, 8, 2)).toEqual({
      end: 55,
      positionLabel: "50 of 100",
      start: 43,
    });
    expect(tvGuideRowWindow(0, 0, 8)).toEqual({ end: 0, positionLabel: "No channels", start: 0 });
  });
});
