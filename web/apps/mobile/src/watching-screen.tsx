import { serverNowOf } from "@loomarr/core/channel-timeline";
import { formatGuideTime } from "@loomarr/core/guide";
import type { PairingCredential } from "@loomarr/core/pairing";
import { Screen } from "@loomarr/design-system";
import { useHaptic } from "@loomarr/design-system/haptics";
import {
  type NativePlayerTransport,
  NativePlayerView,
  type PairedClient,
  pairedNativeImageSource,
  streamRows,
  useNativePlayerChoices,
  useSampledPlaybackFacts,
  useStationIdent,
} from "@loomarr/player/native";
import {
  PlaybarStillSourceProvider,
  PlayerCaption,
  PlayerChrome,
  PlayerCover,
  PlayerInfo,
  PlayerPlaybackInfo,
  PlayerPortrait,
  PlayerSettings,
  PlayerSheet,
  PlayerStationId,
  playbarFromTimeline,
  StatePanel,
  surfPreviousChannel,
  useAutoHideControls,
  useFrameTap,
} from "@loomarr/ui";
import { lockAsync, OrientationLock, unlockAsync } from "expo-screen-orientation";
import * as SecureStore from "expo-secure-store";
import { StatusBar } from "expo-status-bar";
import { type ReactNode, useEffect, useMemo, useState } from "react";
import { Pressable, useWindowDimensions, View } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

// Watching (#2127, the §9 chrome's portrait form): the picture edge to edge, and under it the panel
// the web phone draws — the way out and the live state, the lower third and the transport, the
// filmstrip, surfing and the favourite, and the Info, Settings or Playback info sheet in the panel's place. It
// reads the tuned channel's timeline and subtitles the way every client does, and seeks through the
// player controller.

/** ±10 s, as on every player (#2088 decision 2). */
const SEEK_STEP_MS = 10_000;

/** The viewer's audio and subtitles, remembered in the phone's keychain. */
const preferenceStorage = { getItem: SecureStore.getItemAsync, setItem: SecureStore.setItemAsync };

// PLAYBACK INFO (#2088 decision 13): the Stream rows, read from the player each second while open.
// The platform picks the rung itself, so the quality is always Auto.
const PlaybackInfo = ({
  lagSeconds,
  transport,
}: {
  lagSeconds: number;
  transport: NativePlayerTransport;
}) => {
  const facts = useSampledPlaybackFacts(transport.playbackFacts);
  return <PlayerPlaybackInfo density="touch" stream={streamRows({ auto: true, facts, lagSeconds })} />;
};

interface WatchingScreenProps {
  client: PairedClient;
  credential: PairingCredential;
  /** The shell's tab bar, docked under the portrait panel; landscape leaves it out. */
  footer: ReactNode;
  onLeave: () => void;
}

const WatchingScreen = ({ client, credential, footer, onLeave }: WatchingScreenProps) => {
  const { catalogState, controller, myChannels, myChannelsSnapshot, refresh, snapshot, timeline, transport } =
    client;
  const haptic = useHaptic();
  const [sheet, setSheet] = useState<"info" | "playback-info" | "settings">();
  const toggleSheet = (next: "info" | "settings") => setSheet((open) => (open === next ? undefined : next));

  const channel = snapshot.channel;
  const channelId = channel?.id;
  const playing = snapshot.status === "playing";

  const ident = useStationIdent(snapshot);

  // A new channel closes the sheet: it described the one left behind.
  // biome-ignore lint/correctness/useExhaustiveDependencies: the channel is the trigger, not an input.
  useEffect(() => setSheet(undefined), [channelId]);

  // Landscape's overlay shows and hides like every player's: hidden after a while playing, held while
  // a sheet is open, and cleared at once for the station-ID card.
  const screen = useWindowDimensions();
  const insets = useSafeAreaInsets();
  const landscape = screen.width > screen.height;
  // Watching alone follows the phone round; leaving it puts the app back in portrait.
  useEffect(() => {
    void unlockAsync();
    return () => void lockAsync(OrientationLock.PORTRAIT_UP);
  }, []);
  const controls = useAutoHideControls(playing);
  const { hideControls, holdControls } = controls;
  useEffect(() => {
    if (!sheet) return;
    holdControls.hold(true);
    return () => holdControls.hold(false);
  }, [sheet, holdControls]);
  useEffect(() => {
    if (ident) hideControls();
  }, [ident, hideControls]);

  const stillSource = useMemo(() => (uri: string) => pairedNativeImageSource(credential, uri), [credential]);

  const failure = catalogState.error ?? (snapshot.status === "failed" ? snapshot.error : undefined);
  const live = snapshot.livePlayback ?? {
    lagSeconds: 0,
    mode: "live" as const,
    noticeRevision: 0,
    viewerTimeMs: Date.now(),
  };
  // Settings' quality, audio and subtitles, and the caption's lines at the playhead.
  const choices = useNativePlayerChoices({
    client,
    playingChannelId: playing ? channelId : undefined,
    storage: preferenceStorage,
    viewerTimeMs: live.viewerTimeMs,
  });
  const { captionLines } = choices;

  const nowMs = timeline ? serverNowOf(timeline) : Date.now();
  const chrome = channel
    ? playbarFromTimeline({
        airings: timeline?.airings ?? [],
        channel: { name: channel.name, number: String(channel.number) },
        live,
        nowMs,
        playheadMs: live.viewerTimeMs,
      })
    : undefined;
  const startOverMs = chrome?.startOverMs;
  const info = chrome?.info;
  const channelLabel = channel ? `CH ${channel.number} · ${channel.name}` : "";
  const timeZone = client.guideSnapshot.layout?.timezone;
  const formatTime = (ms: number) => formatGuideTime(ms, timeZone);
  const previousId = surfPreviousChannel(
    channelId,
    myChannelsSnapshot.recentIds,
    snapshot.catalog.map(({ id }) => id),
  );
  const isFavourite = channelId !== undefined && myChannelsSnapshot.favouriteIds.includes(channelId);
  const retry = () => (catalogState.error ? refresh() : void controller.retry());
  const seekBy = (ms: number) => controller.seek(live.viewerTimeMs + ms);
  const tap = useFrameTap({
    ...controls,
    live: true,
    onStep: (direction) => seekBy(direction * SEEK_STEP_MS),
  });

  // One sheet for both forms: in the portrait panel's place, or over the picture's right edge.
  const openSheet =
    sheet === "info" && info ? (
      <PlayerSheet
        channelLabel={channelLabel}
        density="touch"
        icon="info"
        inline={!landscape}
        onClose={() => setSheet(undefined)}
        title="Info"
      >
        <PlayerInfo density="touch" formatTime={formatTime} info={info} />
      </PlayerSheet>
    ) : sheet === "settings" ? (
      <PlayerSheet
        channelLabel={channelLabel}
        density="touch"
        icon="settings"
        inline={!landscape}
        onClose={() => setSheet(undefined)}
        title="Settings"
      >
        <PlayerSettings
          audio={choices.audio}
          density="touch"
          onPlaybackInfo={() => setSheet("playback-info")}
          quality={choices.quality}
          subtitles={choices.subtitles}
        />
      </PlayerSheet>
    ) : sheet === "playback-info" ? (
      <PlayerSheet
        channelLabel={channelLabel}
        density="touch"
        icon="info"
        inline={!landscape}
        onClose={() => setSheet(undefined)}
        title="Playback info"
      >
        <PlaybackInfo lagSeconds={live.lagSeconds} transport={transport} />
      </PlayerSheet>
    ) : undefined;

  const playbar =
    chrome && timeline
      ? {
          onSeek: controller.seek,
          playheadMs: live.viewerTimeMs,
          segments: chrome.segments,
          window: chrome.window,
        }
      : undefined;
  const transportProps = {
    density: "touch" as const,
    infoOpen: sheet === "info",
    live,
    // The phone's own buttons own the volume; a touch transport draws none.
    muted: false,
    onGoLive: () => void controller.goLive(),
    onInfo: info ? () => toggleSheet("info") : undefined,
    onLeave,
    onSeekBack: () => seekBy(-SEEK_STEP_MS),
    onSeekForward: () => seekBy(SEEK_STEP_MS),
    onSettings: () => toggleSheet("settings"),
    onStartOver: startOverMs === undefined ? undefined : () => controller.seek(startOverMs),
    onSubtitles: choices.nextSubtitles,
    onTogglePlay: () => (playing ? controller.pause() : void controller.play()),
    onToggleMute: () => undefined,
    onVolume: () => undefined,
    playing,
    pointer: false,
    settingsOpen: sheet === "settings" || sheet === "playback-info",
    subtitlesOn: choices.subtitlesOn,
    volume: 1,
  };

  const errorCover = failure ? (
    <View style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0, zIndex: 40 }}>
      <PlayerCover
        channelLabel={channelLabel}
        density="touch"
        kind="error"
        message={failure}
        onLeave={onLeave}
        onRetry={retry}
        title="Can't play this channel"
      />
    </View>
  ) : undefined;

  // Landscape (#2088 decision 12): the picture full-bleed under the same overlay as every other
  // player. A tap is the controls', a double-tap on a side steps 10 s.
  if (landscape) {
    return (
      <View style={{ backgroundColor: "black", flex: 1 }}>
        <StatusBar hidden />
        <NativePlayerView style={{ flex: 1 }} transport={transport} />
        <Pressable
          accessibilityLabel={controls.controlsShown ? "Hide the controls" : "Show the controls"}
          accessibilityRole="button"
          onPress={(e) => tap(e.nativeEvent.locationX / screen.width)}
          style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}
        />
        {/* The picture runs under the system bars; what sits on it keeps clear of them. */}
        <View
          pointerEvents="box-none"
          style={{
            bottom: insets.bottom,
            left: insets.left,
            position: "absolute",
            right: insets.right,
            top: insets.top,
          }}
        >
          {ident ? null : <PlayerCaption density="touch" lines={captionLines} />}
          {errorCover ??
            (chrome ? (
              <PlaybarStillSourceProvider value={stillSource}>
                <PlayerChrome
                  band={chrome.band}
                  density="touch"
                  formatTime={formatTime}
                  ident={ident}
                  playbar={playbar}
                  sheet={openSheet}
                  transport={transportProps}
                  visible={controls.controlsShown && snapshot.status !== "tuning"}
                />
              </PlaybarStillSourceProvider>
            ) : catalogState.loading ? (
              <StatePanel density="touch" kind="loading" title="Loading channels" />
            ) : null)}
        </View>
      </View>
    );
  }

  return (
    <Screen density="touch" flush footer={footer}>
      <View style={{ backgroundColor: "black", flex: 1 }}>
        <View style={{ aspectRatio: 16 / 9, backgroundColor: "black", width: "100%" }}>
          <NativePlayerView style={{ flex: 1 }} transport={transport} />
          {errorCover ??
            (ident && chrome ? (
              <View
                pointerEvents="none"
                style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}
              >
                <PlayerStationId band={chrome.band} density="touch" />
              </View>
            ) : (
              <PlayerCaption density="touch" lines={captionLines} />
            ))}
        </View>
        {chrome ? (
          <PlaybarStillSourceProvider value={stillSource}>
            <PlayerPortrait
              band={chrome.band}
              density="touch"
              filmstrip={playbar}
              formatTime={formatTime}
              sheet={openSheet}
              surf={{
                canPrevious: previousId !== undefined,
                canSurf: snapshot.catalog.length > 1,
                favourite: isFavourite,
                onChannelDown: () => {
                  haptic.selection();
                  void controller.step(-1);
                },
                onChannelUp: () => {
                  haptic.selection();
                  void controller.step(1);
                },
                onFavourite: () => channelId && void myChannels.setFavourite(channelId, !isFavourite),
                onPrevious: () => previousId && void controller.tuneChannel(previousId),
              }}
              transport={transportProps}
            />
          </PlaybarStillSourceProvider>
        ) : catalogState.loading ? (
          <StatePanel density="touch" kind="loading" title="Loading channels" />
        ) : failure ? (
          <StatePanel
            action={{ label: "Try again", onPress: retry }}
            density="touch"
            description={failure}
            kind="error"
            title="Can't play this channel"
          />
        ) : null}
      </View>
    </Screen>
  );
};

export type { WatchingScreenProps };
export { WatchingScreen };
