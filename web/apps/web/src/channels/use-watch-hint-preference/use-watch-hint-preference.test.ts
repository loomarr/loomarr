import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useWatchHintPreference } from "./use-watch-hint-preference";

afterEach(() => {
  localStorage.clear();
  vi.restoreAllMocks();
});

describe("useWatchHintPreference", () => {
  it("is unseen on a first visit, and seen on the next once remembered", () => {
    const first = renderHook(() => useWatchHintPreference());
    expect(first.result.current[0]).toBe(false);
    act(() => first.result.current[1]());
    // This visit keeps what it read; the next one reads it remembered.
    expect(first.result.current[0]).toBe(false);
    expect(renderHook(() => useWatchHintPreference()).result.current[0]).toBe(true);
  });

  it("reads a storage that throws as unseen, and remembering there does not throw", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    const { result } = renderHook(() => useWatchHintPreference());
    expect(result.current[0]).toBe(false);
    expect(() => result.current[1]()).not.toThrow();
  });
});
