import * as channelsApi from "@loomarr/api/endpoints/channels";
import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { formatGuideTime } from "@loomarr/core/guide";
import { ThemeScope } from "@loomarr/design-system";
import type { PlayerFailureCause } from "@loomarr/player";
import {
  PlayerAudio,
  PlayerChrome,
  PlayerCover,
  PlayerInfo,
  PlayerMore,
  PlayerOptions,
  PlayerReadout,
  PlayerSheet,
  playbarFromTimeline,
  WatchingPanel,
} from "@loomarr/ui";
import { type ReactNode, useCallback, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { AUTO_SENTINEL, audioOptions, withSaved } from "@/channels/channel-default-audio";
import { ChannelDrawer } from "@/channels/channel-drawer";
import { VideoPlayer, type VideoPlayerChrome } from "@/components/loomarr/player/video-player";
import { TunerLoader } from "@/components/loomarr/shell/tuner-loader";
import { useCoarsePointer } from "@/lib/use-coarse-pointer";
import { usePhoneWidth } from "@/lib/use-phone-width";
import { cn } from "@/lib/utils";
import { TunerOSD } from "../tuner-osd";
import { useSwitchSound } from "../use-switch-sound";
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

// HideForIdent clears the controls as the station-ID card arrives, so the card stands alone as the
// mock draws it; moving the pointer brings the controls back in its place.
const HideForIdent = ({ hide, ident }: { hide: VideoPlayerChrome["hide"]; ident: boolean }) => {
  useEffect(() => {
    if (ident) hide();
  }, [hide, ident]);
  return null;
};

const ChannelWatch = ({ channel, isAdmin, onLeave, player, channels }: ChannelWatchProps) => {
  const drawer = channels?.drawer;
  const phone = channels?.phone;
  const onTuneSettled = channels?.onTuneSettled;
  // A portrait phone (#1785, native mock 5e, decision N5): the picture edge to edge with nothing
  // drawn on it, and the controls, what's next and the favourites under it. Wider draws the §9
  // chrome over the picture; its portrait form comes with the phone step (#2088 step 10).
  const onPhone = usePhoneWidth() && phone !== undefined;
  // A finger gets the chrome's touch form: larger targets, no hover card, no tooltips (§9.5).
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

  const { sheet, toggleSheet, closeSheet, entry, onShortcut } = useWatchShortcuts({
    hasInfo: info !== undefined,
    channels: drawer?.channels,
    onLast: goLast,
    onLeave,
    onTune: player.tune,
    viewerPaused: player.liveTransport.state.mode === "paused",
  });

  // The number readout, top right of the frame: the digits so far, dashed out to the longest channel
  // number, over "Enter channel" or, for a number no channel has, "No such channel". Over the
  // picture it sits on a solid ink backing (§9.2), white, never amber.
  const numberReadout = entry.digits ? (
    <ThemeScope theme="dark">
      <PlayerReadout
        density={density}
        digits={entry.missed ? entry.digits : entry.digits.padEnd(entry.width, "–")}
        missed={entry.missed}
      />
    </ThemeScope>
  ) : undefined;

  // The sheet's own name for the channel it is about.
  const sheetChannel = { name: osdChannel.name, number: String(osdChannel.number) };
  const sheetOf = (title: string, body: ReactNode) => (
    <PlayerSheet channel={sheetChannel} density={density} key={title} onClose={closeSheet} title={title}>
      {body}
    </PlayerSheet>
  );
  const live = player.liveTransport.state;

  // The open sheet, over the dimmed picture. Channels is still the console drawer (#1659 W1) until it
  // is rebuilt on the sheet.
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
          last={touch && last && goLast ? { number: last.number, onPress: goLast } : undefined}
          picture={
            qualityOptions
              ? {
                  detail: qualityOptions.find((o) => o.selected)?.label ?? "Auto",
                  onPress: () => toggleSheet("picture"),
                }
              : undefined
          }
          schedule={
            airings.length > 0
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
    ) : sheet === "info" && info ? (
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
  // frame in the chrome's place — and on a portrait phone, which draws no chrome, over the picture.
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
  const renderChrome = (chrome: VideoPlayerChrome) => (
    <>
      <HoldWhile hold={chrome.hold} on={sheet !== undefined} />
      <HideForIdent hide={chrome.hide} ident={ident} />
      {errorCover ?? (
        <PlayerChrome
          band={chromeData.band}
          density={density}
          formatTime={formatGuideTime}
          ident={ident}
          keys={{
            density,
            last: !touch && last && goLast ? { number: last.number, onPress: goLast } : undefined,
            moreOpen: sheet === "more",
            muted: chrome.muted,
            onChannelDown: player.canSurf ? () => player.step(-1) : undefined,
            onChannelUp: player.canSurf ? () => player.step(1) : undefined,
            onMore: () => toggleSheet("more"),
            onToggleMute: () => chrome.setMuted(!chrome.muted),
            onTogglePlay: chrome.toggle,
            playing: chrome.playing,
            pointer: !touch,
            volume: chrome.volume,
          }}
          live={live}
          onGoLive={chrome.goLive}
          sheet={openSheetFor(chrome)}
          top={{
            audioOpen: sheet === "audio",
            density,
            fullscreen: chrome.fullscreen,
            onAudio: () => toggleSheet("audio"),
            onFullscreen: chrome.toggleFullscreen,
            onLeave,
            pointer: !touch,
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
      onChannelStep={player.step}
      // Wider than a portrait phone, the picture fills the window (letterboxed by the <video>); a
      // portrait phone keeps its 16:9 picture over the panel.
      fill={!onPhone}
    />
  );

  // The star in the panel's channel line pins the tuned channel, as the drawer's star does on wider
  // screens and the iPhone's does (#1816). Without it a phone had no way to fill Favorites.
  const isFavourite = drawer?.favourites.includes(osdChannel.id) ?? false;
  const phonePanel =
    onPhone && phone ? (
      <WatchingPanel
        density="touch"
        channel={{ name: osdChannel.name, number: String(osdChannel.number) }}
        schedule={phone.schedule}
        clockLabel={phone.clockLabel}
        live={player.liveTransport.state}
        canPrevious={phone.previousId !== undefined}
        canSurf={player.canSurf}
        onPrevious={() => phone.previousId && player.tune(phone.previousId)}
        onChannelDown={() => player.step(-1)}
        onChannelUp={() => player.step(1)}
        onPause={() => player.liveTransport.pause()}
        onPlay={() => void player.liveTransport.play()}
        onGoLive={() => void player.liveTransport.goLive()}
        onLeave={onLeave}
        favourites={phone.favourites}
        isFavourite={isFavourite}
        onToggleFavourite={drawer ? () => drawer.onFavourite(osdChannel.id, !isFavourite) : undefined}
        onTune={player.tune}
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
        {tuning && (
          <TunerOSD
            number={osdChannel.number}
            name={osdChannel.name}
            currentTitle={player.currentTitle}
            className="sr-only"
          />
        )}
      </div>
      {phonePanel}
    </section>
  );
};

export type { ChannelWatchProps };
export { ChannelWatch };
