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

// Most tests keep hls.js unsupported and exercise the native/fallthrough seams. The retained-engine
// test opts in and records constructed controllers so a Channel-param rerender can prove reuse.
vi.mock("hls.js", () => {
  class MockHls {
    static isSupported = () => hls.supported;
    static Events = { ERROR: "error", FRAG_BUFFERED: "fragBuffered", MANIFEST_PARSED: "manifestParsed" };
    static ErrorTypes = { MEDIA_ERROR: "mediaError", NETWORK_ERROR: "networkError" };

    media: HTMLMediaElement | null = null;
    url: string | null = null;
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

    constructor() {
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

  it("retains one hls.js controller across a Channel-param tune", async () => {
    hls.supported = true;
    channelPlayUrl.mockImplementation((id: string) =>
      Promise.resolve({ relativeUrl: `/v1/playout/hls/${id}/master.m3u8` }),
    );
    const video = videoEl();
    const { result, rerender } = renderHook(({ id }) => useHlsPlayer(id), {
      initialProps: { id: "ch-1" },
    });

    let release!: () => void;
    act(() => {
      release = result.current.attach(video);
    });
    await waitFor(() => expect(hls.instances).toHaveLength(1));
    const controller = hls.instances[0] as {
      destroy: ReturnType<typeof vi.fn>;
      loadSource: ReturnType<typeof vi.fn>;
      stopLoad: ReturnType<typeof vi.fn>;
      transferMedia: ReturnType<typeof vi.fn>;
    };
    await waitFor(() =>
      expect(controller.loadSource).toHaveBeenCalledWith("/v1/playout/hls/ch-1/master.m3u8"),
    );
    expect(video.dataset.playbackChannel).toBe("ch-1");

    act(() => release());
    rerender({ id: "ch-2" });
    let finalRelease!: () => void;
    act(() => {
      finalRelease = result.current.attach(video);
    });

    await waitFor(() =>
      expect(controller.loadSource).toHaveBeenCalledWith("/v1/playout/hls/ch-2/master.m3u8"),
    );
    expect(video.dataset.playbackChannel).toBe("ch-2");
    expect(hls.instances).toHaveLength(1);
    expect(controller.stopLoad).toHaveBeenCalledOnce();
    expect(controller.transferMedia).toHaveBeenCalledOnce();

    act(() => finalRelease());
    await waitFor(() => expect(controller.destroy).toHaveBeenCalledOnce());
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
