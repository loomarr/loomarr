import { describe, expect, it } from "vitest";
import { languageName } from "./languages";

describe("languages", () => {
  it("localizes a backend-supplied language code in the interface locale", () => {
    expect(languageName("pl", "en")).toBe("Polish");
    expect(languageName("es", "en")).toBe("Spanish");
  });

  it("keeps an unknown code visible instead of inventing a language name", () => {
    expect(languageName("xyz")).toBe("XYZ");
  });
});
