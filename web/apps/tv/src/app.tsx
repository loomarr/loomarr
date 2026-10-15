import type { PairingCredential } from "@loomarr/core/pairing";
import {
  createPairingCredentialStore,
  createPairingTransport,
  PairingSession,
  validatePairingCredential,
} from "@loomarr/core/pairing";
import { BrandLaunch, LoomarrProvider, Surface, Text } from "@loomarr/design-system";
import { createNativeServerDiscovery } from "@loomarr/lan-discovery-native";
import {
  createLastChannelStore,
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
  pairingBackAvailable,
  SurfJourney,
  WatchingSurface,
  watchingScheduleFromGuide,
  watchingScheduleTimeMs,
} from "@loomarr/ui";
import {
  createTvAutoTuneSetting,
  createTvGuideFocusRegistry,
  createTvSurfFocusRegistry,
  DEFAULT_NUMBER_ENTRY_MS,
  initialTvWatchingRemoteState,
  isTvLongSelect,
  reduceTvWatchingRemote,
  restoreTvSurfSelection,
  TV_AUTO_TUNE_CHOICES_MS,
  type TvWatchingRemoteEvent,
  type TvWatchingRemoteIntent,
  type TvWatchingRemoteState,
  tvFavouriteToggle,
  tvGuideFavouriteChannel,
  tvGuideRowWindow,
  tvGuideTimeEdge,
  tvNumberEntryPresentation,
  tvPressIsLongSelectRelease,
  tvSurfFavouriteChannel,
  tvWatchingRemoteEventFromNative,
} from "@loomarr/ui-tv";
import { useKeepAwake } from "expo-keep-awake";
import * as SecureStore from "expo-secure-store";
import * as SplashScreen from "expo-splash-screen";
import { StatusBar } from "expo-status-bar";
import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import {
  AccessibilityInfo,
  AppState,
  BackHandler,
  Platform,
  TVEventControl,
  useTVEventHandler,
  View,
} from "react-native";
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
/** How long the Favorites confirmation stays up after a long-press of Select. */
const favouriteNoteMs = 2_500;

const credentialStore = createPairingCredentialStore({
  deleteItem: SecureStore.deleteItemAsync,
  getItem: SecureStore.getItemAsync,
  setItem: SecureStore.setItemAsync,
});

// The per-device auto-tune duration (#1659 decision N4, WCAG 2.2.1), chosen in the Surf rail.
const autoTuneSetting = createTvAutoTuneSetting({
  getItem: SecureStore.getItemAsync,
  setItem: SecureStore.setItemAsync,
});

// The channel last watched (#23): a relaunch resumes it instead of the first channel.
const lastChannel = createLastChannelStore({
  getItem: SecureStore.getItemAsync,
  setItem: SecureStore.setItemAsync,
});

const tvDiagnostics = { clientVersion, platform: "android_tv", source: "android_tv" } as const;

// The Siri Remote has no number keys, so Apple TV has no digit entry: no "0–9 jump" hint and no
// channel-number wait to tune (#1500 audit #3; maintainer, 2026-10-13: "It doesn't need tuning").
const remoteHasDigits = !Platform.isTVOS;

/**
 * tvOS gives the Menu key to the app only while it is enabled, and then nothing else acts on it, so
 * enable it only while a screen has somewhere to go back to. Everywhere else Menu goes to the Home
 * screen, as Apple's HIG expects at an app's root. A modal dialog takes Menu itself. Android TV's
 * Back always reaches BackHandler.
 */
const useTvMenuKey = (enabled: boolean) => {
  useEffect(() => {
    if (!enabled || !Platform.isTVOS) return undefined;
    TVEventControl.enableTVMenuKey();
    return () => TVEventControl.disableTVMenuKey();
  }, [enabled]);
};

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
    myChannels,
    myChannelsSnapshot,
    refresh,
    serverVersion,
    snapshot,
    transport,
  } = usePairedClient({
    credential,
    diagnostics: tvDiagnostics,
    lastChannel,
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
  const [autoTuneMs, setAutoTuneMs] = useState<number>(DEFAULT_NUMBER_ENTRY_MS);
  // A choice made before the stored value finishes loading wins over that stale read.
  const autoTuneChosen = useRef(false);
  useEffect(() => {
    let current = true;
    void autoTuneSetting
      .load()
      .then((durationMs) => {
        if (current && !autoTuneChosen.current) setAutoTuneMs(durationMs);
      })
      // An unreadable store keeps the default rather than leaving digit entry without a timer.
      .catch(() => undefined);
    return () => {
      current = false;
    };
  }, []);
  const changeAutoTune = useCallback((durationMs: number) => {
    autoTuneChosen.current = true;
    setAutoTuneMs(durationMs);
    // The choice applies now; an unwritable store just means the next launch reads the default.
    void autoTuneSetting.save(durationMs).catch(() => undefined);
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
  useTvMenuKey(clientBackDestination(active) !== null);
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
        case "toggle-pause": {
          // The Watching controls' own transport: Play when paused, Pause otherwise.
          const { channel, status } = controller.getSnapshot();
          if (!channel) break;
          if (status === "paused") void controller.play();
          else controller.pause();
          break;
        }
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
      const result = reduceTvWatchingRemote(remoteStateRef.current, event, autoTuneMs);
      remoteStateRef.current = result.state;
      setRemoteState(result.state);
      if (result.handled) showControlsForActivity();
      runRemoteIntent(result.intent);
    },
    [autoTuneMs, runRemoteIntent, showControlsForActivity],
  );
  useTVEventHandler(({ eventKeyAction, eventType }) => {
    if (active !== "watching") return;
    const event = tvWatchingRemoteEventFromNative(eventType, Date.now(), eventKeyAction);
    if (event && event.key !== "select") dispatchRemoteEvent(event);
  });
  // A long-press of Select on a Guide or Surf row stars or unstars that row's channel (a short
  // press still tunes). The controller shows the change at once and puts it back if the server
  // refuses, so the note reads what the lists say afterwards rather than what was asked for.
  const [favouriteNote, setFavouriteNote] = useState<string | undefined>(undefined);
  const favouriteNoteTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => () => clearTimeout(favouriteNoteTimer.current), []);
  const toggleFavourite = useCallback(
    async (channelId: string) => {
      const { starred } = tvFavouriteToggle(channelId, myChannels.getSnapshot().favouriteIds);
      await myChannels.setFavourite(channelId, starred);
      const applied = myChannels.getSnapshot().favouriteIds.includes(channelId) === starred;
      const name = snapshot.catalog.find(({ id }) => id === channelId)?.name;
      const note = applied
        ? `${starred ? "Added to" : "Removed from"} Favorites${name ? ` · ${name}` : ""}`
        : "Couldn't update Favorites";
      setFavouriteNote(note);
      void AccessibilityInfo.announceForAccessibility(note);
      clearTimeout(favouriteNoteTimer.current);
      favouriteNoteTimer.current = setTimeout(() => setFavouriteNote(undefined), favouriteNoteMs);
    },
    [myChannels, snapshot.catalog],
  );
  // tvOS also delivers the held row's own press when the finger lifts; it is not a tune request.
  const longSelectAtMs = useRef<number | undefined>(undefined);
  const tuneFromList = (channelId: string) => {
    if (tvPressIsLongSelectRelease(longSelectAtMs.current, Date.now())) {
      longSelectAtMs.current = undefined;
      return;
    }
    void controller.tuneChannel(channelId);
    showControlsForActivity();
    setActive("watching");
  };
  useTVEventHandler(({ eventKeyAction, eventType }) => {
    if (eventType === "longSelect") longSelectAtMs.current = Date.now();
    if (active === "watching" || !isTvLongSelect(eventType, eventKeyAction)) return;
    const guideSelection = guide.getSnapshot().selection;
    const channelId =
      active === "guide"
        ? tvGuideFavouriteChannel(
            guideFocusRegistry.current() ??
              (guideSelection ? { kind: "airing", selection: guideSelection } : undefined),
          )
        : tvSurfFavouriteChannel(surfFocusRegistry.current());
    if (channelId) void toggleFavourite(channelId);
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
  const [clockMs, setClockMs] = useState(Date.now);
  useEffect(() => {
    const timer = setInterval(() => setClockMs(Date.now()), 30_000);
    return () => clearInterval(timer);
  }, []);
  const schedule = watchingScheduleFromGuide(
    guideSnapshot.layout,
    snapshot.channel?.id,
    watchingScheduleTimeMs(snapshot, clockMs),
  );
  const dismissControls = useCallback(() => setControlsVisible(false), []);
  const markSwitchShown = useCallback(
    (what: "osd" | "still") => certMarks.held(controller.getSnapshot().attemptId, what),
    [controller],
  );
  return (
    <View style={{ flex: 1 }}>
      <WatchingSurface
        channelDigits={remoteHasDigits}
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
        onOpenGuide={() => {
          // The release of a long press on Watching is not an OK press.
          if (tvPressIsLongSelectRelease(longSelectAtMs.current, Date.now()))
            longSelectAtMs.current = undefined;
          else dispatchRemoteEvent({ key: "select" });
        }}
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
              onTune={tuneFromList}
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
              autoTune={
                remoteHasDigits
                  ? { choicesMs: TV_AUTO_TUNE_CHOICES_MS, onChange: changeAutoTune, valueMs: autoTuneMs }
                  : undefined
              }
              clientVersion={clientVersion}
              controller={guide}
              currentChannelId={snapshot.channel?.id}
              density="tv"
              favoriteChannelIds={myChannelsSnapshot.favouriteIds}
              focusRegistry={surfFocusRegistry}
              onDisconnect={() => session.disconnect()}
              onForget={() => session.forgetServer()}
              onTune={tuneFromList}
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
          {favouriteNote ? (
            <Surface
              accessibilityLiveRegion="polite"
              alignItems="center"
              backgroundColor="$surfaceEntry"
              borderRadius={8}
              bottom={48}
              left={48}
              paddingHorizontal={20}
              paddingVertical={12}
              position="absolute"
            >
              <Text density="tv" numberOfLines={1} textRole="reading">
                {favouriteNote}
              </Text>
            </Surface>
          ) : null}
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
  // PairingShell answers Back on its screens; tvOS sends Menu there only while the key is enabled.
  const pairingState = useSyncExternalStore(session.subscribe, session.snapshot, session.snapshot);
  useTvMenuKey(pairingBackAvailable(pairingState));
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
