import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useSubtitlesPreference } from "./use-subtitles-preference";

const KEY = "loomarr.player.subtitles";

describe("useSubtitlesPreference", () => {
  afterEach(() => {
    localStorage.clear();
    vi.restoreAllMocks();
  });

  it("defaults to Off", () => {
    expect(renderHook(() => useSubtitlesPreference()).result.current[0]).toBe("off");
  });

  it("remembers the viewer's track across mounts", () => {
    const first = renderHook(() => useSubtitlesPreference());
    act(() => first.result.current[1]("en-sdh"));
    expect(first.result.current[0]).toBe("en-sdh");
    expect(localStorage.getItem(KEY)).toBe("en-sdh");
    expect(renderHook(() => useSubtitlesPreference()).result.current[0]).toBe("en-sdh");
  });

  it("reads a stored value that is not a track key as Off", () => {
    localStorage.setItem(KEY, "English <b>");
    expect(renderHook(() => useSubtitlesPreference()).result.current[0]).toBe("off");
  });

  it("falls back to Off, and still holds a choice for the page, when storage throws", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    const { result } = renderHook(() => useSubtitlesPreference());
    expect(result.current[0]).toBe("off");
    act(() => result.current[1]("en"));
    expect(result.current[0]).toBe("en");
  });
});
