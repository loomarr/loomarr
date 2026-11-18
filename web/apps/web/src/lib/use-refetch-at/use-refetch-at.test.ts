import { renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useRefetchAt } from "./use-refetch-at";

describe("useRefetchAt", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("reads again just after the deadline, not before", () => {
    const refetch = vi.fn();
    renderHook(() => useRefetchAt({ deadline: Date.now() + 60_000, answeredAt: 1, refetch }));

    vi.advanceTimersByTime(59_000);
    expect(refetch).not.toHaveBeenCalled();
    vi.advanceTimersByTime(3_000);
    expect(refetch).toHaveBeenCalledTimes(1);
  });

  it("waits for nothing without a deadline, an answer, or while disabled", () => {
    const refetch = vi.fn();
    renderHook(() => useRefetchAt({ deadline: undefined, answeredAt: 1, refetch }));
    renderHook(() => useRefetchAt({ deadline: Date.now() + 1_000, answeredAt: 0, refetch }));
    renderHook(() => useRefetchAt({ deadline: Date.now() + 1_000, answeredAt: 1, refetch, enabled: false }));

    vi.advanceTimersByTime(10 * 60_000);
    expect(refetch).not.toHaveBeenCalled();
  });

  // This clock runs ahead of the server's: each answer still names a deadline this clock has
  // passed. The read is tried again after a growing wait, rather than once and then never.
  it("keeps asking, ever less often, while the server says the deadline has not passed", () => {
    const refetch = vi.fn();
    const deadline = Date.now() - 1;
    const { rerender } = renderHook(({ answeredAt }) => useRefetchAt({ deadline, answeredAt, refetch }), {
      initialProps: { answeredAt: 1 },
    });

    vi.advanceTimersByTime(5_000);
    expect(refetch).toHaveBeenCalledTimes(1);
    rerender({ answeredAt: 2 });
    vi.advanceTimersByTime(9_000);
    expect(refetch).toHaveBeenCalledTimes(1);
    vi.advanceTimersByTime(1_000);
    expect(refetch).toHaveBeenCalledTimes(2);
  });
});
