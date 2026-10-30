import type { TrackDTO } from "@loomarr/api/models/trackDTO";

// The Settings sheet's audio (#2088 decision 6) for the programme at the playhead: the tracks API
// says which audio tracks the stream's Default and Alternate renditions carry, and the viewer's
// remembered language says which of them to play. The Alternate plays only while it carries that
// language; any other programme falls back to the Default, so a remembered Spanish never becomes
// whichever dub airs next.

/**
 * The viewer's remembered audio: "default", or the ISO 639-2 language they chose. A language, not
 * "the alternate": the next programme's alternate may be another language, and a viewer who chose
 * Spanish wants Spanish, not whatever dub comes next.
 */
type AudioPreference = "default" | (string & {});

/** A stored preference as the client reads it back: anything but a language code is the default. */
const audioPreferenceOf = (stored: unknown): AudioPreference =>
  typeof stored === "string" && /^[a-z]{2,3}$/.test(stored) ? stored : "default";

interface AudioChoiceOption {
  label: string;
  selected: boolean;
  value: AudioPreference;
}

interface AudioChoice {
  /** Play the master's Alternate rendition. */
  alternate: boolean;
  /** Default first, then the alternate when the programme carries one; empty until known. */
  options: AudioChoiceOption[];
}

interface AiringAudio {
  audio: readonly TrackDTO[];
  defaultAudio: number;
  alternateAudio?: number;
}

// As the §9 mock reads: the language in itself, then its channels (the encode is always stereo).
// The name is the server's, so a phone (whose engine has no language names) reads as web does; a
// track it can't name ("und", untagged) reads as no language.
const label = (track: TrackDTO) => (track.languageName ? `${track.languageName} · stereo` : "Stereo");

const audioChoice = (airing: AiringAudio | undefined, preference: AudioPreference): AudioChoice => {
  const def = airing?.audio.find((track) => track.index === airing.defaultAudio);
  if (!airing || !def) return { alternate: false, options: [] };
  const alt =
    airing.alternateAudio === undefined
      ? undefined
      : airing.audio.find((track) => track.index === airing.alternateAudio);
  const alternate = alt?.language !== undefined && alt.language === preference;
  const options: AudioChoiceOption[] = [{ label: label(def), selected: !alternate, value: "default" }];
  if (alt?.language) options.push({ label: label(alt), selected: alternate, value: alt.language });
  return { alternate, options };
};

export type { AiringAudio, AudioChoice, AudioChoiceOption, AudioPreference };
export { audioChoice, audioPreferenceOf };
