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
    expect(
      send.mock.calls.flatMap((call) => call[0]).some((event) => event.event === "client.unhandled_error"),
    ).toBe(true);
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

describe("embedded clientDiagnostics singleton", () => {
  const unauthorized = () => Object.assign(new Error("Unauthorized"), { status: 401 });

  it("stays silent until the authenticated shell opens the gate, then stops after a 401", async () => {
    vi.resetModules();
    vi.useFakeTimers();
    const ingest = vi.fn(async (_batch: unknown, _init?: unknown) => undefined);
    vi.doMock("@loomarr/api/endpoints/diagnostics", () => ({ ingestClientDiagnostics: ingest }));
    const { clientDiagnostics } = await import("./client-reporter");
    const oneEvent = { event: "client.unhandled_error", surface: "root", errorClass: "error" } as const;

    clientDiagnostics.record(oneEvent);
    await vi.advanceTimersByTimeAsync(30_000);
    expect(ingest).not.toHaveBeenCalled();

    clientDiagnostics.setAuthenticated(true);
    await vi.advanceTimersByTimeAsync(2_000);
    expect(ingest).toHaveBeenCalledTimes(1);

    ingest.mockRejectedValue(unauthorized());
    clientDiagnostics.record(oneEvent);
    await vi.advanceTimersByTimeAsync(2_000);
    expect(ingest).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(60_000);
    expect(ingest).toHaveBeenCalledTimes(2);
    clientDiagnostics.dispose();
    vi.doUnmock("@loomarr/api/endpoints/diagnostics");
  });
});
