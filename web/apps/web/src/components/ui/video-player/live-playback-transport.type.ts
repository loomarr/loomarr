type LivePlaybackMode = "live" | "paused" | "behind";

interface LivePlaybackState {
  mode: LivePlaybackMode;
  lagSeconds: number;
  viewerTimeMs: number;
  noticeRevision: number;
}

interface LivePlaybackTransport {
  state: LivePlaybackState;
  play: (video: HTMLVideoElement) => Promise<void> | void;
  pause: (video: HTMLVideoElement) => void;
  goLive: (video: HTMLVideoElement) => Promise<void> | void;
  /** Move to a wall-clock instant inside the window; reaching the live edge goes live. */
  seek?: (video: HTMLVideoElement, wallClockMs: number) => Promise<void> | void;
}

export type { LivePlaybackMode, LivePlaybackState, LivePlaybackTransport };
