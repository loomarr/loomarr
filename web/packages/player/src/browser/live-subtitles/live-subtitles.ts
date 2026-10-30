// The viewer's subtitles (#2088 step 7): off, or a text track of the programme at the playhead,
// remembered across channels. The subtitles sidecar (GET /v1/channels/{id}/subtitles) lists each
// airing's tracks by language and title, so a choice is the pair. A programme without that exact
// track plays the first in the same language; one without the language plays none, and the choice
// is kept for the next programme that has it.

interface LiveSubtitleTrack {
  /** ISO 639-2, e.g. `eng`; empty when the media left it untagged. */
  language: string;
  /** The stream's title ("SDH"); empty when the media carries none. */
  title: string;
}

type LiveSubtitles = "off" | LiveSubtitleTrack;

/** The index in `tracks` that `choice` plays, or -1 for none. */
const selectSubtitleTrack = (tracks: readonly LiveSubtitleTrack[], choice: LiveSubtitles): number => {
  if (choice === "off") return -1;
  const exact = tracks.findIndex((t) => t.language === choice.language && t.title === choice.title);
  return exact >= 0 ? exact : tracks.findIndex((t) => t.language === choice.language);
};

/** `C` and the CC tool: off → the first track → … → the last → off. Nothing to cycle stays off. */
const nextSubtitles = (tracks: readonly LiveSubtitleTrack[], choice: LiveSubtitles): LiveSubtitles => {
  const next = selectSubtitleTrack(tracks, choice) + 1;
  const track = tracks[next];
  return track ? { language: track.language, title: track.title } : "off";
};

export type { LiveSubtitles, LiveSubtitleTrack };
export { nextSubtitles, selectSubtitleTrack };
