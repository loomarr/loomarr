import { createGuideController, createGuideSourcePort } from "@loomarr/core/guide";
import type { PairingCredential } from "@loomarr/core/pairing";
import {
  createAuthenticatedFetch,
  createPairingCredentialStore,
  createPairingTransport,
  PairingSession,
  validatePairingCredential,
} from "@loomarr/core/pairing";
import { LoomarrProvider } from "@loomarr/design-system";
import { createPlayerController } from "@loomarr/player";
import { createExpoVideoTransport, NativePlayerView } from "@loomarr/player/native";
import { createChannelCatalogPort, createPlayUrlSourcePort } from "@loomarr/player/server";
import type { ClientDestination } from "@loomarr/ui";
import {
  clientBackDestination,
  GuideJourney,
  PairingShell,
  SurfJourney,
  WatchingSurface,
  watchingScheduleFromGuide,
} from "@loomarr/ui";
import { tvGuideRowWindow } from "@loomarr/ui-tv";
import { useKeepAwake } from "expo-keep-awake";
import * as SecureStore from "expo-secure-store";
import { StatusBar } from "expo-status-bar";
import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import { BackHandler, View } from "react-native";
import { SafeAreaProvider, useSafeAreaInsets } from "react-native-safe-area-context";

const credentialStore = createPairingCredentialStore({
  deleteItem: SecureStore.deleteItemAsync,
  getItem: SecureStore.getItemAsync,
  setItem: SecureStore.setItemAsync,
});

type TvPairedRuntime = {
  credential: PairingCredential;
  request: typeof globalThis.fetch;
  session: PairingSession;
};

const TvShell = ({ runtime }: { runtime: TvPairedRuntime }) => {
  const [active, setActive] = useState<ClientDestination>("watching");
  const [catalogLoading, setCatalogLoading] = useState(true);
  const [controlsVisible, setControlsVisible] = useState(true);
  const [loadError, setLoadError] = useState<string>();
  const refreshRequest = useRef<AbortController | undefined>(undefined);
  const transport = useMemo(createExpoVideoTransport, []);
  const controller = useMemo(
    () =>
      createPlayerController({
        profile: {},
        source: createPlayUrlSourcePort({
          baseUrl: runtime.credential.serverUrl,
          fetch: runtime.request,
        }),
        transport,
      }),
    [runtime.credential.serverUrl, runtime.request, transport],
  );
  const catalog = useMemo(() => createChannelCatalogPort(runtime.request), [runtime.request]);
  const guide = useMemo(
    () => createGuideController({ source: createGuideSourcePort(runtime.request) }),
    [runtime.request],
  );
  const snapshot = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  const guideSnapshot = useSyncExternalStore(guide.subscribe, guide.getSnapshot, guide.getSnapshot);
  const refresh = useCallback(async () => {
    refreshRequest.current?.abort();
    const request = new AbortController();
    refreshRequest.current = request;
    setCatalogLoading(true);
    setLoadError(undefined);
    try {
      await controller.reconcile(await catalog.list(request.signal));
    } catch (error) {
      if (!request.signal.aborted) {
        setLoadError(error instanceof Error ? error.message : "Couldn't load channels.");
      }
    } finally {
      if (refreshRequest.current === request) {
        refreshRequest.current = undefined;
        if (!request.signal.aborted) setCatalogLoading(false);
      }
    }
  }, [catalog, controller]);
  useEffect(() => {
    void refresh();
    return () => {
      refreshRequest.current?.abort();
      guide.dispose();
      controller.dispose();
    };
  }, [controller, guide, refresh]);
  useEffect(() => {
    const channelId = snapshot.channel?.id;
    if (channelId) void guide.refresh(channelId);
  }, [guide, snapshot.channel?.id]);
  useEffect(() => {
    const subscription = BackHandler.addEventListener("hardwareBackPress", () => {
      const destination = clientBackDestination(active);
      if (!destination) return false;
      setActive(destination);
      return true;
    });
    return () => subscription.remove();
  }, [active]);
  const schedule = watchingScheduleFromGuide(
    guideSnapshot.layout,
    snapshot.channel?.id,
    snapshot.livePlayback?.viewerTimeMs ?? Date.now(),
  );
  return (
    <View style={{ flex: 1 }}>
      <WatchingSurface
        chromeVisible={active === "watching"}
        controlsVisible={controlsVisible}
        density="tv"
        loading={catalogLoading}
        loadError={loadError}
        onChannelDown={() => void controller.step(-1)}
        onChannelUp={() => void controller.step(1)}
        onDismissControls={() => setControlsVisible(false)}
        onGoLive={() => void controller.goLive()}
        onOpenGuide={() => setActive("guide")}
        onOpenSurf={() => setActive("surf")}
        onPause={controller.pause}
        onPlay={() => void controller.play()}
        onPrevious={() => void controller.previous()}
        onRetry={() => void (loadError ? refresh() : controller.retry())}
        onShowControls={() => setControlsVisible(true)}
        player={<NativePlayerView style={{ flex: 1 }} transport={transport} />}
        schedule={schedule}
        snapshot={snapshot}
      />
      {active === "watching" ? null : (
        <View style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}>
          {active === "guide" ? (
            <GuideJourney
              channelWindow={(layout, selection) =>
                tvGuideRowWindow(
                  layout.channels.length,
                  Math.max(
                    0,
                    layout.channels.findIndex((channel) => channel.source.channelId === selection.channelId),
                  ),
                  8,
                )
              }
              controller={guide}
              density="tv"
              onTune={(channelId) => {
                void controller.tuneChannel(channelId);
                setActive("watching");
              }}
              preferredChannelId={snapshot.channel?.id}
            />
          ) : (
            <SurfJourney
              clientVersion="prototype"
              controller={guide}
              currentChannelId={snapshot.channel?.id}
              density="tv"
              onTune={(channelId) => {
                void controller.tuneChannel(channelId);
                setActive("watching");
              }}
              playableChannelIds={snapshot.catalog.map(({ id }) => id)}
              recentChannelIds={snapshot.recentChannelIds}
            />
          )}
        </View>
      )}
    </View>
  );
};

const TvPairedRoot = ({
  credential,
  session,
}: {
  credential: PairingCredential;
  session: PairingSession;
}) => {
  const onRevoked = useCallback(() => session.revoked(), [session]);
  const runtime = useMemo<TvPairedRuntime>(
    () => ({ credential, request: createAuthenticatedFetch(credential, onRevoked), session }),
    [credential, onRevoked, session],
  );
  return <TvShell key={credential.token} runtime={runtime} />;
};

const TvClient = () => {
  useKeepAwake();
  const insets = useSafeAreaInsets();
  const session = useMemo(
    () =>
      new PairingSession({
        createTransport: createPairingTransport,
        deviceName: "Loomarr TV",
        store: credentialStore,
        validateCredential: validatePairingCredential,
      }),
    [],
  );
  return (
    <LoomarrProvider insets={insets} theme="dark">
      <PairingShell
        density="tv"
        initialServerUrl={process.env.EXPO_PUBLIC_LOOMARR_URL}
        renderPaired={(credential) => <TvPairedRoot credential={credential} session={session} />}
        session={session}
      />
      <StatusBar hidden />
    </LoomarrProvider>
  );
};

const App = () => (
  <SafeAreaProvider>
    <TvClient />
  </SafeAreaProvider>
);

export default App;
