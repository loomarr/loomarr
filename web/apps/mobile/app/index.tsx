import type { PairingCredential } from "@loomarr/core/pairing";
import {
  createPairingCredentialStore,
  createPairingTransport,
  PairingSession,
  validatePairingCredential,
} from "@loomarr/core/pairing";
import { BottomSheet, Text } from "@loomarr/design-system";
import { PairedNativeImage, usePairedClient } from "@loomarr/player/native";
import type { ClientDestination } from "@loomarr/ui";
import { ClientShell, clientBackDestination, GuideJourney, PairingShell } from "@loomarr/ui";
import * as SecureStore from "expo-secure-store";
import { StatusBar } from "expo-status-bar";
import { useEffect, useMemo, useState } from "react";
import { BackHandler, Platform, View } from "react-native";

const credentialStore = createPairingCredentialStore({
  deleteItem: SecureStore.deleteItemAsync,
  getItem: SecureStore.getItemAsync,
  setItem: SecureStore.setItemAsync,
});

// The selected programme docks as the iPhone's sheet (#1659 mock 5d) or Android's strip (5f).
const guideDock = Platform.OS === "ios" ? BottomSheet : "strip";

const MobileShell = ({ credential, session }: { credential: PairingCredential; session: PairingSession }) => {
  const [active, setActive] = useState<ClientDestination>("guide");
  const { controller, guide, myChannelsSnapshot, snapshot } = usePairedClient({
    credential,
    guideWindowMinutes: 120,
    session,
  });
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
      density="touch"
      onDisconnect={() => session.disconnect()}
      onNavigate={setActive}
      serverName={credential.serverUrl}
    >
      {active === "guide" ? (
        <View style={{ flex: 1 }}>
          <View style={{ paddingBottom: 8, paddingHorizontal: 16, paddingTop: 12 }}>
            <Text accessibilityRole="header" density="touch" textRole="title">
              Guide
            </Text>
          </View>
          <GuideJourney
            controller={guide}
            density="touch"
            dock={guideDock}
            myChannels={myChannelsSnapshot}
            onTune={(channelId) => {
              void controller.tuneChannel(channelId);
              setActive("watching");
            }}
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
          />
        </View>
      ) : undefined}
    </ClientShell>
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
        renderPaired={(credential) => (
          <MobileShell credential={credential} key={credential.token} session={session} />
        )}
        session={session}
      />
      {/* Light glyphs: the provider draws the dark theme whatever the system's appearance. */}
      <StatusBar style="light" />
    </>
  );
};

export default Index;
