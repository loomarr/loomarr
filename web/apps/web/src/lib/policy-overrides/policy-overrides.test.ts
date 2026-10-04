import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import { describe, expect, it } from "vitest";
import { eraDates } from "@/lib/era-dates";
import { datesMode, isOverridden, overrideCount, POLICY_FIELDS, resetField } from "./policy-overrides";

// Every field set to a real channel value, so each reset has something to clear.
const everything: ChannelPolicy = {
  ordering: "sequential",
  audience: { ceiling: "TV-PG", unrated: "exclude" },
  scope: { dates: eraDates({ from: 1990, to: 1999 }), runtimeMax: 5400, series: ["series:tmdb:1"] },
  separation: { movieNoRepeat: "168h", episodeNoRepeat: "24h", seriesMinGap: "2h", blockMax: 2 },
  seasonal: { mode: "exclusive", holidays: ["christmas"] },
  autoCurate: { minScorePct: 70, maxTitles: 5 },
};

describe("overrideCount", () => {
  it("counts nothing on a fresh channel", () => {
    expect(overrideCount({})).toEqual({ overridden: 0, total: POLICY_FIELDS.length });
  });

  it("counts every defaultable field once", () => {
    expect(overrideCount(everything)).toEqual({ overridden: 11, total: 11 });
  });

  it("reads the inherit sentinels as defaults, not overrides", () => {
    const sentinels: ChannelPolicy = {
      ordering: "",
      audience: { ceiling: "", unrated: "" },
      scope: { runtimeMax: 0 },
      separation: { movieNoRepeat: "0s", blockMax: 0 },
      seasonal: { mode: "auto" },
    };
    expect(overrideCount(sentinels).overridden).toBe(0);
  });

  it("counts era-shaped and per-axis dates as one field", () => {
    expect(overrideCount({ scope: { dates: eraDates({ from: 1990, to: 1999 }) } }).overridden).toBe(1);
    expect(overrideCount({ scope: { dates: { movieRelease: [{ from: 1985, to: 1995 }] } } }).overridden).toBe(
      1,
    );
  });
});

describe("resetField", () => {
  it.each(POLICY_FIELDS)("returns %s to its default", (field) => {
    expect(isOverridden(everything, field)).toBe(true);
    const reset = resetField(everything, field);
    expect(isOverridden(reset, field)).toBe(false);
    // Only that field changed.
    for (const other of POLICY_FIELDS.filter((f) => f !== field))
      expect(isOverridden(reset, other)).toBe(true);
  });

  it("writes the exact sentinel each field's editor writes", () => {
    expect(resetField(everything, "ceiling").audience).toEqual({ ceiling: "", unrated: "exclude" });
    expect(resetField(everything, "ordering").ordering).toBe("");
    // A cleared runtime must send 0: undefined is dropped by omitempty.
    expect(resetField(everything, "runtimeMax").scope?.runtimeMax).toBe(0);
    // Separation clears to undefined: 0 would be a real, different value.
    expect(resetField(everything, "blockMax").separation?.blockMax).toBeUndefined();
    expect(resetField(everything, "movieNoRepeat").separation?.movieNoRepeat).toBeUndefined();
    expect(resetField(everything, "seasonalMode").seasonal).toEqual({ mode: "", holidays: ["christmas"] });
  });

  it("drops the dates key but keeps the rest of the scope", () => {
    const scope = resetField(everything, "dates").scope;
    expect(scope).not.toHaveProperty("dates");
    expect(scope).toEqual({ runtimeMax: 5400, series: ["series:tmdb:1"] });
  });

  it("removes the auto-curate opt-in and its thresholds", () => {
    expect(resetField(everything, "autoCurate")).not.toHaveProperty("autoCurate");
  });
});

describe("datesMode", () => {
  it("uses the Era editor for no dates", () => {
    expect(datesMode(undefined)).toBe("era");
    expect(datesMode({})).toBe("era");
    expect(datesMode({ movieRelease: [] })).toBe("era");
  });

  it("uses the Era editor for era-shaped dates, open-ended included", () => {
    expect(datesMode(eraDates({ from: 1990, to: 1999 }))).toBe("era");
    expect(datesMode(eraDates({ from: 1990 }))).toBe("era");
  });

  it("uses the per-axis editor when the windows differ", () => {
    expect(datesMode({ movieRelease: [{ from: 1985, to: 1995 }] })).toBe("axes");
    const era = { from: 1990, to: 1999 };
    expect(
      datesMode({ movieRelease: [era], seriesPremiere: [era], seriesAiring: [{ from: 1990, to: 2005 }] }),
    ).toBe("axes");
    // Two ranges on every axis is not an era either.
    const two = [
      { from: 1960, to: 1969 },
      { from: 1980, to: 1989 },
    ];
    expect(datesMode({ movieRelease: two, seriesPremiere: two, seriesAiring: two })).toBe("axes");
  });
});
