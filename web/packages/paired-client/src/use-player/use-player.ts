import { ClientDiagnosticsReporter, createAuthenticatedBatchSender } from "@loomarr/core/client-diagnostics";
import { createCatalogRefresher, createPlayerController } from "@loomarr/player";
import {
  createExpoVideoTransport,
  createNativePlaybackDiagnostics,
  nativeTransportOf,
} from "@loomarr/player/native";
import { createChannelCatalogPort, createPlayUrlSourcePort } from "@loomarr/player/server";
import { randomUUID } from "expo-crypto";
import { useEffect, useMemo, useRef, useSyncExternalStore } from "react";
import { Image } from "react-native";
import { useDisposedWith } from "../use-disposed-with";
import type { PlayerRuntime, UsePlayerOptions } from "./use-player.type";

// Hermes has no crypto.randomUUID, so expo-crypto supplies the platform's secure one.
const newPlaybackSessionId = (platform: string) => `${platform}-${randomUUID()}`;

/**
 * The paired server's player: its transport, the diagnostics reporter that listens to it, the
 * controller and the channel catalog it plays from. It makes each of them and disposes each when it
 * is replaced or the component goes away.
 */
const usePlayer = ({
  credential,
  diagnostics,
  initialTune,
  lastChannel,
  marks,
  onTune,
  request,
}: UsePlayerOptions): PlayerRuntime => {
  const onTuneRef = useRef(onTune);
  onTuneRef.current = onTune;
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
  useDisposedWith(reporting, ({ playback, reporter }) => {
    playback.dispose();
    reporter.dispose();
  });
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
  useDisposedWith(controller, (built) => built.dispose());
  const catalogRefresher = useMemo(
    () => createCatalogRefresher({ controller, list: createChannelCatalogPort(request).list }),
    [controller, request],
  );
  const catalogState = useSyncExternalStore(
    catalogRefresher.subscribe,
    catalogRefresher.getState,
    catalogRefresher.getState,
  );
  const snapshot = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  useEffect(() => {
    if (!reporting) return;
    return transport.subscribe(reporting.playback.transportEvent);
  }, [reporting, transport]);
  const channelId = snapshot.channel?.id;
  useEffect(() => {
    reporting?.playback.channelChanged(channelId);
  }, [channelId, reporting]);

  return { catalogRefresher, catalogState, controller, snapshot, transport };
};

export { usePlayer };
