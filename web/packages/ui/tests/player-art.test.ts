import { describe, expect, it } from "vitest";

import { logoSize, rungFor } from "../index";

const SET =
  "/v1/images/h?w=300&f=webp 300w, /v1/images/h?w=780&f=webp 780w, /v1/images/h?w=1280&f=webp 1280w, /v1/images/h?w=1920&f=webp 1920w";
const art = { src: "/v1/images/h?w=780&f=jpeg", srcSetWebp: SET };

describe("rungFor", () => {
  it("takes the smallest rung that fills the rendered width at the screen's pixel ratio", () => {
    expect(rungFor(art, 390, 1)).toBe("/v1/images/h?w=780&f=webp");
    expect(rungFor(art, 390, 3)).toBe("/v1/images/h?w=1280&f=webp");
    expect(rungFor(art, 1440, 2)).toBe("/v1/images/h?w=1920&f=webp".replace("1920", "1920"));
  });

  it("never asks for a phone's small rung to be a 4K one", () => {
    expect(rungFor(art, 200, 2)).toBe("/v1/images/h?w=780&f=webp");
    expect(rungFor(art, 100, 1)).toBe("/v1/images/h?w=300&f=webp");
  });

  it("takes the widest rung there is when the screen wants more than the source has", () => {
    expect(rungFor({ ...art, srcSetWebp: SET.split(", ").slice(0, 2).join(", ") }, 1440, 2)).toBe(
      "/v1/images/h?w=780&f=webp",
    );
  });

  it("is the plain src when the image has no srcset", () => {
    expect(rungFor({ src: "/v1/images/h", srcSetWebp: "" }, 500, 2)).toBe("/v1/images/h");
  });
});

describe("logoSize", () => {
  const area = (a: { height: number; width: number }) => a.width * a.height;

  it("gives a wide wordmark and a nearly square logo a similar visual weight", () => {
    const wide = logoSize({ height: 100, width: 600 }, "pointer");
    const square = logoSize({ height: 400, width: 420 }, "pointer");
    expect(area(wide) / area(square)).toBeGreaterThan(0.8);
    expect(area(wide) / area(square)).toBeLessThan(1.25);
    // A fixed height would have drawn the square logo about a sixth the width of the wide one.
    expect(square.width).toBeGreaterThan(wide.width / 3);
  });

  it("keeps any shape inside the box, shrinking an extreme one rather than cropping it", () => {
    const banner = logoSize({ height: 50, width: 1000 }, "pointer");
    expect(banner.width).toBeLessThanOrEqual(280);
    expect(banner.height).toBeLessThanOrEqual(88);
    const tall = logoSize({ height: 1000, width: 100 }, "touch");
    expect(tall.height).toBeLessThanOrEqual(56);
    expect(tall.width).toBeLessThanOrEqual(200);
  });

  it("is smaller on a phone than on a desktop, and square for an image of no shape", () => {
    expect(area(logoSize({ height: 100, width: 100 }, "touch"))).toBeLessThan(
      area(logoSize({ height: 100, width: 100 }, "pointer")),
    );
    expect(logoSize({ height: 0, width: 0 }, "pointer").width).toBe(
      logoSize({ height: 0, width: 0 }, "pointer").height,
    );
  });
});
