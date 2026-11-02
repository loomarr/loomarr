import { serverNowOf } from "@loomarr/core/channel-timeline";
import { formatGuideTime } from "@loomarr/core/guide";
import type { PairingCredential } from "@loomarr/core/pairing";
import {
  createPairingCredentialStore,
  createPairingTransport,
  PairingSession,
  validatePairingCredential,
} from "@loomarr/core/pairing";
import { BrandLaunch, Surface, Text, ViewportStage } from "@loomarr/design-system";
import { createNativeServerDiscovery } from "@loomarr/lan-discovery-native";
import { createNativeApp, NativeAppRoot, useCrashReportUpload } from "@loomarr/native-app";
import {
  createLastChannelStore,
  createPlaybackMarks,
  NativePlayerView,
  PairedNativeImage,
  pairedNativeImageSource,
  usePairedClient,
  useStationIdent,
} from "@loomarr/player/native";
import type { ClientDestination, TvWatchingProps } from "@loomarr/ui";
import {
  clientBackDestination,
  GuideJourney,
  PairingShell,
  PlaybarStillSourceProvider,
  pairingBackAvailable,
  playbarFromTimeline,
  SurfJourney,
  TvWatching,
  useAutoHideControls,
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
/** How long a note (a Favorites change, an expired pause) stays up. */
const noteMs = 2_500;
/** ±10 s, as on every player (#2088 decision 2). */
const SEEK_STEP_MS = 10_000;

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

// Crash capture installs here, before the first render, so a crash while the app starts is reported too.
const nativeApp = createNativeApp({ clientVersion });

// The Siri Remote has no number keys, so Apple TV has no digit entry: no "0–9 jump" hint and no
// channel-number wait to tune (#1500 audit #3; maintainer, 2026-10-13: "It doesn't need tuning").
// react-native-tvos 0.88's types no longer declare Platform.isTVOS; tvOS is iOS on a TV.
const isTvOS = Platform.OS === "ios" && Platform.isTV;
const remoteHasDigits = !isTvOS;

// react-native-tvos 0.88 types useTVEventHandler's eventKeyAction as a string, but the native
// module sends the key action as a number (its TVEventHandler declares `?number`).
const keyActionOf = (value: unknown): number | undefined => (typeof value === "number" ? value : undefined);

/**
 * tvOS gives the Menu key to the app only while it is enabled, and then nothing else acts on it, so
 * enable it only while a screen has somewhere to go back to. Everywhere else Menu goes to the Home
 * screen, as Apple's HIG expects at an app's root. A modal dialog takes Menu itself. Android TV's
 * Back always reaches BackHandler.
 */
const useTvMenuKey = (enabled: boolean) => {
  useEffect(() => {
    if (!enabled || !isTvOS) return undefined;
    TVEventControl.enableTVMenuKey();
    return () => TVEventControl.disableTVMenuKey();
  }, [enabled]);
};

const TvShell = ({ credential, session }: { credential: PairingCredential; session: PairingSession }) => {
  const [active, setActive] = useState<ClientDestination>("watching");
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
    timeline,
    transport,
  } = usePairedClient({
    credential,
    diagnostics: nativeApp.diagnostics,
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
    timeline: true,
  });
  // The §9 controls show and hide as on every player, and clear at once for the station-ID card.
  const playing = snapshot.status === "playing";
  const { controlsShown, dismissControls, hideControls, revealControls } = useAutoHideControls(playing);
  const ident = useStationIdent(snapshot);
  useEffect(() => {
    if (ident) hideControls();
  }, [ident, hideControls]);
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
  // Pending crash reports (last launch's included) go to this server once paired.
  useCrashReportUpload(credential, session);
  useEffect(() => nativeApp.crashReports.recordBreadcrumb("navigation", active), [active]);
  const runRemoteIntent = useCallback(
    (intent: TvWatchingRemoteIntent | undefined) => {
      switch (intent?.kind) {
        case "seek": {
          const viewerTimeMs = controller.getSnapshot().livePlayback?.viewerTimeMs;
          if (viewerTimeMs !== undefined) controller.seek(viewerTimeMs + intent.direction * SEEK_STEP_MS);
          break;
        }
        case "step":
          void controller.step(intent.direction);
          break;
        case "tune-number":
          void controller.tuneNumber(intent.digits);
          break;
        case "show-controls":
          revealControls();
          break;
        case "hide-controls":
          dismissControls();
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
    [controller, dismissControls, revealControls],
  );
  // The remote's map depends on whether the controls are up (#2088 decision 15).
  const dispatchRemoteEvent = useCallback(
    (event: TvWatchingRemoteEvent) => {
      if (certMarks.enabled && event.key !== "timeout") lastKeyAtMs.current = certMarks.now();
      const result = reduceTvWatchingRemote(remoteStateRef.current, event, autoTuneMs, controlsShown);
      remoteStateRef.current = result.state;
      setRemoteState(result.state);
      runRemoteIntent(result.intent);
      return result.handled;
    },
    [autoTuneMs, controlsShown, runRemoteIntent],
  );
  useEffect(() => {
    const subscription = BackHandler.addEventListener("hardwareBackPress", () => {
      // Back hides the controls first; over a bare picture it leaves Watching, the app's root.
      if (active === "watching" && dispatchRemoteEvent({ key: "back" })) return true;
      const destination = clientBackDestination(active);
      if (!destination) return false;
      setActive(destination);
      return true;
    });
    return () => subscription.remove();
  }, [active, dispatchRemoteEvent]);
  useTvMenuKey(clientBackDestination(active) !== null || (active === "watching" && controlsShown));
  useTVEventHandler(({ eventKeyAction, eventType }) => {
    if (active !== "watching") return;
    const event = tvWatchingRemoteEventFromNative(eventType, Date.now(), keyActionOf(eventKeyAction));
    if (!event) return;
    // A key among the controls keeps them up a while longer; Back (tvOS: Menu) is the one that hides them.
    if (controlsShown && event.key !== "back" && event.key !== "menu") revealControls();
    // OK reaches the focused control, or the bare-picture layer, as its own press; Back reaches the
    // BackHandler above.
    if (event.key !== "select" && event.key !== "back") dispatchRemoteEvent(event);
  });
  // A short note bottom left, over whatever is drawn, announced to a screen reader too.
  const [note, setNote] = useState<string | undefined>(undefined);
  const noteTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => () => clearTimeout(noteTimer.current), []);
  const showNote = useCallback((text: string) => {
    setNote(text);
    void AccessibilityInfo.announceForAccessibility(text);
    clearTimeout(noteTimer.current);
    noteTimer.current = setTimeout(() => setNote(undefined), noteMs);
  }, []);
  // A pause held past the window's edge plays on from live, and says so (as web's player does).
  const noticeRevision = snapshot.livePlayback?.noticeRevision ?? 0;
  useEffect(() => {
    if (noticeRevision > 0) showNote("That paused point is no longer available, so you're back live.");
  }, [noticeRevision, showNote]);
  // A long-press of Select on a Guide or Surf row stars or unstars that row's channel (a short
  // press still tunes). The controller shows the change at once and puts it back if the server
  // refuses, so the note reads what the lists say afterwards rather than what was asked for.
  const toggleFavourite = useCallback(
    async (channelId: string) => {
      const { starred } = tvFavouriteToggle(channelId, myChannels.getSnapshot().favouriteIds);
      await myChannels.setFavourite(channelId, starred);
      const applied = myChannels.getSnapshot().favouriteIds.includes(channelId) === starred;
      const name = snapshot.catalog.find(({ id }) => id === channelId)?.name;
      showNote(
        applied
          ? `${starred ? "Added to" : "Removed from"} Favorites${name ? ` · ${name}` : ""}`
          : "Couldn't update Favorites",
      );
    },
    [myChannels, showNote, snapshot.catalog],
  );
  // tvOS also delivers the held row's own press when the finger lifts; it is not a tune request.
  const longSelectAtMs = useRef<number | undefined>(undefined);
  const tuneFromList = (channelId: string) => {
    if (tvPressIsLongSelectRelease(longSelectAtMs.current, Date.now())) {
      longSelectAtMs.current = undefined;
      return;
    }
    void controller.tuneChannel(channelId);
    setActive("watching");
  };
  useTVEventHandler(({ eventKeyAction, eventType }) => {
    if (eventType === "longSelect") longSelectAtMs.current = Date.now();
    if (active === "watching" || !isTvLongSelect(eventType, keyActionOf(eventKeyAction))) return;
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
  const markSwitchShown = useCallback(
    (what: "osd" | "still") => certMarks.held(controller.getSnapshot().attemptId, what),
    [controller],
  );
  const stillSource = useMemo(() => (uri: string) => pairedNativeImageSource(credential, uri), [credential]);

  // The §9 chrome's content: the lower third, the playbar and the transport, read from the tuned
  // channel's timeline at the playhead, as on every player.
  const live = snapshot.livePlayback ?? {
    lagSeconds: 0,
    mode: "live" as const,
    noticeRevision: 0,
    viewerTimeMs: Date.now(),
  };
  const channel = snapshot.channel;
  const chromeData = channel
    ? playbarFromTimeline({
        airings: timeline?.airings ?? [],
        channel: { name: channel.name, number: String(channel.number) },
        live,
        nowMs: timeline ? serverNowOf(timeline) : Date.now(),
        playheadMs: live.viewerTimeMs,
      })
    : undefined;
  const timeZone = guideSnapshot.layout?.timezone;
  const seekBy = (ms: number) => controller.seek(live.viewerTimeMs + ms);
  const startOverMs = chromeData?.startOverMs;
  const chrome: TvWatchingProps["chrome"] = chromeData
    ? {
        band: chromeData.band,
        formatTime: (ms) => formatGuideTime(ms, timeZone),
        ident,
        playbar: timeline
          ? {
              onSeek: controller.seek,
              playheadMs: live.viewerTimeMs,
              segments: chromeData.segments,
              window: chromeData.window,
            }
          : undefined,
        transport: {
          density: "tv",
          live,
          // The TV's own remote owns the volume; a TV transport draws none.
          muted: false,
          onChannels: () => setActive("surf"),
          onGoLive: () => void controller.goLive(),
          onGuide: () => setActive("guide"),
          onSeekBack: () => seekBy(-SEEK_STEP_MS),
          onSeekForward: () => seekBy(SEEK_STEP_MS),
          onStartOver: startOverMs === undefined ? undefined : () => controller.seek(startOverMs),
          onTogglePlay: () => (playing ? controller.pause() : void controller.play()),
          onToggleMute: () => undefined,
          onVolume: () => undefined,
          playing,
          pointer: false,
          volume: 1,
        },
      }
    : undefined;
  return (
    <View style={{ flex: 1 }}>
      <PlaybarStillSourceProvider value={stillSource}>
        <TvWatching
          chrome={chrome}
          chromeVisible={active === "watching"}
          controlsShown={controlsShown}
          loading={catalogState.loading}
          loadError={catalogState.error}
          numberEntry={tvNumberEntryPresentation(remoteState, snapshot.catalog)}
          onBarePress={() => {
            // The release of a long press on Watching is not an OK press.
            if (tvPressIsLongSelectRelease(longSelectAtMs.current, Date.now()))
              longSelectAtMs.current = undefined;
            else dispatchRemoteEvent({ key: "select" });
          }}
          onChangeServer={() => session.chooseServer()}
          onLeave={() => setActive("guide")}
          onRetry={() => {
            if (catalogState.error) refresh();
            else void controller.retry();
          }}
          onSwitchShown={certMarks.enabled ? markSwitchShown : undefined}
          player={<NativePlayerView style={{ flex: 1 }} transport={transport} />}
          snapshot={snapshot}
        />
      </PlaybarStillSourceProvider>
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
        </View>
      )}
      {note ? (
        // Over the picture the lower third owns the foot, so the note takes the readout's top left.
        <Surface
          accessibilityLiveRegion="polite"
          alignItems="center"
          backgroundColor="surfaceEntry"
          borderRadius={8}
          bottom={active === "watching" ? undefined : 48}
          left={48}
          paddingHorizontal={20}
          paddingVertical={12}
          position="absolute"
          top={active === "watching" ? 48 : undefined}
        >
          <Text density="tv" numberOfLines={1} textRole="reading">
            {note}
          </Text>
        </Surface>
      ) : null}
    </View>
  );
};

const TvClient = () => {
  useKeepAwake();
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
  useEffect(() => {
    const unsubscribe = session.subscribe(({ status }) =>
      nativeApp.crashReports.recordBreadcrumb("lifecycle", `pairing ${status}`),
    );
    return () => void unsubscribe();
  }, [session]);
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
    <>
      <ViewportStage>
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
      </ViewportStage>
      <StatusBar hidden />
    </>
  );
};

const App = () => (
  <NativeAppRoot app={nativeApp}>
    <TvClient />
  </NativeAppRoot>
);

export default App;
