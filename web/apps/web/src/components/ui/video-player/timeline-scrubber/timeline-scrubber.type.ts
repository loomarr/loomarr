import type { GuideAiring } from "@loomarr/api";

interface TimelineScrubberProps {
  /** The schedule blocks to draw, in airtime order — programmes and the commercial breaks between. */
  airings: GuideAiring[];
  /** Epoch ms treated as "now" — the live playhead. Defaults to Date.now() at render. */
  nowMs?: number;
  className?: string;
}

export type { TimelineScrubberProps };
