import { describe, expect, it } from "vitest";

import { createChannelLineup, type LineupChannel } from "../channel-lineup";
import {
  createWatchScreenState,
  WATCH_DIGITS_MS,
  WATCH_REST_MS,
  type WatchRequest,
  type WatchScreenEvent,
  type WatchScreenStateArgs,
  watchKeyOf,
  watchScreenReduce,
  watchScreenView,
} from "../watch-screen";

const channel = (number: number, over: Partial<LineupChannel> = {}): LineupChannel => ({
  id: `ch-${number}`,
  name: `Channel ${number}`,
  number,
  playable: true,
  ...over,
});

// Twelve channels; 5 cannot be played.
const CHANNELS = Array.from({ length: 12 }, (_, i) =>
  channel(i + 1, i + 1 === 5 ? { playable: false } : i + 1 === 3 ? { name: "Game Night" } : {}),
);

type Input = WatchScreenEvent extends infer E ? (E extends { at: number } ? Omit<E, "at"> : never) : never;

/** A client in miniature: it keeps the state, builds the lineup from it and ticks at `nextAt`. */
const screen = (
  args: Partial<WatchScreenStateArgs> = {},
  channels = CHANNELS,
  favouriteIds: readonly string[] = ["ch-9", "ch-2"],
) => {
  let state = createWatchScreenState({ at: 0, layout: "full", watchedId: "ch-1", ...args });
  let now = 0;
  const requests: WatchRequest[] = [];
  const lineup = () => createChannelLineup({ channels, favouriteIds, leftIds: state.leftIds });
  const send = (event: Input, at = now) => {
    now = at;
    const step = watchScreenReduce(state, { ...event, at } as WatchScreenEvent, lineup());
    state = step.state;
    requests.push(...step.requests);
    return step.requests;
  };
  const view = (at = now) => watchScreenView(state, lineup(), at);
  const wait = (ms: number) => {
    const until = now + ms;
    for (let guard = 0; guard < 100; guard += 1) {
      const next = view(now).nextAt;
      if (next === null || next > until) break;
      send({ type: "tick" }, Math.max(now, next));
    }
    now = until;
  };
  return {
    get now() {
      return now;
    },
    get state() {
      return state;
    },
    key: (key: string, at = now) => send({ key, type: "key" }, at),
    requests,
    send,
    tunes: () => requests.flatMap((request) => (request.type === "tune" ? [request.channelId] : [])),
    view,
    wait,
  };
};

describe("rule 1: flipping tunes, and only the lower third shows", () => {
  it("tunes the next and previous channel at once, through the lineup's neighbours", () => {
    const s = screen();
    expect(s.key("ArrowDown")).toEqual([{ channelId: "ch-2", type: "tune" }]);
    expect(s.state.watchedId).toBe("ch-2");
    s.key("ArrowUp");
    s.key("ArrowUp");
    expect(s.tunes()).toEqual(["ch-2", "ch-1", "ch-12"]);
  });

  it("passes a channel that cannot be played", () => {
    const s = screen({ watchedId: "ch-4" });
    s.key("ArrowDown");
    expect(s.tunes()).toEqual(["ch-6"]);
  });

  it("never opens the list, and wakes the screen", () => {
    const s = screen();
    s.wait(WATCH_REST_MS);
    expect(s.view().awake).toBe(false);
    s.key("ArrowDown");
    expect(s.view()).toMatchObject({ awake: true, listOpen: false });
  });

  it("flips within the group in play, and in number order from Recent", () => {
    const favourites = screen({ group: "favourites", watchedId: "ch-9" });
    favourites.key("ArrowDown");
    expect(favourites.tunes()).toEqual(["ch-2"]);
    const recent = screen({ group: "recent", watchedId: "ch-7" });
    recent.key("ArrowDown");
    expect(recent.tunes()).toEqual(["ch-8"]);
  });

  it("lands ten flips in a second on the tenth channel along", () => {
    const s = screen();
    for (let i = 0; i < 10; i += 1) s.key("ArrowDown", i * 100);
    // 5 cannot be played, so ten steps from 1 is 12.
    expect(s.tunes().at(-1)).toBe("ch-12");
    expect(s.tunes()).toHaveLength(10);
    expect(s.state.watchedId).toBe("ch-12");
  });
});

describe("rule 2: Channels opens the list, and the list only looks", () => {
  it("opens without tuning, and moving through it never tunes", () => {
    const s = screen();
    expect(s.send({ control: "channels", type: "press" })).toEqual([]);
    expect(s.view().listOpen).toBe(true);
    for (const key of ["ArrowDown", "ArrowDown", "ArrowUp", "ArrowDown", "ArrowRight", "ArrowLeft"])
      s.key(key);
    s.send({ event: { by: 1, type: "move" }, type: "list" });
    expect(s.tunes()).toEqual([]);
  });

  it("tunes on OK and stays open; Esc closes it and gives focus back to Channels", () => {
    const s = screen();
    s.send({ control: "channels", type: "press" });
    s.key("ArrowDown");
    s.key("ArrowDown");
    s.key("Enter");
    expect(s.tunes()).toEqual(["ch-3"]);
    expect(s.view().listOpen).toBe(true);
    const before = s.state.handOff?.seq ?? 0;
    s.key("Escape");
    expect(s.view().listOpen).toBe(false);
    expect(s.state.handOff).toEqual({ seq: before + 1, to: "channels" });
  });

  it("sends focus to the list's cursor when a key reaches it from outside", () => {
    const s = screen();
    s.send({ control: "channels", type: "press" });
    s.key("ArrowDown");
    expect(s.state.handOff?.to).toBe("list");
  });

  it("does not hand focus to Channels when a key opened the list", () => {
    const s = screen();
    s.key("/");
    s.send({ event: { type: "close" }, type: "list" });
    expect(s.state.handOff).toBeUndefined();
  });
});

describe("rule 3: only the bare picture goes quiet", () => {
  it("rests five seconds after the last thing that happened", () => {
    const s = screen();
    expect(s.view().nextAt).toBe(WATCH_REST_MS);
    s.wait(WATCH_REST_MS - 1);
    expect(s.view().awake).toBe(true);
    s.wait(1);
    expect(s.view()).toMatchObject({ awake: false, nextAt: null });
  });

  it.each([
    ["the list is open", { control: "channels", type: "press" }],
    ["a menu is open", { control: "subtitles", type: "press" }],
    ["a control holds focus", { control: "pause", type: "focus" }],
    ["the pointer rests on the controls", { held: true, region: "quietRow", type: "pointerHold" }],
    ["the back control holds focus", { control: "back", type: "focus" }],
    ["the pointer rests on the back control", { held: true, region: "back", type: "pointerHold" }],
    ["the picture is paused", { key: " ", type: "key" }],
    ["digits are pending", { key: "1", type: "key" }],
  ] as const)("never rests while %s", (_, event) => {
    const s = screen();
    s.send(event as Input);
    expect(s.view().nextAt === null || s.state.digits !== undefined).toBe(true);
    if (s.state.digits) {
      // Pending digits hold the screen until they commit.
      expect(s.view(WATCH_DIGITS_MS - 1).awake).toBe(true);
      return;
    }
    s.wait(60_000);
    expect(s.view().awake).toBe(true);
  });

  it("asks nothing for a press of the back control, which the caller acts on", () => {
    const s = screen();
    expect(s.send({ control: "back", type: "press" })).toEqual([]);
  });

  it("rests again five seconds after what held it lets go", () => {
    const s = screen();
    s.send({ held: true, region: "quietRow", type: "pointerHold" });
    s.send({ held: true, region: "lowerThird", type: "pointerHold" });
    s.wait(20_000);
    s.send({ held: false, region: "quietRow", type: "pointerHold" });
    // Still on the lower third.
    expect(s.view().nextAt).toBeNull();
    s.send({ held: false, region: "lowerThird", type: "pointerHold" });
    expect(s.view().nextAt).toBe(s.now + WATCH_REST_MS);
  });

  it("wakes on pointer movement, and the clock alone wakes nothing", () => {
    const s = screen();
    s.wait(WATCH_REST_MS);
    s.send({ type: "tick" });
    expect(s.view().awake).toBe(false);
    s.send({ type: "pointerMove" });
    expect(s.view().awake).toBe(true);
  });

  it("does the job of the key that wakes it", () => {
    const s = screen();
    s.wait(WATCH_REST_MS);
    s.key("m");
    expect(s.requests).toEqual([{ type: "mute" }]);
    expect(s.view().awake).toBe(true);
  });
});

describe("rule 4: digits tune direct", () => {
  it("replaces the numeral and commits on Enter", () => {
    const s = screen();
    s.key("1");
    s.key("2");
    expect(s.view().digits).toEqual({ target: expect.objectContaining({ id: "ch-12" }), typed: "12" });
    s.key("Enter");
    expect(s.tunes()).toEqual(["ch-12"]);
    expect(s.view().digits).toBeUndefined();
  });

  it("commits by itself 1.6 seconds after the last digit", () => {
    const s = screen();
    s.key("7");
    s.wait(WATCH_DIGITS_MS - 1);
    expect(s.tunes()).toEqual([]);
    s.wait(1);
    expect(s.tunes()).toEqual(["ch-7"]);
  });

  it("is cancelled by Esc", () => {
    const s = screen();
    s.key("7");
    s.key("Escape");
    s.wait(WATCH_DIGITS_MS * 2);
    expect(s.tunes()).toEqual([]);
    expect(s.requests).toEqual([]);
  });

  it("says when no channel has the number, and tunes nothing", () => {
    const s = screen();
    s.key("9");
    s.key("9");
    expect(s.view().digits).toEqual({ target: undefined, typed: "99" });
    s.key("Enter");
    expect(s.requests).toEqual([{ text: "There is no channel 99", type: "message" }]);
  });

  it("says when the channel cannot be played", () => {
    const s = screen();
    s.key("5");
    s.key("Enter");
    expect(s.requests).toEqual([{ text: "5 Channel 5 cannot be played right now", type: "message" }]);
  });

  it("takes no more digits than the longest channel number has", () => {
    const s = screen();
    for (const key of ["1", "2", "3"]) s.key(key);
    expect(s.view().digits?.typed).toBe("12");
  });
});

describe("rule 5: pause, behind live and Go live", () => {
  it("pauses and resumes on Space, behind live by the time paused, and End goes live", () => {
    const s = screen();
    s.key(" ", 1000);
    expect(s.view()).toMatchObject({ awake: true, paused: true });
    s.key(" ", 31_000);
    expect(s.requests).toEqual([{ type: "pause" }, { type: "resume" }]);
    expect(s.view().behindSeconds).toBe(30);
    s.key("End");
    expect(s.requests.at(-1)).toEqual({ type: "goLive" });
    expect(s.view().behindSeconds).toBe(0);
  });

  it("adds each pause to how far behind it is", () => {
    const s = screen();
    s.send({ type: "pictureClick" }, 0);
    s.send({ type: "pictureClick" }, 10_000);
    s.send({ type: "pictureClick" }, 12_000);
    s.send({ type: "pictureClick" }, 17_000);
    expect(s.view().behindSeconds).toBe(15);
  });

  it("does nothing on End at live", () => {
    const s = screen();
    s.key("End");
    expect(s.requests).toEqual([]);
  });

  it("toggles full screen on a double click and leaves the picture playing", () => {
    const s = screen();
    s.send({ type: "pictureClick" }, 0);
    s.send({ type: "pictureClick" }, 150);
    s.send({ type: "pictureDoubleClick" }, 150);
    expect(s.view().paused).toBe(false);
    expect(s.requests.at(-1)).toEqual({ on: false, type: "fullScreen" });
  });

  it("closes the open list on a click on the picture, and does not pause", () => {
    const s = screen();
    s.send({ control: "channels", type: "press" });
    s.send({ type: "pictureClick" });
    expect(s.view()).toMatchObject({ listOpen: false, paused: false });
    expect(s.requests).toEqual([]);
  });

  it("ends a pause on a channel change", () => {
    const s = screen();
    s.key(" ", 0);
    s.key("ArrowDown", 4000);
    expect(s.view()).toMatchObject({ behindSeconds: 0, paused: false });
  });
});

describe("the player's own word on pause and live", () => {
  it("takes a seek it did not ask for as behind live, so End goes live", () => {
    const s = screen();
    s.send({ behindMs: 30_000, paused: false, type: "playback" }, 1000);
    expect(s.view().behindSeconds).toBe(30);
    s.key("End");
    expect(s.requests).toEqual([{ type: "goLive" }]);
  });

  it("takes a pause from outside the screen, so Space resumes it", () => {
    const s = screen();
    s.send({ behindMs: 0, paused: true, type: "playback" }, 1000);
    expect(s.view().paused).toBe(true);
    s.key(" ", 2000);
    expect(s.requests).toEqual([{ type: "resume" }]);
  });

  it("drops a behind live the player no longer has, after a Retry", () => {
    const s = screen();
    s.send({ behindMs: 30_000, paused: false, type: "playback" }, 1000);
    s.send({ behindMs: 0, paused: false, type: "playback" }, 2000);
    expect(s.view().behindSeconds).toBe(0);
    s.key("End");
    expect(s.requests).toEqual([]);
  });

  it("wakes the screen when it changes, and not when it says the same again", () => {
    const s = screen();
    s.wait(WATCH_REST_MS);
    expect(s.view().awake).toBe(false);
    s.send({ behindMs: 0, paused: false, type: "playback" });
    expect(s.view().awake).toBe(false);
    s.send({ behindMs: 10_000, paused: false, type: "playback" });
    expect(s.view().awake).toBe(true);
  });

  it("refuses to resume under a cover, as it refuses to pause", () => {
    const s = screen();
    s.key(" ", 0);
    s.send({ picture: "covered", type: "picture" }, 100);
    s.key(" ", 200);
    expect(s.requests).toEqual([{ type: "pause" }]);
  });
});

describe("rule 6: the desktop's keys", () => {
  it("opens the list with find on /", () => {
    const s = screen();
    s.key("/");
    expect(s.view().listOpen).toBe(true);
    expect(s.state.list.query).toBe("");
  });

  it("asks for the Guide on G, and closes the list", () => {
    const s = screen();
    s.send({ control: "channels", type: "press" });
    s.key("G");
    expect(s.requests).toEqual([{ type: "guide" }]);
    expect(s.view().listOpen).toBe(false);
  });

  it("opens Subtitles on C, Audio on A and the shortcuts on ?", () => {
    const s = screen();
    s.key("c");
    expect(s.view().menu).toEqual({ name: "subtitles" });
    s.key("Escape");
    s.key("A");
    expect(s.view().menu).toEqual({ name: "audio" });
    s.key("Escape");
    s.key("?");
    expect(s.requests).toEqual([
      { name: "subtitles", type: "menu" },
      { name: "audio", type: "menu" },
      { name: "shortcuts", type: "menu" },
    ]);
  });

  it("mutes on M", () => {
    const s = screen();
    s.key("M");
    expect(s.requests).toEqual([{ type: "mute" }]);
  });

  it("asks for full screen on F from anywhere", () => {
    const window = screen({ layout: "window" });
    window.key("f");
    expect(window.requests).toEqual([{ on: true, type: "fullScreen" }]);
    const full = screen();
    full.key("c");
    full.key("F");
    full.key("Escape");
    full.send({ control: "channels", type: "press" });
    full.key("f");
    expect(full.requests.filter((request) => request.type === "fullScreen")).toEqual([
      { on: false, type: "fullScreen" },
      { on: false, type: "fullScreen" },
    ]);
    expect(full.view().listOpen).toBe(true);
  });

  it("tunes the last channel on L", () => {
    const s = screen();
    s.key("ArrowDown");
    s.key("l");
    expect(s.tunes()).toEqual(["ch-2", "ch-1"]);
    s.key("l");
    expect(s.tunes().at(-1)).toBe("ch-2");
  });

  it("says why L does nothing when the last channel cannot be played, or there is none", () => {
    const s = screen();
    s.key("l");
    expect(s.requests).toEqual([{ text: "There is no last channel yet", type: "message" }]);
    const gone = screen({ watchedId: "ch-5" });
    gone.key("ArrowDown");
    expect(gone.view().lastChannel).toMatchObject({ id: "ch-5", playable: false });
    gone.key("l");
    expect(gone.requests.at(-1)).toEqual({ text: "5 Channel 5 cannot be played right now", type: "message" });
    expect(gone.tunes()).toEqual(["ch-6"]);
  });

  it("leaves full screen on Esc on the bare picture", () => {
    const s = screen();
    s.key("Escape");
    expect(s.requests).toEqual([{ on: false, type: "fullScreen" }]);
  });

  it("leaves Space to the control that holds focus", () => {
    const s = screen();
    s.send({ control: "volume", type: "focus" });
    s.key(" ");
    expect(s.requests).toEqual([]);
  });

  it("keeps every other key for an open menu", () => {
    const s = screen();
    s.key("c");
    for (const key of ["ArrowDown", "m", " ", "l", "1"]) s.key(key);
    expect(s.requests).toEqual([{ name: "subtitles", type: "menu" }]);
  });

  it("goes live from the quiet row and the lower third's Go live", () => {
    const s = screen();
    s.key(" ", 0);
    s.key(" ", 3000);
    s.send({ control: "goLive", type: "press" });
    expect(s.requests.at(-1)).toEqual({ type: "goLive" });
    s.key(" ", 4000);
    s.key(" ", 7000);
    s.send({ control: "live", type: "press" });
    expect(s.requests.filter((request) => request.type === "goLive")).toHaveLength(2);
  });
});

describe("rule 7: Back goes one step", () => {
  it("gives focus back to a menu's control when a key closes the menu, and not after a pointer", () => {
    const s = screen();
    s.key("c");
    s.key("Escape");
    expect(s.state.handOff).toEqual({ seq: 1, to: "subtitles" });
    s.key("a");
    s.key("a");
    expect(s.state.handOff).toEqual({ seq: 2, to: "audio" });
    s.send({ control: "more", type: "press" });
    s.send({ fromKey: true, type: "closeMenu" });
    expect(s.state.handOff).toEqual({ seq: 3, to: "more" });
    // A pointer closing a menu hands nothing on.
    s.send({ control: "more", type: "press" });
    s.send({ type: "closeMenu" });
    s.send({ control: "audio", type: "press" });
    s.send({ type: "pictureClick" });
    s.key("?");
    s.key("Escape");
    expect(s.state.handOff).toEqual({ seq: 3, to: "more" });
  });

  it("returns from a menu's page to the menu, then closes it", () => {
    const s = screen();
    s.send({ control: "more", type: "press" });
    s.send({ page: "picture", type: "menuPage" });
    s.send({ type: "back" });
    expect(s.view().menu).toEqual({ name: "more" });
    s.key("Escape");
    expect(s.view().menu).toBeUndefined();
    expect(s.requests).toEqual([{ name: "more", type: "menu" }]);
  });

  it("takes the list's tabs back to the list, closes the list, then the bare picture", () => {
    const s = screen();
    s.send({ control: "channels", type: "press" });
    s.send({ event: { type: "tabs" }, type: "list" });
    s.key("Escape");
    expect(s.state.list.cursor.kind).toBe("row");
    s.key("Escape");
    expect(s.view().listOpen).toBe(false);
    expect(s.requests).toEqual([]);
    s.key("Escape");
    expect(s.requests).toEqual([{ on: false, type: "fullScreen" }]);
  });

  it("closes a menu over the list before the list", () => {
    const s = screen();
    s.send({ control: "channels", type: "press" });
    s.key("a");
    s.key("Escape");
    expect(s.view()).toMatchObject({ listOpen: true, menu: undefined });
  });

  it("closes find before the list", () => {
    const s = screen();
    s.key("/");
    s.key("Escape");
    expect(s.view().listOpen).toBe(true);
    expect(s.state.list.query).toBeNull();
  });
});

describe("rule 8: the first-run hint", () => {
  it("shows until the first channel change, then never again", () => {
    const s = screen();
    expect(s.view().hint).toBe(true);
    s.send({ control: "channels", type: "press" });
    expect(s.view().hint).toBe(false);
    s.key("Escape");
    expect(s.view().hint).toBe(true);
    s.key("ArrowDown");
    expect(s.view().hint).toBe(false);
    s.key("ArrowUp");
    s.send({ type: "pointerMove" });
    expect(s.view().hint).toBe(false);
  });

  it("rests with the controls", () => {
    const s = screen();
    s.wait(WATCH_REST_MS);
    expect(s.view().hint).toBe(false);
    s.send({ type: "pointerMove" });
    expect(s.view().hint).toBe(true);
  });

  it("never shows when the caller says it was seen before", () => {
    expect(screen({ hintSeen: true }).view().hint).toBe(false);
  });

  it("goes when the channel changes from outside", () => {
    const s = screen();
    s.send({ channelId: "ch-4", type: "tuned" });
    expect(s.view().hint).toBe(false);
    expect(s.state.leftIds).toEqual(["ch-1"]);
    expect(s.requests).toEqual([]);
  });
});

describe("sequences from the prototype", () => {
  it("flip, flip, wait, wake by key", () => {
    const s = screen();
    s.key("ArrowDown", 0);
    s.key("ArrowDown", 400);
    s.wait(WATCH_REST_MS + 400);
    expect(s.view().awake).toBe(false);
    s.key("ArrowDown");
    expect(s.view().awake).toBe(true);
    expect(s.tunes()).toEqual(["ch-2", "ch-3", "ch-4"]);
  });

  it("open the list, look, tune, close", () => {
    const s = screen();
    s.send({ control: "channels", type: "press" });
    s.send({ event: { channelId: "ch-8", type: "focusRow" }, type: "list" });
    s.send({ event: { by: 1, type: "move" }, type: "list" });
    expect(s.tunes()).toEqual([]);
    s.send({ event: { type: "choose" }, type: "list" });
    expect(s.tunes()).toEqual(["ch-9"]);
    s.send({ event: { type: "back" }, type: "list" });
    expect(s.view().listOpen).toBe(false);
  });

  it("/ find, type, Down, Enter", () => {
    const s = screen();
    s.key("/");
    s.send({ event: { query: "Channel 1", type: "find" }, type: "list" });
    // Channel 1 (watched), 10, 11 and 12 match.
    s.send({ event: { by: 1, type: "move" }, type: "list" });
    s.send({ event: { type: "choose" }, type: "list" });
    expect(s.tunes()).toEqual(["ch-10"]);
    expect(s.state.list.query).toBeNull();
  });

  it("digits with and without a match", () => {
    const s = screen();
    s.key("4");
    s.wait(WATCH_DIGITS_MS);
    s.key("4");
    s.key("0");
    s.wait(WATCH_DIGITS_MS);
    expect(s.tunes()).toEqual(["ch-4"]);
    expect(s.requests.at(-1)).toEqual({ text: "There is no channel 40", type: "message" });
  });

  it("pause, wait, resume, End", () => {
    const s = screen();
    s.key(" ", 0);
    s.wait(90_000);
    expect(s.view().awake).toBe(true);
    s.key(" ");
    expect(s.view().behindSeconds).toBe(90);
    s.key("End");
    expect(s.requests.map((request) => request.type)).toEqual(["pause", "resume", "goLive"]);
  });

  it("Esc from each layer", () => {
    const s = screen();
    s.key("1");
    s.key("Escape");
    expect(s.view().digits).toBeUndefined();
    s.key("/");
    s.key("Escape");
    s.key("Escape");
    expect(s.view().listOpen).toBe(false);
    s.key("?");
    s.key("Escape");
    expect(s.view().menu).toBeUndefined();
    s.key("Escape");
    expect(s.requests).toEqual([
      { name: "shortcuts", type: "menu" },
      { on: false, type: "fullScreen" },
    ]);
  });
});

describe("a flip steps through the rows the list shows", () => {
  it("goes in number order through favourites starred out of it, from a channel that is not one", () => {
    const s = screen({ group: "favourites", watchedId: "ch-1" }, CHANNELS, ["ch-11", "ch-3", "ch-7"]);
    for (let i = 0; i < 4; i += 1) s.key("ArrowDown");
    expect(s.tunes()).toEqual(["ch-3", "ch-7", "ch-11", "ch-3"]);
    s.key("ArrowUp");
    expect(s.tunes().at(-1)).toBe("ch-11");
  });

  it("stops at the watched channel's number place on the way back into the group", () => {
    const s = screen({ group: "favourites", watchedId: "ch-8" }, CHANNELS, ["ch-11", "ch-3", "ch-7"]);
    s.key("ArrowUp");
    expect(s.tunes()).toEqual(["ch-7"]);
  });

  it("goes by decade, then number, in By decade", () => {
    const decades = [1990, 1980, 1990, 1980].map((decade, i) => channel(i + 1, { decade }));
    const s = screen({ group: "decade", watchedId: "ch-4" }, decades);
    s.key("ArrowDown");
    s.key("ArrowDown");
    expect(s.tunes()).toEqual(["ch-1", "ch-3"]);
  });
});

describe("a double click is full screen and nothing else", () => {
  it("leaves the picture playing after its first click closed the list and its second paused", () => {
    const s = screen();
    s.send({ control: "channels", type: "press" });
    s.send({ type: "pictureClick" }, 0);
    s.send({ type: "pictureClick" }, 150);
    expect(s.view().paused).toBe(true);
    s.send({ type: "pictureDoubleClick" }, 150);
    expect(s.view()).toMatchObject({ behindSeconds: 0, listOpen: false, paused: false });
    expect(s.requests.slice(-2)).toEqual([{ on: false, type: "fullScreen" }, { type: "resume" }]);
  });

  it("counts no time behind live for its two clicks", () => {
    const s = screen();
    s.send({ type: "pictureClick" }, 0);
    s.send({ type: "pictureClick" }, 150);
    s.send({ type: "pictureDoubleClick" }, 150);
    expect(s.state.behindMs).toBe(0);
    s.key("End");
    expect(s.requests.some((request) => request.type === "goLive")).toBe(false);
  });

  it("goes back to the playback before its first click however long the system's interval is", () => {
    const s = screen();
    s.send({ type: "pictureClick" }, 0);
    s.send({ type: "pictureClick" }, 600);
    s.send({ type: "pictureDoubleClick" }, 600);
    expect(s.state).toMatchObject({ behindMs: 0, paused: false });
  });

  it("plays on from a pause, behind by the time it was paused before the clicks", () => {
    const s = screen();
    s.key(" ", 0);
    s.send({ type: "pictureClick" }, 10_000);
    s.send({ type: "pictureClick" }, 10_150);
    s.send({ type: "pictureDoubleClick" }, 10_150);
    expect(s.view()).toMatchObject({ behindSeconds: 10, paused: false });
  });
});

describe("a press away from an open menu closes it", () => {
  it("does nothing else for a control that opens no menu", () => {
    const s = screen();
    s.send({ control: "subtitles", type: "press" });
    s.send({ control: "pause", type: "press" });
    expect(s.view()).toMatchObject({ menu: undefined, paused: false });
    s.send({ control: "subtitles", type: "press" });
    s.send({ control: "channels", type: "press" });
    expect(s.view()).toMatchObject({ listOpen: false, menu: undefined });
    expect(s.requests.filter((request) => request.type !== "menu")).toEqual([]);
  });

  it("switches to a control's own menu, or closes its own", () => {
    const s = screen();
    s.send({ control: "subtitles", type: "press" });
    s.send({ control: "audio", type: "press" });
    expect(s.view().menu).toEqual({ name: "audio" });
    s.send({ control: "audio", type: "press" });
    expect(s.view().menu).toBeUndefined();
  });
});

describe("only a picture that fills the screen rests", () => {
  it.each(["window", "portrait"] as const)("never rests in the %s layout", (layout) => {
    const s = screen({ layout });
    expect(s.view().nextAt).toBeNull();
    s.wait(60_000);
    expect(s.view().awake).toBe(true);
  });

  it.each(["full", "tv", "landscape"] as const)("rests in the %s layout", (layout) => {
    const s = screen({ layout });
    s.wait(WATCH_REST_MS);
    expect(s.view().awake).toBe(false);
  });
});

describe("only a playing picture pauses", () => {
  it.each(["tuning", "covered"] as const)("refuses Space and a click while %s", (picture) => {
    const s = screen();
    s.send({ picture, type: "picture" });
    s.key(" ");
    s.send({ type: "pictureClick" });
    expect(s.requests).toEqual([]);
    expect(s.view().paused).toBe(false);
    s.send({ picture: "playing", type: "picture" });
    s.key(" ");
    expect(s.requests).toEqual([{ type: "pause" }]);
  });

  it("holds a pause that a cover came over until the picture is back, and the picture's news wakes nothing", () => {
    const s = screen();
    s.key(" ", 0);
    s.send({ picture: "covered", type: "picture" }, 1000);
    expect(s.state.activeAt).toBe(0);
    s.key(" ", 2000);
    expect(s.requests.at(-1)).toEqual({ type: "pause" });
    s.send({ picture: "playing", type: "picture" }, 3000);
    s.key(" ", 4000);
    expect(s.requests.at(-1)).toEqual({ type: "resume" });
  });
});

describe("what holds the screen awake", () => {
  it("lets go of the pointer and focus on the lower third when the list takes its place", () => {
    const s = screen();
    s.send({ held: true, region: "lowerThird", type: "pointerHold" });
    s.send({ control: "goLive", type: "focus" });
    s.key("/");
    expect(s.state).toMatchObject({ focus: undefined, pointerOn: [] });
    s.key("Escape");
    s.key("Escape");
    expect(s.view().listOpen).toBe(false);
    s.wait(WATCH_REST_MS);
    expect(s.view().awake).toBe(false);
  });

  it("keeps the focus of a television row's Go live when the lower third is not drawn", () => {
    const s = screen({ layout: "tv" });
    s.send({ control: "live", type: "focus" });
    s.send({ control: "channels", type: "press" });
    expect(s.state.focus).toBe("live");
  });

  it("lets go of the lower third's holds when the watched channel leaves the lineup", () => {
    const channels = [...CHANNELS];
    const s = screen({}, channels);
    s.send({ held: true, region: "lowerThird", type: "pointerHold" });
    s.send({ control: "goLive", type: "focus" });
    s.send({ held: true, region: "quietRow", type: "pointerHold" });
    // The channel on screen goes from the lineup, and its lower third with it.
    channels.splice(0, 1);
    s.send({ type: "pointerMove" });
    expect(s.state).toMatchObject({ focus: undefined, pointerOn: ["quietRow"] });
    s.send({ held: false, region: "quietRow", type: "pointerHold" });
    s.wait(WATCH_REST_MS);
    expect(s.view().awake).toBe(false);
  });

  it("keeps the pointer's hold on the quiet row through the list opening and closing", () => {
    const s = screen();
    s.send({ held: true, region: "quietRow", type: "pointerHold" }, 0);
    s.send({ control: "channels", type: "press" }, 0);
    s.send({ control: "channels", type: "press" }, 2000);
    expect(s.view().listOpen).toBe(false);
    s.wait(60_000);
    expect(s.view().awake).toBe(true);
    // The keyboard's way round: "/", then Esc twice, under a still pointer.
    s.key("/");
    s.key("Escape");
    s.key("Escape");
    s.wait(60_000);
    expect(s.view().awake).toBe(true);
  });

  it("gives focus back to Channels only when a key closes the list", () => {
    const s = screen();
    s.send({ control: "channels", type: "press" });
    s.send({ type: "pictureClick" });
    s.send({ control: "channels", type: "press" });
    s.send({ event: { type: "close" }, type: "list" });
    s.send({ control: "channels", type: "press" });
    s.send({ control: "channels", type: "press" });
    expect(s.state.handOff).toBeUndefined();
    s.send({ control: "channels", type: "press" });
    s.send({ event: { type: "back" }, fromKey: true, type: "list" });
    expect(s.state.handOff).toEqual({ seq: 1, to: "channels" });
  });

  it("closes the list and any menu on a change of layout", () => {
    const s = screen();
    s.send({ control: "channels", type: "press" });
    s.key("c");
    s.send({ layout: "window", type: "layout" });
    expect(s.view()).toMatchObject({ listOpen: false, menu: undefined });
  });
});

describe("digits and tunes", () => {
  it("commits digits already due before a late event", () => {
    const s = screen();
    s.key("4", 0);
    s.key("2", 2000);
    expect(s.tunes()).toEqual(["ch-4"]);
    expect(s.view().digits?.typed).toBe("2");
  });

  it("lets a flip win over typed digits, which are dropped", () => {
    const s = screen();
    s.key("1");
    s.key("1");
    s.key("ArrowDown");
    s.wait(WATCH_DIGITS_MS * 2);
    expect(s.tunes()).toEqual(["ch-2"]);
    expect(s.view().digits).toBeUndefined();
  });

  it("returns to live on any tune", () => {
    const s = screen();
    s.key(" ", 0);
    s.key(" ", 30_000);
    expect(s.view().behindSeconds).toBe(30);
    s.key("ArrowDown");
    expect(s.view()).toMatchObject({ behindSeconds: 0, paused: false });
  });

  it("changes nothing on an echo of the channel on screen", () => {
    const s = screen();
    s.key("ArrowDown", 0);
    const before = s.state;
    s.send({ channelId: "ch-2", type: "tuned" }, 3000);
    expect(s.state).toBe(before);
    expect(s.requests).toEqual([{ channelId: "ch-2", type: "tune" }]);
  });
});

describe("watchKeyOf", () => {
  it("passes a plain key and keeps the ones that are not the screen's", () => {
    expect(watchKeyOf({ key: "ArrowDown" })).toBe("ArrowDown");
    expect(watchKeyOf({ ctrlKey: true, key: "f" })).toBeUndefined();
    expect(watchKeyOf({ key: "f", metaKey: true })).toBeUndefined();
    expect(watchKeyOf({ altKey: true, key: "m" })).toBeUndefined();
    expect(watchKeyOf({ defaultPrevented: true, key: "ArrowDown" })).toBeUndefined();
    expect(watchKeyOf({ inField: true, key: "f" })).toBeUndefined();
    expect(watchKeyOf({ composing: true, key: "Enter" })).toBeUndefined();
  });
});
