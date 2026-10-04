import type { PairingCredential } from "@loomarr/core/pairing";
import {
  createPairingCredentialStore,
  createPairingTransport,
  PairingSession,
  validatePairingCredential,
} from "@loomarr/core/pairing";
import { BottomSheet, Text } from "@loomarr/design-system";
import {
  NativePlayerView,
  type PairedClient,
  PairedNativeImage,
  usePairedClient,
  usePairedRequests,
  useShellPause,
} from "@loomarr/player/native";
import type { ClientDestination } from "@loomarr/ui";
import {
  ClientShell,
  clientBackDestination,
  GuideJourney,
  PairingShell,
  RequestsJourney,
  StatePanel,
  WatchingPanel,
  watchingPanelFromGuide,
} from "@loomarr/ui";
import * as SecureStore from "expo-secure-store";
import { StatusBar } from "expo-status-bar";
import { useEffect, useMemo, useState } from "react";
import { BackHandler, Platform, ScrollView, View } from "react-native";

const credentialStore = createPairingCredentialStore({
  deleteItem: SecureStore.deleteItemAsync,
  getItem: SecureStore.getItemAsync,
  setItem: SecureStore.setItemAsync,
});

// The selected programme docks as the iPhone's sheet (#1659 mock 5d) or Android's strip (5f).
const guideDock = Platform.OS === "ios" ? BottomSheet : "strip";

// Watching (#1659 mocks 5e/5g, #1785): the picture edge to edge, and the controls, what's next and
// the favourites under it, the same panel the web phone draws.
const WatchingScreen = ({ client }: { client: PairedClient }) => {
  const { catalogState, controller, guideSnapshot, myChannelsSnapshot, refresh, snapshot, transport } =
    client;
  const [nowMs, setNowMs] = useState(Date.now);
  useEffect(() => {
    const timer = setInterval(() => setNowMs(Date.now()), 30_000);
    return () => clearInterval(timer);
  }, []);
  const playableChannelIds = useMemo(() => snapshot.catalog.map(({ id }) => id), [snapshot.catalog]);
  const panel = watchingPanelFromGuide({
    channelId: snapshot.channel?.id,
    favouriteIds: myChannelsSnapshot.favouriteIds,
    layout: guideSnapshot.layout,
    nowMs,
    playableChannelIds,
    recentIds: myChannelsSnapshot.recentIds,
  });
  const { previousId } = panel;
  const failure = catalogState.error ?? (snapshot.status === "failed" ? snapshot.error : undefined);
  return (
    <View style={{ flex: 1 }}>
      <View style={{ aspectRatio: 16 / 9, backgroundColor: "black", width: "100%" }}>
        <NativePlayerView style={{ flex: 1 }} transport={transport} />
      </View>
      {failure ? (
        <StatePanel
          action={{
            label: "Try again",
            onPress: () => (catalogState.error ? refresh() : void controller.retry()),
          }}
          density="touch"
          description={failure}
          kind="error"
          title="Can't play this channel"
        />
      ) : snapshot.channel ? (
        <ScrollView contentContainerStyle={{ paddingBottom: 16 }}>
          <WatchingPanel
            canPrevious={previousId !== undefined}
            canSurf={snapshot.catalog.length > 1}
            channel={{ name: snapshot.channel.name, number: String(snapshot.channel.number) }}
            clockLabel={panel.clockLabel}
            density="touch"
            favourites={panel.favourites}
            live={snapshot.livePlayback ?? { lagSeconds: 0, mode: "live" }}
            onChannelDown={() => void controller.step(-1)}
            onChannelUp={() => void controller.step(1)}
            onGoLive={() => void controller.goLive()}
            onPause={controller.pause}
            onPlay={() => void controller.play()}
            onPrevious={() => previousId && void controller.tuneChannel(previousId)}
            onTune={(channelId) => void controller.tuneChannel(channelId)}
            schedule={panel.schedule}
          />
        </ScrollView>
      ) : catalogState.loading ? (
        <StatePanel density="touch" kind="loading" title="Loading channels" />
      ) : null}
    </View>
  );
};

const MobileShell = ({ credential, session }: { credential: PairingCredential; session: PairingSession }) => {
  const [active, setActive] = useState<ClientDestination>("guide");
  // The phone opens on its Guide with no picture, so it tunes only when the viewer picks a channel.
  const client = usePairedClient({ credential, guideWindowMinutes: 120, initialTune: "none", session });
  const { controller, guide, myChannelsSnapshot, snapshot } = client;
  // Requests loads with the app so an admin's tab badge counts before they open it.
  const requests = usePairedRequests({ credential, session });
  // The picture is only mounted on Watching, so a stream left running elsewhere would play unseen:
  // pause when the viewer leaves Watching, and resume what the shell paused when they come back.
  const forgetShellPause = useShellPause(controller, active === "watching");
  // Watching a channel from outside it (a Guide pick, a finished request's Open channel).
  const watchChannel = (channelId: string) => {
    // Another channel replaces the stream the shell paused, so resuming it would only blip its
    // audio; the paused channel itself is not re-tuned, so Watching resumes it.
    forgetShellPause(channelId);
    void controller.tuneChannel(channelId);
    setActive("watching");
  };
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
      requestsBadge={requests.needsYouCount}
      serverName={credential.serverUrl}
    >
      {active === "requests" ? (
        // The journey docks its review sheet directly above the tab bar, so it draws the footer itself.
        (navigation) => (
          <RequestsJourney
            controller={requests.controller}
            footer={navigation}
            onOpenChannel={watchChannel}
          />
        )
      ) : active === "guide" ? (
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
            onTune={watchChannel}
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
      ) : active === "watching" ? (
        <WatchingScreen client={client} />
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
