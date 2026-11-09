import { describe, expect, it } from "vitest";
import { serverVersionLabel } from "./server-version";

describe("serverVersionLabel (#2751)", () => {
  it("shows a release by its tag", () => {
    expect(serverVersionLabel({ version: "v0.2.0", commit: "09d0779abc" })).toBe("v0.2.0");
  });
  it("shows an unstamped build as dev and its short commit", () => {
    expect(serverVersionLabel({ version: "dev", commit: "09d0779abcdef" })).toBe("dev · 09d0779");
  });
  it("says dev alone when no commit is known", () => {
    expect(serverVersionLabel({ version: "dev" })).toBe("dev");
  });
  it("marks a modified tree", () => {
    expect(serverVersionLabel({ version: "v0.2.0", dirty: true })).toBe("v0.2.0 (modified)");
  });
});
