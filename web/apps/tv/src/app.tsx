import { ClientDiagnosticsReporter, createAuthenticatedBatchSender } from "@loomarr/core/client-diagnostics";
import { openEventStream } from "@loomarr/core/events";
import { createGuideController, createGuideSourcePort } from "@loomarr/core/guide";
import type { PairingCredential } from "@loomarr/core/pairing";
import {
  createAuthenticatedFetch,
  createPairingCredentialStore,
  createPairingTransport,
  PairingSession,
  validatePairingCredential,
} from "@loomarr/core/pairing";
import { createServerVersionSource } from "@loomarr/core/system-version";
import { BrandLaunch, LoomarrProvider } from "@loomarr/design-system";
import { createNativeServerDiscovery } from "@loomarr/lan-discovery-native";
import {
  createCatalogRefresher,
  createNativePlaybackDiagnostics,
  createPlayerController,
} from "@loomarr/player";
import {
  createExpoVideoTransport,
  createNativeEventStreamFactory,
  createNativePlayerLifecycle,
  NativePlayerView,
  PairedNativeImage,
} from "@loomarr/player/native";
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
import {
  createTvGuideFocusRegistry,
  createTvSurfFocusRegistry,
  initialTvWatchingRemoteState,
  reduceTvWatchingRemote,
  restoreTvSurfSelection,
  type TvWatchingRemoteEvent,
  type TvWatchingRemoteIntent,
  type TvWatchingRemoteState,
  tvGuideRowWindow,
  tvNumberEntryPresentation,
  tvWatchingRemoteEventFromNative,
} from "@loomarr/ui-tv";
import { useKeepAwake } from "expo-keep-awake";
import * as SecureStore from "expo-secure-store";
import * as SplashScreen from "expo-splash-screen";
import { StatusBar } from "expo-status-bar";
import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import { AppState, BackHandler, useTVEventHandler, View } from "react-native";
import { SafeAreaProvider, useSafeAreaInsets } from "react-native-safe-area-context";
import appConfig from "../app.json";

void SplashScreen.preventAutoHideAsync();

const clientVersion = process.env.EXPO_PUBLIC_LOOMARR_CLIENT_VERSION ?? appConfig.expo.version;
const launchMinimumMs = 1_200;

const credentialStore = createPairingCredentialStore({
  deleteItem: SecureStore.deleteItemAsync,
  getItem: SecureStore.getItemAsync,
  setItem: SecureStore.setItemAsync,
});

// Hermes has no crypto.randomUUID; the id only groups one TV session's diagnostics, it is not a secret.
const newPlaybackSessionId = () => `tv-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}`;

type TvPairedRuntime = {
  credential: PairingCredential;
  request: typeof globalThis.fetch;
  session: PairingSession;
};

const TvShell = ({ runtime }: { runtime: TvPairedRuntime }) => {
  const [active, setActive] = useState<ClientDestination>("watching");
  const [controlsActivityKey, setControlsActivityKey] = useState(0);
  const [controlsVisible, setControlsVisible] = useState(true);
  const [serverVersion, setServerVersion] = useState<string>();
  const versionRequest = useRef<AbortController | undefined>(undefined);
  const transport = useMemo(createExpoVideoTransport, []);
  const diagnostics = useMemo(() => {
    const reporter: ClientDiagnosticsReporter = new ClientDiagnosticsReporter(
      createAuthenticatedBatchSender(runtime.request, (events) => reporter.wireBatch(events)),
      { clientVersion, platform: "android_tv", source: "android_tv" },
    );
    return { playback: createNativePlaybackDiagnostics(reporter, newPlaybackSessionId()), reporter };
  }, [runtime.request]);
  const controller = useMemo(
    () =>
      createPlayerController({
        onPlayerError: diagnostics.playback.playerError,
        profile: {},
        source: createPlayUrlSourcePort({
          baseUrl: runtime.credential.serverUrl,
          fetch: runtime.request,
        }),
        transport,
      }),
    [diagnostics, runtime.credential.serverUrl, runtime.request, transport],
  );
  const catalog = useMemo(() => createChannelCatalogPort(runtime.request), [runtime.request]);
  const catalogRefresher = useMemo(
    () => createCatalogRefresher({ controller, list: catalog.list }),
    [catalog, controller],
  );
  const catalogState = useSyncExternalStore(
    catalogRefresher.subscribe,
    catalogRefresher.getState,
    catalogRefresher.getState,
  );
  const version = useMemo(() => createServerVersionSource(runtime.request), [runtime.request]);
  const guide = useMemo(
    () => createGuideController({ source: createGuideSourcePort(runtime.request) }),
    [runtime.request],
  );
  const guideFocusRegistry = useMemo(createTvGuideFocusRegistry, []);
  const surfFocusRegistry = useMemo(createTvSurfFocusRegistry, []);
  const remoteStateRef = useRef<TvWatchingRemoteState>(initialTvWatchingRemoteState);
  const [remoteState, setRemoteState] = useState<TvWatchingRemoteState>(initialTvWatchingRemoteState);
  const snapshot = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  const guideSnapshot = useSyncExternalStore(guide.subscribe, guide.getSnapshot, guide.getSnapshot);
  const loadServerVersion = useCallback(async () => {
    versionRequest.current?.abort();
    const request = new AbortController();
    versionRequest.current = request;
    try {
      const loaded = await version.load(request.signal);
      if (request.signal.aborted) return;
      setServerVersion(loaded);
    } catch {
      if (!request.signal.aborted) setServerVersion(undefined);
    }
  }, [version]);
  const refresh = useCallback(async () => {
    await catalogRefresher.refresh();
    void loadServerVersion();
  }, [catalogRefresher, loadServerVersion]);
  const refreshSafely = useCallback(() => {
    void refresh().catch(() => undefined);
  }, [refresh]);
  const lifecycle = useMemo(
    () => createNativePlayerLifecycle({ controller, refresh, transport }),
    [controller, refresh, transport],
  );
  useEffect(() => {
    if (AppState.currentState === "active") refreshSafely();
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
      controller.dispose();
    };
  }, [catalogRefresher, controller, guide, lifecycle, refreshSafely]);
  useEffect(() => {
    const unsubscribe = transport.subscribe(diagnostics.playback.transportEvent);
    return () => {
      unsubscribe();
      diagnostics.playback.dispose();
      diagnostics.reporter.dispose();
    };
  }, [diagnostics, transport]);
  useEffect(() => {
    const channelId = snapshot.channel?.id;
    diagnostics.playback.channelChanged(channelId);
    if (channelId) void guide.refresh(channelId);
  }, [diagnostics, guide, snapshot.channel?.id]);
  useEffect(() => {
    const createStream = createNativeEventStreamFactory({
      headers: { Authorization: `Bearer ${runtime.credential.token}` },
      onUnauthorized: () => runtime.session.revoked(),
    });
    let closeStream: (() => void) | undefined;
    const openStream = () => {
      if (closeStream) return;
      closeStream = openEventStream(
        {
          onChannel: () => {
            refreshSafely();
            void guide.refresh();
          },
        },
        new URL("/v1/events", runtime.credential.serverUrl).toString(),
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
  }, [guide, refreshSafely, runtime.credential.serverUrl, runtime.credential.token, runtime.session]);
  useEffect(() => {
    const subscription = BackHandler.addEventListener("hardwareBackPress", () => {
      const destination = clientBackDestination(active);
      if (!destination) return false;
      setActive(destination);
      return true;
    });
    return () => subscription.remove();
  }, [active]);
  const runRemoteIntent = useCallback(
    (intent: TvWatchingRemoteIntent | undefined) => {
      switch (intent?.kind) {
        case "step":
          void controller.step(intent.direction);
          break;
        case "tune-number":
          void controller.tuneNumber(intent.digits);
          break;
        case "open-guide":
          setActive("guide");
          break;
        case "open-surf":
          setActive("surf");
          break;
      }
    },
    [controller],
  );
  const showControlsForActivity = useCallback(() => {
    setControlsVisible(true);
    setControlsActivityKey((key) => key + 1);
  }, []);
  const dispatchRemoteEvent = useCallback(
    (event: TvWatchingRemoteEvent) => {
      const result = reduceTvWatchingRemote(remoteStateRef.current, event);
      remoteStateRef.current = result.state;
      setRemoteState(result.state);
      if (result.handled) showControlsForActivity();
      runRemoteIntent(result.intent);
    },
    [runRemoteIntent, showControlsForActivity],
  );
  useTVEventHandler(({ eventKeyAction, eventType }) => {
    if (active !== "watching") return;
    const event = tvWatchingRemoteEventFromNative(eventType, Date.now(), eventKeyAction);
    if (event && event.key !== "select") dispatchRemoteEvent(event);
  });
  useEffect(() => {
    const expiresAtMs = remoteState.numberEntry?.expiresAtMs;
    if (expiresAtMs === undefined || active !== "watching") return;
    const timeout = setTimeout(
      () => dispatchRemoteEvent({ atMs: expiresAtMs, key: "timeout" }),
      Math.max(0, expiresAtMs - Date.now()),
    );
    return () => clearTimeout(timeout);
  }, [active, dispatchRemoteEvent, remoteState.numberEntry?.expiresAtMs]);
  useEffect(() => {
    if (active === "watching" || !remoteStateRef.current.numberEntry) return;
    remoteStateRef.current = initialTvWatchingRemoteState;
    setRemoteState(initialTvWatchingRemoteState);
  }, [active]);
  const schedule = watchingScheduleFromGuide(
    guideSnapshot.layout,
    snapshot.channel?.id,
    snapshot.livePlayback?.viewerTimeMs ?? Date.now(),
  );
  const dismissControls = useCallback(() => setControlsVisible(false), []);
  return (
    <View style={{ flex: 1 }}>
      <WatchingSurface
        chromeVisible={active === "watching"}
        controlsActivityKey={controlsActivityKey}
        controlsVisible={controlsVisible}
        density="tv"
        loading={catalogState.loading}
        loadError={catalogState.error}
        onChannelDown={() => void controller.step(-1)}
        onChannelUp={() => void controller.step(1)}
        onChangeServer={() => runtime.session.chooseServer()}
        onDismissControls={dismissControls}
        onGoLive={() => void controller.goLive()}
        onOpenGuide={() => dispatchRemoteEvent({ key: "select" })}
        onOpenSurf={() => setActive("surf")}
        onPause={controller.pause}
        onPlay={() => void controller.play()}
        onPrevious={() => void controller.previous()}
        onRetry={() => {
          if (catalogState.error) refreshSafely();
          else void controller.retry();
        }}
        onShowControls={showControlsForActivity}
        numberEntry={tvNumberEntryPresentation(remoteState, snapshot.catalog)}
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
              focusRegistry={guideFocusRegistry}
              onTune={(channelId) => {
                void controller.tuneChannel(channelId);
                showControlsForActivity();
                setActive("watching");
              }}
              preferredChannelId={snapshot.channel?.id}
              renderArtwork={(airing) => {
                const uri = airing.source.thumbImage?.src ?? airing.source.thumbUrl;
                return uri ? (
                  <PairedNativeImage
                    credential={runtime.credential}
                    style={{ height: "100%", width: "100%" }}
                    uri={uri}
                  />
                ) : undefined;
              }}
              renderChannelLogo={(channel) =>
                channel.source.logo ? (
                  <PairedNativeImage
                    credential={runtime.credential}
                    resizeMode="contain"
                    style={{ height: "100%", width: "100%" }}
                    uri={channel.source.logo}
                  />
                ) : undefined
              }
            />
          ) : (
            <SurfJourney
              clientVersion={clientVersion}
              controller={guide}
              currentChannelId={snapshot.channel?.id}
              density="tv"
              focusRegistry={surfFocusRegistry}
              onDisconnect={() => runtime.session.disconnect()}
              onForget={() => runtime.session.forgetServer()}
              onTune={(channelId) => {
                void controller.tuneChannel(channelId);
                showControlsForActivity();
                setActive("watching");
              }}
              playableChannelIds={snapshot.catalog.map(({ id }) => id)}
              recentChannelIds={snapshot.recentChannelIds}
              renderArtwork={(channel) =>
                channel.now?.artworkUri ? (
                  <PairedNativeImage
                    credential={runtime.credential}
                    style={{ height: "100%", width: "100%" }}
                    uri={channel.now.artworkUri}
                  />
                ) : undefined
              }
              renderChannelLogo={(channel) =>
                channel.channelLogoUri ? (
                  <PairedNativeImage
                    credential={runtime.credential}
                    resizeMode="contain"
                    style={{ height: "100%", width: "100%" }}
                    uri={channel.channelLogoUri}
                  />
                ) : undefined
              }
              restoreSelection={restoreTvSurfSelection}
              serverName={runtime.credential.serverUrl}
              serverVersion={serverVersion}
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
  const [appForeground, setAppForeground] = useState(AppState.currentState === "active");
  const [launchAnimationFinished, setLaunchAnimationFinished] = useState(false);
  const [launchMinimumElapsed, setLaunchMinimumElapsed] = useState(false);
  const launchStartedAt = useRef(Date.now());
  const nativeSplashHidden = useRef(false);
  const discovery = useMemo(createNativeServerDiscovery, []);
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
  const hideNativeSplash = useCallback(() => {
    if (nativeSplashHidden.current) return;
    nativeSplashHidden.current = true;
    SplashScreen.hide();
  }, []);
  useEffect(() => {
    const remaining = Math.max(0, launchMinimumMs - (Date.now() - launchStartedAt.current));
    const timer = setTimeout(() => setLaunchMinimumElapsed(true), remaining);
    return () => clearTimeout(timer);
  }, []);
  useEffect(() => {
    const subscription = AppState.addEventListener("change", (state) => {
      setAppForeground(state === "active");
    });
    return () => subscription.remove();
  }, []);
  return (
    <LoomarrProvider insets={insets} theme="dark">
      <View onLayout={hideNativeSplash} style={{ flex: 1 }}>
        <PairingShell
          allowServerEntry
          density="tv"
          discovery={discovery}
          discoveryForeground={appForeground}
          initialServerUrl={process.env.EXPO_PUBLIC_LOOMARR_URL}
          renderPaired={(credential) => <TvPairedRoot credential={credential} session={session} />}
          session={session}
        />
        {launchAnimationFinished && launchMinimumElapsed ? null : (
          <View style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}>
            <BrandLaunch density="tv" onFinished={() => setLaunchAnimationFinished(true)} />
          </View>
        )}
      </View>
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
