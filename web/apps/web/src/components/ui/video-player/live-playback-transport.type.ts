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
}

export type { LivePlaybackMode, LivePlaybackState, LivePlaybackTransport };
