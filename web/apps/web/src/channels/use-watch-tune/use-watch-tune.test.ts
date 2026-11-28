import { TUNE_FLIP_MS, TUNE_UNCOVER_MS } from "@loomarr/core/tune";
import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
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

describe("useWatchTune", () => {
  it("covers a change briefly and uncovers it by itself, with no picture event", () => {
    const { result } = renderHook(() => useWatchTune());
    act(() => {
      result.current.flip({ firstTune: false, streaming: true });
    });
    expect(result.current.view.cover).toBe("burst");
    expect(result.current.view.bars).toBe("run");
    advance(TUNE_UNCOVER_MS);
    expect(result.current.view.cover).toBe("none");
    advance(TUNE_FLIP_MS);
    expect(result.current.view.bars).toBe("none");
  });

  it("holds a tune-in's static for the real wait and uncovers it when the picture is ready", () => {
    const { result } = renderHook(() => useWatchTune());
    let flip = 0;
    act(() => {
      flip = result.current.flip({ firstTune: false, streaming: false });
    });
    advance(5_000);
    expect(result.current.view.cover).toBe("cold");
    act(() => result.current.ready(flip));
    expect(result.current.view.cover).toBe("none");
    expect(result.current.view.bars).toBe("locked");
  });

  it("treats the first tune of a page as a tune-in, even to a warmed channel", () => {
    const { result } = renderHook(() => useWatchTune());
    act(() => {
      result.current.flip({ firstTune: true, streaming: true });
    });
    advance(1_000);
    expect(result.current.view.cover).toBe("cold");
  });

  it("ends a tune-in with no picture when it is abandoned", () => {
    const { result } = renderHook(() => useWatchTune());
    let flip = 0;
    act(() => {
      flip = result.current.flip({ firstTune: false, streaming: false });
    });
    act(() => result.current.abandon(flip));
    expect(result.current.view).toMatchObject({ bars: "none", cover: "none" });
  });

  it("ignores the picture of a flip that a later flip replaced", () => {
    const { result } = renderHook(() => useWatchTune());
    let first = 0;
    act(() => {
      first = result.current.flip({ firstTune: false, streaming: false });
    });
    act(() => {
      result.current.flip({ firstTune: false, streaming: false });
    });
    act(() => result.current.ready(first));
    expect(result.current.view.cover).toBe("cold");
  });
});
