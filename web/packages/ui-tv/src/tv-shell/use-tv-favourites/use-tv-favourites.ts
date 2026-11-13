import { useCallback, useEffect, useRef, useState } from "react";
import { AccessibilityInfo } from "react-native";

import { tvFavouriteToggle } from "../../favourite-navigation";

/** How long the Favorites confirmation stays up after a long-press of Select. */
const TV_FAVOURITE_NOTE_MS = 2_500;

const announceToScreenReader = (note: string) => void AccessibilityInfo.announceForAccessibility(note);

/** The viewer's favourites, as the paired client's `myChannels` controller offers them. */
interface TvFavouritesSource {
  getSnapshot: () => { favouriteIds: readonly string[] };
  setFavourite: (channelId: string, starred: boolean) => Promise<unknown>;
}

/**
 * A long-press of Select on a Guide or Surf row stars or unstars that row's channel (a short press
 * still tunes). The controller shows the change at once and puts it back if the server refuses, so
 * the note reads what the lists say afterwards rather than what was asked for. The note is read
 * aloud and clears itself.
 */
const useTvFavourites = (
  myChannels: TvFavouritesSource,
  catalog: readonly { id: string; name: string }[],
  announce: (note: string) => void = announceToScreenReader,
) => {
  const [note, setNote] = useState<string | undefined>(undefined);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => () => clearTimeout(timer.current), []);
  const toggle = useCallback(
    async (channelId: string) => {
      const { starred } = tvFavouriteToggle(channelId, myChannels.getSnapshot().favouriteIds);
      await myChannels.setFavourite(channelId, starred);
      const applied = myChannels.getSnapshot().favouriteIds.includes(channelId) === starred;
      const name = catalog.find(({ id }) => id === channelId)?.name;
      const text = applied
        ? `${starred ? "Added to" : "Removed from"} Favorites${name ? ` · ${name}` : ""}`
        : "Couldn't update Favorites";
      setNote(text);
      announce(text);
      clearTimeout(timer.current);
      timer.current = setTimeout(() => setNote(undefined), TV_FAVOURITE_NOTE_MS);
    },
    [announce, catalog, myChannels],
  );
  return { note, toggle };
};

export type { TvFavouritesSource };
export { TV_FAVOURITE_NOTE_MS, useTvFavourites };
