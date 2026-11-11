import * as channelsApi from "@loomarr/api/endpoints/channels";
import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { formatGuideTime } from "@loomarr/core/guide";
import { ThemeScope } from "@loomarr/design-system";
import type { PlayerFailureCause } from "@loomarr/player";
import {
  PlayerAudio,
  PlayerBottomSheet,
  PlayerChrome,
  PlayerCover,
  PlayerDetails,
  PlayerDetailsPanel,
  PlayerInfo,
  PlayerMenu,
  PlayerMore,
  PlayerOptions,
  PlayerSheet,
  PlayerVolume,
  playbarFromTimeline,
  rungFor,
  watermarkOf,
} from "@loomarr/ui";
import { type ReactNode, useCallback, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { AUTO_SENTINEL, audioOptions, withSaved } from "@/channels/channel-default-audio";
import { ChannelDrawer } from "@/channels/channel-drawer";
import { VideoPlayer, type VideoPlayerChrome } from "@/components/loomarr/player/video-player";
import { TunerLoader } from "@/components/loomarr/shell/tuner-loader";
import { useCoarsePointer } from "@/lib/use-coarse-pointer";
import { usePhoneWidth } from "@/lib/use-phone-width";
import { TunerOSD } from "../tuner-osd";
import { usePlaybackDetails } from "../use-playback-details";
import { useSwitchSound } from "../use-switch-sound";
import { useTunePhase } from "../use-tune-phase";
import type { WatchChannels } from "../use-watch-channels";
import type { WatchPlayer } from "../use-watch-player";
import { useWatchShortcuts } from "../use-watch-shortcuts";
import { useWatchTelemetry } from "../use-watch-telemetry";

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

// The station-ID card's time on screen after a settled tune (playback-clients: Station ID).
const IDENT_MS = 5_000;

// What the stopped cover says for each cause, in plain words (the H mock's copy). A channel the app
// can't play has no cause and reads as a stream that stopped.
const STOPPED_COVER: Record<PlayerFailureCause | "none", { message?: string; title: string }> = {
  channel: {
    message:
      "Loomarr is running, but it couldn't start the video for this channel. Trying again usually fixes it.",
    title: "This channel couldn't start",
  },
  none: { title: "The stream stopped." },
  server: {
    message:
      "The picture stopped because this device lost touch with Loomarr. Check that the server is running and that you're on the same network, then try again.",
    title: "Loomarr's server isn't answering",
  },
  stream: { message: "Trying again usually brings it back.", title: "The stream stopped." },
};

// A rung as Settings names it: the premium by its class, the rest by lines.
const rungLabel = (height: number) => (height >= 2160 ? "4K" : `${height}p`);

// HoldWhile keeps the player's controls shown while `on`: VideoPlayer's hold is a count, so each
// hold(true) is paired with exactly one hold(false), here by the effect's cleanup.
const HoldWhile = ({ hold, on }: { hold: VideoPlayerChrome["hold"]; on: boolean }) => {
  useEffect(() => {
    if (!on) return;
    hold(true);
    return () => hold(false);
  }, [hold, on]);
  return null;
};

// The session's first play starts muted (§9.3), so a channel never blasts at someone who just opened
// the player; a pill takes one tap to undo it. Only the session's first play: later channels, and
// reopening Watch, keep the sound as the viewer left it.
const FIRST_PLAY_KEY = "loomarr.player.first-play";
const firstPlayOfSession = () => {
  try {
    if (sessionStorage.getItem(FIRST_PLAY_KEY)) return false;
    sessionStorage.setItem(FIRST_PLAY_KEY, "1");
  } catch {
    // No session storage (a private window): treat it as a first play only once per mount.
  }
  return true;
};

// FirstPlayMuted mutes the player once, on the session's first play, and tells the screen to show the
// pill until the sound comes on by any means (the pill, the key, M).
const FirstPlayMuted = ({
  chrome,
  onUnmuted,
  onStart,
}: {
  chrome: VideoPlayerChrome;
  onStart: () => void;
  onUnmuted: () => void;
}) => {
  const { setMuted } = chrome;
  const seenMuted = useRef(false);
  const started = useRef(false);
  useEffect(() => {
    if (started.current) return;
    started.current = true;
    if (!firstPlayOfSession()) return;
    setMuted(true);
    onStart();
  }, [setMuted, onStart]);
  useEffect(() => {
    if (chrome.muted) seenMuted.current = true;
    else if (seenMuted.current) onUnmuted();
  }, [chrome.muted, onUnmuted]);
  return null;
};

// HideForIdent clears the controls as the station-ID card arrives, so the card stands alone as the
// mock draws it; moving the pointer brings the controls back in its place.
// How long a change of level or mute shows the volume level (§9.5).
const VOLUME_FLASH_MS = 1_500;

// FlashOnVolume shows the volume level for a moment when the level or mute changes after mount.
// The session's first play mutes itself (quiet): the unmute pill says so, and the level stays away.
const FlashOnVolume = ({
  muted,
  onChange,
  quiet,
  volume,
}: {
  muted: boolean;
  onChange: () => void;
  quiet: boolean;
  volume: number;
}) => {
  const seen = useRef({ muted, volume });
  useEffect(() => {
    if (seen.current.muted === muted && seen.current.volume === volume) return;
    seen.current = { muted, volume };
    if (!quiet) onChange();
  }, [muted, volume, onChange, quiet]);
  return null;
};

const HideForIdent = ({ hide, ident }: { hide: VideoPlayerChrome["hide"]; ident: boolean }) => {
  useEffect(() => {
    if (ident) hide();
  }, [hide, ident]);
  return null;
};

// Picture in picture is the browser's: offered only where it is.
const pipSupported = typeof document !== "undefined" && document.pictureInPictureEnabled === true;

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
  const expiryNoticeRef = useRef({ channelId: channel.id, revision: 0 });
  useEffect(() => {
    if (expiryNoticeRef.current.channelId !== channel.id) {
      expiryNoticeRef.current = { channelId: channel.id, revision: 0 };
    }
    const revision = player.liveTransport.state.noticeRevision;
    if (revision <= expiryNoticeRef.current.revision) return;
    expiryNoticeRef.current.revision = revision;
    toast.info("That paused point is no longer available, so you're back live.");
  }, [channel.id, player.liveTransport.state.noticeRevision]);

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
  const audioValue = channel.policy?.playout?.audioLanguage ?? "";

  // The channel may be set to a track the CURRENTLY-airing programme doesn't carry (set to French,
  // but this film is English-only). Keep that selection VISIBLE in Settings rather than dropping it:
  // the preference still applies when a programme with that track airs (withSaved).
  const selectedAudio = audioValue || AUTO_SENTINEL;
  const audio = withSaved(audioOptions(tracksBody?.audio ?? []), selectedAudio).map((option) => ({
    label: option.label,
    selected: option.value === selectedAudio,
    value: option.value,
  }));

  // Quality (#2088 decision 7): Auto, naming the rung on air, then the channel's rungs. A remembered
  // rung this channel doesn't offer shows Auto chosen, because Auto is what plays.
  const { rungs, height: onAir } = player.ladder;
  const pinned = player.quality !== "auto" && rungs.some((rung) => rung.height === player.quality);
  const qualityOptions =
    rungs.length > 0
      ? [
          { label: onAir ? `Auto · ${rungLabel(onAir)}` : "Auto", selected: !pinned, value: "auto" },
          ...rungs.map((rung) => ({
            label: rungLabel(rung.height),
            selected: pinned && rung.height === player.quality,
            value: String(rung.height),
          })),
        ]
      : undefined;

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
  // hidden. A retune drops it until the new channel settles, then shows it afresh.
  // A retune hides it by derivation: a state change here would render this tree a second time
  // inside the press, holding up the tuning overlay.
  const settled = !tuning && player.status === "playing";
  const [identUp, setIdentUp] = useState(false);
  useEffect(() => {
    if (!settled) return;
    setIdentUp(true);
    const timer = window.setTimeout(() => setIdentUp(false), IDENT_MS);
    return () => window.clearTimeout(timer);
  }, [settled]);
  const ident = settled && identUp;

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
  // The volume level (§9.5, desktop): opened by the Volume key in the menu's place, and shown for a
  // moment when the level or mute changes.
  const [volumeOpen, setVolumeOpen] = useState(false);
  // A count of changes, not a flag: each change restarts the moment; zero is hidden.
  const [volumeFlash, setVolumeFlash] = useState(0);
  const flashVolume = useCallback(() => setVolumeFlash((n) => n + 1), []);
  useEffect(() => {
    if (volumeFlash === 0) return;
    const timer = window.setTimeout(() => setVolumeFlash(0), VOLUME_FLASH_MS);
    return () => window.clearTimeout(timer);
  }, [volumeFlash]);
  const closeVolume = () => {
    setVolumeOpen(false);
    setVolumeFlash(0);
  };

  const { sheet, toggleSheet, closeSheet, entry, onShortcut } = useWatchShortcuts({
    channels: drawer?.channels,
    detailsAsSheet: touch,
    infoAsCard: !touch,
    onDetails: () => setDetailsOpen((open) => !open),
    onLast: goLast,
    // Esc closes the top layer, then leaves: the details panel and the volume level are layers too.
    onLeave: () =>
      detailsOpen ? setDetailsOpen(false) : volumeOpen || volumeFlash > 0 ? closeVolume() : onLeave(),
    onTune: player.tune,
    viewerPaused: player.liveTransport.state.mode === "paused",
  });
  // One thing in the menu's place at a time: opening a menu or sheet closes the level.
  useEffect(() => {
    if (sheet) setVolumeOpen(false);
  }, [sheet]);
  const volumeShown = !touch && !sheet && (volumeOpen || volumeFlash > 0);

  const detailRows = usePlaybackDetails(detailsOpen || sheet === "details", videoRef, {
    lagSeconds: player.liveTransport.state.lagSeconds,
    level: player.ladder.onAir,
    sessionId: player.playbackSessionId,
  });
  const copyDetails = (text: string) => void navigator.clipboard?.writeText(text);

  // The number readout, top right of the frame, drawn by the chrome (it keeps it clear of the
  // watermark): the digits so far, dashed out to the longest channel number, over "Enter channel" or,
  // for a number no channel has, "No such channel". On a solid ink backing (§9.2), white, never amber.
  const readout = entry.digits
    ? { digits: entry.missed ? entry.digits : entry.digits.padEnd(entry.width, "–"), missed: entry.missed }
    : undefined;
  // The channel's bug is burned into the picture; the chrome keeps out of its corner (§9.6, #2772).
  const watermark = watermarkOf(channel.policy?.watermark);

  // Where what a key opens goes (§9.5, variant J): a menu in the control cluster on a pointer, a sheet
  // from the bottom on a phone held upright, the right-hand sheet on a phone on its side. Channels is
  // still the console drawer (#1659 W1) everywhere, until it is rebuilt on the sheet.
  const placement = onPhone ? "bottom" : touch ? "right" : "menu";
  const inMenu = placement === "menu" && sheet !== "channels";
  // The right-hand sheet's own name for the channel it is about.
  const sheetChannel = { name: osdChannel.name, number: String(osdChannel.number) };
  const sheetOf = (title: string, body: ReactNode, back?: () => void) =>
    placement === "menu" ? (
      // More names no heading: its rows say what they are. Picture is its second page.
      <PlayerMenu
        density={density}
        heading={title !== "More"}
        key={title}
        title={title}
        {...(back ? { onBack: back } : {})}
      >
        {body}
      </PlayerMenu>
    ) : placement === "bottom" ? (
      <PlayerBottomSheet key={title} onClose={closeSheet} title={title}>
        {body}
      </PlayerBottomSheet>
    ) : (
      <PlayerSheet channel={sheetChannel} density={density} key={title} onClose={closeSheet} title={title}>
        {body}
      </PlayerSheet>
    );
  const live = player.liveTransport.state;

  // What is open: a menu or a sheet, by placement. Info on a pointer is neither: it is the block on
  // air's card on the film strip, pinned open in full (the strip's `info`).
  const openSheetFor = (chrome: VideoPlayerChrome) =>
    sheet === "audio" ? (
      sheetOf(
        "Audio",
        <PlayerAudio
          density={density}
          onSwitchSound={setSwitchSoundOn}
          options={audio}
          switchSound={switchSoundOn}
        />,
      )
    ) : sheet === "picture" ? (
      sheetOf(
        "Picture",
        qualityOptions ? (
          <PlayerOptions
            density={density}
            label="Picture quality"
            onSelect={(value) => player.setQuality(value === "auto" ? "auto" : Number(value))}
            options={qualityOptions}
          />
        ) : null,
        placement === "menu" ? () => toggleSheet("more") : undefined,
      )
    ) : sheet === "more" ? (
      sheetOf(
        "More",
        <PlayerMore
          channels={drawer ? { onPress: () => toggleSheet("channels") } : undefined}
          density={density}
          formatTime={formatGuideTime}
          goLive={
            live.mode === "live"
              ? undefined
              : { detail: formatGuideTime(serverNowMs ?? Date.now()), onPress: chrome.goLive }
          }
          info={info ? { onPress: () => toggleSheet("info") } : undefined}
          pictureInPicture={
            pipSupported
              ? {
                  onPress: () => {
                    closeSheet();
                    void videoRef.current?.requestPictureInPicture().catch(() => undefined);
                  },
                }
              : undefined
          }
          playbackDetails={{
            onPress: () => {
              if (touch) toggleSheet("details");
              else {
                closeSheet();
                setDetailsOpen(true);
              }
            },
          }}
          picture={
            qualityOptions
              ? {
                  detail: qualityOptions.find((o) => o.selected)?.label ?? "Auto",
                  onPress: () => toggleSheet("picture"),
                }
              : undefined
          }
          // A phone has no film strip in its band, so its More keeps the schedule.
          schedule={
            touch && airings.length > 0
              ? {
                  onSeek: chrome.seekTo,
                  playheadMs: viewerTimeMs,
                  segments: chromeData.segments,
                  window: chromeData.window,
                }
              : undefined
          }
          startOver={
            startOverMs === undefined || !info
              ? undefined
              : { detail: info.title, onPress: () => chrome.seekTo(startOverMs) }
          }
        />,
      )
    ) : sheet === "details" ? (
      sheetOf("Playback details", <PlayerDetails density={density} onCopy={copyDetails} rows={detailRows} />)
    ) : sheet === "info" && info && placement !== "menu" ? (
      sheetOf("Info", <PlayerInfo density={density} formatTime={formatGuideTime} info={info} />)
    ) : sheet === "channels" && drawer ? (
      <div className="absolute inset-y-0 right-0 z-40 flex max-w-full">
        <ChannelDrawer
          channels={drawer.channels}
          tunedId={osdChannel.id}
          favourites={drawer.favourites}
          recent={drawer.recent}
          nowPercent={drawer.nowPercent}
          onTune={player.tune}
          onFavourite={drawer.onFavourite}
          onClose={closeSheet}
        />
      </div>
    ) : undefined;

  // A stream that stopped has nothing for the playbar to stand on, so Retry and the way out take the
  // frame in the chrome's place.
  const errorCover =
    player.status === "error" ? (
      <ThemeScope theme="dark">
        <PlayerCover
          channelLabel={channelLabel}
          density={density}
          kind="error"
          onLeave={onLeave}
          message={STOPPED_COVER[player.failureCause ?? "none"].message}
          onRetry={player.retry}
          title={STOPPED_COVER[player.failureCause ?? "none"].title}
        />
      </ThemeScope>
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
              pointer: !touch,
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
      <section className="relative flex min-h-0 flex-1 flex-col">
        <ThemeScope theme="dark">
          <PlayerCover
            channelLabel={channelLabel}
            density={density}
            kind="off-air"
            // The web mock's words; the way back is an admin's, so members get the first sentence.
            message={
              isAdmin
                ? "This channel is paused, so there's nothing to watch. Resume it under Danger zone."
                : "This channel is paused, so there's nothing to watch."
            }
            onLeave={onLeave}
            title={`${channel.name} is off air`}
          />
        </ThemeScope>
      </section>
    );
  }

  return (
    <section className="flex min-h-0 flex-1 flex-col">
      <div className="relative min-h-0 flex-1">
        {playerEl}
        {/* The wash's centred channel line is the visible readout; this is the same line for a
            screen reader. */}
        {tuning && (
          <TunerOSD number={osdChannel.number} name={osdChannel.name} currentTitle={player.currentTitle} />
        )}
      </div>
    </section>
  );
};

export type { ChannelWatchProps };
export { ChannelWatch };
