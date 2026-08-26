import type { VideoPlayer } from "expo-video";
import { beforeEach, describe, expect, it, vi } from "vitest";

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
    expect(events.mock.calls.map(([event]) => event)).toEqual([
      { attemptId: 7, type: "first-frame" },
      { attemptId: 7, type: "playing" },
      { attemptId: 7, error: "decoder failed", type: "error" },
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
    expect(removals).toHaveLength(2);
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
