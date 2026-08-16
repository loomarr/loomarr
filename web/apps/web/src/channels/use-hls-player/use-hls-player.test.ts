import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// useHlsPlayer's core (bind) drives hls.js over Media Source Extensions, which jsdom cannot run. So
// the seams tested here are the ones that are OURS and reachable without a media pipeline:
//  - the mint: attach() POSTs for a signed play-url and carries a device quality hint;
//  - the failure surfaces: a missing URL and a rejected mint both land on status "error";
//  - the transport fallthrough: with neither hls.js NOR native HLS available, we say so plainly.
// Real playback (MANIFEST_PARSED → playing) is left to the browser, where V46 verified it live.

// Hoisted so the vi.mock factory (which runs before module init) can close over them.
const { channelPlayUrl, hls, unwrap } = vi.hoisted(() => ({
  channelPlayUrl: vi.fn(),
  hls: { supported: false, instances: [] as unknown[] },
  unwrap: vi.fn((res: unknown) => res),
}));
vi.mock("@loomarr/api/endpoints/channels", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@loomarr/api/endpoints/channels")>();
  return {
    ...actual,
    channelPlayUrl,
  };
});
vi.mock("@loomarr/api/unwrap", () => ({ unwrap }));
vi.mock("@loomarr/api/mutator", () => ({
  toProblem: (e: unknown) => ({ detail: (e as Error)?.message, title: "Problem" }),
}));

// Most tests keep hls.js unsupported and exercise the native/fallthrough seams. Handoff tests opt
// in and record constructed controllers so a Channel-param rerender can prove MediaSource reuse.
vi.mock("hls.js", () => {
  class MockHls {
    static isSupported = () => hls.supported;
    static Events = {
      ERROR: "error",
      FRAG_BUFFERED: "fragBuffered",
      MANIFEST_PARSED: "manifestParsed",
    };
    static ErrorTypes = { MEDIA_ERROR: "mediaError", NETWORK_ERROR: "networkError" };

    media: HTMLMediaElement | null = null;
    url: string | null = null;
    config: unknown;
    attachMedia = vi.fn((media: HTMLMediaElement) => {
      this.media = media;
    });
    destroy = vi.fn(() => {
      this.media = null;
    });
    loadSource = vi.fn((url: string) => {
      this.url = url;
    });
    off = vi.fn();
    on = vi.fn();
    recoverMediaError = vi.fn();
    startLoad = vi.fn();
    stopLoad = vi.fn();
    transferMedia = vi.fn(() => null);

    constructor(config: unknown) {
      this.config = config;
      hls.instances.push(this);
    }
  }
  return { default: MockHls };
});

import { useHlsPlayer } from "./use-hls-player";

const videoEl = (canPlay = "") =>
  ({
    canPlayType: () => canPlay,
    dataset: {},
    poster: "",
    pause: vi.fn(),
    play: vi.fn().mockResolvedValue(undefined),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    removeAttribute: vi.fn(),
    load: vi.fn(),
  }) as unknown as HTMLVideoElement;

describe("useHlsPlayer", () => {
  beforeEach(() => {
    channelPlayUrl.mockReset();
    hls.supported = false;
    hls.instances.length = 0;
    unwrap.mockReset().mockImplementation((res: unknown) => res);
  });
  afterEach(() => vi.restoreAllMocks());

  it("mints a signed play-url for the channel, with a device quality hint", async () => {
    channelPlayUrl.mockResolvedValue({ relativeUrl: "/v1/playout/hls/master.m3u8" });
    const { result } = renderHook(() => useHlsPlayer("ch-1"));

    act(() => {
      result.current.attach(videoEl("application/vnd.apple.mpegurl"));
    });

    await waitFor(() => expect(channelPlayUrl).toHaveBeenCalledOnce());
    // The mint now carries the client's DeviceProfile body (§9.1 V48) plus the quality hint. The
    // profile shape is what device-profile.ts probes; here we assert the call CONTRACT — a profile
    // object first, then the quality params — not the specific capabilities (those depend on the JS
    // env's MediaSource, exercised in device-profile's own test).
    expect(channelPlayUrl).toHaveBeenCalledWith(
      "ch-1",
      expect.objectContaining({
        video: expect.any(Array),
        audio: expect.any(Array),
        video10bit: expect.any(Boolean),
      }),
      { quality: expect.stringMatching(/^(auto|720|480)$/) },
      { signal: expect.any(AbortSignal) },
    );
  });

  it("reuses an adjacent warmer's exact signed URL without minting again", async () => {
    const video = videoEl("application/vnd.apple.mpegurl");
    const { result } = renderHook(() =>
      useHlsPlayer("ch-2", {
        id: 2,
        adjacent: true,
        warmed: true,
        playURL: "/v1/playout/hls/ch-2/master.m3u8?sig=warmed",
      }),
    );

    act(() => {
      result.current.attach(video);
    });
    await waitFor(() => expect(video.play).toHaveBeenCalledOnce());
    expect(channelPlayUrl).not.toHaveBeenCalled();
    expect(video.src).toContain("sig=warmed");
  });

  it("keeps one active controller and a fresh one-source standby across repeated tunes", async () => {
    hls.supported = true;
    channelPlayUrl.mockImplementation((id: string) =>
      Promise.resolve({ relativeUrl: `/v1/playout/hls/${id}/master.m3u8` }),
    );
    const frames: Array<() => void> = [];
    const video = {
      ...videoEl(),
      requestVideoFrameCallback: vi.fn((callback: () => void) => {
        frames.push(callback);
        return frames.length;
      }),
      cancelVideoFrameCallback: vi.fn(),
    } as unknown as HTMLVideoElement;
    const { result, rerender } = renderHook(({ id }) => useHlsPlayer(id), {
      initialProps: { id: "ch-1" },
    });

    let release!: () => void;
    act(() => {
      release = result.current.attach(video);
    });
    await waitFor(() => expect(hls.instances).toHaveLength(1));
    const first = hls.instances[0] as {
      destroy: ReturnType<typeof vi.fn>;
      loadSource: ReturnType<typeof vi.fn>;
      on: ReturnType<typeof vi.fn>;
      stopLoad: ReturnType<typeof vi.fn>;
      transferMedia: ReturnType<typeof vi.fn>;
    };
    await waitFor(() => expect(first.loadSource).toHaveBeenCalledWith("/v1/playout/hls/ch-1/master.m3u8"));
    expect(video.dataset.playbackChannel).toBe("ch-1");

    const firstFragment = first.on.mock.calls.find((call: unknown[]) => call[0] === "fragBuffered")?.[1] as
      | (() => void)
      | undefined;
    act(() => firstFragment?.());
    act(() => frames.shift()?.());
    await waitFor(() => expect(hls.instances).toHaveLength(2));
    const freshSecond = hls.instances[1] as {
      attachMedia: ReturnType<typeof vi.fn>;
      destroy: ReturnType<typeof vi.fn>;
      loadSource: ReturnType<typeof vi.fn>;
      on: ReturnType<typeof vi.fn>;
    };
    expect(freshSecond.loadSource).not.toHaveBeenCalled();

    act(() => release());
    rerender({ id: "ch-2" });
    let finalRelease!: () => void;
    act(() => {
      finalRelease = result.current.attach(video);
    });

    await waitFor(() =>
      expect(freshSecond.loadSource).toHaveBeenCalledWith("/v1/playout/hls/ch-2/master.m3u8"),
    );
    expect(video.dataset.playbackChannel).toBe("ch-2");
    expect(first.stopLoad).toHaveBeenCalled();
    expect(first.transferMedia).toHaveBeenCalledOnce();
    expect(first.destroy).not.toHaveBeenCalled();
    expect(freshSecond.attachMedia).toHaveBeenLastCalledWith(video);

    const secondFragment = freshSecond.on.mock.calls.find(
      (call: unknown[]) => call[0] === "fragBuffered",
    )?.[1] as (() => void) | undefined;
    act(() => secondFragment?.());
    act(() => frames.shift()?.());
    await waitFor(() => expect(first.destroy).toHaveBeenCalledOnce());
    await waitFor(() => expect(hls.instances).toHaveLength(3));
    const freshThird = hls.instances[2] as {
      destroy: ReturnType<typeof vi.fn>;
      loadSource: ReturnType<typeof vi.fn>;
    };
    expect(freshThird.loadSource).not.toHaveBeenCalled();
    expect(
      hls.instances.filter(
        (instance) => !(instance as { destroy: ReturnType<typeof vi.fn> }).destroy.mock.calls.length,
      ),
    ).toHaveLength(2);

    act(() => finalRelease());
    rerender({ id: "ch-3" });
    let thirdRelease!: () => void;
    act(() => {
      thirdRelease = result.current.attach(video);
    });
    await waitFor(() =>
      expect(freshThird.loadSource).toHaveBeenCalledWith("/v1/playout/hls/ch-3/master.m3u8"),
    );
    expect(hls.instances).toHaveLength(3);
    for (const instance of hls.instances) {
      expect((instance as { loadSource: ReturnType<typeof vi.fn> }).loadSource).toHaveBeenCalledTimes(
        instance === freshThird ? 1 : instance === freshSecond || instance === first ? 1 : 0,
      );
    }

    act(() => thirdRelease());
    await waitFor(() => expect(freshSecond.destroy).toHaveBeenCalledOnce());
    await waitFor(() => expect(freshThird.destroy).toHaveBeenCalledOnce());
  });

  it("reuses compatible SourceBuffers after a prepared playlist reaches ended", async () => {
    hls.supported = true;
    channelPlayUrl.mockImplementation((id: string) =>
      Promise.resolve({ relativeUrl: `/v1/playout/hls/${id}/master.m3u8` }),
    );
    let replacementLoadedData!: () => void;
    const video = {
      ...videoEl(),
      addEventListener: vi.fn((event: string, callback: () => void) => {
        if (event === "loadeddata") replacementLoadedData = callback;
      }),
      requestVideoFrameCallback: vi.fn(() => 1),
      cancelVideoFrameCallback: vi.fn(),
    } as unknown as HTMLVideoElement;
    const resetCurrentTime = vi.fn();
    Object.defineProperty(video, "currentTime", {
      configurable: true,
      get: () => 1,
      set: resetCurrentTime,
    });
    const { result, rerender } = renderHook(({ id }) => useHlsPlayer(id), {
      initialProps: { id: "ch-1" },
    });

    let release!: () => void;
    act(() => {
      release = result.current.attach(video);
    });
    await waitFor(() => expect(hls.instances).toHaveLength(1));
    const controller = hls.instances[0] as {
      attachMedia: ReturnType<typeof vi.fn>;
      destroy: ReturnType<typeof vi.fn>;
      loadSource: ReturnType<typeof vi.fn>;
      transferMedia: ReturnType<typeof vi.fn>;
    };
    await waitFor(() =>
      expect(controller.loadSource).toHaveBeenCalledWith("/v1/playout/hls/ch-1/master.m3u8"),
    );
    vi.mocked(video.requestVideoFrameCallback).mockClear();
    vi.mocked(video.play).mockClear();

    let finishRemoval!: () => void;
    const remove = vi.fn();
    const transferred = {
      media: video,
      mediaSource: { readyState: "ended" },
      tracks: {
        video: {
          buffer: {
            updating: false,
            buffered: { length: 1, start: () => 0, end: () => 1 },
            addEventListener: vi.fn((event: string, callback: () => void) => {
              if (event === "updateend") finishRemoval = callback;
            }),
            removeEventListener: vi.fn(),
            remove,
          },
        },
      },
    };
    controller.transferMedia.mockReturnValue(transferred);
    act(() => release());
    rerender({ id: "ch-2" });
    act(() => {
      result.current.attach(video);
    });

    await waitFor(() => expect(remove).toHaveBeenCalledWith(0, 1));
    expect(video.pause).toHaveBeenCalledOnce();
    expect(vi.mocked(video.pause).mock.invocationCallOrder.at(-1)).toBeLessThan(
      remove.mock.invocationCallOrder.at(-1) ?? 0,
    );
    // WebKit can hold removal at the current decode position for almost two seconds. Keep the
    // outgoing element at its ended edge while SourceBuffer.remove runs, then rewind only after
    // updateend releases the transferred buffers.
    expect(resetCurrentTime).not.toHaveBeenCalled();
    expect(hls.instances).toHaveLength(2);
    expect(controller.destroy).not.toHaveBeenCalled();
    expect(video.play).not.toHaveBeenCalled();
    const replacement = hls.instances[1] as {
      attachMedia: ReturnType<typeof vi.fn>;
      config: { autoStartLoad?: boolean; enableWorker?: boolean };
      loadSource: ReturnType<typeof vi.fn>;
      on: ReturnType<typeof vi.fn>;
      startLoad: ReturnType<typeof vi.fn>;
    };
    expect(replacement.attachMedia).not.toHaveBeenCalled();
    expect(replacement.config.autoStartLoad).toBe(false);
    // Baseline HLS carries MPEG-TS segments, so transmuxing must stay off the UI thread. hls.js
    // shares and reference-counts its worker across source-scoped controllers; the bounded
    // active/standby pool must preserve that production invariant while disabling auto-load.
    expect(replacement.config.enableWorker).toBe(true);
    expect(replacement.loadSource).not.toHaveBeenCalled();
    expect(replacement.startLoad).not.toHaveBeenCalled();
    expect(video.play).not.toHaveBeenCalled();

    act(() => finishRemoval());
    await waitFor(() => expect(resetCurrentTime).toHaveBeenCalledWith(0));
    await waitFor(() => expect(replacement.attachMedia).toHaveBeenLastCalledWith(transferred));
    await waitFor(() => expect(video.requestVideoFrameCallback).toHaveBeenCalledOnce());
    await waitFor(() =>
      expect(replacement.loadSource).toHaveBeenCalledWith("/v1/playout/hls/ch-2/master.m3u8"),
    );
    await waitFor(() => expect(replacement.startLoad).toHaveBeenCalledOnce());
    expect(replacement.attachMedia.mock.invocationCallOrder.at(-1)).toBeLessThan(
      replacement.loadSource.mock.invocationCallOrder.at(-1) ?? 0,
    );
    expect(replacement.loadSource.mock.invocationCallOrder.at(-1)).toBeLessThan(
      replacement.startLoad.mock.invocationCallOrder.at(-1) ?? 0,
    );
    expect(vi.mocked(video.requestVideoFrameCallback).mock.invocationCallOrder.at(-1)).toBeLessThan(
      replacement.startLoad.mock.invocationCallOrder.at(-1) ?? 0,
    );
    // A cached WebKit append can decode before MANIFEST_PARSED is delivered. The target must have
    // a pending playback join before media loading starts so its first decoded frame cannot surface
    // paused; later manifest/fragment/loadeddata joins still cover WebKit's queued load resets.
    await waitFor(() => expect(video.play).toHaveBeenCalledOnce());
    expect(replacement.loadSource.mock.invocationCallOrder.at(-1)).toBeLessThan(
      vi.mocked(video.play).mock.invocationCallOrder.at(-1) ?? 0,
    );
    expect(vi.mocked(video.play).mock.invocationCallOrder.at(-1)).toBeLessThan(
      replacement.startLoad.mock.invocationCallOrder.at(-1) ?? 0,
    );
    vi.mocked(video.play).mockClear();
    const manifestParsed = replacement.on.mock.calls
      .filter((call: unknown[]) => call[0] === "manifestParsed")
      .at(-1)?.[1] as (() => void) | undefined;
    expect(manifestParsed).toBeTypeOf("function");
    manifestParsed?.();
    await waitFor(() => expect(video.play).toHaveBeenCalledOnce());

    vi.mocked(video.play).mockClear();
    const fragmentBuffered = replacement.on.mock.calls
      .filter((call: unknown[]) => call[0] === "fragBuffered")
      .at(-1)?.[1] as (() => void) | undefined;
    expect(fragmentBuffered).toBeTypeOf("function");
    fragmentBuffered?.();
    expect(video.play).toHaveBeenCalledOnce();

    vi.mocked(video.play).mockClear();
    replacementLoadedData();
    expect(video.play).toHaveBeenCalledOnce();
    replacementLoadedData();
    expect(video.play).toHaveBeenCalledOnce();
  });

  it("does not attach or load a superseded generation after its transferred clear finishes", async () => {
    hls.supported = true;
    channelPlayUrl.mockImplementation((id: string) =>
      Promise.resolve({ relativeUrl: `/v1/playout/hls/${id}/master.m3u8` }),
    );
    const video = videoEl();
    const { result, rerender } = renderHook(({ id }) => useHlsPlayer(id), {
      initialProps: { id: "ch-1" },
    });

    let firstRelease!: () => void;
    act(() => {
      firstRelease = result.current.attach(video);
    });
    await waitFor(() => expect(hls.instances).toHaveLength(1));
    const first = hls.instances[0] as {
      destroy: ReturnType<typeof vi.fn>;
      transferMedia: ReturnType<typeof vi.fn>;
    };
    let finishRemoval!: () => void;
    first.transferMedia.mockReturnValue({
      media: video,
      mediaSource: { readyState: "open" },
      tracks: {
        video: {
          buffer: {
            updating: false,
            buffered: { length: 1, start: () => 0, end: () => 1 },
            addEventListener: vi.fn((event: string, callback: () => void) => {
              if (event === "updateend") finishRemoval = callback;
            }),
            removeEventListener: vi.fn(),
            remove: vi.fn(),
          },
        },
      },
    });

    act(() => firstRelease());
    rerender({ id: "ch-2" });
    let supersededRelease!: () => void;
    act(() => {
      supersededRelease = result.current.attach(video);
    });
    await waitFor(() => expect(finishRemoval).toBeTypeOf("function"));
    expect(hls.instances).toHaveLength(2);

    act(() => supersededRelease());
    const superseded = hls.instances[1] as {
      attachMedia: ReturnType<typeof vi.fn>;
      destroy: ReturnType<typeof vi.fn>;
      loadSource: ReturnType<typeof vi.fn>;
      startLoad: ReturnType<typeof vi.fn>;
    };
    expect(superseded.loadSource).not.toHaveBeenCalled();
    expect(superseded.attachMedia).not.toHaveBeenCalled();
    expect(superseded.startLoad).not.toHaveBeenCalled();
    rerender({ id: "ch-3" });
    let currentRelease!: () => void;
    act(() => {
      currentRelease = result.current.attach(video);
    });
    await waitFor(() => expect(hls.instances).toHaveLength(3));
    const current = hls.instances[2] as {
      attachMedia: ReturnType<typeof vi.fn>;
      destroy: ReturnType<typeof vi.fn>;
      loadSource: ReturnType<typeof vi.fn>;
    };
    await waitFor(() => expect(current.loadSource).toHaveBeenCalledWith("/v1/playout/hls/ch-3/master.m3u8"));

    act(() => finishRemoval());
    await act(async () => undefined);
    expect(hls.instances).toHaveLength(3);
    expect(current.attachMedia).toHaveBeenLastCalledWith(video);
    expect(first.destroy).toHaveBeenCalledOnce();
    expect(superseded.destroy).not.toHaveBeenCalled();
    expect(superseded.attachMedia).not.toHaveBeenCalled();
    expect(superseded.startLoad).not.toHaveBeenCalled();

    act(() => currentRelease());
    await waitFor(() => expect(current.destroy).toHaveBeenCalled());
    await waitFor(() => expect(superseded.destroy).toHaveBeenCalled());
  });

  it("surfaces an error when the mint returns no URL", async () => {
    channelPlayUrl.mockResolvedValue({}); // neither relativeUrl nor url
    const { result } = renderHook(() => useHlsPlayer("ch-1"));

    act(() => {
      result.current.attach(videoEl());
    });

    await waitFor(() => expect(result.current.status).toBe("error"));
    expect(result.current.error).toMatch(/couldn't get a stream/i);
  });

  it("surfaces an error when the mint fails", async () => {
    // Drive the failure through unwrap throwing inside the hook's own .then — the same .catch path a
    // rejected fetch takes, but without a top-level rejected promise vitest would flag as unhandled.
    channelPlayUrl.mockResolvedValue({});
    unwrap.mockImplementation(() => {
      throw new Error("network down");
    });
    const { result } = renderHook(() => useHlsPlayer("ch-1"));

    act(() => {
      result.current.attach(videoEl());
    });

    await waitFor(() => expect(result.current.status).toBe("error"));
    expect(result.current.error).toBe("network down");
  });

  it("reports plainly when the browser can play neither MSE nor native HLS", async () => {
    channelPlayUrl.mockResolvedValue({ relativeUrl: "/v1/playout/hls/master.m3u8" });
    const { result } = renderHook(() => useHlsPlayer("ch-1"));

    act(() => {
      result.current.attach(videoEl("")); // canPlayType → "" : no native HLS, hls.js unsupported
    });

    await waitFor(() => expect(result.current.status).toBe("error"));
    expect(result.current.error).toMatch(/can't play live channels/i);
  });

  it("keeps the tuning overlay up until the replacement produces a decoded frame", async () => {
    channelPlayUrl.mockResolvedValue({ relativeUrl: "/v1/playout/hls/master.m3u8" });
    let firstFrame!: () => void;
    let loadedMetadata!: () => void;
    let loadedData!: () => void;
    const video = {
      ...videoEl("application/vnd.apple.mpegurl"),
      poster: "data:image/png;base64,held",
      addEventListener: vi.fn((event: string, callback: () => void) => {
        if (event === "loadedmetadata") loadedMetadata = callback;
        if (event === "loadeddata") loadedData = callback;
      }),
      requestVideoFrameCallback: vi.fn((callback: () => void) => {
        firstFrame = callback;
        return 1;
      }),
      cancelVideoFrameCallback: vi.fn(),
    } as unknown as HTMLVideoElement;
    const { result } = renderHook(() => useHlsPlayer("ch-1"));

    act(() => {
      result.current.attach(video);
    });
    await waitFor(() => expect(channelPlayUrl).toHaveBeenCalledOnce());
    expect(result.current.status).toBe("loading");
    expect(video.requestVideoFrameCallback).not.toHaveBeenCalled();

    act(() => loadedMetadata());
    expect(video.requestVideoFrameCallback).not.toHaveBeenCalled();
    act(() => loadedData());
    act(() => firstFrame());
    expect(result.current.status).toBe("playing");
    expect(video.removeAttribute).toHaveBeenCalledWith("poster");
  });

  it("ignores a mint that resolves after teardown (no state update)", async () => {
    let resolve!: (v: unknown) => void;
    channelPlayUrl.mockReturnValue(new Promise((r) => (resolve = r)));
    const { result } = renderHook(() => useHlsPlayer("ch-1"));

    let cleanup!: () => void;
    act(() => {
      cleanup = result.current.attach(videoEl());
    });
    // Unmount/switch before the mint lands, then let it resolve — cancelledRef must swallow it.
    act(() => cleanup());
    await act(async () => {
      resolve({ relativeUrl: "/late.m3u8" });
    });

    // The late resolution was cancelled, so status never advanced to playing/error from it.
    expect(result.current.status).toBe("loading");
  });

  it("aborts the superseded mint and lets only the latest attachment continue", async () => {
    const pending: Array<(value: unknown) => void> = [];
    channelPlayUrl.mockImplementation(() => new Promise((resolve) => pending.push(resolve)));
    const { result } = renderHook(() => useHlsPlayer("ch-1"));

    let firstCleanup!: () => void;
    act(() => {
      firstCleanup = result.current.attach(videoEl());
    });
    const firstSignal = channelPlayUrl.mock.calls[0]?.[3]?.signal as AbortSignal;
    act(() => firstCleanup());
    act(() => {
      result.current.attach(videoEl());
    });

    expect(firstSignal.aborted).toBe(true);
    await act(async () => {
      pending[0]?.({ relativeUrl: "/superseded.m3u8" });
      pending[1]?.({});
    });
    await waitFor(() => expect(result.current.status).toBe("error"));
    expect(result.current.error).toMatch(/couldn't get a stream/i);
  });
});
