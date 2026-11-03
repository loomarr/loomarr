// @vitest-environment jsdom

import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useAutoHideControls, useFrameTap } from "../index";

// The package compiles without the DOM lib; jsdom supplies the document at run time.
const document = (
  globalThis as unknown as { document: { createElement(t: string): Parameters<typeof createRoot>[0] } }
).document;

// Renders a hook once and keeps its latest value, as testing-library's renderHook does.
const renderHook = <T>(hook: () => T) => {
  const result = { current: undefined as T };
  const Probe = () => {
    result.current = hook();
    return null;
  };
  act(() => createRoot(document.createElement("div")).render(createElement(Probe)));
  return { result };
};

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

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

  // The caller's own moment (a station-ID card) clears the controls at once; a move brings them back.
  // Held or paused controls stay: a player whose autoplay was refused keeps its Play.
  it("hides at the caller's word during playback, but not while held or paused", () => {
    const { result } = renderHook(() => useAutoHideControls(true));
    act(() => result.current.onPointerActive());
    act(() => result.current.hideControls());
    expect(result.current.controlsShown).toBe(false);
    act(() => result.current.onPointerActive());
    expect(result.current.controlsShown).toBe(true);

    act(() => result.current.holdControls.hold(true));
    act(() => result.current.hideControls());
    expect(result.current.controlsShown).toBe(true);

    const paused = renderHook(() => useAutoHideControls(false));
    act(() => paused.result.current.hideControls());
    expect(paused.result.current.controlsShown).toBe(true);
  });

  // The viewer's own word (a TV's Back) clears the picture paused too; nothing re-shows it until they ask.
  it("dismisses at the viewer's word, paused or playing, but not while held", () => {
    const paused = renderHook(() => useAutoHideControls(false));
    act(() => paused.result.current.dismissControls());
    expect(paused.result.current.controlsShown).toBe(false);
    act(() => vi.advanceTimersByTime(10_000));
    expect(paused.result.current.controlsShown).toBe(false);
    act(() => paused.result.current.revealControls());
    expect(paused.result.current.controlsShown).toBe(true);

    act(() => paused.result.current.holdControls.hold(true));
    act(() => paused.result.current.dismissControls());
    expect(paused.result.current.controlsShown).toBe(true);
  });

  // A TV remote or a phone has no pointer: a stall ending under controls the viewer opened gives them
  // the idle window, not the short grace meant for a pointer that has left.
  it("gives controls revealed without a pointer the idle window after a stall", () => {
    const result = { current: undefined as unknown as ReturnType<typeof useAutoHideControls> };
    const Probe = ({ playing }: { playing: boolean }) => {
      result.current = useAutoHideControls(playing);
      return null;
    };
    const root = createRoot(document.createElement("div"));
    act(() => root.render(createElement(Probe, { playing: true })));
    act(() => result.current.revealControls());
    act(() => root.render(createElement(Probe, { playing: false }))); // the stream stalls
    act(() => root.render(createElement(Probe, { playing: true }))); // and comes back
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

// Touch (#2088 decision 12): a tap is the controls', a double-tap on a side steps.
describe("useFrameTap", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  const tapper = (live: boolean, controlsShown: boolean) => {
    const calls = { hide: 0, reveal: 0, steps: [] as number[] };
    const { result } = renderHook(() =>
      useFrameTap({
        controlsShown,
        hideControls: () => (calls.hide += 1),
        live,
        onStep: (direction) => calls.steps.push(direction),
        revealControls: () => (calls.reveal += 1),
      }),
    );
    return { calls, tap: (across: number) => act(() => result.current(across)) };
  };

  it("toggles the controls once the double-tap window has passed", () => {
    const hidden = tapper(true, false);
    hidden.tap(0.5);
    expect(hidden.calls.reveal).toBe(0);
    act(() => vi.advanceTimersByTime(300));
    expect(hidden.calls.reveal).toBe(1);

    const shown = tapper(true, true);
    shown.tap(0.1);
    act(() => vi.advanceTimersByTime(300));
    expect(shown.calls.hide).toBe(1);
  });

  it("steps from a double-tap on the left or right third, without toggling", () => {
    const { calls, tap } = tapper(true, false);
    tap(0.1);
    tap(0.2);
    tap(0.9);
    tap(0.8);
    act(() => vi.advanceTimersByTime(1_000));
    expect(calls.steps).toEqual([-1, 1]);
    expect(calls.reveal).toBe(0);
  });

  it("never steps from the middle, across sides, too slowly, or off a live channel", () => {
    const middle = tapper(true, false);
    middle.tap(0.5);
    middle.tap(0.5);
    const across = tapper(true, false);
    across.tap(0.1);
    across.tap(0.9);
    const slow = tapper(true, false);
    slow.tap(0.1);
    act(() => vi.advanceTimersByTime(400));
    slow.tap(0.1);
    const notLive = tapper(false, false);
    notLive.tap(0.1);
    notLive.tap(0.1);
    act(() => vi.advanceTimersByTime(1_000));
    for (const { calls } of [middle, across, slow, notLive]) expect(calls.steps).toEqual([]);
  });
});
