import type { PlayerSnapshot } from "@loomarr/player";
import type { ReactNode } from "react";

import type { PlayerChromeProps } from "../player-chrome/player-chrome.type";

interface ChannelNumberEntry {
  channelName?: string;
  digits: string;
  /**
   * When present, draws a live countdown to the auto-tune ("auto-tunes in 1.2 s", #1659 map 5a).
   * Absent (e.g. a platform with no digit entry) draws just the typed digits.
   */
  expiresAtMs?: number;
}

interface TvWatchingProps {
  /** The §9 chrome's content, from the tuned channel's timeline; omitted until a channel is tuned. */
  chrome?: Pick<PlayerChromeProps, "band" | "formatTime" | "ident" | "playbar" | "sheet" | "transport">;
  /** False while another journey (Guide, Surf) is drawn over the still-mounted player. */
  chromeVisible: boolean;
  /** The controls are up: the D-pad belongs to their focus (#2088 decision 15). */
  controlsShown: boolean;
  /** True only while the authoritative Channel catalog request is unresolved. */
  loading: boolean;
  loadError?: string;
  numberEntry?: ChannelNumberEntry;
  /** OK over the bare picture: the app's remote map decides (show the controls, or tune typed digits). */
  onBarePress: () => void;
  /** Returns to server selection while preserving the current credential as a restart fallback. */
  onChangeServer: () => void;
  /** The failed channel's way on: the Guide, where Watching is the TV app's root. */
  onLeave: () => void;
  onRetry: () => void;
  /** Instrumentation: the channel switch overlay showed its readout or its still. */
  onSwitchShown?: (what: "osd" | "still") => void;
  player: ReactNode;
  snapshot: PlayerSnapshot;
}

export type { ChannelNumberEntry, TvWatchingProps };
