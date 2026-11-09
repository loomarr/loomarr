import type { Density } from "@loomarr/design-system";
import type { ReactNode } from "react";

import type { PlayerArt } from "./art";
import type { PlayerWatermark } from "./watermark-clearance";

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
  /** The channel's era decade (1980 for the 1980s): it colours the channel block. Absent is amber. */
  decade?: number;
  /** The wall-clock instant at the playhead, for the band's clock. */
  clockMs?: number;
  /**
   * The programme's title: the series, or the film's. In a break it is the programme that comes
   * back. It is the logo's text fallback and the logo's accessible name (§9.2).
   */
  title?: string;
  /** The programme's clear logo; without one the title is set as text (§9.2). */
  logo?: PlayerArt;
  /** The programme's fanart, dimmed behind the station-ID card; drawn contained at 16:9, never stretched. */
  backdrop?: PlayerArt;
  /** `S2 E4 "Episode"` for an episode, `1994 · Drama` for a film. */
  line?: string;
  /** The programme's own time slot, for the progress line's clocks. */
  startMs?: number;
  stopMs?: number;
  /** 0–1 through the programme's time slot at the playhead. */
  progress?: number;
  /** 0–1 where live is in that slot, set only while the playhead is behind it (the red tick). */
  liveProgress?: number;
  /** `2m 14s behind`, set while the playhead is behind live. */
  behindLabel?: string;
  /** A break is airing: the line reads `Back in n min` in the break colour. */
  inBreak?: boolean;
  breakMinutesLeft?: number;
  /** The next programme; breaks are passed over. Next stays text. */
  next?: { name: string; startMs: number };
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
  /** Formats a wall-clock instant for the band's clocks ("8:44 PM"). */
  formatTime: (ms: number) => string;
  /** Where playback sits against the live edge: behind offers Go live in place of Live. */
  live: PlayerLiveState;
  onGoLive: () => void;
  keys: PlayerKeysProps;
  /**
   * `overlay` draws over the picture and auto-hides (§9.1). `portrait` is the phone held upright
   * (§9.7): the top row above the picture, the band and keys below it, nothing over the picture, and
   * always shown. The app pins the picture at `PORTRAIT_PICTURE_TOP`.
   */
  layout?: "overlay" | "portrait";
  top: PlayerTopRowProps;
  /** Playback details as a panel at the upper left (pointer devices); a phone shows it as a sheet. */
  panel?: ReactNode;
  /** The first play in a session starts muted: the pill that takes one tap to undo it; omitted otherwise. */
  unmute?: () => void;
  /** Digits being typed to tune direct: the readout, on a solid backing, clear of the watermark. */
  readout?: { digits: string; missed?: boolean };
  /** The open right-hand sheet, drawn over a dimmed picture; omitted when none is open. */
  sheet?: ReactNode;
  /**
   * The channel's watermark, burned into the picture (§9.6): the chrome keeps out of its box. Omitted
   * when the channel has none, so nothing moves.
   */
  watermark?: PlayerWatermark;
}

/** The keys (§9.3): the same on desktop and phone. */
interface PlayerKeysProps {
  density: Density;
  /** Pointer devices get a legend under each key, Volume, and tooltips naming each key. */
  pointer: boolean;
  playing: boolean;
  onTogglePlay: () => void;
  /** The channel Last returns to; omitted until there is one (the key is then not drawn). */
  last?: { number: string; onPress: () => void };
  /** The Channel up / down rocker; omitted where the player cannot tune. */
  onChannelUp?: () => void;
  onChannelDown?: () => void;
  muted: boolean;
  /** 0–1. */
  volume: number;
  onToggleMute: () => void;
  /** More: open or close the More sheet. */
  moreOpen: boolean;
  onMore: () => void;
}

/** The top row, upper right (§9.3): Subtitles, Audio, then Fullscreen (pointer) and Leave. */
interface PlayerTopRowProps {
  density: Density;
  pointer: boolean;
  /** Audio: open or close the audio sheet; omitted when there is nothing to choose. */
  audioOpen?: boolean;
  onAudio?: () => void;
  /** Subtitles; omitted until the player has any (the key is then not drawn). */
  subtitlesOn?: boolean;
  subtitlesOpen?: boolean;
  onSubtitles?: () => void;
  /** Fullscreen; omitted where the platform has none (a phone's app is already full screen). */
  fullscreen?: boolean;
  onFullscreen?: () => void;
  onLeave: () => void;
}

interface ClipTransportProps {
  density: Density;
  /** Pointer devices get tooltips naming each tool's key. */
  pointer: boolean;
  playing: boolean;
  onTogglePlay: () => void;
  muted: boolean;
  /** 0–1. */
  volume: number;
  onToggleMute: () => void;
  onVolume: (volume: number) => void;
  elapsedMs: number;
  /** Omitted until the file's length is known; the elapsed readout waits for it. */
  durationMs?: number;
  fullscreen: boolean;
  onFullscreen: () => void;
}

interface ClipChromeProps extends ClipTransportProps {
  /** The player's auto-hide: hidden chrome is transparent and out of reach of pointer and AT. */
  shown: boolean;
  title?: string;
  /** A caller's control at the top-left of the frame (a dialog's close button). */
  leading?: ReactNode;
}

interface PlayerSheetProps {
  /** The channel the sheet is about: its number fills the header's amber block, its name sits over the title. */
  channel: { name: string; number: string };
  children: ReactNode;
  density: Density;
  onClose: () => void;
  title: string;
}

export type {
  ClipChromeProps,
  ClipTransportProps,
  PlaybarBreak,
  PlaybarProgramme,
  PlaybarSegment,
  PlaybarSpot,
  PlaybarWindow,
  PlayerArt,
  PlayerBandData,
  PlayerChromeProps,
  PlayerInfoData,
  PlayerKeysProps,
  PlayerLiveState,
  PlayerSheetProps,
  PlayerTopRowProps,
};
