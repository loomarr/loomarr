import { type RefObject, useCallback, useEffect, useState } from "react";

// usePlaybackState owns everything about the <video> element's transport: play/pause, elapsed and
// duration, mute/volume, and the "metadata has loaded" flag. Extracted from VideoPlayer so the media
// concern (which is all element-driven and effect-heavy) is one testable unit and the component is
// left composing.
//
// The truth always flows FROM the element: `playing` follows the element's play/pause events (not our
// button), `duration` is read off the element on loadedmetadata, never from a caller.
const usePlaybackState = (videoRef: RefObject<HTMLVideoElement | null>) => {
  const [playing, setPlaying] = useState(false);
  const [muted, setMuted] = useState(false);
  // Volume 0–1, mirrored onto the element (the effect below). Kept in state so VolumeControl is a
  // controlled input.
  const [volume, setVolume] = useState(1);
  const [current, setCurrent] = useState(0);
  // ⚠ Duration comes from the ELEMENT once loaded, never from a caller-supplied length (a probed
  // length can disagree with the file).
  const [duration, setDuration] = useState(0);
  // Distinguishes "still loading" from "0 seconds in" for the placeholder / time readout.
  const [ready, setReady] = useState(false);

  // Mirror the volume level onto the element (mute is handled by the element's `muted` attribute).
  useEffect(() => {
    const el = videoRef.current;
    if (el) el.volume = volume;
  }, [volume, videoRef]);

  const seekTo = useCallback(
    (seconds: number) => {
      const el = videoRef.current;
      if (!el) return;
      const max = Number.isFinite(el.duration) ? el.duration : 0;
      el.currentTime = Math.min(Math.max(0, seconds), max);
    },
    [videoRef],
  );

  const toggle = useCallback(() => {
    const el = videoRef.current;
    if (!el) return;
    // The promise is caught: autoplay policies reject play() without a gesture, and an uncaught
    // rejection logs an unhandled-promise error on an ordinary interaction.
    if (el.paused) void el.play().catch(() => setPlaying(false));
    else el.pause();
  }, [videoRef]);

  // Handlers to spread onto the <video>: `playing` follows the element's own events, and metadata load
  // guards against a live stream's Infinity duration (a bar computed from it would be NaN%).
  const mediaHandlers = {
    onPlay: () => setPlaying(true),
    onPause: () => setPlaying(false),
    onTimeUpdate: (e: React.SyntheticEvent<HTMLVideoElement>) => setCurrent(e.currentTarget.currentTime),
    onLoadedMetadata: (e: React.SyntheticEvent<HTMLVideoElement>) => {
      const d = e.currentTarget.duration;
      setDuration(Number.isFinite(d) ? d : 0);
      setReady(true);
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
