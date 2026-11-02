import type { ChannelNumberEntry } from "@loomarr/ui";

type TvRemoteDigit = "0" | "1" | "2" | "3" | "4" | "5" | "6" | "7" | "8" | "9";

type TvWatchingRemoteEvent =
  | { atMs: number; digit: TvRemoteDigit; key: "digit" }
  | { atMs: number; key: "timeout" }
  | { key: "back" | "down" | "left" | "longSelect" | "menu" | "playPause" | "right" | "select" | "up" }
  | { direction: "down" | "up"; key: "channel" };

type TvWatchingRemoteIntent =
  /** Seek 10 s back (-1) or forward (1) on the channel's window. */
  | { direction: -1 | 1; kind: "seek" }
  | { direction: -1 | 1; kind: "step" }
  | { digits: string; kind: "tune-number" }
  | { kind: "hide-controls" | "open-surf" | "show-controls" | "toggle-pause" };

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
  TvWatchingRemoteEvent,
  TvWatchingRemoteIntent,
  TvWatchingRemoteResult,
  TvWatchingRemoteState,
};
