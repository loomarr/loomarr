import type { TitleDTO } from "@loomarr/api";
import { describe, expect, it } from "vitest";
import { journeyProgress, stageOf } from "./journey";

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

  it("keeps a given-up title in the waiting conversation, not a failure stage", () => {
    // §4: unavailable means it gave up after the TTL. That is something to retry, not a
    // verdict on the channel.
    expect(stageOf(title({ state: "unavailable" }))).toBe("waiting");
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
