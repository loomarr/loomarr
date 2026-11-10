// @vitest-environment jsdom

import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  createTvListTune,
  runTvRemoteIntent,
  TV_FAVOURITE_NOTE_MS,
  type TvRemoteController,
  tvGuideChannelWindow,
  tvRemoteHasDigits,
  useTvAutoTune,
  useTvFavourites,
  useTvMenuKey,
  useWatchingControls,
} from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

/** Renders a hook the way a shell does; `latest()` is what it returned on the last render. */
const mountHook = async <Result,>(hook: () => Result) => {
  let result!: Result;
  const Probe = () => {
    result = hook();
    return null;
  };
  const container = (
    globalThis as unknown as { document: { createElement: (tag: string) => never } }
  ).document.createElement("div");
  const root = createRoot(container);
  await act(async () => root.render(<Probe />));
  return {
    latest: () => result,
    rerender: () => act(async () => root.render(<Probe />)),
    unmount: () => act(async () => root.unmount()),
  };
};

afterEach(() => {
  vi.useRealTimers();
});

describe("which remote the TV has", () => {
  it("gives the Siri Remote no digits, and every other remote its number keys", () => {
    expect(tvRemoteHasDigits({ OS: "ios", isTV: true })).toBe(false);
    expect(tvRemoteHasDigits({ OS: "android", isTV: true })).toBe(true);
    expect(tvRemoteHasDigits({ OS: "ios", isTV: false })).toBe(true);
  });
});

describe("useTvMenuKey", () => {
  const control = () => ({ disableTVMenuKey: vi.fn(), enableTVMenuKey: vi.fn() });

  it("holds the Menu key on tvOS only while there is somewhere to go back to", async () => {
    const menu = control();
    let enabled = true;
    const view = await mountHook(() => useTvMenuKey(enabled, { control: menu, tvOS: true }));
    expect(menu.enableTVMenuKey).toHaveBeenCalledTimes(1);
    expect(menu.disableTVMenuKey).not.toHaveBeenCalled();

    // At a root, Menu goes to the Home screen.
    enabled = false;
    await view.rerender();
    expect(menu.disableTVMenuKey).toHaveBeenCalledTimes(1);

    enabled = true;
    await view.rerender();
    expect(menu.enableTVMenuKey).toHaveBeenCalledTimes(2);
    await view.unmount();
    expect(menu.disableTVMenuKey).toHaveBeenCalledTimes(2);
  });

  it("never takes the key while no screen has anywhere to go back to", async () => {
    const menu = control();
    const view = await mountHook(() => useTvMenuKey(false, { control: menu, tvOS: true }));
    await view.unmount();
    expect(menu.enableTVMenuKey).not.toHaveBeenCalled();
    expect(menu.disableTVMenuKey).not.toHaveBeenCalled();
  });

  it("leaves Android TV alone: its Back always reaches the app", async () => {
    const menu = control();
    const view = await mountHook(() => useTvMenuKey(true, { control: menu, tvOS: false }));
    await view.unmount();
    expect(menu.enableTVMenuKey).not.toHaveBeenCalled();
  });
});

describe("useTvAutoTune", () => {
  const store = (stored: Promise<number>) => ({
    load: vi.fn(() => stored),
    save: vi.fn(async (durationMs: number) => durationMs),
  });
  const settle = () => act(async () => undefined);

  it("starts on the default wait and takes the stored one once it loads", async () => {
    const setting = store(Promise.resolve(5_000));
    const view = await mountHook(() => useTvAutoTune(setting));
    await settle();
    expect(view.latest()[0]).toBe(5_000);
    await view.unmount();
  });

  it("lets a choice made before the stored value loads win over that stale read", async () => {
    let loaded: (durationMs: number) => void = () => undefined;
    const setting = store(new Promise<number>((resolve) => (loaded = resolve)));
    const view = await mountHook(() => useTvAutoTune(setting));
    const [defaultMs, change] = view.latest();

    await act(async () => change(3_000));
    await act(async () => loaded(10_000));

    expect(view.latest()[0]).toBe(3_000);
    expect(defaultMs).not.toBe(3_000);
    expect(setting.save).toHaveBeenCalledExactlyOnceWith(3_000);
    await view.unmount();
  });

  it("keeps the default when the store cannot be read, and applies a choice the store cannot write", async () => {
    const setting = {
      load: vi.fn(() => Promise.reject(new Error("locked"))),
      save: vi.fn(() => Promise.reject(new Error("full"))),
    };
    const view = await mountHook(() => useTvAutoTune(setting));
    const defaultMs = view.latest()[0];
    await settle();
    expect(view.latest()[0]).toBe(defaultMs);

    await act(async () => view.latest()[1](2_000));
    expect(view.latest()[0]).toBe(2_000);
    await view.unmount();
  });
});

describe("useWatchingControls", () => {
  it("shows the controls, restarts their timer on activity and hides them on dismiss", async () => {
    const view = await mountHook(useWatchingControls);
    expect(view.latest()).toMatchObject({ activityKey: 0, visible: true });

    await act(async () => view.latest().dismiss());
    expect(view.latest().visible).toBe(false);

    await act(async () => view.latest().showForActivity());
    expect(view.latest()).toMatchObject({ activityKey: 1, visible: true });
    await view.unmount();
  });

  it("keeps both callbacks across renders, so playback re-renders never restart the inactivity timer", async () => {
    const view = await mountHook(useWatchingControls);
    const first = view.latest();
    await act(async () => first.showForActivity());
    await view.rerender();

    expect(view.latest().dismiss).toBe(first.dismiss);
    expect(view.latest().showForActivity).toBe(first.showForActivity);
    await view.unmount();
  });
});

describe("a remote press on Watching", () => {
  const controller = (
    state: { channel?: unknown; status: string } = { channel: { id: "a" }, status: "playing" },
  ) =>
    ({
      getSnapshot: () => state,
      pause: vi.fn(),
      play: vi.fn(async () => undefined),
      step: vi.fn(async () => undefined),
      tuneChannel: vi.fn(async () => undefined),
      tuneNumber: vi.fn(async () => undefined),
    }) satisfies TvRemoteController;

  it("steps the channel and tunes a typed number", () => {
    const player = controller();
    const open = vi.fn();
    runTvRemoteIntent({ direction: 1, kind: "step" }, player, open);
    runTvRemoteIntent({ direction: -1, kind: "step" }, player, open);
    runTvRemoteIntent({ digits: "042", kind: "tune-number" }, player, open);

    expect(player.step.mock.calls).toEqual([[1], [-1]]);
    expect(player.tuneNumber).toHaveBeenCalledExactlyOnceWith("042");
    expect(open).not.toHaveBeenCalled();
  });

  it("opens the Guide or Surf, and does nothing for a press that asked for nothing", () => {
    const player = controller();
    const open = vi.fn();
    runTvRemoteIntent({ kind: "open-guide" }, player, open);
    runTvRemoteIntent({ kind: "open-surf" }, player, open);
    runTvRemoteIntent(undefined, player, open);

    expect(open.mock.calls).toEqual([["guide"], ["surf"]]);
    expect(player.pause).not.toHaveBeenCalled();
    expect(player.step).not.toHaveBeenCalled();
  });

  it("pauses a playing channel and plays a paused one, and leaves an untuned player alone", () => {
    const playing = controller({ channel: { id: "a" }, status: "playing" });
    runTvRemoteIntent({ kind: "toggle-pause" }, playing, vi.fn());
    expect(playing.pause).toHaveBeenCalledTimes(1);
    expect(playing.play).not.toHaveBeenCalled();

    const paused = controller({ channel: { id: "a" }, status: "paused" });
    runTvRemoteIntent({ kind: "toggle-pause" }, paused, vi.fn());
    expect(paused.play).toHaveBeenCalledTimes(1);
    expect(paused.pause).not.toHaveBeenCalled();

    const idle = controller({ status: "idle" });
    runTvRemoteIntent({ kind: "toggle-pause" }, idle, vi.fn());
    expect(idle.play).not.toHaveBeenCalled();
    expect(idle.pause).not.toHaveBeenCalled();
  });
});

describe("picking a channel in the Guide or Surf", () => {
  const pick = (releaseOfLongPress: boolean) => {
    const player = { tuneChannel: vi.fn(async () => undefined) };
    const showControls = vi.fn();
    const open = vi.fn();
    createTvListTune(
      { consumeLongSelectRelease: () => releaseOfLongPress },
      player,
      showControls,
      open,
    )("ten");
    return { open, player, showControls };
  };

  it("tunes the channel, restarts the controls' timer and shows Watching", () => {
    const { open, player, showControls } = pick(false);
    expect(player.tuneChannel).toHaveBeenCalledExactlyOnceWith("ten");
    expect(showControls).toHaveBeenCalledTimes(1);
    expect(open).toHaveBeenCalledExactlyOnceWith("watching");
  });

  it("ignores the press tvOS sends when a long-press Select lifts, which already starred the row", () => {
    const { open, player, showControls } = pick(true);
    expect(player.tuneChannel).not.toHaveBeenCalled();
    expect(showControls).not.toHaveBeenCalled();
    expect(open).not.toHaveBeenCalled();
  });
});

describe("useTvFavourites", () => {
  const favourites = (initial: string[], honours = true) => {
    let ids = initial;
    return {
      getSnapshot: () => ({ favouriteIds: ids }),
      setFavourite: vi.fn(async (channelId: string, starred: boolean) => {
        if (honours) ids = starred ? [...ids, channelId] : ids.filter((id) => id !== channelId);
      }),
    };
  };
  const catalog = [{ id: "ten", name: "Ten" }];

  it("stars a channel that is not a favourite, says so aloud, and clears the note", async () => {
    vi.useFakeTimers();
    const source = favourites([]);
    const announce = vi.fn();
    const view = await mountHook(() => useTvFavourites(source, catalog, announce));

    await act(async () => view.latest().toggle("ten"));

    expect(source.setFavourite).toHaveBeenCalledExactlyOnceWith("ten", true);
    expect(view.latest().note).toBe("Added to Favorites · Ten");
    expect(announce).toHaveBeenCalledExactlyOnceWith("Added to Favorites · Ten");

    await act(async () => vi.advanceTimersByTime(TV_FAVOURITE_NOTE_MS - 1));
    expect(view.latest().note).toBe("Added to Favorites · Ten");
    await act(async () => vi.advanceTimersByTime(1));
    expect(view.latest().note).toBeUndefined();
    await view.unmount();
  });

  it("unstars a favourite", async () => {
    vi.useFakeTimers();
    const source = favourites(["ten"]);
    const view = await mountHook(() => useTvFavourites(source, catalog, vi.fn()));

    await act(async () => view.latest().toggle("ten"));

    expect(source.setFavourite).toHaveBeenCalledExactlyOnceWith("ten", false);
    expect(view.latest().note).toBe("Removed from Favorites · Ten");
    await view.unmount();
  });

  it("reads what the lists say afterwards when the server refuses the change", async () => {
    vi.useFakeTimers();
    const source = favourites([], false);
    const announce = vi.fn();
    const view = await mountHook(() => useTvFavourites(source, catalog, announce));

    await act(async () => view.latest().toggle("ten"));

    expect(view.latest().note).toBe("Couldn't update Favorites");
    expect(announce).toHaveBeenCalledExactlyOnceWith("Couldn't update Favorites");
    await view.unmount();
  });

  it("leaves the channel's name off a channel the catalog no longer has", async () => {
    vi.useFakeTimers();
    const view = await mountHook(() => useTvFavourites(favourites([]), [], vi.fn()));
    await act(async () => view.latest().toggle("gone"));
    expect(view.latest().note).toBe("Added to Favorites");
    await view.unmount();
  });

  it("cancels the pending clear when the shell goes away", async () => {
    vi.useFakeTimers();
    const view = await mountHook(() => useTvFavourites(favourites([]), catalog, vi.fn()));
    await act(async () => view.latest().toggle("ten"));
    expect(vi.getTimerCount()).toBe(1);

    await view.unmount();
    expect(vi.getTimerCount()).toBe(0);
  });
});

describe("the Guide's channel window", () => {
  const layout = (count: number) =>
    ({ channels: Array.from({ length: count }, (_, i) => ({ source: { channelId: `c${i}` } })) }) as never;

  it("mounts the rows around the selected channel, not the whole lineup", () => {
    expect(tvGuideChannelWindow(layout(100), { channelId: "c50" })).toEqual({
      end: 56,
      positionLabel: "51 of 100",
      start: 44,
    });
  });

  it("falls back to the first rows when the selected channel is not in the layout", () => {
    expect(tvGuideChannelWindow(layout(100), { channelId: "elsewhere" })).toEqual({
      end: 10,
      positionLabel: "1 of 100",
      start: 0,
    });
  });
});
