import { TUNE_FLIP_MS, TUNE_UNCOVER_MS } from "@loomarr/core/tune";
import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { WatchTuneReport } from "../use-watch-player";
import { useWatchTune } from "./use-watch-tune";

beforeEach(() => vi.useFakeTimers({ now: 1_000_000 }));
afterEach(() => vi.useRealTimers());

// The hook sets its next timer as it draws, so time moves on a step at a time.
const advance = (ms: number) => {
  for (let step = 0; step < ms; step += 10) {
    act(() => {
      vi.advanceTimersByTime(Math.min(10, ms - step));
    });
  }
};

const report = (attemptId: number, over: Partial<WatchTuneReport> = {}): WatchTuneReport => ({
  attemptId,
  channelId: `ch-${attemptId}`,
  reason: "channel",
  warm: false,
  ...over,
});

// A page whose first picture has played, so the next tune is a channel change.
const playing = () => {
  const hook = renderHook(() => useWatchTune());
  act(() => hook.result.current.onTune(report(1)));
  act(() => hook.result.current.settle(1, "picture"));
  advance(TUNE_FLIP_MS);
  return hook;
};

describe("useWatchTune", () => {
  it("holds the page's first tune as a tune-in, even to a warmed channel, until its picture", () => {
    const { result } = renderHook(() => useWatchTune());
    act(() => result.current.onTune(report(1, { warm: true })));
    advance(1_000);
    expect(result.current.view.cover).toBe("cold");
    act(() => result.current.settle(1, "picture"));
    expect(result.current.view).toMatchObject({ bars: "locked", cover: "none" });
  });

  it("covers a change to a warmed channel briefly and uncovers it by itself", () => {
    const { result } = playing();
    act(() => result.current.onTune(report(2, { warm: true })));
    expect(result.current.view).toMatchObject({ bars: "run", cover: "burst" });
    advance(TUNE_UNCOVER_MS);
    expect(result.current.view.cover).toBe("none");
  });

  it("holds a channel that must start for the real wait", () => {
    const { result } = playing();
    act(() => result.current.onTune(report(2)));
    advance(5_000);
    expect(result.current.view.cover).toBe("cold");
    act(() => result.current.settle(2, "picture"));
    expect(result.current.view.cover).toBe("none");
  });

  it("ends a tune-in with no picture when its attempt comes to none", () => {
    const { result } = playing();
    act(() => result.current.onTune(report(2)));
    act(() => result.current.settle(2, "none"));
    expect(result.current.view).toMatchObject({ bars: "none", cover: "none" });
  });

  it("ignores the picture of an attempt a later tune replaced", () => {
    const { result } = playing();
    act(() => result.current.onTune(report(2)));
    act(() => result.current.onTune(report(3)));
    act(() => result.current.settle(2, "picture"));
    expect(result.current.view.cover).toBe("cold");
  });

  it("takes a reconnect to the channel on screen as reconnecting, not a tune", () => {
    const { result } = playing();
    act(() => result.current.onTune(report(2, { channelId: "ch-1", reason: "retry" })));
    expect(result.current.reconnecting).toBe(true);
    expect(result.current.view.cover).toBe("none");
    act(() => result.current.settle(2, "picture"));
    expect(result.current.reconnecting).toBe(false);
  });
});
