import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { nextBuildNumber } from "./asc-next-build-number.mjs";

describe("nextBuildNumber", () => {
  it("goes one past App Store Connect's highest build when the run number is behind", () => {
    // 2026-10-13: CI's run number was 5 while a Mac had already uploaded 5 and 6.
    assert.equal(nextBuildNumber(5, ["4", "6", "5"]), 7);
  });

  it("keeps the run number when it is already ahead", () => {
    assert.equal(nextBuildNumber(9, ["4", "5"]), 9);
  });

  it("compares numbers, not strings, and ignores versions that are not whole numbers", () => {
    assert.equal(nextBuildNumber(1, ["9", "10", "1.2", "", "007"]), 11);
  });

  it("starts at the floor for an app with no builds", () => {
    assert.equal(nextBuildNumber(1, []), 1);
  });
});
