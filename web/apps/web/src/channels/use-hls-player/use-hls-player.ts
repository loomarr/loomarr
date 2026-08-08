import { channelsApi, toProblem, unwrap } from "@loomarr/api";
import Hls from "hls.js";
import { useCallback, useRef, useState } from "react";

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

// qualityHint derives the play-url `quality` cap from the device — NOT to pick the rendition (the
// client does that live), but to keep the server from OFFERING renditions this device cannot use.
// Save-Data and a slow effective type say "small screen or metered link, cap low"; a small
// viewport says the same. Absent any signal we send nothing (auto = full ladder). Coarse on
// purpose: a starting hint the client refines from.
const qualityHint = (): "auto" | "720" | "480" => {
  const conn = (navigator as Navigator & { connection?: { saveData?: boolean; effectiveType?: string } })
    .connection;
  if (conn?.saveData) return "480";
  if (conn?.effectiveType === "2g" || conn?.effectiveType === "slow-2g") return "480";
  if (conn?.effectiveType === "3g") return "720";

  const px = Math.max(window.screen.width, window.screen.height) * (window.devicePixelRatio || 1);
  if (px <= 640) return "480";
  if (px <= 1280) return "720";
  return "auto";
};

function useHlsPlayer(channelId: string): UseHlsPlayer {
  const [status, setStatus] = useState<PlayerStatus>("idle");
  const [error, setError] = useState<string | undefined>();
  // Kept so the async mint result can be ignored if the element unmounted first (React 18 strict
  // mode double-invokes effects, and a channel switch can unmount mid-mint).
  const cancelledRef = useRef(false);

  const bind = useCallback((video: HTMLVideoElement, url: string): (() => void) => {
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
      });
      hls.loadSource(url);
      hls.attachMedia(video);
      hls.on(Hls.Events.MANIFEST_PARSED, () => {
        void video.play().catch(() => {
          /* autoplay policy — the control handles it */
        });
        setStatus("playing");
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
            setStatus("error");
            setError("The stream stopped. Try again in a moment.");
            hls.destroy();
        }
      });
      return () => hls.destroy();
    }

    // NATIVE HLS (Safari/iOS): hls.js is unsupported here BECAUSE the platform plays `.m3u8`
    // itself, doing its own ABR. Only reached when MSE is absent, so this is a genuine native-HLS
    // browser, not a Chromium false-positive.
    if (video.canPlayType("application/vnd.apple.mpegurl")) {
      video.src = url;
      void video.play().catch(() => {
        /* autoplay may be blocked; the play control covers it */
      });
      setStatus("playing");
      return () => {
        video.removeAttribute("src");
        video.load();
      };
    }

    setStatus("error");
    setError("This browser can't play live channels.");
    return () => undefined;
  }, []);

  const attach = useCallback(
    (video: HTMLVideoElement): (() => void) => {
      cancelledRef.current = false;
      setStatus("loading");
      setError(undefined);

      // The mint is the STANDALONE client function, not the useChannelPlayUrl() mutation hook, and
      // that choice is load-bearing: the mutation object gets a fresh identity every render, which
      // made `attach` unstable, which made VideoPlayer's `[attach]` effect re-fire on every render
      // and setState in a loop ("Maximum update depth exceeded"). The plain function has no such
      // identity, so `attach` depends only on the stable `channelId` and `bind`.
      let teardown: (() => void) | undefined;
      channelsApi
        .channelPlayUrl(channelId, { quality: qualityHint() })
        .then((res) => {
          if (cancelledRef.current) return;
          const body = unwrap(res);
          // Prefer the RELATIVE same-origin URL: the browser is already on Loomarr's origin, so
          // this needs no CORS (hls.js fetches by XHR) and works even when server.public_url is
          // unset. The absolute `url` is the native-app form and only a fallback here.
          const src = body?.relativeUrl || body?.url;
          if (!src) {
            setStatus("error");
            setError("Couldn't get a stream for this channel.");
            return;
          }
          teardown = bind(video, src);
        })
        .catch((e) => {
          if (cancelledRef.current) return;
          setStatus("error");
          setError(toProblem(e).detail ?? toProblem(e).title ?? "Couldn't start this channel.");
        });

      return () => {
        cancelledRef.current = true;
        teardown?.();
      };
    },
    [channelId, bind],
  );

  return { status, error, attach };
}

export type { PlayerStatus, UseHlsPlayer };
export { useHlsPlayer };
