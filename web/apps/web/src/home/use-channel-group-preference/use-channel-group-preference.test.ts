import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CHANNEL_GROUP_KEY, useChannelGroupPreference } from "./use-channel-group-preference";

afterEach(() => {
  localStorage.clear();
  vi.restoreAllMocks();
});

describe("useChannelGroupPreference", () => {
  it("opens on All when nothing is remembered", () => {
    const { result } = renderHook(() => useChannelGroupPreference());
    expect(result.current[0]).toBe("all");
  });

  it("remembers the chosen group for the next visit", () => {
    const first = renderHook(() => useChannelGroupPreference());
    act(() => first.result.current[1]("decade"));
    expect(localStorage.getItem(CHANNEL_GROUP_KEY)).toBe("decade");
    expect(renderHook(() => useChannelGroupPreference()).result.current[0]).toBe("decade");
  });

  it("ignores a stored value that is not a group", () => {
    localStorage.setItem(CHANNEL_GROUP_KEY, "toString");
    expect(renderHook(() => useChannelGroupPreference()).result.current[0]).toBe("all");
  });

  it("opens on All when storage cannot be read, as in a private window", () => {
    localStorage.setItem(CHANNEL_GROUP_KEY, "decade");
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("denied");
    });
    expect(renderHook(() => useChannelGroupPreference()).result.current[0]).toBe("all");
  });

  it("still switches when storage refuses", () => {
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("full");
    });
    const { result } = renderHook(() => useChannelGroupPreference());
    act(() => result.current[1]("recent"));
    expect(result.current[0]).toBe("recent");
  });
});
