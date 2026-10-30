import { useCallback, useState } from "react";

// The viewer's audio (#2088 decision 6): the channel's default, or a language they chose from a
// programme's alternate, remembered per browser like quality and subtitles. A language, not "the
// alternate": the next programme's alternate may be another language, and a viewer who chose
// Spanish wants Spanish, not whatever dub comes next. A stored value that is not a language code
// reads as the default, as does a storage that throws (private mode, blocked site data).
const KEY = "loomarr.player.audio";

/** "default", or the ISO 639-2 language the viewer chose. */
type AudioPreference = "default" | (string & {});

const read = (): AudioPreference => {
  try {
    const stored: unknown = JSON.parse(localStorage.getItem(KEY) ?? '"default"');
    return typeof stored === "string" && /^[a-z]{2,3}$/.test(stored) ? stored : "default";
  } catch {
    return "default";
  }
};

const useAudioPreference = (): [AudioPreference, (audio: AudioPreference) => void] => {
  const [audio, setAudio] = useState(read);
  const update = useCallback((next: AudioPreference) => {
    setAudio(next);
    try {
      localStorage.setItem(KEY, JSON.stringify(next));
    } catch {
      // Not persisted; the choice still holds for this page.
    }
  }, []);
  return [audio, update];
};

export type { AudioPreference };
export { useAudioPreference };
