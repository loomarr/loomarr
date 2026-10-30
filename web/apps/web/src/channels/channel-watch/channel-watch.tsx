import * as channelsApi from "@loomarr/api/endpoints/channels";
import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { formatGuideTime } from "@loomarr/core/guide";
import { ThemeScope } from "@loomarr/design-system";
import {
  PlayerChrome,
  PlayerCover,
  PlayerSettings,
  PlayerSheet,
  playbarFromTimeline,
  type SurfChannelData,
  WatchingPanel,
  type WatchingScheduleData,
} from "@loomarr/ui";
import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { AUTO_SENTINEL, audioOptions, withSaved } from "@/channels/channel-default-audio";
import { ChannelDrawer, type DrawerChannel } from "@/channels/channel-drawer";
import { useHlsPlayer } from "@/channels/use-hls-player";
import { useNumberEntry } from "@/channels/use-number-entry";
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
// ⚠ Settings only SHOWS the channel's default audio language. An admin sets it on the channel's info
// page (#2088); the per-viewer choice between renditions comes with the alternate-audio step.

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
  // What a portrait phone shows under the picture (#1785, native mock 5e), read from the guide: the
  // tuned channel's now and next, the viewer's favourites, the channel Previous returns to, and the
  // wall clock.
  phone?: {
    schedule?: WatchingScheduleData;
    favourites: readonly SurfChannelData[];
    previousId?: string;
    clockLabel?: string;
  };
}

const PLAYHEAD_DRIFT_THRESHOLD_MS = 30_000;

// The one sheet the chrome can have open at a time (§9.1, plane 5).
type WatchSheet = "channels" | "settings";

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

const ChannelWatch = ({
  channel,
  isAdmin,
  onLeave,
  tuner,
  drawer,
  onTuneSettled,
  phone,
}: ChannelWatchProps) => {
  // A portrait phone (#1785, native mock 5e, decision N5): the picture edge to edge with nothing
  // drawn on it, and the controls, what's next and the favourites under it. Wider draws the §9
  // chrome over the picture; its portrait form comes with the phone step (#2088 step 10).
  const onPhone = usePhoneWidth() && phone !== undefined;
  // A finger gets the chrome's touch form: larger targets, no hover card, no tooltips (§9.5).
  const touch = useCoarsePointer();
  const density = touch ? "touch" : "pointer";
  const tunerReady = tuner?.ready;
  // The tuner warms this Channel's neighbours once its own manifest has arrived, not before: the
  // warm requests would otherwise share the browser's per-host connections with the target's
  // manifest and first segments (#1484). It is still well ahead of the first frame.
  const onManifest = useCallback(() => tunerReady?.(channel.id), [channel.id, tunerReady]);
  const player = useHlsPlayer(channel.id, tuner?.attempt, onManifest);
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
  const diagnosticAiring = airings.find(
    (airing) => !airing.nominal && viewerTimeMs >= airing.startMs && viewerTimeMs < airing.stopMs,
  );
  const scheduleBlockId = diagnosticAiring?.scheduleBlockId;
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

  const osdChannel = tuner?.requestedChannel ?? channel;
  const tuning = player.status === "loading";
  const channelLabel = `CH ${osdChannel.number} · ${osdChannel.name}`;

  // The lower third and the playbar, read from the timeline the way every client reads it (§9). The
  // live edge is the server's clock, projected forward since the read; until the first read it is
  // this machine's.
  const chromeData = playbarFromTimeline({
    airings,
    channel: { name: osdChannel.name, number: String(osdChannel.number) },
    live: player.liveTransport.state,
    nowMs: serverNowMs ?? Date.now(),
    playheadMs: viewerTimeMs,
  });

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

  // The one right-hand sheet (Settings or Channels) and 0-9 direct tune (#1659 W1). G opens and
  // closes Channels; digits open a number entry; Enter tunes it at once; Escape closes the sheet.
  // Closing a sheet hands focus back to what opened it (its button, or the frame for G).
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
  // A preview card is open on the playbar: the controls stay shown under it.
  const [previewOpen, setPreviewOpen] = useState(false);
  const tunerTune = tuner?.tune;
  const directTune = useCallback((channelId: string) => tunerTune?.(channelId), [tunerTune]);
  const entry = useNumberEntry({ channels: drawer?.channels ?? [], onTune: directTune });
  const onShortcut = (key: string): boolean => {
    if (key === "Escape" && sheet) {
      closeSheet();
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

  // Go live acts on the player's own <video> through the live transport, as the play toggle does.
  const withVideo = (act: (video: HTMLVideoElement) => unknown) => {
    const video = videoRef.current;
    if (video) void act(video);
  };

  // The open sheet, over the dimmed picture. Channels is still the console drawer (#1659 W1) until it
  // is rebuilt on the sheet.
  const openSheet =
    sheet === "settings" ? (
      <PlayerSheet
        channelLabel={channelLabel}
        density={density}
        icon="settings"
        onClose={closeSheet}
        title="Settings"
      >
        <PlayerSettings
          audio={audio}
          density={density}
          onSwitchSound={setSwitchSoundOn}
          switchSound={switchSoundOn}
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

  // The chrome over the picture (§9), or the error cover in its place.
  const renderChrome = (chrome: VideoPlayerChrome) => (
    <>
      <HoldWhile hold={chrome.hold} on={previewOpen || sheet !== undefined} />
      {errorCover ?? (
        <PlayerChrome
          band={chromeData.band}
          density={density}
          formatTime={formatGuideTime}
          onPreviewChange={setPreviewOpen}
          playbar={
            airings.length > 0
              ? { playheadMs: viewerTimeMs, segments: chromeData.segments, window: chromeData.window }
              : undefined
          }
          sheet={openSheet}
          transport={{
            channelsOpen: tuner && drawer ? sheet === "channels" : undefined,
            density,
            fullscreen: chrome.fullscreen,
            live: player.liveTransport.state,
            muted: chrome.muted,
            onChannels: tuner && drawer ? () => toggleSheet("channels") : undefined,
            onFullscreen: chrome.toggleFullscreen,
            onGoLive: () => withVideo((video) => player.liveTransport.goLive(video)),
            onLeave,
            onSettings: () => toggleSheet("settings"),
            onTogglePlay: chrome.toggle,
            onToggleMute: () => chrome.setMuted(!chrome.muted),
            onVolume: chrome.setVolume,
            playing: chrome.playing,
            pointer: !touch,
            settingsOpen: sheet === "settings",
            volume: chrome.volume,
          }}
          visible={chrome.controlsShown && !tuning}
        />
      )}
    </>
  );

  // The player. `live` = no seek bar of its own; `renderChrome` = the §9 chrome; `attach` binds
  // hls.js. Fullscreen is the player's OWN state, handed to the chrome.
  const playerEl = (
    <VideoPlayer
      live
      liveTransport={player.liveTransport}
      renderChrome={onPhone ? undefined : renderChrome}
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

  // The star in the panel's channel line pins the tuned channel, as the drawer's star does on wider
  // screens and the iPhone's does (#1816). Without it a phone had no way to fill Favorites.
  const isFavourite = drawer?.favourites.includes(osdChannel.id) ?? false;
  const phonePanel =
    onPhone && phone && tuner ? (
      <WatchingPanel
        density="touch"
        channel={{ name: osdChannel.name, number: String(osdChannel.number) }}
        schedule={phone.schedule}
        clockLabel={phone.clockLabel}
        live={player.liveTransport.state}
        canPrevious={phone.previousId !== undefined}
        canSurf={tuner.canSurf}
        onPrevious={() => phone.previousId && tuner.tune(phone.previousId)}
        onChannelDown={() => tuner.step(-1)}
        onChannelUp={() => tuner.step(1)}
        onPause={() => withVideo((video) => player.liveTransport.pause(video))}
        onPlay={() => withVideo((video) => player.liveTransport.play(video))}
        onGoLive={() => withVideo((video) => player.liveTransport.goLive(video))}
        onLeave={onLeave}
        favourites={phone.favourites}
        isFavourite={isFavourite}
        onToggleFavourite={drawer ? () => drawer.onFavourite(osdChannel.id, !isFavourite) : undefined}
        onTune={tuner.tune}
      />
    ) : undefined;

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
        {onPhone && errorCover}
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
      {phonePanel}
    </section>
  );
};

export type { ChannelWatchProps };
export { ChannelWatch };
