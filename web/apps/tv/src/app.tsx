import { ClientDiagnosticsReporter } from "@loomarr/core/client-diagnostics";
import { createGuideController, createGuideSourcePort, type GuideController } from "@loomarr/core/guide";
import type { PairingCredential } from "@loomarr/core/pairing";
import {
  createAuthenticatedFetch,
  createPairingCredentialStore,
  createPairingTransport,
  PairingSession,
  validatePairingCredential,
} from "@loomarr/core/pairing";
import { LoomarrProvider } from "@loomarr/design-system";
import type { PairedNativePlayer } from "@loomarr/player/native";
import { NativePlayerView, PairedNativeImage, usePairedNativePlayer } from "@loomarr/player/native";
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
  createTvNumberEntryController,
  createTvSurfFocusRegistry,
  handleTvWatchingRemoteEvent,
  restoreTvSurfSelection,
  tvGuideRowWindow,
} from "@loomarr/ui-tv";
import { useKeepAwake } from "expo-keep-awake";
import * as SecureStore from "expo-secure-store";
import { StatusBar } from "expo-status-bar";
import { useCallback, useEffect, useMemo, useState, useSyncExternalStore } from "react";
import { BackHandler, Platform, useTVEventHandler, View } from "react-native";
import { SafeAreaProvider, useSafeAreaInsets } from "react-native-safe-area-context";
import appConfig from "../app.json";

const credentialStore = createPairingCredentialStore({
  deleteItem: SecureStore.deleteItemAsync,
  getItem: SecureStore.getItemAsync,
  setItem: SecureStore.setItemAsync,
});

const TvWatching = ({
  interactive,
  guide,
  onNavigate,
  player,
}: {
  interactive: boolean;
  guide: GuideController;
  onNavigate: (destination: ClientDestination) => void;
  player: PairedNativePlayer;
}) => {
  const { controller, loadError, refresh, snapshot, transport } = player;
  const guideSnapshot = useSyncExternalStore(guide.subscribe, guide.getSnapshot, guide.getSnapshot);
  useEffect(() => {
    void guide.refresh(snapshot.channel?.id);
  }, [guide, snapshot.channel?.id]);
  const schedule = watchingScheduleFromGuide(
    guideSnapshot.layout,
    snapshot.channel?.id,
    snapshot.livePlayback?.viewerTimeMs ?? Date.now(),
  );
  const numberEntry = useMemo(
    () => createTvNumberEntryController({ onCommit: (digits) => void controller.tuneNumber(digits) }),
    [controller],
  );
  const numberEntrySnapshot = useSyncExternalStore(
    numberEntry.subscribe,
    numberEntry.getSnapshot,
    numberEntry.getSnapshot,
  );
  useEffect(() => () => numberEntry.dispose(), [numberEntry]);
  useEffect(() => {
    if (!interactive) numberEntry.cancel();
  }, [interactive, numberEntry]);
  useTVEventHandler(({ eventType }) => {
    if (!interactive) return;
    handleTvWatchingRemoteEvent(eventType, Boolean(numberEntrySnapshot.digits), {
      commitNumber: numberEntry.commit,
      enterNumber: numberEntry.pushEvent,
      openGuide: () => onNavigate("guide"),
      openSurf: () => onNavigate("surf"),
      revealOverlay: controller.revealOverlay,
      step: (direction) => void controller.step(direction),
    });
  });
  const numberEntryChannel = snapshot.catalog.find(
    (channel) => String(channel.number) === numberEntrySnapshot.digits,
  );
  return (
    <WatchingSurface
      chromeVisible={interactive}
      density="tv"
      loadError={loadError}
      numberEntry={{ channelName: numberEntryChannel?.name, digits: numberEntrySnapshot.digits }}
      onChannelDown={() => void controller.step(-1)}
      onChannelUp={() => void controller.step(1)}
      onDismissControls={controller.dismissOverlay}
      onGoLive={() => void controller.goLive()}
      onOpenGuide={() => onNavigate("guide")}
      onOpenSurf={() => onNavigate("surf")}
      onPause={controller.pause}
      onPlay={() => void controller.play()}
      onPrevious={() => void controller.previous()}
      onRetry={() => void (loadError ? refresh() : controller.retry())}
      onShowControls={controller.revealOverlay}
      player={<NativePlayerView style={{ flex: 1 }} transport={transport} />}
      schedule={schedule}
      snapshot={snapshot}
    />
  );
};

const TvShell = ({ credential, session }: { credential: PairingCredential; session: PairingSession }) => {
  const [active, setActive] = useState<ClientDestination>("watching");
  const onRevoked = useCallback(() => session.revoked(), [session]);
  const authenticatedFetch = useMemo(
    () => createAuthenticatedFetch(credential, onRevoked),
    [credential, onRevoked],
  );
  const guide = useMemo(
    () => createGuideController({ source: createGuideSourcePort(authenticatedFetch) }),
    [authenticatedFetch],
  );
  const guideFocusRegistry = useMemo(createTvGuideFocusRegistry, []);
  const surfFocusRegistry = useMemo(createTvSurfFocusRegistry, []);
  const diagnostics = useMemo(() => {
    if (Platform.OS !== "android") return undefined;
    let reporter: ClientDiagnosticsReporter;
    reporter = new ClientDiagnosticsReporter(
      async (events) => {
        const response = await authenticatedFetch("/v1/diagnostics/client-events", {
          body: JSON.stringify(reporter.wireBatch(events)),
          headers: { "Content-Type": "application/json" },
          method: "POST",
        });
        if (!response.ok) throw new Error(`Client diagnostics were rejected (${response.status}).`);
      },
      {
        clientVersion: appConfig.expo.version,
        platform: Platform.constants.Model.toLowerCase().includes("shield") ? "shield_tv" : "android_tv",
        source: "android_tv",
      },
    );
    return reporter;
  }, [authenticatedFetch]);
  const onChannelEvent = useCallback(() => guide.refresh(), [guide]);
  const player = usePairedNativePlayer({ credential, diagnostics, onChannelEvent, onRevoked });
  useEffect(() => () => diagnostics?.dispose(), [diagnostics]);
  useEffect(() => () => guide.dispose(), [guide]);
  useEffect(() => {
    const subscription = BackHandler.addEventListener("hardwareBackPress", () => {
      const destination = clientBackDestination(active);
      if (!destination) return false;
      setActive(destination);
      return true;
    });
    return () => subscription.remove();
  }, [active]);
  return (
    <View style={{ flex: 1 }}>
      <TvWatching guide={guide} interactive={active === "watching"} onNavigate={setActive} player={player} />
      {active === "guide" ? (
        <View style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}>
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
              void player.controller.tuneChannel(channelId);
              setActive("watching");
            }}
            preferredChannelId={player.snapshot.channel?.id}
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
        </View>
      ) : active === "surf" ? (
        <View style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}>
          <SurfJourney
            clientName="Loomarr TV"
            clientVersion={appConfig.expo.version}
            controller={guide}
            currentChannelId={player.snapshot.channel?.id}
            density="tv"
            focusRegistry={surfFocusRegistry}
            onDisconnect={() => session.disconnect()}
            onTune={(channelId) => {
              void player.controller.tuneChannel(channelId);
              setActive("watching");
            }}
            playableChannelIds={player.snapshot.catalog.map(({ id }) => id)}
            recentChannelIds={player.snapshot.recentChannelIds}
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
            serverVersion={player.serverVersion}
          />
        </View>
      ) : null}
    </View>
  );
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
    <LoomarrProvider insets={insets}>
      <PairingShell
        density="tv"
        initialServerUrl={process.env.EXPO_PUBLIC_LOOMARR_URL}
        renderPaired={(credential) => <TvShell credential={credential} session={session} />}
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
