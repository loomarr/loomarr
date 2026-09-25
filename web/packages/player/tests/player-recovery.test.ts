import {
  createPlayerController,
  type PlayerChannel,
  type PlayerErrorReport,
  type PlayerSourcePort,
  type PlayerTransport,
  type PlayerTransportEvent,
} from "@loomarr/player";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const channels: PlayerChannel[] = [{ id: "simpsons", inAppPlayable: true, name: "Simpsons", number: 7 }];
const backoffMs = [1_000, 3_000, 7_000];

const harness = () => {
  const listeners = new Set<(event: PlayerTransportEvent) => void>();
  const transport: PlayerTransport = {
    dispose: vi.fn(),
    goLive: vi.fn(),
    pause: vi.fn(),
    play: vi.fn(),
    replace: vi.fn().mockResolvedValue(undefined),
    subscribe: vi.fn((listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    }),
  };
  const source: PlayerSourcePort = {
    mint: vi.fn((channel) => Promise.resolve({ uri: `https://loomarr.test/${channel.id}.m3u8` })),
  };
  const reports: PlayerErrorReport[] = [];
  const controller = createPlayerController({
    onPlayerError: (report) => reports.push(report),
    profile: {},
    recovery: { backoffMs },
    source,
    transport,
  });
  const emit = (event: PlayerTransportEvent) => {
    for (const listener of listeners) listener(event);
  };
  const currentAttempt = () => controller.getSnapshot().attemptId ?? 0;
  return { controller, currentAttempt, emit, reports, source, transport };
};

describe("player automatic recovery", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-09-25T00:00:00Z"));
  });
  afterEach(() => vi.useRealTimers());

  it("re-mints and re-tunes the same channel after backoff instead of publishing failed", async () => {
    const { controller, currentAttempt, emit, source, transport } = harness();
    await controller.reconcile(channels);
    emit({ attemptId: currentAttempt(), type: "first-frame" });
    expect(controller.getSnapshot().status).toBe("playing");

    emit({ attemptId: currentAttempt(), error: "BehindLiveWindow", type: "error" });
    expect(controller.getSnapshot()).toMatchObject({
      reconnecting: { attempt: 1, maxAttempts: 3 },
      status: "tuning",
    });
    expect(controller.getSnapshot().error).toBeUndefined();
    expect(source.mint).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(999);
    expect(source.mint).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(source.mint).toHaveBeenCalledTimes(2);
    expect(transport.replace).toHaveBeenCalledTimes(2);
    expect(controller.getSnapshot()).toMatchObject({ channel: { id: "simpsons" }, tuneReason: "retry" });
  });

  it("returns to playing with no manual action once a retry succeeds", async () => {
    const { controller, currentAttempt, emit } = harness();
    await controller.reconcile(channels);
    emit({ attemptId: currentAttempt(), error: "boom", type: "error" });
    await vi.advanceTimersByTimeAsync(1_000);
    emit({ attemptId: currentAttempt(), type: "first-frame" });

    const snapshot = controller.getSnapshot();
    expect(snapshot.status).toBe("playing");
    expect(snapshot.reconnecting).toBeUndefined();
    expect(snapshot.error).toBeUndefined();
  });

  it("backs off exponentially and shows manual Retry only after N consecutive failures", async () => {
    const { controller, currentAttempt, emit, source } = harness();
    await controller.reconcile(channels);
    const statuses: string[] = [];

    for (const wait of backoffMs) {
      emit({ attemptId: currentAttempt(), error: "boom", type: "error" });
      statuses.push(controller.getSnapshot().status);
      await vi.advanceTimersByTimeAsync(wait - 1);
      expect(source.mint).toHaveBeenCalledTimes(statuses.length);
      await vi.advanceTimersByTimeAsync(1);
    }
    expect(statuses).toEqual(["tuning", "tuning", "tuning"]);
    expect(source.mint).toHaveBeenCalledTimes(4);

    emit({ attemptId: currentAttempt(), error: "final boom", type: "error" });
    expect(controller.getSnapshot()).toMatchObject({ error: "final boom", status: "failed" });
    expect(controller.getSnapshot().reconnecting).toBeUndefined();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(source.mint).toHaveBeenCalledTimes(4);
  });

  it("a success resets the failure budget", async () => {
    const { controller, currentAttempt, emit } = harness();
    await controller.reconcile(channels);
    for (let round = 0; round < 3; round += 1) {
      emit({ attemptId: currentAttempt(), error: "boom", type: "error" });
      await vi.advanceTimersByTimeAsync(7_000);
      emit({ attemptId: currentAttempt(), type: "first-frame" });
    }
    emit({ attemptId: currentAttempt(), error: "boom", type: "error" });
    expect(controller.getSnapshot()).toMatchObject({ reconnecting: { attempt: 1 }, status: "tuning" });
  });

  it("emits exactly one diagnostics report per error with channel, attempt and elapsed since tune", async () => {
    const { controller, currentAttempt, emit, reports } = harness();
    await controller.reconcile(channels);
    await vi.advanceTimersByTimeAsync(4_000);
    emit({ attemptId: currentAttempt(), error: "BehindLiveWindow", type: "error" });
    await vi.advanceTimersByTimeAsync(1_000);
    await vi.advanceTimersByTimeAsync(2_000);
    emit({ attemptId: currentAttempt(), error: "HTTP 502", type: "error" });

    expect(reports).toEqual([
      { attempt: 1, channelId: "simpsons", elapsedMs: 4_000, error: "BehindLiveWindow", fatal: false },
      { attempt: 2, channelId: "simpsons", elapsedMs: 7_000, error: "HTTP 502", fatal: false },
    ]);
  });

  it("reports the exhausting error as fatal", async () => {
    const { controller, currentAttempt, emit, reports } = harness();
    await controller.reconcile(channels);
    for (const wait of backoffMs) {
      emit({ attemptId: currentAttempt(), error: "boom", type: "error" });
      await vi.advanceTimersByTimeAsync(wait);
    }
    emit({ attemptId: currentAttempt(), error: "boom", type: "error" });
    expect(reports.map(({ attempt, fatal }) => [attempt, fatal])).toEqual([
      [1, false],
      [2, false],
      [3, false],
      [4, true],
    ]);
  });

  it("a user tune cancels a pending automatic retry", async () => {
    const { controller, currentAttempt, emit, source } = harness();
    await controller.reconcile(channels);
    emit({ attemptId: currentAttempt(), error: "boom", type: "error" });
    await controller.retry();
    expect(source.mint).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(10_000);
    expect(source.mint).toHaveBeenCalledTimes(2);
  });

  it("dispose cancels a pending automatic retry", async () => {
    const { controller, currentAttempt, emit, source } = harness();
    await controller.reconcile(channels);
    emit({ attemptId: currentAttempt(), error: "boom", type: "error" });
    controller.dispose();
    await vi.advanceTimersByTimeAsync(10_000);
    expect(source.mint).toHaveBeenCalledTimes(1);
  });
});
