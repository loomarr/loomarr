import { useCallback, useState } from "react";

// Whether this browser's viewer has changed channel on Watch before, so the first-run hint is not
// shown again. Per browser, like the Channel-change sound: the web player keeps no server-side
// viewer preferences. A storage that throws (private mode, blocked site data) means not seen.
const KEY = "loomarr.player.first-run-hint";

const read = (): boolean => {
  try {
    return localStorage.getItem(KEY) === "seen";
  } catch {
    return false;
  }
};

/** Whether the hint was seen on an earlier visit, read once, and the call that remembers it. */
const useWatchHintPreference = (): [boolean, () => void] => {
  const [seen] = useState(read);
  const remember = useCallback(() => {
    try {
      localStorage.setItem(KEY, "seen");
    } catch {
      // Not remembered; the hint still goes for this visit.
    }
  }, []);
  return [seen, remember];
};

export { useWatchHintPreference };
