import { type AudioPreference, audioPreferenceOf } from "@loomarr/core/audio-choice";
import { useCallback, useState } from "react";

// The viewer's audio (#2088 decision 6): the channel's default, or a language they chose from a
// programme's alternate, remembered per browser like quality and subtitles. A stored value that is
// not a language code reads as the default, as does a storage that throws (private mode, blocked
// site data).
const KEY = "loomarr.player.audio";

const read = (): AudioPreference => {
  try {
    return audioPreferenceOf(JSON.parse(localStorage.getItem(KEY) ?? '"default"'));
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

export { useAudioPreference };
