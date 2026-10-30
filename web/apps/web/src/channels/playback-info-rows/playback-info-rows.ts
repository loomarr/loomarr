import type { ChannelHealth } from "@loomarr/api/models/channelHealth";

// Playback info's admin rows (#2088 decision 13), as `[label, value]`: the server's own words about
// the channel's encode, from the playout status every admin surface reads, and the session the
// client diagnostics report under. The Stream rows are the player's own (`streamRows`).

type Row = readonly [string, string];

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

export { adminRows };
