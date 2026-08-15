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

  it("delegates live pause and resume to the shared live transport", () => {
    const transport = {
      state: { mode: "live" as const, lagSeconds: 0, viewerTimeMs: 1_000, noticeRevision: 0 },
      play: vi.fn(),
      pause: vi.fn(),
      goLive: vi.fn(),
    };
    const paused = { current: fakeVideo({ paused: true }) };
    const playing = { current: fakeVideo({ paused: false }) };
    const { result: pausedResult } = renderHook(() => usePlaybackState(paused, {}, transport));
    const { result: playingResult } = renderHook(() => usePlaybackState(playing, {}, transport));

    act(() => pausedResult.current.toggle());
    act(() => playingResult.current.toggle());

    expect(transport.play).toHaveBeenCalledWith(paused.current);
    expect(transport.pause).toHaveBeenCalledWith(playing.current);
    expect(paused.current.play).not.toHaveBeenCalled();
    expect(playing.current.pause).not.toHaveBeenCalled();
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

  // --- the playback WINDOW (§10 V54) ------------------------------------------------------
  //
  // ⚠ The split-review preview plays a 30-second cut out of a 22-minute composite that has not
  // been split yet. Everything below exists so the operator is shown the SEGMENT, never the reel.
  //
  // These fire the handlers with the real fake element as `currentTarget`, because the window path
  // writes back to it (seeking on load, pausing and snapping at the end) — the lightweight `evt`
  // above cannot express that.
  describe("playback window", () => {
    const on = (el: HTMLVideoElement) => ({ currentTarget: el }) as React.SyntheticEvent<HTMLVideoElement>;

    it("seeks to the window start once metadata has loaded", () => {
      // ⚠ In the handler, not an effect: before metadata the element has no seekable range and a
      // currentTime write is silently dropped.
      const el = fakeVideo({ duration: 1320, currentTime: 0 });
      const { result } = renderHook(() => usePlaybackState({ current: el }, { startAt: 720, endAt: 750 }));

      act(() => result.current.mediaHandlers.onLoadedMetadata(on(el)));

      expect(el.currentTime).toBe(720);
    });

    it("reports the WINDOW's length, not the reel's", () => {
      // THE assertion. A 30s cut of a 22-minute reel must read 0:04 / 0:30 — showing 12:04 / 22:00
      // would present the whole recording as if it were the clip.
      const el = fakeVideo({ duration: 1320, currentTime: 724 });
      const { result } = renderHook(() => usePlaybackState({ current: el }, { startAt: 720, endAt: 750 }));

      act(() => result.current.mediaHandlers.onLoadedMetadata(on(el)));
      act(() => result.current.mediaHandlers.onTimeUpdate(on(el)));

      expect(result.current.duration).toBe(30);
      expect(result.current.current).toBe(4);
    });

    it("never reports a window longer than the file", () => {
      // A proposal can claim a span the file does not have (a truncated download). Reporting 30s
      // over a 2s file is the same lie in the other direction.
      const el = fakeVideo({ duration: 2, currentTime: 0 });
      const { result } = renderHook(() => usePlaybackState({ current: el }, { startAt: 0, endAt: 30 }));

      act(() => result.current.mediaHandlers.onLoadedMetadata(on(el)));

      expect(result.current.duration).toBe(2);
    });

    it("pauses and snaps at the window end", () => {
      // timeupdate fires ~4Hz, so we routinely arrive past endAt; without the snap the readout
      // flicks to 0:31 / 0:30 on the last frame.
      const el = fakeVideo({ duration: 1320, currentTime: 750.24 });
      const { result } = renderHook(() => usePlaybackState({ current: el }, { startAt: 720, endAt: 750 }));

      act(() => result.current.mediaHandlers.onLoadedMetadata(on(el)));
      act(() => result.current.mediaHandlers.onTimeUpdate(on(el)));

      expect(el.pause).toHaveBeenCalledOnce();
      expect(el.currentTime).toBe(750);
      expect(result.current.current).toBe(30);
    });

    it("seeks in WINDOW time, so the ±5s keys stay inside the segment", () => {
      // ⚠ The half-applied-window bug: relative `current` with an absolute `seekTo` sends
      // ArrowLeft to the start of the REEL. VideoPlayer calls seekTo(current ± 5).
      const el = fakeVideo({ duration: 1320, currentTime: 721 });
      const { result } = renderHook(() => usePlaybackState({ current: el }, { startAt: 720, endAt: 750 }));

      act(() => result.current.seekTo(-4)); // one second in, back five
      expect(el.currentTime).toBe(720);

      act(() => result.current.seekTo(999)); // past the end
      expect(el.currentTime).toBe(750);
    });

    it("replays from the window start when Play is pressed at the end", () => {
      // Without the rewind, Play advances one frame and onTimeUpdate re-pauses — "the button is
      // broken".
      const el = fakeVideo({ duration: 1320, currentTime: 750, paused: true });
      const { result } = renderHook(() => usePlaybackState({ current: el }, { startAt: 720, endAt: 750 }));

      act(() => result.current.toggle());

      expect(el.currentTime).toBe(720);
      expect(el.play).toHaveBeenCalledOnce();
    });

    it("re-clamps when the window moves under a mounted player", () => {
      // The operator retypes a cut point while the preview is open.
      const el = fakeVideo({ duration: 1320, currentTime: 724 });
      const { rerender } = renderHook(({ w }) => usePlaybackState({ current: el }, w), {
        initialProps: { w: { startAt: 720, endAt: 750 } },
      });

      rerender({ w: { startAt: 900, endAt: 930 } });

      expect(el.currentTime).toBe(900);
    });

    it("leaves the playhead alone when it is still inside the new window", () => {
      // Yanking it back on every keystroke would make the mm:ss field unusable while previewing.
      const el = fakeVideo({ duration: 1320, currentTime: 724 });
      const { rerender } = renderHook(({ w }) => usePlaybackState({ current: el }, w), {
        initialProps: { w: { startAt: 720, endAt: 750 } },
      });

      rerender({ w: { startAt: 720, endAt: 800 } });

      expect(el.currentTime).toBe(724);
    });
  });

  it("mirrors the volume level onto the element", () => {
    const el = fakeVideo({ volume: 1 });
    const ref = { current: el };
    const { result } = renderHook(() => usePlaybackState(ref));

    act(() => result.current.setVolume(0.3));
    expect(el.volume).toBeCloseTo(0.3);
  });
});
