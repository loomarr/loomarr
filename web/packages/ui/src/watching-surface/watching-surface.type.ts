import type { PlayerSnapshot } from "@loomarr/player";
import type { ReactNode } from "react";

import type { ProgrammeIdentityData } from "../identity";
import type { PlaybarSegment, PlaybarWindow, PlayerBandData } from "./player-chrome";

interface ChannelNumberEntry {
  channelName?: string;
  digits: string;
  /**
   * When present, draws a live countdown to the auto-tune ("auto-tunes in 1.2 s", #1659 map 5a).
   * Absent (e.g. a platform with no digit entry) draws just the typed digits.
   */
  expiresAtMs?: number;
}

interface WatchingProgrammeData extends ProgrammeIdentityData {
  progressPercent?: number;
  /** "12m left": shown on the channel switch overlay's surf card. */
  remainingLabel?: string;
}

interface WatchingScheduleData {
  next?: Pick<ProgrammeIdentityData, "timeLabel" | "title">;
  now?: WatchingProgrammeData;
}

/** The chrome's data for the tuned channel: what `playerChromeFromGuide` reads from the Guide. */
interface WatchingChromeData {
  band: PlayerBandData;
  segments: readonly PlaybarSegment[];
  window: PlaybarWindow;
}

interface WatchingSurfaceProps {
  /** The tuned channel's band and film strip; omitted until a channel is tuned. */
  chrome?: WatchingChromeData;
  /** False while another transient journey is drawn over the still-mounted player. */
  chromeVisible?: boolean;
  /** Presentation-owned visibility; playback state deliberately does not own UI chrome. */
  controlsVisible?: boolean;
  /** Changes only after handled Watching activity, not while live playback presentation updates. */
  controlsActivityKey?: number;
  /** Formats a wall-clock instant for the band's clocks ("8:44 PM"). */
  formatTime: (ms: number) => string;
  /** True only while the authoritative Channel catalog request is unresolved. */
  loading?: boolean;
  loadError?: string;
  numberEntry?: ChannelNumberEntry;
  /** Returns to server selection while preserving the current credential as a restart fallback. */
  onChangeServer?: () => void;
  onDismissControls: () => void;
  onGoLive: () => void;
  onOpenGuide: () => void;
  onRetry: () => void;
  /** Instrumentation (TV): the channel switch overlay showed its readout or its still. */
  onSwitchShown?: (what: "osd" | "still") => void;
  /** Tunes a channel by id: the Last channel key. */
  onTuneChannel: (channelId: string) => void;
  /** The wall-clock instant at the playhead, for the film strip. */
  playheadMs: number;
  player: ReactNode;
  snapshot: PlayerSnapshot;
}

export type {
  ChannelNumberEntry,
  WatchingChromeData,
  WatchingProgrammeData,
  WatchingScheduleData,
  WatchingSurfaceProps,
};
