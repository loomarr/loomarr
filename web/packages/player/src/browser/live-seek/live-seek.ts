// Seeking within a live channel's window (#2088 decision 2): the ±10 s controls, Start over and the
// playbar's past all name a WALL-CLOCK instant, and the element only knows media time. The frame on
// screen ties the two together — it is at `currentTime` and aired at `displayedWallMs` — so the
// target's media time is that frame's plus the difference. Arithmetic only: the browser transport
// reads the element and applies the answer.
import { LIVE_DVR_HORIZON_SECONDS } from "../../live-clock";

/** The stream's DVR window: a target older than this has already left the server's playlist. */
const LIVE_DVR_HORIZON_MS = LIVE_DVR_HORIZON_SECONDS * 1_000;
/** Inside this many seconds of the live edge a seek lands on live rather than a few seconds behind. */
const LIVE_SNAP_SECONDS = 3;
/** A seek to the window's very start would expire before the next tick; land this far inside it. */
const HORIZON_MARGIN_MS = 5_000;

interface LiveSeekInput {
  /** The element's media time of the frame on screen, in seconds. */
  currentTime: number;
  /** When the frame on screen aired. */
  displayedWallMs: number;
  /** The media time of the live edge (hls.js's live sync position, else the seekable end). */
  liveEdge?: number;
  nowMs: number;
  /** What the element can seek within, in seconds; omitted while the pipeline hasn't said. */
  seekable?: { start: number; end: number };
  /** Where the viewer asked to be. */
  targetWallMs: number;
}

type LiveSeek = { kind: "live" } | { kind: "media"; mediaTime: number; wallMs: number };

const liveSeek = ({
  currentTime,
  displayedWallMs,
  liveEdge,
  nowMs,
  seekable,
  targetWallMs,
}: LiveSeekInput): LiveSeek => {
  const earliestWallMs = nowMs - LIVE_DVR_HORIZON_MS + HORIZON_MARGIN_MS;
  const wanted = Math.max(targetWallMs, earliestWallMs);
  let mediaTime = currentTime + (wanted - displayedWallMs) / 1000;
  if (seekable) mediaTime = Math.min(Math.max(mediaTime, seekable.start), seekable.end);
  const edge = liveEdge ?? seekable?.end;
  if (wanted >= nowMs || (edge !== undefined && mediaTime >= edge - LIVE_SNAP_SECONDS))
    return { kind: "live" };
  return { kind: "media", mediaTime, wallMs: displayedWallMs + (mediaTime - currentTime) * 1000 };
};

export type { LiveSeek, LiveSeekInput };
export { liveSeek };
