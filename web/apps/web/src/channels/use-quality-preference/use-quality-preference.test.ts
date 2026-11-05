import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { useQualityPreference } from "./use-quality-preference";

describe("useQualityPreference", () => {
  beforeEach(() => localStorage.clear());

  it("is Auto until the viewer picks a rung, and remembers the rung", () => {
    const first = renderHook(() => useQualityPreference());
    expect(first.result.current[0]).toBe("auto");
    act(() => first.result.current[1](1080));
    expect(first.result.current[0]).toBe(1080);

    expect(renderHook(() => useQualityPreference()).result.current[0]).toBe(1080);
  });

  it("reads anything that is not a height as Auto", () => {
    localStorage.setItem("loomarr.player.quality", "high");
    expect(renderHook(() => useQualityPreference()).result.current[0]).toBe("auto");
  });
});
