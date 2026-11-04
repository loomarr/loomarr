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

  // A full-bleed player's frame is the whole window, so a pointer that never leaves it must still let
  // the picture go bare (Emby-style), and a move brings the controls back.
  it("hides after the idle window with the pointer resting on the frame, and a move reveals", () => {
    const { result } = renderHook(() => useAutoHideControls(true));
    act(() => result.current.onPointerActive());
    act(() => vi.advanceTimersByTime(2000));
    expect(result.current.controlsShown).toBe(true);
    act(() => vi.advanceTimersByTime(1000)); // past IDLE
    expect(result.current.controlsShown).toBe(false);

    act(() => result.current.onPointerActive());
    expect(result.current.controlsShown).toBe(true);
  });

  it("gives a menu closed under a resting pointer the idle window, not the short grace", () => {
    const { result } = renderHook(() => useAutoHideControls(true));
    act(() => result.current.onPointerActive());
    act(() => result.current.holdControls.hold(true));
    act(() => vi.advanceTimersByTime(5000));
    expect(result.current.controlsShown).toBe(true);

    act(() => result.current.holdControls.hold(false));
    act(() => vi.advanceTimersByTime(700));
    expect(result.current.controlsShown).toBe(true);
    act(() => vi.advanceTimersByTime(2200));
    expect(result.current.controlsShown).toBe(false);
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
