import type { ClientDiagnosticsReporter } from "@loomarr/core/client-diagnostics";
import type { PlayerErrorReport, PlayerTransportEvent } from "../player-controller";

type NativeDiagnosticsRecorder = Pick<ClientDiagnosticsReporter, "flush" | "record">;

/**
 * The wire contract carries no error prose, so the native message is reduced to a closed cause
 * token; attempt and seconds-since-tune ride in the same errorCode (`cause.aN.tNs`).
 */
const causeOf = (message: string): string => {
  const text = message.toLowerCase();
  if (text.includes("behindlivewindow") || text.includes("behind live window")) return "behind_live_window";
  const status = /\b([45]\d{2})\b/.exec(text)?.[1];
  if (status) return `http_${status}`;
  if (/timeout|timed out|network|connect|unable to|io error/.test(text)) return "network";
  if (text.includes("decod")) return "decoder";
  return "unknown";
};

interface NativePlaybackDiagnostics {
  channelChanged: (channelId: string | undefined) => void;
  dispose: () => void;
  playerError: (report: PlayerErrorReport) => void;
  transportEvent: (event: PlayerTransportEvent) => void;
}

const createNativePlaybackDiagnostics = (
  reporter: NativeDiagnosticsRecorder,
  playbackSessionId: string,
): NativePlaybackDiagnostics => {
  if (!playbackSessionId || playbackSessionId.length > 128) {
    throw new Error("Native playback session identity must contain 1-128 characters.");
  }
  let channelId: string | undefined;
  let disposed = false;

  return {
    channelChanged: (nextChannelId) => {
      if (disposed || !nextChannelId || nextChannelId === channelId) return;
      const previousChannelId = channelId;
      reporter.record(
        previousChannelId
          ? {
              channelId: nextChannelId,
              event: "player.source_replaced",
              playbackSessionId,
              previousChannelId,
              reason: "channel_change",
              transport: "native_hls",
            }
          : {
              channelId: nextChannelId,
              event: "player.attached",
              playbackSessionId,
              reason: "mount",
              transport: "native_hls",
            },
      );
      channelId = nextChannelId;
    },
    dispose: () => {
      if (disposed) return;
      disposed = true;
      if (channelId) {
        reporter.record({
          channelId,
          event: "player.detached",
          playbackSessionId,
          reason: "unmount",
          transport: "native_hls",
        });
        void reporter.flush();
      }
    },
    transportEvent: (event) => {
      if (disposed || !channelId) return;
      if (event.type === "first-frame") {
        reporter.record({
          channelId,
          event: "player.ready",
          playbackSessionId,
          transport: "native_hls",
        });
      }
    },
    playerError: ({ attempt, channelId: erroredChannelId, elapsedMs, error, fatal }) => {
      if (disposed) return;
      reporter.record({
        channelId: erroredChannelId,
        errorCode: `${causeOf(error)}.a${attempt}.t${Math.round(elapsedMs / 1_000)}s`,
        event: "player.media_error",
        fatal,
        playbackSessionId,
        transport: "native_hls",
      });
      // Errors are the events an operator is looking for; do not wait for the batch timer.
      if (fatal) void reporter.flush();
    },
  };
};

export type { NativeDiagnosticsRecorder, NativePlaybackDiagnostics };
export { createNativePlaybackDiagnostics };
