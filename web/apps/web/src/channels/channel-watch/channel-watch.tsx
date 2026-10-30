import * as channelsApi from "@loomarr/api/endpoints/channels";
import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import type { GuideAiring } from "@loomarr/api/models/guideAiring";
import { unwrap } from "@loomarr/api/unwrap";
import { type SurfChannelData, WatchingPanel, type WatchingScheduleData } from "@loomarr/ui";
import { ChevronDown, ChevronUp, Play, Volume2, X } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { AUTO_SENTINEL, audioOptions, withSaved } from "@/channels/channel-default-audio";
import { ChannelDrawer, type DrawerChannel } from "@/channels/channel-drawer";
import { useHlsPlayer } from "@/channels/use-hls-player";
import { useNumberEntry } from "@/channels/use-number-entry";
import { TunerLoader } from "@/components/loomarr/shell/tuner-loader";
import { Button } from "@/components/ui/button";
import { VideoPlayer } from "@/components/ui/video-player";
import { TimelineScrubber } from "@/components/ui/video-player/timeline-scrubber";
import { TrackSelectMenu } from "@/components/ui/video-player/track-select-menu";
import { clientDiagnostics } from "@/diagnostics/client-reporter";
import { usePhoneWidth } from "@/lib/use-phone-width";
import { cn } from "@/lib/utils";
import { type SwitchSound, startChannelSwitchSound } from "../switch-sound";
import { TunerOSD } from "../tuner-osd";
import type { TuneAttempt } from "../tuner-timing";
import type { TuneDirection } from "../use-channel-tuner";
import { useSwitchSoundPreference } from "../use-switch-sound-preference";

// ChannelWatch — the full-bleed `/watch/$id` player: play a channel live in the browser (§9.1,
// #2088).
//
// The player itself is the shared VideoPlayer primitive in `live` mode with hls.js bound through its
// `attach` seam (useHlsPlayer). In place of a seek bar it passes VideoPlayer a `scrubber`: the
// mini-guide timeline (ChannelTimeline, §9.1 V47), so the control bar shows where you are in the
// schedule. This component owns the surrounding SURFACE: the picture filling the window, the channel
// controls, and the ✕ that leaves.
//
// ⚠ The audio menu here only SHOWS the channel's default language. An admin sets it on the channel's
// info page (#2088); the per-viewer choice between renditions comes with the alternate-audio step.

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

// clock renders ms-into-a-programme as m:ss (the mock's {elapsed} / {total} format).
const clock = (ms: number): string => {
  const total = Math.max(0, Math.floor(ms / 1000));
  return `${Math.floor(total / 60)}:${String(total % 60).padStart(2, "0")}`;
};

// programmeTime renders the controls-row time for the programme airing NOW (the mock's
// "{elapsed} / {total}   {remaining}"): how far into the current show, the show's length, and the
// time left — all PROGRAMME time from the schedule, not video time. null when nothing is airing at
// this instant (the player omits the time). Rendered as a node so the "/ total" is dimmed like the
// mock and the "left" reads grey beside it.
const programmeTime = (airings: GuideAiring[], now: number): React.ReactNode => {
  const current = airings.find((a) => now >= a.startMs && now < a.stopMs);
  if (!current) return null;
  const elapsed = now - current.startMs;
  const total = current.stopMs - current.startMs;
  const leftMin = Math.max(0, Math.round((current.stopMs - now) / 60_000));
  return (
    <span className="flex items-center gap-2">
      <span>
        {clock(elapsed)} <span className="text-muted-foreground">/ {clock(total)}</span>
      </span>
      <span className="text-muted-foreground">{leftMin}m left</span>
    </span>
  );
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
  // drawn on it, and the controls, what's next and the favourites under it. Wider keeps the frame
  // with the overlay controls, the channels drawer and the keyboard hints.
  const onPhone = usePhoneWidth() && phone !== undefined;
  const tunerReady = tuner?.ready;
  // The tuner warms this Channel's neighbours once its own manifest has arrived, not before: the
  // warm requests would otherwise share the browser's per-host connections with the target's
  // manifest and first segments (#1484). It is still well ahead of the first frame.
  const onManifest = useCallback(() => tunerReady?.(channel.id), [channel.id, tunerReady]);
  const player = useHlsPlayer(channel.id, tuner?.attempt, onManifest);
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
  // `active` gates the idle poster vs the live player, and it now starts TRUE: opening Watch tunes
  // in (§9.1 V54). Watch is the first section a channel opens on, and a player that sits behind a
  // second click makes "open the channel" a two-step act to do the obvious thing.
  //
  // ⚠ **This does NOT guarantee the video starts, and that is a browser rule, not a bug.** Autoplay
  // with sound requires user activation. Reaching here by CLICKING a channel keeps the document's
  // sticky activation, so it plays; arriving by a pasted link or a reload has no activation and the
  // browser will hold the first frame with the controls showing. Mounting the player either way is
  // still the right call — a paused player with a play button is one click from playing, which is
  // exactly where the old poster left you, and every other visit skips the click entirely.
  //
  // The poster is kept for the paused/off-air case below, which is a different claim: nothing to
  // play at all, rather than something waiting for permission to start.
  const [active, setActive] = useState(true);

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

  // The pickers' options are the tracks the airing programme actually carries — fetched, never
  // hardcoded. `enabled` gates the probe on a playing channel (a paused one has nothing to probe).
  const tracks = channelsApi.useChannelTracks(channel.id, {
    query: { enabled: !paused && player.status === "playing", retry: false },
  });
  const tracksBody = unwrap(tracks.data);

  // The mini-guide scrubber's data — the channel's schedule strip (now + next few + the commercial
  // breaks between them, each with episode detail + a TMDB still). Only while active (a poster needs
  // no timeline) and unpaused. Refetched on an interval so the live playhead and "what's next" stay
  // current as programmes roll.
  const timeline = channelsApi.useChannelTimeline(channel.id, undefined, {
    query: { enabled: active && !paused, retry: false, refetchInterval: 30_000 },
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
  // but this film is English-only). Keep that selection VISIBLE in the menu rather than dropping it:
  // the preference still applies when a programme with that track airs (withSaved).
  const audioOpts = withSaved(audioOptions(tracksBody?.audio ?? []), audioValue || AUTO_SENTINEL);

  // The mini-guide scrubber (§9.1 V47) fills the player's full-width `scrubber` slot. Shown once we
  // have a timeline and the stream is healthy; otherwise the player's control bar has no scrubber row.
  const scrubber =
    airings.length > 0 && player.status !== "error" ? (
      <TimelineScrubber airings={airings} nowMs={player.liveTransport.state.viewerTimeMs} />
    ) : undefined;

  // The controls-row time (mock): elapsed / total + "N min left" for the programme airing now, from
  // the schedule (the player has no source for programme time, so channel-watch derives it).
  const timeLeft = programmeTime(airings, player.liveTransport.state.viewerTimeMs);
  const osdChannel = tuner?.requestedChannel ?? channel;
  const tuning = player.status === "loading";

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

  // The channels drawer and 0-9 direct tune (#1659 W1, from the console mock). G opens and closes
  // the drawer; digits open a number entry; Enter tunes it at once. Closing the drawer hands focus
  // back to the button that opened it.
  const [drawerOpen, setDrawerOpen] = useState(false);
  const drawerButtonRef = useRef<HTMLButtonElement>(null);
  const tunerTune = tuner?.tune;
  const directTune = useCallback((channelId: string) => tunerTune?.(channelId), [tunerTune]);
  const entry = useNumberEntry({ channels: drawer?.channels ?? [], onTune: directTune });
  const closeDrawer = useCallback(() => {
    setDrawerOpen(false);
    drawerButtonRef.current?.focus({ preventScroll: true });
  }, []);
  const onShortcut = (key: string): boolean => {
    if (!tuner || !drawer) return false;
    if (entry.press(key)) return true;
    if (key === "Enter") return entry.commit();
    if (key === "g" || key === "G") {
      setDrawerOpen((open) => !open);
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

  // The player's live top bar: "CH {n}" (left, after the LIVE badge) + the channel name, matching the
  // mock's "CH 3" line. The encoder line ("h264 · 1080p") the mock also shows is admin telemetry not
  // fetched here; the channel identity is what a viewer needs.
  const topBar = (
    <>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="text-static-200 hover:bg-static-700 hover:text-static-50 -ml-1 size-8"
        aria-label="Leave Watch"
        onClick={onLeave}
      >
        <X className="size-4" aria-hidden />
      </Button>
      <span className="text-static-300 shrink-0 font-mono text-xs">CH {channel.number}</span>
      <span className="text-static-200 min-w-0 truncate text-xs">{channel.name}</span>
    </>
  );

  // Audio IN the player bar (§9.1 V47), beside fullscreen: the channel's default track, read-only for
  // everyone here (an admin changes it on the channel's info page), and the channel-change sound
  // toggle. Options are the airing's real tracks (fetched).
  const barControls = (
    <>
      {tuner && (
        <fieldset className="flex min-w-0 items-center gap-0.5 border-0 p-0">
          <legend className="sr-only">Channel navigation</legend>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="text-static-200 hover:bg-static-700 hover:text-static-50 size-8"
            aria-label="Channel down"
            disabled={!tuner.canSurf}
            onClick={() => tuner.step(-1)}
          >
            <ChevronDown className="size-4" aria-hidden />
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="text-static-200 hover:bg-static-700 hover:text-static-50 size-8"
            aria-label="Channel up"
            disabled={!tuner.canSurf}
            onClick={() => tuner.step(1)}
          >
            <ChevronUp className="size-4" aria-hidden />
          </Button>
        </fieldset>
      )}
      {tuner && drawer && (
        <button
          ref={drawerButtonRef}
          type="button"
          aria-expanded={drawerOpen}
          onClick={() => setDrawerOpen((open) => !open)}
          className={cn(
            "focus-visible:ring-ring flex h-9 shrink-0 items-center gap-[7px] rounded-lg border px-[13px] text-xs focus-visible:ring-2 focus-visible:outline-none",
            // Pressed is neutral (#1909 Q2): amber is focus or the primary action.
            drawerOpen
              ? "border-foreground bg-static-800 text-foreground font-semibold"
              : "border-static-0/14 bg-static-0/6 text-foreground hover:bg-static-0/12",
          )}
        >
          <span aria-hidden>☰</span> Channels
        </button>
      )}
      <TrackSelectMenu
        icon={Volume2}
        label="Audio"
        options={audioOpts}
        value={audioValue || AUTO_SENTINEL}
        onChange={() => undefined}
        readOnly
        toggles={[
          { label: "Channel-change sound", checked: switchSoundOn, onCheckedChange: setSwitchSoundOn },
        ]}
      />
    </>
  );

  // The player. `live` = no seek; `scrubber` = full-width mini-guide; `topBar`/`timeLeft` = the live
  // chrome; `barControls` = the audio menu beside fullscreen; `attach` binds hls.js. Fullscreen
  // is the player's OWN control. Rendered only when active so the stream is not requested until asked.
  const playerEl = (
    <VideoPlayer
      live
      liveTransport={player.liveTransport}
      scrubber={scrubber}
      // While the wash is up its readout names the channel; the title returns with the picture.
      topBar={tuning ? undefined : topBar}
      timeLeft={timeLeft}
      barControls={barControls}
      // The tuner "acquiring signal" overlay covers the warm-up beat (cold encoder, first segment
      // not cut yet) so the viewer sees a channel tuning in, not a black frame. Only while loading —
      // an error shows its own message below, and a playing stream needs no overlay.
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
      panel={
        drawerOpen && tuner && drawer ? (
          <ChannelDrawer
            channels={drawer.channels}
            tunedId={osdChannel.id}
            favourites={drawer.favourites}
            recent={drawer.recent}
            nowPercent={drawer.nowPercent}
            onTune={tuner.tune}
            onFavourite={drawer.onFavourite}
            onClose={closeDrawer}
          />
        ) : undefined
      }
      hints={
        tuner && drawer ? (
          <p className="text-muted-foreground hidden justify-center font-mono text-[11px] tracking-[.08em] lg:flex">
            0–9 TUNE DIRECT · ⌃⌄ CHANNEL · SPACE PAUSE · G CHANNELS · M MUTE
          </p>
        ) : undefined
      }
      onShortcut={onShortcut}
      attach={attach}
      onChannelStep={tuner?.step}
      chrome={!onPhone}
      // Wider than a portrait phone, the picture fills the window (letterboxed by the <video>); a
      // portrait phone keeps its 16:9 picture over the panel.
      className={cn("bg-black", !onPhone && "aspect-auto h-full")}
    />
  );

  // The phone's controls act on the player's own <video> through the live transport, as the overlay's
  // play toggle and live indicator do.
  const withVideo = (act: (video: HTMLVideoElement) => unknown) => {
    const video = videoRef.current;
    if (video) void act(video);
  };
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

  return (
    <section className="flex min-h-0 flex-1 flex-col">
      {paused ? (
        <IdleFrame
          title={`${channel.name} is off air`}
          // The web mock's words; the way back is an admin's, so members get the first sentence.
          sub={
            isAdmin
              ? "This channel is paused, so there's nothing to watch. Resume it under Danger zone."
              : "This channel is paused, so there's nothing to watch."
          }
          onLeave={onLeave}
        />
      ) : active ? (
        <div className="flex min-h-0 flex-1 flex-col">
          <div className={cn("relative", !onPhone && "min-h-0 flex-1")}>
            {playerEl}
            {/* While the wash is up, its centred channel line is the visible readout, so the OSD
                  card would only repeat it; it stays as the screen-reader announcement. */}
            {(tuning || tuner?.acknowledging) && tuner && (
              <TunerOSD
                number={osdChannel.number}
                name={osdChannel.name}
                currentTitle={tuner.currentTitle}
                className={tuning ? "sr-only" : "absolute top-4 left-4 z-[2]"}
              />
            )}
          </div>

          {/* Loading/error needs prose outside the frame. Once media is ready, the compact live
                control in the playback bar owns live/paused/behind state without duplicating it in
                a second status line below the picture. */}
          {player.status !== "playing" && (
            <p className="text-muted-foreground px-4 pt-3 text-xs">
              {player.status === "error" ? (player.error ?? "The stream stopped.") : "Tuning in…"}
            </p>
          )}
          {player.status === "error" && tuner && (
            <Button variant="outline" size="sm" onClick={tuner.retry} className="mx-4 my-3 self-start">
              Retry channel
            </Button>
          )}
          {phonePanel}
        </div>
      ) : (
        <button
          type="button"
          onClick={() => setActive(true)}
          className="group from-static-800 relative flex w-full flex-1 cursor-pointer flex-col items-center justify-center gap-3 bg-radial to-black text-center"
        >
          <span className="bg-signal/90 flex size-14 items-center justify-center rounded-full text-black shadow-lg transition-transform group-hover:scale-105">
            <Play className="size-6 translate-x-0.5 fill-current" aria-hidden />
          </span>
          <span className="flex flex-col gap-1">
            <span className="text-lg font-semibold">Watch live</span>
            <span className="text-muted-foreground text-sm">Join {channel.name} mid-programme.</span>
          </span>
        </button>
      )}
    </section>
  );
};

// IdleFrame is the poster shown when there is nothing to play (paused/off air). It fills the window
// like the player does, so it carries the player's ✕ too: the full-bleed route has no other way out.
const IdleFrame = ({ title, sub, onLeave }: { title: string; sub: string; onLeave: () => void }) => (
  <div className="from-static-800 relative flex w-full flex-1 flex-col items-center justify-center gap-2 bg-radial to-black px-12 text-center">
    <Button
      type="button"
      variant="ghost"
      size="icon"
      className="text-static-200 hover:bg-static-700 hover:text-static-50 absolute top-3 left-3 size-8"
      aria-label="Leave Watch"
      onClick={onLeave}
    >
      <X className="size-4" aria-hidden />
    </Button>
    <p className="text-lg font-semibold">{title}</p>
    <p className="text-muted-foreground text-sm">{sub}</p>
  </div>
);

export type { ChannelWatchProps };
export { ChannelWatch };
