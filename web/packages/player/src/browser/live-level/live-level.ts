/** One variant of a live channel's master playlist, as hls.js parsed it. */
interface LiveLevel {
  /** Declared bits per second. */
  bitrate?: number;
  /** The variant's CODECS attribute, verbatim; absent when the master did not name it. */
  codecs?: string;
  /** FRAME-RATE, frames per second. */
  frameRate?: number;
  height?: number;
  /** VIDEO-RANGE: SDR or PQ. */
  videoRange?: string;
}

/** A premium variant: more than the 1080p baseline, or HDR (#1512 G10). */
const isPremium = (level: LiveLevel) => (level.height ?? 0) > 1080 || level.videoRange === "PQ";

/**
 * The level a live channel plays, pinned for the whole tune. A 4K channel's master lists the H.264
 * 1080p SDR baseline and one premium HEVC variant, and playing the premium starts its own 4K encode
 * on the server, so a browser takes the premium only when `canPlay` (MediaSource.isTypeSupported)
 * accepts that variant's exact CODECS string, and otherwise the baseline. Never an ABR choice: a
 * bandwidth switch between them would change codec mid-stream and start or strand an encode.
 */
const decodes = (level: LiveLevel, canPlay: (mime: string) => boolean) =>
  level.codecs !== undefined && canPlay(`video/mp4; codecs="${level.codecs}"`);

const selectLiveLevel = (levels: readonly LiveLevel[], canPlay: (mime: string) => boolean): number => {
  const premium = levels.findIndex((level) => isPremium(level) && decodes(level, canPlay));
  if (premium >= 0) return premium;
  const baseline = levels.findIndex((level) => !isPremium(level));
  return Math.max(baseline, 0);
};

/** The viewer's quality (#2088 decision 7): the automatic choice, or a rung by its height. */
type LiveQuality = "auto" | number;

/** A rung the viewer can pick: a level of the master, by its index and height. */
interface LiveRung {
  height: number;
  index: number;
}

/**
 * The rungs a channel offers that this browser plays, and the height on air now; empty until the
 * manifest is read, and on native HLS, which picks for itself.
 */
interface LiveLadder {
  height?: number;
  /** The level on air, as its master declared it: what Playback details reads (§9.5). */
  onAir?: LiveLevel;
  rungs: readonly LiveRung[];
}

/**
 * The rungs a viewer may choose among: every level the master offers that this browser can play —
 * the baseline always, the premium only where it decodes. Nothing new is encoded: these are the
 * channel's own formats, and choosing the premium is the same opt-in Auto makes.
 */
const playableRungs = (levels: readonly LiveLevel[], canPlay: (mime: string) => boolean): LiveRung[] =>
  levels.flatMap((level, index) =>
    level.height === undefined || (isPremium(level) && !decodes(level, canPlay))
      ? []
      : [{ height: level.height, index }],
  );

/** The level to play: the viewer's rung when the master offers it here, else Auto's choice. */
const selectQualityLevel = (
  levels: readonly LiveLevel[],
  canPlay: (mime: string) => boolean,
  quality: LiveQuality,
): number => {
  const rung =
    quality === "auto" ? undefined : playableRungs(levels, canPlay).find((r) => r.height === quality);
  return rung?.index ?? selectLiveLevel(levels, canPlay);
};

export type { LiveLadder, LiveLevel, LiveQuality, LiveRung };
export { playableRungs, selectLiveLevel, selectQualityLevel };
