import { describe, expect, it } from "vitest";
import { detailRows } from "./use-playback-details";

const rowsOf = (facts: Parameters<typeof detailRows>[0]) =>
  Object.fromEntries(detailRows(facts, "loomarr.lan").map((r) => [r.label, r.value]));

describe("detailRows", () => {
  it("reads the figures the browser has, in the player's own words", () => {
    const rows = rowsOf({
      lagSeconds: 2.34,
      level: {
        bitrate: 4_600_000,
        codecs: "avc1.640029,mp4a.40.2",
        frameRate: 29.97,
        height: 1080,
        videoRange: "SDR",
      },
      sessionId: "playback_4f2a9c1d77",
      video: { bufferedAheadSeconds: 6.24, droppedFrames: 3, height: 1080, totalFrames: 12_418, width: 1920 },
    });
    expect(rows).toEqual({
      "Behind live": "2.3 s",
      Bitrate: "4.6 Mbps",
      "Buffer ahead": "6.2 s",
      "Dropped frames": "3 of 12,418",
      HDR: "None (SDR)",
      Resolution: "1920×1080 · 29.97 fps",
      Server: "loomarr.lan",
      Session: "4f2a9c1d",
      Video: "avc1.640029,mp4a.40.2",
    });
  });

  it("names HDR plainly and leaves out every row the browser cannot read, never inventing one", () => {
    expect(rowsOf({ lagSeconds: 0, level: { height: 2160, videoRange: "PQ" } }).HDR).toBe("HDR10 (PQ)");
    // Native HLS: no level, no frame counts, no session. Only what is true is left.
    expect(rowsOf({ lagSeconds: 0 })).toEqual({ "Behind live": "0.0 s", Server: "loomarr.lan" });
  });
});
