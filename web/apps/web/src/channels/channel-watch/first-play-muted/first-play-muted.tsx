import { useEffect, useRef } from "react";
import type { VideoPlayerChrome } from "@/components/loomarr/player/video-player";

// The session's first play starts muted (§9.3), so a channel never blasts at someone who just opened
// the player; a pill takes one tap to undo it. Only the session's first play: later channels, and
// reopening Watch, keep the sound as the viewer left it.
const FIRST_PLAY_KEY = "loomarr.player.first-play";
const firstPlayOfSession = () => {
  try {
    if (sessionStorage.getItem(FIRST_PLAY_KEY)) return false;
    sessionStorage.setItem(FIRST_PLAY_KEY, "1");
  } catch {
    // No session storage (a private window): treat it as a first play only once per mount.
  }
  return true;
};

// FirstPlayMuted mutes the player once, on the session's first play, and tells the screen to show the
// pill until the sound comes on by any means (the pill, the key, M).
const FirstPlayMuted = ({
  chrome,
  onUnmuted,
  onStart,
}: {
  chrome: VideoPlayerChrome;
  onStart: () => void;
  onUnmuted: () => void;
}) => {
  const { setMuted } = chrome;
  const seenMuted = useRef(false);
  const started = useRef(false);
  useEffect(() => {
    if (started.current) return;
    started.current = true;
    if (!firstPlayOfSession()) return;
    setMuted(true);
    onStart();
  }, [setMuted, onStart]);
  useEffect(() => {
    if (chrome.muted) seenMuted.current = true;
    else if (seenMuted.current) onUnmuted();
  }, [chrome.muted, onUnmuted]);
  return null;
};

export { FirstPlayMuted };
