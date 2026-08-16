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

const createHlsController = (HlsController: typeof Hls): Hls =>
  new HlsController({
    // Manifest parsing is safe before attachment on a fresh source-scoped controller, but fragment
    // loading is not. The handoff below performs the one explicit start only after attach.
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
  const [state, setState] = useState<{ channelId: string; status: PlayerStatus; error?: string }>({
    channelId,
    status: "idle",
  });
  // A channel change is loading immediately, before VideoPlayer's passive attach effect runs. The
  // previous source's "playing" state must never hide the new attempt's tuning presentation.
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
  const warmedPlayURL = attempt?.playURL;

  const bind = useCallback(
    async (video: HTMLVideoElement, url: string, current: () => boolean): Promise<() => void> => {
      // hls.js is almost half a megabyte minified. Loading it with the Watch route made the route's
      // controls and programme context wait for a transport library they do not need to render.
      // Fetch it only after the signed URL arrives; the page paints first, then playback attaches.
      // The generation check matters because a channel switch can happen while this chunk is in
      // flight — an obsolete import must never attach its stream to the replacement video.
      const { default: Hls } = await import("hls.js");
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
        markTunePhase(attempt, "first-frame");
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
      let joinReplacementOnLoadedData: (() => void) | undefined;
      const onLoadedData = () => {
        armFirstFrameWatch();
        const joinReplacement = joinReplacementOnLoadedData;
        joinReplacementOnLoadedData = undefined;
        joinReplacement?.();
      };
      const stopFirstFrameWatch = () => {
        joinReplacementOnLoadedData = undefined;
        if (frameCallback !== undefined) video.cancelVideoFrameCallback?.(frameCallback);
        video.removeEventListener("loadeddata", onLoadedData);
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
      if (Hls.isSupported()) {
        const previous = hlsRef.current.instance;
        let transferred = previous?.url && previous.media === video ? previous.transferMedia() : null;
        previous?.stopLoad();
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
        joinReplacementOnLoadedData = playReplacement;
        const onManifestParsed = () => {
          manifestParsed = true;
          markTunePhase(attempt, "manifest");
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
        if (requestFrame) video.addEventListener("loadeddata", onLoadedData, { once: true });
        else armFirstFrameWatch();
        hls.on(Hls.Events.MANIFEST_PARSED, onManifestParsed);
        hls.on(Hls.Events.FRAG_BUFFERED, onFragmentBuffered);
        hls.on(Hls.Events.ERROR, onError);
        hls.loadSource(url);

        // Preserve the reusable MediaSource AND its compatible SourceBuffers, but never their
        // outgoing Channel-relative bytes. transferMedia() is hls.js's public cross-controller
        // handoff; a controller is source-scoped, while the expensive MSE/decoder allocation lives
        // across tunes. Prepared publications leave MediaSource in `ended`, which remains reusable:
        // SourceBuffer.remove/append transitions it back to `open`. Only a closed source (or failed
        // clear) needs hls.js's full MSE reset below.
        if (transferred?.mediaSource && transferred.mediaSource.readyState !== "closed") {
          try {
            await clearTransferredBuffers(transferred);
          } catch {
            // A failed range removal cannot be reused safely. Attaching the element directly gives
            // the replacement controller a fresh MediaSource; the transferred source loses its
            // final reference when the element's blob URL is replaced.
            transferred = null;
          }
          // Do not seek into the range being removed. WebKit can keep SourceBuffer.remove pending
          // while its decoder still owns the current position, turning an otherwise cached tune
          // into a multi-second stall. Rewind only after updateend releases the outgoing bytes.
          video.currentTime = 0;
        } else {
          transferred = null;
        }
        if (!current()) {
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
        if (transferred) hls.attachMedia(transferred);
        else hls.attachMedia(video);
        // Attachment is the first point where a frame callback can only belong to this source:
        // transferred buffers are empty and a fresh attach has replaced the old MediaSource. Arm
        // before startLoad so a fast cached append cannot present its first frame before the
        // observer exists and leave certification (and the tuning overlay) waiting for a later one.
        armFirstFrameWatch();
        replacementAttached = true;
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
      if (video.canPlayType("application/vnd.apple.mpegurl")) {
        const onManifest = () => markTunePhase(attempt, "manifest");
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
    [attempt, channelId],
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
