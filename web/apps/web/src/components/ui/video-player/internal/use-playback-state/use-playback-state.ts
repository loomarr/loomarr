import { type RefObject, useCallback, useEffect, useRef, useState } from "react";
import type { LivePlaybackTransport } from "../../live-playback-transport.type";

// usePlaybackState owns everything about the <video> element's transport: play/pause, elapsed and
// duration, mute/volume, and the "metadata has loaded" flag. Extracted from VideoPlayer so the media
// concern (which is all element-driven and effect-heavy) is one testable unit and the component is
// left composing.
//
// The truth always flows FROM the element: `playing` follows the element's play/pause events (not our
// button), `duration` is read off the element on loadedmetadata, never from a caller.

// PlaybackWindow restricts playback to a slice of the source (§10 V54). Seconds of media time —
// the unit the element and everything here already speaks.
type PlaybackWindow = { startAt?: number; endAt?: number };

const clamp = (n: number, lo: number, hi: number) => Math.min(Math.max(n, lo), hi);

const usePlaybackState = (
  videoRef: RefObject<HTMLVideoElement | null>,
  { startAt, endAt }: PlaybackWindow = {},
  liveTransport?: LivePlaybackTransport,
) => {
  const [playing, setPlaying] = useState(false);
  const [muted, setMuted] = useState(false);
  // Volume 0–1, mirrored onto the element (the effect below). Kept in state so VolumeControl is a
  // controlled input.
  const [volume, setVolume] = useState(1);
  // ⚠ RAW = the element's own clock. What callers see is WINDOW-RELATIVE, derived below. Keeping
  // both apart is the whole trick: the element must seek in its own time, and the operator must
  // read the segment's.
  const [rawCurrent, setRawCurrent] = useState(0);
  // ⚠ Duration comes from the ELEMENT once loaded, never from a caller-supplied length (a probed
  // length can disagree with the file).
  const [rawDuration, setRawDuration] = useState(0);
  // Distinguishes "still loading" from "0 seconds in" for the placeholder / time readout.
  const [ready, setReady] = useState(false);

  // The window, resolved against what the element actually turned out to be.
  //
  // ⚠ `min` against `rawDuration` is not defensive tidying. The split-review preview is handed a
  // segment's span taken from a PROPOSAL, and the file may be shorter than the proposal believes
  // (a truncated download, a story fixture). Reporting "0:04 / 0:30" over a 2-second file is the
  // same lie as reporting the whole reel's length — in the other direction.
  const from = startAt ?? 0;
  const to = rawDuration > 0 ? Math.min(endAt ?? rawDuration, rawDuration) : (endAt ?? 0);
  // ⚠ **This is the point of the window.** A 30-second cut of a 22-minute reel must read
  // "0:04 / 0:30". Handing the reel's own numbers to the readout would present the whole recording
  // as if it were the clip, which is exactly what makes the preview useless for judging one cut.
  const duration = Math.max(0, to - from);
  const current = clamp(rawCurrent - from, 0, duration);

  // Mirror the volume level onto the element (mute is handled by the element's `muted` attribute).
  useEffect(() => {
    const el = videoRef.current;
    if (el) el.volume = volume;
  }, [volume, videoRef]);

  // seekTo takes WINDOW time and writes ELEMENT time.
  //
  // ⚠ Both halves or neither. VideoPlayer's keyboard handler calls `seekTo(current ± 5)` with the
  // window-relative `current` above, so making `current` relative while leaving this absolute
  // sends ArrowLeft to the start of the REEL rather than back five seconds. Half-applying the
  // window is worse than not applying it, and it has its own test.
  //
  // Windowless callers are byte-identical: `lo` is 0 and `hi` is the element's duration, which is
  // exactly the clamp this had before.
  const seekTo = useCallback(
    (seconds: number) => {
      const el = videoRef.current;
      if (!el) return;
      const elMax = Number.isFinite(el.duration) ? el.duration : 0;
      const lo = startAt ?? 0;
      const hi = Math.min(endAt ?? elMax, elMax);
      el.currentTime = clamp(lo + seconds, lo, hi);
    },
    [videoRef, startAt, endAt],
  );

  const toggle = useCallback(() => {
    const el = videoRef.current;
    if (!el) return;
    if (el.paused) {
      if (liveTransport) {
        void liveTransport.play(el);
        return;
      }
      // ⚠ Replay from the window's start. Without this, pressing Play after the window ended
      // advances one frame and `onTimeUpdate` immediately re-pauses — the classic "the button is
      // broken". The 0.05 slack absorbs the snap below.
      if (endAt !== undefined && el.currentTime >= endAt - 0.05) el.currentTime = startAt ?? 0;
      // The promise is caught: autoplay policies reject play() without a gesture, and an uncaught
      // rejection logs an unhandled-promise error on an ordinary interaction.
      void el.play().catch(() => setPlaying(false));
    } else if (liveTransport) liveTransport.pause(el);
    else el.pause();
  }, [videoRef, startAt, endAt, liveTransport]);

  // Re-clamp when the WINDOW MOVES under a mounted player — the operator retypes a cut point in
  // the mm:ss field while the preview is open.
  //
  // ⚠ **Only on a CHANGE, never on mount**, which is why the previous window is tracked rather
  // than leaning on the dependency array. A mount-time run fights the two handlers that own the
  // playhead: it re-seeks before `onLoadedMetadata` can (where the seek actually sticks), and it
  // drags the playhead off `endAt` after `onTimeUpdate` has parked it there — which makes the
  // player unable to stop at the end of its own window. Found by the pause test, not by reading.
  //
  // ⚠ And only when the playhead has fallen OUTSIDE the new window. Yanking it back to the start
  // on every keystroke would make the mm:ss field unusable while previewing.
  const windowRef = useRef<{ startAt?: number; endAt?: number }>({ startAt, endAt });
  useEffect(() => {
    const prev = windowRef.current;
    const moved = prev.startAt !== startAt || prev.endAt !== endAt;
    windowRef.current = { startAt, endAt };
    if (!moved) return;
    const el = videoRef.current;
    if (!el || (startAt === undefined && endAt === undefined)) return;
    const lo = startAt ?? 0;
    if (el.currentTime < lo || (endAt !== undefined && el.currentTime > endAt)) el.currentTime = lo;
  }, [startAt, endAt, videoRef]);

  // Handlers to spread onto the <video>: `playing` follows the element's own events, and metadata load
  // guards against a live stream's Infinity duration (a bar computed from it would be NaN%).
  const mediaHandlers = {
    onPlay: () => setPlaying(true),
    onPause: () => setPlaying(false),
    onTimeUpdate: (e: React.SyntheticEvent<HTMLVideoElement>) => {
      const el = e.currentTarget;
      // ⚠ Stop at the window's end, and SNAP. `timeupdate` fires about four times a second, so we
      // routinely arrive a couple of hundred milliseconds past `endAt`; without the snap the
      // readout flicks to "0:31 / 0:30" on the last frame. Pause, never loop and never rewind —
      // that is what <video> does at its natural end, so the toggle correctly reads "Play".
      if (endAt !== undefined && el.currentTime >= endAt) {
        el.pause();
        el.currentTime = endAt;
        setRawCurrent(endAt);
        return;
      }
      setRawCurrent(el.currentTime);
    },
    onLoadedMetadata: (e: React.SyntheticEvent<HTMLVideoElement>) => {
      const el = e.currentTarget;
      const d = el.duration;
      setRawDuration(Number.isFinite(d) ? d : 0);
      setReady(true);
      // ⚠ Seek HERE, in the handler, not in an effect. Before metadata the element has no seekable
      // range and a `currentTime` write is silently dropped. This also re-fires on a `src` change,
      // which re-clamps — desired.
      if (startAt !== undefined && el.currentTime < startAt) el.currentTime = startAt;
    },
  };

  return {
    playing,
    setPlaying,
    muted,
    setMuted,
    volume,
    setVolume,
    current,
    duration,
    ready,
    toggle,
    seekTo,
    mediaHandlers,
  };
};

export { usePlaybackState };
