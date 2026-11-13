import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useVolumeLevel } from "./use-volume-level";

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe("useVolumeLevel", () => {
  it("starts hidden", () => {
    const { result } = renderHook(() => useVolumeLevel());
    expect(result.current.volumeOpen).toBe(false);
    expect(result.current.volumeFlash).toBe(0);
  });

  it("flashes for a moment and then hides, each change restarting the moment", () => {
    const { result } = renderHook(() => useVolumeLevel());

    act(() => result.current.flashVolume());
    expect(result.current.volumeFlash).toBe(1);

    act(() => void vi.advanceTimersByTime(1_000));
    act(() => result.current.flashVolume());
    expect(result.current.volumeFlash).toBe(2);

    // 1.4 s after the second change: the first moment would have ended, the second has not.
    act(() => void vi.advanceTimersByTime(1_400));
    expect(result.current.volumeFlash).toBe(2);
    act(() => void vi.advanceTimersByTime(100));
    expect(result.current.volumeFlash).toBe(0);
  });

  it("closes the level and its flash together", () => {
    const { result } = renderHook(() => useVolumeLevel());
    act(() => {
      result.current.setVolumeOpen(true);
      result.current.flashVolume();
    });

    act(() => result.current.closeVolume());

    expect(result.current.volumeOpen).toBe(false);
    expect(result.current.volumeFlash).toBe(0);
  });
});
