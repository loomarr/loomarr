import { type AudioPreference, audioPreferenceOf } from "@loomarr/core/audio-choice";
import * as SecureStore from "expo-secure-store";
import { useCallback, useEffect, useRef, useState } from "react";

// The viewer's audio (#2088 decision 6) on this phone, remembered across launches as web remembers
// it per browser: the channel's default until the store answers, and the default again when it
// can't (a phone with no keychain to hand).
const KEY = "loomarr.player.audio";

const useAudioPreference = (): [AudioPreference, (audio: AudioPreference) => void] => {
  const [audio, setAudio] = useState<AudioPreference>("default");
  // A choice made before the store answers wins over what it remembered.
  const chosen = useRef(false);
  useEffect(() => {
    SecureStore.getItemAsync(KEY).then(
      (stored) => chosen.current || setAudio(audioPreferenceOf(stored)),
      () => undefined,
    );
  }, []);
  const update = useCallback((next: AudioPreference) => {
    chosen.current = true;
    setAudio(next);
    // Not persisted, the choice still holds until the app closes.
    SecureStore.setItemAsync(KEY, next).catch(() => undefined);
  }, []);
  return [audio, update];
};

export { useAudioPreference };
