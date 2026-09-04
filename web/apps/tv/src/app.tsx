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
import { createPlayUrlSourcePort } from "@loomarr/player/server";
import type { ClientDestination } from "@loomarr/ui";
import { ClientShell, clientBackDestination, PairingShell, WatchingSurface } from "@loomarr/ui";
import { useKeepAwake } from "expo-keep-awake";
import * as SecureStore from "expo-secure-store";
import { StatusBar } from "expo-status-bar";
import { useCallback, useEffect, useMemo, useState, useSyncExternalStore } from "react";
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
  const [controlsVisible, setControlsVisible] = useState(true);
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
  const snapshot = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  useEffect(() => () => controller.dispose(), [controller]);
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
      <WatchingSurface
        chromeVisible={active === "watching"}
        controlsVisible={controlsVisible}
        density="tv"
        onChannelDown={() => void controller.step(-1)}
        onChannelUp={() => void controller.step(1)}
        onDismissControls={() => setControlsVisible(false)}
        onGoLive={() => void controller.goLive()}
        onOpenGuide={() => setActive("guide")}
        onOpenSurf={() => setActive("surf")}
        onPause={controller.pause}
        onPlay={() => void controller.play()}
        onPrevious={() => void controller.previous()}
        onRetry={() => void controller.retry()}
        onShowControls={() => setControlsVisible(true)}
        player={<NativePlayerView style={{ flex: 1 }} transport={transport} />}
        snapshot={snapshot}
      />
      {active === "watching" ? null : (
        <View style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}>
          <ClientShell
            active={active}
            density="tv"
            onDisconnect={() => runtime.session.disconnect()}
            onNavigate={setActive}
            serverName={runtime.credential.serverUrl}
          />
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
