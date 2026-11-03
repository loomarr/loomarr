import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import type { PlayerTuneReport, TuneDirection } from "@loomarr/player";
import { useBrowserPlayer } from "@loomarr/player/browser";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { clientDiagnostics } from "@/diagnostics/client-reporter";
import { channelSourcePort } from "../channel-play-url";
import { beginTune, markTunePhase, type TuneAttempt } from "../tuner-timing";
import { deviceProfile } from "../use-hls-player/device-profile";
import type {
  UseWatchPlayerOptions,
  WatchLiveTransport,
  WatchPlayer,
  WatchPlayerStatus,
} from "./use-watch-player.type";

// useWatchPlayer — the Watch page's player: the shared player controller (one tune path for every
// way of asking: a step, a typed number, a pick from the drawer, the URL) running over the browser
// transport, with the URL kept in step with the channel it is tuned to.
//
// The controller owns tuning, latest-request-wins, warming and recovery; the URL is only where the
// viewer's intent can come from outside it (a link, the back button) and where its result is shown.

const NO_CHANNELS: ChannelDTO[] = [];
const CANT_PLAY = "This channel can't be played in the app.";
const REMEMBERED_ATTEMPTS = 8;

const useWatchPlayer = ({ channelId, channels, nowNext, onNavigate }: UseWatchPlayerOptions): WatchPlayer => {
  const profile = useMemo(() => deviceProfile(), []);
  const onNavigateRef = useRef(onNavigate);
  onNavigateRef.current = onNavigate;

  // Timing marks follow the controller's attempts: one is begun as each tune is published and
  // completed as its phases arrive.
  const attempts = useRef(new Map<number, TuneAttempt>());
  const onTune = useCallback((report: PlayerTuneReport) => {
    const attempt = beginTune(report.reason === "step", report.warm);
    attempts.current.set(report.attemptId, attempt);
    for (const stale of [...attempts.current.keys()].slice(0, -REMEMBERED_ATTEMPTS)) {
      attempts.current.delete(stale);
    }
    // The wash is up once the frame after this one has painted.
    if (typeof requestAnimationFrame !== "function") {
      markTunePhase(attempt, "osd");
      return;
    }
    requestAnimationFrame(() => requestAnimationFrame(() => markTunePhase(attempt, "osd")));
  }, []);
  const onPhase = useCallback(
    (attemptId: number, phase: "first-frame" | "manifest") =>
      markTunePhase(attempts.current.get(attemptId), phase),
    [],
  );
  const recordDiagnostic = useCallback(
    (observation: Parameters<typeof clientDiagnostics.record>[0]) => clientDiagnostics.record(observation),
    [],
  );

  const player = useBrowserPlayer({
    channels: channels ?? NO_CHANNELS,
    onPhase,
    onTune,
    profile,
    recordDiagnostic,
    source: channelSourcePort,
  });
  const { snapshot } = player;
  const { goLive, pause, play, retry, step, tuneChannel } = player.controls;

  const lineupKnown = channels !== undefined;
  const requested = channels?.find((channel) => channel.id === channelId);
  // The URL names a channel this app can't play: the player parks, and the page says so.
  const parked = lineupKnown && !requested?.inAppPlayable;
  const catalogReady = snapshot.catalog.length > 0;

  // The URL → the player. `urlTarget` is the channel the URL last named or was last sent to; the
  // router commits our own navigations after the fact, and a commit that is merely catching up to a
  // newer request must never re-tune an older one.
  const urlTarget = useRef<string | undefined>(undefined);
  const ownNavigations = useRef(new Set<string>());
  const wasParked = useRef(false);
  const tunedId = snapshot.channel?.id;
  // Read when the URL moves, not a reason to run again: a change of the tuned channel is the player's own.
  const tunedRef = useRef(tunedId);
  tunedRef.current = tunedId;
  useEffect(() => {
    if (!catalogReady) return;
    if (channelId === urlTarget.current) {
      ownNavigations.current.clear();
      return;
    }
    if (ownNavigations.current.delete(channelId)) return;
    urlTarget.current = channelId;
    if (parked) {
      // Nothing to play here; hold the outgoing channel silent until the viewer leaves.
      pause();
      wasParked.current = true;
      return;
    }
    // Coming back to the channel the player was parked on: its stream was released with the picture.
    const restart = wasParked.current && channelId === tunedRef.current;
    wasParked.current = false;
    void (restart ? retry() : tuneChannel(channelId));
  }, [channelId, catalogReady, parked, pause, retry, tuneChannel]);

  // The player → the URL: a channel the controller tuned is where the URL goes.
  useEffect(() => {
    if (!tunedId || tunedId === urlTarget.current) return;
    urlTarget.current = tunedId;
    ownNavigations.current.add(tunedId);
    onNavigateRef.current(tunedId);
  }, [tunedId]);

  // The tuning channel's picture, decoded off-screen first so a half-loaded image never flashes.
  const stillUri = snapshot.status === "tuning" ? snapshot.stillUri : undefined;
  const stillAttempt = snapshot.attemptId;
  const [decodedStill, setDecodedStill] = useState<string>();
  useEffect(() => {
    if (!stillUri) {
      setDecodedStill(undefined);
      return;
    }
    let current = true;
    const image = new Image();
    image.onload = () => {
      if (!current) return;
      setDecodedStill(stillUri);
      markTunePhase(stillAttempt === undefined ? undefined : attempts.current.get(stillAttempt), "still");
    };
    image.src = stillUri;
    return () => {
      current = false;
    };
  }, [stillUri, stillAttempt]);

  const fallbackLive = useRef(Date.now());
  const live = snapshot.livePlayback ?? {
    lagSeconds: 0,
    mode: "live" as const,
    noticeRevision: 0,
    viewerTimeMs: fallbackLive.current,
  };
  const liveTransport = useMemo<WatchLiveTransport>(
    () => ({ goLive, pause, play, state: live }),
    [goLive, live, pause, play],
  );

  const channel = parked ? requested : channels?.find((candidate) => candidate.id === tunedId);
  const currentTitle = channel
    ? nowNext.find((entry) => entry.channelId === channel.id)?.now?.title
    : undefined;

  let status: WatchPlayerStatus = "loading";
  let error: string | undefined;
  if (parked) {
    status = "error";
    error = CANT_PLAY;
  } else if (snapshot.status === "failed") {
    status = "error";
    error = snapshot.error;
  } else if (snapshot.status === "playing" || snapshot.status === "paused") {
    status = "playing";
  }

  return {
    attach: player.attach,
    canSurf: snapshot.catalog.length > 1,
    channel,
    currentTitle,
    error,
    liveTransport,
    playbackSessionId: player.playbackSessionId,
    retry: () => void retry(),
    status,
    step: (direction: TuneDirection) => void step(direction),
    stillURL: decodedStill === stillUri ? stillUri : undefined,
    tune: (target: string) => {
      const known = channels?.find((candidate) => candidate.id === target);
      // A channel the app can't play is still a place to go: its Watch page says why it is off.
      if (known && !known.inAppPlayable) onNavigateRef.current(target);
      else void tuneChannel(target);
    },
  };
};

export { useWatchPlayer };
