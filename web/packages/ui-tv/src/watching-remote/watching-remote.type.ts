interface TvWatchingRemotePort {
  commitNumber: () => void;
  enterNumber: (eventType: string) => boolean;
  openGuide: () => void;
  openSurf: () => void;
  pause: () => void;
  play: () => void;
  revealOverlay: () => void;
  step: (direction: -1 | 1) => void;
  togglePlayback: () => void;
}

export type { TvWatchingRemotePort };
