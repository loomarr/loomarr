import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useAutoHideControls } from "./use-auto-hide-controls";

// The overlay auto-hide, tested at the hook boundary: `playing` in, `controlsShown` + handlers out.
// Timers are faked so the IDLE (mouse present) and GRACE (mouse gone) windows are deterministic.
describe("useAutoHideControls", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("keeps controls shown while paused, ignoring any timer", () => {
    const { result } = renderHook(() => useAutoHideControls(false));
    act(() => result.current.onPointerActive());
    act(() => vi.advanceTimersByTime(5000));
    expect(result.current.controlsShown).toBe(true);
  });

  it("keeps controls shown while the pointer rests on the frame during playback", () => {
    const { result } = renderHook(() => useAutoHideControls(true));
    act(() => result.current.onPointerActive());
    // Idle window elapses, but the pointer is still here ⇒ stays shown.
    act(() => vi.advanceTimersByTime(3000));
    expect(result.current.controlsShown).toBe(true);
  });

  it("hides shortly after the pointer leaves during playback", () => {
    const { result } = renderHook(() => useAutoHideControls(true));
    act(() => result.current.onPointerActive());
    expect(result.current.controlsShown).toBe(true);

    act(() => result.current.onPointerLeave());
    act(() => vi.advanceTimersByTime(700)); // past GRACE
    expect(result.current.controlsShown).toBe(false);
  });

  it("keeps controls shown while a bar control holds, even with the pointer gone", () => {
    const { result } = renderHook(() => useAutoHideControls(true));
    act(() => result.current.onPointerActive());
    act(() => result.current.onPointerLeave());
    act(() => result.current.holdControls.hold(true)); // e.g. a menu opened
    act(() => vi.advanceTimersByTime(700));
    expect(result.current.controlsShown).toBe(true);
  });

  it("re-hides once the hold clears while the pointer is still gone (menu closed)", () => {
    const { result } = renderHook(() => useAutoHideControls(true));
    act(() => result.current.onPointerActive());
    act(() => result.current.onPointerLeave());
    act(() => result.current.holdControls.hold(true));
    act(() => vi.advanceTimersByTime(700));
    expect(result.current.controlsShown).toBe(true);

    act(() => result.current.holdControls.hold(false));
    act(() => vi.advanceTimersByTime(700));
    expect(result.current.controlsShown).toBe(false);
  });

  it("counts nested holds — one release does not unhold two opens", () => {
    const { result } = renderHook(() => useAutoHideControls(true));
    act(() => result.current.onPointerLeave());
    act(() => result.current.holdControls.hold(true));
    act(() => result.current.holdControls.hold(true));
    act(() => result.current.holdControls.hold(false)); // still one hold outstanding
    act(() => vi.advanceTimersByTime(700));
    expect(result.current.controlsShown).toBe(true);
  });
});
