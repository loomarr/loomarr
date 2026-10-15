import type { TvFavouriteToggle } from "./favourite-navigation.type";

/**
 * A long press of Select on the Siri Remote (and the D-pad centre on Android TV), as react-native-tvos
 * names it for `useTVEventHandler`. tvOS sends it twice, when the hold is recognised (key action 0)
 * and when it is let go (1); only the release counts, so one hold toggles once.
 */
const isTvLongSelect = (eventType: string, eventKeyAction?: number): boolean =>
  eventType === "longSelect" && eventKeyAction !== 0;

/**
 * How long after a long press the focused row's own press-release is still that same hold's. tvOS sends
 * longSelect once the hold is recognised, then the row's onPress when the finger lifts, so the hold
 * would also tune the channel it just starred; measured on the Apple TV simulator at ~0.7 s after the
 * toggle for a 1.5 s hold, and on Watching in the same millisecond as the release's longSelect. The
 * stamp is refreshed by every longSelect, the release's included, so the window is measured from the
 * lift and a long hold does not outrun it; it is kept tight so a real OK just after is never swallowed.
 */
const tvLongSelectReleaseWindowMs = 1_500;

/**
 * Whether a row's press is the release of a long-press that already starred it, so it must not tune.
 * `armedAtMs` is when the last longSelect event arrived, whatever its key action.
 */
const tvPressIsLongSelectRelease = (armedAtMs: number | undefined, nowMs: number): boolean =>
  armedAtMs !== undefined && nowMs - armedAtMs <= tvLongSelectReleaseWindowMs;

/** What a long-press on `channelId` asks for: the opposite of where the channel is now. */
const tvFavouriteToggle = (channelId: string, favouriteIds: readonly string[]): TvFavouriteToggle => ({
  channelId,
  starred: !favouriteIds.includes(channelId),
});

export { isTvLongSelect, tvFavouriteToggle, tvPressIsLongSelectRelease };
