import { type AudioPreference, audioPreferenceOf } from "@loomarr/core/audio-choice";
import { type LiveSubtitles, subtitlesPreferenceOf } from "@loomarr/player/native";
import * as SecureStore from "expo-secure-store";
import { useCallback, useEffect, useRef, useState } from "react";

// The viewer's audio (#2088 decision 6) and subtitles (decision 5) on this phone, remembered across
// launches as web remembers them per browser. Each reads as its default until the keychain answers,
// and stays the default when it can't; a choice made before it answers wins over what it remembered.

const useStoredPreference = <T>(key: string, fallback: T, parse: (stored: unknown) => T) => {
  const [value, setValue] = useState<T>(fallback);
  const chosen = useRef(false);
  useEffect(() => {
    SecureStore.getItemAsync(key).then(
      (stored) => {
        if (chosen.current || stored === null) return;
        try {
          setValue(parse(JSON.parse(stored)));
        } catch {
          // Not a stored choice: the default stands.
        }
      },
      () => undefined,
    );
  }, [key, parse]);
  const update = useCallback(
    (next: T) => {
      chosen.current = true;
      setValue(next);
      // Not persisted, the choice still holds until the app closes.
      SecureStore.setItemAsync(key, JSON.stringify(next)).catch(() => undefined);
    },
    [key],
  );
  return [value, update] as const;
};

const useAudioPreference = () =>
  useStoredPreference<AudioPreference>("loomarr.player.audio", "default", audioPreferenceOf);

const useSubtitlesPreference = () =>
  useStoredPreference<LiveSubtitles>("loomarr.player.subtitles", "off", subtitlesPreferenceOf);

export { useAudioPreference, useSubtitlesPreference };
