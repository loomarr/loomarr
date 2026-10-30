import type { PairingCredential } from "@loomarr/core/pairing";
import { createVideoPlayer, type VideoPlayer, VideoView, type VideoViewProps } from "expo-video";
import { useEffect, useState, useSyncExternalStore } from "react";
import { Image, type ImageProps } from "react-native";
import { selectAudioRendition } from "../live-audio";
import { LIVE_DVR_HORIZON_MS, liveSeek } from "../live-seek";
import { nativeErrorCause } from "../native-playback-diagnostics";
import type { LivePlaybackFacts } from "../playback-info";
import { createPlaybackMarks, type PlaybackMarks } from "../playback-marks";
import type {
  LivePlaybackMode,
  LivePlaybackState,
  PlayerTransport,
  PlayerTransportEvent,
} from "../player-controller";
import type { PlayerSource } from "../player-source";
import { GENERIC_START_FAILURE } from "../start-failure";

interface NativePlayerTransport extends PlayerTransport {
  /** Signals the first frame rendered by the native VideoView for the active attempt. */
  firstFrame: () => void;
  getPlayer: () => VideoPlayer | undefined;
  /**
   * What the player is playing now, read on demand (Playback info samples it while open): the
   * decoded picture and what the platform says of its rung, the buffer ahead and the signed stream
   * URL. Undefined before a source is bound.
   */
  playbackFacts: () => LivePlaybackFacts | undefined;
  resume: () => void;
  /**
   * Plays the master's Alternate audio rendition, or its Default (#2088 decision 6). The choice
   * outlives tunes: each new stream plays it as soon as the player lists its renditions.
   */
  setAlternateAudio: (alternate: boolean) => void;
  subscribePlayer: (listener: () => void) => () => void;
  suspend: () => void;
}

interface NativePlayerViewProps {
  style?: VideoViewProps["style"];
  transport: NativePlayerTransport;
}

interface PairedNativeImageProps {
  credential: Pick<PairingCredential, "serverUrl" | "token">;
  resizeMode?: ImageProps["resizeMode"];
  style?: ImageProps["style"];
  uri: string;
}

const pairedNativeImageSource = (
  credential: Pick<PairingCredential, "serverUrl" | "token">,
  rawUrl: string,
): { headers?: { Authorization: string }; uri: string } | undefined => {
  try {
    const uri = new URL(rawUrl, `${credential.serverUrl}/`);
    if (uri.protocol !== "http:" && uri.protocol !== "https:") return undefined;
    if (uri.origin === new URL(credential.serverUrl).origin) {
      return { headers: { Authorization: `Bearer ${credential.token}` }, uri: uri.toString() };
    }
    return uri.protocol === "https:" ? { uri: uri.toString() } : undefined;
  } catch {
    return undefined;
  }
};

/**
 * A stall that leaves a live viewer further behind than this rejoins the live edge: AVPlayer resumes
 * where it froze, and a viewer who didn't pause shouldn't be left watching the past.
 */
const STALL_REJOIN_LAG_SECONDS = 30;

/** Matches the HTTP status ExoPlayer embeds in its own error prose (e.g. "Response code: 502"). */
const NATIVE_5XX_STATUS = /\b5\d{2}\b/;

/**
 * Whether a native player error reported before this attempt's first frame is a server-refused
 * request, rather than a mid-stream drop.
 *
 * The source port's `start` already read the master's answer from JS, so a refused start, with
 * the server's reason, never reaches the player. This covers what the player itself fetches after
 * that (a variant playlist, or a warmed source opened directly). It only reads the status ExoPlayer
 * puts in its message; AVPlayer puts none there ("resource unavailable"), and the transport never
 * re-requests a playlist to find out, because the manifest request is itself a tune.
 */
const isNativeStartFailure = (message: string): boolean => NATIVE_5XX_STATUS.test(message);

/** The codec family a platform's video MIME type names, as Playback info's Codec row reads it. */
const VIDEO_MIME_CODECS: Record<string, string> = { "video/avc": "avc1", "video/hevc": "hvc1" };

const createNativePlayerTransport = (
  initialPlayer: VideoPlayer,
  recreatePlayer?: () => VideoPlayer,
  marks: PlaybackMarks = createPlaybackMarks({ enabled: false }),
): NativePlayerTransport => {
  let disposed = false;
  let activeAttemptId: number | undefined;
  let player: VideoPlayer | undefined;
  let replacement = Promise.resolve();
  let replacing = 0; // replacements queued or running
  const listeners = new Set<(event: PlayerTransportEvent) => void>();
  const playerListeners = new Set<() => void>();
  let playingSubscription: { remove: () => void } | undefined;
  let statusSubscription: { remove: () => void } | undefined;
  let timeSubscription: { remove: () => void } | undefined;
  let trackSubscription: { remove: () => void } | undefined;
  let audioSubscription: { remove: () => void } | undefined;
  let alternateAudio = false;
  let streamUrl: string | undefined;
  // A stall is buffering after the attempt's first frame while the viewer has not paused.
  let framedAttemptId: number | undefined;
  let stalledSinceMs: number | undefined;
  // Set when a stall that began at the live edge ends; the next known position decides the rejoin.
  let rejoinAfterStall = false;
  let stallBeganLive = false;
  let liveMode: LivePlaybackMode = "live";
  let noticeRevision = 0;
  let serverClockOffsetMs: number | undefined;
  let viewerTimeMs = Date.now();

  const liveNow = () => Date.now() + (serverClockOffsetMs ?? 0);

  const emit = (event: PlayerTransportEvent) => {
    if (disposed) return;
    for (const listener of listeners) listener(event);
  };

  const liveState = (
    currentLiveTimestamp: number | null = player?.currentLiveTimestamp ?? null,
    currentOffsetFromLive: number | null = player?.currentOffsetFromLive ?? null,
  ): LivePlaybackState => {
    const now = liveNow();
    if (currentLiveTimestamp !== null && Number.isFinite(currentLiveTimestamp)) {
      viewerTimeMs = currentLiveTimestamp;
    } else if (currentOffsetFromLive !== null && Number.isFinite(currentOffsetFromLive)) {
      viewerTimeMs = now - Math.max(0, currentOffsetFromLive) * 1_000;
    } else if (liveMode === "live") {
      // Expo may omit both values for a live HLS stream without EXT-X-PROGRAM-DATE-TIME.
      // The viewer is still at the live edge, so keeping the tune-time fallback would freeze
      // programme identity and progress across every later schedule boundary.
      viewerTimeMs = now;
    }
    return {
      lagSeconds: liveMode === "live" ? 0 : Math.max(0, Math.round((now - viewerTimeMs) / 1_000)),
      mode: liveMode,
      noticeRevision,
      viewerTimeMs,
    };
  };

  const emitLiveState = (currentLiveTimestamp?: number | null, currentOffsetFromLive?: number | null) => {
    if (activeAttemptId === undefined) return;
    emit({
      attemptId: activeAttemptId,
      state: liveState(currentLiveTimestamp, currentOffsetFromLive),
      type: "live-state",
    });
  };

  const endStall = (why: "error" | "released" | "resumed" | "retuned") => {
    if (stalledSinceMs !== undefined && activeAttemptId !== undefined) {
      marks.stallEnd(activeAttemptId, marks.now() - stalledSinceMs, why);
    }
    rejoinAfterStall = why === "resumed" && stalledSinceMs !== undefined && stallBeganLive;
    stalledSinceMs = undefined;
  };

  const seekToLiveEdge = (current: VideoPlayer) => {
    const offset = current.currentOffsetFromLive;
    if (offset !== null && Number.isFinite(offset) && offset > 0) {
      current.seekBy(offset);
    } else if (Number.isFinite(current.duration) && current.duration > 0) {
      current.currentTime = current.duration;
    }
    liveMode = "live";
    viewerTimeMs = liveNow();
  };

  /** Seconds the displayed frame trails the Channel wall clock, or undefined while the player can't say. */
  const lagBehindLive = (currentLiveTimestamp: number | null, currentOffsetFromLive: number | null) => {
    if (currentLiveTimestamp !== null && Number.isFinite(currentLiveTimestamp)) {
      return (liveNow() - currentLiveTimestamp) / 1_000;
    }
    if (currentOffsetFromLive !== null && Number.isFinite(currentOffsetFromLive))
      return currentOffsetFromLive;
    return undefined;
  };

  /** After a stall the viewer didn't cause, returns to the live edge once the player reports a position. */
  const rejoinLiveAfterStall = (
    currentLiveTimestamp: number | null,
    currentOffsetFromLive: number | null,
  ) => {
    if (!rejoinAfterStall || !player) return;
    if (liveMode !== "live") {
      rejoinAfterStall = false;
      return;
    }
    const lag = lagBehindLive(currentLiveTimestamp, currentOffsetFromLive);
    if (lag === undefined) return;
    rejoinAfterStall = false;
    if (lag > STALL_REJOIN_LAG_SECONDS) seekToLiveEdge(player);
  };

  // The rendition the viewer's choice names, by the master's NAME; a stream that names neither (a
  // direct play) keeps whatever the player picked.
  const playChosenAudio = (current: VideoPlayer) => {
    const renditions = current.availableAudioTracks;
    const chosen = renditions[selectAudioRendition(renditions, alternateAudio)];
    const playing = current.audioTrack;
    if (chosen && (playing?.name ?? playing?.label) !== (chosen.name ?? chosen.label)) {
      current.audioTrack = chosen;
    }
  };

  const attachPlayer = (next: VideoPlayer) => {
    player = next;
    next.loop = false;
    next.showNowPlayingNotification = false;
    next.staysActiveInBackground = false;
    next.timeUpdateEventInterval = 0.25;
    // The channel packager cuts 1 s segments, so one segment is enough to start (or resume after a
    // stall). ExoPlayer's 2 s default makes every tune and surf wait for a second segment. Expo takes
    // the whole object; the fields left out keep their defaults.
    next.bufferOptions = { minBufferForPlayback: 1 };
    statusSubscription = next.addListener("statusChange", ({ error, status }) => {
      if (activeAttemptId === undefined) return;
      if (status === "error") {
        const attemptId = activeAttemptId;
        const fallbackMessage = error?.message ?? "Native playback failed.";
        endStall("error");
        const message =
          framedAttemptId !== attemptId && isNativeStartFailure(fallbackMessage)
            ? GENERIC_START_FAILURE
            : fallbackMessage;
        marks.error(attemptId, nativeErrorCause(fallbackMessage));
        emit({ attemptId, error: message, type: "error" });
      } else if (status === "loading") {
        if (framedAttemptId === activeAttemptId && stalledSinceMs === undefined && liveMode !== "paused") {
          stalledSinceMs = marks.now();
          stallBeganLive = liveMode === "live";
          rejoinAfterStall = false;
          marks.stallStart(activeAttemptId);
          emit({ attemptId: activeAttemptId, type: "stalled" });
        }
      } else if (status === "readyToPlay") {
        endStall("resumed");
      }
    });
    if (marks.enabled) {
      trackSubscription = next.addListener("videoTrackChange", ({ videoTrack }) => {
        if (activeAttemptId === undefined) return;
        marks.format(
          activeAttemptId,
          videoTrack && {
            bitrate: videoTrack.bitrate,
            frameRate: videoTrack.frameRate,
            height: videoTrack.size.height,
            mimeType: videoTrack.mimeType,
            width: videoTrack.size.width,
          },
        );
      });
    }
    audioSubscription = next.addListener("availableAudioTracksChange", () => playChosenAudio(next));
    playingSubscription = next.addListener("playingChange", ({ isPlaying }) => {
      if (!isPlaying || activeAttemptId === undefined) return;
      rejoinLiveAfterStall(next.currentLiveTimestamp, next.currentOffsetFromLive);
      emit({ attemptId: activeAttemptId, type: "playing" });
      emitLiveState();
    });
    timeSubscription = next.addListener("timeUpdate", ({ currentLiveTimestamp, currentOffsetFromLive }) => {
      rejoinLiveAfterStall(currentLiveTimestamp, currentOffsetFromLive);
      emitLiveState(currentLiveTimestamp, currentOffsetFromLive);
    });
  };

  const releasePlayer = () => {
    const current = player;
    if (!current) return;
    current.pause();
    endStall("released");
    statusSubscription?.remove();
    playingSubscription?.remove();
    timeSubscription?.remove();
    trackSubscription?.remove();
    audioSubscription?.remove();
    statusSubscription = undefined;
    playingSubscription = undefined;
    timeSubscription = undefined;
    trackSubscription = undefined;
    audioSubscription = undefined;
    activeAttemptId = undefined;
    player = undefined;
    current.release();
    for (const listener of playerListeners) listener();
  };

  attachPlayer(initialPlayer);

  return {
    dispose: () => {
      if (disposed) return;
      disposed = true;
      releasePlayer();
      listeners.clear();
      playerListeners.clear();
    },
    firstFrame: () => {
      if (activeAttemptId === undefined) return;
      if (framedAttemptId !== activeAttemptId) marks.firstFrame(activeAttemptId);
      framedAttemptId = activeAttemptId;
      emit({ attemptId: activeAttemptId, type: "first-frame" });
    },
    getPlayer: () => player,
    playbackFacts: () => {
      if (!player || streamUrl === undefined) return undefined;
      const track = player.videoTrack;
      return {
        bitrate: track?.bitrate ?? undefined,
        bufferMs: Math.max(0, player.bufferedPosition - player.currentTime) * 1_000,
        codecs: track?.mimeType ? VIDEO_MIME_CODECS[track.mimeType] : undefined,
        frameRate: track?.frameRate ?? undefined,
        height: track?.size.height ?? 0,
        url: streamUrl,
        width: track?.size.width ?? 0,
      };
    },
    goLive: () => {
      if (!player) return;
      seekToLiveEdge(player);
      emitLiveState(null, 0);
      player.play();
    },
    pause: () => {
      if (!player) return;
      const timestamp = player.currentLiveTimestamp;
      const offset = player.currentOffsetFromLive;
      if (timestamp !== null && Number.isFinite(timestamp)) {
        viewerTimeMs = timestamp;
      } else if (offset !== null && Number.isFinite(offset)) {
        viewerTimeMs = liveNow() - Math.max(0, offset) * 1_000;
      }
      liveMode = "paused";
      player.pause();
      if (activeAttemptId !== undefined) emit({ attemptId: activeAttemptId, type: "paused" });
      emitLiveState();
    },
    play: () => {
      if (!player) return;
      if (liveMode === "paused") {
        if (liveNow() - viewerTimeMs >= LIVE_DVR_HORIZON_MS) {
          noticeRevision += 1;
          seekToLiveEdge(player);
          emitLiveState(null, 0);
        } else {
          liveMode = "behind";
          emitLiveState();
        }
      }
      player.play();
    },
    replace: async (source: PlayerSource, context: { attemptId: number; signal: AbortSignal }) => {
      const run = async () => {
        if (disposed || context.signal.aborted) return;
        const current = player;
        if (!current) throw new Error("Native player is unavailable.");
        endStall("retuned");
        activeAttemptId = context.attemptId;
        liveMode = "live";
        noticeRevision = 0;
        serverClockOffsetMs =
          source.serverTimeMs !== undefined && Number.isFinite(source.serverTimeMs)
            ? source.serverTimeMs - Date.now()
            : undefined;
        viewerTimeMs = liveNow();
        streamUrl = source.uri;
        await current.replaceAsync({
          contentType: "hls",
          headers: source.headers ? { ...source.headers } : undefined,
          // A warm read the master already: start from its only variant and skip that fetch (#1037).
          uri: source.mediaUri ?? source.uri,
          useCaching: false,
        });
      };
      // With nothing queued, the native call goes out in this same task, ahead of the switch
      // overlay's render and mount, so the player starts before the main thread is busy (#1037).
      const queued = replacing === 0 ? run() : replacement.catch(() => undefined).then(run);
      replacing += 1;
      replacement = queued;
      try {
        await queued;
      } finally {
        replacing -= 1;
      }
    },
    seek: (wallMs) => {
      if (!player) return;
      const current = player;
      // Refresh the displayed frame's wall time first: the seek is measured from it.
      liveState();
      const offset = current.currentOffsetFromLive;
      const target = liveSeek({
        currentTime: current.currentTime,
        displayedWallMs: viewerTimeMs,
        liveEdge: offset !== null && Number.isFinite(offset) ? current.currentTime + offset : undefined,
        nowMs: liveNow(),
        targetWallMs: wallMs,
      });
      if (target.kind === "live") {
        seekToLiveEdge(current);
        emitLiveState(null, 0);
        current.play();
        return;
      }
      current.currentTime = target.mediaTime;
      // A paused viewer stays paused at the new point; a playing one now watches behind live.
      if (liveMode === "live") liveMode = "behind";
      emitLiveState(target.wallMs, null);
    },
    resume: () => {
      if (disposed || player) return;
      if (!recreatePlayer) throw new Error("Native player cannot resume without a player factory.");
      attachPlayer(recreatePlayer());
      liveMode = "live";
      noticeRevision = 0;
      viewerTimeMs = Date.now();
      for (const listener of playerListeners) listener();
    },
    setAlternateAudio: (alternate) => {
      alternateAudio = alternate;
      if (player) playChosenAudio(player);
    },
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    subscribePlayer: (listener) => {
      playerListeners.add(listener);
      return () => playerListeners.delete(listener);
    },
    suspend: releasePlayer,
  };
};

const createExpoVideoTransport = (marks?: PlaybackMarks): NativePlayerTransport =>
  createNativePlayerTransport(createVideoPlayer(null), () => createVideoPlayer(null), marks);

const NativePlayerView = ({ style, transport }: NativePlayerViewProps) => {
  const player = useSyncExternalStore(transport.subscribePlayer, transport.getPlayer, transport.getPlayer);
  return player ? (
    <VideoView
      allowsPictureInPicture={false}
      allowsVideoFrameAnalysis={false}
      contentFit="contain"
      nativeControls={false}
      onFirstFrameRender={transport.firstFrame}
      player={player}
      startsPictureInPictureAutomatically={false}
      style={style}
      surfaceType="surfaceView"
    />
  ) : null;
};

/**
 * The height of the rendition on the picture, for Settings' `Auto · 720p` (#2127): the platform
 * player picks the rung itself and lets no client choose one. Undefined until a track is known.
 */
const useNativeVideoHeight = (transport: NativePlayerTransport): number | undefined => {
  const player = useSyncExternalStore(transport.subscribePlayer, transport.getPlayer, transport.getPlayer);
  const [height, setHeight] = useState<number>();
  useEffect(() => {
    setHeight(player?.videoTrack?.size.height);
    const subscription = player?.addListener("videoTrackChange", ({ videoTrack }) =>
      setHeight(videoTrack?.size.height),
    );
    return () => subscription?.remove();
  }, [player]);
  return height;
};

const PairedNativeImage = ({ credential, resizeMode = "cover", style, uri }: PairedNativeImageProps) => {
  const source = pairedNativeImageSource(credential, uri);
  return source ? <Image resizeMode={resizeMode} source={source} style={style} /> : null;
};

export type { NativePlayerTransport, NativePlayerViewProps, PairedNativeImageProps };
export {
  createExpoVideoTransport,
  createNativePlayerTransport,
  NativePlayerView,
  PairedNativeImage,
  pairedNativeImageSource,
  useNativeVideoHeight,
};
