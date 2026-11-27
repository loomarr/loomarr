import { describe, expect, it } from "vitest";
import { monogramOf } from "../src/guide/channel-ident";

describe("monogramOf", () => {
  it("takes the initials of the first two significant words", () => {
    expect(monogramOf("1980s Action Heroes")).toBe("AH");
    expect(monogramOf("Saturday Cartoons")).toBe("SC");
  });

  // A leading number reads badly as an initial: "90s Action" is "AC", not "9A".
  it("skips a leading numeric word", () => {
    expect(monogramOf("90s Action Heroes")).toBe("AH");
  });

  it("falls back to the first two letters of a single word", () => {
    expect(monogramOf("Cartoons")).toBe("CA");
  });

  // A name with nothing usable still gets a mark: a blank square reads as a failed image
  // load rather than as a channel without a logo.
  it("always produces something", () => {
    expect(monogramOf("")).toBe("CH");
    expect(monogramOf("!!! ???")).toBe("CH");
  });
});
