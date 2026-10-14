import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useSwitchSoundPreference } from "./use-switch-sound-preference";

const KEY = "loomarr.player.channel-change-sound";

describe("useSwitchSoundPreference", () => {
  afterEach(() => {
    localStorage.clear();
    vi.restoreAllMocks();
  });

  it("defaults to on", () => {
    const { result } = renderHook(() => useSwitchSoundPreference());
    expect(result.current[0]).toBe(true);
  });

  it("remembers the viewer's choice across mounts", () => {
    const first = renderHook(() => useSwitchSoundPreference());
    act(() => first.result.current[1](false));
    expect(first.result.current[0]).toBe(false);
    expect(localStorage.getItem(KEY)).toBe("off");
    expect(renderHook(() => useSwitchSoundPreference()).result.current[0]).toBe(false);
  });

  it("falls back to on, and still holds a choice for the page, when storage throws", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    const { result } = renderHook(() => useSwitchSoundPreference());
    expect(result.current[0]).toBe(true);
    act(() => result.current[1](false));
    expect(result.current[0]).toBe(false);
  });
});
