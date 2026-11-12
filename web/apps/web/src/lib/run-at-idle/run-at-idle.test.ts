import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { runAtIdle } from "./run-at-idle";

describe("runAtIdle", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("starts nothing until the browser is idle, then one task at a time", async () => {
    const callbacks: (() => void)[] = [];
    vi.stubGlobal("requestIdleCallback", (run: () => void) => callbacks.push(run));
    vi.stubGlobal("cancelIdleCallback", vi.fn());
    const first = vi.fn(() => Promise.resolve());
    const second = vi.fn(() => Promise.resolve());

    runAtIdle([first, second]);
    expect(first).not.toHaveBeenCalled();

    callbacks.shift()?.();
    expect(first).toHaveBeenCalledOnce();
    expect(second).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(0);
    callbacks.shift()?.();
    expect(second).toHaveBeenCalledOnce();
  });

  it("falls back to a timer where there is no requestIdleCallback", async () => {
    vi.stubGlobal("requestIdleCallback", undefined);
    const task = vi.fn(() => Promise.resolve());

    runAtIdle([task]);
    expect(task).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(200);
    expect(task).toHaveBeenCalledOnce();
  });

  it("goes on past a task that fails", async () => {
    vi.stubGlobal("requestIdleCallback", undefined);
    const after = vi.fn(() => Promise.resolve());

    runAtIdle([() => Promise.reject(new Error("offline")), after]);
    await vi.advanceTimersByTimeAsync(400);

    expect(after).toHaveBeenCalledOnce();
  });

  it("stops what has not started when cancelled", async () => {
    vi.stubGlobal("requestIdleCallback", undefined);
    const task = vi.fn(() => Promise.resolve());

    const stop = runAtIdle([task]);
    stop();
    await vi.advanceTimersByTimeAsync(1_000);

    expect(task).not.toHaveBeenCalled();
  });
});
