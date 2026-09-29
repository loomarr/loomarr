import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { usePhoneWidth } from "./use-phone-width";

// A matchMedia stand-in for `(min-width: 48rem)` whose answer the test can flip.
const stubMedia = (tablet: boolean) => {
  const listeners = new Set<() => void>();
  const list = {
    matches: tablet,
    addEventListener: (_: string, fn: () => void) => listeners.add(fn),
    removeEventListener: (_: string, fn: () => void) => listeners.delete(fn),
  };
  vi.stubGlobal("matchMedia", () => list);
  return {
    resize: (toTablet: boolean) => {
      list.matches = toTablet;
      for (const fn of listeners) fn();
    },
  };
};

describe("usePhoneWidth", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("is true below the tablet breakpoint and follows a resize across it", () => {
    const media = stubMedia(false);
    const { result } = renderHook(() => usePhoneWidth());
    expect(result.current).toBe(true);
    act(() => media.resize(true));
    expect(result.current).toBe(false);
  });

  it("treats a browser without matchMedia as wide", () => {
    vi.stubGlobal("matchMedia", undefined);
    expect(renderHook(() => usePhoneWidth()).result.current).toBe(false);
  });
});
