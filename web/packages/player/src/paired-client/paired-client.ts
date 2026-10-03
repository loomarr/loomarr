import { ClientDiagnosticsReporter, createAuthenticatedBatchSender } from "@loomarr/core/client-diagnostics";
import { openEventStream } from "@loomarr/core/events";
import { createGuideController, createGuideSourcePort, guideWindow } from "@loomarr/core/guide";
import { createMyChannelsController, createMyChannelsPort } from "@loomarr/core/my-channels";
import { createAuthenticatedFetch } from "@loomarr/core/pairing";
import { createServerVersionSource } from "@loomarr/core/system-version";
import { randomUUID } from "expo-crypto";
import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import { AppState, Image } from "react-native";

import { createCatalogRefresher } from "../catalog-refresher";
import { createExpoVideoTransport } from "../native/native";
import { createNativeEventStreamFactory } from "../native-event-stream";
import { createNativePlaybackDiagnostics } from "../native-playback-diagnostics";
import { createNativePlayerLifecycle } from "../native-player-lifecycle";
import { createChannelCatalogPort, createPlayUrlSourcePort } from "../play-url-source";
import { createPlayerController } from "../player-controller";
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
      playback: createNativePlaybackDiagnostics(reporter, newPlaybackSessionId(diagnostics.platform)),
      reporter,
    };
  }, [diagnostics, request]);
  const controller = useMemo(
    () =>
      createPlayerController({
        onPlayerError: reporting?.playback.playerError,
        onTune: (report) => onTuneRef.current?.(report),
        // The signed still needs no auth header, so the platform image cache can hold it for the overlay.
        prefetchStill: (uri) => void Image.prefetch(uri).catch(() => undefined),
        profile: {},
        source: createPlayUrlSourcePort({ baseUrl: credential.serverUrl, fetch: request }),
        transport,
      }),
    [credential.serverUrl, reporting, request, transport],
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
  const settledChannelId = snapshot.status === "playing" ? channelId : undefined;
  useEffect(() => {
    if (settledChannelId) void myChannels.recordTune(settledChannelId).catch(() => undefined);
  }, [myChannels, settledChannelId]);
  useEffect(() => {
    const createStream = createNativeEventStreamFactory({
      headers: { Authorization: `Bearer ${credential.token}` },
      onUnauthorized: () => session.revoked(),
    });
    let closeStream: (() => void) | undefined;
    const openStream = () => {
      if (closeStream) return;
      closeStream = openEventStream(
        {
          onChannel: () => {
            refresh();
            void guide.refresh();
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
  }, [credential.serverUrl, credential.token, guide, refresh, session]);

  return {
    catalogState,
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

export { usePairedClient };
