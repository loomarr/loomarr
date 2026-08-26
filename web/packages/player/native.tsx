import type { PairingCredential } from "@loomarr/core/pairing";
import { createAuthenticatedFetch } from "@loomarr/core/pairing";
import { createServerVersionSource } from "@loomarr/core/system-version";
import { createVideoPlayer, type VideoPlayer, VideoView, type VideoViewProps } from "expo-video";
import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import { AppState, Image, type ImageProps } from "react-native";
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
  player: VideoPlayer;
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
  initialTune?: "first" | "none";
  credential: PairingCredential;
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

const createNativePlayerTransport = (player: VideoPlayer): NativePlayerTransport => {
  let disposed = false;
  let activeAttemptId: number | undefined;
  let replacement = Promise.resolve();
  const listeners = new Set<(event: PlayerTransportEvent) => void>();
  const emit = (event: PlayerTransportEvent) => {
    if (disposed) return;
    for (const listener of listeners) listener(event);
  };
  const statusSubscription = player.addListener("statusChange", ({ error, status }) => {
    if (status === "error" && activeAttemptId !== undefined) {
      emit({ attemptId: activeAttemptId, error: error?.message ?? "Native playback failed.", type: "error" });
    }
  });
  const playingSubscription = player.addListener("playingChange", ({ isPlaying }) => {
    if (isPlaying && activeAttemptId !== undefined) emit({ attemptId: activeAttemptId, type: "playing" });
  });

  player.loop = false;
  player.showNowPlayingNotification = false;
  player.staysActiveInBackground = false;
  player.timeUpdateEventInterval = 0.25;

  return {
    dispose: () => {
      if (disposed) return;
      disposed = true;
      player.pause();
      statusSubscription.remove();
      playingSubscription.remove();
      listeners.clear();
      player.release();
    },
    firstFrame: () => {
      if (activeAttemptId !== undefined) emit({ attemptId: activeAttemptId, type: "first-frame" });
    },
    goLive: () => {
      const offset = player.currentOffsetFromLive;
      if (offset !== null && Number.isFinite(offset) && offset > 0) {
        player.seekBy(offset);
      } else if (Number.isFinite(player.duration) && player.duration > 0) {
        player.currentTime = player.duration;
      }
    },
    pause: () => player.pause(),
    play: () => player.play(),
    player,
    replace: async (source: PlayerSource, context: { attemptId: number; signal: AbortSignal }) => {
      const queued = replacement
        .catch(() => undefined)
        .then(async () => {
          if (disposed || context.signal.aborted) return;
          activeAttemptId = context.attemptId;
          await player.replaceAsync({
            contentType: "hls",
            headers: source.headers ? { ...source.headers } : undefined,
            uri: source.uri,
            useCaching: false,
          });
        });
      replacement = queued;
      await queued;
    },
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
};

const createExpoVideoTransport = (): NativePlayerTransport =>
  createNativePlayerTransport(createVideoPlayer(null));

const usePairedNativePlayer = ({
  credential,
  initialTune,
  onRevoked,
  profile = conservativeDeviceProfile,
}: PairedNativePlayerOptions): PairedNativePlayer => {
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

  useEffect(() => {
    void refresh();
    const subscription = AppState.addEventListener("change", (state) => {
      if (state === "active") {
        void refresh().then(() => controller.play());
      } else {
        refreshRequest.current?.abort();
        controller.pause();
      }
    });
    return () => {
      refreshRequest.current?.abort();
      subscription.remove();
      controller.dispose();
    };
  }, [controller, refresh]);

  return { controller, loadError, refresh, serverVersion, snapshot, transport };
};

const NativePlayerView = ({ style, transport }: NativePlayerViewProps) => (
  <VideoView
    allowsPictureInPicture={false}
    allowsVideoFrameAnalysis={false}
    contentFit="contain"
    nativeControls={false}
    onFirstFrameRender={transport.firstFrame}
    player={transport.player}
    startsPictureInPictureAutomatically={false}
    style={style}
    surfaceType="surfaceView"
  />
);

const PairedNativeImage = ({ credential, resizeMode = "cover", style, uri }: PairedNativeImageProps) => {
  const source = pairedNativeImageSource(credential, uri);
  return source ? <Image resizeMode={resizeMode} source={source} style={style} /> : null;
};

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
