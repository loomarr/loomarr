import type Hls from "hls.js";

// What the browser transport needs to know about the <video> element and its MediaSource, kept out of
// the transport itself: reading the displayed frame's programme time, holding the outgoing frame while
// a source is replaced, and handing a MediaSource from one hls.js controller to the next.

type NativeDatedMedia = HTMLVideoElement & { getStartDate?: () => Date };

type TransferredMedia = NonNullable<ReturnType<Hls["transferMedia"]>>;

interface DisplayedClockAnchor {
  mediaTimeSeconds: number;
  wallClockMs: number;
}

interface DisplayedClockSample {
  anchor?: DisplayedClockAnchor;
  /** Programme-date-time of the frame on screen, absent while the stream's metadata warms. */
  programTimeMs?: number;
}

/**
 * The programme time of the frame on screen, not the wall-clock edge the Channel is producing.
 * hls.js maps currentTime through EXT-X-PROGRAM-DATE-TIME directly. Safari's native HLS surface
 * exposes the same mapping as the Date at media time zero, so add the element's currentTime there.
 * When the date is briefly lost, the last known mapping keeps the position on the decoded frame.
 */
const displayedProgramTime = (
  video: HTMLVideoElement | undefined,
  hls: Hls | undefined,
  previous?: DisplayedClockAnchor,
): DisplayedClockSample => {
  const mediaTime = video?.currentTime;
  const hasMediaTime = typeof mediaTime === "number" && Number.isFinite(mediaTime);
  const playingMs = hls?.playingDate?.getTime();
  if (playingMs !== undefined && Number.isFinite(playingMs)) {
    return {
      anchor: hasMediaTime ? { mediaTimeSeconds: mediaTime, wallClockMs: playingMs } : previous,
      programTimeMs: playingMs,
    };
  }

  const startMs = (video as NativeDatedMedia | undefined)?.getStartDate?.().getTime();
  if (startMs !== undefined && Number.isFinite(startMs) && hasMediaTime) {
    const wallClockMs = startMs + mediaTime * 1_000;
    return { anchor: { mediaTimeSeconds: mediaTime, wallClockMs }, programTimeMs: wallClockMs };
  }
  if (previous && hasMediaTime) {
    return {
      anchor: previous,
      programTimeMs: previous.wallClockMs + (mediaTime - previous.mediaTimeSeconds) * 1_000,
    };
  }
  return { anchor: previous };
};

const containsMediaTime = (ranges: TimeRanges, point: number): boolean => {
  // An empty TimeRanges means the native/MSE pipeline has not published its window yet. Treat it as
  // unknown rather than expired; the server is authoritative and may still serve the paused point.
  if (ranges.length === 0) return true;
  for (let index = 0; index < ranges.length; index++) {
    if (point >= ranges.start(index) - 0.1 && point <= ranges.end(index) + 0.1) return true;
  }
  return false;
};

// Resolves once the frame after this one has painted, in a task of its own so animation-frame
// callbacks queued for that frame run first. A slow device can take tens of milliseconds a frame,
// so the timer that ends the wait is only for a hidden document, which stops delivering frames.
const afterNextPaint = () =>
  new Promise<void>((resolve) => {
    const fallback = setTimeout(resolve, 500);
    if (typeof requestAnimationFrame !== "function") return;
    requestAnimationFrame(() =>
      requestAnimationFrame(() => {
        clearTimeout(fallback);
        setTimeout(resolve, 0);
      }),
    );
  });

// Hold the outgoing decoded picture inside the same <video> while a source is replaced. This is a
// poster, not a hidden player or decoder: transport teardown can release immediately, while the
// viewer keeps the last honest frame until the replacement produces its first one.
//
// Capture and hold are two steps because the frame must be read BEFORE the outgoing transport is
// torn down (native HLS teardown empties the element: readyState 0, no frame), while the PNG encode
// is the expensive part and waits for the tuning overlay's paint.
const captureDecodedFrame = (video: HTMLVideoElement): HTMLCanvasElement | undefined => {
  if (
    video.readyState < HTMLMediaElement.HAVE_CURRENT_DATA ||
    video.videoWidth < 1 ||
    video.videoHeight < 1
  ) {
    return undefined;
  }
  const canvas = document.createElement("canvas");
  canvas.width = video.videoWidth;
  canvas.height = video.videoHeight;
  const context = canvas.getContext("2d");
  if (!context) return undefined;
  try {
    context.drawImage(video, 0, 0, canvas.width, canvas.height);
    return canvas;
  } catch {
    // A cross-origin source without canvas permission cannot be captured. Leave the element's
    // existing poster alone; source replacement still proceeds normally.
    return undefined;
  }
};

const holdCapturedFrame = (video: HTMLVideoElement, frame: HTMLCanvasElement | undefined) => {
  if (!frame) return;
  try {
    video.poster = frame.toDataURL("image/png");
  } catch {
    // Same as capture: a tainted canvas leaves the poster alone.
  }
};

const clearHeldFrame = (video: HTMLVideoElement) => {
  if (video.poster.startsWith("data:image/png;base64,")) video.removeAttribute("poster");
};

const clearTransferredBuffers = async (transferred: TransferredMedia) => {
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
  transferred: TransferredMedia,
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

const releaseTransferredDecoder = async (video: HTMLVideoElement, transferred: TransferredMedia) => {
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

export type { DisplayedClockAnchor, TransferredMedia };
export {
  afterNextPaint,
  captureDecodedFrame,
  clearHeldFrame,
  clearTransferredBuffers,
  containsMediaTime,
  discardTransferredMedia,
  displayedProgramTime,
  holdCapturedFrame,
  releaseTransferredDecoder,
  webKitRequiresFreshMSEHandoff,
};
