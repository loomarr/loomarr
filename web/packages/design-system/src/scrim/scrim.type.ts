interface ScrimProps {
  /** The edge the ink fades away toward: the trailing side, or the top. */
  from: "right" | "top";
  /** How far the fade runs in from that edge, in points. */
  feather: number;
  /** The ink's opacity where it is full, 0 to 1. */
  strength?: number;
  /**
   * How far the ink reaches above and below its parent, fading out over that distance, so what
   * stands on it stands on no box.
   */
  soften?: number;
  testID?: string;
}

export type { ScrimProps };
