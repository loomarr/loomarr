/**
 * Certification marks (#1037): one `LoomarrCert ...` line per playback event, written to the platform
 * console so `adb logcat` carries tune, held-frame, first-frame, stall, error and format timing. Lines
 * are `key=value` pairs on one monotonic clock, parsed by scripts/shield-cert/. They carry ids,
 * numbers and closed tokens only: never a URL, a title or an error message.
 */

const MARK_PREFIX = "LoomarrCert";
const MARK_VERSION = 1;

type MarkValue = number | string | undefined;

interface PlaybackTuneMark {
  attemptId: number;
  channelId: string;
  channelNumber: number;
  /** Monotonic time of the remote key that caused this tune, when one did. */
  keyAtMs?: number;
  reason: string;
  /** True when the tune reused a warmed neighbour's source; false when it minted (a cold tune). */
  warm: boolean;
}

interface PlaybackFormatMark {
  bitrate?: number | null;
  frameRate?: number | null;
  height?: number;
  mimeType?: string | null;
  width?: number;
}

interface PlaybackMarks {
  enabled: boolean;
  error: (attemptId: number, cause: string) => void;
  firstFrame: (attemptId: number) => void;
  format: (attemptId: number, format: PlaybackFormatMark | null) => void;
  /** The switch overlay committed its readout ("osd") or loaded the channel's still ("still"). */
  held: (attemptId: number | undefined, what: "osd" | "still") => void;
  /** The clock every mark uses, for callers that stamp a key before the tune it causes. */
  now: () => number;
  stallEnd: (
    attemptId: number,
    durationMs: number,
    why: "error" | "released" | "resumed" | "retuned",
  ) => void;
  stallStart: (attemptId: number) => void;
  tune: (mark: PlaybackTuneMark) => void;
}

interface PlaybackMarksOptions {
  enabled: boolean;
  now?: () => number;
  sink?: (line: string) => void;
}

const monotonicNow = (): number =>
  typeof globalThis.performance?.now === "function" ? globalThis.performance.now() : Date.now();

const token = (value: MarkValue): string | undefined => {
  if (value === undefined) return undefined;
  if (typeof value === "number")
    return Number.isFinite(value) ? String(Math.round(value * 10) / 10) : undefined;
  const cleaned = value.replace(/[^\w./:-]/g, "_");
  return cleaned.length > 0 ? cleaned.slice(0, 64) : undefined;
};

const noop = () => undefined;

const disabledMarks: PlaybackMarks = {
  enabled: false,
  error: noop,
  firstFrame: noop,
  format: noop,
  held: noop,
  now: monotonicNow,
  stallEnd: noop,
  stallStart: noop,
  tune: noop,
};

const createPlaybackMarks = ({
  enabled,
  now = monotonicNow,
  // React Native routes console.info to logcat (tag ReactNativeJS), in release builds too.
  sink = (line) => console.info(line),
}: PlaybackMarksOptions): PlaybackMarks => {
  if (!enabled) return disabledMarks;

  const write = (event: string, fields: Record<string, MarkValue>) => {
    let line = `${MARK_PREFIX} v=${MARK_VERSION} ev=${event} t=${token(now())}`;
    for (const [key, value] of Object.entries(fields)) {
      const text = token(value);
      if (text !== undefined) line += ` ${key}=${text}`;
    }
    try {
      sink(line);
    } catch {
      // Marks must never affect playback.
    }
  };

  return {
    enabled: true,
    error: (attemptId, cause) => write("error", { att: attemptId, cause }),
    firstFrame: (attemptId) => write("first-frame", { att: attemptId }),
    format: (attemptId, format) =>
      write("format", {
        att: attemptId,
        br: format?.bitrate ?? undefined,
        fps: format?.frameRate ?? undefined,
        h: format?.height,
        mime: format?.mimeType ?? (format ? undefined : "none"),
        w: format?.width,
      }),
    held: (attemptId, what) => write("held", { att: attemptId, what }),
    now,
    stallEnd: (attemptId, durationMs, why) => write("stall-end", { att: attemptId, dur: durationMs, why }),
    stallStart: (attemptId) => write("stall-start", { att: attemptId }),
    tune: ({ attemptId, channelId, channelNumber, keyAtMs, reason, warm }) =>
      write("tune", {
        att: attemptId,
        ch: channelId,
        key: keyAtMs,
        num: channelNumber,
        path: warm ? "warm" : "cold",
        why: reason,
      }),
  };
};

export type { PlaybackFormatMark, PlaybackMarks, PlaybackMarksOptions, PlaybackTuneMark };
export { createPlaybackMarks, MARK_PREFIX };
