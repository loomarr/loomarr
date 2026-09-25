import type { TitleDTO } from "@loomarr/api";
import { describe, expect, it } from "vitest";
import { isInFlight, journeyProgress, stageOf } from "./journey";

const title = (over: Partial<TitleDTO> = {}): TitleDTO => ({
  key: "movie:tmdb:603",
  mediaType: "movie",
  state: "wanted",
  ...over,
});

describe("stageOf", () => {
  it("collapses the five provisioning states into what a member asks", () => {
    expect(stageOf(title({ state: "wanted" }))).toBe("waiting");
    expect(stageOf(title({ state: "requested" }))).toBe("acquiring");
    expect(stageOf(title({ state: "downloading" }))).toBe("acquiring");
    expect(stageOf(title({ state: "available" }))).toBe("ready");
  });

  it("gives a given-up title its own stage, not the queued one", () => {
    // §4: unavailable means the reconciler gave up after the TTL. Nothing is going to request it
    // next, so calling it "waiting" ("Approved and queued") told the operator something false
    // (#1404).
    expect(stageOf(title({ state: "unavailable" }))).toBe("attention");
  });
});

describe("isInFlight", () => {
  it("is true only for titles still moving towards available", () => {
    expect(isInFlight(title({ state: "wanted" }))).toBe(true);
    expect(isInFlight(title({ state: "requested" }))).toBe(true);
    expect(isInFlight(title({ state: "downloading" }))).toBe(true);
    expect(isInFlight(title({ state: "available" }))).toBe(false);
    expect(isInFlight(title({ state: "unavailable" }))).toBe(false);
  });
});

describe("journeyProgress", () => {
  it("counts what has landed out of everything asked for", () => {
    const titles = [
      title({ state: "available" }),
      title({ state: "downloading" }),
      title({ state: "wanted" }),
    ];
    expect(journeyProgress(titles)).toEqual({ ready: 1, total: 3 });
  });

  it("keeps a given-up title in the denominator so the fraction cannot lie", () => {
    // Dropping it would silently shrink what was requested and read as better progress.
    const titles = [title({ state: "available" }), title({ state: "unavailable" })];
    expect(journeyProgress(titles)).toEqual({ ready: 1, total: 2 });
  });
});
