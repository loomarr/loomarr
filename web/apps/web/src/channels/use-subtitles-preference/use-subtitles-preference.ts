import { SUBTITLES_OFF } from "@loomarr/core/channel-subtitles";
import { useCallback, useState } from "react";

// The viewer's subtitles: a track key ("en", "en-sdh") or Off, per browser, default Off. It follows
// the viewer across channels and programmes: a programme without that track plays its language's
// full track, else none. Like the channel-change sound it lives in localStorage; a storage that
// throws, or a value that is not a key, just means Off.
const KEY = "loomarr.player.subtitles";
const TRACK_KEY = /^[a-z0-9-]{1,40}$/;

const read = (): string => {
  try {
    const stored = localStorage.getItem(KEY) ?? "";
    return TRACK_KEY.test(stored) ? stored : SUBTITLES_OFF;
  } catch {
    return SUBTITLES_OFF;
  }
};

const useSubtitlesPreference = (): [string, (key: string) => void] => {
  const [key, setKey] = useState(read);
  const update = useCallback((next: string) => {
    setKey(next);
    try {
      localStorage.setItem(KEY, next);
    } catch {
      // Not persisted; the choice still holds for this page.
    }
  }, []);
  return [key, update];
};

export { useSubtitlesPreference };
