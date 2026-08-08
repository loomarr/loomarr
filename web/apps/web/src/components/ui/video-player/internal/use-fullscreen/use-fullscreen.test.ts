import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useFullscreen } from "./use-fullscreen";

// jsdom implements no Fullscreen API, so the element methods and `document.fullscreenElement` are
// stubbed. What is OURS to test: the toggle calls request vs exit based on current state, and the
// `fullscreenchange` listener tracks the browser's own signal (so Esc-to-exit flips our state).
describe("useFullscreen", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    Object.defineProperty(document, "fullscreenElement", { value: null, configurable: true });
  });

  const wrapperRef = () => {
    const el = document.createElement("div");
    el.requestFullscreen = vi.fn().mockResolvedValue(undefined);
    return { current: el };
  };

  it("requests fullscreen on the wrapper when none is active", () => {
    Object.defineProperty(document, "fullscreenElement", { value: null, configurable: true });
    const ref = wrapperRef();
    const { result } = renderHook(() => useFullscreen(ref));

    act(() => result.current.toggleFullscreen());
    expect(ref.current.requestFullscreen).toHaveBeenCalledOnce();
  });

  it("exits fullscreen when one is active", () => {
    const ref = wrapperRef();
    Object.defineProperty(document, "fullscreenElement", { value: ref.current, configurable: true });
    const exit = vi.fn().mockResolvedValue(undefined);
    document.exitFullscreen = exit;
    const { result } = renderHook(() => useFullscreen(ref));

    act(() => result.current.toggleFullscreen());
    expect(exit).toHaveBeenCalledOnce();
    expect(ref.current.requestFullscreen).not.toHaveBeenCalled();
  });

  it("tracks the browser's fullscreenchange (Esc-to-exit flips state back)", () => {
    const ref = wrapperRef();
    const { result } = renderHook(() => useFullscreen(ref));
    expect(result.current.fullscreen).toBe(false);

    // Enter: the element becomes the fullscreen element, then the browser fires the event.
    Object.defineProperty(document, "fullscreenElement", { value: ref.current, configurable: true });
    act(() => document.dispatchEvent(new Event("fullscreenchange")));
    expect(result.current.fullscreen).toBe(true);

    // Exit (e.g. Esc): the element clears, the event fires again ⇒ back to false.
    Object.defineProperty(document, "fullscreenElement", { value: null, configurable: true });
    act(() => document.dispatchEvent(new Event("fullscreenchange")));
    expect(result.current.fullscreen).toBe(false);
  });
});
