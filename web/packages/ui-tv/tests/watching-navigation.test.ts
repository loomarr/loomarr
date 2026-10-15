import { describe, expect, it } from "vitest";

import {
  clampNumberEntryMs,
  DEFAULT_NUMBER_ENTRY_MS,
  initialTvWatchingRemoteState,
  MAX_NUMBER_ENTRY_MS,
  MIN_NUMBER_ENTRY_MS,
  reduceTvWatchingRemote,
  type TvWatchingRemoteState,
  tvNumberEntryPresentation,
  tvWatchingRemoteEventFromNative,
} from "../index";

describe("TV Watching remote navigation", () => {
  it("translates native TV event names at the platform boundary", () => {
    expect(tvWatchingRemoteEventFromNative("7", 100)).toEqual({
      atMs: 100,
      digit: "7",
      key: "digit",
    });
    expect(tvWatchingRemoteEventFromNative("channelUp", 100)).toEqual({
      direction: "up",
      key: "channel",
    });
    expect(tvWatchingRemoteEventFromNative("channelDown", 100)).toEqual({
      direction: "down",
      key: "channel",
    });
    expect(tvWatchingRemoteEventFromNative("select", 100)).toEqual({ key: "select" });
    expect(tvWatchingRemoteEventFromNative("select", 100, 0)).toBeUndefined();
    expect(tvWatchingRemoteEventFromNative("select", 100, 1)).toEqual({ key: "select" });
  });

  // react-native-tvos delivers playPause and longSelect with eventKeyAction 0 on the way down and 1 on
  // the way up; a physical press is the pair, and only the release may act.
  it("acts once on a Play/Pause press: the key-down is dropped, the release counts", () => {
    const presses = [0, 1].map((action) => tvWatchingRemoteEventFromNative("playPause", 100, action));
    expect(presses).toEqual([undefined, { key: "playPause" }]);
    expect(tvWatchingRemoteEventFromNative("playPause", 100)).toEqual({ key: "playPause" });
    const toggles = presses.map((event) =>
      event ? reduceTvWatchingRemote(initialTvWatchingRemoteState, event).intent : undefined,
    );
    expect(toggles).toEqual([undefined, { kind: "toggle-pause" }]);
  });

  it("toggles pause and keeps typed channel digits pending", () => {
    const typing = reduceTvWatchingRemote(initialTvWatchingRemoteState, {
      atMs: 100,
      digit: "4",
      key: "digit",
    });
    const result = reduceTvWatchingRemote(typing.state, { key: "playPause" });
    expect(result).toEqual({ handled: true, intent: { kind: "toggle-pause" }, state: typing.state });
  });

  it("long-presses Select once, on release, as handled activity with no intent", () => {
    const presses = [0, 1].map((action) => tvWatchingRemoteEventFromNative("longSelect", 100, action));
    expect(presses).toEqual([undefined, { key: "longSelect" }]);
    // No programme details panel exists yet: the press wakes the chrome and opens nothing.
    expect(reduceTvWatchingRemote(initialTvWatchingRemoteState, { key: "longSelect" })).toEqual({
      handled: true,
      state: initialTvWatchingRemoteState,
    });
  });

  it("leaves a long press from opening the Guide the way a short Select does", () => {
    const short = reduceTvWatchingRemote(initialTvWatchingRemoteState, { key: "select" });
    const long = reduceTvWatchingRemote(initialTvWatchingRemoteState, { key: "longSelect" });
    expect(short.intent).toEqual({ kind: "open-guide" });
    expect(long.intent).toBeUndefined();
  });

  it("keeps a swipe down on the touch surface as channel down", () => {
    expect(tvWatchingRemoteEventFromNative("down", 100, 1)).toEqual({ key: "down" });
    expect(reduceTvWatchingRemote(initialTvWatchingRemoteState, { key: "down" }).intent).toEqual({
      direction: -1,
      kind: "step",
    });
  });

  it("maps the current Shield remote controls and leaves platform-owned keys unhandled", () => {
    expect(reduceTvWatchingRemote(initialTvWatchingRemoteState, { key: "up" })).toMatchObject({
      handled: true,
      intent: { direction: 1, kind: "step" },
    });
    expect(
      reduceTvWatchingRemote(initialTvWatchingRemoteState, { direction: "down", key: "channel" }),
    ).toMatchObject({ handled: true, intent: { direction: -1, kind: "step" } });
    expect(reduceTvWatchingRemote(initialTvWatchingRemoteState, { key: "select" })).toMatchObject({
      handled: true,
      intent: { kind: "open-guide" },
    });
    expect(reduceTvWatchingRemote(initialTvWatchingRemoteState, { key: "menu" })).toMatchObject({
      handled: true,
      intent: { kind: "open-surf" },
    });
    expect(reduceTvWatchingRemote(initialTvWatchingRemoteState, { key: "back" }).handled).toBe(false);
    expect(reduceTvWatchingRemote(initialTvWatchingRemoteState, { key: "right" }).handled).toBe(false);
  });

  it("keeps the last three digits and restarts the bounded timeout", () => {
    let state: TvWatchingRemoteState = initialTvWatchingRemoteState;
    for (const [digit, atMs] of [
      ["1", 0],
      ["2", 100],
      ["3", 200],
      ["4", 300],
    ] as const) {
      state = reduceTvWatchingRemote(state, { atMs, digit, key: "digit" }).state;
    }

    expect(state).toEqual({ numberEntry: { digits: "234", expiresAtMs: 1_500 } });
    expect(reduceTvWatchingRemote(state, { atMs: 1_499, key: "timeout" })).toEqual({
      handled: false,
      state,
    });
    expect(reduceTvWatchingRemote(state, { atMs: 1_500, key: "timeout" })).toEqual({
      handled: true,
      intent: { digits: "234", kind: "tune-number" },
      state: {},
    });
  });

  it("commits digits with Select instead of opening Guide", () => {
    const entered = reduceTvWatchingRemote(initialTvWatchingRemoteState, {
      atMs: 10,
      digit: "7",
      key: "digit",
    }).state;

    expect(reduceTvWatchingRemote(entered, { key: "select" })).toEqual({
      handled: true,
      intent: { digits: "7", kind: "tune-number" },
      state: {},
    });
  });

  it("previews only an exact Channel number and keeps leading-zero tune identity", () => {
    const channels = [
      { name: "Science Fiction", number: 7 },
      { name: "Nature", number: 21 },
    ];
    const partial = reduceTvWatchingRemote(initialTvWatchingRemoteState, {
      atMs: 10,
      digit: "2",
      key: "digit",
    }).state;
    expect(tvNumberEntryPresentation(partial, channels)).toEqual({
      channelName: undefined,
      digits: "2",
      expiresAtMs: 1_210,
    });

    const leadingZero = reduceTvWatchingRemote(partial, { atMs: 20, digit: "1", key: "digit" }).state;
    expect(tvNumberEntryPresentation(leadingZero, channels)?.channelName).toBe("Nature");
    const exact = reduceTvWatchingRemote(initialTvWatchingRemoteState, {
      atMs: 30,
      digit: "0",
      key: "digit",
    }).state;
    const seven = reduceTvWatchingRemote(exact, { atMs: 40, digit: "7", key: "digit" }).state;
    expect(tvNumberEntryPresentation(seven, channels)?.channelName).toBe("Science Fiction");
  });

  it("accepts a longer per-device auto-tune duration (#1659 decision N4, WCAG 2.2.1)", () => {
    const state = reduceTvWatchingRemote(
      initialTvWatchingRemoteState,
      { atMs: 1_000, digit: "4", key: "digit" },
      5_000,
    ).state;
    expect(state).toEqual({ numberEntry: { digits: "4", expiresAtMs: 6_000 } });
  });

  it("clamps a configured auto-tune duration to the supported range", () => {
    expect(clampNumberEntryMs(0)).toBe(MIN_NUMBER_ENTRY_MS);
    expect(clampNumberEntryMs(DEFAULT_NUMBER_ENTRY_MS)).toBe(DEFAULT_NUMBER_ENTRY_MS);
    expect(clampNumberEntryMs(60_000)).toBe(MAX_NUMBER_ENTRY_MS);
  });
});
