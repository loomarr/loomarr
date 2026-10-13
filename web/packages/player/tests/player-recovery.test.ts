import {
  createPlayerController,
  type PlayerChannel,
  type PlayerErrorReport,
  type PlayerSourcePort,
  type PlayerTransport,
  type PlayerTransportEvent,
} from "@loomarr/player";
import { createPlayUrlSourcePort } from "@loomarr/player/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const channels: PlayerChannel[] = [{ id: "simpsons", inAppPlayable: true, name: "Simpsons", number: 7 }];
const backoffMs = [1_000, 3_000, 7_000];

const mintedSource = (): PlayerSourcePort => ({
  mint: vi.fn((channel) => Promise.resolve({ uri: `https://loomarr.test/${channel.id}.m3u8` })),
});

const harness = (source: PlayerSourcePort = mintedSource()) => {
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

// AVPlayer reports a refused manifest only as "resource unavailable" (Apple TV audit #5), so the
// paired source reads the server's answer itself; these drive the real source over a fake server.
describe("a start the paired server refuses (#1455)", () => {
  const reason = "Loomarr couldn't start this channel's in-app stream.";
  const master = "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=4000000\nbaseline/index.m3u8?sig=one\n";
  const refused = () =>
    new Response(JSON.stringify({ detail: reason, status: 502, title: "Couldn't start the channel" }), {
      headers: { "Content-Type": "application/problem+json" },
      status: 502,
    });
  const started = () => new Response(master, { status: 200 });
  const pairedSource = (...masters: (() => Response)[]) => {
    const request = vi.fn((input: RequestInfo | URL) => {
      if (String(input).endsWith("/play-url")) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              expiresAt: "2026-09-25T01:00:00Z",
              relativeUrl: "/v1/playout/hls/simpsons/master.m3u8?sig=one",
              url: "",
            }),
            { status: 200 },
          ),
        );
      }
      const next = masters.length > 1 ? masters.shift() : masters[0];
      return Promise.resolve((next ?? started)());
    });
    const masterRequests = () =>
      request.mock.calls.filter(([input]) => String(input).includes("master.m3u8"));
    return {
      masterRequests,
      source: createPlayUrlSourcePort({ baseUrl: "https://loomarr.test", fetch: request }),
    };
  };

  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-09-25T00:00:00Z"));
  });
  afterEach(() => vi.useRealTimers());

  it("shows the server's reason with Retry on the first refusal and never retries it alone", async () => {
    const { masterRequests, source } = pairedSource(refused);
    const { controller, reports, transport } = harness(source);

    await controller.reconcile(channels);

    expect(controller.getSnapshot()).toMatchObject({ error: reason, status: "failed" });
    expect(controller.getSnapshot().reconnecting).toBeUndefined();
    expect(transport.replace).not.toHaveBeenCalled();
    expect(reports).toEqual([
      expect.objectContaining({ attempt: 1, error: reason, fatal: true, status: 502 }),
    ]);
    await vi.advanceTimersByTimeAsync(60_000);
    expect(masterRequests()).toHaveLength(1);
  });

  it("opens the master's only variant, so the player never repeats the tune", async () => {
    const { masterRequests, source } = pairedSource(started);
    const { controller, transport } = harness(source);

    await controller.reconcile(channels);

    expect(masterRequests()).toHaveLength(1);
    expect(transport.replace).toHaveBeenCalledWith(
      {
        expiresAt: Date.parse("2026-09-25T01:00:00Z"),
        mediaUri: "https://loomarr.test/v1/playout/hls/simpsons/baseline/index.m3u8?sig=one",
        uri: "https://loomarr.test/v1/playout/hls/simpsons/master.m3u8?sig=one",
      },
      expect.objectContaining({ attemptId: 1 }),
    );
  });

  it("ends an automatic recovery at the refusal instead of spending the rest of the budget", async () => {
    const { masterRequests, source } = pairedSource(started, refused);
    const { controller, currentAttempt, emit, reports } = harness(source);
    await controller.reconcile(channels);
    emit({ attemptId: currentAttempt(), type: "first-frame" });

    emit({
      attemptId: currentAttempt(),
      error: "Failed to load the player item: resource unavailable",
      type: "error",
    });
    expect(controller.getSnapshot()).toMatchObject({ reconnecting: { attempt: 1 }, status: "tuning" });
    await vi.advanceTimersByTimeAsync(backoffMs[0] ?? 0);

    expect(controller.getSnapshot()).toMatchObject({ error: reason, status: "failed" });
    expect(reports.map(({ attempt, fatal, status }) => [attempt, fatal, status])).toEqual([
      [1, false, undefined],
      [2, true, 502],
    ]);
    await vi.advanceTimersByTimeAsync(60_000);
    expect(masterRequests()).toHaveLength(2);
  });

  it("plays again from Retry once the server can start the channel", async () => {
    const { source } = pairedSource(refused, started);
    const { controller, currentAttempt, emit } = harness(source);
    await controller.reconcile(channels);
    expect(controller.getSnapshot().status).toBe("failed");

    await controller.retry();
    emit({ attemptId: currentAttempt(), type: "first-frame" });

    expect(controller.getSnapshot()).toMatchObject({ status: "playing" });
    expect(controller.getSnapshot().error).toBeUndefined();
  });

  it("says the server can't be reached instead of a raw network error", async () => {
    const request = vi.fn().mockRejectedValue(new TypeError("Network request failed"));
    const { controller } = harness(
      createPlayUrlSourcePort({ baseUrl: "https://loomarr.test", fetch: request }),
    );

    await controller.reconcile(channels);

    expect(controller.getSnapshot()).toMatchObject({
      error: "Couldn't reach Loomarr. Check your connection and try again.",
      status: "failed",
    });
  });
});
