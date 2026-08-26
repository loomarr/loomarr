import type { VideoPlayer } from "expo-video";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("expo-video", () => ({
  createVideoPlayer: vi.fn(),
  VideoView: vi.fn(),
}));

const { createNativePlayerTransport, pairedNativeImageSource } = await import("@loomarr/player/native");

type PlayerListener = (payload: never) => void;

const nativePlayer = () => {
  const listeners = new Map<string, PlayerListener>();
  const removals: ReturnType<typeof vi.fn>[] = [];
  const player = {
    addListener: vi.fn((event: string, listener: PlayerListener) => {
      listeners.set(event, listener);
      const remove = vi.fn(() => listeners.delete(event));
      removals.push(remove);
      return { remove };
    }),
    currentLiveTimestamp: null as number | null,
    currentOffsetFromLive: null as number | null,
    currentTime: 0,
    duration: 0,
    loop: true,
    pause: vi.fn(),
    play: vi.fn(),
    release: vi.fn(),
    replaceAsync: vi.fn().mockResolvedValue(undefined),
    seekBy: vi.fn(),
    showNowPlayingNotification: true,
    staysActiveInBackground: true,
    timeUpdateEventInterval: 0,
  };
  return {
    emit: (event: string, payload: unknown) => listeners.get(event)?.(payload as never),
    player: player as unknown as VideoPlayer,
    raw: player,
    removals,
  };
};

describe("Expo video transport", () => {
  beforeEach(() => vi.clearAllMocks());
  afterEach(() => vi.useRealTimers());

  it("forces foreground-only playback and emits current native state", async () => {
    const { emit, player, raw } = nativePlayer();
    const transport = createNativePlayerTransport(player);
    const events = vi.fn();
    transport.subscribe(events);
    const signal = new AbortController().signal;

    await transport.replace({ uri: "https://loomarr.test/live.m3u8" }, { attemptId: 7, signal });
    transport.firstFrame();
    emit("playingChange", { isPlaying: true });
    emit("statusChange", { error: { message: "decoder failed" }, status: "error" });

    expect(raw.loop).toBe(false);
    expect(raw.showNowPlayingNotification).toBe(false);
    expect(raw.staysActiveInBackground).toBe(false);
    expect(raw.timeUpdateEventInterval).toBe(0.25);
    expect(raw.replaceAsync).toHaveBeenCalledWith({
      contentType: "hls",
      headers: undefined,
      uri: "https://loomarr.test/live.m3u8",
      useCaching: false,
    });
    expect(events.mock.calls.map(([event]) => event.type)).toEqual([
      "first-frame",
      "playing",
      "live-state",
      "error",
    ]);
  });

  it("serializes replacements and skips an aborted superseded source", async () => {
    let resolveFirst!: () => void;
    const first = new Promise<void>((resolve) => {
      resolveFirst = resolve;
    });
    const { player, raw } = nativePlayer();
    raw.replaceAsync.mockImplementationOnce(() => first).mockResolvedValueOnce(undefined);
    const transport = createNativePlayerTransport(player);
    const events = vi.fn();
    transport.subscribe(events);
    const oldRequest = new AbortController();
    const newRequest = new AbortController();

    const oldReplace = transport.replace(
      { uri: "https://loomarr.test/old.m3u8" },
      { attemptId: 1, signal: oldRequest.signal },
    );
    await vi.waitFor(() => expect(raw.replaceAsync).toHaveBeenCalledTimes(1));
    oldRequest.abort();
    const newReplace = transport.replace(
      { uri: "https://loomarr.test/new.m3u8" },
      { attemptId: 2, signal: newRequest.signal },
    );
    transport.firstFrame();
    expect(events).toHaveBeenCalledWith({ attemptId: 1, type: "first-frame" });
    resolveFirst();
    await oldReplace;
    await newReplace;
    expect(raw.replaceAsync).toHaveBeenCalledTimes(2);
    expect(raw.replaceAsync).toHaveBeenLastCalledWith(
      expect.objectContaining({ uri: "https://loomarr.test/new.m3u8" }),
    );
  });

  it("pauses and releases synchronously and idempotently on disposal", () => {
    const { emit, player, raw, removals } = nativePlayer();
    const transport = createNativePlayerTransport(player);
    const events = vi.fn();
    transport.subscribe(events);

    transport.dispose();
    transport.dispose();
    emit("playingChange", { isPlaying: true });

    expect(raw.pause).toHaveBeenCalledOnce();
    expect(raw.release).toHaveBeenCalledOnce();
    expect(removals).toHaveLength(3);
    expect(removals.every((remove) => remove.mock.calls.length === 1)).toBe(true);
    expect(events).not.toHaveBeenCalled();
  });

  it("releases on suspension and reattaches a fresh foreground player", async () => {
    const first = nativePlayer();
    const second = nativePlayer();
    const transport = createNativePlayerTransport(first.player, () => second.player);
    const playerChanges = vi.fn();
    transport.subscribePlayer(playerChanges);

    transport.suspend();

    expect(first.raw.pause).toHaveBeenCalledOnce();
    expect(first.raw.release).toHaveBeenCalledOnce();
    expect(transport.getPlayer()).toBeUndefined();

    transport.resume();
    expect(transport.getPlayer()).toBe(second.player);
    expect(second.raw.staysActiveInBackground).toBe(false);
    expect(playerChanges).toHaveBeenCalledTimes(2);

    await transport.replace(
      { uri: "https://loomarr.test/resumed.m3u8" },
      { attemptId: 9, signal: new AbortController().signal },
    );
    expect(second.raw.replaceAsync).toHaveBeenCalledWith(
      expect.objectContaining({ uri: "https://loomarr.test/resumed.m3u8" }),
    );
  });

  it("seeks to the reported live edge without creating another player", () => {
    const { player, raw } = nativePlayer();
    raw.currentOffsetFromLive = 12;
    const transport = createNativePlayerTransport(player);
    transport.goLive();
    expect(raw.seekBy).toHaveBeenCalledWith(12);
    expect(raw.play).toHaveBeenCalledOnce();
  });

  it("publishes frame time, bounded pause lag, and an explicit return to live", async () => {
    vi.useFakeTimers();
    const now = new Date("2026-08-26T20:00:00Z");
    vi.setSystemTime(now);
    const { emit, player, raw } = nativePlayer();
    raw.currentLiveTimestamp = now.getTime() - 12_000;
    raw.currentOffsetFromLive = 12;
    const transport = createNativePlayerTransport(player);
    const events = vi.fn();
    transport.subscribe(events);
    await transport.replace(
      { uri: "https://loomarr.test/live.m3u8" },
      { attemptId: 4, signal: new AbortController().signal },
    );

    emit("timeUpdate", {
      bufferedPosition: 30,
      currentLiveTimestamp: raw.currentLiveTimestamp,
      currentOffsetFromLive: raw.currentOffsetFromLive,
      currentTime: 100,
    });
    transport.pause();
    vi.advanceTimersByTime(30_000);
    transport.play();

    expect(events).toHaveBeenCalledWith({
      attemptId: 4,
      state: { lagSeconds: 0, mode: "live", noticeRevision: 0, viewerTimeMs: now.getTime() - 12_000 },
      type: "live-state",
    });
    expect(events).toHaveBeenCalledWith({
      attemptId: 4,
      state: { lagSeconds: 42, mode: "behind", noticeRevision: 0, viewerTimeMs: now.getTime() - 12_000 },
      type: "live-state",
    });

    transport.goLive();
    expect(events).toHaveBeenLastCalledWith({
      attemptId: 4,
      state: { lagSeconds: 0, mode: "live", noticeRevision: 0, viewerTimeMs: now.getTime() + 30_000 },
      type: "live-state",
    });
  });

  it("returns an expired paused point to live and increments its notice revision", async () => {
    vi.useFakeTimers();
    const now = new Date("2026-08-26T20:00:00Z");
    vi.setSystemTime(now);
    const { player, raw } = nativePlayer();
    raw.currentLiveTimestamp = now.getTime() - 901_000;
    raw.currentOffsetFromLive = 901;
    const transport = createNativePlayerTransport(player);
    const events = vi.fn();
    transport.subscribe(events);
    await transport.replace(
      { uri: "https://loomarr.test/live.m3u8" },
      { attemptId: 8, signal: new AbortController().signal },
    );

    transport.pause();
    transport.play();

    expect(raw.seekBy).toHaveBeenCalledWith(901);
    expect(events).toHaveBeenLastCalledWith({
      attemptId: 8,
      state: { lagSeconds: 0, mode: "live", noticeRevision: 1, viewerTimeMs: now.getTime() },
      type: "live-state",
    });
  });
});

describe("paired native image source", () => {
  const credential = { serverUrl: "http://loomarr.test:8080", token: "device-secret" };

  it("authenticates only same-origin image service paths", () => {
    expect(pairedNativeImageSource(credential, "/v1/images/poster.jpg")).toEqual({
      headers: { Authorization: "Bearer device-secret" },
      uri: "http://loomarr.test:8080/v1/images/poster.jpg",
    });
  });

  it("never sends the device token to an external image host", () => {
    expect(pairedNativeImageSource(credential, "https://cdn.example/poster.jpg")).toEqual({
      uri: "https://cdn.example/poster.jpg",
    });
    expect(pairedNativeImageSource(credential, "http://cdn.example/poster.jpg")).toBeUndefined();
    expect(pairedNativeImageSource(credential, "javascript:alert(1)")).toBeUndefined();
  });
});
