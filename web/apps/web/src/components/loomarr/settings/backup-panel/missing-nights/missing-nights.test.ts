import { describe, expect, it } from "vitest";
import { missingNights } from "./missing-nights";

// Noon local time, so a timezone offset can never push a backup onto a neighbouring day.
const at = (month: number, day: number, hour = 12) => new Date(2026, month - 1, day, hour).getTime() / 1000;
const entries = (...times: number[]) => times.map((writtenAt) => ({ writtenAt }));

describe("missingNights", () => {
  it("counts the nights between the oldest and newest backup that have none", () => {
    // The review's case: Nov 7, 5, 3 and 2 on disk, so Nov 6 and Nov 4 are missing.
    expect(missingNights(entries(at(11, 7), at(11, 5), at(11, 3), at(11, 2)))).toBe(2);
  });

  it("is zero for an unbroken series", () => {
    expect(missingNights(entries(at(11, 7), at(11, 6), at(11, 5)))).toBe(0);
  });

  it("counts a manual backup beside the nightly as one night", () => {
    expect(missingNights(entries(at(11, 7, 18), at(11, 7, 3), at(11, 6)))).toBe(0);
  });

  it("does not guess about a series of fewer than two nights", () => {
    expect(missingNights([])).toBe(0);
    expect(missingNights(entries(at(11, 7)))).toBe(0);
  });
});
