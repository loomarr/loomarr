import { afterEach, describe, expect, it, vi } from "vitest";
import { ClientDiagnosticsReporter, type SendBatch } from "./client-diagnostics";

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

  it("retains errors ahead of routine events and restores a failed batch", async () => {
    const send = vi.fn<SendBatch>().mockRejectedValueOnce(new Error("offline")).mockResolvedValue(undefined);
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

    await reporter.flush();
    await reporter.flush();
    expect(send).toHaveBeenCalledTimes(2);
    expect(send.mock.calls[1]?.[0].some(({ event }) => event === "client.unhandled_error")).toBe(true);
    reporter.dispose();
  });
});
