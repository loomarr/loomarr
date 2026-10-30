import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useCoarsePointer } from "./use-coarse-pointer";

// A matchMedia stand-in for `(pointer: coarse)` whose answer the test can flip.
const stubMedia = (coarse: boolean) => {
  const listeners = new Set<() => void>();
  const list = {
    matches: coarse,
    addEventListener: (_: string, fn: () => void) => listeners.add(fn),
    removeEventListener: (_: string, fn: () => void) => listeners.delete(fn),
  };
  vi.stubGlobal("matchMedia", () => list);
  return {
    become: (next: boolean) => {
      list.matches = next;
      for (const fn of listeners) fn();
    },
  };
};

describe("useCoarsePointer", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("is true for a finger and follows the primary pointer as it changes", () => {
    const media = stubMedia(true);
    const { result } = renderHook(() => useCoarsePointer());
    expect(result.current).toBe(true);
    act(() => media.become(false));
    expect(result.current).toBe(false);
  });

  it("is false without matchMedia, so jsdom gets the pointer form", () => {
    vi.stubGlobal("matchMedia", undefined);
    const { result } = renderHook(() => useCoarsePointer());
    expect(result.current).toBe(false);
  });
});
