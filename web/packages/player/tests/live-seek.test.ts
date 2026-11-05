import { liveSeek } from "@loomarr/player/browser";
import { describe, expect, it } from "vitest";

// A channel two hours into its stream, played live: the frame on screen is at the live sync position,
// media 7200 s, and aired at NOW − 8 s (the live latency). The element holds fifteen minutes of
// segments, the newest ending a little past the sync position.
const NOW = Date.UTC(2026, 9, 29, 20, 44);
const at = (seconds: number) => NOW + seconds * 1000;
const live = {
  currentTime: 7200,
  displayedWallMs: at(-8),
  liveEdge: 7200,
  nowMs: NOW,
  seekable: { start: 7208 - 900, end: 7208 },
};

describe("liveSeek", () => {
  it("goes back ten seconds from live, in media time, keeping the wall clock it lands on", () => {
    expect(liveSeek({ ...live, targetWallMs: at(-18) })).toEqual({
      kind: "media",
      mediaTime: 7190,
      wallMs: at(-18),
    });
  });

  it("lands on live when forward reaches the edge, rather than a few seconds behind it", () => {
    const behind = { ...live, currentTime: 7190, displayedWallMs: at(-18) };
    expect(liveSeek({ ...behind, targetWallMs: at(-8) })).toEqual({ kind: "live" });
    // Still short of the edge: a real position.
    expect(liveSeek({ ...behind, targetWallMs: at(-14) })).toMatchObject({ kind: "media", mediaTime: 7194 });
  });

  it("treats an instant at or after now as live", () => {
    expect(liveSeek({ ...live, liveEdge: undefined, seekable: undefined, targetWallMs: at(5) })).toEqual({
      kind: "live",
    });
  });

  it("holds a seek inside the window: never older than the horizon, never before what the element holds", () => {
    // Start over on a programme that began 40 minutes ago: as far back as the window goes, less the margin.
    const old = liveSeek({ ...live, seekable: undefined, targetWallMs: at(-40 * 60) });
    expect(old).toMatchObject({ kind: "media", wallMs: at(-15 * 60 + 5) });

    // The element holds less than the server's window (it joined late): its start is the floor.
    const short = liveSeek({ ...live, seekable: { start: 7100, end: 7208 }, targetWallMs: at(-10 * 60) });
    expect(short).toEqual({ kind: "media", mediaTime: 7100, wallMs: at(-108) });
  });
});
