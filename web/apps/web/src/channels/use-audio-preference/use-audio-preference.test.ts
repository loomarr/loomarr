import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { useAudioPreference } from "./use-audio-preference";

describe("useAudioPreference", () => {
  beforeEach(() => localStorage.clear());

  it("is the channel's default until the viewer picks a language, and remembers it", () => {
    const first = renderHook(() => useAudioPreference());
    expect(first.result.current[0]).toBe("default");
    act(() => first.result.current[1]("spa"));
    expect(first.result.current[0]).toBe("spa");
    expect(renderHook(() => useAudioPreference()).result.current[0]).toBe("spa");
  });

  it("remembers the default once the viewer goes back to it", () => {
    const hook = renderHook(() => useAudioPreference());
    act(() => hook.result.current[1]("spa"));
    act(() => hook.result.current[1]("default"));
    expect(renderHook(() => useAudioPreference()).result.current[0]).toBe("default");
  });

  it("reads anything that is not a language code as the default", () => {
    for (const stored of ["{not json", '"Spanish"', '{"language":"spa"}', "42", '""']) {
      localStorage.setItem("loomarr.player.audio", stored);
      expect(renderHook(() => useAudioPreference()).result.current[0], stored).toBe("default");
    }
  });
});
