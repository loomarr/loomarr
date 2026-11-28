import { describe, expect, it } from "vitest";

import { POSTER_TONES, posterMonogram, posterTone } from "../index";

const IDS = Array.from({ length: 40 }, (_, i) => String(90000801 + i));

describe("a poster with no artwork", () => {
  it("keeps one colour per title, from the fixed set", () => {
    for (const id of IDS) {
      expect(posterTone(id)).toBe(posterTone(id));
      expect(POSTER_TONES).toContain(posterTone(id));
    }
  });

  it("spreads neighbouring ids across the set", () => {
    expect(new Set(IDS.map(posterTone)).size).toBe(POSTER_TONES.length);
  });

  it.each([
    ["Upriver", "U"],
    ["Paper Kites", "PK"],
    ["The Glass Pier", "GP"],
    ["The", "T"],
    ["Copper Hill Revisited", "CH"],
    ["1977", "1"],
    ["2 Quiet Orchards", "2Q"],
    ["field_notes.2022", "FN"],
    ["(Untitled) & Co", "UC"],
    ["Élan Vital", "ÉV"],
    ["", ""],
  ])("draws %j as %j", (name, monogram) => {
    expect(posterMonogram(name)).toBe(monogram);
  });
});
