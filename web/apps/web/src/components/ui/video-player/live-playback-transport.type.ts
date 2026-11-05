import type { LivePlaybackState } from "@loomarr/player";

type LivePlaybackMode = LivePlaybackState["mode"];

/** A live player's own controls: pause stays paused until play, and goLive returns to the live edge. */
interface LivePlaybackTransport {
  state: LivePlaybackState;
  play: () => Promise<void> | void;
  pause: () => void;
  goLive: () => Promise<void> | void;
  /** Move to a wall-clock instant inside the window; reaching the live edge goes live. */
  seek?: (wallClockMs: number) => Promise<void> | void;
}

export type { LivePlaybackMode, LivePlaybackState, LivePlaybackTransport };
