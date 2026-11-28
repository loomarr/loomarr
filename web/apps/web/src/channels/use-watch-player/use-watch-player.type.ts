import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import type { LivePlaybackState, PlayerFailureCause, TuneReason } from "@loomarr/player";
import type { LiveLadder, LiveQuality } from "@loomarr/player/browser";

/**
 * The viewer's live position and what moves it: pause stays paused until play, goLive rejoins the
 * edge, and seek moves to a wall-clock instant inside the window.
 */
interface WatchLiveTransport {
  state: LivePlaybackState;
  play: () => Promise<void> | void;
  pause: () => void;
  goLive: () => Promise<void> | void;
  seek: (wallClockMs: number) => Promise<void> | void;
}

interface UseWatchPlayerOptions {
  /** The channel the URL names. */
  channelId: string;
  /** Every channel the server lists; undefined until it has been read. */
  channels: ChannelDTO[] | undefined;
  /** The player tuned another channel (a step, a typed number, a pick): the URL follows it. */
  onNavigate: (channelId: string) => void;
  /**
   * A tune began: its attempt, its channel, why, and whether it reuses a warmed neighbour's source.
   * Said as the tune starts, before the call that asked for it returns.
   */
  onTune?: (report: WatchTuneReport) => void;
}

/** A tune as it begins. `attemptId` is the one `WatchPlayer.attemptId` names while it plays. */
interface WatchTuneReport {
  attemptId: number;
  channelId: string;
  /** Why the player tuned: `retry` is a reconnect, by the player itself or the viewer's Retry. */
  reason: TuneReason;
  warm: boolean;
}

/** What the picture is doing: waiting for a first frame, showing one, or stopped for good. */
type WatchPlayerStatus = "loading" | "playing" | "error";

interface WatchPlayer {
  /** Binds the player to the surface's <video>. Pass to the video surface's `attach`. */
  attach: (video: HTMLVideoElement) => () => void;
  /** The tune attempt `status` is about; undefined before the first tune. */
  attemptId?: number;
  /** The channel being watched or tuned, from the lineup; undefined until the player has one. */
  channel?: ChannelDTO;
  error?: string;
  /** With an `error` from the player: what stopped the picture. Absent for a channel the app can't play. */
  failureCause?: PlayerFailureCause;
  /** The tuned channel's rungs this browser plays and the height on air; empty until known. */
  ladder: LiveLadder;
  /** The viewer's live position and the controls that move it. */
  liveTransport: WatchLiveTransport;
  /** The controller holds the picture paused, whoever asked (`status` reads a paused picture as playing). */
  paused: boolean;
  playbackSessionId: string;
  /** The viewer's remembered quality: Auto, or a rung by height. */
  quality: LiveQuality;
  retry: () => void;
  setQuality: (quality: LiveQuality) => void;
  status: WatchPlayerStatus;
  tune: (channelId: string) => void;
}

export type { UseWatchPlayerOptions, WatchLiveTransport, WatchPlayer, WatchPlayerStatus, WatchTuneReport };
