import { describe, expect, it } from "vitest";

import { problemEntries } from "../src/library/library-needs/problem-entries";

const worded = (extraCopies: number, brokenCopies?: number) => {
  const [entry] = problemEntries([
    {
      id: "1",
      kind: "movie",
      name: "Coral Meridian",
      unplayable: 0,
      extraCopies,
      extraBytes: 58_400_000,
      brokenCopies,
    },
  ]);
  const [{ headline, reason } = { headline: "", reason: "" }] = entry?.reasons ?? [];
  // The size has its own formatter; only the words are under test.
  return [headline, reason.replace(/taking .+? and never/, "taking SIZE and never")];
};

describe("Needs you's words for a title's copies", () => {
  it("counts the broken and the extra copies apart", () => {
    expect(worded(2, 1)).toEqual([
      "Coral Meridian has 1 broken and 1 extra copy",
      "1 broken and 1 extra copy are taking SIZE and never play. Removing them gets the space back.",
    ]);
    expect(worded(3, 1)[0]).toBe("Coral Meridian has 1 broken and 2 extra copies");
  });

  it("calls broken copies broken", () => {
    expect(worded(1, 1)).toEqual([
      "Coral Meridian has 1 broken copy",
      "1 broken copy is taking SIZE and never plays. Removing it gets the space back.",
    ]);
    expect(worded(2, 2)[0]).toBe("Coral Meridian has 2 broken copies");
  });

  it("calls them extra where none is known to be broken", () => {
    expect(worded(2)[0]).toBe("Coral Meridian has 2 extra copies");
    expect(worded(1, 0)).toEqual([
      "Coral Meridian has an extra copy",
      "The extra copy is taking SIZE and never plays. Removing it gets the space back.",
    ]);
  });
});
