interface TvWatchingRemotePort {
  commitNumber: () => void;
  enterNumber: (eventType: string) => boolean;
  openGuide: () => void;
  openSurf: () => void;
  revealOverlay: () => void;
  step: (direction: -1 | 1) => void;
}

export type { TvWatchingRemotePort };
