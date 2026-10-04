import type {
  TvNumberEntryPresentation,
  TvNumberedChannel,
  TvRemoteDigit,
  TvWatchingRemoteEvent,
  TvWatchingRemoteResult,
  TvWatchingRemoteState,
} from "./watching-navigation.type";

const MAX_CHANNEL_DIGITS = 3;
/** Default auto-tune delay. WCAG 2.2.1 (Q-N4): a per-device setting may raise this up to the max. */
const DEFAULT_NUMBER_ENTRY_MS = 1_200;
const MIN_NUMBER_ENTRY_MS = 1_200;
const MAX_NUMBER_ENTRY_MS = 10_000;

/** Clamps a candidate auto-tune duration to the range a device may be configured to use. */
const clampNumberEntryMs = (durationMs: number): number =>
  Math.min(MAX_NUMBER_ENTRY_MS, Math.max(MIN_NUMBER_ENTRY_MS, Math.round(durationMs)));

const tvWatchingRemoteEventFromNative = (
  eventType: string,
  atMs: number,
  eventKeyAction?: number,
): TvWatchingRemoteEvent | undefined => {
  // react-native-tvos emits Android key-down (0) and key-up (1) events. Normalizing
  // only the release prevents one physical press from producing two product intents.
  if (eventKeyAction === 0) return undefined;
  if (/^[0-9]$/.test(eventType)) {
    return { atMs, digit: eventType as TvRemoteDigit, key: "digit" };
  }
  switch (eventType) {
    case "up":
    case "down":
    case "left":
    case "right":
    case "select":
    case "menu":
    case "back":
      return { key: eventType };
    case "channelUp":
      return { direction: "up", key: "channel" };
    case "channelDown":
      return { direction: "down", key: "channel" };
    default:
      return undefined;
  }
};

const initialTvWatchingRemoteState: TvWatchingRemoteState = {};

const numberEntryIntent = (state: TvWatchingRemoteState, handled: boolean): TvWatchingRemoteResult => {
  const digits = state.numberEntry?.digits;
  return digits
    ? { handled, intent: { digits, kind: "tune-number" }, state: initialTvWatchingRemoteState }
    : { handled, state };
};

const reduceTvWatchingRemote = (
  state: TvWatchingRemoteState,
  event: TvWatchingRemoteEvent,
  durationMs: number = DEFAULT_NUMBER_ENTRY_MS,
): TvWatchingRemoteResult => {
  switch (event.key) {
    case "digit": {
      const digits = `${state.numberEntry?.digits ?? ""}${event.digit}`.slice(-MAX_CHANNEL_DIGITS);
      return {
        handled: true,
        state: { numberEntry: { digits, expiresAtMs: event.atMs + clampNumberEntryMs(durationMs) } },
      };
    }
    case "timeout":
      return state.numberEntry && event.atMs >= state.numberEntry.expiresAtMs
        ? numberEntryIntent(state, true)
        : { handled: false, state };
    case "select":
      return state.numberEntry
        ? numberEntryIntent(state, true)
        : { handled: true, intent: { kind: "open-guide" }, state };
    case "left":
    case "menu":
      return { handled: true, intent: { kind: "open-surf" }, state };
    case "up":
      return { handled: true, intent: { direction: 1, kind: "step" }, state };
    case "down":
      return { handled: true, intent: { direction: -1, kind: "step" }, state };
    case "channel":
      return {
        handled: true,
        intent: { direction: event.direction === "up" ? 1 : -1, kind: "step" },
        state,
      };
    case "back":
    case "right":
      return { handled: false, state };
  }
};

const tvNumberEntryPresentation = (
  state: TvWatchingRemoteState,
  channels: readonly TvNumberedChannel[],
): TvNumberEntryPresentation | undefined => {
  const entry = state.numberEntry;
  if (!entry) return undefined;
  const number = Number.parseInt(entry.digits, 10);
  const channelName = channels.find((channel) => channel.number === number)?.name;
  return { channelName, digits: entry.digits, expiresAtMs: entry.expiresAtMs };
};

export {
  clampNumberEntryMs,
  DEFAULT_NUMBER_ENTRY_MS,
  initialTvWatchingRemoteState,
  MAX_NUMBER_ENTRY_MS,
  MIN_NUMBER_ENTRY_MS,
  reduceTvWatchingRemote,
  tvNumberEntryPresentation,
  tvWatchingRemoteEventFromNative,
};
