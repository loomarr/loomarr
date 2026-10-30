import type { TrackDTO } from "@loomarr/api/models/trackDTO";
import { ownLanguageLabel } from "../channel-default-audio/language-label";
import type { AudioPreference } from "../use-audio-preference";

// The Settings sheet's audio (#2088 decision 6) for the programme at the playhead: the tracks API
// says which audio tracks the stream's Default and Alternate renditions carry, and the viewer's
// remembered language says which of them to play. The Alternate plays only while it carries that
// language; any other programme falls back to the Default, so a remembered Spanish never becomes
// whichever dub airs next.

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
const label = (track: TrackDTO) =>
  track.language ? `${ownLanguageLabel(track.language)} · stereo` : "Stereo";

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

export type { AudioChoice, AudioChoiceOption };
export { audioChoice };
