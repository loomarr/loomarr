import type { PairingCredential } from "@loomarr/core/pairing";
import {
  createPairingCredentialStore,
  createPairingTransport,
  PairingSession,
  validatePairingCredential,
} from "@loomarr/core/pairing";
import { NativePlayerView, usePairedNativePlayer } from "@loomarr/player/native";
import type { ClientDestination } from "@loomarr/ui";
import { ClientShell, clientBackDestination, PairingShell, WatchingSurface } from "@loomarr/ui";
import * as SecureStore from "expo-secure-store";
import { StatusBar } from "expo-status-bar";
import { useCallback, useEffect, useMemo, useState } from "react";
import { BackHandler, Platform } from "react-native";

const credentialStore = createPairingCredentialStore({
  deleteItem: SecureStore.deleteItemAsync,
  getItem: SecureStore.getItemAsync,
  setItem: SecureStore.setItemAsync,
});

const MobileWatching = ({
  credential,
  onNavigate,
  session,
}: {
  credential: PairingCredential;
  onNavigate: (destination: ClientDestination) => void;
  session: PairingSession;
}) => {
  const onRevoked = useCallback(() => session.revoked(), [session]);
  const { controller, loadError, refresh, snapshot, transport } = usePairedNativePlayer({
    credential,
    onRevoked,
  });
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
  useEffect(() => {
    const subscription = BackHandler.addEventListener("hardwareBackPress", () => {
      const destination = clientBackDestination(active);
      if (!destination) return false;
      setActive(destination);
      return true;
    });
    return () => subscription.remove();
  }, [active]);
  return active === "watching" ? (
    <MobileWatching credential={credential} onNavigate={setActive} session={session} />
  ) : (
    <ClientShell
      active={active}
      density="touch"
      onDisconnect={() => session.disconnect()}
      onNavigate={setActive}
      serverName={credential.serverUrl}
    />
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
