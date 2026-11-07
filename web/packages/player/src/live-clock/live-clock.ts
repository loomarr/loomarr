import type { LivePlaybackMode, LivePlaybackState } from "../player-controller";

/** How far back the server keeps a channel's window: a paused point older than this has expired. */
const LIVE_DVR_HORIZON_SECONDS = 15 * 60;
/**
 * A stall that leaves a live viewer further behind than this rejoins the live edge: a player resumes
 * where it froze, and a viewer who didn't pause shouldn't be left watching the past.
 */
const STALL_REJOIN_LAG_SECONDS = 30;

/** What the player knows about the frame on screen. Either field may be absent while metadata warms. */
interface LivePosition {
  /** Programme-date-time of the displayed frame, in Unix milliseconds. */
  programTimeMs?: number | null;
  /** Seconds the displayed frame trails the stream's live edge. */
  offsetFromLiveSeconds?: number | null;
}

interface LiveClockOptions {
  /** The device's wall clock in Unix milliseconds. Injected so tests can drive time. */
  now?: () => number;
}

/** `resume` found the paused point still inside the window (`behind`), gone from it (`expired`), or nothing paused. */
type LiveResume = "behind" | "expired" | "live";

/** How a stall ended: only the stream resuming on its own can leave a viewer behind. */
type StallEnd = "error" | "released" | "resumed" | "retuned";

interface LiveClock {
  /** Starts an attempt at the live edge. `serverTimeMs` is the server's wall clock at mint time. */
  begin: (serverTimeMs?: number) => void;
  /** The stall that started with `beginStall` stopped. */
  endStall: (why: StallEnd) => void;
  /**
   * The viewer's position has left the stream's window (the player lost the media behind it).
   * Returns to live and raises the notice, announced once per call.
   */
  expire: () => void;
  /** Returns to the live edge. */
  goLive: () => void;
  mode: () => LivePlaybackMode;
  /** The server's wall clock now: the device's, corrected by the skew measured at `begin`. */
  now: () => number;
  /** Freezes the viewer's position at the displayed frame. Samples are ignored until `resume` or `goLive`. */
  pause: (position?: LivePosition) => void;
  /**
   * Leaves a pause: behind live from the paused point, or live again when that point has expired,
   * either by age or because the player reports it no longer holds the media (`pointAvailable: false`).
   */
  resume: (options?: { pointAvailable?: boolean }) => LiveResume;
  /**
   * After a stall the viewer didn't cause, whether to rejoin the live edge. Answers `true` once,
   * when the stream first reports a position more than {@link STALL_REJOIN_LAG_SECONDS} behind live.
   */
  shouldRejoin: (position: LivePosition) => boolean;
  /**
   * The viewer moved to `wallClockMs` inside the window, short of the live edge (going live is
   * `goLive`). A paused viewer stays paused at the new point; a playing one plays on behind live.
   */
  seekTo: (wallClockMs: number) => void;
  /** The viewer's live state, reading the displayed frame from `position` when given. */
  state: (position?: LivePosition) => LivePlaybackState;
  /** Buffering began after the attempt's first frame while the viewer had not paused. */
  beginStall: () => void;
}

const finite = (value: number | null | undefined): value is number =>
  value !== null && value !== undefined && Number.isFinite(value);

/**
 * The live clock of one playing channel: where the viewer is relative to the channel's wall clock,
 * what pausing and resuming do to that, and when a stall sends them back to the live edge. Pure
 * state over an injected wall clock: the platform transport feeds it positions and acts on its answers.
 */
const createLiveClock = ({ now: deviceNow = () => Date.now() }: LiveClockOptions = {}): LiveClock => {
  let mode: LivePlaybackMode = "live";
  let noticeRevision = 0;
  let skewMs = 0;
  let viewerTimeMs = deviceNow();
  let lagMs = 0;
  let stalled = false;
  let stallBeganLive = false;
  let rejoinPending = false;

  const now = () => deviceNow() + skewMs;

  /** Updates the viewer's time from the displayed frame; a frozen (paused) frame is never re-read. */
  const sample = (position?: LivePosition) => {
    if (mode === "paused") return;
    if (finite(position?.programTimeMs)) {
      viewerTimeMs = position.programTimeMs;
    } else if (finite(position?.offsetFromLiveSeconds)) {
      viewerTimeMs = now() - Math.max(0, position.offsetFromLiveSeconds) * 1_000;
    } else if (mode === "live") {
      // A live stream without programme-date-time reports neither value. The viewer is still at the
      // live edge; keeping the last time would freeze programme identity across every schedule boundary.
      viewerTimeMs = now();
    } else {
      viewerTimeMs = now() - lagMs;
    }
  };

  const lagSeconds = () => Math.max(0, Math.round((now() - viewerTimeMs) / 1_000));

  const goLive = () => {
    mode = "live";
    lagMs = 0;
    viewerTimeMs = now();
  };

  /** The position the viewer chose has left the stream's window: back to live, once announced. */
  const expire = () => {
    noticeRevision += 1;
    goLive();
  };

  /** Seconds the displayed frame trails the wall clock, or undefined while the player can't say. */
  const lagBehindLive = ({ offsetFromLiveSeconds, programTimeMs }: LivePosition) => {
    if (finite(programTimeMs)) return (now() - programTimeMs) / 1_000;
    if (finite(offsetFromLiveSeconds)) return offsetFromLiveSeconds;
    return undefined;
  };

  return {
    begin: (serverTimeMs) => {
      skewMs = finite(serverTimeMs) ? serverTimeMs - deviceNow() : 0;
      noticeRevision = 0;
      stalled = false;
      rejoinPending = false;
      goLive();
    },
    beginStall: () => {
      if (mode === "paused" || stalled) return;
      stalled = true;
      stallBeganLive = mode === "live";
      rejoinPending = false;
    },
    endStall: (why) => {
      rejoinPending = why === "resumed" && stalled && stallBeganLive;
      stalled = false;
    },
    goLive,
    mode: () => mode,
    now,
    pause: (position) => {
      sample(position);
      mode = "paused";
    },
    expire,
    resume: ({ pointAvailable = true } = {}) => {
      if (mode !== "paused") return mode;
      if (!pointAvailable || lagSeconds() >= LIVE_DVR_HORIZON_SECONDS) {
        expire();
        return "expired";
      }
      mode = "behind";
      lagMs = Math.max(0, now() - viewerTimeMs);
      return "behind";
    },
    shouldRejoin: (position) => {
      if (!rejoinPending) return false;
      if (mode !== "live") {
        rejoinPending = false;
        return false;
      }
      const lag = lagBehindLive(position);
      if (lag === undefined) return false;
      rejoinPending = false;
      return lag > STALL_REJOIN_LAG_SECONDS;
    },
    seekTo: (wallClockMs) => {
      viewerTimeMs = wallClockMs;
      if (mode === "paused") return;
      mode = "behind";
      lagMs = Math.max(0, now() - wallClockMs);
    },
    state: (position) => {
      sample(position);
      return {
        lagSeconds: mode === "live" ? 0 : lagSeconds(),
        mode,
        noticeRevision,
        viewerTimeMs,
      };
    },
  };
};

export type { LiveClock, LiveClockOptions, LivePosition, LiveResume, StallEnd };
export { createLiveClock, LIVE_DVR_HORIZON_SECONDS, STALL_REJOIN_LAG_SECONDS };
