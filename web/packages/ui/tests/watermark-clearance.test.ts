import { describe, expect, it } from "vitest";

import { clearOfWatermark, pictureIn, watermarkOf } from "../index";

// The mock's frames (design/watch-player): desktop 1440 × 900, landscape 844 × 390, portrait 390 × 844.
const desktop = { height: 900, width: 1440 };
const landscape = { height: 390, width: 844 };
const portrait = { height: 844, width: 390 };

const top = (frame: typeof desktop, corner: "top-right" | "bottom-right") =>
  clearOfWatermark({
    base: 40,
    edge: "top",
    extent: 48,
    frame,
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
      clearOfWatermark({ base: 40, edge: "top", extent: 48, frame: desktop, watermark: undefined }),
    ).toBe(40);
    // A portrait phone: the picture is a strip mid-screen, nowhere near the top row.
    expect(top(portrait, "top-right")).toBe(40);
  });

  it("lifts the band and keys clear of a bottom bug", () => {
    const lifted = clearOfWatermark({
      base: 48,
      edge: "bottom",
      extent: 208,
      frame: desktop,
      watermark: { corner: "bottom-left", margin: 0.05, size: 0.06 },
    });
    // The bug spans 85.5 to 134.1 from the bottom edge; the chrome starts a gap past it, on the grid.
    expect(lifted).toBe(152);
    // A short band on a portrait phone: the bug is a third of the way up the screen, nowhere near it.
    expect(
      clearOfWatermark({
        base: 16,
        edge: "bottom",
        extent: 200,
        frame: portrait,
        watermark: { corner: "bottom-left", margin: 0.05, size: 0.06 },
      }),
    ).toBe(16);
  });

  it("never overlaps: the chrome that moved starts at least a gap past the bug's far edge", () => {
    for (const frame of [desktop, landscape]) {
      for (const corner of ["top-right", "top-left"] as const) {
        const at = clearOfWatermark({
          base: 40,
          edge: "top",
          extent: 48,
          frame,
          watermark: { corner, margin: 0.05, size: 0.06 },
        });
        const p = pictureIn(frame);
        const far = p.y + (0.05 + 0.06) * p.height;
        expect(at === 40 ? 40 + 48 + 16 <= p.y + 0.05 * p.height : at >= far + 16).toBe(true);
      }
    }
  });
});
