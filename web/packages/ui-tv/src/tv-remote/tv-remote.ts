import { tvPressIsLongSelectRelease } from "../favourite-navigation";
import type { TvWatchingFocus, TvWatchingRemoteEvent, TvWatchingRemoteState } from "../watching-navigation";
import {
  DEFAULT_NUMBER_ENTRY_MS,
  initialTvWatchingRemoteState,
  reduceTvWatchingRemote,
  tvWatchingRemoteEventFromNative,
} from "../watching-navigation";
import type { TvRemote, TvRemoteOptions } from "./tv-remote.type";

/**
 * The TV remote on Watching: digit entry with its auto-tune timer, and the long-press stamp that
 * keeps a hold's release from also counting as OK. Every Watching event goes through `onEvent` with
 * its outcome, the timer's own timeout included, so the app only runs intents.
 */
const createTvRemote = ({
  clock,
  numberEntryMs = DEFAULT_NUMBER_ENTRY_MS,
  onEvent,
}: TvRemoteOptions): TvRemote => {
  let state: TvWatchingRemoteState = initialTvWatchingRemoteState;
  let durationMs = numberEntryMs;
  let expiry: { handle: unknown } | undefined;
  let longSelectAtMs: number | undefined;
  const listeners = new Set<() => void>();

  const cancelExpiry = () => {
    if (expiry) clock.clearTimeout(expiry.handle);
    expiry = undefined;
  };

  const setState = (next: TvWatchingRemoteState) => {
    if (next === state) return;
    const entryChanged = next.numberEntry !== state.numberEntry;
    state = next;
    if (entryChanged) {
      cancelExpiry();
      const expiresAtMs = next.numberEntry?.expiresAtMs;
      if (expiresAtMs !== undefined) {
        expiry = {
          handle: clock.setTimeout(
            () => {
              expiry = undefined;
              dispatch({ atMs: expiresAtMs, key: "timeout" });
            },
            Math.max(0, expiresAtMs - clock.now()),
          ),
        };
      }
    }
    for (const listener of listeners) listener();
  };

  const dispatch = (event: TvWatchingRemoteEvent, focus: TvWatchingFocus = "picture") => {
    const { handled, intent, state: next } = reduceTvWatchingRemote(state, event, durationMs, focus);
    setState(next);
    onEvent(event, { handled, intent });
  };

  // A row's or Watching's press that is a long press letting go; reading it spends the stamp.
  const consumeLongSelectRelease = () => {
    if (!tvPressIsLongSelectRelease(longSelectAtMs, clock.now())) return false;
    longSelectAtMs = undefined;
    return true;
  };

  return {
    consumeLongSelectRelease,
    dispose: cancelExpiry,
    getSnapshot: () => state,
    leaveWatching: () => setState(initialTvWatchingRemoteState),
    nativeKey: (eventType, eventKeyAction, watching, focus = "picture") => {
      if (eventType === "longSelect") longSelectAtMs = clock.now();
      if (!watching) return;
      const event = tvWatchingRemoteEventFromNative(eventType, clock.now(), eventKeyAction);
      // OK on Watching arrives as the surface's own press (`select`), not here, so it counts once.
      if (event && event.key !== "select") dispatch(event, focus);
    },
    select: () => {
      if (!consumeLongSelectRelease()) dispatch({ key: "select" });
    },
    setNumberEntryMs: (next) => {
      durationMs = next;
    },
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
};

export { createTvRemote };
