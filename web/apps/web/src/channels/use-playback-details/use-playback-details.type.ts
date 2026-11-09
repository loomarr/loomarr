import type { LiveLevel } from "@loomarr/player/browser";

// What the browser really knows about the stream being played (§9.5). Every field is optional: a row
// the player has no figure for is not shown.
interface PlaybackFacts {
  /** The level on air as its master declared it (hls.js), absent on native HLS. */
  level?: LiveLevel;
  /** Seconds behind the live edge. */
  lagSeconds: number;
  /** The playback session's id. */
  sessionId?: string;
  /** What the <video> reports. */
  video?: {
    bufferedAheadSeconds?: number;
    droppedFrames?: number;
    height: number;
    totalFrames?: number;
    width: number;
  };
}

export type { PlaybackFacts };
