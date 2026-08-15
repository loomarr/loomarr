import { toProblem } from "@loomarr/api/mutator";
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

      let firstFrame = false;
      const onFirstFrame = () => {
        if (firstFrame) return;
        firstFrame = true;
        markTunePhase(attempt, "first-frame");
        setState({ channelId, status: "playing" });
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
      const onLoadedData = () => armFirstFrameWatch();
      // Detaching MediaSource can leave its last decoded picture retained. Clear that source before
      // watching the replacement: WebKit otherwise reports the retained picture through a newly
      // registered callback and attributes it to the next Channel. The poster captured by the tuner
      // continues to hold the outgoing picture while the element itself becomes source-empty.
      video.removeAttribute("src");
      video.load();
      if (requestFrame) video.addEventListener("loadeddata", onLoadedData, { once: true });
      else armFirstFrameWatch();
      const stopFirstFrameWatch = () => {
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
        const hls = new Hls({
          capLevelToPlayerSize: true,
          enableWorker: true,
          // Live channel: keep chasing the live edge, and be patient while it warms up. A channel
          // takes a few seconds to produce its first segment (the encoder spins up), during which
          // the playlist may briefly have no media — hls.js must RETRY, not give up.
          liveDurationInfinity: true,
          manifestLoadingMaxRetry: 8,
          manifestLoadingRetryDelay: 1000,
          levelLoadingMaxRetry: 8,
          fragLoadingMaxRetry: 8,
          // ⚠ **Start ~TWO segments from the live edge — the balance between fast first-paint and a
          // survivable buffer (both measured in-browser).** hls.js's default liveSyncDurationCount is 3
          // (~12s at our 4s segments) — the black window V47 first fixed by dropping it to 1. But 1
          // sits the playhead AT the live edge, and on a TRANSCODING channel (HEVC→h264 at ~1.2×
          // realtime, "little margin" per the doctor) the encoder never pulls ahead, so headroom stays
          // pinned at 0s and any dip below realtime plays out as a BLACK FRAME with nothing buffered to
          // cover it (measured: a transcoding channel held 0s headroom for 30s and fired a `waiting`
          // event). Two segments keeps first-paint fast (one extra ~4s segment vs V47) while giving a
          // one-segment cushion against those dips. A direct-play (`-c copy`) channel fills far past
          // this instantly, so it costs the fast path nothing.
          liveSyncDurationCount: 2,
          // liveMaxLatencyDurationCount governs when hls.js gives up chasing and hard-seeks to the edge
          // (which itself looks like a stall). Keep it well above the sync target so a slow transcode is
          // allowed to drift and be absorbed by the buffer instead of triggering a corrective seek.
          liveMaxLatencyDurationCount: 10,
          // Generous forward buffer + a back buffer so the player keeps BUILDING a cushion after the
          // fast start (start near the edge, buffer up behind the scenes — what Emby/Jellyfin do). The
          // back buffer also lets a viewer nudge back a few seconds without a re-fetch.
          maxBufferLength: 60,
          backBufferLength: 30,
        });
        hls.loadSource(url);
        hls.attachMedia(video);
        hls.on(Hls.Events.MANIFEST_PARSED, () => {
          markTunePhase(attempt, "manifest");
          void video.play().catch(() => {
            /* autoplay policy — the control handles it */
          });
        });
        hls.on(Hls.Events.ERROR, (_evt, data) => {
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
              hls.destroy();
          }
        });
        return () => {
          stopFirstFrameWatch();
          hls.destroy();
        };
      }

      // NATIVE HLS (Safari/iOS): hls.js is unsupported here BECAUSE the platform plays `.m3u8`
      // itself, doing its own ABR. Only reached when MSE is absent, so this is a genuine native-HLS
      // browser, not a Chromium false-positive.
      if (video.canPlayType("application/vnd.apple.mpegurl")) {
        const onManifest = () => markTunePhase(attempt, "manifest");
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
