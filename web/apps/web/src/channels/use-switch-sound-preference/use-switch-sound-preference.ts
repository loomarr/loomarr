import { useCallback, useState } from "react";

// The viewer's "Channel-change sound" preference: per browser, default ON. The web player keeps no
// server-side viewer preferences, so it lives in localStorage; a storage that throws (private mode,
// blocked site data) just means the default.
const KEY = "loomarr.player.channel-change-sound";

const read = (): boolean => {
  try {
    return localStorage.getItem(KEY) !== "off";
  } catch {
    return true;
  }
};

const useSwitchSoundPreference = (): [boolean, (enabled: boolean) => void] => {
  const [enabled, setEnabled] = useState(read);
  const update = useCallback((next: boolean) => {
    setEnabled(next);
    try {
      localStorage.setItem(KEY, next ? "on" : "off");
    } catch {
      // Not persisted; the choice still holds for this page.
    }
  }, []);
  return [enabled, update];
};

export { useSwitchSoundPreference };
