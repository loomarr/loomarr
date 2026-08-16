import { toProblem } from "@loomarr/api/mutator";
import type Hls from "hls.js";
import { useCallback, useRef, useState } from "react";
import { mintChannelPlaySource } from "../channel-play-url";
import { markTunePhase, type TuneAttempt } from "../tuner-timing";

// useHlsPlayer — binds a channel's live ABR HLS to a <video> element (§9.1 Watch, V46).
//
// It returns an `attach(video)` the VideoPlayer primitive drives: the primitive owns the element
// and its accessible controls; this hook owns the two things that make a live CHANNEL different
// from a file:
//
//  1. THE TRANSPORT SPLIT. Safari/iOS play `.m3u8` natively (set video.src and go); every other
//     browser needs hls.js over Media Source Extensions. We pick the native path when the browser
//     advertises it (canPlayType) and fall back to hls.js otherwise — the same master playlist a
//     native app would hand to AVPlayer/ExoPlayer, which is why the transport is HLS.
//  2. THE SIGNED URL. The stream authenticates by a short-lived signed URL, not the device token
//     (§11) — so the source comes from POST /v1/channels/{id}/play-url, minted per session. The
//     device token never touches the browser.
//
// Rendition SELECTION stays with the client: hls.js measures real throughput and switches per
// segment, and capLevelToPlayerSize keeps it from fetching a rendition larger than the frame. The
// server only decides which renditions to OFFER, hinted by the device signals below.

type PlayerStatus = "idle" | "loading" | "playing" | "error";

interface UseHlsPlayer {
  status: PlayerStatus;
  error?: string;
  /**
   * Bind live playback to a <video>. Pass to VideoPlayer's `attach` prop. Mints a signed URL,
   * attaches via native HLS or hls.js, and returns a cleanup that tears both down.
   */
  attach: (video: HTMLVideoElement) => () => void;
}

let cachedHlsController: typeof Hls | undefined;

const clearTransferredBuffers = async (transferred: NonNullable<ReturnType<Hls["transferMedia"]>>) => {
  for (const track of Object.values(transferred.tracks)) {
    const buffer = track?.buffer;
    if (!buffer) continue;
    if (buffer.updating) buffer.abort();
    if (buffer.buffered.length === 0) continue;
    const start = buffer.buffered.start(0);
    const end = buffer.buffered.end(buffer.buffered.length - 1);
    await new Promise<void>((resolve, reject) => {
      const done = () => {
        buffer.removeEventListener("updateend", complete);
        buffer.removeEventListener("error", failed);
      };
      const complete = () => {
        done();
        resolve();
      };
      const failed = () => {
        done();
        reject(new Error("source buffer clear failed"));
      };
      buffer.addEventListener("updateend", complete, { once: true });
      buffer.addEventListener("error", failed, { once: true });
      buffer.remove(start, end);
    });
  }
};

const transferredBufferedRange = (
  transferred: NonNullable<ReturnType<Hls["transferMedia"]>>,
): { start: number; end: number } | undefined => {
  let start = Number.NEGATIVE_INFINITY;
  let end = Number.POSITIVE_INFINITY;
  let found = false;
  for (const track of Object.values(transferred.tracks)) {
    const buffered = track?.buffer?.buffered;
    if (!buffered?.length) continue;
    const last = buffered.length - 1;
    start = Math.max(start, buffered.start(last));
    end = Math.min(end, buffered.end(last));
    found = true;
  }
  return found && start < end ? { start, end } : undefined;
};

const releaseTransferredDecoder = async (
  video: HTMLVideoElement,
  transferred: NonNullable<ReturnType<Hls["transferMedia"]>>,
) => {
  const range = transferredBufferedRange(transferred);
  if (!range || video.currentTime + 0.05 >= range.end) return;

  // TimeRanges.end() is a half-open boundary, not a presentable timestamp. WebKit can leave a seek
  // to that exact value pending until live playback naturally reaches it. Park at the final
  // presentable-frame interval instead, and cap the acknowledgement: a platform that cannot release
  // this already-buffered seek promptly takes the bounded fresh-MSE fallback rather than delaying
  // the viewer by the outgoing fragment remainder.
  const target = Math.max(range.start, range.end - 0.05);
  await new Promise<void>((resolve, reject) => {
    let complete = false;
    const finish = (error?: Error) => {
      if (complete) return;
      complete = true;
      window.clearTimeout(timer);
      video.removeEventListener("seeked", onSeeked);
      if (error) reject(error);
      else resolve();
    };
    const onSeeked = () => finish();
    const timer = window.setTimeout(() => {
      if (video.currentTime + 0.05 >= target) finish();
      else finish(new Error("decoder release seek timed out"));
    }, 100);
    video.addEventListener("seeked", onSeeked, { once: true });
    video.currentTime = target;
  });
};

const webKitRequiresFreshMSEHandoff = (): boolean => {
  if (typeof navigator === "undefined") return false;
  const agent = navigator.userAgent;
  return /AppleWebKit/i.test(agent) && !/(?:Chromium|Chrome|CriOS|Edg)/i.test(agent);
};

const discardTransferredMedia = (
  video: HTMLVideoElement,
  objectURL: string | undefined,
  resetBeforeReplacement = true,
) => {
  // transferMedia deliberately leaves the MediaSource object URL on the element so another hls.js
  // controller can adopt it. The fresh-MSE branch does not adopt that transfer. Replacing src
  // without first revoking the abandoned URL leaves WebKit retaining each ended MediaSource and its
  // decoder lease; over repeated tunes, opening the next source then drifts into multi-second waits.
  // Detach it explicitly while the held poster owns continuity, then let the fresh controller bind.
  let detached = false;
  if (objectURL && (video.src === objectURL || video.currentSrc === objectURL)) {
    video.removeAttribute("src");
    detached = true;
  }
  for (const source of video.querySelectorAll("source")) {
    if (objectURL && source.src !== objectURL) continue;
    source.remove();
    detached = true;
  }
  // WebKit may synchronously spend more than a second processing an empty load. Skip that
  // intermediate state when the fresh controller below immediately replaces the source; assigning
  // its MediaSource performs the required element load. A superseded generation still resets here
  // because no replacement follows to release the detached decoder.
  if (detached && resetBeforeReplacement) video.load();
  if (objectURL?.startsWith("blob:")) URL.revokeObjectURL(objectURL);
};

const createHlsController = (HlsController: typeof Hls): Hls =>
  new HlsController({
    // A source-scoped controller stays empty until its transferred MediaSource is attached. The
    // handoff below then loads the source and performs one explicit media start.
    autoStartLoad: false,
    capLevelToPlayerSize: true,
    // Baseline HLS is MPEG-TS. Keep its transmux off the UI thread; hls.js shares and reference-
    // counts this worker across the bounded source-scoped controller pair.
    enableWorker: true,
    // Live channel: keep chasing the live edge, and be patient while it warms up. A channel takes a
    // few seconds to produce its first segment (the encoder spins up), during which the playlist
    // may briefly have no media — hls.js must RETRY, not give up.
    liveDurationInfinity: true,
    manifestLoadingMaxRetry: 8,
    manifestLoadingRetryDelay: 1000,
    levelLoadingMaxRetry: 8,
    fragLoadingMaxRetry: 8,
    // ⚠ Start ~TWO segments from the live edge — the balance between fast first-paint and a
    // survivable buffer (both measured in-browser). hls.js's default is 3 (~12s at our 4s
    // segments); 1 sits at the live edge and leaves transcodes no cushion against a realtime dip.
    liveSyncDurationCount: 2,
    // Keep corrective live-edge seeks well above the sync target so a slow transcode can drift and
    // let its buffer absorb a dip rather than causing another visible stall.
    liveMaxLatencyDurationCount: 10,
    // Build a forward cushion after fast start; keep enough back buffer for a short viewer rewind.
    maxBufferLength: 60,
    backBufferLength: 30,
  });

function useHlsPlayer(channelId: string, attempt?: TuneAttempt): UseHlsPlayer {
  // TanStack commits the route parameter after the tuner has already activated its target. That
  // commit can drop the transient attempt object while the Channel itself is unchanged. Retain the
  // attempt for that Channel so VideoPlayer does not tear down and reattach the same live source
  // immediately after its first frame; the next Channel replaces it atomically.
  const retainedAttemptRef = useRef<{ channelId: string; attempt?: TuneAttempt }>({ channelId, attempt });
  if (retainedAttemptRef.current.channelId !== channelId) {
    retainedAttemptRef.current = { channelId, attempt };
  } else if (attempt) {
    retainedAttemptRef.current.attempt = attempt;
  }
  const playbackAttempt = retainedAttemptRef.current.attempt;
  const [state, setState] = useState<{ channelId: string; status: PlayerStatus; error?: string }>({
    channelId,
    status: "idle",
  });
  // A channel change is loading immediately, before VideoPlayer's source-binding layout effect
  // runs. The previous source's "playing" state must never hide the new attempt's tuning presentation.
  const status = state.channelId === channelId ? state.status : "loading";
  const error = state.channelId === channelId ? state.error : undefined;
  // Each attachment gets a generation in addition to its AbortController. Abort stops the fetch;
  // generation guards the small race where a promise has already resolved and queued its callback.
  // A boolean cannot do this: the next attach resets it to false and accidentally re-authorizes an
  // older request that resolves late.
  const generationRef = useRef(0);
  // A Channel route-param change keeps this hook and its <video> mounted. The active controller
  // transfers its MediaSource and compatible SourceBuffers to a fresh, unused standby. Controllers
  // are source-scoped: once the replacement paints, the detached old active is destroyed and a new
  // standby is constructed off the critical path. Only two are live; unmount destroys both.
  const hlsRef = useRef<{
    instance?: Hls;
    standby?: Hls;
    standbyFresh?: boolean;
    destroyTimer?: number;
  }>({});
  const warmedPlayURL = playbackAttempt?.playURL;

  const bind = useCallback(
    async (video: HTMLVideoElement, url: string, current: () => boolean): Promise<() => void> => {
      // hls.js is almost half a megabyte minified. Loading it with the Watch route made the route's
      // controls and programme context wait for a transport library they do not need to render.
      // Fetch it only after the signed URL arrives; the page paints first, then playback attaches.
      // The generation check matters because a channel switch can happen while this chunk is in
      // flight — an obsolete import must never attach its stream to the replacement video.
      // WebKit can defer even an already-resolved dynamic import behind media work for hundreds of
      // milliseconds. Resolve the transport class once per document; every later tune takes the
      // synchronous cached value while the bounded controller pool still owns runtime resources.
      // Safari-family browsers have a native HLS pipeline that avoids the repeated MediaSource
      // replacement hls.js needs. Prefer it before importing the half-megabyte controller. The UA
      // guard is load-bearing: some Chromium builds answer "maybe" for the MIME type without being
      // able to play HLS, and must continue through hls.js below.
      const nativeHLS =
        webKitRequiresFreshMSEHandoff() && Boolean(video.canPlayType("application/vnd.apple.mpegurl"));
      const HlsController = nativeHLS ? undefined : (cachedHlsController ?? (await import("hls.js")).default);
      if (HlsController) cachedHlsController = HlsController;
      if (!current()) return () => undefined;

      // Name the generation on the element before any decoded-frame observer is armed. The tuner
      // certification captures this value when requestVideoFrameCallback is REQUESTED, so a late
      // callback from the outgoing Channel cannot be mistaken for the replacement when both use
      // the same transferred MediaSource blob URL.
      video.dataset.playbackChannel = channelId;

      let replenishAfterFirstFrame: (() => void) | undefined;
      let firstFrame = false;
      const onFirstFrame = () => {
        if (firstFrame) return;
        firstFrame = true;
        if (video.poster.startsWith("data:image/png;base64,")) video.removeAttribute("poster");
        markTunePhase(playbackAttempt, "first-frame");
        setState({ channelId, status: "playing" });
        replenishAfterFirstFrame?.();
      };
      let frameCallback: number | undefined;
      let firstFrameWatchArmed = false;
      const requestFrame = (
        video as Partial<Pick<HTMLVideoElement, "requestVideoFrameCallback">>
      ).requestVideoFrameCallback?.bind(video);
      const armFirstFrameWatch = () => {
        if (firstFrameWatchArmed) return;
        firstFrameWatchArmed = true;
        if (requestFrame) frameCallback = requestFrame(() => onFirstFrame());
        else video.addEventListener("playing", onFirstFrame, { once: true });
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
        if (frameCallback !== undefined) video.cancelVideoFrameCallback?.(frameCallback);
        video.removeEventListener("loadedmetadata", onLoadedMetadata);
        video.removeEventListener("loadeddata", onLoadedData);
        video.removeEventListener("canplay", onCanPlay);
        video.removeEventListener("playing", onFirstFrame);
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
        const previous = hlsRef.current.instance;
        let transferred: ReturnType<Hls["transferMedia"]> = null;
        let transferredObjectURL: string | undefined;
        if (previous?.url && previous.media === video) {
          previous.stopLoad();
          // Freeze the decoder before removing its current SourceBuffer range. WebKit otherwise
          // holds the still-playing outgoing bytes until their natural end, adding roughly the
          // remaining segment duration to an otherwise cached adjacent tune. VideoPlayer keeps the
          // last decoded frame as the handoff poster until the replacement produces its own frame.
          // WebKit's fresh-MSE branch detaches the outgoing source immediately below. Avoid
          // explicitly changing the element to paused first: preserving its active playback intent
          // lets the replacement join as soon as its first bytes are ready.
          if (!discardTransferForWebKit) video.pause();
          transferredObjectURL = video.currentSrc || video.src;
          transferred = previous.transferMedia();
        } else {
          previous?.stopLoad();
        }
        // Each hls.js controller owns one source URL for its lifetime. A normal tune consumes the
        // already-constructed fresh standby; a superseding burst can arrive before replenishment,
        // in which case retire the older detached controller and create the required source owner.
        const parked = hlsRef.current.standby;
        const hls = parked && hlsRef.current.standbyFresh ? parked : createHlsController(Hls);
        if (parked && parked !== hls && parked !== previous) parked.destroy();
        hlsRef.current.instance = hls;
        hlsRef.current.standby = previous;
        hlsRef.current.standbyFresh = false;
        replenishAfterFirstFrame = () => {
          if (!current() || hlsRef.current.instance !== hls) return;
          const retired = hlsRef.current.standby;
          if (retired && retired !== hls) retired.destroy();
          hlsRef.current.standby = createHlsController(Hls);
          hlsRef.current.standbyFresh = true;
        };
        let manifestParsed = false;
        let replacementAttached = false;
        const playReplacement = () => {
          if (!replacementAttached) return;
          void video.play().catch(() => {
            /* autoplay policy — the control handles it */
          });
        };
        joinReplacementOnLoadedMetadata = playReplacement;
        joinReplacementOnLoadedData = playReplacement;
        joinReplacementOnCanPlay = playReplacement;
        const onManifestParsed = () => {
          manifestParsed = true;
          markTunePhase(playbackAttempt, "manifest");
          playReplacement();
        };
        const onFragmentBuffered = () => {
          armFirstFrameWatch();
          // attachMedia can queue a target loadstart after it returns. WebKit then resets the
          // element to paused even when the immediate post-attach play() succeeded. Join once as
          // bytes buffer and once more when target loadeddata proves that queued loadstart is past;
          // both belong to this viewer tune, rather than being an autoplay retry loop.
          playReplacement();
        };
        const onError = (_evt: string, data: { fatal: boolean; type: string }) => {
          if (!data.fatal) return; // non-fatal: hls.js recovers on its own
          // ⚠ A fatal error during a LIVE stream is usually RECOVERABLE, not terminal — the channel
          // is warming up (empty playlist for a beat) or a segment hiccuped. hls.js has built-in
          // recovery for exactly this, so attempt it before declaring the stream dead: reload the
          // network pipeline for network errors, recover the media buffer for media errors. Only a
          // genuinely unrecoverable error (or one that keeps recurring) surfaces to the viewer.
          switch (data.type) {
            case Hls.ErrorTypes.NETWORK_ERROR:
              hls.startLoad(); // re-fetch the manifest/segments — the fix for warmup 404s/empties
              break;
            case Hls.ErrorTypes.MEDIA_ERROR:
              hls.recoverMediaError();
              break;
            default:
              setState({ channelId, status: "error", error: "The stream stopped. Try again in a moment." });
              if (hlsRef.current.instance === hls) {
                const standby = hlsRef.current.standby;
                hlsRef.current.instance = undefined;
                hlsRef.current.standby = undefined;
                hlsRef.current.standbyFresh = false;
                if (standby && standby !== hls) standby.destroy();
              }
              hls.destroy();
          }
        };
        if (requestFrame) {
          video.addEventListener("loadedmetadata", onLoadedMetadata, { once: true });
          video.addEventListener("loadeddata", onLoadedData, { once: true });
          video.addEventListener("canplay", onCanPlay, { once: true });
        } else armFirstFrameWatch();
        hls.on(Hls.Events.MANIFEST_PARSED, onManifestParsed);
        hls.on(Hls.Events.FRAG_BUFFERED, onFragmentBuffered);
        hls.on(Hls.Events.ERROR, onError);

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
              await releaseTransferredDecoder(video, transferred);
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
          if (discardTransfer) discardTransferredMedia(video, transferredObjectURL);
          stopFirstFrameWatch();
          hls.off(Hls.Events.MANIFEST_PARSED, onManifestParsed);
          hls.off(Hls.Events.FRAG_BUFFERED, onFragmentBuffered);
          hls.off(Hls.Events.ERROR, onError);
          hls.stopLoad();
          // A newer bind has already swapped this controller into standby; keep the bounded pool.
          // If it is still active, the Watch surface was left while clear was pending, so no future
          // bind can own either controller and both must be released here.
          if (hlsRef.current.instance === hls) {
            const standby = hlsRef.current.standby;
            hls.destroy();
            if (standby && standby !== hls) standby.destroy();
            hlsRef.current.instance = undefined;
            hlsRef.current.standby = undefined;
            hlsRef.current.standbyFresh = false;
          }
          return () => undefined;
        }
        let sourceLoaded = false;
        if (discardTransfer) {
          discardTransferredMedia(video, transferredObjectURL, !discardTransferForWebKit);
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
        if (transferred) video.currentTime = 0;
        if (transferred) hls.attachMedia(transferred);
        else hls.attachMedia(video);
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
        return () => {
          stopFirstFrameWatch();
          hls.off(Hls.Events.MANIFEST_PARSED, onManifestParsed);
          hls.off(Hls.Events.FRAG_BUFFERED, onFragmentBuffered);
          hls.off(Hls.Events.ERROR, onError);
          if (hlsRef.current.instance !== hls) return;
          hls.stopLoad();
          hlsRef.current.destroyTimer = window.setTimeout(() => {
            if (hlsRef.current.instance !== hls) return;
            const standby = hlsRef.current.standby;
            hls.destroy();
            if (standby && standby !== hls) standby.destroy();
            hlsRef.current.instance = undefined;
            hlsRef.current.standby = undefined;
            hlsRef.current.standbyFresh = false;
            hlsRef.current.destroyTimer = undefined;
          }, 0);
        };
      }

      // NATIVE HLS (Safari/iOS): hls.js is unsupported here BECAUSE the platform plays `.m3u8`
      // itself, doing its own ABR. Only reached when MSE is absent, so this is a genuine native-HLS
      // browser, not a Chromium false-positive.
      if (nativeHLS || video.canPlayType("application/vnd.apple.mpegurl")) {
        const onManifest = () => markTunePhase(playbackAttempt, "manifest");
        // Native HLS has no transport controller to detach the previous URL for us.
        video.removeAttribute("src");
        video.load();
        if (requestFrame) video.addEventListener("loadeddata", onLoadedData, { once: true });
        else armFirstFrameWatch();
        video.addEventListener("loadedmetadata", onManifest, { once: true });
        video.src = url;
        void video.play().catch(() => {
          /* autoplay may be blocked; the play control covers it */
        });
        return () => {
          stopFirstFrameWatch();
          video.removeEventListener("loadedmetadata", onManifest);
          video.removeAttribute("src");
          video.load();
        };
      }

      stopFirstFrameWatch();
      setState({ channelId, status: "error", error: "This browser can't play live channels." });
      return () => undefined;
    },
    [channelId, playbackAttempt],
  );

  const attach = useCallback(
    (video: HTMLVideoElement): (() => void) => {
      if (hlsRef.current.destroyTimer !== undefined) {
        window.clearTimeout(hlsRef.current.destroyTimer);
        hlsRef.current.destroyTimer = undefined;
      }
      const generation = ++generationRef.current;
      const controller = new AbortController();
      const current = () => generationRef.current === generation && !controller.signal.aborted;
      setState({ channelId, status: "loading" });

      // The mint is the STANDALONE client function, not the useChannelPlayUrl() mutation hook, and
      // that choice is load-bearing: the mutation object gets a fresh identity every render, which
      // made `attach` unstable, which made VideoPlayer's `[attach]` effect re-fire on every render
      // and setState in a loop ("Maximum update depth exceeded"). The plain function has no such
      // identity, so `attach` depends only on the stable `channelId` and `bind`.
      let teardown: (() => void) | undefined;
      // Reuse an adjacent warmer's signed URL when present. Otherwise mint normally. Both paths
      // arrive at the same transport attachment; warming never creates a second player.
      const source = warmedPlayURL
        ? Promise.resolve({ url: warmedPlayURL, expiresAt: Number.POSITIVE_INFINITY })
        : mintChannelPlaySource(channelId, controller.signal);
      source
        .then((body) => {
          if (!current()) return;
          // mintChannelPlaySource already prefers the relative same-origin form, which avoids
          // CORS and works when server.public_url is unset.
          const src = body?.url;
          if (!src) {
            setState({ channelId, status: "error", error: "Couldn't get a stream for this channel." });
            return;
          }
          return bind(video, src, current).then((nextTeardown) => {
            if (!current()) {
              nextTeardown();
              return;
            }
            teardown = nextTeardown;
          });
        })
        .catch((e) => {
          if (!current()) return;
          setState({
            channelId,
            status: "error",
            error: toProblem(e).detail ?? toProblem(e).title ?? "Couldn't start this channel.",
          });
        });

      return () => {
        controller.abort();
        if (generationRef.current === generation) generationRef.current++;
        teardown?.();
      };
    },
    [channelId, bind, warmedPlayURL],
  );

  return { status, error, attach };
}

export type { PlayerStatus, UseHlsPlayer };
export { useHlsPlayer };
