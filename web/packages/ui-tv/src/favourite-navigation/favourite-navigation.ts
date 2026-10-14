import type { TvFavouriteToggle } from "./favourite-navigation.type";

/**
 * A long press of Select on the Siri Remote (and the D-pad centre on Android TV), as react-native-tvos
 * names it for `useTVEventHandler`. tvOS sends it twice, when the hold is recognised (key action 0)
 * and when it is let go (1); only the release counts, so one hold toggles once.
 */
const isTvLongSelect = (eventType: string, eventKeyAction?: number): boolean =>
  eventType === "longSelect" && eventKeyAction !== 0;

/** What a long-press on `channelId` asks for: the opposite of where the channel is now. */
const tvFavouriteToggle = (channelId: string, favouriteIds: readonly string[]): TvFavouriteToggle => ({
  channelId,
  starred: !favouriteIds.includes(channelId),
});

export { isTvLongSelect, tvFavouriteToggle };
