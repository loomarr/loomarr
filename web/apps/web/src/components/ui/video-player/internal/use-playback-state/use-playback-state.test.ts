import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { usePlaybackState } from "./use-playback-state";

// jsdom has no media pipeline (play() is not a function, duration is NaN, no events fire on their
// own). So this tests the parts that are OURS: the handlers translate element events into state, the
// transport controls act on the element, seek clamps, and volume mirrors down.
const fakeVideo = (over: Partial<HTMLVideoElement> = {}) =>
  ({
    paused: true,
    duration: 120,
    currentTime: 0,
    volume: 1,
    play: vi.fn().mockResolvedValue(undefined),
    pause: vi.fn(),
    ...over,
  }) as unknown as HTMLVideoElement;

const evt = (currentTime: number, duration: number) =>
  ({ currentTarget: { currentTime, duration } }) as React.SyntheticEvent<HTMLVideoElement>;

describe("usePlaybackState", () => {
  it("follows the element's play/pause events, not our calls", () => {
    const ref = { current: fakeVideo() };
    const { result } = renderHook(() => usePlaybackState(ref));
    expect(result.current.playing).toBe(false);

    act(() => result.current.mediaHandlers.onPlay());
    expect(result.current.playing).toBe(true);
    act(() => result.current.mediaHandlers.onPause());
    expect(result.current.playing).toBe(false);
  });

  it("reads duration off the element on metadata load; ignores a live Infinity", () => {
    const ref = { current: fakeVideo() };
    const { result } = renderHook(() => usePlaybackState(ref));
    expect(result.current.ready).toBe(false);

    act(() => result.current.mediaHandlers.onLoadedMetadata(evt(0, 90)));
    expect(result.current.duration).toBe(90);
    expect(result.current.ready).toBe(true);

    // A live stream reports Infinity — kept out of `duration` so a bar from it is not NaN%.
    act(() => result.current.mediaHandlers.onLoadedMetadata(evt(0, Number.POSITIVE_INFINITY)));
    expect(result.current.duration).toBe(0);
  });

  it("toggle plays a paused element and pauses a playing one", () => {
    const paused = { current: fakeVideo({ paused: true }) };
    const { result: r1 } = renderHook(() => usePlaybackState(paused));
    act(() => r1.current.toggle());
    expect(paused.current.play).toHaveBeenCalledOnce();

    const playingEl = { current: fakeVideo({ paused: false }) };
    const { result: r2 } = renderHook(() => usePlaybackState(playingEl));
    act(() => r2.current.toggle());
    expect(playingEl.current.pause).toHaveBeenCalledOnce();
  });

  it("seekTo clamps to [0, duration]", () => {
    const el = fakeVideo({ duration: 100, currentTime: 0 });
    const ref = { current: el };
    const { result } = renderHook(() => usePlaybackState(ref));

    act(() => result.current.seekTo(50));
    expect(el.currentTime).toBe(50);
    act(() => result.current.seekTo(-10)); // clamps up to 0
    expect(el.currentTime).toBe(0);
    act(() => result.current.seekTo(999)); // clamps down to duration
    expect(el.currentTime).toBe(100);
  });

  it("mirrors the volume level onto the element", () => {
    const el = fakeVideo({ volume: 1 });
    const ref = { current: el };
    const { result } = renderHook(() => usePlaybackState(ref));

    act(() => result.current.setVolume(0.3));
    expect(el.volume).toBeCloseTo(0.3);
  });
});
