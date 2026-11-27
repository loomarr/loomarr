import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useStationIdent } from "./use-station-ident";

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe("useStationIdent", () => {
  it("shows the card for five seconds once a tune settles, then takes it away", () => {
    const { result, rerender } = renderHook(({ settled }) => useStationIdent(settled), {
      initialProps: { settled: false },
    });
    expect(result.current).toBe(false);

    rerender({ settled: true });
    expect(result.current).toBe(true);

    act(() => void vi.advanceTimersByTime(4_999));
    expect(result.current).toBe(true);
    act(() => void vi.advanceTimersByTime(1));
    expect(result.current).toBe(false);
  });

  it("drops the card at once on a retune and shows it afresh when the next one settles", () => {
    const { result, rerender } = renderHook(({ settled }) => useStationIdent(settled), {
      initialProps: { settled: true },
    });
    expect(result.current).toBe(true);

    rerender({ settled: false });
    expect(result.current).toBe(false);

    rerender({ settled: true });
    expect(result.current).toBe(true);
  });
});
