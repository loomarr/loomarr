import { type AudioPreference, audioPreferenceOf } from "@loomarr/core/audio-choice";
import { useCallback, useEffect, useRef, useState } from "react";

import { type LiveSubtitles, subtitlesPreferenceOf } from "../live-subtitles";

/** The device-state storage the apps already use (expo-secure-store's async get/set). */
interface PlayerPreferenceStorage {
  getItem: (key: string) => Promise<string | null>;
  setItem: (key: string, value: string) => Promise<void>;
}

// The viewer's audio (#2088 decision 6) and subtitles (decision 5) on this device, remembered across
// launches as web remembers them per browser. Each reads as its default until storage answers, and
// stays the default when it can't; a choice made before it answers wins over what it remembered.
// Storage is injected, as for the last channel, so the phone and the TV share one store.

const useStoredPreference = <T>(
  storage: PlayerPreferenceStorage,
  key: string,
  fallback: T,
  parse: (stored: unknown) => T,
) => {
  const [value, setValue] = useState<T>(fallback);
  const chosen = useRef(false);
  useEffect(() => {
    storage.getItem(key).then(
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
  }, [storage, key, parse]);
  const update = useCallback(
    (next: T) => {
      chosen.current = true;
      setValue(next);
      // Not persisted, the choice still holds until the app closes.
      storage.setItem(key, JSON.stringify(next)).catch(() => undefined);
    },
    [storage, key],
  );
  return [value, update] as const;
};

const useAudioPreference = (storage: PlayerPreferenceStorage) =>
  useStoredPreference<AudioPreference>(storage, "loomarr.player.audio", "default", audioPreferenceOf);

const useSubtitlesPreference = (storage: PlayerPreferenceStorage) =>
  useStoredPreference<LiveSubtitles>(storage, "loomarr.player.subtitles", "off", subtitlesPreferenceOf);

export type { PlayerPreferenceStorage };
export { useAudioPreference, useSubtitlesPreference };
