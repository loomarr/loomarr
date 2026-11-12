import { createChannelAudioSource } from "@loomarr/core/channel-audio";
import { createChannelSubtitlesSource } from "@loomarr/core/channel-subtitles";
import { ClientDiagnosticsReporter, createAuthenticatedBatchSender } from "@loomarr/core/client-diagnostics";
import { openEventStream } from "@loomarr/core/events";
import { createGuideController, createGuideSourcePort, guideWindow } from "@loomarr/core/guide";
import { createMyChannelsController, createMyChannelsPort } from "@loomarr/core/my-channels";
import { createAuthenticatedFetch } from "@loomarr/core/pairing";
import { createServerVersionSource } from "@loomarr/core/system-version";
import { createCatalogRefresher, createPlayerController } from "@loomarr/player";
import {
  createExpoVideoTransport,
  createNativeEventStreamFactory,
  createNativePlaybackDiagnostics,
  createNativePlayerLifecycle,
  nativeTransportOf,
} from "@loomarr/player/native";
import { createChannelCatalogPort, createPlayUrlSourcePort } from "@loomarr/player/server";
import { randomUUID } from "expo-crypto";
import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import { AppState, Image } from "react-native";

import type { PairedClient, PairedClientOptions } from "./paired-client.type";

// Hermes has no crypto.randomUUID, so expo-crypto supplies the platform's secure one.
const newPlaybackSessionId = (platform: string) => `${platform}-${randomUUID()}`;

/**
 * The paired server's runtime for a native client: the player and its catalog, the guide, the
 * viewer's favourites and recents, the server's version, the live event stream, and the app's
 * foreground and background. The host keys it by the credential's token, so a new pairing starts
 * a new runtime.
 */
const usePairedClient = ({
  credential,
  diagnostics,
  guideWindowMinutes,
  initialTune,
  lastChannel,
  marks,
  onTune,
  session,
}: PairedClientOptions): PairedClient => {
  const onTuneRef = useRef(onTune);
  onTuneRef.current = onTune;
  const [serverVersion, setServerVersion] = useState<string>();
  const versionRequest = useRef<AbortController | undefined>(undefined);
  const request = useMemo(
    () => createAuthenticatedFetch(credential, () => session.revoked()),
    [credential, session],
  );
  const transport = useMemo(() => createExpoVideoTransport(marks), [marks]);
  const reporting = useMemo(() => {
    if (!diagnostics) return undefined;
    const reporter: ClientDiagnosticsReporter = new ClientDiagnosticsReporter(
      createAuthenticatedBatchSender(request, (events) => reporter.wireBatch(events)),
      diagnostics,
    );
    return {
      playback: createNativePlaybackDiagnostics(
        reporter,
        newPlaybackSessionId(diagnostics.platform),
        nativeTransportOf(diagnostics.platform),
      ),
      reporter,
    };
  }, [diagnostics, request]);
  const controller = useMemo(
    () =>
      createPlayerController({
        initialTune,
        onPlayerError: reporting?.playback.playerError,
        onTune: (report) => onTuneRef.current?.(report),
        // The signed still needs no auth header, so the platform image cache can hold it for the overlay.
        prefetchStill: (uri) => void Image.prefetch(uri).catch(() => undefined),
        restoreChannelId: lastChannel?.load,
        source: createPlayUrlSourcePort({ baseUrl: credential.serverUrl, fetch: request }),
        transport,
      }),
    [credential.serverUrl, initialTune, lastChannel, reporting, request, transport],
  );
  const catalogRefresher = useMemo(
    () => createCatalogRefresher({ controller, list: createChannelCatalogPort(request).list }),
    [controller, request],
  );
  const catalogState = useSyncExternalStore(
    catalogRefresher.subscribe,
    catalogRefresher.getState,
    catalogRefresher.getState,
  );
  const version = useMemo(() => createServerVersionSource(request), [request]);
  const channelAudio = useMemo(() => createChannelAudioSource(request), [request]);
  const channelSubtitles = useMemo(() => createChannelSubtitlesSource(request), [request]);
  const guide = useMemo(
    () =>
      createGuideController({
        resolveWindow: guideWindowMinutes
          ? (at) =>
              guideWindow({
                at,
                dayOffset: 0,
                hourShift: 0,
                startHour: null,
                windowMinutes: guideWindowMinutes,
              })
          : undefined,
        source: createGuideSourcePort(request),
      }),
    [guideWindowMinutes, request],
  );
  const myChannels = useMemo(
    () => createMyChannelsController({ port: createMyChannelsPort(request) }),
    [request],
  );
  const snapshot = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  const guideSnapshot = useSyncExternalStore(guide.subscribe, guide.getSnapshot, guide.getSnapshot);
  const myChannelsSnapshot = useSyncExternalStore(
    myChannels.subscribe,
    myChannels.getSnapshot,
    myChannels.getSnapshot,
  );
  const loadServerVersion = useCallback(async () => {
    versionRequest.current?.abort();
    const pending = new AbortController();
    versionRequest.current = pending;
    try {
      const loaded = await version.load(pending.signal);
      if (pending.signal.aborted) return;
      setServerVersion(loaded);
    } catch {
      if (!pending.signal.aborted) setServerVersion(undefined);
    }
  }, [version]);
  const reload = useCallback(async () => {
    await catalogRefresher.refresh();
    void loadServerVersion();
    void myChannels.refresh();
  }, [catalogRefresher, loadServerVersion, myChannels]);
  const refresh = useCallback(() => {
    void reload().catch(() => undefined);
  }, [reload]);
  const lifecycle = useMemo(
    () => createNativePlayerLifecycle({ controller, refresh: reload, transport }),
    [controller, reload, transport],
  );
  useEffect(() => {
    if (AppState.currentState === "active") refresh();
    else lifecycle.enterBackground();
    const subscription = AppState.addEventListener("change", (state) => {
      if (state === "active") {
        void lifecycle.enterForeground().catch(() => undefined);
      } else {
        catalogRefresher.abort();
        versionRequest.current?.abort();
        lifecycle.enterBackground();
      }
    });
    return () => {
      catalogRefresher.abort();
      versionRequest.current?.abort();
      subscription.remove();
      guide.dispose();
      myChannels.dispose();
      controller.dispose();
    };
  }, [catalogRefresher, controller, guide, lifecycle, myChannels, refresh]);
  useEffect(() => {
    if (!reporting) return;
    const unsubscribe = transport.subscribe(reporting.playback.transportEvent);
    return () => {
      unsubscribe();
      reporting.playback.dispose();
      reporting.reporter.dispose();
    };
  }, [reporting, transport]);
  const channelId = snapshot.channel?.id;
  useEffect(() => {
    reporting?.playback.channelChanged(channelId);
    if (channelId) void guide.refresh(channelId);
  }, [channelId, guide, reporting]);
  // A tune that settles (its first frame plays) joins the viewer's recents (#1666), once per tune,
  // as the web Watch page records it; a warmed neighbour never plays, so it never counts.
  // Pausing and resuming settles the same channel again; only a tune to another channel counts.
  const settledChannelId = snapshot.status === "playing" ? channelId : undefined;
  const recordedChannelId = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (!settledChannelId || settledChannelId === recordedChannelId.current) return;
    recordedChannelId.current = settledChannelId;
    void myChannels.recordTune(settledChannelId).catch(() => undefined);
    // An unwritable store only means the next launch starts on the first channel.
    void lastChannel?.save(settledChannelId).catch(() => undefined);
  }, [lastChannel, myChannels, settledChannelId]);
  useEffect(() => {
    const createStream = createNativeEventStreamFactory({
      headers: { Authorization: `Bearer ${credential.token}` },
      onUnauthorized: () => session.revoked(),
    });
    let closeStream: (() => void) | undefined;
    const reload = () => {
      refresh();
      void guide.refresh();
    };
    const openStream = () => {
      if (closeStream) return;
      // Frames sent while the stream was down are gone, so each time it comes back the catalog
      // and guide load again. Its first open needs none: they load as it opens, on launch and on
      // every return to the foreground.
      let opens = 0;
      closeStream = openEventStream(
        { onChannel: reload },
        new URL("/v1/events", credential.serverUrl).toString(),
        createStream,
        () => {
          if (opens++ > 0) reload();
        },
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
  }, [credential.serverUrl, credential.token, guide, refresh, session]);

  return {
    catalogState,
    channelAudio,
    channelSubtitles,
    controller,
    guide,
    guideSnapshot,
    myChannels,
    myChannelsSnapshot,
    refresh,
    serverVersion,
    snapshot,
    transport,
  };
};

/**
 * How the phone runs the paired client: it opens on its Guide with no picture, so nothing tunes until
 * the viewer picks a channel, and its Guide loads the next 2 hours, as the web phone does.
 */
const phoneClientOptions = { guideWindowMinutes: 120, initialTune: "none" } as const satisfies Pick<
  PairedClientOptions,
  "guideWindowMinutes" | "initialTune"
>;

/**
 * The "Try again" of a playback failure: a catalog that never loaded loads again; any other failure
 * re-tunes the channel.
 */
const retryPairedClient = ({
  catalogState,
  controller,
  refresh,
}: Pick<PairedClient, "catalogState" | "controller" | "refresh">) => {
  if (catalogState.error) refresh();
  else void controller.retry();
};

export { phoneClientOptions, retryPairedClient, usePairedClient };
