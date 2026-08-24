import type { PairingCredential } from "@loomarr/core/pairing";
import {
  createPairingCredentialStore,
  createPairingTransport,
  PairingSession,
  validatePairingCredential,
} from "@loomarr/core/pairing";
import type { ClientDestination } from "@loomarr/ui";
import { ClientShell, PairingShell } from "@loomarr/ui";
import * as SecureStore from "expo-secure-store";
import { StatusBar } from "expo-status-bar";
import { useMemo, useState } from "react";
import { Platform } from "react-native";

const credentialStore = createPairingCredentialStore({
  deleteItem: SecureStore.deleteItemAsync,
  getItem: SecureStore.getItemAsync,
  setItem: SecureStore.setItemAsync,
});

const MobileShell = ({ credential }: { credential: PairingCredential }) => {
  const [active, setActive] = useState<ClientDestination>("guide");
  return (
    <ClientShell active={active} density="touch" onNavigate={setActive} serverName={credential.serverUrl} />
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
        renderPaired={(credential) => <MobileShell credential={credential} />}
        session={session}
      />
      <StatusBar style="auto" />
    </>
  );
};

export default Index;
