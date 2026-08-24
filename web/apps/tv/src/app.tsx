import type { PairingCredential } from "@loomarr/core/pairing";
import {
  createPairingCredentialStore,
  createPairingTransport,
  PairingSession,
  validatePairingCredential,
} from "@loomarr/core/pairing";
import { LoomarrProvider } from "@loomarr/design-system";
import type { ClientDestination } from "@loomarr/ui";
import { ClientShell, clientBackDestination, PairingShell } from "@loomarr/ui";
import { useKeepAwake } from "expo-keep-awake";
import * as SecureStore from "expo-secure-store";
import { StatusBar } from "expo-status-bar";
import { useEffect, useMemo, useState } from "react";
import { BackHandler } from "react-native";
import { SafeAreaProvider, useSafeAreaInsets } from "react-native-safe-area-context";

const credentialStore = createPairingCredentialStore({
  deleteItem: SecureStore.deleteItemAsync,
  getItem: SecureStore.getItemAsync,
  setItem: SecureStore.setItemAsync,
});

const TvShell = ({ credential, session }: { credential: PairingCredential; session: PairingSession }) => {
  const [active, setActive] = useState<ClientDestination>("watching");
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
    <ClientShell
      active={active}
      density="tv"
      onDisconnect={() => session.disconnect()}
      onNavigate={setActive}
      serverName={credential.serverUrl}
    />
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
