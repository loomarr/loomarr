import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { FLIP_MS, HOLD_MS, useTunePhase } from "./use-tune-phase";

describe("useTunePhase", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  const setup = (tuning: boolean, channelId = "a") =>
    renderHook((p: { channelId: string; tuning: boolean }) => useTunePhase(p.tuning, p.channelId), {
      initialProps: { channelId, tuning },
    });

  it("is idle while nothing is tuning", () => {
    expect(setup(false).result.current).toBe("idle");
  });

  it("is the quiet flip from the first moment of a wait, and never the signature inside 960 ms", () => {
    const { result, rerender } = setup(false);
    rerender({ channelId: "b", tuning: true });
    expect(result.current).toBe("flip");
    act(() => vi.advanceTimersByTime(FLIP_MS - 1));
    expect(result.current).toBe("flip");
    // The picture is ready: the signature never showed.
    rerender({ channelId: "b", tuning: false });
    expect(result.current).toBe("idle");
    act(() => vi.advanceTimersByTime(5_000));
    expect(result.current).toBe("idle");
  });

  it("earns the full signature at 960 ms and the hold at 1.5 s, and ends when the picture is ready", () => {
    const { result, rerender } = setup(true);
    act(() => vi.advanceTimersByTime(FLIP_MS));
    expect(result.current).toBe("signature");
    act(() => vi.advanceTimersByTime(HOLD_MS - FLIP_MS));
    expect(result.current).toBe("hold");
    rerender({ channelId: "a", tuning: false });
    expect(result.current).toBe("idle");
  });

  it("re-targets on a new channel while still waiting: surfing never climbs to the signature", () => {
    const { result, rerender } = setup(true, "a");
    act(() => vi.advanceTimersByTime(FLIP_MS - 100));
    rerender({ channelId: "b", tuning: true });
    act(() => vi.advanceTimersByTime(FLIP_MS - 100));
    expect(result.current).toBe("flip");
    act(() => vi.advanceTimersByTime(100));
    expect(result.current).toBe("signature");
  });
});
