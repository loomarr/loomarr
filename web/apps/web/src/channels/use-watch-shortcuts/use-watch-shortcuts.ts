import { useCallback, useEffect, useRef, useState } from "react";
import type { DrawerChannel } from "../channel-drawer";
import { useNumberEntry } from "../use-number-entry";

// Info closes by itself after eight seconds while the channel plays (playback-clients: Panels).
const INFO_PLAYING_MS = 8_000;

// The one sheet the chrome can have open at a time (§9.1, layer 4).
type WatchSheet = "audio" | "channels" | "details" | "info" | "more" | "picture" | "subtitles";

type WatchShortcutsOptions = {
  // C steps the subtitles to the next choice, round to Off; omitted while there is nothing to step.
  onNextSubtitles?: () => void;
  // I toggles Playback details (§9.3): a sheet (a phone), or `onDetails` for the pointer's panel.
  detailsAsSheet?: boolean;
  // Info is the strip's on-air card (a pointer, variant J): nothing in it takes focus, so opening it
  // hands focus straight back to what opened the layer, where Escape still reaches the player.
  infoAsCard?: boolean;
  onDetails?: () => void;
  // The channels drawer's list, which a typed number is looked up in; undefined without a drawer.
  channels?: DrawerChannel[];
  // Back to the channel before this one; omitted when there is none (L then does nothing).
  onLast?: () => void;
  // Esc with nothing open leaves the player.
  onLeave?: () => void;
  onTune: (channelId: string) => void;
  // The viewer paused: Info stays open to be read.
  viewerPaused: boolean;
};

// The Watch player's one right-hand sheet (Subtitles, Audio, Picture, More, Info or Channels) and its
// keys (§9.3): A opens Audio, C steps the subtitles, I Playback details, G Channels, L goes to the last
// channel; digits open a number entry;
// Enter tunes it at once; Escape closes the sheet, then leaves. Closing a sheet hands focus back to
// what opened it (its button, or the frame for a key).
const useWatchShortcuts = ({
  detailsAsSheet = false,
  infoAsCard = false,
  onDetails,
  onNextSubtitles,
  channels,
  onLast,
  onLeave,
  onTune,
  viewerPaused,
}: WatchShortcutsOptions) => {
  const [sheet, setSheet] = useState<WatchSheet | undefined>(undefined);
  const openerRef = useRef<HTMLElement | null>(null);

  const toggleSheet = (next: WatchSheet) => {
    if (!sheet && document.activeElement instanceof HTMLElement) openerRef.current = document.activeElement;
    setSheet((open) => (open === next ? undefined : next));
  };
  const closeSheet = useCallback(() => setSheet(undefined), []);

  useEffect(() => {
    const focusless = sheet === "info" && infoAsCard;
    if (sheet && !focusless) return;
    openerRef.current?.focus({ preventScroll: true });
    if (!sheet) openerRef.current = null;
  }, [sheet, infoAsCard]);

  useEffect(() => {
    if (sheet !== "info" || viewerPaused) return;
    const timer = window.setTimeout(closeSheet, INFO_PLAYING_MS);
    return () => window.clearTimeout(timer);
  }, [sheet, viewerPaused, closeSheet]);

  const entry = useNumberEntry({ channels: channels ?? [], onTune });

  // Whether the key was the player's to handle.
  const onShortcut = (key: string): boolean => {
    if (key === "Escape") {
      if (sheet) closeSheet();
      else if (onLeave) onLeave();
      else return false;
      return true;
    }
    if (key === "a" || key === "A") {
      toggleSheet("audio");
      return true;
    }
    if ((key === "c" || key === "C") && onNextSubtitles) {
      onNextSubtitles();
      return true;
    }
    if ((key === "i" || key === "I") && (detailsAsSheet || onDetails)) {
      if (detailsAsSheet) toggleSheet("details");
      else onDetails?.();
      return true;
    }
    if ((key === "l" || key === "L") && onLast) {
      onLast();
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
