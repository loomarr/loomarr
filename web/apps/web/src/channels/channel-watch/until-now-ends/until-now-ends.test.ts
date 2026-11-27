import { describe, expect, it } from "vitest";
import { untilNowEnds } from "./until-now-ends";

describe("untilNowEnds", () => {
  it("is the time left in the airing on now", () => {
    const airings = [{ stopMs: 1_000 }, { stopMs: 4_000 }, { stopMs: 9_000 }];
    expect(untilNowEnds(airings, 2_500)).toBe(1_500);
  });

  it("finds the soonest end, whatever order a pending placeholder puts it in", () => {
    expect(untilNowEnds([{ stopMs: 9_000 }, { stopMs: 4_000 }], 2_500)).toBe(1_500);
  });

  it("knows no end once everything in the strip has ended, or the server's now is unknown", () => {
    expect(untilNowEnds([{ stopMs: 1_000 }], 2_500)).toBeUndefined();
    expect(untilNowEnds([], 2_500)).toBeUndefined();
    expect(untilNowEnds([{ stopMs: 4_000 }], undefined)).toBeUndefined();
  });
});
