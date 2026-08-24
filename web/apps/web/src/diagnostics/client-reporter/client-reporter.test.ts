import { afterEach, describe, expect, it, vi } from "vitest";
import type { SendBatch } from "./client-reporter";
import { ClientDiagnosticsReporter } from "./client-reporter";

afterEach(() => vi.useRealTimers());

describe("ClientDiagnosticsReporter", () => {
  it("batches without blocking the caller", async () => {
    vi.useFakeTimers();
    const send = vi.fn<SendBatch>(async () => undefined);
    const reporter = new ClientDiagnosticsReporter(send);

    reporter.record({ event: "client.unhandled_error", surface: "root", errorClass: "error" });
    expect(send).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(2_000);

    expect(send).toHaveBeenCalledTimes(1);
    expect(send.mock.calls[0]?.[0]).toMatchObject([{ event: "client.unhandled_error", surface: "root" }]);
  });

  it("keeps errors when the bounded queue must drop a routine event", async () => {
    const send = vi.fn<SendBatch>(async () => undefined);
    const reporter = new ClientDiagnosticsReporter(send);
    reporter.record({ event: "client.unhandled_error", surface: "root", errorClass: "error" });
    for (let i = 0; i < 100; i++) {
      reporter.record({
        event: "player.attached",
        playbackSessionId: "session_1",
        channelId: `channel_${i}`,
        transport: "hls_js",
      });
    }

    for (let i = 0; i < 5; i++) await reporter.flush();
    expect(send.mock.calls.flatMap((call) => call[0]).some((event) => event.event === "client.unhandled_error")).toBe(true);
    expect(send.mock.calls.flatMap((call) => call[0])).toHaveLength(100);
  });

  it("restores a failed batch for a later retry", async () => {
    const send = vi.fn<SendBatch>().mockRejectedValueOnce(new Error("offline")).mockResolvedValue(undefined);
    const reporter = new ClientDiagnosticsReporter(send);
    reporter.record({ event: "client.api_failed", requestId: "request_1", httpStatus: 502 });

    await reporter.flush();
    await reporter.flush();
    expect(send).toHaveBeenCalledTimes(2);
  });
});
