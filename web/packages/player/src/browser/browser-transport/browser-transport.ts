import type Hls from "hls.js";
import { createLiveClock, type LivePosition } from "../../live-clock";
import type { PlayerTransportEvent } from "../../player-controller";
import type { PlayerSource } from "../../player-source";
import { type LiveQuality, selectQualityLevel } from "../live-level";
import { liveSeek } from "../live-seek";
import {
  afterNextPaint,
  captureDecodedFrame,
  clearHeldFrame,
  containsMediaTime,
  displayedProgramTime,
  holdCapturedFrame,
  webKitRequiresFreshMSEHandoff,
} from "../media-element";
import type {
  ActiveSource,
  BrowserTransport,
  BrowserTransportOptions,
  HlsPool,
  PlaybackFlags,
  TransportHost,
  TuneFrame,
} from "./browser-transport.type";
import { createFirstFrameWatch } from "./first-frame-watch";
import { bindHls } from "./hls-bind";
import { canPlayMime, levelsOf } from "./hls-support";
import { attachPlaybackDiagnostics } from "./playback-diagnostics";

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

/** The oldest media a paused point may be asked for is bounded by the server window; this is the keepalive cadence. */
const KEEPALIVE_INTERVAL_MS = 10_000;
const HEARTBEAT_INTERVAL_MS = 1_000;
const UNSUPPORTED_BROWSER = "This browser can't play live channels.";

let cachedHlsController: typeof Hls | undefined;

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
  const flags: PlaybackFlags = { stalled: false };
  // The media time the viewer paused at, so resuming returns to exactly that frame.
  let pausedMediaTime: number | undefined;
  let teardownBind: (() => void) | undefined;
  let clearHeld: (() => void) | undefined;
  const pool: HlsPool = {};
  const active: ActiveSource = { lastKeepaliveMs: 0 };

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

  const host: TransportHost = {
    active,
    channelId,
    clock,
    emit,
    emitLive,
    expireToLive,
    flags,
    onLadder,
    onPhase,
    playbackSessionId,
    pool,
    position,
    quality: () => quality,
    recordDiagnostic,
    rejoinAfterStall,
    resetActive,
  };

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

    const diagnostics = attachPlaybackDiagnostics({ attemptId, el, hlsController: HlsController, host });
    const { diagnosticBase } = diagnostics;

    // Name the generation on the element before any decoded-frame observer is armed. The tuner
    // certification captures this value when requestVideoFrameCallback is REQUESTED, so a late
    // callback from the outgoing Channel cannot be mistaken for the replacement when both use
    // the same transferred MediaSource blob URL.
    const playbackChannel = channelId();
    if (playbackChannel !== undefined) el.dataset.playbackChannel = playbackChannel;

    const frame: TuneFrame = { painted: false };
    const onFirstFrame = () => {
      if (frame.painted) return;
      frame.painted = true;
      clearHeldFrame(el);
      flags.framedAttemptId = attemptId;
      onPhase?.(attemptId, "first-frame");
      recordDiagnostic({ ...diagnosticBase(), event: "player.ready" });
      emit({ attemptId, type: "first-frame" });
      emitLive();
      frame.replenish?.();
    };
    const watch = createFirstFrameWatch(el, onFirstFrame);
    const manifestArrived = () => {
      onPhase?.(attemptId, "manifest");
      settle();
    };

    if (HlsController?.isSupported()) {
      return bindHls({
        attemptId,
        current,
        diagnostics,
        el,
        finish,
        frame,
        HlsController,
        host,
        manifestArrived,
        runCleanups,
        settle,
        url,
        watch,
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
      watch.listenForLoadedData();
      el.addEventListener("loadedmetadata", manifestArrived, { once: true });
      el.src = url;
      void el.play().catch(() => {
        /* autoplay may be blocked; the play control covers it */
      });
      return finish(() => {
        diagnostics.stop();
        watch.stop();
        el.removeEventListener("loadedmetadata", manifestArrived);
        resetActive();
        el.removeAttribute("src");
        el.load();
      });
    }

    watch.stop();
    recordDiagnostic({
      ...diagnosticBase(),
      event: "player.media_error",
      errorCode: "unsupported",
      fatal: true,
    });
    diagnostics.stop("retry");
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
      // Read the outgoing picture before the teardown below: native HLS (WebKit) cleanup empties
      // the element (readyState 0, no frame), so a capture after it has nothing to draw (#2691).
      const outgoingFrame = video ? captureDecodedFrame(video) : undefined;
      releaseBind();
      if (clearHeld) video?.removeEventListener("loadeddata", clearHeld);
      const el = await attachedVideo(signal);
      if (!el || !current()) return;
      // The tuning overlay paints first. Holding the outgoing frame (a canvas encode) and binding
      // the new source block the main thread, and the viewer's acknowledgement must not wait on them.
      await afterNextPaint();
      if (!current()) return;
      activeAttemptId = attemptId;
      flags.framedAttemptId = undefined;
      flags.stalled = false;
      pausedMediaTime = undefined;
      // The outgoing channel's rungs are not this one's: empty until its manifest is read.
      onLadder?.({ rungs: [] });
      clock.begin(source.serverTimeMs);
      // The picture the viewer was watching holds under the tuning overlay until this source has
      // frame data of its own: `loadeddata` itself means the replacement has a current frame.
      if (el === video) holdCapturedFrame(el, outgoingFrame);
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

export { createBrowserTransport };
