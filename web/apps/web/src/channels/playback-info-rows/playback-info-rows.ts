import type { ChannelHealth } from "@loomarr/api";
import type { LivePlaybackFacts } from "@loomarr/player/browser";

// Playback info's rows (#2088 decision 13), as `[label, value]`. The Stream rows are this browser's
// facts; the admin rows are the server's own words about the channel's encode, from the playout
// status every admin surface reads, and the session the client diagnostics report under.

type Row = readonly [string, string];

const H264_PROFILES: Record<string, string> = { "42": "Baseline", "4D": "Main", "64": "High" };
const AUDIO_CODECS: Record<string, string> = {
  "ac-3": "AC-3",
  "ec-3": "E-AC-3",
  "mp4a.40.2": "AAC-LC",
  "mp4a.40.5": "HE-AAC",
  "mp4a.40.29": "HE-AACv2",
};

// One RFC 6381 codec, read as a person names it: `avc1.640028` → `H.264 High`.
const describeCodec = (codec: string): string => {
  const [family, profile] = codec.split(".");
  if (family === "avc1" || family === "avc3") {
    const name = H264_PROFILES[(profile ?? "").slice(0, 2).toUpperCase()];
    return name ? `H.264 ${name}` : "H.264";
  }
  if (family === "hvc1" || family === "hev1") return profile === "2" ? "HEVC Main 10" : "HEVC Main";
  return AUDIO_CODECS[codec.toLowerCase()] ?? codec;
};

const describeCodecs = (codecs: string) =>
  codecs
    .split(",")
    .map((c) => describeCodec(c.trim()))
    .join(" · ");

const bitrateLabel = (bitsPerSecond: number) =>
  bitsPerSecond >= 1_000_000
    ? `${(bitsPerSecond / 1_000_000).toFixed(1)} Mbps`
    : `${Math.round(bitsPerSecond / 1_000)} kbps`;

const lagLabel = (seconds: number) =>
  seconds < 60 ? `${seconds} s` : `${Math.floor(seconds / 60)}m ${String(seconds % 60).padStart(2, "0")}s`;

interface StreamRowsArgs {
  /** Whether the viewer's quality is Auto, which the Quality row says first. */
  auto: boolean;
  /** Undefined before the player has bound a source: only how far behind live is known. */
  facts?: LivePlaybackFacts;
  lagSeconds: number;
  /** The rung's name: `1080p`, `4K`. */
  rungLabel: (height: number) => string;
}

const streamRows = ({ auto, facts, lagSeconds, rungLabel }: StreamRowsArgs): Row[] => {
  if (!facts) return [["Behind live", lagLabel(lagSeconds)]];
  const rung = facts.rung ? ` (rung ${facts.rung.number} of ${facts.rung.of})` : "";
  const resolution = facts.width && facts.height ? `${facts.width}×${facts.height}` : undefined;
  const frameRate = facts.frameRate ? ` @ ${Number(facts.frameRate.toFixed(2))}` : "";
  return [
    ...(facts.height
      ? ([["Quality", `${auto ? "Auto · " : ""}${rungLabel(facts.height)}${rung}`]] as const)
      : []),
    ...(resolution ? ([["Resolution", `${resolution}${frameRate}`]] as const) : []),
    ...(facts.codecs ? ([["Codec", describeCodecs(facts.codecs)]] as const) : []),
    ...(facts.bitrate ? ([["Bitrate", bitrateLabel(facts.bitrate)]] as const) : []),
    ["Buffer", `${(facts.bufferMs / 1_000).toFixed(1)} s`],
    ["Behind live", lagLabel(lagSeconds)],
  ];
};

// The server's side: how the channel is encoded for browsers, and how well. A channel the server
// isn't encoding for browsers just now (a warm grace ended, a status not read yet) has only the
// session.
const adminRows = (health: ChannelHealth | undefined, sessionId: string): Row[] => [
  ...(health
    ? ([
        [
          "Encoder",
          health.mode === "direct-play"
            ? "copy · direct play"
            : `${health.encoder} · ${health.hardware ? "hardware" : "software"}`,
        ],
        ["Transcode", health.reason],
        ["Health", health.health],
      ] as const)
    : []),
  ["Session", sessionId],
];

export type { StreamRowsArgs };
export { adminRows, describeCodecs, streamRows };
