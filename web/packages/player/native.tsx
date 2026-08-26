import type { ClientDiagnosticsReporter } from "@loomarr/core/client-diagnostics";
import { openEventStream } from "@loomarr/core/events";
import type { PairingCredential } from "@loomarr/core/pairing";
import { createAuthenticatedFetch } from "@loomarr/core/pairing";
import { createServerVersionSource } from "@loomarr/core/system-version";
import { createVideoPlayer, type VideoPlayer, VideoView, type VideoViewProps } from "expo-video";
import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import { AppState, Image, type ImageProps } from "react-native";
import { createNativeEventStreamFactory } from "./src/native-event-stream";
import { createNativePlaybackDiagnostics } from "./src/native-playback-diagnostics";
import { createNativePlayerLifecycle } from "./src/native-player-lifecycle";
import { createChannelCatalogPort, createPlayUrlSourcePort } from "./src/play-url-source";
import {
  createPlayerController,
  type DevicePlaybackProfile,
  type PlayerController,
  type PlayerSnapshot,
  type PlayerSource,
  type PlayerTransport,
  type PlayerTransportEvent,
} from "./src/player-controller";

interface NativePlayerTransport extends PlayerTransport {
  firstFrame: () => void;
  getPlayer: () => VideoPlayer | undefined;
  resume: () => void;
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

interface PairedNativePlayerOptions {
  diagnostics?: ClientDiagnosticsReporter;
  initialTune?: "first" | "none";
  credential: PairingCredential;
  onChannelEvent?: () => Promise<void> | void;
  onRevoked: () => Promise<void> | void;
  profile?: DevicePlaybackProfile;
}

interface PairedNativePlayer {
  controller: PlayerController;
  loadError?: string;
  refresh: () => Promise<void>;
  serverVersion?: string;
  snapshot: PlayerSnapshot;
  transport: NativePlayerTransport;
}

const conservativeDeviceProfile: DevicePlaybackProfile = {};
let playbackSessionSequence = 0;

const pairedNativeImageSource = (
  credential: Pick<PairingCredential, "serverUrl" | "token">,
  rawUrl: string,
): { headers?: { Authorization: string }; uri: string } | undefined => {
  try {
    const uri = new URL(
      rawUrl.startsWith("/") ? `${credential.serverUrl}${rawUrl}` : rawUrl,
      `${credential.serverUrl}/`,
    );
    if (uri.protocol !== "http:" && uri.protocol !== "https:") return undefined;
    if (uri.origin === new URL(credential.serverUrl).origin) {
      return { headers: { Authorization: `Bearer ${credential.token}` }, uri: uri.toString() };
    }
    return uri.protocol === "https:" ? { uri: uri.toString() } : undefined;
  } catch {
    return undefined;
  }
};

const createNativePlayerTransport = (
  initialPlayer: VideoPlayer,
  recreatePlayer?: () => VideoPlayer,
): NativePlayerTransport => {
  let disposed = false;
  let activeAttemptId: number | undefined;
  let player: VideoPlayer | undefined;
  let replacement = Promise.resolve();
  const listeners = new Set<(event: PlayerTransportEvent) => void>();
  const playerListeners = new Set<() => void>();
  let playingSubscription: { remove: () => void } | undefined;
  let statusSubscription: { remove: () => void } | undefined;
  const emit = (event: PlayerTransportEvent) => {
    if (disposed) return;
    for (const listener of listeners) listener(event);
  };
  const publishPlayer = () => {
    for (const listener of playerListeners) listener();
  };
  const attachPlayer = (next: VideoPlayer) => {
    player = next;
    next.loop = false;
    next.showNowPlayingNotification = false;
    next.staysActiveInBackground = false;
    next.timeUpdateEventInterval = 0.25;
    statusSubscription = next.addListener("statusChange", ({ error, status }) => {
      if (status === "error" && activeAttemptId !== undefined) {
        emit({
          attemptId: activeAttemptId,
          error: error?.message ?? "Native playback failed.",
          type: "error",
        });
      }
    });
    playingSubscription = next.addListener("playingChange", ({ isPlaying }) => {
      if (isPlaying && activeAttemptId !== undefined) emit({ attemptId: activeAttemptId, type: "playing" });
    });
  };
  const releasePlayer = () => {
    const current = player;
    if (!current) return;
    current.pause();
    statusSubscription?.remove();
    playingSubscription?.remove();
    statusSubscription = undefined;
    playingSubscription = undefined;
    activeAttemptId = undefined;
    player = undefined;
    current.release();
    publishPlayer();
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
      if (activeAttemptId !== undefined) emit({ attemptId: activeAttemptId, type: "first-frame" });
    },
    getPlayer: () => player,
    goLive: () => {
      if (!player) return;
      const offset = player.currentOffsetFromLive;
      if (offset !== null && Number.isFinite(offset) && offset > 0) {
        player.seekBy(offset);
      } else if (Number.isFinite(player.duration) && player.duration > 0) {
        player.currentTime = player.duration;
      }
    },
    pause: () => player?.pause(),
    play: () => player?.play(),
    replace: async (source: PlayerSource, context: { attemptId: number; signal: AbortSignal }) => {
      const queued = replacement
        .catch(() => undefined)
        .then(async () => {
          if (disposed || context.signal.aborted) return;
          const current = player;
          if (!current) throw new Error("Native player is suspended.");
          activeAttemptId = context.attemptId;
          await current.replaceAsync({
            contentType: "hls",
            headers: source.headers ? { ...source.headers } : undefined,
            uri: source.uri,
            useCaching: false,
          });
        });
      replacement = queued;
      await queued;
    },
    resume: () => {
      if (disposed || player) return;
      if (!recreatePlayer) throw new Error("Native player cannot resume without a player factory.");
      attachPlayer(recreatePlayer());
      publishPlayer();
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

const createExpoVideoTransport = (): NativePlayerTransport =>
  createNativePlayerTransport(createVideoPlayer(null), () => createVideoPlayer(null));

const usePairedNativePlayer = ({
  credential,
  diagnostics,
  initialTune,
  onChannelEvent,
  onRevoked,
  profile = conservativeDeviceProfile,
}: PairedNativePlayerOptions): PairedNativePlayer => {
  const playbackSessionId = useMemo(() => {
    playbackSessionSequence += 1;
    return `native-${Date.now()}-${playbackSessionSequence}`;
  }, []);
  const [loadError, setLoadError] = useState<string>();
  const [serverVersion, setServerVersion] = useState<string>();
  const refreshRequest = useRef<AbortController | undefined>(undefined);
  const authenticatedFetch = useMemo(
    () => createAuthenticatedFetch(credential, onRevoked),
    [credential, onRevoked],
  );
  const catalog = useMemo(() => createChannelCatalogPort(authenticatedFetch), [authenticatedFetch]);
  const version = useMemo(() => createServerVersionSource(authenticatedFetch), [authenticatedFetch]);
  const transport = useMemo(createExpoVideoTransport, []);
  const playbackDiagnostics = useMemo(
    () => (diagnostics ? createNativePlaybackDiagnostics(diagnostics, playbackSessionId) : undefined),
    [diagnostics, playbackSessionId],
  );
  const controller = useMemo(
    () =>
      createPlayerController({
        initialTune,
        profile,
        source: createPlayUrlSourcePort({ baseUrl: credential.serverUrl, fetch: authenticatedFetch }),
        transport,
      }),
    [authenticatedFetch, credential.serverUrl, initialTune, profile, transport],
  );
  const snapshot = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  const refresh = useCallback(async () => {
    refreshRequest.current?.abort();
    const request = new AbortController();
    refreshRequest.current = request;
    try {
      const channels = await catalog.list(request.signal);
      setLoadError(undefined);
      await controller.reconcile(channels);
      try {
        setServerVersion(await version.load(request.signal));
      } catch {
        if (!request.signal.aborted) setServerVersion(undefined);
      }
    } catch (error) {
      if (!request.signal.aborted) {
        setLoadError(error instanceof Error ? error.message : "Couldn't load channels.");
      }
    } finally {
      if (refreshRequest.current === request) refreshRequest.current = undefined;
    }
  }, [catalog, controller, version]);
  const lifecycle = useMemo(
    () => createNativePlayerLifecycle({ controller, refresh, transport }),
    [controller, refresh, transport],
  );

  useEffect(() => {
    if (AppState.currentState === "active") void refresh();
    else lifecycle.enterBackground();
    const subscription = AppState.addEventListener("change", (state) => {
      if (state === "active") {
        void lifecycle.enterForeground();
      } else {
        refreshRequest.current?.abort();
        lifecycle.enterBackground();
      }
    });
    return () => {
      refreshRequest.current?.abort();
      subscription.remove();
      controller.dispose();
    };
  }, [controller, lifecycle, refresh]);

  useEffect(() => {
    const createStream = createNativeEventStreamFactory({
      headers: { Authorization: `Bearer ${credential.token}` },
      onUnauthorized: onRevoked,
    });
    let closeStream: (() => void) | undefined;
    const openStream = () => {
      if (closeStream) return;
      closeStream = openEventStream(
        {
          onChannel: () => {
            void refresh();
            void onChannelEvent?.();
          },
        },
        new URL("/v1/events", credential.serverUrl).toString(),
        createStream,
      );
    };
    const closeActiveStream = () => {
      closeStream?.();
      closeStream = undefined;
    };
    if (AppState.currentState === "active") openStream();
    const subscription = AppState.addEventListener("change", (state) => {
      if (state === "active") openStream();
      else closeActiveStream();
    });
    return () => {
      subscription.remove();
      closeActiveStream();
    };
  }, [credential.serverUrl, credential.token, onChannelEvent, onRevoked, refresh]);

  useEffect(() => {
    playbackDiagnostics?.channelChanged(snapshot.channel?.id);
  }, [playbackDiagnostics, snapshot.channel?.id]);

  useEffect(() => {
    if (!playbackDiagnostics) return;
    return transport.subscribe(playbackDiagnostics.transportEvent);
  }, [playbackDiagnostics, transport]);

  useEffect(() => () => playbackDiagnostics?.dispose(), [playbackDiagnostics]);

  return { controller, loadError, refresh, serverVersion, snapshot, transport };
};

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

const PairedNativeImage = ({ credential, resizeMode = "cover", style, uri }: PairedNativeImageProps) => {
  const source = pairedNativeImageSource(credential, uri);
  return source ? <Image resizeMode={resizeMode} source={source} style={style} /> : null;
};

export type {
  NativeEventRequest,
  NativeEventStreamOptions,
  ParsedEventFrame,
} from "./src/native-event-stream";
export {
  createNativeEventStream,
  createNativeEventStreamFactory,
  parseEventFrames,
} from "./src/native-event-stream";
export type { NativeDiagnosticsRecorder, NativePlaybackDiagnostics } from "./src/native-playback-diagnostics";
export { createNativePlaybackDiagnostics } from "./src/native-playback-diagnostics";
export type {
  NativeLifecycleTransport,
  NativePlayerLifecycle,
  NativePlayerLifecycleOptions,
} from "./src/native-player-lifecycle";
export { createNativePlayerLifecycle } from "./src/native-player-lifecycle";
export type {
  NativePlayerTransport,
  NativePlayerViewProps,
  PairedNativeImageProps,
  PairedNativePlayer,
  PairedNativePlayerOptions,
};
export {
  createExpoVideoTransport,
  createNativePlayerTransport,
  NativePlayerView,
  PairedNativeImage,
  pairedNativeImageSource,
  usePairedNativePlayer,
};
