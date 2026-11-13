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
  useChannelAudio,
  usePairedAccount,
  usePairedClient,
  usePairedRequests,
  usePairedSubtitles,
} from "@loomarr/paired-client";
import { NativePlayerView, PairedNativeImage, useShellTune } from "@loomarr/player/native";
import type { ApprovalBrowser } from "@loomarr/ui";
import {
  ClientShell,
  DeviceSheet,
  GuideJourney,
  PairingShell,
  PlayerAudio,
  PlayerBottomSheet,
  PlayerChrome,
  PlayerInfo,
  PlayerMore,
  PlayerOptions,
  PlayerSheet,
  playerChromeFromGuide,
  RequestsJourney,
  ScreenHeader,
  StatePanel,
  surfCardsFromGuide,
  useClientDestination,
  useSurfCards,
  watchingScheduleTimeMs,
} from "@loomarr/ui";
import Constants from "expo-constants";
import * as ScreenOrientation from "expo-screen-orientation";
import * as SecureStore from "expo-secure-store";
import { StatusBar } from "expo-status-bar";
import * as WebBrowser from "expo-web-browser";
import { type ReactNode, useEffect, useMemo, useState } from "react";
import { BackHandler, Platform, Pressable, StyleSheet, useWindowDimensions, View } from "react-native";
import { nativeApp } from "../src/native-app";

const credentialStore = createPairingCredentialStore({
  deleteItem: SecureStore.deleteItemAsync,
  getItem: SecureStore.getItemAsync,
  setItem: SecureStore.setItemAsync,
});

// What the device remembers: the channel last watched (#23), which the Guide starts on and Watching
// tunes first, and the viewer's subtitles.
const deviceStorage = { getItem: SecureStore.getItemAsync, setItem: SecureStore.setItemAsync };
const lastChannel = createLastChannelStore(deviceStorage);

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

// The app is upright everywhere but Watching. iOS launches upright from Info.plist (the plugin's
// initialOrientation); Android has no such key, so the lock is also taken as the bundle loads.
const lockUpright = () =>
  void ScreenOrientation.lockAsync(ScreenOrientation.OrientationLock.PORTRAIT_UP).catch(() => undefined);
lockUpright();

// On its side, the chrome is over the picture and leaves it bare after this long without a touch,
// while playing and with nothing open (§9.1), as the television's does.
const CHROME_IDLE_MS = 5_000;

/** What a Watching key opens as a sheet: Subtitles or Audio (utilities), More, or Info (from More). */
type WatchingSheet = "audio" | "info" | "more" | "subtitles";

// Watching turns with the phone while it is mounted, and puts the app upright again as it goes.
const useTurnWithPhone = () => {
  useEffect(() => {
    void ScreenOrientation.unlockAsync().catch(() => undefined);
    return lockUpright;
  }, []);
};

// Watching (§9, variant J): full screen, the same chrome the web draws. Held upright, the picture is
// a strip mid-screen, with a toolbar above it (Leave and the rating, then the utilities) and the band
// and keys below it. On its side, the picture fills the screen and the chrome comes and goes over it. Leave goes back to the Guide; More opens the schedule, Go live, Start
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
  const {
    catalogState,
    channelAudio,
    channelSubtitles,
    controller,
    guideSnapshot,
    refresh,
    snapshot,
    transport,
  } = client;
  const haptic = useHaptic();
  const [nowMs, setNowMs] = useState(Date.now);
  useEffect(() => {
    const timer = setInterval(() => setNowMs(Date.now()), 30_000);
    return () => clearInterval(timer);
  }, []);
  const [sheet, setSheet] = useState<WatchingSheet | undefined>(undefined);
  const closeSheet = () => setSheet(undefined);
  const toggleSheet = (name: WatchingSheet) => () => setSheet((open) => (open === name ? undefined : name));
  // Android's Back closes an open sheet first. It is subscribed while one is open, after the shell's
  // own Back, and React Native asks the newest listener first; without it, Back from Watching leaves
  // the app with the sheet still open.
  useEffect(() => {
    if (!sheet) return;
    const subscription = BackHandler.addEventListener("hardwareBackPress", () => {
      setSheet(undefined);
      return true;
    });
    return () => subscription.remove();
  }, [sheet]);
  const audioOptions = useChannelAudio(channelAudio, snapshot.channel?.id, sheet === "audio");
  const subtitles = usePairedSubtitles(channelSubtitles, deviceStorage, snapshot);
  useTurnWithPhone();
  const { height, width } = useWindowDimensions();
  const onSide = width > height;
  // On its side the chrome comes and goes: a tap on the picture toggles it, and it leaves after a
  // while untouched. Upright it is always there, as nothing covers the picture.
  const [shown, setShown] = useState(true);
  const [touches, setTouches] = useState(0);
  const touched = () => setTouches((n) => n + 1);
  const chrome = playerChromeFromGuide({ layout: guideSnapshot.layout, nowMs, snapshot });
  // Flipping (§9.2): each Channel key tunes at once, and the cards ride along in the band's place.
  const surfCards = useMemo(
    () => surfCardsFromGuide(snapshot.catalog, guideSnapshot.layout, nowMs),
    [snapshot.catalog, guideSnapshot.layout, nowMs],
  );
  const surf = useSurfCards(
    surfCards,
    snapshot.channel?.id,
    snapshot.tuneReason === "step" ? snapshot.channel?.id : undefined,
  );
  const formatTime = (ms: number) => formatGuideTime(ms, guideSnapshot.layout?.timezone);
  const live = snapshot.livePlayback ?? LIVE;
  const playing = snapshot.status === "playing";
  // A tune under way is a picture on its way, so the key offers Pause, as J draws it and the web does.
  const keyPlaying = playing || snapshot.status === "tuning";
  const previous = snapshot.catalog.find(({ id }) => id === snapshot.previousChannelId);
  const failure = catalogState.error ?? (snapshot.status === "failed" ? snapshot.error : undefined);
  const step = (direction: -1 | 1) => () => {
    haptic.selection();
    void controller.step(direction);
  };
  // `touches` is read so each touch restarts the wait.
  useEffect(() => {
    if (!onSide || !shown || sheet || !playing || touches < 0) return;
    const timer = setTimeout(() => setShown(false), CHROME_IDLE_MS);
    return () => clearTimeout(timer);
  }, [onSide, shown, sheet, playing, touches]);
  const visible = !onSide || shown || sheet !== undefined;

  // What a key opens (§9.5, variant J): held upright, a sheet rising from the bottom over the picture
  // undimmed; on its side, the right-hand sheet, as a touch screen's web player has it.
  const sheetOf = (title: string, body: ReactNode) =>
    onSide && snapshot.channel ? (
      <PlayerSheet
        channel={{ name: snapshot.channel.name, number: String(snapshot.channel.number) }}
        density="touch"
        onClose={closeSheet}
        title={title}
      >
        {body}
      </PlayerSheet>
    ) : (
      <PlayerBottomSheet onClose={closeSheet} title={title}>
        {body}
      </PlayerBottomSheet>
    );

  const sheetFor = (name: WatchingSheet) => {
    if (!chrome || !snapshot.channel) return undefined;
    const { info, segments, startOverMs, window } = chrome;
    // Subtitles: a choice leaves the sheet open, so the line changes on the picture above it.
    if (name === "subtitles")
      return sheetOf(
        "Subtitles",
        <PlayerOptions
          density="touch"
          label="Subtitles"
          onSelect={subtitles.choose}
          options={subtitles.options}
        />,
      );
    // Audio: the channel's language, read-only (an admin sets it on the channel's page).
    if (name === "audio")
      return sheetOf("Audio", <PlayerAudio density="touch" options={audioOptions ?? []} />);
    if (name === "info")
      return info
        ? sheetOf("Info", <PlayerInfo density="touch" formatTime={formatTime} info={info} />)
        : undefined;
    return sheetOf(
      "More",
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
      />,
    );
  };

  return (
    <View style={{ backgroundColor: "black", flex: 1 }}>
      {/* The picture, letterboxed and centred: a strip mid-screen upright, the whole screen on its side. */}
      <NativePlayerView
        style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}
        transport={transport}
      />
      {/* On its side: no status bar over the picture, and a tap on the picture shows or hides the chrome. */}
      <StatusBar hidden={onSide} style="light" />
      {onSide && !failure ? (
        <Pressable
          accessibilityLabel={visible ? "Hide controls" : "Show controls"}
          accessibilityRole="button"
          onPress={() => {
            setShown(!visible);
            touched();
          }}
          style={StyleSheet.absoluteFill}
        />
      ) : null}
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
        // Any touch in the chrome (a key, a sheet's row) restarts the wait before it leaves.
        <View onTouchStart={touched} pointerEvents="box-none" style={StyleSheet.absoluteFill}>
          <PlayerChrome
            band={chrome.band}
            captions={subtitles.captions}
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
              onMore: toggleSheet("more"),
              onTogglePlay: keyPlaying ? controller.pause : () => void controller.play(),
              playing: keyPlaying,
              pointer: false,
            }}
            layout={onSide ? "overlay" : "portrait"}
            live={live}
            onGoLive={() => void controller.goLive()}
            onLeave={onLeave}
            {...(surf && snapshot.channel
              ? {
                  // While flipping the band is down, so the readout names the channel (§9.2).
                  readout: {
                    digits: String(snapshot.channel.number),
                    name: snapshot.channel.name,
                    quiet: true,
                  },
                  surf,
                }
              : {})}
            sheet={sheet ? sheetFor(sheet) : undefined}
            utilities={{
              audioOpen: sheet === "audio",
              density: "touch",
              onAudio: toggleSheet("audio"),
              onSubtitles: toggleSheet("subtitles"),
              pointer: false,
              subtitlesOn: subtitles.on,
              subtitlesOpen: sheet === "subtitles",
            }}
            visible={visible}
          />
        </View>
      ) : catalogState.loading ? (
        <View style={{ flex: 1, justifyContent: "center", paddingHorizontal: PHONE_GUTTER }}>
          <StatePanel density="touch" kind="loading" title="Loading channels" />
        </View>
      ) : snapshot.catalog.length === 0 ? (
        // Nothing on air to tune (#3836): the Guide's own empty state, with the way back Leave would
        // have been, since Leave lives in the chrome and Watching has no tab bar.
        <View style={{ flex: 1, justifyContent: "center", paddingHorizontal: PHONE_GUTTER }}>
          <StatePanel
            action={{ label: "Back to the Guide", onPress: onLeave }}
            density="touch"
            description="No playable channels are available yet."
            kind="empty"
            title="No channels on air"
          />
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
  const { controller, guide, guideSnapshot, myChannelsSnapshot, snapshot } = client;
  // Opening Watching with nothing tuned yet resumes the channel the device last watched, or, on a
  // device that has watched nothing, the lineup's first channel (#3836): Watching has nothing to show
  // without a channel, and its Leave is in the chrome a channel brings.
  const { restoredChannelId } = snapshot;
  const openingChannelId = restoredChannelId ?? (snapshot.channel ? undefined : snapshot.catalog[0]?.id);
  useEffect(() => {
    if (active === "watching" && openingChannelId) void controller.tuneChannel(openingChannelId);
  }, [active, controller, openingChannelId]);
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
            timeZone={guideSnapshot.layout?.timezone}
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
