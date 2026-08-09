import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// useHlsPlayer's core (bind) drives hls.js over Media Source Extensions, which jsdom cannot run. So
// the seams tested here are the ones that are OURS and reachable without a media pipeline:
//  - the mint: attach() POSTs for a signed play-url and carries a device quality hint;
//  - the failure surfaces: a missing URL and a rejected mint both land on status "error";
//  - the transport fallthrough: with neither hls.js NOR native HLS available, we say so plainly.
// Real playback (MANIFEST_PARSED → playing) is left to the browser, where V46 verified it live.

// Hoisted so the vi.mock factory (which runs before module init) can close over them.
const { channelPlayUrl, unwrap } = vi.hoisted(() => ({
  channelPlayUrl: vi.fn(),
  unwrap: vi.fn((res: unknown) => res),
}));
vi.mock("@loomarr/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@loomarr/api")>();
  return {
    ...actual,
    channelsApi: { ...actual.channelsApi, channelPlayUrl },
    unwrap,
    toProblem: (e: unknown) => ({ detail: (e as Error)?.message, title: "Problem" }),
  };
});

// hls.js reports unsupported so bind() takes the non-MSE path — the test drives a <video> whose
// canPlayType also returns "" (no native HLS), landing on the deterministic error branch instead of
// constructing a real Hls instance.
vi.mock("hls.js", () => ({
  default: { isSupported: () => false, Events: {}, ErrorTypes: {} },
}));

import { useHlsPlayer } from "./use-hls-player";

const videoEl = (canPlay = "") =>
  ({
    canPlayType: () => canPlay,
    play: vi.fn().mockResolvedValue(undefined),
    removeAttribute: vi.fn(),
    load: vi.fn(),
  }) as unknown as HTMLVideoElement;

describe("useHlsPlayer", () => {
  beforeEach(() => {
    channelPlayUrl.mockReset();
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
    );
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
});
