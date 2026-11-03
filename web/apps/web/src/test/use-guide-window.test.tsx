import { DEFAULT_WINDOW_MINUTES, useGuideWindow } from "@loomarr/core/guide";
import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// The guide window is part of the react-query key (`['/v1/guide', {from, to}]`). A key that
// changes every minute refetches the whole four-hour guide per open tab per minute, and a screen
// without placeholder data unmounts every section while it does. The hook pins the window and
// slides it only when the clock nears its end.

const MINUTE = 60_000;
const T0 = Date.UTC(2026, 10, 2, 20, 0, 0);

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(T0);
});
afterEach(() => vi.useRealTimers());

const advance = (ms: number) => act(() => vi.advanceTimersByTime(ms));

describe("useGuideWindow", () => {
  it("keeps the window key stable while the clock moves through the first half of it", () => {
    const { result } = renderHook(() => useGuideWindow());
    const first = result.current.window;
    const keys = new Set([JSON.stringify(first)]);

    for (let minute = 1; minute <= 120; minute++) {
      advance(MINUTE);
      keys.add(JSON.stringify(result.current.window));
    }

    expect(keys.size).toBe(1);
    expect(result.current.window).toEqual(first);
  });

  it("keeps ticking the live clock outside the window", () => {
    const { result } = renderHook(() => useGuideWindow());
    const before = result.current.nowMs;

    advance(5 * MINUTE);

    expect(result.current.nowMs).toBeGreaterThanOrEqual(before + 5 * MINUTE - 30_000);
    expect(result.current.nowMs).toBe(Date.now());
  });

  it("slides forward once now is within 30 minutes of the window end, then holds again", () => {
    const { result } = renderHook(() => useGuideWindow());
    const first = result.current.window;
    // The window starts 30 minutes before mount and spans the default length.
    expect(first.to - first.from).toBe(DEFAULT_WINDOW_MINUTES * MINUTE);

    // 31 minutes before the end: still outside the 30-minute margin, so still pinned.
    advance(first.to - 31 * MINUTE - Date.now());
    expect(result.current.window).toEqual(first);

    advance(2 * MINUTE);
    const slid = result.current.window;
    expect(slid.from).toBeGreaterThan(first.from);
    expect(slid.to - Date.now()).toBeGreaterThan(30 * MINUTE);
    // Re-pinned at the first tick inside the margin.
    expect(result.current.anchorMs).toBe(first.to - 30 * MINUTE);

    // Pinned again afterwards.
    advance(60 * MINUTE);
    expect(result.current.window).toEqual(slid);
  });

  it("slides sooner for a shorter span", () => {
    const { result } = renderHook(() => useGuideWindow(120));
    const first = result.current.window;

    advance(61 * MINUTE);

    expect(result.current.window.from).toBeGreaterThan(first.from);
  });
});
