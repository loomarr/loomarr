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
  /** The programme's fanart (a thumbnail without one): the block's still and its card's hero. */
  stillUri?: string;
  /** The title's clear logo, set on the card's hero, sized by area (§9.2). */
  logo?: PlayerArt;
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
   * back. The band always types it; a title's logo never stands in for it there (§9.2).
   */
  title?: string;
  /** The programme's clear logo, for the preview card's hero; never drawn in the band (§9.2). */
  logo?: PlayerArt;
  /** The programme's fanart, dimmed behind the station-ID card; drawn contained at 16:9, never stretched. */
  backdrop?: PlayerArt;
  /** The programme's rating (`TV-PG`), shown beside Leave while the controls are up. */
  rating?: string;
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
   * (§9.7): the player centres the picture as a strip mid-screen, with a toolbar above it and the band
   * and keys below, nothing over the picture, and always shown. `tv` is the ten-foot overlay: 5% clear
   * at every edge, four keys, no utilities row and no Leave (Back leaves).
   */
  layout?: "overlay" | "portrait" | "tv";
  /** Leave: the back arrow in the top left corner (§9.3). Omitted on a television, where Back leaves. */
  onLeave?: () => void;
  /**
   * What is on and what follows, for the film strip that is the band's foot on desktop and television
   * (§9.2). Omitted on phones (their strip is in More) and while nothing is scheduled.
   */
  schedule?: PlayerSchedule;
  /** The utilities over the keys (§9.3): a toolbar above the picture on an upright phone. */
  utilities: PlayerUtilitiesProps;
  /** Playback details as a panel at the upper left (pointer devices); a phone shows it as a sheet. */
  panel?: ReactNode;
  /** The first play in a session starts muted: the pill that takes one tap to undo it; omitted otherwise. */
  unmute?: () => void;
  /**
   * Digits being typed to tune direct: the readout, on a solid backing, clear of the watermark, with
   * the channel they reach and, on a remote, the auto-tune countdown.
   */
  readout?: { countdown?: string; digits: string; missed?: boolean; name?: string };
  /**
   * The open menu (§9.5, variant J): on desktop and television, what a key opens sits in the control
   * cluster, right over the keys, with the picture whole. Omitted when none is open.
   */
  menu?: ReactNode;
  /**
   * The open sheet: the right-hand sheet over a dimmed picture on a phone on its side, the bottom
   * sheet (picture undimmed, so a choice shows as it is made) on one held upright. Omitted when none
   * is open.
   */
  sheet?: ReactNode;
  /**
   * The channel's watermark, burned into the picture (§9.6): the chrome keeps out of its box. Omitted
   * when the channel has none, so nothing moves.
   */
  watermark?: PlayerWatermark;
}

/**
 * The keys (§9.3): Last channel, Channel down, Play, Channel up, Volume (desktop) and More. A
 * television (density `tv`) has four: Last channel, Subtitles, Audio and More; the remote has the rest.
 */
interface PlayerKeysProps {
  density: Density;
  /** Pointer devices get a legend under a key while it is pointed at or focused. */
  pointer: boolean;
  /** Play / Pause; omitted on a television, whose remote has the button (the key is then not drawn). */
  playing?: boolean;
  onTogglePlay?: () => void;
  /** The channel Last returns to; omitted until there is one (the key is then not drawn). */
  last?: { number: string; onPress: () => void };
  /** The Channel up / down rocker; omitted where the player cannot tune. */
  onChannelUp?: () => void;
  onChannelDown?: () => void;
  /**
   * Volume, on desktop: it opens the level in the menu's place (§9.5). Omitted on a phone and a
   * television, whose own buttons set it (the key is then not drawn).
   */
  volume?: { muted: boolean; onPress: () => void; open: boolean };
  /** More: open or close the More sheet. Omitted where the player has nothing for More to open. */
  moreOpen?: boolean;
  onMore?: () => void;
}

/**
 * The utilities (§9.3): a smaller row of icon keys over the main keys. Subtitles and Audio, then
 * Fullscreen where a window can fill the screen, or AirPlay and picture in picture on an iPhone. A key
 * whose handler is omitted is not drawn. On a television Subtitles and Audio are main keys instead.
 */
interface PlayerUtilitiesProps {
  density: Density;
  pointer: boolean;
  /** Audio: open or close what Audio opens; omitted when there is nothing to choose. */
  audioOpen?: boolean;
  onAudio?: () => void;
  /** Subtitles; omitted until the player has any (the key is then not drawn). */
  subtitlesOn?: boolean;
  subtitlesOpen?: boolean;
  onSubtitles?: () => void;
  /** Fullscreen; omitted where the platform has none (a phone's app is already full screen). */
  fullscreen?: boolean;
  onFullscreen?: () => void;
  /** AirPlay (iPhone): the system's route picker. */
  onAirPlay?: () => void;
  /** Picture in picture, where the player is a phone app that has a key for it. */
  onPictureInPicture?: () => void;
}

/** The film strip's data (§9.2): the timeline it draws and where the viewer is in it. */
interface PlayerSchedule {
  segments: readonly PlaybarSegment[];
  window: PlaybarWindow;
  /** The wall-clock instant at the playhead. */
  playheadMs: number;
  /** Seek to an instant in the past; without it the strip only previews. */
  onSeek?: (wallClockMs: number) => void;
  /**
   * Pressing a block that does not seek. A television passes the Guide here: OK opens the Guide on
   * Watching (playback-clients), and the strip is where focus rests while the controls are up.
   */
  onSelect?: () => void;
  /**
   * Info is open (§9.5, variant J): on desktop and television it is the block on air's own card,
   * pinned open in full. A phone has no strip, so its Info is a sheet.
   */
  info?: boolean;
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
  PlayerSchedule,
  PlayerSheetProps,
  PlayerUtilitiesProps,
};
