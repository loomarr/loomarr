import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useRunFeedback } from "./use-run-feedback";

describe("useRunFeedback", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("tracks each key independently", () => {
    const { result } = renderHook(() => useRunFeedback());

    act(() => result.current.start("backup"));
    expect(result.current.isBusy("backup")).toBe(true);
    // A second job running is not this job running — the Tasks page shows one spinner per row.
    expect(result.current.isBusy("reconcile")).toBe(false);

    act(() => result.current.finish("backup"));
    expect(result.current.isBusy("backup")).toBe(false);
  });

  // The floor is the reason the hook exists. A job that finishes in under a frame would
  // otherwise flash a spinner so briefly it reads as a click that never registered.
  it("holds busy for the minimum visible time even when the work resolves at once", async () => {
    const { result } = renderHook(() => useRunFeedback(600));

    act(() => result.current.start("fast"));
    let settled: Promise<void>;
    act(() => {
      settled = result.current.settle("fast", Promise.resolve());
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(599);
    });
    expect(result.current.isBusy("fast")).toBe(true);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
      await settled;
    });
    expect(result.current.isBusy("fast")).toBe(false);
  });

  // ...and the floor is a MINIMUM, not a timeout: slow work must keep the control busy past it,
  // or the button would go idle while the refetch is still in flight and show stale rows.
  it("waits for slower work rather than releasing at the floor", async () => {
    const { result } = renderHook(() => useRunFeedback(100));

    let resolveWork: () => void = () => {};
    const work = new Promise<void>((r) => {
      resolveWork = r;
    });

    act(() => result.current.start("slow"));
    let settled: Promise<void>;
    act(() => {
      settled = result.current.settle("slow", work);
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
    });
    expect(result.current.isBusy("slow")).toBe(true);

    await act(async () => {
      resolveWork();
      await settled;
    });
    expect(result.current.isBusy("slow")).toBe(false);
  });

  // settle() is called with a refetch promise at every real call site, but the argument is
  // optional and the floor alone must still release the key.
  it("settles on the floor alone when given no work", async () => {
    const { result } = renderHook(() => useRunFeedback(50));

    act(() => result.current.start("bare"));
    let settled: Promise<void>;
    act(() => {
      settled = result.current.settle("bare");
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(50);
      await settled;
    });
    expect(result.current.isBusy("bare")).toBe(false);
  });
});
