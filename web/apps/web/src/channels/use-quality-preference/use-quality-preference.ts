import type { LiveQuality } from "@loomarr/player/browser";
import { useCallback, useState } from "react";

// The viewer's quality (#2088 decision 7): Auto, or a rung by its height, remembered per browser like
// the channel-change sound. A stored value that is not a height reads as Auto, as does a storage that
// throws (private mode, blocked site data).
const KEY = "loomarr.player.quality";

const read = (): LiveQuality => {
  try {
    const height = Number(localStorage.getItem(KEY));
    return Number.isInteger(height) && height > 0 ? height : "auto";
  } catch {
    return "auto";
  }
};

const useQualityPreference = (): [LiveQuality, (quality: LiveQuality) => void] => {
  const [quality, setQuality] = useState(read);
  const update = useCallback((next: LiveQuality) => {
    setQuality(next);
    try {
      localStorage.setItem(KEY, String(next));
    } catch {
      // Not persisted; the choice still holds for this page.
    }
  }, []);
  return [quality, update];
};

export { useQualityPreference };
