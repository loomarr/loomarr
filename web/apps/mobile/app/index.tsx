import { createGuideController, createGuideSourcePort } from "@loomarr/core/guide";
import type { PairingCredential } from "@loomarr/core/pairing";
import {
  createAuthenticatedFetch,
  createPairingCredentialStore,
  createPairingTransport,
  PairingSession,
  validatePairingCredential,
} from "@loomarr/core/pairing";
import type { PairedNativePlayer } from "@loomarr/player/native";
import { NativePlayerView, usePairedNativePlayer } from "@loomarr/player/native";
import type { ClientDestination } from "@loomarr/ui";
import {
  ClientNavigation,
  ClientShell,
  clientBackDestination,
  GuideJourney,
  PairingShell,
  WatchingSurface,
} from "@loomarr/ui";
import * as SecureStore from "expo-secure-store";
import { StatusBar } from "expo-status-bar";
import { useCallback, useEffect, useMemo, useState } from "react";
import { BackHandler, Platform, View } from "react-native";

const credentialStore = createPairingCredentialStore({
  deleteItem: SecureStore.deleteItemAsync,
  getItem: SecureStore.getItemAsync,
  setItem: SecureStore.setItemAsync,
});

const MobileWatching = ({
  onNavigate,
  player,
}: {
  onNavigate: (destination: ClientDestination) => void;
  player: PairedNativePlayer;
}) => {
  const { controller, loadError, refresh, snapshot, transport } = player;
  return (
    <WatchingSurface
      density="touch"
      loadError={loadError}
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

const MobileShell = ({ credential, session }: { credential: PairingCredential; session: PairingSession }) => {
  const [active, setActive] = useState<ClientDestination>("guide");
  const onRevoked = useCallback(() => session.revoked(), [session]);
  const player = usePairedNativePlayer({ credential, initialTune: "none", onRevoked });
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
      <MobileWatching onNavigate={setActive} player={player} />
      {active === "guide" ? (
        <View style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}>
          <GuideJourney
            controller={guide}
            density="touch"
            onTune={(channelId) => {
              void player.controller.tuneChannel(channelId);
              setActive("watching");
            }}
            preferredChannelId={player.snapshot.channel?.id}
          />
          <ClientNavigation active="guide" density="touch" onNavigate={setActive} />
        </View>
      ) : active === "surf" ? (
        <View style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}>
          <ClientShell
            active={active}
            density="touch"
            onDisconnect={() => session.disconnect()}
            onNavigate={setActive}
            serverName={credential.serverUrl}
          />
        </View>
      ) : null}
    </View>
  );
};

const Index = () => {
  const session = useMemo(
    () =>
      new PairingSession({
        createTransport: createPairingTransport,
        deviceName: `${Platform.OS === "ios" ? "iPhone" : "Android"} Loomarr`,
        store: credentialStore,
        validateCredential: validatePairingCredential,
      }),
    [],
  );
  return (
    <>
      <PairingShell
        allowServerEntry
        density="touch"
        initialServerUrl={process.env.EXPO_PUBLIC_LOOMARR_URL}
        renderPaired={(credential) => <MobileShell credential={credential} session={session} />}
        session={session}
      />
      <StatusBar style="auto" />
    </>
  );
};

export default Index;
