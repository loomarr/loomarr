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
  type MasterAnswer = (init?: RequestInit) => Response | Promise<Response>;
  const pairedSource = (...masters: MasterAnswer[]) => {
    const request = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const channelId = /\/channels\/([^/]+)\/play-url$/.exec(String(input))?.[1];
      if (channelId) {
        return new Response(
          JSON.stringify({
            expiresAt: "2026-09-25T01:00:00Z",
            relativeUrl: `/v1/playout/hls/${channelId}/master.m3u8?sig=one`,
            serverTimeMs: Date.now() + 5_000,
            url: "",
          }),
          { status: 200 },
        );
      }
      const next = masters.length > 1 ? masters.shift() : masters[0];
      return (next ?? started)(init);
    });
    // The real tune's master reads (a warm's carry mode=warm).
    const masterRequests = (channelId = "simpsons") =>
      request.mock.calls.filter(([input]) => {
        const url = String(input);
        return url.includes(`/${channelId}/master.m3u8`) && !url.includes("mode=warm");
      });
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
        serverTimeMs: Date.now() + 5_000,
        uri: "https://loomarr.test/v1/playout/hls/simpsons/master.m3u8?sig=one",
      },
      expect.objectContaining({ attemptId: 1 }),
    );
  });

  it("counts the server's start wait in the clock the player starts from", async () => {
    const slowStart: MasterAnswer = () =>
      new Promise((resolve) => setTimeout(() => resolve(started()), 8_000));
    const { source } = pairedSource(slowStart);
    const { controller, transport } = harness(source);
    const mintedServerTime = Date.now() + 5_000;

    const tuning = controller.reconcile(channels);
    await vi.advanceTimersByTimeAsync(8_000);
    await tuning;

    // Without the wait counted, the live clock would run 8 s behind the server's for the whole tune.
    expect(transport.replace).toHaveBeenCalledWith(
      expect.objectContaining({ serverTimeMs: mintedServerTime + 8_000 }),
      expect.anything(),
    );
  });

  it("retries a start that failed without the server's reason, as the player's own errors are", async () => {
    const { masterRequests, source } = pairedSource(() => new Response("", { status: 404 }), started);
    const { controller, reports, transport } = harness(source);

    await controller.reconcile(channels);
    expect(controller.getSnapshot()).toMatchObject({ reconnecting: { attempt: 1 }, status: "tuning" });
    expect(reports).toEqual([expect.objectContaining({ attempt: 1, fatal: false })]);

    await vi.advanceTimersByTimeAsync(backoffMs[0] ?? 0);
    expect(masterRequests()).toHaveLength(2);
    expect(transport.replace).toHaveBeenCalledOnce();
  });

  it("abandons a pending start when the viewer surfs on, without a failure or a stale player", async () => {
    const pending: MasterAnswer = (init) =>
      new Promise((_, reject) =>
        init?.signal?.addEventListener("abort", () => reject(new DOMException("Aborted", "AbortError"))),
      );
    const { masterRequests, source } = pairedSource(pending, started);
    const two: PlayerChannel[] = [
      ...channels,
      { id: "futurama", inAppPlayable: true, name: "Futurama", number: 8 },
    ];
    const { controller, reports, transport } = harness(source);

    const first = controller.reconcile(two);
    await vi.advanceTimersByTimeAsync(0);
    await controller.step(1);
    await first;

    expect(masterRequests("simpsons")).toHaveLength(1);
    expect(controller.getSnapshot()).toMatchObject({ channel: { id: "futurama" } });
    expect(controller.getSnapshot().status).not.toBe("failed");
    expect(reports).toEqual([]);
    expect(transport.replace).toHaveBeenCalledOnce();
    expect(transport.replace).toHaveBeenCalledWith(
      expect.objectContaining({ uri: expect.stringContaining("/futurama/master.m3u8") }),
      expect.anything(),
    );
  });

  it("tunes a warmed neighbour without reading its master again", async () => {
    const { masterRequests, source } = pairedSource(started);
    const two: PlayerChannel[] = [
      ...channels,
      { id: "futurama", inAppPlayable: true, name: "Futurama", number: 8 },
    ];
    const { controller } = harness(source);
    await controller.reconcile(two);
    await vi.advanceTimersByTimeAsync(0); // the neighbour's warm settles

    await controller.step(1);

    expect(controller.getSnapshot()).toMatchObject({ channel: { id: "futurama" }, status: "tuning" });
    expect(masterRequests("futurama")).toHaveLength(0);
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
