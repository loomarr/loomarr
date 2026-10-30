import type { LiveSubtitles } from "@loomarr/player/browser";
import { useCallback, useState } from "react";

// The viewer's subtitles (#2088 step 7): off, or a track by language and title, remembered per
// browser like quality. A stored value that is not a track reads as off, as does a storage that
// throws (private mode, blocked site data).
const KEY = "loomarr.player.subtitles";

const read = (): LiveSubtitles => {
  try {
    const stored: unknown = JSON.parse(localStorage.getItem(KEY) ?? '"off"');
    if (
      typeof stored === "object" &&
      stored !== null &&
      "language" in stored &&
      "title" in stored &&
      typeof stored.language === "string" &&
      typeof stored.title === "string"
    ) {
      return { language: stored.language, title: stored.title };
    }
    return "off";
  } catch {
    return "off";
  }
};

const useSubtitlesPreference = (): [LiveSubtitles, (subtitles: LiveSubtitles) => void] => {
  const [subtitles, setSubtitles] = useState(read);
  const update = useCallback((next: LiveSubtitles) => {
    setSubtitles(next);
    try {
      localStorage.setItem(KEY, JSON.stringify(next));
    } catch {
      // Not persisted; the choice still holds for this page.
    }
  }, []);
  return [subtitles, update];
};

export { useSubtitlesPreference };
