import { ClientCrashBreadcrumbCategory } from "@loomarr/api/models/clientCrashBreadcrumbCategory";
import type { PairingCredential } from "@loomarr/core/pairing";
import {
  createPairingCredentialStore,
  createPairingTransport,
  PairingSession,
  validatePairingCredential,
} from "@loomarr/core/pairing";
import type { RequestsController } from "@loomarr/core/requests";
import { BottomSheet, PHONE_GUTTER } from "@loomarr/design-system";
import { type HapticFeedback, useHaptic } from "@loomarr/design-system/haptics";
import { createNativeServerDiscovery } from "@loomarr/lan-discovery-native";
import { useCrashReportUpload } from "@loomarr/native-app";
import {
  createLastChannelStore,
  PairedNativeImage,
  usePairedAccount,
  usePairedClient,
  usePairedRequests,
  useShellPause,
} from "@loomarr/player/native";
import type { ApprovalBrowser, ClientDestination } from "@loomarr/ui";
import {
  ClientShell,
  clientBackDestination,
  DeviceSheet,
  GuideJourney,
  PairingShell,
  RequestsJourney,
  ScreenHeader,
} from "@loomarr/ui";
import Constants from "expo-constants";
import * as SecureStore from "expo-secure-store";
import { StatusBar } from "expo-status-bar";
import * as WebBrowser from "expo-web-browser";
import { useEffect, useMemo, useState } from "react";
import { AppState, BackHandler, Platform, View } from "react-native";
import { nativeApp } from "../src/native-app";
import { WatchingScreen } from "../src/watching-screen";

const credentialStore = createPairingCredentialStore({
  deleteItem: SecureStore.deleteItemAsync,
  getItem: SecureStore.getItemAsync,
  setItem: SecureStore.setItemAsync,
});

// The channel last watched (#23): the Guide starts on it and Watching tunes it first.
const lastChannel = createLastChannelStore({
  getItem: SecureStore.getItemAsync,
  setItem: SecureStore.setItemAsync,
});

// The phone approves itself on the server's `/pair` page, in a browser sheet over the app (an
// SFSafariViewController, or a Custom Tab on Android); the pairing shell dismisses it once paired.
// ⚠ Android has no dismissBrowser: Expo returns undefined there, not a promise, and the `.catch` on
// it threw as every Android phone finished pairing. The viewer leaves the Custom Tab themselves.
const approvalBrowser: ApprovalBrowser = {
  close: () => void Promise.resolve(WebBrowser.dismissBrowser()).catch(() => undefined),
  name: Platform.OS === "ios" ? "Safari" : "your browser",
  open: (url) => {
    void WebBrowser.openBrowserAsync(url, {
      presentationStyle: WebBrowser.WebBrowserPresentationStyle.PAGE_SHEET,
    });
  },
};
// A request decided (approved or denied) taps success; a refused one stays silent.
const withDecisionHaptic = (controller: RequestsController, haptic: HapticFeedback): RequestsController => {
  const tapWhenDone =
    <Args extends unknown[], Result extends { kind: string }>(decide: (...args: Args) => Promise<Result>) =>
    async (...args: Args) => {
      const result = await decide(...args);
      if (result.kind === "done") haptic.success();
      return result;
    };
  return {
    ...controller,
    approve: tapWhenDone(controller.approve),
    approveFillerPull: tapWhenDone(controller.approveFillerPull),
    approveFillerPulls: tapWhenDone(controller.approveFillerPulls),
    approveMany: tapWhenDone(controller.approveMany),
    deny: tapWhenDone(controller.deny),
  };
};

// The selected programme docks as the iPhone's sheet (#1659 mock 5d) or Android's strip (5f).
const guideDock = Platform.OS === "ios" ? BottomSheet : "strip";

const MobileShell = ({ credential, session }: { credential: PairingCredential; session: PairingSession }) => {
  const [active, setActive] = useState<ClientDestination>("guide");
  // The Device sheet: opened from the Guide's and Requests' header buttons, docked above the tab bar.
  const [deviceOpen, setDeviceOpen] = useState(false);
  const account = usePairedAccount({ credential, session });
  // The version and build app.config.cjs stamps into the binary (expo-constants has no native-version field).
  const appVersion = Constants.expoConfig?.version;
  const appBuild = Constants.expoConfig?.ios?.buildNumber;
  const deviceAction = {
    accessibilityLabel: "Device and account",
    icon: "account",
    onPress: () => setDeviceOpen(true),
  } as const;
  // The phone opens on its Guide with no picture, so it tunes only when the viewer picks a channel.
  const client = usePairedClient({
    credential,
    diagnostics: nativeApp.diagnostics,
    guideWindowMinutes: 120,
    initialTune: "none",
    lastChannel,
    session,
    timeline: true,
  });
  const { controller, guide, myChannelsSnapshot, snapshot } = client;
  // Opening Watching with nothing tuned yet resumes the channel the device last watched.
  const { restoredChannelId } = snapshot;
  useEffect(() => {
    if (active === "watching" && restoredChannelId) void controller.tuneChannel(restoredChannelId);
  }, [active, controller, restoredChannelId]);
  // Requests loads with the app so an admin's tab badge counts before they open it.
  const requests = usePairedRequests({ credential, session });
  const haptic = useHaptic();
  const requestsController = useMemo(
    () => withDecisionHaptic(requests.controller, haptic),
    [haptic, requests.controller],
  );
  // The picture is only mounted on Watching, so a stream left running elsewhere would play unseen:
  // pause when the viewer leaves Watching, and resume what the shell paused, at live, when they come back.
  const forgetShellPause = useShellPause(controller, active === "watching");
  // Pending crash reports (last launch's included) go to this server once paired.
  useCrashReportUpload(credential, session);
  useEffect(
    () => nativeApp.crashReports.recordBreadcrumb(ClientCrashBreadcrumbCategory.navigation, active),
    [active],
  );
  // Watching a channel from outside it (a Guide pick, a finished request's Open channel).
  const watchChannel = (channelId: string) => {
    haptic.selection();
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
      onNavigate={(destination) => {
        setDeviceOpen(false);
        setActive(destination);
      }}
      requestsBadge={requests.needsYouCount}
      sheet={
        deviceOpen ? (
          <DeviceSheet
            account={account}
            appVersion={appVersion ? (appBuild ? `${appVersion} (${appBuild})` : appVersion) : undefined}
            onDisconnect={() => session.disconnect()}
            onDismiss={() => setDeviceOpen(false)}
            serverUrl={credential.serverUrl}
            serverVersion={client.serverVersion}
          />
        ) : undefined
      }
    >
      {active === "requests" ? (
        // The journey docks its review sheet directly above the tab bar, so it draws the footer itself.
        (navigation) => (
          <RequestsJourney
            channels={snapshot.catalog}
            controller={requestsController}
            footer={navigation}
            headerAction={deviceAction}
            onOpenChannel={watchChannel}
          />
        )
      ) : active === "guide" ? (
        <View style={{ flex: 1 }}>
          <View style={{ paddingHorizontal: PHONE_GUTTER }}>
            <ScreenHeader actions={[deviceAction]} title="Guide" />
          </View>
          <GuideJourney
            controller={guide}
            density="touch"
            dock={guideDock}
            myChannels={myChannelsSnapshot}
            onTune={watchChannel}
            preferredChannelId={snapshot.channel?.id ?? restoredChannelId}
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
      ) : (
        // Watching draws the tab bar itself: in landscape the picture takes the whole screen.
        (navigation) => (
          <WatchingScreen
            client={client}
            credential={credential}
            footer={navigation}
            onLeave={() => setActive("guide")}
          />
        )
      )}
    </ClientShell>
  );
};

const Index = () => {
  // Discovery browses only while the connect screen is in the foreground (design "Finding a server").
  // Android phones keep manual entry until their Bonjour path pairs to the public URL as well.
  const [appForeground, setAppForeground] = useState(AppState.currentState === "active");
  const discovery = useMemo(() => (Platform.OS === "ios" ? createNativeServerDiscovery() : undefined), []);
  useEffect(() => {
    const subscription = AppState.addEventListener("change", (state) => {
      setAppForeground(state === "active");
    });
    return () => subscription.remove();
  }, []);
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
  // The success tap belongs to the moment pairing completes: awaiting approval, then paired. A launch
  // that restores a saved credential goes loading → paired with nothing pressed, so it stays silent.
  const haptic = useHaptic();
  useEffect(() => {
    let previous = session.snapshot().status;
    const unsubscribe = session.subscribe((state) => {
      if (previous === "awaiting-approval" && state.status === "paired") haptic.success();
      previous = state.status;
    });
    return () => void unsubscribe();
  }, [haptic, session]);
  useEffect(() => {
    const unsubscribe = session.subscribe(({ status }) =>
      nativeApp.crashReports.recordBreadcrumb(ClientCrashBreadcrumbCategory.lifecycle, `pairing ${status}`),
    );
    return () => void unsubscribe();
  }, [session]);
  return (
    <>
      <PairingShell
        allowServerEntry
        approvalBrowser={approvalBrowser}
        density="touch"
        discovery={discovery}
        discoveryForeground={appForeground}
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
