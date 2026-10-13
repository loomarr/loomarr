import { describe, expect, it } from "vitest";
import { eraDates, eraOf } from "./era-dates";

describe("era-dates", () => {
  it("writes an era as the same range on every date axis", () => {
    const era = { from: 1990, to: 1999 };
    expect(eraDates(era)).toEqual({ movieRelease: [era], seriesPremiere: [era], seriesAiring: [era] });
  });

  it("treats a range open at both ends as no date scope", () => {
    expect(eraDates({})).toBeUndefined();
    expect(eraDates(undefined)).toBeUndefined();
  });

  it("reads the era back only from era-shaped dates", () => {
    expect(eraOf(eraDates({ from: 1990 }))).toEqual({ from: 1990 });
    expect(eraOf({ movieRelease: [{ from: 1990, to: 1999 }] })).toBeUndefined();
    expect(
      eraOf({
        movieRelease: [{ from: 1990, to: 1999 }],
        seriesPremiere: [{ from: 1990, to: 1999 }],
        seriesAiring: [{ from: 1985, to: 1999 }],
      }),
    ).toBeUndefined();
    expect(eraOf(undefined)).toBeUndefined();
  });
});
