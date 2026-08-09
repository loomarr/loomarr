import { describe, expect, it } from "vitest";
import { languageLabel } from "./language-label";

describe("languageLabel", () => {
  it("resolves a language code to a human name via Intl (not a hardcoded table)", () => {
    // jsdom ships Intl.DisplayNames; "en" is the canonical 2-letter code Intl always knows.
    expect(languageLabel("en")).toBe("English");
  });

  it("falls back to the upper-cased code when Intl can't resolve it", () => {
    // A made-up tag Intl cannot map — better to show the raw code than crash or blank.
    expect(languageLabel("zzx")).toBe("ZZX");
  });

  it("renders 'Unknown' for an untagged (empty) track", () => {
    expect(languageLabel("")).toBe("Unknown");
    expect(languageLabel("   ")).toBe("Unknown");
  });
});
