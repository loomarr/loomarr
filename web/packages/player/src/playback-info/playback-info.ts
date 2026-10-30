import { useEffect, useState } from "react";

// Playback info's Stream rows (#2088 decision 13), as `[label, value]`: what this player is playing,
// read the same on every device. The server's side of the encode is an admin surface's own.

// How often the Stream rows read the player again while the sheet is open: the buffer moves.
const SAMPLE_MS = 1_000;

type Row = readonly [string, string];

interface LivePlaybackFacts {
  /** The playing rung's declared bandwidth, bit/s; undefined where the player doesn't say. */
  bitrate?: number;
  bufferMs: number;
  /**
   * The playing rung's codecs, RFC 6381 style (`avc1.640028,mp4a.40.2`); a native player knows only
   * the video's family (`avc1`). Undefined where the player doesn't say.
   */
  codecs?: string;
  frameRate?: number;
  height: number;
  /** The playing rung's place in the master, from 1; undefined where the platform picks for itself. */
  rung?: { number: number; of: number };
  /** The signed stream URL this player was given, as minted (often relative). */
  url: string;
  width: number;
}

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

/** A rung's name: `1080p`, `4K`. */
const rungLabel = (height: number) => (height >= 2160 ? "4K" : `${height}p`);

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
}

const streamRows = ({ auto, facts, lagSeconds }: StreamRowsArgs): Row[] => {
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

/** The player's facts, read again each second while the caller (an open sheet) is mounted. */
const useSampledPlaybackFacts = (read: () => LivePlaybackFacts | undefined) => {
  const [facts, setFacts] = useState(read);
  useEffect(() => {
    setFacts(read());
    const timer = setInterval(() => setFacts(read()), SAMPLE_MS);
    return () => clearInterval(timer);
  }, [read]);
  return facts;
};

export type { LivePlaybackFacts, StreamRowsArgs };
export { describeCodecs, rungLabel, streamRows, useSampledPlaybackFacts };
