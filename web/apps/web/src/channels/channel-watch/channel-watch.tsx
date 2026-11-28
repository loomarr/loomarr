import * as channelsApi from "@loomarr/api/endpoints/channels";
import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { audioChoices } from "@loomarr/core/channel-audio";
import {
  createChannelSubtitlesSource,
  subtitleChoices,
  useChannelSubtitles,
} from "@loomarr/core/channel-subtitles";
import { formatGuideTime } from "@loomarr/core/guide";
import { type WatchRequest, watchKeyOf } from "@loomarr/core/watch-screen";
import { Layer, Stack, ThemeScope } from "@loomarr/design-system";
import {
  PlayerCaption,
  PlayerCover,
  playbarFromTimeline,
  UnmutePill,
  useWatchScreen,
  WatchScreen,
  watermarkOf,
} from "@loomarr/ui";
import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { VideoPlayer } from "@/components/loomarr/player/video-player";
import { copyText } from "@/lib/copy-text";
import { useCoarsePointer } from "@/lib/use-coarse-pointer";
import { usePlaybackDetails } from "../use-playback-details";
import { useSubtitlesPreference } from "../use-subtitles-preference";
import { useSwitchSound } from "../use-switch-sound";
import type { WatchChannels } from "../use-watch-channels";
import { useWatchHintPreference } from "../use-watch-hint-preference";
import type { WatchPlayer } from "../use-watch-player";
import { useWatchTelemetry } from "../use-watch-telemetry";
import type { WatchTune } from "../use-watch-tune";
import { ChromeBridge, type PlayerSound } from "./chrome-bridge";
import { FirstPlayMuted } from "./first-play-muted";
import { PausedCover } from "./paused-cover";
import { programmeOf } from "./programme-of";
import { qualityOptionsFor } from "./quality-options";
import { StoppedCover } from "./stopped-cover";
import { untilNowEnds } from "./until-now-ends";
import { useExpiryNotice } from "./use-expiry-notice";
import { watchMenu } from "./watch-menu";

// ChannelWatch: the full-bleed `/watch/$id` page, The Cross's watching screen over the real player
// (frontend-design §9.1, "On the web's Watch route"). The player is the shared VideoPlayer in `live`
// mode with the shared player controller bound through its `attach` seam (useWatchPlayer, owned by
// the route); it draws no controls of its own. `WatchScreen` draws everything over it from
// `@loomarr/core/watch-screen`, and this component does what the screen asks.

// Subtitles are read beside the stream with the session cookie, the same-origin request every
// member read makes.
const subtitlesSource = createChannelSubtitlesSource((input, init) => fetch(input, init));

/** Ten seconds through the live window, for `←` and `→` on the bare picture. */
const SEEK_STEP_MS = 10_000;
/** The unmute pill stands under the first-run hint, clear of it. */
const PILL_TOP = 120;
/** Subtitles stand clear of the lower third while it shows, and low on the bare picture. */
const CAPTION_AWAKE = 280;
const CAPTION_RESTING = 64;
/** The ids of the screen's own tunes that are remembered, to tell an echo from a change from outside. */
const REMEMBERED_TUNES = 8;

const fieldSelector = "input, textarea, select, [contenteditable]:not([contenteditable=false])";

interface ChannelWatchProps {
  channel: ChannelDTO;
  isAdmin: boolean;
  /** The named back control: where the viewer came from, else the Guide. */
  back: { label: string; onPress: () => void };
  /** Opens the Guide. */
  onGuide: () => void;
  /** The player and its tuner: what plays, what the viewer asked of it, and its controls. */
  player: WatchPlayer;
  /** The channels the screen lists, and the viewer's own. */
  channels: WatchChannels;
  /** The tune, which the route keeps beside the player, so it hears every tune the player begins. */
  tune: WatchTune;
}

const ChannelWatch = ({ back, channel, channels, isAdmin, onGuide, player, tune }: ChannelWatchProps) => {
  const { lineup, onTuneSettled } = channels;
  const touch = useCoarsePointer();
  const density = touch ? "touch" : "pointer";
  // The player's own <video>, caught at the attach seam, so the channel-change sound can follow the
  // player's volume and mute without VideoPlayer knowing about it.
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const playerAttach = player.attach;
  const attach = useCallback(
    (el: HTMLVideoElement) => {
      videoRef.current = el;
      return playerAttach(el);
    },
    [playerAttach],
  );
  useExpiryNotice(channel.id, player.liveTransport.state.noticeRevision);

  // Whether a picture has played on this page: the channel-change sound waits for one.
  const [heldFrame, setHeldFrame] = useState(false);
  useEffect(() => {
    if (player.status === "playing") setHeldFrame(true);
  }, [player.status]);

  // Recents record a tune once it settles, never a channel flipped past (the API's rule): once per
  // channel, on its first frame.
  const settledRef = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (player.status !== "playing" || settledRef.current === channel.id) return;
    settledRef.current = channel.id;
    onTuneSettled(channel.id);
  }, [player.status, channel.id, onTuneSettled]);

  // The first play of the session is muted, with a pill to undo it (§9.3).
  const [startedMuted, setStartedMuted] = useState(false);
  const onFirstPlayStart = useCallback(() => setStartedMuted(true), []);
  const onFirstPlayUnmuted = useCallback(() => setStartedMuted(false), []);
  const [sound, setSound] = useState<PlayerSound | undefined>(undefined);
  const soundRef = useRef(sound);
  soundRef.current = sound;

  const offAir = channel.status === "paused" || channel.status === "detached";

  // The tune follows the player: its first frame is the picture, a failure or a channel off air the
  // end of a tune-in with none.
  const { settle } = tune;
  useEffect(() => {
    if (player.status === "playing") settle(player.attemptId, "picture");
    else if (player.status === "error" || offAir) settle(player.attemptId, "none");
  }, [player.attemptId, player.status, offAir, settle]);

  // A browser may refuse to start a picture no one pressed for (its autoplay policy): the element
  // stops before its first frame, unasked. Then a press of the cover's Play starts it.
  const [refused, setRefused] = useState(false);
  const waiting = useRef({ loading: player.status === "loading", paused: player.paused });
  waiting.current = { loading: player.status === "loading", paused: player.paused };
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return undefined;
    const stopped = () => {
      if (video.paused && !video.ended && waiting.current.loading && !waiting.current.paused) setRefused(true);
    };
    const started = () => setRefused(false);
    video.addEventListener("pause", stopped);
    video.addEventListener("canplay", stopped);
    video.addEventListener("playing", started);
    return () => {
      video.removeEventListener("pause", stopped);
      video.removeEventListener("canplay", stopped);
      video.removeEventListener("playing", started);
    };
  }, []);
  useEffect(() => {
    if (player.attemptId !== undefined) setRefused(false);
  }, [player.attemptId]);

  // Settings' audio list is the tracks the airing programme actually carries, fetched, never
  // hardcoded. `enabled` gates the probe on a playing channel (a paused one has nothing to probe).
  const tracks = channelsApi.useChannelTracks(channel.id, {
    query: { enabled: !offAir && player.status === "playing", retry: false },
  });
  const audio = audioChoices(unwrap(tracks.data)?.audio ?? [], channel.policy?.playout?.audioLanguage);

  // The channel's schedule strip, for the lower third and Start over. A schedule change reads it
  // again on the channel's frame; time alone changes it as the airing on now ends, so it is read
  // again then (#3352).
  const timeline = channelsApi.useChannelTimeline(channel.id, undefined, {
    query: { enabled: !offAir, retry: false },
  });
  const timelineBody = unwrap(timeline.data);
  const airings = timelineBody?.airings ?? [];
  const serverAtProjection = timelineBody?.serverNowMs;
  const serverNowMs =
    serverAtProjection === undefined
      ? undefined
      : serverAtProjection + Math.max(0, Date.now() - timeline.dataUpdatedAt);
  const nowEndsInMs = untilNowEnds(airings, serverAtProjection);
  const { refetch: refetchTimeline } = timeline;
  useEffect(() => {
    if (offAir || nowEndsInMs === undefined) return;
    const timer = setTimeout(() => void refetchTimeline(), nowEndsInMs);
    return () => clearTimeout(timer);
  }, [offAir, nowEndsInMs, refetchTimeline]);
  const live = player.liveTransport.state;
  const viewerTimeMs = live.viewerTimeMs;
  useWatchTelemetry({
    airings,
    channelId: channel.id,
    playbackSessionId: player.playbackSessionId,
    serverNowMs,
    viewerTimeMs,
  });
  const chromeData = playbarFromTimeline({
    airings,
    channel: {
      ...(channel.decade === undefined ? {} : { decade: channel.decade }),
      era: channel.policy?.filler?.era,
      name: channel.name,
      number: String(channel.number),
      place: channel.policy?.filler?.geography?.place,
    },
    live,
    nowMs: serverNowMs ?? Date.now(),
    playheadMs: viewerTimeMs,
  });

  // Subtitles (§9.5): the viewer's remembered track, followed along the picture's own clock, so a
  // viewer behind live reads the lines of what is on screen.
  const [subtitlesKey, setSubtitlesKey] = useSubtitlesPreference();
  const subtitles = useChannelSubtitles(
    subtitlesSource,
    channel.id,
    viewerTimeMs,
    subtitlesKey,
    !offAir && player.status === "playing",
  );
  const subtitleOptions = subtitleChoices(subtitles.tracks ?? [], subtitlesKey);
  const qualityOptions = qualityOptionsFor(player);
  const [detailsOpen, setDetailsOpen] = useState(false);
  const detailRows = usePlaybackDetails(detailsOpen, videoRef, {
    lagSeconds: live.lagSeconds,
    level: player.ladder.onAir,
    sessionId: player.playbackSessionId,
  });

  // The Channel-change sound (§9.4): its switch in Audio decides it.
  const [switchSoundOn, setSwitchSoundOn] = useSwitchSound({
    channelId: channel.id,
    heldFrame,
    tuning: player.status === "loading",
    video: videoRef,
  });

  // A tune the screen asked for is never told back to it; a change of channel from anywhere else
  // (the URL, a link, the back button, More's Last channel) is.
  const requested = useRef<string[]>([]);
  const fullScreenKey = useRef<string | undefined>(undefined);
  const onRequest = useCallback(
    (request: WatchRequest) => {
      const transport = player.liveTransport;
      switch (request.type) {
        case "tune":
          requested.current = [...requested.current, request.channelId].slice(-REMEMBERED_TUNES);
          player.tune(request.channelId);
          return;
        case "pause":
          transport.pause();
          return;
        case "resume":
          void transport.play();
          return;
        case "goLive":
          void transport.goLive();
          return;
        case "mute": {
          const current = soundRef.current;
          current?.setMuted(!current.muted);
          return;
        }
        case "fullScreen": {
          // The screen's layout is full in a window too (#4732), so the browser's own state says
          // which way to go; Esc only ever leaves full screen.
          const inFullScreen = Boolean(document.fullscreenElement);
          if (inFullScreen) void document.exitFullscreen().catch(() => undefined);
          else if (fullScreenKey.current !== "Escape")
            void document.documentElement.requestFullscreen?.().catch(() => undefined);
          return;
        }
        case "guide":
          onGuide();
          return;
        case "message":
          toast(request.text);
          return;
        case "airplay":
        case "menu":
          return;
      }
    },
    [onGuide, player],
  );

  const [hintSeen, rememberHint] = useWatchHintPreference();
  const model = useWatchScreen({
    hintSeen,
    layout: "full",
    lineup,
    onRequest,
    watchedId: player.channel?.id ?? channel.id,
  });
  const { send } = model;

  // What the player really does, however it got there (a seek, Start over, the media keys, a
  // Retry), in whole seconds behind live, so the clock alone says nothing new.
  const behindMs = Math.round(player.liveTransport.state.lagSeconds) * 1000;
  useEffect(() => {
    send({ behindMs, paused: player.paused, type: "playback" });
  }, [behindMs, player.paused, send]);

  // The browser's own full screen, however it was entered or left (F, the control, a double click,
  // the browser's Esc or its own controls), names the full-screen control.
  const [fullScreen, setFullScreen] = useState(() => Boolean(document.fullscreenElement));
  useEffect(() => {
    const changed = () => setFullScreen(Boolean(document.fullscreenElement));
    document.addEventListener("fullscreenchange", changed);
    return () => document.removeEventListener("fullscreenchange", changed);
  }, []);

  const seenHint = model.state.hintSeen;
  useEffect(() => {
    if (seenHint && !hintSeen) rememberHint();
  }, [seenHint, hintSeen, rememberHint]);

  // Read when the player's channel moves, not a reason to look again: the screen moving on ahead of
  // the player is its own tune, still on its way.
  const watchedRef = useRef(model.state.watchedId);
  watchedRef.current = model.state.watchedId;
  const tunedId = player.channel?.id;
  useEffect(() => {
    if (!tunedId) return;
    if (tunedId === watchedRef.current) {
      // Caught up: every tune the screen asked for has landed or been replaced.
      requested.current = [];
      return;
    }
    if (requested.current.includes(tunedId)) return;
    send({ channelId: tunedId, type: "tuned" });
  }, [tunedId, send]);

  // The screen's keys reach it from the document in the bubbling phase, after every control.
  const latest = useRef(model);
  latest.current = model;
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const target = event.target instanceof Element ? event.target : null;
      const key = watchKeyOf({
        altKey: event.altKey,
        composing: event.isComposing,
        ctrlKey: event.ctrlKey,
        defaultPrevented: event.defaultPrevented,
        inField: Boolean(target?.closest(fieldSelector)),
        key: event.key,
        metaKey: event.metaKey,
      });
      if (key === undefined) return;
      const { state } = latest.current;
      if ((key === "ArrowLeft" || key === "ArrowRight") && !state.list.open && !state.menu && !state.focus) {
        // The bare picture's own: ten seconds through the live window; forward from live does nothing.
        const transport = player.liveTransport;
        if (key === "ArrowRight" && transport.state.mode === "live") return;
        event.preventDefault();
        void transport.seek(
          transport.state.viewerTimeMs + (key === "ArrowLeft" ? -SEEK_STEP_MS : SEEK_STEP_MS),
        );
        return;
      }
      fullScreenKey.current = key;
      send({ key, type: "key" });
      fullScreenKey.current = undefined;
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [player.liveTransport, send]);

  // Playback details reads the element only while its page is open.
  const menuPage = model.view.menu?.page;
  useEffect(() => setDetailsOpen(menuPage === "details"), [menuPage]);

  const channelLabel = `CH ${channel.number} · ${channel.name}`;
  const cover = offAir ? (
    <PausedCover
      channelLabel={channelLabel}
      channelName={channel.name}
      density={density}
      isAdmin={isAdmin}
      onLeave={back.onPress}
    />
  ) : player.status === "error" ? (
    <StoppedCover
      channelLabel={channelLabel}
      density={density}
      failureCause={player.failureCause}
      onLeave={back.onPress}
      onRetry={player.retry}
    />
  ) : player.status === "loading" && tune.reconnecting ? (
    // A reconnect to the channel on screen is not a tune: no static and no "Tuning in".
    <ThemeScope theme="dark">
      <PlayerCover
        channelLabel={channelLabel}
        density={density}
        kind="off-air"
        message="The picture will be back in a moment."
        title="Reconnecting"
      />
    </ThemeScope>
  ) : refused && player.status === "loading" ? (
    <ThemeScope theme="dark">
      <PlayerCover
        channelLabel={channelLabel}
        density={density}
        kind="off-air"
        message="Your browser waits for you to start the picture."
        onRetry={() => void player.liveTransport.play()}
        retryLabel="Play"
        title="Ready to play"
      />
    </ThemeScope>
  ) : undefined;

  // The watched channel's star, from More, said in the message line.
  const starred = lineup.favouriteIds.includes(channel.id);
  const named = `${channel.number} ${channel.name}`;
  const toggleFavourite = () => {
    channels.setFavourite(channel.id, !starred).then(
      () => toast(starred ? `${named} removed from Favourites` : `${named} added to Favourites`),
      () => toast(starred ? `${named} could not be removed from Favourites` : `${named} could not be added to Favourites`),
    );
  };

  const { info, startOverMs } = chromeData;
  const watched = { band: chromeData.band, id: channel.id, playheadMs: viewerTimeMs };
  const muted = sound?.muted ?? false;

  const picture = (
    <Stack flexBasis="0%" flexGrow={1} flexShrink={1} minHeight={0} position="relative">
      <VideoPlayer
        attach={attach}
        fill
        // The watching screen's keys are the only keys here.
        keys={false}
        live
        liveTransport={player.liveTransport}
        renderChrome={(chrome) => (
          <>
            <FirstPlayMuted chrome={chrome} onStart={onFirstPlayStart} onUnmuted={onFirstPlayUnmuted} />
            <ChromeBridge chrome={chrome} onSound={setSound} />
          </>
        )}
      />
      {subtitles.lines.length > 0 ? (
        <PlayerCaption
          bottom={model.view.awake ? CAPTION_AWAKE : CAPTION_RESTING}
          density={density}
          lines={subtitles.lines}
          where="pointer"
        />
      ) : null}
    </Stack>
  );

  return (
    <Stack flexBasis="0%" flexGrow={1} flexShrink={1} minHeight={0} position="relative" render="section">
      <WatchScreen
        back={back}
        cover={cover}
        detail={(each) => (each.minutesLeft === undefined ? {} : { minutesLeft: each.minutesLeft })}
        fullScreen={fullScreen}
        layout="full"
        model={model}
        muted={muted}
        picture={picture}
        programme={(each) => programmeOf(each, watched, formatGuideTime)}
        renderMenu={(slot) =>
          watchMenu({
            audio,
            density,
            detailRows,
            goLive:
              live.mode === "live"
                ? undefined
                : {
                    detail: formatGuideTime(serverNowMs ?? Date.now()),
                    onPress: () => {
                      // A press away from the menu only closes it, so the menu closes first.
                      slot.close();
                      send({ control: "goLive", type: "press" });
                    },
                  },
            favourite: {
              on: starred,
              onPress: () => {
                slot.close();
                toggleFavourite();
              },
            },
            onCopyDetails: (text) => void copyText(text),
            onLastChannel: player.tune,
            onQuality: (value) => player.setQuality(value === "auto" ? "auto" : Number(value)),
            onSubtitles: setSubtitlesKey,
            qualityOptions,
            slot,
            startOver:
              startOverMs === undefined || !info
                ? undefined
                : { detail: info.title, onPress: () => void player.liveTransport.seek(startOverMs) },
            subtitleOptions,
            switchSound: { on: switchSoundOn, onChange: setSwitchSoundOn },
            video: videoRef,
          })
        }
        tune={tune.view}
        watermark={watermarkOf(channel.policy?.watermark)}
      />
      {startedMuted && muted ? (
        <Layer centred top={PILL_TOP}>
          <UnmutePill onPress={() => sound?.setMuted(false)} />
        </Layer>
      ) : null}
    </Stack>
  );
};

export type { ChannelWatchProps };
export { ChannelWatch };
