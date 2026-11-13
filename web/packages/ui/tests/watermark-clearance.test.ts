import { describe, expect, it } from "vitest";

import { clearOfWatermark, pictureIn, watermarkOf } from "../index";

type WatermarkCorner = "bottom-left" | "bottom-right" | "top-left" | "top-right";

// The mock's frames (design/watch-player): desktop 1440 × 900, landscape 844 × 390, portrait 390 × 844.
const desktop = { height: 900, width: 1440 };
const landscape = { height: 390, width: 844 };
const portrait = { height: 844, width: 390 };

// The top row hangs from the top edge on the right.
const top = (frame: typeof desktop, corner: WatermarkCorner, side: "left" | "right" = "right") =>
  clearOfWatermark({
    base: 40,
    edge: "top",
    extent: 48,
    frame,
    side,
    watermark: { corner, margin: 0.05, size: 0.06 },
  });

describe("watermarkOf", () => {
  it("is playout's default when the policy says nothing: top-right, 6 % tall, 5 % in", () => {
    expect(watermarkOf(undefined)).toEqual({ corner: "top-right", margin: 0.05, size: 0.06 });
    expect(watermarkOf({})).toEqual({ corner: "top-right", margin: 0.05, size: 0.06 });
  });

  it("reads the channel's own corner, size and margin, and nothing when it is switched off", () => {
    expect(watermarkOf({ corner: "bottom-left", margin: 0.1, size: 0.08 })).toEqual({
      corner: "bottom-left",
      margin: 0.1,
      size: 0.08,
    });
    expect(watermarkOf({ enabled: false })).toBeUndefined();
  });
});

describe("pictureIn", () => {
  it("letterboxes 16:9 inside a taller frame and fills a 16:9 one", () => {
    expect(pictureIn(desktop)).toEqual({ height: 810, width: 1440, x: 0, y: 45 });
    const strip = pictureIn(portrait);
    expect(strip.width).toBe(390);
    expect(strip.y).toBeCloseTo((844 - 219.375) / 2);
    expect(pictureIn(landscape).width).toBeCloseTo(693.33, 1);
  });
});

describe("clearOfWatermark", () => {
  it("starts the top row below a top-right bug it would cover", () => {
    // Desktop: the picture is 810 tall from y = 45; the bug runs 45 + 40.5 = 85.5 to 134.1.
    expect(top(desktop, "top-right")).toBe(152);
    // Landscape: the picture is 390 × 219 inside 844 × 390 pillarbox; the bug is small but in the way.
    expect(top(landscape, "top-right")).toBeGreaterThan(16);
  });

  it("moves nothing when the bug is in another corner, absent, or already clear", () => {
    expect(top(desktop, "bottom-right")).toBe(40);
    expect(
      clearOfWatermark({
        base: 40,
        edge: "top",
        extent: 48,
        frame: desktop,
        side: "right",
        watermark: undefined,
      }),
    ).toBe(40);
    // A portrait phone: the picture is a strip mid-screen, nowhere near the top row.
    expect(top(portrait, "top-right")).toBe(40);
  });

  // #3397: the bug's side counts, not only its edge. A top-left bug used to push the top-right row
  // down to 152 as if it were in its corner.
  it("moves only the chrome on the bug's corner: every corner against each side", () => {
    const moved = (corner: WatermarkCorner, edge: "bottom" | "top", side: "left" | "right") =>
      clearOfWatermark({
        base: 40,
        edge,
        extent: 48,
        frame: desktop,
        side,
        watermark: { corner, margin: 0.05, size: 0.06 },
      }) !== 40;
    const corners: WatermarkCorner[] = ["top-left", "top-right", "bottom-left", "bottom-right"];
    for (const corner of corners)
      for (const edge of ["top", "bottom"] as const)
        for (const side of ["left", "right"] as const)
          expect(moved(corner, edge, side), `${corner} vs ${edge}-${side}`).toBe(
            corner === `${edge}-${side}`,
          );
    // The issue's repro: the top-right row stays at 40 for a top-left bug.
    expect(top(desktop, "top-left")).toBe(40);
    // And the upper-left chrome (the unmute pill) steps below it instead.
    expect(top(desktop, "top-left", "left")).toBe(152);
  });

  it("lifts the band and keys, which span the frame, clear of either bottom bug", () => {
    for (const corner of ["bottom-left", "bottom-right"] as const) {
      const lifted = clearOfWatermark({
        base: 48,
        edge: "bottom",
        extent: 208,
        frame: desktop,
        side: "both",
        watermark: { corner, margin: 0.05, size: 0.06 },
      });
      // The bug spans 85.5 to 134.1 from the bottom edge; the chrome starts a gap past it, on the grid.
      expect(lifted, corner).toBe(152);
    }
    // A short band on a portrait phone: the bug is a third of the way up the screen, nowhere near it.
    expect(
      clearOfWatermark({
        base: 16,
        edge: "bottom",
        extent: 200,
        frame: portrait,
        side: "both",
        watermark: { corner: "bottom-left", margin: 0.05, size: 0.06 },
      }),
    ).toBe(16);
  });

  it("never overlaps: the chrome that moved starts at least a gap past the bug's far edge", () => {
    for (const frame of [desktop, landscape]) {
      for (const corner of ["top-right", "top-left"] as const) {
        const side = corner === "top-right" ? "right" : "left";
        const at = top(frame, corner, side);
        const p = pictureIn(frame);
        const far = p.y + (0.05 + 0.06) * p.height;
        expect(at === 40 ? 40 + 48 + 16 <= p.y + 0.05 * p.height : at >= far + 16).toBe(true);
      }
    }
  });
});
