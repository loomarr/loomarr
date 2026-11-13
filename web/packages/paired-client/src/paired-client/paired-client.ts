import { createChannelAudioSource } from "@loomarr/core/channel-audio";
import { createChannelSubtitlesSource } from "@loomarr/core/channel-subtitles";
import { createAuthenticatedFetch } from "@loomarr/core/pairing";
import { createServerVersionSource } from "@loomarr/core/system-version";
import { createNativePlayerLifecycle } from "@loomarr/player/native";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AppState } from "react-native";
import { useEventStream } from "../use-event-stream";
import { useGuide } from "../use-guide";
import { useMyChannels } from "../use-my-channels";
import { usePlayer } from "../use-player";
import type { PairedClient, PairedClientOptions } from "./paired-client.type";

/**
 * The paired server's runtime for a native client: the player and its catalog, the guide, the
 * viewer's favourites and recents, the server's version, the live event stream, and the app's
 * foreground and background. The host keys it by the credential's token, so a new pairing starts
 * a new runtime. Each object is made and disposed by its own hook (`usePlayer`, `useGuide`,
 * `useMyChannels`, `useEventStream`); this one joins them to the request they share and to the app's
 * lifecycle.
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
  const [serverVersion, setServerVersion] = useState<string>();
  const versionRequest = useRef<AbortController | undefined>(undefined);
  const request = useMemo(
    () => createAuthenticatedFetch(credential, () => session.revoked()),
    [credential, session],
  );
  const { catalogRefresher, catalogState, controller, snapshot, transport } = usePlayer({
    credential,
    diagnostics,
    initialTune,
    lastChannel,
    marks,
    onTune,
    request,
  });
  const channelId = snapshot.channel?.id;
  const { guide, guideSnapshot } = useGuide({ channelId, guideWindowMinutes, request });
  const settledChannelId = snapshot.status === "playing" ? channelId : undefined;
  const { myChannels, myChannelsSnapshot } = useMyChannels({ lastChannel, request, settledChannelId });
  const version = useMemo(() => createServerVersionSource(request), [request]);
  const channelAudio = useMemo(() => createChannelAudioSource(request), [request]);
  const channelSubtitles = useMemo(() => createChannelSubtitlesSource(request), [request]);
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
  // The three loads run together and each settles on its own, so one that fails (the catalog, say)
  // never skips the others. The catalog's failure is still the caller's: it is the one the player's
  // state shows, and the one a return to the foreground waits on.
  const reload = useCallback(async () => {
    const [catalog] = await Promise.allSettled([
      catalogRefresher.refresh(),
      loadServerVersion(),
      myChannels.refresh(),
    ]);
    if (catalog.status === "rejected") throw catalog.reason;
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
    };
  }, [catalogRefresher, lifecycle, refresh]);
  const reloadAll = useCallback(() => {
    refresh();
    void guide.refresh();
  }, [guide, refresh]);
  useEventStream({ credential, onReload: reloadAll, session });

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
