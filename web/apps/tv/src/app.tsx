import type { PairingCredential } from "@loomarr/core/pairing";
import {
  createPairingCredentialStore,
  createPairingTransport,
  PairingSession,
  validatePairingCredential,
} from "@loomarr/core/pairing";
import { LoomarrProvider } from "@loomarr/design-system";
import type { ClientDestination } from "@loomarr/ui";
import { ClientShell, PairingShell } from "@loomarr/ui";
import { useKeepAwake } from "expo-keep-awake";
import * as SecureStore from "expo-secure-store";
import { StatusBar } from "expo-status-bar";
import { useMemo, useState } from "react";

const credentialStore = createPairingCredentialStore({
  deleteItem: SecureStore.deleteItemAsync,
  getItem: SecureStore.getItemAsync,
  setItem: SecureStore.setItemAsync,
});

const TvShell = ({ credential }: { credential: PairingCredential }) => {
  const [active, setActive] = useState<ClientDestination>("watching");
  return (
    <ClientShell active={active} density="tv" onNavigate={setActive} serverName={credential.serverUrl} />
  );
};

const App = () => {
  useKeepAwake();
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
    <LoomarrProvider>
      <PairingShell
        density="tv"
        initialServerUrl={process.env.EXPO_PUBLIC_LOOMARR_URL}
        renderPaired={(credential) => <TvShell credential={credential} />}
        session={session}
      />
      <StatusBar hidden />
    </LoomarrProvider>
  );
};

export default App;
