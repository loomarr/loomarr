import type {
  TvWatchingFocus,
  TvWatchingRemoteEvent,
  TvWatchingRemoteIntent,
  TvWatchingRemoteState,
} from "../watching-navigation";

type TvRemoteClock = {
  clearTimeout(handle: unknown): void;
  now(): number;
  setTimeout(run: () => void, delayMs: number): unknown;
};

type TvRemoteOutcome = { handled: boolean; intent?: TvWatchingRemoteIntent };

type TvRemoteOptions = {
  clock: TvRemoteClock;
  numberEntryMs?: number;
  onEvent: (event: TvWatchingRemoteEvent, outcome: TvRemoteOutcome) => void;
};

type TvRemote = {
  /** Whether a press is the release of a long press (and so not a tune or OK); spends the stamp. */
  consumeLongSelectRelease(): boolean;
  dispose(): void;
  getSnapshot(): TvWatchingRemoteState;
  /** Leaving Watching drops a half-typed channel number and its timer. */
  leaveWatching(): void;
  /**
   * A react-native-tvos key event; only Watching (`watching`) acts on it, but any screen arms a long
   * press. `focus` is where Watching is: with the controls up Left and Right move focus through
   * them, in the More menu every arrow does, and Menu closes an open layer.
   */
  nativeKey(
    eventType: string,
    eventKeyAction: number | undefined,
    watching: boolean,
    focus?: TvWatchingFocus,
  ): void;
  /** OK on Watching, unless it is a long press letting go. */
  select(): void;
  setNumberEntryMs(durationMs: number): void;
  subscribe(listener: () => void): () => void;
};

export type { TvRemote, TvRemoteClock, TvRemoteOptions, TvRemoteOutcome };
