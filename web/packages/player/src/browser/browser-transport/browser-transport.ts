import type { ClientObservation } from "@loomarr/core/client-diagnostics";
import type Hls from "hls.js";
import { createLiveClock, type LivePosition } from "../../live-clock";
import type { PlayerTransport, PlayerTransportEvent } from "../../player-controller";
import type { PlayerSource } from "../../player-source";
import { parseStartFailureReason } from "../../start-failure";
import { liveHlsConfig } from "../live-hls-config";
import {
  type LiveLadder,
  type LiveLevel,
  type LiveQuality,
  playableRungs,
  selectQualityLevel,
} from "../live-level";
import { liveSeek } from "../live-seek";
import {
  afterNextPaint,
  clearHeldFrame,
  clearTransferredBuffers,
  containsMediaTime,
  type DisplayedClockAnchor,
  discardTransferredMedia,
  displayedProgramTime,
  holdDecodedFrame,
  releaseTransferredDecoder,
  webKitRequiresFreshMSEHandoff,
} from "../media-element";

// The browser's PlayerTransport: binds a channel's live ABR HLS to the page's one <video> element
// under the shared player controller, as the native transports bind it to expo-video.
//
// The controller owns what a tune IS (intent, minting, retries, the Reconnecting and failed states);
// this owns the two things that make a live CHANNEL different from a file in a browser:
//
//  1. THE TRANSPORT SPLIT. Safari/iOS play `.m3u8` natively (set video.src and go); every other
//     browser needs hls.js over Media Source Extensions. We pick hls.js whenever it is supported and
//     fall back to native HLS otherwise.
//  2. THE LIVE CLOCK. Where the viewer is relative to the channel's wall clock comes from the shared
//     live clock; this reads the displayed frame's programme time off the element and acts on the
//     clock's answers (seek to the live edge, resume a paused point).
//
// Rendition SELECTION stays with the client: hls.js measures real throughput and switches per
// segment, and capLevelToPlayerSize keeps it from fetching a rendition larger than the frame. The
// server only decides which renditions to OFFER, hinted by the quality the play-url mint asks for.

type BrowserTunePhase = "first-frame" | "manifest";

interface BrowserTransportOptions {
  /**
   * The channel being tuned, for diagnostics. Read when an observation is recorded, which is after the
   * controller has published the attempt's snapshot.
   */
  channelId: () => string | undefined;
  /** The tuned channel's ladder changed: read from its manifest, a level switch, or a new tune. */
  onLadder?: (ladder: LiveLadder) => void;
  /** An attempt reached a phase of its tune, for timing. */
  onPhase?: (attemptId: number, phase: BrowserTunePhase) => void;
  recordDiagnostic: (observation: ClientObservation) => void;
}

interface BrowserTransport extends PlayerTransport {
  /**
   * Binds the transport to the page's <video>. A replacement requested before the element exists
   * waits for it. The returned release detaches the element and frees every hls.js controller.
   */
  attach: (video: HTMLVideoElement) => () => void;
  /** Names this player's session in client diagnostics. */
  playbackSessionId: string;
  /**
   * The viewer's quality (#2088 decision 7): Auto, or a rung by height, played when the master
   * offers it. A change switches the playing level at once and never re-binds the source.
   */
  setQuality: (quality: LiveQuality) => void;
}

/** The oldest media a paused point may be asked for is bounded by the server window; this is the keepalive cadence. */
const KEEPALIVE_INTERVAL_MS = 10_000;
const HEARTBEAT_INTERVAL_MS = 1_000;
const UNSUPPORTED_BROWSER = "This browser can't play live channels.";
const STREAM_STOPPED = "The stream stopped. Try again in a moment.";

let cachedHlsController: typeof Hls | undefined;

interface ManifestLoadFailure {
  details?: string;
  response?: { code?: number; data?: unknown };
}

const createHlsController = (HlsController: typeof Hls): Hls => new HlsController({ ...liveHlsConfig });

// What MediaSource can decode: the gate on the premium rung.
const canPlayMime = (mime: string) => typeof MediaSource !== "undefined" && MediaSource.isTypeSupported(mime);

const levelsOf = (hls: Hls): LiveLevel[] =>
  hls.levels.map((l) => ({ codecs: l.attrs.CODECS, height: l.height, videoRange: l.videoRange }));

/**
 * The reason a channel's playlist request was refused, when the server decided it (a 5xx problem
 * response). A 404 or an empty playlist is the ordinary warm-up race and stays on hls.js's own
 * retry; see `parseStartFailureReason` for how the problem body itself is read.
 */
const manifestStartFailure = (
  data: ManifestLoadFailure,
  manifestLoadError: string,
): { reason: string; status: number } | undefined => {
  const code = data.response?.code;
  if (data.details !== manifestLoadError || code === undefined) return undefined;
  const reason = parseStartFailureReason(code, data.response?.data);
  return reason === undefined ? undefined : { reason, status: code };
};

const createPlaybackSessionID = () => {
  const bytes = new Uint8Array(16);
  globalThis.crypto.getRandomValues(bytes);
  return `web_${Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("")}`;
};

const createBrowserTransport = ({
  channelId,
  onLadder,
  onPhase,
  recordDiagnostic,
}: BrowserTransportOptions): BrowserTransport => {
  const playbackSessionId = createPlaybackSessionID();
  // The viewer's quality, read by each tune's manifest.
  let quality: LiveQuality = "auto";
  const clock = createLiveClock();
  const listeners = new Set<(event: PlayerTransportEvent) => void>();
  const videoWaiters = new Set<() => void>();
  let disposed = false;
  let video: HTMLVideoElement | undefined;
  // Each replacement gets a generation in addition to its AbortSignal. Abort stops the fetch;
  // generation guards the small race where a promise has already resolved and queued its callback.
  let generation = 0;
  let activeAttemptId: number | undefined;
  let framedAttemptId: number | undefined;
  let stalled = false;
  // The media time the viewer paused at, so resuming returns to exactly that frame.
  let pausedMediaTime: number | undefined;
  let teardownBind: (() => void) | undefined;
  let clearHeld: (() => void) | undefined;
  // A channel change keeps this transport and its <video> mounted. The active controller transfers
  // its MediaSource and compatible SourceBuffers to a fresh, unused standby. Controllers are
  // source-scoped: once the replacement paints, the detached old active is destroyed and a new
  // standby is constructed off the critical path. Only two are live; release destroys both.
  const pool: { instance?: Hls; standby?: Hls; standbyFresh?: boolean } = {};
  const active: { anchor?: DisplayedClockAnchor; hls?: Hls; lastKeepaliveMs: number; sourceURL?: string } = {
    lastKeepaliveMs: 0,
  };

  const emit = (event: PlayerTransportEvent) => {
    if (disposed) return;
    for (const listener of listeners) listener(event);
  };

  /** What the element says about the frame on screen. */
  const position = (): LivePosition => {
    const sample = displayedProgramTime(video, active.hls, active.anchor);
    active.anchor = sample.anchor;
    return { programTimeMs: sample.programTimeMs };
  };

  const emitLive = () => {
    if (activeAttemptId === undefined) return;
    emit({ attemptId: activeAttemptId, state: clock.state(position()), type: "live-state" });
  };

  const resetActive = () => {
    active.anchor = undefined;
    active.hls = undefined;
    active.lastKeepaliveMs = 0;
    active.sourceURL = undefined;
  };

  const destroyPool = () => {
    const { instance, standby } = pool;
    pool.instance = undefined;
    pool.standby = undefined;
    pool.standbyFresh = false;
    instance?.destroy();
    if (standby && standby !== instance) standby.destroy();
    resetActive();
  };

  /** Plays on from the live edge: the buffer is reloaded from the edge and the element joins it. */
  const seekToLiveEdge = (el: HTMLVideoElement) => {
    pausedMediaTime = undefined;
    active.hls?.startLoad(-1);
    const liveEdge = active.hls?.liveSyncPosition;
    if (liveEdge !== null && liveEdge !== undefined && Number.isFinite(liveEdge)) {
      el.currentTime = liveEdge;
    } else if (el.seekable.length > 0) {
      el.currentTime = el.seekable.end(el.seekable.length - 1);
    }
  };

  /** The position the viewer held has left the server's window: live again, with a notice. */
  const expireToLive = (el: HTMLVideoElement) => {
    clock.expire();
    seekToLiveEdge(el);
    emitLive();
  };

  /** A stall that left a live viewer too far behind rejoins the live edge once the position is known. */
  const rejoinAfterStall = () => {
    if (!video || !clock.shouldRejoin(position())) return;
    clock.goLive();
    seekToLiveEdge(video);
    emitLive();
  };

  /** Back to the live edge, playing: Go live, and a seek that reaches the edge. */
  const goLive = async () => {
    if (!video) return;
    clock.goLive();
    seekToLiveEdge(video);
    emitLive();
    await video.play().catch(() => undefined);
  };

  const releaseBind = () => {
    const teardown = teardownBind;
    teardownBind = undefined;
    teardown?.();
  };

  const attachedVideo = (signal: AbortSignal): Promise<HTMLVideoElement | undefined> =>
    new Promise((resolve) => {
      if (video) {
        resolve(video);
        return;
      }
      const settle = () => {
        videoWaiters.delete(settle);
        signal.removeEventListener("abort", settle);
        resolve(video);
      };
      videoWaiters.add(settle);
      signal.addEventListener("abort", settle, { once: true });
    });

  /**
   * Opens `url` on the element and resolves once this tune's first manifest has arrived, the tune
   * failed for good, or it was superseded or aborted. Playback errors reach the controller as events.
   */
  const bind = async (
    el: HTMLVideoElement,
    url: string,
    attemptId: number,
    current: () => boolean,
    signal: AbortSignal,
  ): Promise<void> => {
    // hls.js is almost half a megabyte minified; fetch it only when a source arrives, so the page
    // paints first and playback attaches after. WebKit can defer even an already-resolved dynamic
    // import behind media work for hundreds of milliseconds, so the class is resolved once per
    // document and every later tune takes the synchronous cached value. Safari-family browsers have
    // a native HLS pipeline that avoids the repeated MediaSource replacement hls.js needs; prefer it
    // before importing the controller. The UA guard is load-bearing: some Chromium builds answer
    // "maybe" for the MIME type without being able to play HLS, and must continue through hls.js.
    const nativeHLS =
      webKitRequiresFreshMSEHandoff() && Boolean(el.canPlayType("application/vnd.apple.mpegurl"));
    const HlsController = nativeHLS ? undefined : (cachedHlsController ?? (await import("hls.js")).default);
    if (HlsController) cachedHlsController = HlsController;
    if (!current()) return;

    let settle!: () => void;
    const settled = new Promise<void>((resolve) => {
      settle = resolve;
    });
    signal.addEventListener("abort", settle, { once: true });
    const cleanups: Array<() => void> = [() => signal.removeEventListener("abort", settle)];
    const runCleanups = () => {
      for (const cleanup of cleanups.splice(0).reverse()) cleanup();
    };
    const finish = (teardown: () => void): Promise<void> => {
      cleanups.push(teardown);
      teardownBind = () => {
        runCleanups();
        settle();
      };
      return settled;
    };

    const diagnosticBase = () =>
      ({
        playbackSessionId,
        channelId: channelId() ?? "",
        transport: HlsController?.isSupported() ? "hls_js" : "native_hls",
      }) as const;
    const bufferedMs = () => {
      if (el.buffered.length === 0) return 0;
      return Math.max(0, Math.round((el.buffered.end(el.buffered.length - 1) - el.currentTime) * 1_000));
    };
    const reportPosition = (
      event: "player.buffering_started" | "player.buffering_ended" | "player.seeking" | "player.seeked",
    ) =>
      recordDiagnostic({
        ...diagnosticBase(),
        event,
        viewerTimeMs: Math.round(clock.state(position()).viewerTimeMs),
        ...(event.startsWith("player.buffering") ? { bufferedMs: bufferedMs() } : {}),
      });
    const framed = () => framedAttemptId === attemptId;
    const onWaiting = () => {
      reportPosition("player.buffering_started");
      // Buffering before the first frame is the start itself; one the viewer paused for is not a stall.
      if (!framed() || stalled || clock.mode() === "paused") return;
      stalled = true;
      clock.beginStall();
      emit({ attemptId, type: "stalled" });
    };
    const onPlaying = () => {
      reportPosition("player.buffering_ended");
      if (stalled) {
        stalled = false;
        clock.endStall("resumed");
      }
      // Until the first decoded frame the controller keeps its tuning picture: `first-frame` is the
      // start's own signal, so a `playing` that beats it is not announced.
      if (!framed()) return;
      rejoinAfterStall();
      emit({ attemptId, type: "playing" });
      emitLive();
    };
    const onNativeError = () =>
      recordDiagnostic({
        ...diagnosticBase(),
        event: "player.media_error",
        errorCode: `media_${el.error?.code ?? 0}`,
        fatal: true,
      });
    el.addEventListener("waiting", onWaiting);
    el.addEventListener("playing", onPlaying);
    el.addEventListener("error", onNativeError);
    recordDiagnostic({ ...diagnosticBase(), event: "player.attached", reason: "mount" });
    const stopDiagnostics = (reason: "unmount" | "retry" = "unmount") => {
      el.removeEventListener("waiting", onWaiting);
      el.removeEventListener("playing", onPlaying);
      el.removeEventListener("error", onNativeError);
      recordDiagnostic({ ...diagnosticBase(), event: "player.detached", reason });
    };

    // Name the generation on the element before any decoded-frame observer is armed. The tuner
    // certification captures this value when requestVideoFrameCallback is REQUESTED, so a late
    // callback from the outgoing Channel cannot be mistaken for the replacement when both use
    // the same transferred MediaSource blob URL.
    const playbackChannel = channelId();
    if (playbackChannel !== undefined) el.dataset.playbackChannel = playbackChannel;

    let replenishAfterFirstFrame: (() => void) | undefined;
    let firstFrame = false;
    const onFirstFrame = () => {
      if (firstFrame) return;
      firstFrame = true;
      clearHeldFrame(el);
      framedAttemptId = attemptId;
      onPhase?.(attemptId, "first-frame");
      recordDiagnostic({ ...diagnosticBase(), event: "player.ready" });
      emit({ attemptId, type: "first-frame" });
      emitLive();
      replenishAfterFirstFrame?.();
    };
    let frameCallback: number | undefined;
    let firstFrameWatchArmed = false;
    const requestFrame = (
      el as Partial<Pick<HTMLVideoElement, "requestVideoFrameCallback">>
    ).requestVideoFrameCallback?.bind(el);
    const armFirstFrameWatch = () => {
      if (firstFrameWatchArmed) return;
      firstFrameWatchArmed = true;
      if (requestFrame) frameCallback = requestFrame(() => onFirstFrame());
      else el.addEventListener("playing", onFirstFrame, { once: true });
    };
    let joinReplacementOnLoadedMetadata: (() => void) | undefined;
    let joinReplacementOnLoadedData: (() => void) | undefined;
    let joinReplacementOnCanPlay: (() => void) | undefined;
    const onLoadedMetadata = () => {
      const joinReplacement = joinReplacementOnLoadedMetadata;
      joinReplacementOnLoadedMetadata = undefined;
      joinReplacement?.();
    };
    const onLoadedData = () => {
      armFirstFrameWatch();
      const joinReplacement = joinReplacementOnLoadedData;
      joinReplacementOnLoadedData = undefined;
      joinReplacement?.();
    };
    const onCanPlay = () => {
      const joinReplacement = joinReplacementOnCanPlay;
      joinReplacementOnCanPlay = undefined;
      joinReplacement?.();
    };
    const stopFirstFrameWatch = () => {
      joinReplacementOnLoadedMetadata = undefined;
      joinReplacementOnLoadedData = undefined;
      joinReplacementOnCanPlay = undefined;
      if (frameCallback !== undefined) el.cancelVideoFrameCallback?.(frameCallback);
      el.removeEventListener("loadedmetadata", onLoadedMetadata);
      el.removeEventListener("loadeddata", onLoadedData);
      el.removeEventListener("canplay", onCanPlay);
      el.removeEventListener("playing", onFirstFrame);
    };
    const manifestArrived = () => {
      onPhase?.(attemptId, "manifest");
      settle();
    };

    // hls.js FIRST when it is supported — deliberately, and it is the fix for a real bug.
    //
    // ⚠ Chromium returns a truthy `canPlayType("application/vnd.apple.mpegurl")` ("maybe") on some
    // builds but CANNOT actually decode HLS — so a native-first order set `video.src`, the element
    // sat at readyState 0 with a black frame, and no .m3u8 was ever fetched. hls.js (Media Source
    // Extensions) is the correct path on every non-Safari browser, so we take it whenever it is
    // supported and only fall back to native HLS when it is not (that is Safari/iOS, which plays
    // `.m3u8` natively and genuinely). capLevelToPlayerSize stops hls.js fetching a rendition
    // larger than the frame; low-latency off — this is live TV, and the steadier buffer survives a
    // flaky link.
    if (HlsController?.isSupported()) {
      const Hls = HlsController;
      const discardTransferForWebKit = webKitRequiresFreshMSEHandoff();
      const previous = pool.instance;
      let transferred: ReturnType<Hls["transferMedia"]> = null;
      let transferredObjectURL: string | undefined;
      if (previous?.url && previous.media === el) {
        previous.stopLoad();
        // Freeze the decoder before removing its current SourceBuffer range. WebKit otherwise
        // holds the still-playing outgoing bytes until their natural end, adding roughly the
        // remaining segment duration to an otherwise cached adjacent tune. The held poster keeps the
        // last decoded frame until the replacement produces its own frame. WebKit's fresh-MSE branch
        // detaches the outgoing source immediately below. Avoid explicitly changing the element to
        // paused first: preserving its active playback intent lets the replacement join as soon as
        // its first bytes are ready.
        if (!discardTransferForWebKit) el.pause();
        transferredObjectURL = el.currentSrc || el.src;
        transferred = previous.transferMedia();
      } else {
        previous?.stopLoad();
      }
      // Each hls.js controller owns one source URL for its lifetime. A normal tune consumes the
      // already-constructed fresh standby; a superseding burst can arrive before replenishment,
      // in which case retire the older detached controller and create the required source owner.
      const parked = pool.standby;
      const hls = parked && pool.standbyFresh ? parked : createHlsController(Hls);
      if (parked && parked !== hls && parked !== previous) parked.destroy();
      pool.instance = hls;
      pool.standby = previous;
      pool.standbyFresh = false;
      resetActive();
      active.hls = hls;
      active.sourceURL = url;
      active.lastKeepaliveMs = Date.now();
      replenishAfterFirstFrame = () => {
        if (!current() || pool.instance !== hls) return;
        const retired = pool.standby;
        if (retired && retired !== hls) retired.destroy();
        pool.standby = createHlsController(Hls);
        pool.standbyFresh = true;
      };
      let manifestParsed = false;
      let replacementAttached = false;
      const playReplacement = () => {
        // Fragment-buffered continues throughout a live session. It may finish after the viewer
        // deliberately pauses, so only the initial/live join is allowed to drive autoplay.
        if (!replacementAttached || clock.mode() === "paused") return;
        void el.play().catch(() => {
          /* autoplay policy — the control handles it */
        });
      };
      joinReplacementOnLoadedMetadata = playReplacement;
      joinReplacementOnLoadedData = playReplacement;
      joinReplacementOnCanPlay = playReplacement;
      const joinTransferredLiveSync = () => {
        if (!transferred || firstFrame || !current()) return;
        const liveSyncPosition = hls.liveSyncPosition;
        if (
          liveSyncPosition === null ||
          !Number.isFinite(liveSyncPosition) ||
          Math.abs(el.currentTime - liveSyncPosition) <= 0.05
        ) {
          return;
        }
        // Clearing a transferred MediaSource temporarily rewinds the persistent element. hls.js
        // computes the replacement's live start from its level details, but Firefox does not
        // always apply that implicit jump before the first append. Join explicitly so cached
        // media cannot wait for the next playlist refresh.
        el.currentTime = liveSyncPosition;
      };
      const onManifestParsed = () => {
        manifestParsed = true;
        // One level for the whole tune (#1512 G10): the viewer's rung when they chose one and it is
        // offered here; otherwise the premium only when this browser decodes its exact CODECS, else
        // the baseline. Manual, so ABR and player-size capping never move it onto a codec it cannot
        // play or a 4K encode it did not choose.
        const levels = levelsOf(hls);
        const level = selectQualityLevel(levels, canPlayMime, quality);
        hls.startLevel = level;
        hls.loadLevel = level;
        if (current())
          onLadder?.({ height: levels[level]?.height, rungs: playableRungs(levels, canPlayMime) });
        joinTransferredLiveSync();
        playReplacement();
        manifestArrived();
      };
      const onLevelUpdated = () => joinTransferredLiveSync();
      const onLevelSwitched = (_: unknown, data: { level: number }) => {
        if (!current()) return;
        const levels = levelsOf(hls);
        onLadder?.({ height: levels[data.level]?.height, rungs: playableRungs(levels, canPlayMime) });
      };
      const onFragmentBuffered = () => {
        armFirstFrameWatch();
        // attachMedia can queue a target loadstart after it returns. WebKit then resets the
        // element to paused even when the immediate post-attach play() succeeded. Join once as
        // bytes buffer and once more when target loadeddata proves that queued loadstart is past;
        // both belong to this viewer tune, rather than being an autoplay retry loop.
        playReplacement();
      };
      const stopWithError = (message: string, startStatus?: number) => {
        if (current()) {
          emit({
            attemptId,
            error: message,
            ...(startStatus === undefined ? {} : { startStatus }),
            type: "error",
          });
        }
        if (pool.instance === hls) {
          const standby = pool.standby;
          pool.instance = undefined;
          pool.standby = undefined;
          pool.standbyFresh = false;
          resetActive();
          if (standby && standby !== hls) standby.destroy();
        }
        hls.destroy();
        settle();
      };
      const onError = (_evt: string, data: { fatal: boolean; type: string } & ManifestLoadFailure) => {
        recordDiagnostic({
          ...diagnosticBase(),
          event: "player.media_error",
          errorCode:
            data.type === Hls.ErrorTypes.NETWORK_ERROR
              ? "hls_network"
              : data.type === Hls.ErrorTypes.MEDIA_ERROR
                ? "hls_media"
                : "hls_other",
          fatal: data.fatal,
        });
        if (!data.fatal) return; // non-fatal: hls.js recovers on its own
        // ⚠ A fatal error during a LIVE stream is usually RECOVERABLE, not terminal — the channel
        // is warming up (empty playlist for a beat) or a segment hiccuped. hls.js has built-in
        // recovery for exactly this, so attempt it before declaring the stream dead: reload the
        // network pipeline for network errors, recover the media buffer for media errors. Only a
        // genuinely unrecoverable error (or one that keeps recurring) surfaces to the viewer.
        const refused =
          data.type === Hls.ErrorTypes.NETWORK_ERROR
            ? manifestStartFailure(data, Hls.ErrorDetails.MANIFEST_LOAD_ERROR)
            : undefined;
        if (refused) {
          stopWithError(refused.reason, refused.status);
          return;
        }
        switch (data.type) {
          case Hls.ErrorTypes.NETWORK_ERROR:
            if (clock.mode() === "live") {
              hls.startLoad(); // re-fetch the manifest/segments — the fix for warmup 404s/empties
            } else {
              expireToLive(el); // the window moved past the viewer's position
            }
            break;
          case Hls.ErrorTypes.MEDIA_ERROR:
            hls.recoverMediaError();
            break;
          default:
            stopWithError(STREAM_STOPPED);
        }
      };
      if (requestFrame) {
        el.addEventListener("loadedmetadata", onLoadedMetadata, { once: true });
        el.addEventListener("loadeddata", onLoadedData, { once: true });
        el.addEventListener("canplay", onCanPlay, { once: true });
      } else armFirstFrameWatch();
      hls.on(Hls.Events.MANIFEST_PARSED, onManifestParsed);
      hls.on(Hls.Events.LEVEL_UPDATED, onLevelUpdated);
      hls.on(Hls.Events.LEVEL_SWITCHED, onLevelSwitched);
      hls.on(Hls.Events.FRAG_BUFFERED, onFragmentBuffered);
      hls.on(Hls.Events.ERROR, onError);
      const detachHandlers = () => {
        stopFirstFrameWatch();
        hls.off(Hls.Events.MANIFEST_PARSED, onManifestParsed);
        hls.off(Hls.Events.LEVEL_UPDATED, onLevelUpdated);
        hls.off(Hls.Events.LEVEL_SWITCHED, onLevelSwitched);
        hls.off(Hls.Events.FRAG_BUFFERED, onFragmentBuffered);
        hls.off(Hls.Events.ERROR, onError);
      };

      // Preserve an OPEN MediaSource and compatible SourceBuffers, but never outgoing
      // Channel-relative bytes. Decoder release seeks to the last presentable frame rather than
      // the half-open end and is time-bounded; ended and closed sources, a seek timeout, and
      // failed clears consume the standby's bounded fresh-MSE branch.
      let discardTransfer = false;
      if (transferred?.mediaSource?.readyState === "open") {
        // Hosted and shipping WebKit can block its event loop while seeking an open live source,
        // so a JavaScript timer cannot honestly bound that transfer path. Consume the already-
        // constructed fresh standby instead. Chromium and Firefox keep the lower-allocation
        // transfer path, whose in-range seek remains explicitly time-bounded.
        if (discardTransferForWebKit) {
          discardTransfer = true;
          transferred = null;
        } else {
          try {
            await releaseTransferredDecoder(el, transferred);
            await clearTransferredBuffers(transferred);
          } catch {
            // A decoder seek or range removal that cannot finish promptly cannot be reused safely.
            // Attaching the element directly gives the replacement controller a fresh MediaSource.
            discardTransfer = true;
            transferred = null;
          }
        }
      } else {
        discardTransfer = Boolean(transferred);
        transferred = null;
      }
      if (!current()) {
        if (discardTransfer) discardTransferredMedia(el, transferredObjectURL);
        detachHandlers();
        hls.stopLoad();
        // A newer bind has already swapped this controller into standby; keep the bounded pool.
        // If it is still active, the transport was released while the clear was pending, so no
        // future bind can own either controller and both must be released here.
        if (pool.instance === hls) {
          const standby = pool.standby;
          hls.destroy();
          if (standby && standby !== hls) standby.destroy();
          pool.instance = undefined;
          pool.standby = undefined;
          pool.standbyFresh = false;
          resetActive();
        }
        stopDiagnostics("retry");
        runCleanups();
        return;
      }
      let sourceLoaded = false;
      if (discardTransfer) {
        discardTransferredMedia(el, transferredObjectURL, !discardTransferForWebKit);
        // A fresh controller has no SourceBuffers to adopt, so manifest parsing can overlap its
        // MediaSource attachment safely. autoStartLoad remains false: init/media bytes still wait
        // for attachment and the generation-scoped start below.
        hls.loadSource(url);
        sourceLoaded = true;
      }
      // Do not rewind into the range being removed. WebKit can keep SourceBuffer.remove pending
      // while its decoder still owns that position, turning an otherwise cached tune into a
      // multi-second stall. Rewind only after updateend releases the outgoing bytes, and only
      // while this generation still owns the element.
      if (transferred) el.currentTime = 0;
      if (transferred) hls.attachMedia(transferred);
      else hls.attachMedia(el);
      // Attachment is the first point where a frame callback can only belong to this source:
      // transferred buffers are empty and a fresh attach has replaced the old MediaSource. Arm
      // before startLoad so a fast cached append cannot present its first frame before the
      // observer exists and leave certification (and the tuning overlay) waiting for a later one.
      armFirstFrameWatch();
      replacementAttached = true;
      // hls.js transfer is an attach-before-source transaction. Parsing a source on a detached
      // controller can fetch its init segment before the transferred SourceBuffers are adopted;
      // WebKit can then strand that controller without ever requesting the media fragment. The
      // fresh branch above has no transferred buffers and deliberately overlaps manifest parse.
      if (!sourceLoaded) hls.loadSource(url);
      // Queue the target join before any media bytes can arrive. WebKit can decode a cached first
      // append before MANIFEST_PARSED is delivered; waiting for that event leaves a real target
      // frame paused. Later event joins remain necessary because loadstart can reset the element.
      playReplacement();
      hls.startLoad();
      if (manifestParsed) playReplacement();
      return finish(() => {
        stopDiagnostics();
        detachHandlers();
        if (pool.instance !== hls) return;
        resetActive();
        hls.stopLoad();
      });
    }

    // NATIVE HLS (Safari/iOS): hls.js is unsupported here BECAUSE the platform plays `.m3u8`
    // itself, doing its own ABR. Only reached when MSE is absent, so this is a genuine native-HLS
    // browser, not a Chromium false-positive.
    if (nativeHLS || el.canPlayType("application/vnd.apple.mpegurl")) {
      resetActive();
      active.sourceURL = url;
      active.lastKeepaliveMs = Date.now();
      // Native HLS has no transport controller to detach the previous URL for us.
      el.removeAttribute("src");
      el.load();
      if (requestFrame) el.addEventListener("loadeddata", onLoadedData, { once: true });
      else armFirstFrameWatch();
      el.addEventListener("loadedmetadata", manifestArrived, { once: true });
      el.src = url;
      void el.play().catch(() => {
        /* autoplay may be blocked; the play control covers it */
      });
      return finish(() => {
        stopDiagnostics();
        stopFirstFrameWatch();
        el.removeEventListener("loadedmetadata", manifestArrived);
        resetActive();
        el.removeAttribute("src");
        el.load();
      });
    }

    stopFirstFrameWatch();
    recordDiagnostic({
      ...diagnosticBase(),
      event: "player.media_error",
      errorCode: "unsupported",
      fatal: true,
    });
    stopDiagnostics("retry");
    runCleanups();
    throw new Error(UNSUPPORTED_BROWSER);
  };

  return {
    attach: (element) => {
      video = element;
      for (const waiter of [...videoWaiters]) waiter();
      // Keep programme context on the frame the decoder advances to. The one-second heartbeat is the
      // stall/lag clock; timeupdate makes ordinary playback boundary-accurate instead of letting the
      // chrome lead by as much as a polling interval.
      const onTimeUpdate = () => {
        rejoinAfterStall();
        emitLive();
      };
      element.addEventListener("timeupdate", onTimeUpdate);
      const heartbeat = window.setInterval(() => {
        emitLive();
        // Native HLS implementations may stop playlist polling while the element is paused. A cheap
        // manifest read keeps the one shared server remux leased; it never creates a viewer encoder.
        const at = Date.now();
        if (clock.mode() !== "paused" || !active.sourceURL) return;
        if (at - active.lastKeepaliveMs < KEEPALIVE_INTERVAL_MS) return;
        active.lastKeepaliveMs = at;
        void fetch(active.sourceURL, { cache: "no-store" })
          .then((response) => response.arrayBuffer())
          .catch(() => undefined);
      }, HEARTBEAT_INTERVAL_MS);
      return () => {
        window.clearInterval(heartbeat);
        element.removeEventListener("timeupdate", onTimeUpdate);
        if (video !== element) return;
        generation += 1;
        releaseBind();
        destroyPool();
        video = undefined;
        for (const waiter of [...videoWaiters]) waiter();
      };
    },
    dispose: () => {
      if (disposed) return;
      disposed = true;
      generation += 1;
      releaseBind();
      destroyPool();
      listeners.clear();
      for (const waiter of [...videoWaiters]) waiter();
    },
    goLive,
    pause: () => {
      if (!video) return;
      clock.pause(position());
      pausedMediaTime = video.currentTime;
      video.pause();
      if (activeAttemptId !== undefined) emit({ attemptId: activeAttemptId, type: "paused" });
      emitLive();
    },
    play: async () => {
      const el = video;
      if (!el) return;
      if (clock.mode() !== "paused") {
        await el.play().catch(() => undefined);
        return;
      }
      // A paused point stays paused until the viewer resumes. Then it either still plays from
      // exactly that frame, or the server's window has moved past it and the viewer is back live.
      const point = pausedMediaTime;
      const pointAvailable = point !== undefined && containsMediaTime(el.seekable, point);
      if (clock.resume({ pointAvailable }) === "expired") {
        seekToLiveEdge(el);
      } else if (point !== undefined) {
        active.hls?.startLoad(point);
        el.currentTime = point;
        pausedMediaTime = undefined;
      }
      emitLive();
      await el.play().catch(() => undefined);
    },
    playbackSessionId,
    // A seek keeps the viewer's play state: paused stays paused at the new point, playing plays on
    // from it. A target at the live edge is Go live. The frame on screen maps the wall-clock target
    // to media time (liveSeek), clamped to what the element still holds.
    seek: async (wallClockMs) => {
      const el = video;
      if (!el || activeAttemptId === undefined) return;
      const displayedWallMs = clock.state(position()).viewerTimeMs;
      const last = el.seekable.length - 1;
      const liveSyncPosition = active.hls?.liveSyncPosition;
      const seek = liveSeek({
        currentTime: el.currentTime,
        displayedWallMs,
        liveEdge:
          liveSyncPosition !== null && liveSyncPosition !== undefined && Number.isFinite(liveSyncPosition)
            ? liveSyncPosition
            : last >= 0
              ? el.seekable.end(last)
              : undefined,
        nowMs: clock.now(),
        seekable: last >= 0 ? { end: el.seekable.end(last), start: el.seekable.start(0) } : undefined,
        targetWallMs: wallClockMs,
      });
      if (seek.kind === "live") {
        await goLive();
        return;
      }
      active.hls?.startLoad(seek.mediaTime);
      el.currentTime = seek.mediaTime;
      if (clock.mode() === "paused") pausedMediaTime = seek.mediaTime;
      clock.seekTo(seek.wallMs);
      emitLive();
    },
    // A deliberate change, so a brief rebuffer is fair: move the playing controller to the rung now,
    // or back to Auto's choice. A later tune reads `quality` on its manifest.
    setQuality: (next) => {
      quality = next;
      const hls = active.hls;
      if (!hls || hls.levels.length === 0) return;
      const level = selectQualityLevel(levelsOf(hls), canPlayMime, quality);
      if (level !== hls.currentLevel) hls.currentLevel = level;
    },
    replace: async (source: PlayerSource, { attemptId, signal }) => {
      const mine = ++generation;
      const current = () => !disposed && generation === mine && !signal.aborted;
      releaseBind();
      if (clearHeld) video?.removeEventListener("loadeddata", clearHeld);
      const el = await attachedVideo(signal);
      if (!el || !current()) return;
      // The tuning overlay paints first. Holding the outgoing frame (a canvas encode) and binding
      // the new source block the main thread, and the viewer's acknowledgement must not wait on them.
      await afterNextPaint();
      if (!current()) return;
      activeAttemptId = attemptId;
      framedAttemptId = undefined;
      stalled = false;
      pausedMediaTime = undefined;
      // The outgoing channel's rungs are not this one's: empty until its manifest is read.
      onLadder?.({ rungs: [] });
      clock.begin(source.serverTimeMs);
      // The picture the viewer was watching holds under the tuning overlay until this source has
      // frame data of its own: `loadeddata` itself means the replacement has a current frame.
      holdDecodedFrame(el);
      const release = () => clearHeldFrame(el);
      clearHeld = release;
      el.addEventListener("loadeddata", release, { once: true });
      emitLive();
      await bind(el, source.uri, attemptId, current, signal);
    },
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
};

export type { BrowserTransport, BrowserTransportOptions, BrowserTunePhase };
export { createBrowserTransport };
