import type { PairingCredential } from "@loomarr/core/pairing";
import { createAuthenticatedFetch } from "@loomarr/core/pairing";
import { createVideoPlayer, type VideoPlayer, VideoView, type VideoViewProps } from "expo-video";
import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import { AppState } from "react-native";
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

interface PairedNativePlayerOptions {
  credential: PairingCredential;
  onRevoked: () => Promise<void> | void;
  profile?: DevicePlaybackProfile;
}

interface PairedNativePlayer {
  controller: PlayerController;
  loadError?: string;
  refresh: () => Promise<void>;
  snapshot: PlayerSnapshot;
  transport: NativePlayerTransport;
}

const conservativeDeviceProfile: DevicePlaybackProfile = {};

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
  onRevoked,
  profile = conservativeDeviceProfile,
}: PairedNativePlayerOptions): PairedNativePlayer => {
  const [loadError, setLoadError] = useState<string>();
  const refreshRequest = useRef<AbortController | undefined>(undefined);
  const authenticatedFetch = useMemo(
    () => createAuthenticatedFetch(credential, onRevoked),
    [credential, onRevoked],
  );
  const catalog = useMemo(() => createChannelCatalogPort(authenticatedFetch), [authenticatedFetch]);
  const transport = useMemo(createExpoVideoTransport, []);
  const controller = useMemo(
    () =>
      createPlayerController({
        profile,
        source: createPlayUrlSourcePort({ baseUrl: credential.serverUrl, fetch: authenticatedFetch }),
        transport,
      }),
    [authenticatedFetch, credential.serverUrl, profile, transport],
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
    } catch (error) {
      if (!request.signal.aborted) {
        setLoadError(error instanceof Error ? error.message : "Couldn't load channels.");
      }
    } finally {
      if (refreshRequest.current === request) refreshRequest.current = undefined;
    }
  }, [catalog, controller]);

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

  return { controller, loadError, refresh, snapshot, transport };
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

export type { NativePlayerTransport, NativePlayerViewProps, PairedNativePlayer, PairedNativePlayerOptions };
export { createExpoVideoTransport, createNativePlayerTransport, NativePlayerView, usePairedNativePlayer };
