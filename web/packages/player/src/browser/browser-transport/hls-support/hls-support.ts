import type Hls from "hls.js";
import { parseStartFailureReason } from "../../../start-failure";
import { liveHlsConfig } from "../../live-hls-config";
import type { LiveLevel } from "../../live-level";

interface ManifestLoadFailure {
  details?: string;
  response?: { code?: number; data?: unknown };
}

const createHlsController = (HlsController: typeof Hls): Hls => new HlsController({ ...liveHlsConfig });

// What MediaSource can decode: the gate on the premium rung.
const canPlayMime = (mime: string) => typeof MediaSource !== "undefined" && MediaSource.isTypeSupported(mime);

const levelsOf = (hls: Hls): LiveLevel[] =>
  hls.levels.map((l) => ({
    bitrate: l.bitrate,
    codecs: l.attrs.CODECS,
    frameRate: Number.parseFloat(l.attrs["FRAME-RATE"] ?? "") || undefined,
    height: l.height,
    videoRange: l.videoRange,
  }));

// The level on air as the ladder carries it: its height for Settings, the whole level for Playback details.
const onAirOf = (levels: readonly LiveLevel[], index: number) => {
  const level = levels[index];
  return level ? { height: level.height, onAir: level } : {};
};

/**
 * The reason a channel's playlist request was refused, when the server decided it (a 5xx problem
 * response). A 404 or an empty playlist is the ordinary warm-up race and stays on hls.js's own
 * retry; see `parseStartFailureReason` for how the problem body itself is read.
 */
const manifestStartFailure = (
  data: ManifestLoadFailure,
  manifestLoadError: string,
): { reason: string; status: number } | undefined => {
  const code = data.response?.code;
  if (data.details !== manifestLoadError || code === undefined) return undefined;
  const reason = parseStartFailureReason(code, data.response?.data);
  return reason === undefined ? undefined : { reason, status: code };
};

export type { ManifestLoadFailure };
export { canPlayMime, createHlsController, levelsOf, manifestStartFailure, onAirOf };
