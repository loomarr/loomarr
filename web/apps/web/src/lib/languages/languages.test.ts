import { describe, expect, it } from "vitest";
import { languageList, languageName } from "./languages";

describe("languages", () => {
  it("localizes a backend-supplied language code in the interface locale", () => {
    expect(languageName("pl", "en")).toBe("Polish");
    expect(languageName("es", "en")).toBe("Spanish");
  });

  it("keeps an unknown code visible instead of inventing a language name", () => {
    expect(languageName("xyz")).toBe("XYZ");
  });

  it("reads a set of languages as a list in the interface locale", () => {
    expect(languageList(["en"], "en")).toBe("English");
    expect(languageList(["en", "es"], "en")).toBe("English and Spanish");
    expect(languageList(["en", "es", "pl"], "en")).toBe("English, Spanish, and Polish");
  });
});
