import * as channelsApi from "@loomarr/api/endpoints/channels";
import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { audioChoices } from "@loomarr/core/channel-audio";
import {
  createChannelSubtitlesSource,
  nextSubtitle,
  SUBTITLES_OFF,
  subtitleChoices,
  useChannelSubtitles,
} from "@loomarr/core/channel-subtitles";
import { formatGuideTime } from "@loomarr/core/guide";
import { Stack } from "@loomarr/design-system";
import {
  PlayerChrome,
  PlayerDetailsPanel,
  PlayerVolume,
  playbarFromTimeline,
  rungFor,
  surfCardsFromGuide,
  useSurfCards,
  watermarkOf,
} from "@loomarr/ui";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { VideoPlayer, type VideoPlayerChrome } from "@/components/loomarr/player/video-player";
import { TunerLoader } from "@/components/loomarr/shell/tuner-loader";
import { copyText } from "@/lib/copy-text";
import { useCoarsePointer } from "@/lib/use-coarse-pointer";
import { usePhoneWidth } from "@/lib/use-phone-width";
import { TunerOSD } from "../tuner-osd";
import { usePlaybackDetails } from "../use-playback-details";
import { useSubtitlesPreference } from "../use-subtitles-preference";
import { useSwitchSound } from "../use-switch-sound";
import { useTunePhase } from "../use-tune-phase";
import type { WatchChannels } from "../use-watch-channels";
import type { WatchPlayer } from "../use-watch-player";
import { useWatchShortcuts } from "../use-watch-shortcuts";
import { useWatchTelemetry } from "../use-watch-telemetry";
import { FirstPlayMuted } from "./first-play-muted";
import { FlashOnVolume } from "./flash-on-volume";
import { HideForIdent } from "./hide-for-ident";
import { HoldWhile } from "./hold-while";
import { PausedCover } from "./paused-cover";
import { qualityOptionsFor } from "./quality-options";
import { sheetFor } from "./sheet-for";
import { StoppedCover } from "./stopped-cover";
import { useExpiryNotice } from "./use-expiry-notice";
import { useStationIdent } from "./use-station-ident";
import { useVolumeLevel } from "./use-volume-level";

// ChannelWatch — the full-bleed `/watch/$id` player: play a channel live in the browser (§9, #2088).
//
// The player itself is the shared VideoPlayer primitive in `live` mode with the shared player
// controller's browser transport bound through its `attach` seam (useWatchPlayer, owned by the route).
// Its controls are the §9 player chrome from `@loomarr/ui` (PlayerChrome, variant H: the band, the
// keys and the top row), drawn through VideoPlayer's `renderChrome` so they share its auto-hide and
// go fullscreen with it. This component owns what the chrome is fed: the channel's timeline, the one
// right-hand sheet, and the Leave that exits.
//
// ⚠ Audio only SHOWS the channel's default audio language. An admin sets it on the channel's info
// page (#2088); the per-viewer choice between renditions comes with the alternate-audio step.

// Subtitles are read beside the stream with the session cookie, the same-origin request every
// member read makes.
const subtitlesSource = createChannelSubtitlesSource((input, init) => fetch(input, init));

interface ChannelWatchProps {
  channel: ChannelDTO;
  isAdmin: boolean;
  /** Leave the player (its ✕): back where the viewer came from. */
  onLeave: () => void;
  /** The player and its tuner: what plays, what the viewer asked of it, and its controls. */
  player: WatchPlayer;
  /** The channels around this one: the drawer, the phone panel, and the recents record. */
  channels?: WatchChannels;
}

const ChannelWatch = ({ channel, isAdmin, onLeave, player, channels }: ChannelWatchProps) => {
  const drawer = channels?.drawer;
  const phone = channels?.phone;
  const onTuneSettled = channels?.onTuneSettled;
  // A portrait phone (§9.7, #2775): the picture pinned under the top row, the band and keys below it
  // in the black space, nothing over the picture. Wider draws the chrome over the picture.
  const onPhone = usePhoneWidth();
  // A finger gets the chrome's touch form: larger targets, no hover card, no tooltips (§9.5). So
  // does the portrait layout, which is a phone's: its band is already touch-sized, and the pointer
  // keys (legends, a Volume key) are wider than a 390 px frame and clipped More.
  const touch = useCoarsePointer() || onPhone;
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

  // Whether the player holds a picture: once any channel has played here, a switch leaves its last
  // frame held in the <video> while the next one loads (VideoPlayer's held poster), and the tuner's
  // wash drains it. ChannelWatch stays mounted across a switch, so this outlives the channel prop.
  // Before the first frame (a cold start) there is nothing under the loader to drain.
  const [heldFrame, setHeldFrame] = useState(false);
  useEffect(() => {
    if (player.status === "playing") setHeldFrame(true);
  }, [player.status]);

  // Recents record a tune once it settles, never a warm or a channel surfed past (the API's rule):
  // once per channel, on its first frame.
  const settledRef = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (player.status !== "playing" || settledRef.current === channel.id) return;
    settledRef.current = channel.id;
    onTuneSettled?.(channel.id);
  }, [player.status, channel.id, onTuneSettled]);

  // The first play of the session is muted, with a pill to undo it (§9.3).
  const [startedMuted, setStartedMuted] = useState(false);
  const onFirstPlayStart = useCallback(() => setStartedMuted(true), []);
  const onFirstPlayUnmuted = useCallback(() => setStartedMuted(false), []);

  const paused = channel.status === "paused" || channel.status === "detached";

  // Settings' audio list is the tracks the airing programme actually carries — fetched, never
  // hardcoded. `enabled` gates the probe on a playing channel (a paused one has nothing to probe).
  const tracks = channelsApi.useChannelTracks(channel.id, {
    query: { enabled: !paused && player.status === "playing", retry: false },
  });
  const tracksBody = unwrap(tracks.data);

  // The chrome's data — the channel's schedule strip (programmes and the breaks between them, each
  // with its stills and every spot in a break). Refetched on an interval so the live edge and what's
  // coming stay current as programmes roll.
  const timeline = channelsApi.useChannelTimeline(channel.id, undefined, {
    query: { enabled: !paused, retry: false, refetchInterval: 30_000 },
  });
  const timelineBody = unwrap(timeline.data);
  const airings = timelineBody?.airings ?? [];
  const serverAtProjection = timelineBody?.serverNowMs;
  const serverNowMs =
    serverAtProjection === undefined
      ? undefined
      : serverAtProjection + Math.max(0, Date.now() - timeline.dataUpdatedAt);
  const viewerTimeMs = player.liveTransport.state.viewerTimeMs;
  useWatchTelemetry({
    channelId: channel.id,
    playbackSessionId: player.playbackSessionId,
    airings,
    viewerTimeMs,
    serverNowMs,
  });
  // The channel may be set to a track the CURRENTLY-airing programme doesn't carry (set to French,
  // but this film is English-only). The selection stays VISIBLE rather than dropped: the preference
  // still applies when a programme with that track airs (audioChoices keeps it).
  const audio = audioChoices(tracksBody?.audio ?? [], channel.policy?.playout?.audioLanguage);

  // Subtitles (§9.5): the viewer's remembered track, followed along the picture's own clock, so a
  // viewer behind live reads the lines of what is on screen. C steps through the menu's choices.
  const [subtitlesKey, setSubtitlesKey] = useSubtitlesPreference();
  const subtitles = useChannelSubtitles(
    subtitlesSource,
    channel.id,
    viewerTimeMs,
    subtitlesKey,
    !paused && player.status === "playing",
  );
  const subtitleOptions = subtitleChoices(subtitles.tracks ?? [], subtitlesKey);
  const subtitlesOn = subtitlesKey !== SUBTITLES_OFF;

  const qualityOptions = qualityOptionsFor(player);

  // The channel on the OSD is the one asked for, from the moment the controller starts its tune.
  const osdChannel = channel;
  const tuning = player.status === "loading";
  // The tune only dresses the wait (§9.4): a quiet flip first, the full signature only when the picture
  // is slow (960 ms), and its length is playback readiness, never a timer.
  const tunePhase = useTunePhase(tuning, channel.id);
  const signature = tuning && tunePhase !== "flip";
  const channelLabel = `CH ${osdChannel.number} · ${osdChannel.name}`;

  // The lower third and the playbar, read from the timeline the way every client reads it (§9). The
  // live edge is the server's clock, projected forward since the read; until the first read it is
  // this machine's.
  const filler = osdChannel.policy?.filler;
  const chromeData = playbarFromTimeline({
    airings,
    channel: {
      ...(osdChannel.decade === undefined ? {} : { decade: osdChannel.decade }),
      era: filler?.era,
      name: osdChannel.name,
      number: String(osdChannel.number),
      place: filler?.geography?.place,
    },
    live: player.liveTransport.state,
    nowMs: serverNowMs ?? Date.now(),
    playheadMs: viewerTimeMs,
  });
  const { info, startOverMs } = chromeData;
  // The hold (a wait of 1.5 s or more): the programme's fanart, from the rung this screen will draw.
  const holdArt = chromeData.band.backdrop;
  const holdBackdrop =
    tunePhase === "hold" && holdArt
      ? rungFor(holdArt, window.innerWidth, window.devicePixelRatio)
      : undefined;

  // The station-ID card (§9.1): every settled tune shows it for five seconds, while the controls are
  // hidden.
  const settled = !tuning && player.status === "playing";
  const ident = useStationIdent(settled);

  const [switchSoundOn, setSwitchSoundOn] = useSwitchSound({
    channelId: osdChannel.id,
    heldFrame,
    tuning,
    video: videoRef,
  });

  // Last channel (§9.3): the newest recent that isn't this one, with its number for the key.
  const previousId = phone?.previousId;
  const previous = previousId ? drawer?.channels.find((c) => c.id === previousId) : undefined;
  const last = previous ? { id: previous.id, number: String(previous.number) } : undefined;
  const goLast = last ? () => player.tune(last.id) : undefined;

  // Playback details (§9.5): a panel at the upper left on a pointer, a sheet on a phone. I toggles it.
  const [detailsOpen, setDetailsOpen] = useState(false);
  const { closeVolume, flashVolume, setVolumeFlash, setVolumeOpen, volumeFlash, volumeOpen } =
    useVolumeLevel();

  const { sheet, toggleSheet, closeSheet, entry, onShortcut } = useWatchShortcuts({
    channels: drawer?.channels,
    detailsAsSheet: touch,
    infoAsCard: !touch,
    onDetails: () => setDetailsOpen((open) => !open),
    onLast: goLast,
    onNextSubtitles: () => setSubtitlesKey(nextSubtitle(subtitleOptions)),
    // Esc closes the top layer, then leaves: the details panel and the volume level are layers too.
    onLeave: () =>
      detailsOpen ? setDetailsOpen(false) : volumeOpen || volumeFlash > 0 ? closeVolume() : onLeave(),
    onTune: player.tune,
    viewerPaused: player.liveTransport.state.mode === "paused",
  });
  // One thing in the menu's place at a time: opening a menu or sheet closes the level.
  useEffect(() => {
    if (sheet) setVolumeOpen(false);
  }, [sheet, setVolumeOpen]);
  const volumeShown = !touch && !sheet && (volumeOpen || volumeFlash > 0);

  const detailRows = usePlaybackDetails(detailsOpen || sheet === "details", videoRef, {
    lagSeconds: player.liveTransport.state.lagSeconds,
    level: player.ladder.onAir,
    sessionId: player.playbackSessionId,
  });
  const copyDetails = (text: string) => void copyText(text);

  // The number readout, top right of the frame, drawn by the chrome (it keeps it clear of the
  // watermark): the digits so far, dashed out to the longest channel number, over "Enter channel" or,
  // for a number no channel has, "No such channel". On a solid ink backing (§9.2), white, never amber.
  // Flipping (§9.2): each Channel key tunes at once, and the cards ride along in the band's place; the
  // band is down, so the readout names the channel.
  const guide = channels?.guide;
  const surfCards = useMemo(
    () => (guide ? surfCardsFromGuide(player.catalog, guide.layout, guide.nowMs) : []),
    [player.catalog, guide],
  );
  const surf = useSurfCards(surfCards, channel.id, player.flipKey);
  const readout = entry.digits
    ? { digits: entry.missed ? entry.digits : entry.digits.padEnd(entry.width, "–"), missed: entry.missed }
    : surf
      ? { digits: String(osdChannel.number), name: osdChannel.name, quiet: true }
      : undefined;
  // The channel's bug is burned into the picture; the chrome keeps out of its corner (§9.6, #2772).
  const watermark = watermarkOf(channel.policy?.watermark);

  // Where what a key opens goes (§9.5, variant J): a menu in the control cluster on a pointer, a sheet
  // from the bottom on a phone held upright, the right-hand sheet on a phone on its side. Channels is
  // still the console drawer (#1659 W1) everywhere, until it is rebuilt on the sheet.
  const placement = onPhone ? "bottom" : touch ? "right" : "menu";
  const inMenu = placement === "menu" && sheet !== "channels";
  const live = player.liveTransport.state;

  // What is open: a menu or a sheet, by placement. Info on a pointer is neither: it is the block on
  // air's card on the film strip, pinned open in full (the strip's `info`).
  const openSheetFor = (chrome: VideoPlayerChrome) =>
    sheetFor({
      audio,
      chrome,
      closeSheet,
      copyDetails,
      density,
      detailRows,
      drawer,
      hasAirings: airings.length > 0,
      info,
      live,
      osdChannel,
      placement,
      player,
      playbackWindow: chromeData.window,
      qualityOptions,
      segments: chromeData.segments,
      serverNowMs,
      setDetailsOpen,
      setSubtitlesKey,
      setSwitchSoundOn,
      sheet,
      startOverMs,
      subtitleOptions,
      switchSoundOn,
      toggleSheet,
      touch,
      video: videoRef,
      viewerTimeMs,
    });

  // A stream that stopped has nothing for the playbar to stand on, so Retry and the way out take the
  // frame in the chrome's place.
  const errorCover =
    player.status === "error" ? (
      <StoppedCover
        channelLabel={channelLabel}
        density={density}
        failureCause={player.failureCause}
        onLeave={onLeave}
        onRetry={player.retry}
      />
    ) : undefined;

  // The chrome over the picture (§9), or the error cover in its place.
  const renderChrome = (chrome: VideoPlayerChrome) => {
    const opened = openSheetFor(chrome);
    return (
      <>
        <HoldWhile hold={chrome.hold} on={sheet !== undefined || volumeShown} />
        <FirstPlayMuted chrome={chrome} onStart={onFirstPlayStart} onUnmuted={onFirstPlayUnmuted} />
        <FlashOnVolume
          muted={chrome.muted}
          onChange={flashVolume}
          quiet={startedMuted}
          volume={chrome.volume}
        />
        <HideForIdent hide={chrome.hide} ident={ident} />
        {errorCover ?? (
          <PlayerChrome
            band={chromeData.band}
            captions={subtitles.lines}
            density={density}
            formatTime={formatGuideTime}
            ident={ident}
            layout={onPhone ? "portrait" : "overlay"}
            keys={{
              density,
              // Last is a key everywhere (§9.7); a phone's More has no row for it.
              last: last && goLast ? { number: last.number, onPress: goLast } : undefined,
              moreOpen: sheet === "more",
              onChannelDown: player.canSurf ? () => player.step(-1) : undefined,
              onChannelUp: player.canSurf ? () => player.step(1) : undefined,
              onMore: () => toggleSheet("more"),
              onTogglePlay: chrome.toggle,
              playing: chrome.playing,
              pointer: !touch,
              // Volume is desktop's: a phone's own buttons set it.
              volume: touch
                ? undefined
                : {
                    muted: chrome.muted,
                    onPress: () => {
                      closeSheet();
                      setVolumeFlash(0);
                      setVolumeOpen((open) => !open);
                    },
                    open: volumeOpen,
                  },
            }}
            live={live}
            menu={
              volumeShown ? (
                <PlayerVolume
                  muted={chrome.muted}
                  onVolume={(level) => {
                    chrome.setVolume(level);
                    chrome.setMuted(level === 0);
                  }}
                  volume={chrome.volume}
                />
              ) : inMenu ? (
                opened
              ) : undefined
            }
            onGoLive={chrome.goLive}
            readout={readout}
            {...(surf ? { surf } : {})}
            panel={
              detailsOpen && !touch ? (
                <PlayerDetailsPanel
                  density={density}
                  onClose={() => setDetailsOpen(false)}
                  onCopy={copyDetails}
                  rows={detailRows}
                />
              ) : undefined
            }
            onLeave={onLeave}
            // The film strip is the band's foot on a pointer device (§9.2); a phone's is in More.
            schedule={
              !touch && airings.length > 0
                ? {
                    info: sheet === "info",
                    onSeek: chrome.seekTo,
                    playheadMs: viewerTimeMs,
                    segments: chromeData.segments,
                    window: chromeData.window,
                  }
                : undefined
            }
            sheet={inMenu ? undefined : opened}
            utilities={{
              audioOpen: sheet === "audio",
              density,
              fullscreen: chrome.fullscreen,
              onAudio: () => toggleSheet("audio"),
              onFullscreen: chrome.toggleFullscreen,
              onSubtitles: () => toggleSheet("subtitles"),
              pointer: !touch,
              subtitlesOn,
              subtitlesOpen: sheet === "subtitles",
            }}
            unmute={startedMuted && chrome.muted ? () => chrome.setMuted(false) : undefined}
            // The quiet flip keeps the band up (§9.4); only the full signature hides it.
            visible={onPhone || (chrome.controlsShown && !signature)}
            watermark={onPhone ? undefined : watermark}
          />
        )}
      </>
    );
  };

  // The player. `live` = no seek bar of its own; `renderChrome` = the §9 chrome; `attach` binds
  // hls.js. Fullscreen is the player's OWN state, handed to the chrome.
  const playerEl = (
    <VideoPlayer
      live
      liveTransport={player.liveTransport}
      renderChrome={renderChrome}
      // The tuner "acquiring signal" overlay covers the warm-up beat (cold encoder, first segment
      // not cut yet) so the viewer sees a channel tuning in, not a black frame. Only while loading —
      // an error draws its own cover, and a playing stream needs no overlay.
      // Under it, the tuned channel's still (decoded on demand for a cold channel) replaces the
      // previous channel's held frame, so the picture already belongs to the channel being tuned.
      // The loader's wash covers both (#1620): it drains a held frame on a switch, and a still that
      // lands later arrives already under the snow, so the readout never sits on a picture.
      overlay={
        signature ? (
          <>
            {player.stillURL && (
              <img src={player.stillURL} alt="" className="absolute inset-0 h-full w-full object-contain" />
            )}
            <TunerLoader backdrop={holdBackdrop} channel={osdChannel} heldFrame={heldFrame} />
          </>
        ) : undefined
      }
      onShortcut={onShortcut}
      attach={attach}
      onChannelStep={player.step}
      // The picture fills the window, letterboxed and centred by the <video>: on an upright phone a
      // strip mid-screen, with the toolbar above it and the band and keys below (§9.7).
      fill
    />
  );

  if (paused) {
    return (
      <PausedCover
        channelLabel={channelLabel}
        channelName={channel.name}
        density={density}
        isAdmin={isAdmin}
        onLeave={onLeave}
      />
    );
  }

  return (
    <Stack className="min-h-0" flexBasis="0%" flexGrow={1} flexShrink={1} render="section">
      <div className="relative min-h-0 flex-1">
        {playerEl}
        {/* The wash's centred channel line is the visible readout; this is the same line for a
            screen reader. */}
        {tuning && (
          <TunerOSD number={osdChannel.number} name={osdChannel.name} currentTitle={player.currentTitle} />
        )}
      </div>
    </Stack>
  );
};

export type { ChannelWatchProps };
export { ChannelWatch };
