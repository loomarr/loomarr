import { describe, expect, it } from "vitest";
import { accents, tintSurface } from "../tokens";
import { AA_SMALL, compositeTint, contrast } from "./contrast";

describe("WCAG contrast", () => {
  it("white on the app background is AAA", () => {
    // #FFFFFF on #0B0C0E is ~20:1 — a sanity anchor for the luminance math.
    expect(contrast("#FFFFFF", "#0B0C0E")).toBeGreaterThan(19);
  });

  it("the badge/tint rule holds: every -300 stop clears AA on its composited 15% tint", () => {
    // The exact contract from frontend-design §2.1 — the base stops fail on the
    // 15% wash, the -300 stops pass. This is what the generator gates CI on.
    for (const [name, a] of Object.entries(accents)) {
      if (a.on === a.base) continue;
      const tintBg = compositeTint(a.base, 15, tintSurface);
      expect(contrast(a.on, tintBg), `${name}-300 on 15% tint`).toBeGreaterThanOrEqual(AA_SMALL);
      expect(contrast(a.base, tintBg), `${name} base on 15% tint should fail (why -300 exists)`).toBeLessThan(
        AA_SMALL,
      );
    }
  });

  it("compositeTint blends per-channel toward the surface", () => {
    // 0% → the surface itself; 100% → the pure accent.
    expect(compositeTint("#FFB020", 0, "#131519")).toBe("#131519");
    expect(compositeTint("#FFB020", 100, "#131519")).toBe("#FFB020");
  });
});
