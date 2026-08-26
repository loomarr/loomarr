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
import type { PairedNativePlayer } from "@loomarr/player/native";
import { NativePlayerView, usePairedNativePlayer } from "@loomarr/player/native";
import type { ClientDestination } from "@loomarr/ui";
import {
  ClientNavigation,
  ClientShell,
  clientBackDestination,
  GuideJourney,
  PairingShell,
  SurfJourney,
  WatchingSurface,
} from "@loomarr/ui";
import { createTvNumberEntryController } from "@loomarr/ui-tv";
import { useKeepAwake } from "expo-keep-awake";
import * as SecureStore from "expo-secure-store";
import { StatusBar } from "expo-status-bar";
import { useCallback, useEffect, useMemo, useState, useSyncExternalStore } from "react";
import { BackHandler, useTVEventHandler, View } from "react-native";
import { SafeAreaProvider, useSafeAreaInsets } from "react-native-safe-area-context";

const credentialStore = createPairingCredentialStore({
  deleteItem: SecureStore.deleteItemAsync,
  getItem: SecureStore.getItemAsync,
  setItem: SecureStore.setItemAsync,
});

const TvWatching = ({
  interactive,
  onNavigate,
  player,
}: {
  interactive: boolean;
  onNavigate: (destination: ClientDestination) => void;
  player: PairedNativePlayer;
}) => {
  const { controller, loadError, refresh, snapshot, transport } = player;
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
    if (numberEntry.pushEvent(eventType)) {
      controller.revealOverlay();
    } else if (eventType === "select" && numberEntrySnapshot.digits) {
      numberEntry.commit();
    } else if (eventType === "up" || eventType === "channelUp") void controller.step(1);
    else if (eventType === "down" || eventType === "channelDown") void controller.step(-1);
    else if (eventType === "left" || eventType === "menu") onNavigate("surf");
    else controller.revealOverlay();
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
      snapshot={snapshot}
    />
  );
};

const TvShell = ({ credential, session }: { credential: PairingCredential; session: PairingSession }) => {
  const [active, setActive] = useState<ClientDestination>("watching");
  const onRevoked = useCallback(() => session.revoked(), [session]);
  const player = usePairedNativePlayer({ credential, onRevoked });
  const authenticatedFetch = useMemo(
    () => createAuthenticatedFetch(credential, onRevoked),
    [credential, onRevoked],
  );
  const guide = useMemo(
    () => createGuideController({ source: createGuideSourcePort(authenticatedFetch) }),
    [authenticatedFetch],
  );
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
      <TvWatching interactive={active === "watching"} onNavigate={setActive} player={player} />
      {active === "guide" ? (
        <View style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}>
          <GuideJourney
            controller={guide}
            density="tv"
            onTune={(channelId) => {
              void player.controller.tuneChannel(channelId);
              setActive("watching");
            }}
            preferredChannelId={player.snapshot.channel?.id}
          />
          <ClientNavigation active="guide" density="tv" onNavigate={setActive} />
        </View>
      ) : active === "surf" ? (
        <View style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}>
          <ClientShell
            active={active}
            density="tv"
            onDisconnect={() => session.disconnect()}
            onNavigate={setActive}
            serverName={credential.serverUrl}
          >
            <SurfJourney
              clientName="Loomarr TV"
              clientVersion="prototype"
              controller={guide}
              currentChannelId={player.snapshot.channel?.id}
              density="tv"
              onTune={(channelId) => {
                void player.controller.tuneChannel(channelId);
                setActive("watching");
              }}
              playableChannelIds={player.snapshot.catalog.map(({ id }) => id)}
              recentChannelIds={player.snapshot.recentChannelIds}
            />
          </ClientShell>
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
