/** One variant of a live channel's master playlist, as hls.js parsed it. */
interface LiveLevel {
  /** The variant's CODECS attribute, verbatim; absent when the master did not name it. */
  codecs?: string;
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
const selectLiveLevel = (levels: readonly LiveLevel[], canPlay: (mime: string) => boolean): number => {
  const premium = levels.findIndex(
    (level) =>
      isPremium(level) && level.codecs !== undefined && canPlay(`video/mp4; codecs="${level.codecs}"`),
  );
  if (premium >= 0) return premium;
  const baseline = levels.findIndex((level) => !isPremium(level));
  return Math.max(baseline, 0);
};

export type { LiveLevel };
export { selectLiveLevel };
