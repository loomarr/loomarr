import { useCallback, useEffect, useRef, useState } from "react";
import type { DrawerChannel } from "../channel-drawer";
import { useNumberEntry } from "../use-number-entry";

// Info closes by itself after eight seconds while the channel plays (playback-clients: Panels).
const INFO_PLAYING_MS = 8_000;

// The one sheet the chrome can have open at a time (§9.1, plane 5).
type WatchSheet = "channels" | "info" | "settings";

type WatchShortcutsOptions = {
  // The airing programme has an Info sheet to show.
  hasInfo: boolean;
  // The channels drawer's list, which a typed number is looked up in; undefined without a drawer.
  channels?: DrawerChannel[];
  onTune: (channelId: string) => void;
  // The viewer paused: Info stays open to be read.
  viewerPaused: boolean;
};

// The Watch player's one right-hand sheet (Settings, Info or Channels) and its keys (#1659 W1): I
// opens and closes Info, G Channels; digits open a number entry; Enter tunes it at once; Escape
// closes the sheet. Closing a sheet hands focus back to what opened it (its button, or the frame
// for a key).
const useWatchShortcuts = ({ hasInfo, channels, onTune, viewerPaused }: WatchShortcutsOptions) => {
  const [sheet, setSheet] = useState<WatchSheet | undefined>(undefined);
  const openerRef = useRef<HTMLElement | null>(null);

  const toggleSheet = (next: WatchSheet) => {
    if (!sheet && document.activeElement instanceof HTMLElement) openerRef.current = document.activeElement;
    setSheet((open) => (open === next ? undefined : next));
  };
  const closeSheet = useCallback(() => setSheet(undefined), []);

  useEffect(() => {
    if (sheet) return;
    openerRef.current?.focus({ preventScroll: true });
    openerRef.current = null;
  }, [sheet]);

  useEffect(() => {
    if (sheet !== "info" || viewerPaused) return;
    const timer = window.setTimeout(closeSheet, INFO_PLAYING_MS);
    return () => window.clearTimeout(timer);
  }, [sheet, viewerPaused, closeSheet]);

  const entry = useNumberEntry({ channels: channels ?? [], onTune });

  // Whether the key was the player's to handle.
  const onShortcut = (key: string): boolean => {
    if (key === "Escape" && sheet) {
      closeSheet();
      return true;
    }
    if ((key === "i" || key === "I") && hasInfo) {
      toggleSheet("info");
      return true;
    }
    if (!channels) return false;
    if (entry.press(key)) return true;
    if (key === "Enter") return entry.commit();
    if (key === "g" || key === "G") {
      toggleSheet("channels");
      return true;
    }
    return false;
  };

  return { sheet, toggleSheet, closeSheet, entry, onShortcut };
};

export type { WatchSheet };
export { useWatchShortcuts };
