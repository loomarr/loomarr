import { audioChoice } from "@loomarr/core/audio-choice";
import { serverNowOf } from "@loomarr/core/channel-timeline";
import { formatGuideTime } from "@loomarr/core/guide";
import type { PairingCredential } from "@loomarr/core/pairing";
import { useHaptic } from "@loomarr/design-system/haptics";
import {
  cueLinesAt,
  type NativePlayerTransport,
  NativePlayerView,
  type PairedClient,
  pairedNativeImageSource,
  rungLabel,
  streamRows,
  subtitleLabel,
  useNativeVideoHeight,
  usePairedSubtitles,
  useSampledPlaybackFacts,
} from "@loomarr/player/native";
import {
  PlaybarStillSourceProvider,
  PlayerCaption,
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
} from "@loomarr/ui";
import { useEffect, useMemo, useRef, useState } from "react";
import { View } from "react-native";
import { useAudioPreference, useSubtitlesPreference } from "./player-preferences";

// Watching (#2127, the §9 chrome's portrait form): the picture edge to edge, and under it the panel
// the web phone draws — the way out and the live state, the lower third and the transport, the
// filmstrip, surfing and the favourite, and the Info, Settings or Playback info sheet in the panel's place. It
// reads the tuned channel's timeline and subtitles the way every client does, and seeks through the
// player controller.

/** ±10 s, as on every player (#2088 decision 2). */
const SEEK_STEP_MS = 10_000;
/** The station-ID card shows for five seconds on every settled tune (§9.1). */
const IDENT_MS = 5_000;

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
  onLeave: () => void;
}

const WatchingScreen = ({ client, credential, onLeave }: WatchingScreenProps) => {
  const {
    catalogState,
    controller,
    myChannels,
    myChannelsSnapshot,
    refresh,
    snapshot,
    timeline,
    tracks,
    transport,
  } = client;
  const haptic = useHaptic();
  const [sheet, setSheet] = useState<"info" | "playback-info" | "settings">();
  const toggleSheet = (next: "info" | "settings") => setSheet((open) => (open === next ? undefined : next));

  // Settings' audio: the programme's languages at the playhead, the viewer's remembered choice
  // between them, and the player playing it.
  const [audioPreference, setAudioPreference] = useAudioPreference();
  const audio = audioChoice(tracks, audioPreference);
  useEffect(() => transport.setAlternateAudio(audio.alternate), [audio.alternate, transport]);
  const videoHeight = useNativeVideoHeight(transport);
  const channel = snapshot.channel;
  const channelId = channel?.id;
  const playing = snapshot.status === "playing";

  // The station ID, shown once as each tune settles and dropped while the next one tunes. Keyed by
  // the tune, not by playing: resuming from pause is not a new arrival.
  const [identFor, setIdentFor] = useState<number>();
  const settledAttempt = playing ? snapshot.attemptId : undefined;
  const shownAttempt = useRef<number | undefined>(undefined);
  useEffect(() => {
    if (settledAttempt === undefined || settledAttempt === shownAttempt.current) return;
    shownAttempt.current = settledAttempt;
    setIdentFor(settledAttempt);
  }, [settledAttempt]);
  // The card's own five seconds, whatever the viewer does meanwhile.
  useEffect(() => {
    if (identFor === undefined) return;
    const timer = setTimeout(() => setIdentFor(undefined), IDENT_MS);
    return () => clearTimeout(timer);
  }, [identFor]);
  const ident = identFor !== undefined && identFor === snapshot.attemptId && snapshot.status !== "tuning";

  // A new channel closes the sheet: it described the one left behind.
  // biome-ignore lint/correctness/useExhaustiveDependencies: the channel is the trigger, not an input.
  useEffect(() => setSheet(undefined), [channelId]);

  const stillSource = useMemo(() => (uri: string) => pairedNativeImageSource(credential, uri), [credential]);

  const failure = catalogState.error ?? (snapshot.status === "failed" ? snapshot.error : undefined);
  const live = snapshot.livePlayback ?? {
    lagSeconds: 0,
    mode: "live" as const,
    noticeRevision: 0,
    viewerTimeMs: Date.now(),
  };
  // The subtitles at the playhead (#2088 decision 5): the sidecar's tracks for Settings, and the
  // chosen track's lines drawn over the picture (expo-video takes no sidecar text track).
  const [subtitlesChoice, setSubtitles] = useSubtitlesPreference();
  const subtitles = usePairedSubtitles(
    client.subtitlesSource,
    playing ? channelId : undefined,
    live.viewerTimeMs,
    subtitlesChoice,
  );
  const captionLines = cueLinesAt(subtitles.cues, live.viewerTimeMs);

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

  return (
    <View style={{ backgroundColor: "black", flex: 1 }}>
      <View style={{ aspectRatio: 16 / 9, backgroundColor: "black", width: "100%" }}>
        <NativePlayerView style={{ flex: 1 }} transport={transport} />
        {failure ? (
          <View style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}>
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
        ) : ident && chrome ? (
          <View pointerEvents="none" style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}>
            <PlayerStationId band={chrome.band} density="touch" />
          </View>
        ) : (
          <PlayerCaption density="touch" lines={captionLines} />
        )}
      </View>
      {chrome ? (
        <PlaybarStillSourceProvider value={stillSource}>
          <PlayerPortrait
            band={chrome.band}
            density="touch"
            filmstrip={
              timeline
                ? {
                    onSeek: controller.seek,
                    playheadMs: live.viewerTimeMs,
                    segments: chrome.segments,
                    window: chrome.window,
                  }
                : undefined
            }
            formatTime={formatTime}
            sheet={
              sheet === "info" && info ? (
                <PlayerSheet
                  channelLabel={channelLabel}
                  density="touch"
                  icon="info"
                  inline
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
                  inline
                  onClose={() => setSheet(undefined)}
                  title="Settings"
                >
                  <PlayerSettings
                    audio={
                      audio.options.length > 1
                        ? { onSelect: setAudioPreference, options: audio.options }
                        : { options: audio.options }
                    }
                    density="touch"
                    onPlaybackInfo={() => setSheet("playback-info")}
                    subtitles={
                      subtitles.tracks.length > 0
                        ? {
                            onSelect: (value) =>
                              setSubtitles(
                                value === "off" ? "off" : (subtitles.tracks[Number(value)] ?? "off"),
                              ),
                            options: [
                              { label: "Off", selected: subtitles.active < 0, value: "off" },
                              ...subtitles.tracks.map((track, i) => ({
                                label: subtitleLabel(track),
                                selected: i === subtitles.active,
                                value: String(i),
                              })),
                            ],
                          }
                        : undefined
                    }
                    quality={{
                      options: [
                        {
                          label: videoHeight ? `Auto · ${rungLabel(videoHeight)}` : "Auto",
                          selected: true,
                          value: "auto",
                        },
                      ],
                    }}
                  />
                </PlayerSheet>
              ) : sheet === "playback-info" ? (
                <PlayerSheet
                  channelLabel={channelLabel}
                  density="touch"
                  icon="info"
                  inline
                  onClose={() => setSheet(undefined)}
                  title="Playback info"
                >
                  <PlaybackInfo lagSeconds={live.lagSeconds} transport={transport} />
                </PlayerSheet>
              ) : undefined
            }
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
            transport={{
              infoOpen: sheet === "info",
              live,
              onGoLive: () => void controller.goLive(),
              onInfo: info ? () => toggleSheet("info") : undefined,
              onLeave,
              onSettings: () => toggleSheet("settings"),
              onSeekBack: () => seekBy(-SEEK_STEP_MS),
              onSeekForward: () => seekBy(SEEK_STEP_MS),
              onStartOver: startOverMs === undefined ? undefined : () => controller.seek(startOverMs),
              onTogglePlay: () => (playing ? controller.pause() : void controller.play()),
              playing,
              settingsOpen: sheet === "settings" || sheet === "playback-info",
            }}
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
  );
};

export type { WatchingScreenProps };
export { WatchingScreen };
