import { describe, expect, it } from "vitest";

import { createChannelLineup } from "../channel-lineup";
import {
  createGuideViewState,
  type GuideViewChannel,
  type GuideViewContext,
  type GuideViewEvent,
  type GuideViewState,
  guideViewLook,
  guideViewReduce,
  guideViewSpan,
} from "../guide-view";

const HOUR = 3_600_000;
const BASE = 1_000 * HOUR;
const at = (hours: number) => BASE + hours * HOUR;

const programme = (channel: string, from: number, to: number) => ({
  id: `${channel}${from}`,
  startMs: at(from),
  stopMs: at(to),
  title: `${channel} at ${from}`,
});

const channel = (
  id: string,
  number: number,
  spans: [number, number][],
  decade?: number,
): GuideViewChannel => ({
  decade,
  id,
  name: id.toUpperCase(),
  number,
  playable: true,
  programmes: spans.map(([from, to]) => programme(id, from, to)),
});

const channels = [
  channel(
    "a",
    1,
    [
      [-2, -1],
      [-1, 1],
      [1, 2],
      [2, 3.5],
      [3.5, 6],
    ],
    1980,
  ),
  // A break from 0 to 1.5.
  channel(
    "b",
    2,
    [
      [-1, 0],
      [1.5, 4],
    ],
    1990,
  ),
  channel("c", 3, []),
  channel("d", 4, [[0, 8]]),
];

const context = (over: Partial<GuideViewContext<GuideViewChannel>> = {}) => ({
  formatTime: (ms: number) => `T+${(ms - BASE) / HOUR}`,
  lineup: createChannelLineup({ channels, favouriteIds: ["b"] }),
  nowMs: at(0.5),
  untilMs: at(8),
  widthMs: 3 * HOUR,
  ...over,
  // These tests' zone keeps whole hours.
  hourStartMs: Math.floor((over.nowMs ?? at(0.5)) / HOUR) * HOUR,
});

const open = (watchedId = "a", ctx = context(), television = false) =>
  createGuideViewState({ television, watchedId }, ctx);

const run = (state: GuideViewState, events: GuideViewEvent[], ctx = context()) =>
  events.reduce((step, event) => guideViewReduce(step.state, event, ctx), { state } as ReturnType<
    typeof guideViewReduce
  >);

const on = (state: GuideViewState) =>
  state.cursor.kind === "programme" ? state.cursor.programmeId : `tab:${state.cursor.group}`;

const up: GuideViewEvent = { by: -1, type: "move" };
const down: GuideViewEvent = { by: 1, type: "move" };
const left: GuideViewEvent = { by: -1, type: "across" };
const right: GuideViewEvent = { by: 1, type: "across" };
const later: GuideViewEvent = { by: 1, type: "page" };
const earlier: GuideViewEvent = { by: -1, type: "page" };
const ok: GuideViewEvent = { type: "choose" };
const back: GuideViewEvent = { type: "back" };

describe("the Guide's model", () => {
  it("opens on what is on now on the watched channel, from the present hour", () => {
    const state = open("b", context({ nowMs: at(1.75) }));
    expect(on(state)).toBe("b1.5");
    expect(state.startMs).toBe(at(1));
  });

  it("keeps the time of day on Up and Down, taking the next drawn programme in a break", () => {
    const state = open();
    expect(on(run(state, [down]).state)).toBe("b1.5");
    // The time kept is the one the cursor started on, not the break's neighbour's.
    expect(on(run(state, [down, up]).state)).toBe("a-1");
  });

  it("passes over a row with nothing drawn", () => {
    expect(on(run(open(), [down, down]).state)).toBe("d0");
  });

  it("pages by an hour past the last programme drawn, and back, never before now", () => {
    const walked = run(open(), [right, right]).state;
    expect(on(walked)).toBe("a2");
    expect(walked.startMs).toBe(at(0));
    const paged = run(walked, [right]).state;
    expect([on(paged), paged.startMs]).toEqual(["a3.5", at(1)]);
    const home = run(paged, [left, left, left, left, left]).state;
    expect([on(home), home.startMs]).toEqual(["a-1", at(0)]);
  });

  it("keeps the cursor on something drawn through Earlier and Later, within the schedule", () => {
    const one = run(open(), [later]).state;
    expect([on(one), one.startMs]).toEqual(["a1", at(1)]);
    const end = run(open(), Array(12).fill(later)).state;
    expect(end.startMs).toBe(at(5));
    expect(guideViewSpan(end, context())).toMatchObject({ canLater: false, endsAtMs: at(8) });
    const first = run(end, Array(12).fill(earlier)).state;
    expect(first.startMs).toBe(at(0));
    expect(guideViewSpan(first, context())).toMatchObject({ canEarlier: false, endsAtMs: undefined });
  });

  it("goes on paging past midnight for as long as the schedule holds", () => {
    const ctx = context({ untilMs: at(40) });
    expect(run(open("a", ctx), Array(30).fill(later), ctx).state.startMs).toBe(at(30));
  });

  it("tunes what is on now elsewhere and stays, and returns from the watched programme", () => {
    expect(run(open(), [down, down, ok]).tune).toBe("d");
    expect(run(open(), [down, down, ok]).close).toBeUndefined();
    expect(run(open(), [ok])).toMatchObject({ close: true });
  });

  it("says when a later programme starts and tunes nothing", () => {
    const step = run(open(), [right, ok]);
    expect(step.tune).toBeUndefined();
    expect(step.message).toBe("a at 1 starts at T+1. Enter on what is on now watches A");
    expect(run(open("a", context(), true), [right, ok]).message).toContain(". OK on what is on now");
  });

  it("returns one step on Back: from the tabs to the rows, then to the picture", () => {
    const tabs = run(open(), [up]).state;
    expect(on(tabs)).toBe("tab:all");
    const rows = run(tabs, [back]);
    expect([on(rows.state), rows.close]).toEqual(["a-1", undefined]);
    expect(run(rows.state, [back]).close).toBe(true);
  });

  it("says whether the header describes what is playing", () => {
    const look = (events: GuideViewEvent[]) => guideViewLook(run(open(), events).state, context());
    expect(look([]).playing).toBe(true);
    expect(look([right])).toMatchObject({ playing: false, programme: { id: "a1" } });
    expect(look([down])).toMatchObject({ channel: { id: "b" }, playing: false });
    expect(look([up])).toMatchObject({ channel: { id: "a" }, playing: true });
  });

  it("works the group tabs as the channel list does", () => {
    const tabs = run(open(), [up, left]).state;
    expect([on(tabs), tabs.group]).toEqual(["tab:favourites", "all"]);
    const chosen = run(tabs, [ok]).state;
    expect([on(chosen), chosen.group]).toEqual(["tab:favourites", "favourites"]);
    expect(on(run(chosen, [down]).state)).toBe("a-1");
  });

  it("tunes on a click on a row's channel or on what is on now, and only looks at a later one", () => {
    expect(run(open(), [{ channelId: "d", type: "pressChannel" }]).tune).toBe("d");
    expect(run(open(), [{ channelId: "d", programmeId: "d0", type: "press" }]).tune).toBe("d");
    const looked = run(open(), [{ channelId: "b", programmeId: "b1.5", type: "press" }]);
    expect([on(looked.state), looked.tune, looked.message]).toEqual(["b1.5", undefined, undefined]);
    expect(on(run(open(), [{ channelId: "a", programmeId: "a-2", type: "press" }]).state)).toBe("a-1");
    expect(
      run(open(), [{ channelId: "b", keyed: true, programmeId: "b1.5", type: "press" }]).message,
    ).toContain("starts at T+1.5");
  });

  it("lets the time move on without moving the cursor", () => {
    const state = run(open(), [right]).state;
    const after = context({ nowMs: at(0.9) });
    const next = run(state, [{ type: "refresh" }], after).state;
    expect(next).toBe(state);
    // "On now" follows the clock.
    expect(guideViewLook(next, context({ nowMs: at(1.5) })).playing).toBe(true);
    const nextHour = run(state, [{ type: "refresh" }], context({ nowMs: at(1.1) })).state;
    expect([on(nextHour), nextHour.startMs]).toEqual(["a1", at(1)]);
  });

  it.each([
    ["5 h 30 min", 5.5],
    ["5 h 45 min", 5.75],
  ])("starts and pages on the viewer's hours in a zone %s off", (_name, offset) => {
    // The local hour holding now began this long after a UTC hour.
    const hourStartMs = at(1 - (offset % 1));
    const ctx = { ...context(), hourStartMs, nowMs: hourStartMs + HOUR / 2, untilMs: hourStartMs + 6 * HOUR };
    const first = open("d", ctx);
    expect(first.startMs).toBe(hourStartMs);
    const one = run(first, [later], ctx).state;
    expect(one.startMs).toBe(hourStartMs + HOUR);
    expect(run(one, [earlier, earlier], ctx).state.startMs).toBe(hourStartMs);
  });

  it("moves the cursor off a programme that ends under it, and only then", () => {
    const state = open();
    expect(run(state, [{ type: "refresh" }], context({ nowMs: at(0.9) })).state).toBe(state);
    const after = context({ nowMs: at(1) });
    const next = run(state, [{ type: "refresh" }], after).state;
    expect(on(next)).toBe("a1");
    expect(guideViewLook(next, after).programme?.id).toBe("a1");
    // Even before the caller says so, the header never describes the ended programme.
    expect(guideViewLook(state, after).programme?.id).toBe("a1");
    expect(on(run(next, [left], after).state)).toBe("a1");
    // A channel in a break takes the next programme drawn.
    const onB = run(
      state,
      [{ channelId: "b", programmeId: "b-1", type: "focusProgramme" }],
      context({ nowMs: at(-0.5) }),
    );
    expect(on(run(onB.state, [{ type: "refresh" }], context()).state)).toBe("b1.5");
  });

  it("takes the cursor to the watched channel when a tune removes its row from the group", () => {
    const favourites = createGuideViewState({ group: "favourites", watchedId: "a" }, context());
    expect(on(favourites)).toBe("a-1");
    const next = run(favourites, [{ channelId: "d", type: "tuned" }]).state;
    expect(on(next)).toBe("d0");
  });

  it("works with a single channel", () => {
    const ctx = context({ lineup: createChannelLineup({ channels: channels.slice(0, 1) }) });
    const state = open("a", ctx);
    expect(on(run(state, [down], ctx).state)).toBe("a-1");
    expect(on(run(state, [up], ctx).state)).toBe("tab:all");
    expect(run(state, [ok], ctx).close).toBe(true);
  });

  it("keeps the cursor when the watched channel changes from outside", () => {
    const state = run(open(), [right]).state;
    const next = run(state, [{ channelId: "d", type: "tuned" }]).state;
    expect([on(next), next.watchedId]).toEqual(["a1", "d"]);
    expect(guideViewLook(next, context()).playing).toBe(false);
  });
});
