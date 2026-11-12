import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import type { ChannelNowNext } from "@loomarr/api/models/channelNowNext";
import type { LivePlaybackState, PlayerFailureCause, PlayerSnapshot, TuneDirection } from "@loomarr/player";
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
  nowNext: ChannelNowNext[];
  /** The player tuned another channel (a step, a typed number, a pick): the URL follows it. */
  onNavigate: (channelId: string) => void;
}

/** What the picture is doing: waiting for a first frame, showing one, or stopped for good. */
type WatchPlayerStatus = "loading" | "playing" | "error";

interface WatchPlayer {
  /** Binds the player to the surface's <video>. Pass to the video surface's `attach`. */
  attach: (video: HTMLVideoElement) => () => void;
  canSurf: boolean;
  /** The channels Channel up and down step through, in that order. */
  catalog: PlayerSnapshot["catalog"];
  /** Changes with every Channel up or down (the channel the step tuned); undefined after any other tune. */
  flipKey?: string;
  /** The channel being watched or tuned, from the lineup; undefined until the player has one. */
  channel?: ChannelDTO;
  /** What is airing on `channel` now. */
  currentTitle?: string;
  error?: string;
  /** With an `error` from the player: what stopped the picture. Absent for a channel the app can't play. */
  failureCause?: PlayerFailureCause;
  /** The tuned channel's rungs this browser plays and the height on air; empty until known. */
  ladder: LiveLadder;
  /** The viewer's live position and the controls that move it. */
  liveTransport: WatchLiveTransport;
  playbackSessionId: string;
  /** The viewer's remembered quality: Auto, or a rung by height. */
  quality: LiveQuality;
  retry: () => void;
  setQuality: (quality: LiveQuality) => void;
  status: WatchPlayerStatus;
  step: (direction: TuneDirection) => void;
  /** The tuning channel's picture, once decoded, to show until its stream's first frame. */
  stillURL?: string;
  tune: (channelId: string) => void;
}

export type { UseWatchPlayerOptions, WatchLiveTransport, WatchPlayer, WatchPlayerStatus };
