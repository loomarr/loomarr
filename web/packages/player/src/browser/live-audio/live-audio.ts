// The viewer's audio (#2088 decision 6): a watched channel's master always names two audio
// renditions in each variant's group, "Default" (the channel's default language, muxed with the
// video) and "Alternate" (the programme's second language, or the default again when it has none).
// The master carries no languages, because it is read once per tune while they change programme by
// programme; the page labels the two from the tracks API at its playhead and says which to play.

const DEFAULT_AUDIO = "Default";
const ALTERNATE_AUDIO = "Alternate";

/** One audio rendition as a player lists it: hls.js's `name`, or a native AudioTrack's `label`. */
interface LiveAudioRendition {
  name?: string;
  label?: string;
}

const named = (rendition: LiveAudioRendition) => rendition.name ?? rendition.label;

/**
 * The rendition to play: the Alternate when the viewer chose it and the master offers it, else the
 * Default; -1 when the master names neither (a channel served before #2088 step 9, or a direct play).
 */
const selectAudioRendition = (renditions: readonly LiveAudioRendition[], alternate: boolean): number => {
  const alt = alternate ? renditions.findIndex((r) => named(r) === ALTERNATE_AUDIO) : -1;
  return alt >= 0 ? alt : renditions.findIndex((r) => named(r) === DEFAULT_AUDIO);
};

export type { LiveAudioRendition };
export { selectAudioRendition };
