import { useEffect } from "react";
import type { VideoPlayerChrome } from "@/components/loomarr/player/video-player";

/** What the watching screen needs of the player's own state: its mute. */
interface PlayerSound {
  muted: boolean;
  setMuted: (muted: boolean) => void;
}

/**
 * Hands the player's sound up to the screen that draws the controls. The player keeps its element's
 * state and draws nothing here; the watching screen draws Mute from what this reports.
 */
const ChromeBridge = ({
  chrome,
  onSound,
}: {
  chrome: VideoPlayerChrome;
  onSound: (sound: PlayerSound) => void;
}) => {
  const { muted, setMuted } = chrome;
  useEffect(() => onSound({ muted, setMuted }), [muted, onSound, setMuted]);
  return null;
};

export type { PlayerSound };
export { ChromeBridge };
