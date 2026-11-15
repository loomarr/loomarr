import type Hls from "hls.js";
import type { TransportHost } from "../browser-transport.type";

interface PlaybackDiagnosticsOptions {
  attemptId: number;
  el: HTMLVideoElement;
  /** The hls.js class driving this tune, or undefined when the platform's native HLS plays it. */
  hlsController: typeof Hls | undefined;
  host: TransportHost;
}

/**
 * Records the element's buffering and error events for one tune, and turns a stall into the
 * controller's `stalled` and `playing` events. Attaching records `player.attached`; `stop` removes
 * the listeners and records `player.detached`.
 */
const attachPlaybackDiagnostics = ({ attemptId, el, hlsController, host }: PlaybackDiagnosticsOptions) => {
  const { clock, flags, recordDiagnostic } = host;
  const diagnosticBase = () =>
    ({
      playbackSessionId: host.playbackSessionId,
      channelId: host.channelId() ?? "",
      transport: hlsController?.isSupported() ? "hls_js" : "native_hls",
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
      viewerTimeMs: Math.round(clock.state(host.position()).viewerTimeMs),
      ...(event.startsWith("player.buffering") ? { bufferedMs: bufferedMs() } : {}),
    });
  const framed = () => flags.framedAttemptId === attemptId;
  const onWaiting = () => {
    reportPosition("player.buffering_started");
    // Buffering before the first frame is the start itself; one the viewer paused for is not a stall.
    if (!framed() || flags.stalled || clock.mode() === "paused") return;
    flags.stalled = true;
    clock.beginStall();
    host.emit({ attemptId, type: "stalled" });
  };
  const onPlaying = () => {
    reportPosition("player.buffering_ended");
    if (flags.stalled) {
      flags.stalled = false;
      clock.endStall("resumed");
    }
    // Until the first decoded frame the controller keeps its tuning picture: `first-frame` is the
    // start's own signal, so a `playing` that beats it is not announced.
    if (!framed()) return;
    host.rejoinAfterStall();
    host.emit({ attemptId, type: "playing" });
    host.emitLive();
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
  const stop = (reason: "unmount" | "retry" = "unmount") => {
    el.removeEventListener("waiting", onWaiting);
    el.removeEventListener("playing", onPlaying);
    el.removeEventListener("error", onNativeError);
    recordDiagnostic({ ...diagnosticBase(), event: "player.detached", reason });
  };

  return { diagnosticBase, stop };
};

export type { PlaybackDiagnosticsOptions };
export { attachPlaybackDiagnostics };
