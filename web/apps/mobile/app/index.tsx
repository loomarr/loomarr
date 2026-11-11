import { formatGuideTime } from "@loomarr/core/guide";
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
import { useAppForeground, usePairingBreadcrumbs, useShellCrashReporting } from "@loomarr/native-app";
import {
  createLastChannelStore,
  type PairedClient,
  phoneClientOptions,
  retryPairedClient,
  usePairedAccount,
  usePairedClient,
  usePairedRequests,
} from "@loomarr/paired-client";
import { NativePlayerView, PairedNativeImage, useShellTune } from "@loomarr/player/native";
import type { ApprovalBrowser } from "@loomarr/ui";
import {
  ClientShell,
  DeviceSheet,
  GuideJourney,
  PairingShell,
  PlayerChrome,
  PlayerInfo,
  PlayerMore,
  PlayerSheet,
  playerChromeFromGuide,
  RequestsJourney,
  ScreenHeader,
  StatePanel,
  useClientDestination,
  watchingScheduleTimeMs,
} from "@loomarr/ui";
import Constants from "expo-constants";
import * as SecureStore from "expo-secure-store";
import { StatusBar } from "expo-status-bar";
import * as WebBrowser from "expo-web-browser";
import { useEffect, useMemo, useState } from "react";
import { Platform, View } from "react-native";
import { nativeApp } from "../src/native-app";

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

const LIVE = { lagSeconds: 0, mode: "live" } as const;

// Watching (§9, variant J, the phone held upright): full screen, the picture a strip mid-screen, a
// toolbar above it (Leave and the rating, then the utilities) and the band and keys below it, the
// same chrome the web draws. Leave goes back to the Guide; More opens the schedule, Go live, Start
// over, Info and Channels (the Guide, where the favourites are).
const WatchingScreen = ({
  client,
  onLeave,
  onOpenChannels,
}: {
  client: PairedClient;
  onLeave: () => void;
  onOpenChannels: () => void;
}) => {
  const { catalogState, controller, guideSnapshot, refresh, snapshot, transport } = client;
  const haptic = useHaptic();
  const [nowMs, setNowMs] = useState(Date.now);
  useEffect(() => {
    const timer = setInterval(() => setNowMs(Date.now()), 30_000);
    return () => clearInterval(timer);
  }, []);
  const [sheet, setSheet] = useState<"info" | "more" | undefined>(undefined);
  const closeSheet = () => setSheet(undefined);
  const chrome = playerChromeFromGuide({ layout: guideSnapshot.layout, nowMs, snapshot });
  const formatTime = (ms: number) => formatGuideTime(ms, guideSnapshot.layout?.timezone);
  const live = snapshot.livePlayback ?? LIVE;
  const playing = snapshot.status === "playing";
  const previous = snapshot.catalog.find(({ id }) => id === snapshot.previousChannelId);
  const failure = catalogState.error ?? (snapshot.status === "failed" ? snapshot.error : undefined);
  const step = (direction: -1 | 1) => () => {
    haptic.selection();
    void controller.step(direction);
  };

  const sheetFor = (name: "info" | "more") => {
    if (!chrome || !snapshot.channel) return undefined;
    const channel = { name: snapshot.channel.name, number: String(snapshot.channel.number) };
    const { info, segments, startOverMs, window } = chrome;
    if (name === "info")
      return info ? (
        <PlayerSheet channel={channel} density="touch" onClose={closeSheet} title="Info">
          <PlayerInfo density="touch" formatTime={formatTime} info={info} />
        </PlayerSheet>
      ) : undefined;
    return (
      <PlayerSheet channel={channel} density="touch" onClose={closeSheet} title="More">
        <PlayerMore
          channels={{
            onPress: () => {
              closeSheet();
              onOpenChannels();
            },
          }}
          density="touch"
          formatTime={formatTime}
          goLive={
            live.mode === "live"
              ? undefined
              : {
                  detail: formatTime(nowMs),
                  onPress: () => {
                    closeSheet();
                    void controller.goLive();
                  },
                }
          }
          info={info ? { onPress: () => setSheet("info") } : undefined}
          schedule={
            segments.length > 0
              ? {
                  onSeek: (wallClockMs) => void controller.seek(wallClockMs),
                  playheadMs: watchingScheduleTimeMs(snapshot, nowMs),
                  segments,
                  window,
                }
              : undefined
          }
          startOver={
            startOverMs === undefined || !info
              ? undefined
              : {
                  detail: info.title,
                  onPress: () => {
                    closeSheet();
                    void controller.seek(startOverMs);
                  },
                }
          }
        />
      </PlayerSheet>
    );
  };

  return (
    <View style={{ backgroundColor: "black", flex: 1 }}>
      {/* The picture, letterboxed and centred: a strip mid-screen. */}
      <NativePlayerView
        style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}
        transport={transport}
      />
      {failure ? (
        <View style={{ flex: 1, justifyContent: "center" }}>
          <StatePanel
            action={{
              label: "Try again",
              onPress: () => retryPairedClient({ catalogState, controller, refresh }),
            }}
            density="touch"
            description={failure}
            kind="error"
            title="Can't play this channel"
          />
        </View>
      ) : chrome ? (
        <PlayerChrome
          band={chrome.band}
          density="touch"
          formatTime={formatTime}
          keys={{
            density: "touch",
            last: previous
              ? {
                  number: String(previous.number),
                  onPress: () => {
                    haptic.selection();
                    void controller.tuneChannel(previous.id);
                  },
                }
              : undefined,
            moreOpen: sheet === "more",
            onChannelDown: snapshot.catalog.length > 1 ? step(-1) : undefined,
            onChannelUp: snapshot.catalog.length > 1 ? step(1) : undefined,
            onMore: () => setSheet((open) => (open === "more" ? undefined : "more")),
            onTogglePlay: playing ? controller.pause : () => void controller.play(),
            playing,
            pointer: false,
          }}
          layout="portrait"
          live={live}
          onGoLive={() => void controller.goLive()}
          onLeave={onLeave}
          sheet={sheet ? sheetFor(sheet) : undefined}
          utilities={{ density: "touch", pointer: false }}
          visible
        />
      ) : catalogState.loading ? (
        <View style={{ flex: 1, justifyContent: "center" }}>
          <StatePanel density="touch" kind="loading" title="Loading channels" />
        </View>
      ) : null}
    </View>
  );
};

const MobileShell = ({ credential, session }: { credential: PairingCredential; session: PairingSession }) => {
  const [active, setActive] = useClientDestination("guide");
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
    ...phoneClientOptions,
    credential,
    diagnostics: nativeApp.diagnostics,
    lastChannel,
    session,
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
  const tuneFromShell = useShellTune(controller, active === "watching");
  // Pending crash reports (last launch's included) go to this server once paired.
  useShellCrashReporting(credential, session, active);
  // Watching a channel from outside it (a Guide pick, a finished request's Open channel).
  const watchChannel = (channelId: string) => {
    haptic.selection();
    tuneFromShell(channelId);
    setActive("watching");
  };
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
        // Watching is full screen (§9.7): no tab bar, and Leave goes back to the Guide.
        () => (
          <WatchingScreen
            client={client}
            onLeave={() => setActive("guide")}
            onOpenChannels={() => setActive("guide")}
          />
        )
      )}
    </ClientShell>
  );
};

const Index = () => {
  // Discovery browses only while the connect screen is in the foreground (design "Finding a server").
  // Android phones keep manual entry until their Bonjour path pairs to the public URL as well.
  const appForeground = useAppForeground();
  const discovery = useMemo(() => (Platform.OS === "ios" ? createNativeServerDiscovery() : undefined), []);
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
  usePairingBreadcrumbs(session);
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
