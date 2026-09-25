import { afterEach, describe, expect, it, vi } from "vitest";
import {
  ClientDiagnosticsReporter,
  createAuthenticatedBatchSender,
  type SendBatch,
} from "./client-diagnostics";

const deferred = () => {
  let reject!: (error: unknown) => void;
  const promise = new Promise<void>((_resolve, onReject) => {
    reject = onReject;
  });
  return { promise, reject };
};

afterEach(() => vi.useRealTimers());

describe("ClientDiagnosticsReporter", () => {
  it("batches a generated-contract identity without blocking the caller", async () => {
    vi.useFakeTimers();
    const send = vi.fn<SendBatch>(async () => undefined);
    const reporter = new ClientDiagnosticsReporter(send, {
      clientVersion: "0.0.1",
      platform: "shield_tv",
      source: "android_tv",
    });

    reporter.record({
      channelId: "seven",
      event: "player.ready",
      playbackSessionId: "native-1",
      transport: "native_hls",
    });
    expect(send).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(2_000);

    const events = send.mock.calls[0]?.[0] ?? [];
    expect(reporter.wireBatch(events)).toMatchObject({
      clientVersion: "0.0.1",
      events: [{ channelId: "seven", event: "player.ready", occurredAt: expect.any(Number) }],
      platform: "shield_tv",
      source: "android_tv",
    });
  });

  it("retains errors ahead of routine events when its queue is saturated", async () => {
    const send = vi.fn<SendBatch>(async () => undefined);
    const reporter = new ClientDiagnosticsReporter(send);
    reporter.record({ event: "client.unhandled_error", errorClass: "error", surface: "root" });
    for (let index = 0; index < 100; index++) {
      reporter.record({
        channelId: `channel_${index}`,
        event: "player.attached",
        playbackSessionId: "native-1",
        transport: "native_hls",
      });
    }

    for (let index = 0; index < 5; index++) await reporter.flush();
    const sent = send.mock.calls.flatMap(([events]) => events);
    expect(sent).toHaveLength(100);
    expect(sent.some(({ event }) => event === "client.unhandled_error")).toBe(true);
    reporter.dispose();
  });

  it("restores a failed batch without overflowing when new observations fill the queue", async () => {
    const first = deferred();
    const send = vi
      .fn<SendBatch>()
      .mockImplementationOnce(() => first.promise)
      .mockResolvedValue(undefined);
    const reporter = new ClientDiagnosticsReporter(send);
    reporter.record({ event: "client.api_failed", httpStatus: 502, requestId: "request_1" });
    for (let index = 0; index < 19; index++) {
      reporter.record({
        channelId: `old_${index}`,
        event: "player.attached",
        playbackSessionId: "native-1",
        transport: "native_hls",
      });
    }

    const failedFlush = reporter.flush();
    for (let index = 0; index < 100; index++) {
      reporter.record({
        channelId: `new_${index}`,
        event: "player.attached",
        playbackSessionId: "native-1",
        transport: "native_hls",
      });
    }
    first.reject(new Error("offline"));
    await failedFlush;
    for (let index = 0; index < 5; index++) await reporter.flush();

    const retried = send.mock.calls.slice(1).flatMap(([events]) => events);
    expect(retried).toHaveLength(100);
    expect(retried.some(({ event }) => event === "client.api_failed")).toBe(true);
    reporter.dispose();
  });

  it("stops accepting or scheduling observations after disposal", async () => {
    vi.useFakeTimers();
    const send = vi.fn<SendBatch>(async () => undefined);
    const reporter = new ClientDiagnosticsReporter(send);
    reporter.dispose();
    reporter.record({ event: "client.unhandled_error", errorClass: "error", surface: "root" });

    await vi.runAllTimersAsync();
    expect(send).not.toHaveBeenCalled();
  });
});

describe("createAuthenticatedBatchSender", () => {
  it("posts the wire batch through the authenticated fetch and throws on refusal", async () => {
    const request = vi.fn<typeof fetch>(async () => new Response(null, { status: 204 }));
    const reporter = new ClientDiagnosticsReporter(async () => undefined, {
      clientVersion: "0.0.1",
      platform: "shield_tv",
      source: "android_tv",
    });
    const send = createAuthenticatedBatchSender(request, (events) => reporter.wireBatch(events));
    const events = [{ event: "player.ready" as const, occurredAt: 1 }];

    await send(events);
    const [url, init] = request.mock.calls[0] ?? [];
    expect(String(url)).toContain("/v1/diagnostics/client-events");
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toMatchObject({
      events,
      platform: "shield_tv",
      source: "android_tv",
    });

    request.mockResolvedValueOnce(new Response(null, { status: 429 }));
    await expect(send(events)).rejects.toThrow("429");
  });
});
