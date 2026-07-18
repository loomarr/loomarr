import { describe, expect, it } from "vitest";
import { accents, semanticAliases, staticExtras, staticScale } from "./tokens";

describe("tokens palette", () => {
  it("every color value is a valid #RRGGBB hex", () => {
    const hexes = [
      ...Object.values(staticScale),
      ...Object.values(staticExtras),
      ...Object.values(accents).flatMap((a) => [a.base, a.on]),
    ];
    for (const h of hexes) expect(h, h).toMatch(/^#[0-9A-F]{6}$/i);
  });

  it("every semantic alias maps to a real accent (§2.1)", () => {
    for (const accent of Object.values(semanticAliases)) {
      expect(accents, `alias → ${accent}`).toHaveProperty(accent);
    }
  });
});
