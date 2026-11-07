import { describe, expect, it } from "vitest";
import { validateSplitReviewSearch } from "./split-review-search";

describe("validateSplitReviewSearch", () => {
  it("keeps the two known origins and drops anything else", () => {
    expect(validateSplitReviewSearch({ from: "incoming" })).toEqual({ from: "incoming" });
    expect(validateSplitReviewSearch({ from: "library" })).toEqual({ from: "library" });
    expect(validateSplitReviewSearch({ from: "/channels" })).toEqual({});
    expect(validateSplitReviewSearch({})).toEqual({});
  });
});
