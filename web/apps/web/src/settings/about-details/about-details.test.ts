import { describe, expect, it } from "vitest";
import { aboutDetails, aboutVersion } from "./about-details";

const NOW = Date.parse("2026-07-29T12:00:00Z");
const STARTED = "2026-07-23T07:48:00Z"; // 6d 4h 12m before NOW

const version = {
  version: "v0.9.3",
  commit: "9871fef2ac10",
  builtAt: "2026-07-22T18:04:00Z",
  goVersion: "go1.23.4",
  platform: "linux/amd64",
  startedAt: STARTED,
  backend: "sqlite",
  schemaVersion: 20,
  ready: true,
};

const value = (rows: ReturnType<typeof aboutDetails>, label: string) =>
  rows.find((r) => r.label === label)?.value;

describe("aboutDetails", () => {
  it("lists what someone quotes in a bug report, in order", () => {
    expect(aboutDetails(version, NOW)).toEqual([
      { label: "Version", value: "v0.9.3" },
      { label: "Commit", value: "9871fef2ac10" },
      { label: "Built", value: "2026-07-22T18:04:00Z" },
      { label: "Go runtime", value: "go1.23.4 · linux/amd64" },
      { label: "Uptime", value: "6d 4h 12m" },
      { label: "Database", value: "sqlite · schema 20" },
      { label: "Status", value: "Ready" },
    ]);
  });

  // Right after a restart: "just started", never "0m", which reads like a broken value.
  it("names a fresh start rather than showing zero", () => {
    expect(value(aboutDetails({ ...version, startedAt: "2026-07-29T11:59:40Z" }, NOW), "Uptime")).toBe(
      "just started",
    );
  });

  // ⚠ Rows the server cannot fill are ABSENT, not blank.
  it("omits rows the server did not send", () => {
    expect(aboutDetails({ version: "dev", ready: true }, NOW)).toEqual([
      { label: "Version", value: "dev" },
      { label: "Status", value: "Ready" },
    ]);
  });

  it("keeps a backend whose schema version is unknown", () => {
    expect(value(aboutDetails({ ...version, schemaVersion: undefined }, NOW), "Database")).toBe("sqlite");
  });

  // "v0.9.3" and "v0.9.3 plus local edits" are different things when diagnosing a problem.
  it("marks a build made from a modified working tree", () => {
    expect(aboutVersion({ ...version, dirty: true })).toBe("v0.9.3 (modified)");
    expect(value(aboutDetails({ ...version, dirty: true }, NOW), "Version")).toBe("v0.9.3 (modified)");
  });

  // Not ready reports WHY: the detail is the reason to open the page.
  it("reports why the instance is not ready", () => {
    expect(
      value(
        aboutDetails({ version: "v0.9.3", ready: false, detail: "media server unreachable" }, NOW),
        "Status",
      ),
    ).toBe("media server unreachable");
  });
});
