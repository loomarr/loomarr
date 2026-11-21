import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { describeRefusal, nextBuildNumber } from "./asc-next-build-number.mjs";

describe("describeRefusal", () => {
  it("names Apple's reason, so a missing agreement reads differently from a revoked key", () => {
    const body = JSON.stringify({
      errors: [
        {
          code: "FORBIDDEN_ERROR",
          detail: "A required agreement is missing or has expired.",
          status: "403",
          title: "This request is forbidden for security reasons",
        },
      ],
    });
    assert.equal(
      describeRefusal(403, "/v1/apps", body),
      "App Store Connect answered 403 for /v1/apps (FORBIDDEN_ERROR: This request is forbidden for security reasons: A required agreement is missing or has expired.)",
    );
  });

  it("falls back to the status when the body is not Apple's JSON", () => {
    assert.equal(
      describeRefusal(502, "/v1/apps", "<html>Bad gateway</html>"),
      "App Store Connect answered 502 for /v1/apps",
    );
    assert.equal(describeRefusal(403, "/v1/apps", ""), "App Store Connect answered 403 for /v1/apps");
  });

  it("cuts a long detail short", () => {
    const body = JSON.stringify({ errors: [{ code: "X", detail: "d".repeat(500) }] });
    assert.equal(
      describeRefusal(403, "/v1/apps", body).length,
      "App Store Connect answered 403 for /v1/apps (X: )".length + 200,
    );
  });
});

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
