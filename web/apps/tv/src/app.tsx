import type { PairingCredential } from "@loomarr/core/pairing";
import {
  createPairingCredentialStore,
  createPairingTransport,
  PairingSession,
  validatePairingCredential,
} from "@loomarr/core/pairing";
import { BrandLaunch, LoomarrProvider } from "@loomarr/design-system";
import { createNativeServerDiscovery } from "@loomarr/lan-discovery-native";
import {
  createPlaybackMarks,
  NativePlayerView,
  PairedNativeImage,
  usePairedClient,
} from "@loomarr/player/native";
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
  createTvAutoTuneSetting,
  createTvGuideFocusRegistry,
  createTvSurfFocusRegistry,
  DEFAULT_NUMBER_ENTRY_MS,
  initialTvWatchingRemoteState,
  reduceTvWatchingRemote,
  restoreTvSurfSelection,
  type TvWatchingRemoteEvent,
  type TvWatchingRemoteIntent,
  type TvWatchingRemoteState,
  tvGuideRowWindow,
  tvGuideTimeEdge,
  tvNumberEntryPresentation,
  tvWatchingRemoteEventFromNative,
} from "@loomarr/ui-tv";
import { useKeepAwake } from "expo-keep-awake";
import * as SecureStore from "expo-secure-store";
import * as SplashScreen from "expo-splash-screen";
import { StatusBar } from "expo-status-bar";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AppState, BackHandler, useTVEventHandler, View } from "react-native";
import { SafeAreaProvider, useSafeAreaInsets } from "react-native-safe-area-context";
import appConfig from "../app.json";

void SplashScreen.preventAutoHideAsync();

const clientVersion = process.env.EXPO_PUBLIC_LOOMARR_CLIENT_VERSION ?? appConfig.expo.version;
const launchMinimumMs = 1_200;

// Shield certification marks (#1037, docs/engineering/shield-certification.md): always on in a
// development build; a release build writes them only when bundled with EXPO_PUBLIC_LOOMARR_CERT_MARKS=1.
const certMarks = createPlaybackMarks({
  enabled: __DEV__ || process.env.EXPO_PUBLIC_LOOMARR_CERT_MARKS === "1",
});
/** A remote key older than this did not cause the tune (a guide pick, a retry, the boot tune). */
const keyToTuneMaxMs = 1_500;

const credentialStore = createPairingCredentialStore({
  deleteItem: SecureStore.deleteItemAsync,
  getItem: SecureStore.getItemAsync,
  setItem: SecureStore.setItemAsync,
});

// The per-device auto-tune duration (#1659 decision N4, WCAG 2.2.1). No picker writes it yet: its
// placement awaits a mock, so every device reads the 1.2 s default until one does.
const autoTuneSetting = createTvAutoTuneSetting({
  getItem: SecureStore.getItemAsync,
  setItem: SecureStore.setItemAsync,
});

const tvDiagnostics = { clientVersion, platform: "android_tv", source: "android_tv" } as const;

const TvShell = ({ credential, session }: { credential: PairingCredential; session: PairingSession }) => {
  const [active, setActive] = useState<ClientDestination>("watching");
  const [controlsActivityKey, setControlsActivityKey] = useState(0);
  const [controlsVisible, setControlsVisible] = useState(true);
  const lastKeyAtMs = useRef<number | undefined>(undefined);
  const {
    catalogState,
    controller,
    guide,
    guideSnapshot,
    myChannelsSnapshot,
    refresh,
    serverVersion,
    snapshot,
    transport,
  } = usePairedClient({
    credential,
    diagnostics: tvDiagnostics,
    marks: certMarks,
    onTune: ({ attemptId, channel, reason, warm }) => {
      const keyAtMs = lastKeyAtMs.current;
      lastKeyAtMs.current = undefined;
      certMarks.tune({
        attemptId,
        channelId: channel.id,
        channelNumber: channel.number,
        keyAtMs: keyAtMs !== undefined && certMarks.now() - keyAtMs <= keyToTuneMaxMs ? keyAtMs : undefined,
        reason,
        warm,
      });
    },
    session,
  });
  const guideFocusRegistry = useMemo(createTvGuideFocusRegistry, []);
  const surfFocusRegistry = useMemo(createTvSurfFocusRegistry, []);
  const remoteStateRef = useRef<TvWatchingRemoteState>(initialTvWatchingRemoteState);
  const [remoteState, setRemoteState] = useState<TvWatchingRemoteState>(initialTvWatchingRemoteState);
  const autoTuneMs = useRef(DEFAULT_NUMBER_ENTRY_MS);
  useEffect(() => {
    let current = true;
    void autoTuneSetting
      .load()
      .then((durationMs) => {
        if (current) autoTuneMs.current = durationMs;
      })
      // An unreadable store keeps the default rather than leaving digit entry without a timer.
      .catch(() => undefined);
    return () => {
      current = false;
    };
  }, []);
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
      if (certMarks.enabled && event.key !== "timeout") lastKeyAtMs.current = certMarks.now();
      const result = reduceTvWatchingRemote(remoteStateRef.current, event, autoTuneMs.current);
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
  const markSwitchShown = useCallback(
    (what: "osd" | "still") => certMarks.held(controller.getSnapshot().attemptId, what),
    [controller],
  );
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
        onChangeServer={() => session.chooseServer()}
        onDismissControls={dismissControls}
        onGoLive={() => void controller.goLive()}
        onOpenGuide={() => dispatchRemoteEvent({ key: "select" })}
        onOpenSurf={() => setActive("surf")}
        onPause={controller.pause}
        onPlay={() => void controller.play()}
        onPrevious={() => void controller.previous()}
        onRetry={() => {
          if (catalogState.error) refresh();
          else void controller.retry();
        }}
        onShowControls={showControlsForActivity}
        onSwitchShown={certMarks.enabled ? markSwitchShown : undefined}
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
              myChannels={myChannelsSnapshot}
              onTimeEdge={(side) => void tvGuideTimeEdge(guide, guideFocusRegistry, side, Date.now())}
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
                    credential={credential}
                    style={{ height: "100%", width: "100%" }}
                    uri={uri}
                  />
                ) : undefined;
              }}
              renderChannelLogo={(channel) =>
                channel.source.logo ? (
                  <PairedNativeImage
                    credential={credential}
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
              onDisconnect={() => session.disconnect()}
              onForget={() => session.forgetServer()}
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
                    credential={credential}
                    style={{ height: "100%", width: "100%" }}
                    uri={channel.now.artworkUri}
                  />
                ) : undefined
              }
              renderChannelLogo={(channel) =>
                channel.channelLogoUri ? (
                  <PairedNativeImage
                    credential={credential}
                    resizeMode="contain"
                    style={{ height: "100%", width: "100%" }}
                    uri={channel.channelLogoUri}
                  />
                ) : undefined
              }
              restoreSelection={restoreTvSurfSelection}
              serverName={credential.serverUrl}
              serverVersion={serverVersion}
            />
          )}
        </View>
      )}
    </View>
  );
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
          renderPaired={(credential) => (
            <TvShell credential={credential} key={credential.token} session={session} />
          )}
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
