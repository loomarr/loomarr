import type { ChannelNowNext } from "@loomarr/api";
import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { channel } from "@/test/fixtures/channels";
import { adjacentChannel, surfableCatalog, useChannelTuner } from "./use-channel-tuner";

const channels = [
  channel({ id: "ch-30", number: 30, name: "Thirty", inAppPlayable: true }),
  channel({ id: "ch-10", number: 10, name: "Ten", inAppPlayable: true }),
  channel({ id: "ch-20", number: 20, name: "Twenty", inAppPlayable: false }),
];

const noWarm = vi.fn().mockResolvedValue(undefined);

describe("channel tuner", () => {
  beforeEach(() => {
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(0);
      return 1;
    });
  });

  afterEach(() => vi.useRealTimers());

  it("sorts only the server-declared surfable channels and wraps", () => {
    const catalog = surfableCatalog(channels);
    expect(catalog.map((entry) => entry.id)).toEqual(["ch-10", "ch-30"]);
    expect(adjacentChannel(catalog, "ch-30", 1)?.id).toBe("ch-10");
    expect(adjacentChannel(catalog, "ch-10", -1)?.id).toBe("ch-30");
  });

  it("bases a rapid burst on the last REQUESTED channel before the route catches up", () => {
    const onTune = vi.fn();
    const { result } = renderHook(() =>
      useChannelTuner({ currentId: "ch-10", channels, nowNext: [], onTune, warmChannel: noWarm }),
    );

    act(() => {
      result.current.step(1); // 10 → 30
      result.current.step(1); // 30 → 10, while currentId is still 10
      result.current.step(-1); // 10 → 30
    });

    expect(onTune.mock.calls.map(([target]) => target.id)).toEqual(["ch-30", "ch-10", "ch-30"]);
    expect(result.current.channel?.id).toBe("ch-30");
    expect(result.current.attempt?.id).toBeGreaterThan(0);
  });

  it("acknowledges the requested channel before replacing the active presentation", () => {
    const frames: FrameRequestCallback[] = [];
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      frames.push(callback);
      return frames.length;
    });
    const onTune = vi.fn();
    const { result } = renderHook(() =>
      useChannelTuner({ currentId: "ch-10", channels, nowNext: [], onTune, warmChannel: noWarm }),
    );

    act(() => result.current.step(1));

    expect(result.current.channel?.id).toBe("ch-10");
    expect(result.current.requestedChannel?.id).toBe("ch-30");
    expect(result.current.acknowledging).toBe(true);
    expect(onTune).not.toHaveBeenCalled();

    act(() => frames.shift()?.(16));
    act(() => frames.shift()?.(32));

    expect(result.current.channel?.id).toBe("ch-30");
    expect(result.current.acknowledging).toBe(false);
    expect(onTune).toHaveBeenCalledWith(expect.objectContaining({ id: "ch-30" }));
  });

  it("does not lose a tune when the browser stops delivering animation frames", () => {
    vi.useFakeTimers();
    vi.stubGlobal("requestAnimationFrame", () => 1);
    const onTune = vi.fn();
    const { result } = renderHook(() =>
      useChannelTuner({ currentId: "ch-10", channels, nowNext: [], onTune, warmChannel: noWarm }),
    );

    act(() => result.current.step(1));
    expect(result.current.acknowledging).toBe(true);
    expect(onTune).not.toHaveBeenCalled();

    act(() => vi.advanceTimersByTime(50));

    expect(result.current.channel?.id).toBe("ch-30");
    expect(result.current.acknowledging).toBe(false);
    expect(onTune).toHaveBeenCalledWith(expect.objectContaining({ id: "ch-30" }));
  });

  it("pairs the selected channel with the already-loaded now row", () => {
    const nowNext: ChannelNowNext[] = [
      { channelId: "ch-10", now: { title: "The First Feature", gap: false, startMs: 1, stopMs: 2 } },
    ];
    const { result } = renderHook(() =>
      useChannelTuner({ currentId: "ch-10", channels, nowNext, onTune: vi.fn(), warmChannel: noWarm }),
    );
    expect(result.current.currentTitle).toBe("The First Feature");
  });

  it("reuses the exact signed URL and warmed state when tuning to a prepared neighbor", async () => {
    const mark = vi.spyOn(performance, "mark");
    const warmChannel = vi.fn().mockResolvedValue({
      url: "/v1/playout/hls/ch-30/master.m3u8?sig=one",
      expiresAt: Date.now() + 60 * 60 * 1000,
      warmed: true,
    });
    const { result } = renderHook(() =>
      useChannelTuner({ currentId: "ch-10", channels, nowNext: [], onTune: vi.fn(), warmChannel }),
    );
    expect(warmChannel).not.toHaveBeenCalled();
    act(() => result.current.ready("ch-10"));
    await vi.waitFor(() => expect(warmChannel).toHaveBeenCalledWith("ch-30", expect.any(AbortSignal)));
    await act(async () => Promise.resolve());
    expect(mark).toHaveBeenCalledWith("loomarr:tuner:warm:ch-30");
    act(() => result.current.step(1));
    expect(result.current.attempt?.warmed).toBe(true);
    expect(result.current.attempt?.playURL).toContain("sig=one");
  });

  it("does not certify an adjacent warm when the assets were unavailable", async () => {
    const mark = vi.spyOn(performance, "mark");
    mark.mockClear();
    const warmChannel = vi.fn().mockResolvedValue({
      url: "/v1/playout/hls/ch-30/master.m3u8?sig=miss",
      expiresAt: Date.now() + 60 * 60 * 1000,
      warmed: false,
    });
    const { result } = renderHook(() =>
      useChannelTuner({ currentId: "ch-10", channels, nowNext: [], onTune: vi.fn(), warmChannel }),
    );

    act(() => result.current.ready("ch-10"));
    await vi.waitFor(() => expect(warmChannel).toHaveBeenCalled());
    await act(async () => Promise.resolve());

    expect(mark).not.toHaveBeenCalledWith("loomarr:tuner:warm:ch-30");
  });
});
