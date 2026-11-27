import type Hls from "hls.js";
import { playableRungs, selectQualityLevel } from "../../live-level";
import {
  clearTransferredBuffers,
  discardTransferredMedia,
  releaseTransferredDecoder,
  webKitRequiresFreshMSEHandoff,
} from "../../media-element";
import type { TransportHost, TuneFrame } from "../browser-transport.type";
import type { createFirstFrameWatch } from "../first-frame-watch";
import {
  canPlayMime,
  createHlsController,
  levelsOf,
  type ManifestLoadFailure,
  manifestStartFailure,
  onAirOf,
} from "../hls-support";
import type { attachPlaybackDiagnostics } from "../playback-diagnostics";

const STREAM_STOPPED = "The stream stopped. Try again in a moment.";

interface HlsBindOptions {
  attemptId: number;
  current: () => boolean;
  diagnostics: ReturnType<typeof attachPlaybackDiagnostics>;
  el: HTMLVideoElement;
  /** Registers the tune's teardown and resolves when the tune is settled. */
  finish: (teardown: () => void) => Promise<void>;
  frame: TuneFrame;
  HlsController: typeof Hls;
  host: TransportHost;
  manifestArrived: () => void;
  runCleanups: () => void;
  settle: () => void;
  url: string;
  watch: ReturnType<typeof createFirstFrameWatch>;
}

/**
 * Plays `url` through an hls.js controller from the transport's pool, transferring the outgoing
 * channel's MediaSource to it when the browser allows.
 *
 * hls.js FIRST when it is supported — deliberately, and it is the fix for a real bug.
 *
 * ⚠ Chromium returns a truthy `canPlayType("application/vnd.apple.mpegurl")` ("maybe") on some
 * builds but CANNOT actually decode HLS — so a native-first order set `video.src`, the element
 * sat at readyState 0 with a black frame, and no .m3u8 was ever fetched. hls.js (Media Source
 * Extensions) is the correct path on every non-Safari browser, so we take it whenever it is
 * supported and only fall back to native HLS when it is not (that is Safari/iOS, which plays
 * `.m3u8` natively and genuinely). capLevelToPlayerSize stops hls.js fetching a rendition
 * larger than the frame; low-latency off — this is live TV, and the steadier buffer survives a
 * flaky link.
 */
const bindHls = async ({
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
}: HlsBindOptions): Promise<void> => {
  const { active, clock, pool } = host;
  const { diagnosticBase } = diagnostics;
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
  host.resetActive();
  active.hls = hls;
  active.sourceURL = url;
  active.lastKeepaliveMs = Date.now();
  frame.replenish = () => {
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
  watch.joinWhenLoaded(playReplacement);
  const joinTransferredLiveSync = () => {
    if (!transferred || frame.painted || !current()) return;
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
    const level = selectQualityLevel(levels, canPlayMime, host.quality());
    hls.startLevel = level;
    hls.loadLevel = level;
    if (current()) host.onLadder?.({ ...onAirOf(levels, level), rungs: playableRungs(levels, canPlayMime) });
    joinTransferredLiveSync();
    playReplacement();
    manifestArrived();
  };
  const onLevelUpdated = () => joinTransferredLiveSync();
  const onLevelSwitched = (_: unknown, data: { level: number }) => {
    if (!current()) return;
    const levels = levelsOf(hls);
    host.onLadder?.({ ...onAirOf(levels, data.level), rungs: playableRungs(levels, canPlayMime) });
  };
  const onFragmentBuffered = () => {
    watch.arm();
    // attachMedia can queue a target loadstart after it returns. WebKit then resets the
    // element to paused even when the immediate post-attach play() succeeded. Join once as
    // bytes buffer and once more when target loadeddata proves that queued loadstart is past;
    // both belong to this viewer tune, rather than being an autoplay retry loop.
    playReplacement();
  };
  const stopWithError = (message: string, startStatus?: number) => {
    if (current()) {
      host.emit({
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
      host.resetActive();
      if (standby && standby !== hls) standby.destroy();
    }
    hls.destroy();
    settle();
  };
  const onError = (_evt: string, data: { fatal: boolean; type: string } & ManifestLoadFailure) => {
    host.recordDiagnostic({
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
          host.expireToLive(el); // the window moved past the viewer's position
        }
        break;
      case Hls.ErrorTypes.MEDIA_ERROR:
        hls.recoverMediaError();
        break;
      default:
        stopWithError(STREAM_STOPPED);
    }
  };
  watch.listenForLoad();
  hls.on(Hls.Events.MANIFEST_PARSED, onManifestParsed);
  hls.on(Hls.Events.LEVEL_UPDATED, onLevelUpdated);
  hls.on(Hls.Events.LEVEL_SWITCHED, onLevelSwitched);
  hls.on(Hls.Events.FRAG_BUFFERED, onFragmentBuffered);
  hls.on(Hls.Events.ERROR, onError);
  const detachHandlers = () => {
    watch.stop();
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
      host.resetActive();
    }
    diagnostics.stop("retry");
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
  watch.arm();
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
    diagnostics.stop();
    detachHandlers();
    if (pool.instance !== hls) return;
    host.resetActive();
    hls.stopLoad();
  });
};

export { bindHls };
