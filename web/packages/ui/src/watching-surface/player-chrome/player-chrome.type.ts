import type { Density } from "@loomarr/design-system";
import type { ReactNode } from "react";

/** One spot in a break's pod: a commercial, a bumper, a station ID… (§9.3). */
interface PlaybarSpot {
  id: string;
  /** The clip's viewer name, or its kind when the server sent none ("Station ID"). */
  name: string;
  /** `brand · era` for a commercial that has them, else its kind; empty when the name is the kind. */
  detail: string;
  durationMs: number;
  /** "30s", "1:05". */
  durationLabel: string;
  stillUri?: string;
}

interface PlaybarProgramme {
  kind: "programme";
  id: string;
  startMs: number;
  stopMs: number;
  /** The series, or the film's title. */
  name: string;
  /** `S2 E4 "Episode"` for an episode, `1994 · Drama` for a film. */
  line?: string;
  /** S/E, rating and genres, for the preview card. */
  chips: readonly string[];
  synopsis?: string;
  stillUri?: string;
}

interface PlaybarBreak {
  kind: "break";
  id: string;
  startMs: number;
  stopMs: number;
  spots: readonly PlaybarSpot[];
}

type PlaybarSegment = PlaybarBreak | PlaybarProgramme;

/** The playbar's time window: the seekable past behind live, then what's coming (§9.3). */
interface PlaybarWindow {
  startMs: number;
  /** The live edge. */
  nowMs: number;
  endMs: number;
}

/** Where playback sits against the live edge: paused and behind offer Go live. */
interface PlayerLiveState {
  lagSeconds: number;
  mode: "behind" | "live" | "paused";
}

interface PlayerBandData {
  channelName: string;
  channelNumber: string;
  /** `Series · S2 E4 "Episode"`, `Commercials · <clip>`; omitted while nothing is known. */
  programmeLine?: string;
  /** 0–1 through the programme (or the break) airing at the playhead. */
  progress?: number;
  /** `9m in · 13m left`, `2m 14s behind`, `Break · 3 min`. */
  timeLabel?: string;
  /** A break is airing: the progress line takes the break's tint. */
  inBreak?: boolean;
  /** `1990s · Portland, ME`: the station's era and place, read on the station-ID card. */
  station?: string;
}

/** The Info sheet's programme (§9, mock "info"): the block at the playhead and what follows it. */
interface PlayerInfoData {
  /** The band's programme line. */
  title: string;
  /** `1994 · TV-PG · Mystery · Drama`. */
  facts?: string;
  synopsis?: string;
  stillUri?: string;
  startMs: number;
  stopMs: number;
  /** 0–1 through the block. */
  progress: number;
  /** The band's `9m in · 13m left`. */
  timeLabel?: string;
  /** The next programme; breaks are passed over. */
  upNext?: { startMs: number; name: string; line?: string };
}

interface PlayerChromeProps {
  band: PlayerBandData;
  density: Density;
  /** The controls are shown (pointer moved, focus inside, paused…); hidden leaves the picture bare. */
  visible: boolean;
  /**
   * The station-ID card (§9.1): just after a tune settles, the band alone, fuller, while the controls
   * are hidden. The app times it; shown controls take its place.
   */
  ident?: boolean;
  /**
   * The preview playbar's blocks and window; omitted while the schedule hasn't loaded. With `onSeek`
   * the past is seekable: a block that has begun takes the viewer to its start (or the window's).
   */
  playbar?: {
    onSeek?: (wallClockMs: number) => void;
    playheadMs: number;
    segments: readonly PlaybarSegment[];
    window: PlaybarWindow;
  };
  /** Formats a wall-clock instant for the playbar ("8:44 PM"). */
  formatTime: (ms: number) => string;
  /** A preview card opened or closed: the app keeps its controls shown while one is open. */
  onPreviewChange?: (open: boolean) => void;
  transport: PlayerTransportProps;
  /** The open right-hand sheet, drawn over a dimmed picture; omitted when none is open. */
  sheet?: ReactNode;
}

interface PlayerTransportProps {
  density: Density;
  /** Pointer devices get tooltips naming each tool's key. */
  pointer: boolean;
  playing: boolean;
  onTogglePlay: () => void;
  /** ⟲/⟳ 10 s through the window; omitted where the player cannot seek. Forward is off while live. */
  onSeekBack?: () => void;
  onSeekForward?: () => void;
  live: PlayerLiveState;
  onGoLive: () => void;
  /** Back to the programme's start; omitted unless it began inside the window. */
  onStartOver?: () => void;
  muted: boolean;
  /** 0–1. */
  volume: number;
  onToggleMute: () => void;
  onVolume: (volume: number) => void;
  /** Settings (⚙): open or close the settings sheet. */
  settingsOpen: boolean;
  onSettings: () => void;
  /** Info (ⓘ): open or close the info sheet; omitted while there is no programme to describe. */
  infoOpen?: boolean;
  onInfo?: () => void;
  /** Channels: open or close the channels sheet; omitted where there is no channel list. */
  channelsOpen?: boolean;
  onChannels?: () => void;
  /** Picture-in-picture; omitted where the browser has none. */
  pictureInPicture?: boolean;
  onPictureInPicture?: () => void;
  /** Fullscreen; omitted where the platform has none (a phone's app is already full screen). */
  fullscreen?: boolean;
  onFullscreen?: () => void;
  onLeave: () => void;
}

interface PlayerSheetProps {
  /** The channel the sheet is about, beside its title ("CH 7 · Coast 90s"). */
  channelLabel: string;
  children: ReactNode;
  density: Density;
  icon: "info" | "keyboard" | "list" | "settings";
  /** Drawn in the portrait panel's place under the picture, rather than over the picture's right edge. */
  inline?: boolean;
  onClose: () => void;
  title: string;
}

export type {
  PlaybarBreak,
  PlaybarProgramme,
  PlaybarSegment,
  PlaybarSpot,
  PlaybarWindow,
  PlayerBandData,
  PlayerChromeProps,
  PlayerInfoData,
  PlayerLiveState,
  PlayerSheetProps,
  PlayerTransportProps,
};
