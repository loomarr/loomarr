import { afterEach, describe, expect, it, vi } from "vitest";

import {
  type ChannelListEvent,
  type ChannelListState,
  channelListAllowsPress,
  channelListReduce,
  channelListView,
  createChannelListState,
  listRest,
  pinnedShift,
  scrollBounds,
} from "../channel-list";
import { createChannelLineup, type LineupChannel } from "../channel-lineup";

const channel = (number: number, over: Partial<LineupChannel> = {}): LineupChannel => ({
  decade: number <= 3 ? 1980 : 1990,
  id: `ch-${number}`,
  name: `Channel ${number}`,
  number,
  playable: true,
  ...over,
});

const lineup = createChannelLineup({
  channels: [
    channel(1, { onNow: "Late harbour" }),
    channel(2),
    channel(3, { name: "Game Night" }),
    channel(4),
    channel(5, { decade: undefined }),
    channel(6, { playable: false }),
  ],
  favouriteIds: ["ch-4", "ch-2"],
});

const ids = (state: ChannelListState) => channelListView(state, lineup).rows.map((each) => each.number);

/** Plays events through the model and collects what it asked the caller to tune. */
const play = (state: ChannelListState, ...events: ChannelListEvent[]) => {
  const tuned: string[] = [];
  let at = state;
  for (const event of events) {
    const step = channelListReduce(at, event, lineup);
    at = step.state;
    if (step.tune) tuned.push(step.tune);
  }
  return { state: at, tuned };
};

const open = (watchedId = "ch-3", over = {}) =>
  play(createChannelListState({ watchedId, ...over }), { type: "open" }).state;

const up: ChannelListEvent = { by: -1, type: "move" };
const down: ChannelListEvent = { by: 1, type: "move" };
const left: ChannelListEvent = { by: -1, type: "across" };
const right: ChannelListEvent = { by: 1, type: "across" };

afterEach(() => {
  vi.useRealTimers();
});

describe("channel list cursor", () => {
  it("starts on the watched channel when the list opens", () => {
    expect(open().cursor).toEqual({ channelId: "ch-3", kind: "row" });
  });

  it("looks and never tunes while it moves, and stops at the list's ends", () => {
    const moved = play(open(), down, down, down, down, down, down, up);
    expect(moved.tuned).toEqual([]);
    expect(moved.state.cursor).toEqual({ channelId: "ch-5", kind: "row" });
    expect(moved.state.watchedId).toBe("ch-3");
  });

  it("goes Up from the first row to the tabs, moves between them and comes back Down", () => {
    const onTabs = play(open("ch-1"), up).state;
    expect(onTabs.cursor).toEqual({ group: "all", kind: "tab", returnTo: "ch-1" });
    expect(play(onTabs, right).state.cursor).toMatchObject({ group: "recent", kind: "tab" });
    expect(play(onTabs, right, right, right, right).state.cursor).toMatchObject({ group: "decade" });
    expect(play(onTabs, left, down).state.cursor).toEqual({ channelId: "ch-1", kind: "row" });
    expect(play(open(), { type: "tabs" }).state.cursor).toMatchObject({ kind: "tab", returnTo: "ch-3" });
    expect(play(onTabs, up).state).toEqual(onTabs);
  });

  it("changes group on choosing a tab and puts the cursor back on the watched channel", () => {
    const chosen = play(open(), { type: "tabs" }, left, { type: "choose" });
    expect(chosen.state.group).toBe("favourites");
    expect(chosen.state.cursor).toEqual({ channelId: "ch-3", kind: "row" });
    expect(chosen.tuned).toEqual([]);
    expect(play(open(), { group: "decade", type: "chooseTab" }).state.group).toBe("decade");
  });

  it("changes group directly with Left and Right in the open list", () => {
    const state = play(open(), down, right).state;
    expect(state.group).toBe("recent");
    expect(state.cursor).toEqual({ channelId: "ch-3", kind: "row" });
    expect(play(open("ch-3", { group: "favourites" }), left).state.group).toBe("favourites");
  });

  it("ignores moving while the list is closed", () => {
    const closed = createChannelListState({ watchedId: "ch-3" });
    expect(play(closed, down, left, { type: "tabs" }).state).toEqual(closed);
  });

  it("follows a tune from outside", () => {
    const state = play(open(), down, { channelId: "ch-1", type: "tuned" }).state;
    expect(state.watchedId).toBe("ch-1");
    expect(state.cursor).toEqual({ channelId: "ch-1", kind: "row" });
  });

  it("goes where a focus engine lands", () => {
    const state = play(open(), { channelId: "ch-4", type: "focusRow" }, { group: "all", type: "focusTab" });
    expect(state.state.cursor).toEqual({ group: "all", kind: "tab", returnTo: "ch-4" });
    expect(state.tuned).toEqual([]);
  });
});

describe("channel list choosing", () => {
  it("asks for a tune and leaves the list open", () => {
    const chosen = play(open(), down, { type: "choose" });
    expect(chosen.tuned).toEqual(["ch-4"]);
    expect(chosen.state.open).toBe(true);
    // The caller answers with `tuned`.
    expect(chosen.state.watchedId).toBe("ch-3");
  });

  it("tunes a clicked row", () => {
    expect(play(open(), { channelId: "ch-1", type: "choose" }).tuned).toEqual(["ch-1"]);
  });

  it("closes on the watched channel without tuning", () => {
    const chosen = play(open(), { type: "choose" });
    expect(chosen.tuned).toEqual([]);
    expect(chosen.state.open).toBe(false);
  });

  it("does nothing on a channel the viewer cannot play", () => {
    const state = open();
    const chosen = play(state, { channelId: "ch-6", type: "choose" });
    expect(chosen.tuned).toEqual([]);
    expect(chosen.state).toEqual(state);
  });

  it("closes only when asked: time alone changes nothing", () => {
    vi.useFakeTimers();
    let state = open();
    const caller = () => state;
    const before = caller();
    state = play(state, down).state;
    const moved = caller();
    vi.advanceTimersByTime(60 * 60_000);
    expect(vi.getTimerCount()).toBe(0);
    expect(caller()).toBe(moved);
    expect(caller().open).toBe(before.open);
    expect(play(caller(), { type: "close" }).state.open).toBe(false);
  });
});

describe("channel list groups", () => {
  it("always holds the watched channel", () => {
    expect(ids(open("ch-3", { group: "favourites" }))).toEqual([4, 2, 3]);
    expect(ids(open("ch-3", { group: "recent" }))).toEqual([3]);
    expect(ids(open("ch-5", { group: "decade" }))).toEqual([5, 1, 2, 3, 4, 6]);
    expect(ids(open("ch-3"))).toEqual([1, 2, 3, 4, 5, 6]);
  });

  it("heads By decade with its decades", () => {
    const view = channelListView(open("ch-5", { group: "decade" }), lineup);
    expect([...view.headings]).toEqual([
      ["ch-1", 1980],
      ["ch-4", 1990],
    ]);
  });

  it("says which group is empty", () => {
    expect(channelListView(open("ch-3", { group: "recent" }), lineup).notice).toBe("empty-recent");
    expect(channelListView(open("ch-3", { group: "favourites" }), lineup).notice).toBeUndefined();
    const bare = createChannelLineup({ channels: [channel(1, { decade: undefined })] });
    const state = createChannelListState({ group: "favourites", watchedId: "ch-1" });
    expect(channelListView(state, bare).notice).toBe("empty-favourites");
    expect(channelListView({ ...state, group: "decade" }, bare).notice).toBe("empty-decade");
  });
});

describe("channel list find", () => {
  const find = (query: string): ChannelListEvent => ({ query, type: "find" });

  it("matches a name, an exact number and the title on now, resting on the first result", () => {
    expect(ids(play(open(), find("game")).state)).toEqual([3]);
    expect(ids(play(open(), find("4")).state)).toEqual([4]);
    const harbour = play(open(), find("harbour")).state;
    expect(ids(harbour)).toEqual([1]);
    expect(harbour.cursor).toEqual({ channelId: "ch-1", kind: "row" });
  });

  it("restores the group on closing find", () => {
    const state = play(open("ch-3", { group: "favourites" }), find("game"), { type: "clearFind" }).state;
    expect(state).toMatchObject({ group: "favourites", query: null });
    expect(state.cursor).toEqual({ channelId: "ch-3", kind: "row" });
  });

  it("offers every channel when nothing matches", () => {
    const none = play(open("ch-3", { group: "favourites" }), find("zzz")).state;
    expect(channelListView(none, lineup).notice).toBe("no-match");
    expect(none.cursor).toEqual({ kind: "none" });
    expect(play(none, { type: "showAll" }).state).toMatchObject({ group: "all", query: null });
  });

  it("tunes a chosen result and closes find", () => {
    const chosen = play(open(), find("harbour"), { type: "choose" });
    expect(chosen.tuned).toEqual(["ch-1"]);
    expect(chosen.state).toMatchObject({ open: true, query: null });
  });

  it("keeps the list in place while the query is blank, with no tabs to reach", () => {
    const state = play(open("ch-1"), find(""), up, right).state;
    expect(ids(state)).toEqual([1, 2, 3, 4, 5, 6]);
    expect(state.cursor).toEqual({ channelId: "ch-1", kind: "row" });
    expect(state.group).toBe("all");
  });

  it("does not exist on a television", () => {
    const state = open("ch-3", { television: true });
    expect(play(state, find("game")).state).toEqual(state);
  });
});

describe("channel list geometry", () => {
  it("rests the pinned channel at the offset", () => {
    expect(listRest({ anchorY: 900, pinnedHeight: 96, restOffset: 200, viewport: 500 })).toEqual({
      height: 500,
      offset: 200,
      scroll: 700,
    });
  });

  it("rests a television on whole rows above and below the pinned channel", () => {
    const rest = listRest({ anchorY: 920, pinnedHeight: 100, restOffset: 310, viewport: 450, wholeRows: 48 });
    expect(rest.offset % 48).toBe(0);
    expect((rest.height - rest.offset - 100) % 48).toBe(0);
    expect(rest.height).toBeLessThanOrEqual(450);
    expect(rest.scroll).toBe(920 - rest.offset);
  });

  it("stops scrolling at the list's ends, and always reaches the rest", () => {
    expect(scrollBounds({ contentHeight: 2000, height: 500, rest: 700, room: 500 })).toEqual({
      max: 1000,
      min: 500,
    });
    expect(scrollBounds({ contentHeight: 1100, height: 500, rest: 300, room: 500 })).toEqual({
      max: 300,
      min: 300,
    });
  });

  it("holds the pinned channel at whichever edge it would leave by", () => {
    const view = { anchorY: 1000, height: 500, pinnedHeight: 100 };
    expect(pinnedShift({ ...view, scroll: 700 })).toBe(0);
    expect(pinnedShift({ ...view, scroll: 1200 })).toBe(200);
    expect(pinnedShift({ ...view, scroll: 300 })).toBe(-300);
  });
});

describe("channel list repeat presses", () => {
  const first = { at: 0, touch: false, x: 100, y: 200 };

  it("waits for a pointer to move", () => {
    expect(channelListAllowsPress(undefined, first)).toBe(true);
    expect(channelListAllowsPress(first, { ...first, at: 5_000, x: 110 })).toBe(false);
    expect(channelListAllowsPress(first, { ...first, at: 10, y: 214 })).toBe(true);
  });

  it("lets a finger press again after a short gap", () => {
    const tap = { ...first, touch: true };
    expect(channelListAllowsPress(tap, { ...tap, at: 100 })).toBe(false);
    expect(channelListAllowsPress(tap, { ...tap, at: 500 })).toBe(true);
  });
});
