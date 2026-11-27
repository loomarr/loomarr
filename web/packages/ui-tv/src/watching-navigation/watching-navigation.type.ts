import type { ChannelNumberEntry } from "@loomarr/ui";

type TvRemoteDigit = "0" | "1" | "2" | "3" | "4" | "5" | "6" | "7" | "8" | "9";

type TvWatchingRemoteEvent =
  | { atMs: number; digit: TvRemoteDigit; key: "digit" }
  | { atMs: number; key: "timeout" }
  | { key: "back" | "down" | "left" | "longSelect" | "menu" | "playPause" | "right" | "select" | "up" }
  | { direction: "down" | "up"; key: "channel" };

type TvWatchingRemoteIntent =
  | { direction: -1 | 1; kind: "step" }
  | { digits: string; kind: "tune-number" }
  | { kind: "close-layer" | "open-guide" | "open-surf" | "toggle-pause" };

/**
 * Where Watching is, for the remote (frontend-design §9.5, §9.7): the bare picture; the controls up
 * (Left and Right move focus along the strip and keys); Info, the on-air card pinned open (the same,
 * and Menu closes it); or a menu open, Audio or More (every arrow moves focus through its rows, and
 * Menu closes it).
 */
type TvWatchingFocus = "controls" | "info" | "menu" | "picture";

type TvWatchingRemoteState = {
  numberEntry?: {
    digits: string;
    expiresAtMs: number;
  };
};

type TvWatchingRemoteResult = {
  handled: boolean;
  intent?: TvWatchingRemoteIntent;
  state: TvWatchingRemoteState;
};

type TvNumberedChannel = {
  name: string;
  number: number;
};

type TvNumberEntryPresentation = ChannelNumberEntry & { expiresAtMs: number };

export type {
  TvNumberEntryPresentation,
  TvNumberedChannel,
  TvRemoteDigit,
  TvWatchingFocus,
  TvWatchingRemoteEvent,
  TvWatchingRemoteIntent,
  TvWatchingRemoteResult,
  TvWatchingRemoteState,
};
