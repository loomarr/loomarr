import type { ClientObservation } from "@loomarr/core/client-diagnostics";
import type Hls from "hls.js";
import type { LiveClock, LivePosition } from "../../live-clock";
import type { PlayerTransport, PlayerTransportEvent } from "../../player-controller";
import type { LiveLadder, LiveQuality } from "../live-level";
import type { DisplayedClockAnchor } from "../media-element";

type BrowserTunePhase = "first-frame" | "manifest";

interface BrowserTransportOptions {
  /**
   * The channel being tuned, for diagnostics. Read when an observation is recorded, which is after the
   * controller has published the attempt's snapshot.
   */
  channelId: () => string | undefined;
  /** The tuned channel's ladder changed: read from its manifest, a level switch, or a new tune. */
  onLadder?: (ladder: LiveLadder) => void;
  /** An attempt reached a phase of its tune, for timing. */
  onPhase?: (attemptId: number, phase: BrowserTunePhase) => void;
  recordDiagnostic: (observation: ClientObservation) => void;
}

interface BrowserTransport extends PlayerTransport {
  /**
   * Binds the transport to the page's <video>. A replacement requested before the element exists
   * waits for it. The returned release detaches the element and frees every hls.js controller.
   */
  attach: (video: HTMLVideoElement) => () => void;
  /** Names this player's session in client diagnostics. */
  playbackSessionId: string;
  /**
   * The viewer's quality (#2088 decision 7): Auto, or a rung by height, played when the master
   * offers it. A change switches the playing level at once and never re-binds the source.
   */
  setQuality: (quality: LiveQuality) => void;
}

/**
 * A channel change keeps the transport and its <video> mounted. The active controller transfers its
 * MediaSource and compatible SourceBuffers to a fresh, unused standby. Controllers are source-scoped:
 * once the replacement paints, the detached old active is destroyed and a new standby is constructed
 * off the critical path. Only two are live; release destroys both.
 */
interface HlsPool {
  instance?: Hls;
  standby?: Hls;
  standbyFresh?: boolean;
}

/** The source on air: the controller playing it, the clock anchor reading its frames, and its keepalive. */
interface ActiveSource {
  anchor?: DisplayedClockAnchor;
  hls?: Hls;
  lastKeepaliveMs: number;
  sourceURL?: string;
}

/** What the tune in flight has reached, shared by the element's event handlers. */
interface PlaybackFlags {
  framedAttemptId?: number;
  stalled: boolean;
}

/** One tune's first decoded frame: whether it has painted, and what to run once it does. */
interface TuneFrame {
  painted: boolean;
  replenish?: () => void;
}

/** The transport's shared state and actions, handed to the modules that bind one source. */
interface TransportHost {
  active: ActiveSource;
  channelId: () => string | undefined;
  clock: LiveClock;
  emit: (event: PlayerTransportEvent) => void;
  emitLive: () => void;
  expireToLive: (el: HTMLVideoElement) => void;
  flags: PlaybackFlags;
  onLadder?: BrowserTransportOptions["onLadder"];
  onPhase?: BrowserTransportOptions["onPhase"];
  playbackSessionId: string;
  pool: HlsPool;
  position: () => LivePosition;
  quality: () => LiveQuality;
  recordDiagnostic: BrowserTransportOptions["recordDiagnostic"];
  rejoinAfterStall: () => void;
  resetActive: () => void;
}

export type {
  ActiveSource,
  BrowserTransport,
  BrowserTransportOptions,
  BrowserTunePhase,
  HlsPool,
  PlaybackFlags,
  TransportHost,
  TuneFrame,
};
