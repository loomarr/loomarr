import { formatGuideTime } from "@loomarr/core/guide";
import type { PairingCredential } from "@loomarr/core/pairing";
import {
  createPairingCredentialStore,
  createPairingTransport,
  PairingSession,
  validatePairingCredential,
} from "@loomarr/core/pairing";
import { Surface, Text, ViewportStage } from "@loomarr/design-system";
import { createNativeServerDiscovery } from "@loomarr/lan-discovery-native";
import {
  createNativeApp,
  LaunchStage,
  NativeAppRoot,
  useAppForeground,
  usePairingBreadcrumbs,
  useShellCrashReporting,
} from "@loomarr/native-app";
import { createLastChannelStore, retryPairedClient, usePairedClient } from "@loomarr/paired-client";
import { createPlaybackMarks, NativePlayerView, PairedNativeImage } from "@loomarr/player/native";
import {
  clientBackDestination,
  GuideJourney,
  PairingShell,
  pairingBackAvailable,
  playerChromeFromGuide,
  SurfJourney,
  useClientDestination,
  WatchingSurface,
  watchingScheduleTimeMs,
} from "@loomarr/ui";
import {
  createTvAutoTuneSetting,
  createTvGuideFocusRegistry,
  createTvListTune,
  createTvSurfFocusRegistry,
  isTvLongSelect,
  restoreTvSurfSelection,
  runTvRemoteIntent,
  TV_AUTO_TUNE_CHOICES_MS,
  TvScreens,
  tvGuideChannelWindow,
  tvGuideFavouriteChannel,
  tvGuideTimeEdge,
  tvNumberEntryPresentation,
  tvRemoteHasDigits,
  tvSurfFavouriteChannel,
  useTvAutoTune,
  useTvFavourites,
  useTvMenuKey,
  useWatchingControls,
} from "@loomarr/ui-tv";
import { useKeepAwake } from "expo-keep-awake";
import * as SecureStore from "expo-secure-store";
import * as SplashScreen from "expo-splash-screen";
import { StatusBar } from "expo-status-bar";
import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import { TVEventControl, useTVEventHandler } from "react-native";
import appConfig from "../app.json";
import { useTvRemote } from "./use-tv-remote";

void SplashScreen.preventAutoHideAsync();
const hideNativeSplash = () => SplashScreen.hide();

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
const remoteHasDigits = tvRemoteHasDigits();

// react-native-tvos 0.88 types useTVEventHandler's eventKeyAction as a string, but the native
// module sends the key action as a number (its TVEventHandler declares `?number`).
const keyActionOf = (value: unknown): number | undefined => (typeof value === "number" ? value : undefined);

const TvShell = ({ credential, session }: { credential: PairingCredential; session: PairingSession }) => {
  const [active, setActive] = useClientDestination("watching");
  const controls = useWatchingControls();
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
  });
  const guideFocusRegistry = useMemo(createTvGuideFocusRegistry, []);
  const surfFocusRegistry = useMemo(createTvSurfFocusRegistry, []);
  const [autoTuneMs, changeAutoTune] = useTvAutoTune(autoTuneSetting);
  useTvMenuKey(clientBackDestination(active) !== null, { control: TVEventControl });
  // Pending crash reports (last launch's included) go to this server once paired.
  useShellCrashReporting(credential, session, active);
  const [remote, remoteState] = useTvRemote(autoTuneMs, (event, { handled, intent }) => {
    if (certMarks.enabled && event.key !== "timeout") lastKeyAtMs.current = certMarks.now();
    if (handled) controls.showForActivity();
    runTvRemoteIntent(intent, controller, setActive);
  });
  useTVEventHandler(({ eventKeyAction, eventType }) =>
    remote.nativeKey(eventType, keyActionOf(eventKeyAction), active === "watching", controls.visible),
  );
  const { note: favouriteNote, toggle: toggleFavourite } = useTvFavourites(myChannels, snapshot.catalog);
  const tuneFromList = createTvListTune(remote, controller, controls.showForActivity, setActive);
  useTVEventHandler(({ eventKeyAction, eventType }) => {
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
    if (active !== "watching") remote.leaveWatching();
  }, [active, remote]);
  const [clockMs, setClockMs] = useState(Date.now);
  useEffect(() => {
    const timer = setInterval(() => setClockMs(Date.now()), 30_000);
    return () => clearInterval(timer);
  }, []);
  const chrome = playerChromeFromGuide({ layout: guideSnapshot.layout, nowMs: clockMs, snapshot });
  const markSwitchShown = useCallback(
    (what: "osd" | "still") => certMarks.held(controller.getSnapshot().attemptId, what),
    [controller],
  );
  return (
    <TvScreens
      active={active}
      watching={
        <WatchingSurface
          chrome={chrome}
          chromeVisible={active === "watching"}
          controlsActivityKey={controls.activityKey}
          controlsVisible={controls.visible}
          formatTime={(ms) => formatGuideTime(ms, guideSnapshot.layout?.timezone)}
          loading={catalogState.loading}
          loadError={catalogState.error}
          onChangeServer={() => session.chooseServer()}
          onDismissControls={controls.dismiss}
          onGoLive={() => void controller.goLive()}
          onOpenGuide={remote.select}
          onRetry={() => retryPairedClient({ catalogState, controller, refresh })}
          onSwitchShown={certMarks.enabled ? markSwitchShown : undefined}
          onTuneChannel={(channelId) => {
            controls.showForActivity();
            void controller.tuneChannel(channelId);
          }}
          numberEntry={tvNumberEntryPresentation(remoteState, snapshot.catalog)}
          playheadMs={watchingScheduleTimeMs(snapshot, clockMs)}
          player={<NativePlayerView style={{ flex: 1 }} transport={transport} />}
          snapshot={snapshot}
        />
      }
      overlay={
        <>
          {active === "guide" ? (
            <GuideJourney
              channelWindow={tvGuideChannelWindow}
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
              backgroundColor="surfaceEntry"
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
        </>
      }
    />
  );
};

const TvClient = () => {
  useKeepAwake();
  const appForeground = useAppForeground();
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
  usePairingBreadcrumbs(session);
  // PairingShell answers Back on its screens; tvOS sends Menu there only while the key is enabled.
  const pairingState = useSyncExternalStore(session.subscribe, session.snapshot, session.snapshot);
  useTvMenuKey(pairingBackAvailable(pairingState), { control: TVEventControl });
  return (
    <>
      <ViewportStage>
        <LaunchStage density="tv" hideNativeSplash={hideNativeSplash} minimumMs={launchMinimumMs}>
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
        </LaunchStage>
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
