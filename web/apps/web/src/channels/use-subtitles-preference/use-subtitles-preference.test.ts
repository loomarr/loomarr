import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { useSubtitlesPreference } from "./use-subtitles-preference";

describe("useSubtitlesPreference", () => {
  beforeEach(() => localStorage.clear());

  it("is off until the viewer picks a track, and remembers the track", () => {
    const first = renderHook(() => useSubtitlesPreference());
    expect(first.result.current[0]).toBe("off");
    act(() => first.result.current[1]({ language: "eng", title: "SDH" }));
    expect(first.result.current[0]).toEqual({ language: "eng", title: "SDH" });

    expect(renderHook(() => useSubtitlesPreference()).result.current[0]).toEqual({
      language: "eng",
      title: "SDH",
    });
  });

  it("remembers off once the viewer turns them off again", () => {
    const hook = renderHook(() => useSubtitlesPreference());
    act(() => hook.result.current[1]({ language: "eng", title: "" }));
    act(() => hook.result.current[1]("off"));
    expect(renderHook(() => useSubtitlesPreference()).result.current[0]).toBe("off");
  });

  it("reads anything that is not a track as off", () => {
    for (const stored of [
      "{not json",
      '"eng"',
      '{"language":"eng"}',
      '{"language":"en","name":"English"}',
      "42",
    ]) {
      localStorage.setItem("loomarr.player.subtitles", stored);
      expect(renderHook(() => useSubtitlesPreference()).result.current[0], stored).toBe("off");
    }
  });
});
