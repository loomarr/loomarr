import { describe, expect, it } from "vitest";

import {
  clampNumberEntryMs,
  DEFAULT_NUMBER_ENTRY_MS,
  initialTvWatchingRemoteState,
  MAX_NUMBER_ENTRY_MS,
  MIN_NUMBER_ENTRY_MS,
  reduceTvWatchingRemote,
  type TvWatchingRemoteEvent,
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
    // No programme details panel exists yet: the press opens nothing.
    expect(reduceTvWatchingRemote(initialTvWatchingRemoteState, { key: "longSelect" })).toEqual({
      handled: true,
      state: initialTvWatchingRemoteState,
    });
  });

  // #2088 decision 15: with the controls hidden the remote drives the picture.
  it("maps the remote over a bare picture: OK or Down shows the controls, Left/Right step 10 s", () => {
    const hidden = (event: TvWatchingRemoteEvent) =>
      reduceTvWatchingRemote(initialTvWatchingRemoteState, event, DEFAULT_NUMBER_ENTRY_MS, false);
    expect(hidden({ key: "select" })).toMatchObject({ handled: true, intent: { kind: "show-controls" } });
    expect(hidden({ key: "down" })).toMatchObject({ handled: true, intent: { kind: "show-controls" } });
    expect(hidden({ key: "left" })).toMatchObject({ handled: true, intent: { direction: -1, kind: "seek" } });
    expect(hidden({ key: "right" })).toMatchObject({ handled: true, intent: { direction: 1, kind: "seek" } });
    expect(hidden({ key: "up" })).toMatchObject({ handled: true, intent: { direction: 1, kind: "step" } });
    expect(hidden({ direction: "down", key: "channel" })).toMatchObject({
      handled: true,
      intent: { direction: -1, kind: "step" },
    });
    expect(hidden({ key: "playPause" }).intent).toEqual({ kind: "toggle-pause" });
    // Android TV's ☰ key still opens Surf; tvOS's Menu is the platform's Back and reaches no app at the root.
    expect(hidden({ key: "menu" }).intent).toEqual({ kind: "open-surf" });
    // Back leaves Watching, which the platform's back handling owns.
    expect(hidden({ key: "back" }).handled).toBe(false);
    expect(hidden({ key: "longSelect" }).intent).toBeUndefined();
  });

  // With the controls shown the D-pad belongs to focus, Back hides them, and nothing tunes.
  it("hands the D-pad to focus while the controls show, and Back hides them", () => {
    const shown = (event: TvWatchingRemoteEvent) =>
      reduceTvWatchingRemote(initialTvWatchingRemoteState, event, DEFAULT_NUMBER_ENTRY_MS, true);
    for (const key of ["up", "down", "left", "right", "select", "menu"] as const) {
      expect(shown({ key })).toEqual({ handled: false, state: initialTvWatchingRemoteState });
    }
    expect(shown({ direction: "up", key: "channel" }).handled).toBe(false);
    expect(shown({ atMs: 10, digit: "4", key: "digit" }).handled).toBe(false);
    expect(shown({ key: "back" })).toEqual({
      handled: true,
      intent: { kind: "hide-controls" },
      state: initialTvWatchingRemoteState,
    });
    expect(shown({ key: "playPause" }).intent).toEqual({ kind: "toggle-pause" });
  });

  it("finishes a number typed over the picture even after the controls come up", () => {
    const typed = reduceTvWatchingRemote(initialTvWatchingRemoteState, {
      atMs: 0,
      digit: "7",
      key: "digit",
    }).state;
    expect(
      reduceTvWatchingRemote(typed, { atMs: 1_200, key: "timeout" }, DEFAULT_NUMBER_ENTRY_MS, true).intent,
    ).toEqual({ digits: "7", kind: "tune-number" });
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

  it("commits digits with Select instead of showing the controls", () => {
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
