import { describe, expect, it, vi } from "vitest";

vi.mock("expo-video", () => ({
  createVideoPlayer: vi.fn(),
  VideoView: vi.fn(),
}));

const { createNativePlaybackDiagnostics, nativeTransportOf } = await import("@loomarr/player/native");

describe("native transport label", () => {
  it("names AVPlayer's HLS on Apple platforms and Media3 on Android platforms", () => {
    expect(nativeTransportOf("ios")).toBe("native_hls");
    expect(nativeTransportOf("apple_tv")).toBe("native_hls");
    expect(nativeTransportOf("android")).toBe("media3");
    expect(nativeTransportOf("android_tv")).toBe("media3");
    expect(nativeTransportOf("shield_tv")).toBe("media3");
  });

  it("stamps every observation with the transport it was given", () => {
    const reporter = { flush: vi.fn(async () => undefined), record: vi.fn() };
    const diagnostics = createNativePlaybackDiagnostics(reporter, "native-1", "media3");
    diagnostics.channelChanged("seven");
    diagnostics.transportEvent({ attemptId: 1, type: "first-frame" });
    diagnostics.playerError({ attempt: 1, channelId: "seven", elapsedMs: 0, error: "x", fatal: true });
    diagnostics.dispose();
    const transports = reporter.record.mock.calls.map(([event]) => event.transport);
    expect(transports).toEqual(["media3", "media3", "media3", "media3"]);
  });
});

describe("native playback diagnostics", () => {
  it("records the closed lifecycle vocabulary without retaining native error prose", () => {
    const reporter = { flush: vi.fn(async () => undefined), record: vi.fn() };
    const diagnostics = createNativePlaybackDiagnostics(reporter, "native-1", "native_hls");

    diagnostics.transportEvent({ attemptId: 1, type: "first-frame" });
    diagnostics.channelChanged(undefined);
    diagnostics.channelChanged("seven");
    diagnostics.channelChanged("seven");
    diagnostics.transportEvent({ attemptId: 1, type: "first-frame" });
    diagnostics.channelChanged("eight");
    diagnostics.playerError({
      attempt: 2,
      channelId: "eight",
      elapsedMs: 41_400,
      error: "secret decoder detail",
      fatal: false,
    });
    diagnostics.dispose();
    diagnostics.dispose();

    expect(reporter.record.mock.calls.map(([event]) => event)).toEqual([
      {
        channelId: "seven",
        event: "player.attached",
        playbackSessionId: "native-1",
        reason: "mount",
        transport: "native_hls",
      },
      {
        channelId: "seven",
        event: "player.ready",
        playbackSessionId: "native-1",
        transport: "native_hls",
      },
      {
        channelId: "eight",
        event: "player.source_replaced",
        playbackSessionId: "native-1",
        previousChannelId: "seven",
        reason: "channel_change",
        transport: "native_hls",
      },
      {
        channelId: "eight",
        errorCode: "decoder.a2.t41s",
        event: "player.media_error",
        fatal: false,
        playbackSessionId: "native-1",
        transport: "native_hls",
      },
      {
        channelId: "eight",
        event: "player.detached",
        playbackSessionId: "native-1",
        reason: "unmount",
        transport: "native_hls",
      },
    ]);
    expect(JSON.stringify(reporter.record.mock.calls)).not.toContain("secret decoder detail");
    expect(reporter.flush).toHaveBeenCalledOnce();
  });

  it("codes a refused start by its HTTP status, not the server's sentence", () => {
    const reporter = { flush: vi.fn(async () => undefined), record: vi.fn() };
    const diagnostics = createNativePlaybackDiagnostics(reporter, "native-1", "native_hls");

    diagnostics.playerError({
      attempt: 1,
      channelId: "seven",
      elapsedMs: 2_200,
      error: "Loomarr couldn't start this channel's in-app stream.",
      fatal: true,
      status: 502,
    });

    expect(reporter.record).toHaveBeenCalledWith(
      expect.objectContaining({ errorCode: "http_502.a1.t2s", fatal: true }),
    );
  });

  it("rejects an unbounded client-authored playback session identity", () => {
    const reporter = { flush: vi.fn(async () => undefined), record: vi.fn() };
    expect(() => createNativePlaybackDiagnostics(reporter, "", "native_hls")).toThrow("1-128 characters");
    expect(() => createNativePlaybackDiagnostics(reporter, "x".repeat(129), "native_hls")).toThrow(
      "1-128 characters",
    );
  });
});
