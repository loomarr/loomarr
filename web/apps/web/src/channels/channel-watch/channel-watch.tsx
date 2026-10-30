import * as channelsApi from "@loomarr/api/endpoints/channels";
import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { formatGuideTime } from "@loomarr/core/guide";
import { ThemeScope } from "@loomarr/design-system";
import { type LiveSubtitleCues, nextSubtitles } from "@loomarr/player/browser";
import {
  PlayerChrome,
  PlayerCover,
  PlayerInfo,
  PlayerPortrait,
  PlayerSettings,
  PlayerSheet,
  PlayerShortcuts,
  PlayerStationId,
  type PlayerTransportProps,
  playbarFromTimeline,
} from "@loomarr/ui";
import { useCallback, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { toast } from "sonner";
import { audioChoice } from "@/channels/audio-choice";
import { AUTO_SENTINEL, audioOptions, withSaved } from "@/channels/channel-default-audio";
import { ChannelDrawer, type DrawerChannel } from "@/channels/channel-drawer";
import { PlaybackInfoSheet } from "@/channels/playback-info-sheet";
import { useAudioPreference } from "@/channels/use-audio-preference";
import { useHlsPlayer } from "@/channels/use-hls-player";
import { useLiveSubtitles } from "@/channels/use-live-subtitles";
import { useNumberEntry } from "@/channels/use-number-entry";
import { useQualityPreference } from "@/channels/use-quality-preference";
import { useSubtitlesPreference } from "@/channels/use-subtitles-preference";
import { TunerLoader } from "@/components/loomarr/shell/tuner-loader";
import { VideoPlayer, type VideoPlayerChrome } from "@/components/ui/video-player";
import { clientDiagnostics } from "@/diagnostics/client-reporter";
import { useCoarsePointer } from "@/lib/use-coarse-pointer";
import { usePhoneWidth } from "@/lib/use-phone-width";
import { cn } from "@/lib/utils";
import { type SwitchSound, startChannelSwitchSound } from "../switch-sound";
import { TunerOSD } from "../tuner-osd";
import type { TuneAttempt } from "../tuner-timing";
import type { TuneDirection } from "../use-channel-tuner";
import { useSwitchSoundPreference } from "../use-switch-sound-preference";

// ChannelWatch — the full-bleed `/watch/$id` player: play a channel live in the browser (§9, #2088).
//
// The player itself is the shared VideoPlayer primitive in `live` mode with hls.js bound through its
// `attach` seam (useHlsPlayer). Its controls are the §9 player chrome from `@loomarr/ui`
// (PlayerChrome: the lower third, the preview playbar and the transport), drawn through VideoPlayer's
// `renderChrome` so they share its auto-hide and go fullscreen with it. This component owns what the
// chrome is fed: the channel's timeline, the tuner, the one right-hand sheet, and the ✕ that leaves.
//
// ⚠ Settings' audio is a VIEWER's choice between the stream's two renditions (#2088 decision 6). The
// channel's default language itself is an admin's, on the channel's info page.

interface ChannelWatchProps {
  channel: ChannelDTO;
  isAdmin: boolean;
  /** Leave the player (its ✕): back where the viewer came from. */
  onLeave: () => void;
  tuner?: {
    canSurf: boolean;
    requestedChannel?: ChannelDTO;
    currentTitle?: string;
    attempt?: TuneAttempt;
    acknowledging?: boolean;
    ready: (channelId: string) => void;
    step: (direction: TuneDirection) => void;
    tune: (channelId: string) => void;
    retry: () => void;
  };
  // The channels drawer's data (#1659 W1): every channel read from the guide, and the viewer's own
  // favourites and recent channels. Also the list a typed number is looked up in.
  drawer?: {
    channels: DrawerChannel[];
    favourites: string[];
    recent: string[];
    nowPercent: number;
    onFavourite: (channelId: string, favourite: boolean) => void;
  };
  // A tune has settled on this channel (its first frame played): the viewer's recents record it.
  onTuneSettled?: (channelId: string) => void;
  // The player went into or out of the browser's picture-in-picture: the route keeps Watch mounted
  // while it is, so leaving /watch doesn't stop the floating picture (#2088 decision 4).
  onPictureInPicture?: (active: boolean) => void;
  // The channel the portrait panel's Previous returns to (#2088 step 10), read from the viewer's recents.
  previousId?: string;
}

const PLAYHEAD_DRIFT_THRESHOLD_MS = 30_000;
// The chrome's ⟲/⟳ step, the same as ←/→ (#2088 decision 2).
const SEEK_STEP_MS = 10_000;

// The station-ID card's time on screen after a settled tune, and the Info sheet's during playback
// (playback-clients: Station ID, Panels).
const IDENT_MS = 5_000;
const INFO_PLAYING_MS = 8_000;

// A rung as Settings names it: the premium by its class, the rest by lines.
const rungLabel = (height: number) => (height >= 2160 ? "4K" : `${height}p`);

// The one sheet the chrome can have open at a time (§9.1, plane 5).
type WatchSheet = "channels" | "info" | "playback-info" | "settings" | "shortcuts";

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

// ReportPictureInPicture tells the screen when the player enters or leaves picture-in-picture, so the
// authed layout keeps Watch mounted while the floating picture plays off /watch.
const ReportPictureInPicture = ({
  active,
  report,
}: {
  active: boolean;
  report?: (active: boolean) => void;
}) => {
  useEffect(() => {
    report?.(active);
  }, [active, report]);
  return null;
};

// The `?` sheet's rows (#2088 decision 11), only for what this Watch answers: the tuner's keys with a
// tuner, Info with a programme, picture-in-picture where the browser has it, Subtitles with a track.
const shortcutRows = ({
  info,
  pictureInPicture,
  subtitles,
  tuner,
}: {
  info: boolean;
  pictureInPicture: boolean;
  subtitles: boolean;
  tuner: boolean;
}): (readonly [string, string])[] => [
  ["Space / K", "Pause / play"],
  ["← / →", "Back / forward 10 s"],
  ["L", "Go live"],
  ...(tuner
    ? ([
        ["↑ / ↓  PgUp / PgDn", "Channel up / down"],
        ["0–9  Enter", "Tune a channel number"],
        ["G", "Channels"],
      ] as const)
    : []),
  ["M", "Mute"],
  ["− / =", "Volume down / up"],
  ["F", "Fullscreen"],
  ...(pictureInPicture ? ([["P", "Picture-in-picture"]] as const) : []),
  ...(subtitles ? ([["C", "Subtitles"]] as const) : []),
  ...(info ? ([["I", "Info"]] as const) : []),
  ["?", "This sheet"],
  ["Esc", "Close / leave"],
];

// HideForIdent clears the controls as the station-ID card arrives, so the card stands alone as the
// mock draws it; moving the pointer brings the controls back in its place.
const HideForIdent = ({ hide, ident }: { hide: VideoPlayerChrome["hide"]; ident: boolean }) => {
  useEffect(() => {
    if (ident) hide();
  }, [hide, ident]);
  return null;
};

const ChannelWatch = ({
  channel,
  isAdmin,
  onLeave,
  tuner,
  drawer,
  onTuneSettled,
  onPictureInPicture,
  previousId,
}: ChannelWatchProps) => {
  // A portrait phone (§9, mock "portrait"): a 16:9 picture with nothing on it but the station-ID
  // card, and the chrome's planes stacked in a panel under it. Wider draws the chrome over the picture.
  const onPhone = usePhoneWidth();
  // The portrait panel's element, under the picture: the chrome draws into it through a portal.
  const [panelEl, setPanelEl] = useState<HTMLDivElement | null>(null);
  // A finger gets the chrome's touch form: larger targets, no hover card, no tooltips (§9.5).
  const touch = useCoarsePointer();
  const density = touch ? "touch" : "pointer";
  const tunerReady = tuner?.ready;
  // The tuner warms this Channel's neighbours once its own manifest has arrived, not before: the
  // warm requests would otherwise share the browser's per-host connections with the target's
  // manifest and first segments (#1484). It is still well ahead of the first frame.
  const onManifest = useCallback(() => tunerReady?.(channel.id), [channel.id, tunerReady]);
  // The viewer's quality, remembered across channels: a rung a channel doesn't offer plays Auto.
  const [quality, setQuality] = useQualityPreference();
  // The viewer's subtitles, remembered across channels: a track a programme doesn't carry shows none.
  const [subtitles, setSubtitles] = useSubtitlesPreference();
  // The cues reach the player a render after the sidecar answers for its playhead: the playhead is
  // the player's own, so the two cannot be read in one pass.
  const [subtitleCues, setSubtitleCues] = useState<LiveSubtitleCues>();
  // The viewer's audio, remembered across channels as a language; whether that is the programme's
  // alternate reaches the player a render later, from the tracks at its playhead, like the cues.
  const [audioPreference, setAudioPreference] = useAudioPreference();
  const [alternateAudio, setAlternateAudio] = useState(false);
  const player = useHlsPlayer(channel.id, tuner?.attempt, onManifest, quality, subtitleCues, alternateAudio);
  const atPlayhead = useLiveSubtitles(
    channel.id,
    player.liveTransport.state.viewerTimeMs,
    subtitles,
    player.status === "playing",
  );
  useEffect(() => setSubtitleCues(atPlayhead.cues), [atPlayhead.cues]);
  const { active: subtitleActive, tracks: subtitleTracks } = atPlayhead;
  // C and the CC tool step on from the track shown (or off) through the programme's tracks.
  const cycleSubtitles =
    subtitleTracks.length > 0
      ? () => setSubtitles(nextSubtitles(subtitleTracks, subtitleTracks[subtitleActive] ?? "off"))
      : undefined;
  // The player's own <video>, caught at the attach seam, so the channel-change sound can follow the
  // player's volume and mute, and Go live can act on it, without VideoPlayer knowing about either.
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

  const paused = channel.status === "paused" || channel.status === "detached";

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
  const diagnosticAiring = airings.find(
    (airing) => !airing.nominal && viewerTimeMs >= airing.startMs && viewerTimeMs < airing.stopMs,
  );
  const scheduleBlockId = diagnosticAiring?.scheduleBlockId;

  // Settings' audio is the tracks the programme AT THE PLAYHEAD actually carries — fetched, never
  // hardcoded — asked once per programme by its start, so a viewer behind live hears the choices
  // of what they are watching. `enabled` gates the probe on a playing channel.
  const tracks = channelsApi.useChannelTracks(
    channel.id,
    diagnosticAiring ? { at: diagnosticAiring.startMs } : undefined,
    { query: { enabled: !paused && player.status === "playing", retry: false } },
  );
  const tracksBody = unwrap(tracks.data);
  const audioAtPlayhead = audioChoice(tracksBody, audioPreference);
  useEffect(() => setAlternateAudio(audioAtPlayhead.alternate), [audioAtPlayhead.alternate]);
  const previousBlockRef = useRef<{ channelId: string; scheduleBlockId?: string }>({ channelId: channel.id });
  const driftedBlockRef = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (previousBlockRef.current.channelId !== channel.id) {
      previousBlockRef.current = { channelId: channel.id };
      driftedBlockRef.current = undefined;
    }
    if (!diagnosticAiring || !scheduleBlockId || serverNowMs === undefined) return;
    if (previousBlockRef.current.scheduleBlockId === scheduleBlockId) return;
    clientDiagnostics.record({
      event: "player.schedule_block_changed",
      playbackSessionId: player.playbackSessionId,
      channelId: channel.id,
      scheduleBlockId,
      previousScheduleBlockId: previousBlockRef.current.scheduleBlockId,
      blockKind:
        diagnosticAiring.kind === "program" || diagnosticAiring.kind === "filler"
          ? diagnosticAiring.kind
          : "flex",
      viewerTimeMs: Math.round(viewerTimeMs),
      serverTimeMs: Math.round(serverNowMs),
    });
    previousBlockRef.current.scheduleBlockId = scheduleBlockId;
  }, [channel.id, diagnosticAiring, player.playbackSessionId, scheduleBlockId, serverNowMs, viewerTimeMs]);
  useEffect(() => {
    if (!scheduleBlockId || serverNowMs === undefined || driftedBlockRef.current === scheduleBlockId) return;
    const driftMs = Math.round(viewerTimeMs - serverNowMs);
    if (Math.abs(driftMs) < PLAYHEAD_DRIFT_THRESHOLD_MS) return;
    clientDiagnostics.record({
      event: "player.playhead_drift",
      playbackSessionId: player.playbackSessionId,
      channelId: channel.id,
      viewerTimeMs: Math.round(viewerTimeMs),
      serverTimeMs: Math.round(serverNowMs),
      driftMs,
    });
    driftedBlockRef.current = scheduleBlockId;
  }, [channel.id, player.playbackSessionId, scheduleBlockId, serverNowMs, viewerTimeMs]);
  // Audio (#2088 decision 6): the programme's default and its alternate, the viewer's to choose.
  // Until its tracks are known, the channel's language, shown: the channel may be set to a track
  // the programme doesn't carry (set to French, but this film is English-only), and that selection
  // stays VISIBLE rather than dropped, since it still applies when a programme with that track airs.
  const audioValue = channel.policy?.playout?.audioLanguage || AUTO_SENTINEL;
  const audio =
    audioAtPlayhead.options.length > 1
      ? { onSelect: setAudioPreference, options: audioAtPlayhead.options }
      : {
          options: withSaved(audioOptions(tracksBody?.audio ?? []), audioValue).map((option) => ({
            label: option.label,
            selected: option.value === audioValue,
            value: option.value,
          })),
        };

  // Quality (#2088 decision 7): Auto, naming the rung on air, then the channel's rungs. A remembered
  // rung this channel doesn't offer shows Auto chosen, because Auto is what plays.
  const { rungs, height: onAir } = player.ladder;
  const pinned = quality !== "auto" && rungs.some((rung) => rung.height === quality);
  const qualityOptions =
    rungs.length > 0
      ? [
          { label: onAir ? `Auto · ${rungLabel(onAir)}` : "Auto", selected: !pinned, value: "auto" },
          ...rungs.map((rung) => ({
            label: rungLabel(rung.height),
            selected: pinned && rung.height === quality,
            value: String(rung.height),
          })),
        ]
      : undefined;

  const osdChannel = tuner?.requestedChannel ?? channel;
  const tuning = player.status === "loading";
  const channelLabel = `CH ${osdChannel.number} · ${osdChannel.name}`;

  // The lower third and the playbar, read from the timeline the way every client reads it (§9). The
  // live edge is the server's clock, projected forward since the read; until the first read it is
  // this machine's.
  const filler = osdChannel.policy?.filler;
  const chromeData = playbarFromTimeline({
    airings,
    channel: {
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

  // The station-ID card (§9.1): every settled tune shows it for five seconds, while the controls are
  // hidden. A retune drops it until the new channel settles, then shows it afresh.
  const [ident, setIdent] = useState(false);
  useEffect(() => {
    if (tuning || player.status !== "playing") {
      setIdent(false);
      return;
    }
    setIdent(true);
    const timer = window.setTimeout(() => setIdent(false), IDENT_MS);
    return () => window.clearTimeout(timer);
  }, [tuning, player.status]);

  // The channel-change sound (#1620), a dial set: a clunk, then static until the new channel's first
  // frame, cut at lock. It starts as the tuned channel changes — the moment the viewer asks, not when
  // transport catches up — and only once a picture has played here: a cold start is not a channel
  // change, and an autoplayed first tune has had no gesture. The module owns the rest of the gate
  // (the viewer's preference, the player's mute and volume, the page's user activation).
  //
  // While the static plays, the programme's own audio is muted, as a set's is; it comes back at lock.
  // That is a mute of the element alone: the player's own mute state is untouched, so its button
  // still shows the viewer's choice, and the element is only unmuted again if the static muted it.
  const [switchSoundOn, setSwitchSoundOn] = useSwitchSoundPreference();
  const soundedChannelRef = useRef(osdChannel.id);
  const switchSoundRef = useRef<{ sound: SwitchSound; programmeMuted: boolean } | undefined>(undefined);
  const endSwitchSound = useCallback(() => {
    const running = switchSoundRef.current;
    if (!running) return;
    switchSoundRef.current = undefined;
    running.sound.lock();
    if (videoRef.current?.muted) videoRef.current.muted = running.programmeMuted;
  }, []);
  useEffect(() => {
    if (soundedChannelRef.current === osdChannel.id) return;
    soundedChannelRef.current = osdChannel.id;
    const video = videoRef.current;
    if (!heldFrame || !video) return;
    // Surfing on while the static still runs: that one ends, and the new clunk takes over.
    endSwitchSound();
    const sound = startChannelSwitchSound(video, { enabled: switchSoundOn });
    if (!sound) return;
    switchSoundRef.current = { sound, programmeMuted: video.muted };
    video.muted = true;
  }, [osdChannel.id, heldFrame, switchSoundOn, endSwitchSound]);
  // Lock: the first frame has decoded (or the tune failed), so the static cuts off.
  useEffect(() => {
    if (!tuning) endSwitchSound();
  }, [tuning, endSwitchSound]);
  useEffect(() => endSwitchSound, [endSwitchSound]);

  // The one right-hand sheet (Settings, Info, Shortcuts or Channels) and 0-9 direct tune (#1659 W1).
  // I opens and closes Info, ? Shortcuts, G Channels; digits open a number entry; Enter tunes it at
  // once; Escape closes the sheet, or with none open leaves. Closing a sheet hands focus back to what
  // opened it (its button, or the frame for a key).
  const [sheet, setSheet] = useState<WatchSheet | undefined>(undefined);
  const sheetOpenerRef = useRef<HTMLElement | null>(null);
  const toggleSheet = (next: WatchSheet) => {
    if (!sheet && document.activeElement instanceof HTMLElement)
      sheetOpenerRef.current = document.activeElement;
    setSheet((open) => (open === next ? undefined : next));
  };
  const closeSheet = useCallback(() => setSheet(undefined), []);
  useEffect(() => {
    if (sheet) return;
    sheetOpenerRef.current?.focus({ preventScroll: true });
    sheetOpenerRef.current = null;
  }, [sheet]);
  // Info closes by itself after eight seconds while the channel plays; paused, it stays to be read.
  const viewerPaused = player.liveTransport.state.mode === "paused";
  useEffect(() => {
    if (sheet !== "info" || viewerPaused) return;
    const timer = window.setTimeout(closeSheet, INFO_PLAYING_MS);
    return () => window.clearTimeout(timer);
  }, [sheet, viewerPaused, closeSheet]);
  // A preview card is open on the playbar: the controls stay shown under it.
  const [previewOpen, setPreviewOpen] = useState(false);
  const tunerTune = tuner?.tune;
  const directTune = useCallback((channelId: string) => tunerTune?.(channelId), [tunerTune]);
  const entry = useNumberEntry({ channels: drawer?.channels ?? [], onTune: directTune });
  const onShortcut = (key: string): boolean => {
    // Esc closes the top layer, then leaves Watch (#2088 decision 11).
    if (key === "Escape") {
      if (sheet) closeSheet();
      else onLeave();
      return true;
    }
    if (key === "?") {
      toggleSheet("shortcuts");
      return true;
    }
    if ((key === "c" || key === "C") && cycleSubtitles) {
      cycleSubtitles();
      return true;
    }
    if ((key === "i" || key === "I") && info) {
      toggleSheet("info");
      return true;
    }
    if (!tuner || !drawer) return false;
    if (entry.press(key)) return true;
    if (key === "Enter") return entry.commit();
    if (key === "g" || key === "G") {
      toggleSheet("channels");
      return true;
    }
    return false;
  };

  // The number readout, top right of the frame (the console mock): the digits so far, dashed out
  // to the longest channel number, over "ENTER CHANNEL" or, for a number no channel has, "NO SUCH
  // CHANNEL".
  const numberReadout = entry.digits ? (
    <div className="absolute top-14 right-7 text-right" role="status">
      <div className="text-signal font-mono text-[52px] font-semibold tracking-[.1em] [text-shadow:0_0_18px_rgb(255_176_32/.5)]">
        {entry.missed ? entry.digits : entry.digits.padEnd(entry.width, "–")}
      </div>
      <div
        className={cn(
          "animate-pulse font-mono text-[11px] tracking-[.2em]",
          entry.missed ? "text-onair-300" : "text-signal/65",
        )}
      >
        {entry.missed ? "NO SUCH CHANNEL" : "ENTER CHANNEL"}
      </div>
    </div>
  ) : undefined;

  // The open sheet, over the dimmed picture. Channels is still the console drawer (#1659 W1) until it
  // is rebuilt on the sheet.
  const openSheet =
    sheet === "settings" ? (
      <PlayerSheet
        channelLabel={channelLabel}
        density={density}
        icon="settings"
        inline={onPhone}
        onClose={closeSheet}
        title="Settings"
      >
        <PlayerSettings
          audio={audio}
          density={density}
          // Settings hands its place to Playback info, and focus goes back to ⚙, Settings' opener: the
          // row pressed leaves with Settings, and focus on <body> would answer no key, not even Esc.
          onPlaybackInfo={() => {
            sheetOpenerRef.current?.focus({ preventScroll: true });
            setSheet("playback-info");
          }}
          onSwitchSound={setSwitchSoundOn}
          quality={
            qualityOptions && {
              onSelect: (value) => setQuality(value === "auto" ? "auto" : Number(value)),
              options: qualityOptions,
            }
          }
          subtitles={
            subtitleTracks.length > 0
              ? {
                  onSelect: (value) =>
                    setSubtitles(value === "off" ? "off" : (subtitleTracks[Number(value)] ?? "off")),
                  options: [
                    { label: "Off", selected: subtitleActive < 0, value: "off" },
                    ...subtitleTracks.map((track, i) => ({
                      label: atPlayhead.labelOf(track),
                      selected: i === subtitleActive,
                      value: String(i),
                    })),
                  ],
                }
              : undefined
          }
          switchSound={switchSoundOn}
        />
      </PlayerSheet>
    ) : sheet === "playback-info" ? (
      <PlayerSheet
        channelLabel={channelLabel}
        density={density}
        icon="info"
        inline={onPhone}
        onClose={closeSheet}
        title="Playback info"
      >
        <PlaybackInfoSheet
          auto={!pinned}
          channelId={channel.id}
          density={density}
          isAdmin={isAdmin}
          lagSeconds={player.liveTransport.state.lagSeconds}
          playbackFacts={player.playbackFacts}
          rungLabel={rungLabel}
          sessionId={player.playbackSessionId}
        />
      </PlayerSheet>
    ) : sheet === "info" && info ? (
      <PlayerSheet
        channelLabel={channelLabel}
        density={density}
        icon="info"
        inline={onPhone}
        onClose={closeSheet}
        title="Info"
      >
        <PlayerInfo density={density} formatTime={formatGuideTime} info={info} />
      </PlayerSheet>
    ) : sheet === "shortcuts" ? (
      <PlayerSheet
        channelLabel={channelLabel}
        density={density}
        icon="keyboard"
        inline={onPhone}
        onClose={closeSheet}
        title="Keyboard shortcuts"
      >
        <PlayerShortcuts
          density={density}
          rows={shortcutRows({
            info: info !== undefined,
            pictureInPicture: document.pictureInPictureEnabled === true,
            subtitles: cycleSubtitles !== undefined,
            tuner: tuner !== undefined && drawer !== undefined,
          })}
        />
      </PlayerSheet>
    ) : sheet === "channels" && tuner && drawer ? (
      <div className="absolute inset-y-0 right-0 z-40 flex max-w-full">
        <ChannelDrawer
          channels={drawer.channels}
          tunedId={osdChannel.id}
          favourites={drawer.favourites}
          recent={drawer.recent}
          nowPercent={drawer.nowPercent}
          onTune={tuner.tune}
          onFavourite={drawer.onFavourite}
          onClose={closeSheet}
        />
      </div>
    ) : undefined;

  // A stream that stopped has nothing for the playbar to stand on, so Retry and the way out take the
  // frame in the chrome's place — and on a portrait phone, which draws no chrome, over the picture.
  const errorCover =
    player.status === "error" ? (
      <ThemeScope theme="dark">
        <PlayerCover
          channelLabel={channelLabel}
          density={density}
          kind="error"
          onLeave={onLeave}
          onRetry={tuner?.retry}
          title="The stream stopped."
        />
      </ThemeScope>
    ) : undefined;

  // The transport and the playbar, acting through the player: the same for both forms of the chrome.
  const transportFor = (chrome: VideoPlayerChrome): PlayerTransportProps => ({
    channelsOpen: tuner && drawer ? sheet === "channels" : undefined,
    density,
    fullscreen: chrome.fullscreen,
    infoOpen: sheet === "info",
    pictureInPicture: chrome.pictureInPicture,
    live: player.liveTransport.state,
    muted: chrome.muted,
    onChannels: tuner && drawer ? () => toggleSheet("channels") : undefined,
    onFullscreen: chrome.toggleFullscreen,
    onGoLive: chrome.goLive,
    onInfo: info ? () => toggleSheet("info") : undefined,
    onLeave,
    onPictureInPicture: chrome.togglePictureInPicture,
    onSeekBack: () => chrome.seekBy(-SEEK_STEP_MS),
    onSeekForward: () => chrome.seekBy(SEEK_STEP_MS),
    onSettings: () => toggleSheet("settings"),
    onSubtitles: cycleSubtitles,
    subtitlesOn: subtitleActive >= 0,
    onStartOver: startOverMs === undefined ? undefined : () => chrome.seekTo(startOverMs),
    onTogglePlay: chrome.toggle,
    onToggleMute: () => chrome.setMuted(!chrome.muted),
    onVolume: chrome.setVolume,
    playing: chrome.playing,
    pointer: !touch,
    settingsOpen: sheet === "settings" || sheet === "playback-info",
    volume: chrome.volume,
  });
  const playbarFor = (chrome: VideoPlayerChrome) =>
    airings.length > 0
      ? {
          onSeek: chrome.seekTo,
          playheadMs: viewerTimeMs,
          segments: chromeData.segments,
          window: chromeData.window,
        }
      : undefined;

  // The star pins the tuned channel to the viewer's Favorites, as the drawer's star does (#1816).
  const isFavourite = drawer?.favourites.includes(osdChannel.id) ?? false;

  // The chrome over the picture (§9), or the error cover in its place. On a portrait phone the picture
  // carries only the station-ID card (or the error cover), and the planes stack in the panel under it,
  // drawn from here into the panel so they act through the same player as the overlay does.
  const renderChrome = (chrome: VideoPlayerChrome) => (
    <>
      <ReportPictureInPicture active={chrome.pictureInPicture} report={onPictureInPicture} />
      {onPhone ? (
        <>
          {errorCover ?? (ident ? <PlayerStationId band={chromeData.band} density="touch" /> : null)}
          {panelEl &&
            createPortal(
              <PlayerPortrait
                band={chromeData.band}
                density={density}
                filmstrip={playbarFor(chrome)}
                formatTime={formatGuideTime}
                sheet={openSheet}
                surf={
                  tuner && {
                    canPrevious: previousId !== undefined,
                    canSurf: tuner.canSurf,
                    favourite: isFavourite,
                    onChannelDown: () => tuner.step(-1),
                    onChannelUp: () => tuner.step(1),
                    onFavourite: drawer ? () => drawer.onFavourite(osdChannel.id, !isFavourite) : undefined,
                    onPrevious: () => previousId && tuner.tune(previousId),
                  }
                }
                transport={transportFor(chrome)}
              />,
              panelEl,
            )}
        </>
      ) : (
        <>
          <HoldWhile hold={chrome.hold} on={previewOpen || sheet !== undefined} />
          <HideForIdent hide={chrome.hide} ident={ident} />
          {errorCover ?? (
            <PlayerChrome
              band={chromeData.band}
              density={density}
              formatTime={formatGuideTime}
              ident={ident}
              onPreviewChange={setPreviewOpen}
              playbar={playbarFor(chrome)}
              sheet={openSheet}
              transport={transportFor(chrome)}
              visible={chrome.controlsShown && !tuning}
            />
          )}
        </>
      )}
    </>
  );

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
        tuning || numberReadout ? (
          <>
            {tuning && player.stillURL && (
              <img src={player.stillURL} alt="" className="absolute inset-0 h-full w-full object-contain" />
            )}
            {tuning && <TunerLoader channel={osdChannel} heldFrame={heldFrame} />}
            {numberReadout}
          </>
        ) : undefined
      }
      onShortcut={onShortcut}
      attach={attach}
      onChannelStep={tuner?.step}
      // Wider than a portrait phone, the picture fills the window (letterboxed by the <video>); a
      // portrait phone keeps its 16:9 picture over the panel.
      className={cn("bg-black", !onPhone && "aspect-auto h-full")}
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
      <div className={cn("relative", !onPhone && "min-h-0 flex-1")}>
        {playerEl}
        {/* While the wash is up, its centred channel line is the visible readout, so the OSD card
            would only repeat it; it stays as the screen-reader announcement. */}
        {(tuning || tuner?.acknowledging) && tuner && (
          <TunerOSD
            number={osdChannel.number}
            name={osdChannel.name}
            currentTitle={tuner.currentTitle}
            className={tuning ? "sr-only" : "absolute top-4 left-4 z-[2]"}
          />
        )}
      </div>
      {/* The portrait panel's host: layout only, the panel itself is @loomarr/ui's PlayerPortrait. */}
      {onPhone ? (
        <div ref={setPanelEl} style={{ display: "flex", flex: 1, flexDirection: "column", minHeight: 0 }} />
      ) : null}
    </section>
  );
};

export type { ChannelWatchProps };
export { ChannelWatch };
