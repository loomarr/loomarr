import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useElapsed } from "./use-elapsed";

describe("useElapsed", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("counts whole seconds while running", () => {
    const { result } = renderHook(() => useElapsed(true));
    expect(result.current).toBe(0);

    act(() => void vi.advanceTimersByTime(3000));
    expect(result.current).toBe(3);
  });

  it("stays at zero and starts no timer when idle", () => {
    const { result } = renderHook(() => useElapsed(false));
    act(() => void vi.advanceTimersByTime(5000));
    expect(result.current).toBe(0);
  });

  // The count is only meaningful relative to the run being watched, so a new run starts
  // from 0 rather than continuing the previous one's total.
  it("resets when a new run starts", () => {
    const { result, rerender } = renderHook(({ running }) => useElapsed(running), {
      initialProps: { running: true },
    });
    act(() => void vi.advanceTimersByTime(4000));
    expect(result.current).toBe(4);

    rerender({ running: false });
    expect(result.current).toBe(0);

    rerender({ running: true });
    act(() => void vi.advanceTimersByTime(1000));
    expect(result.current).toBe(1);
  });
});
