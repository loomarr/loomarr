import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

import { semanticThemes } from "../index";
import {
  AA_SMALL,
  accents,
  compositeTint,
  contrast,
  semanticAliases,
  staticExtras,
  staticScale,
  tintSurface,
  webThemeCss,
  webTokenSet,
} from "../web-theme";

describe("web theme palette", () => {
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

  it("the raw palette is the semantic dark theme's, not a second copy", () => {
    const dark = semanticThemes.dark;
    expect(staticScale["static-950"]).toBe(dark.surface.canvas);
    expect(staticScale["static-100"]).toBe(dark.content.primary);
    expect(staticExtras["border-control"]).toBe(dark.border.control);
    expect(accents.onair.on).toBe(dark.state.live);
    expect(accents.suggest.on).toBe(dark.accent.suggest);
  });

  it("the committed theme.css is what the tokens generate", () => {
    const committed = readFileSync(new URL("../generated/theme.css", import.meta.url), "utf8");
    expect(committed).toBe(webThemeCss());
  });

  it("the token set lists the aliases beside the raw colors", () => {
    const { colors } = webTokenSet();
    expect(colors.primary).toBe(accents.signal.base);
    expect(colors["static-950"]).toBe(staticScale["static-950"]);
  });
});

describe("WCAG contrast", () => {
  it("white on the app background is AAA", () => {
    // #FFFFFF on #0B0C0E is ~20:1 — a sanity anchor for the luminance math.
    expect(contrast("#FFFFFF", "#0B0C0E")).toBeGreaterThan(19);
  });

  it("the badge/tint rule holds: every -300 stop clears AA on its composited 15% tint", () => {
    // The exact contract from frontend-design §2.1 — the base stops fail on the 15% wash, the -300
    // stops pass.
    for (const [name, a] of Object.entries(accents)) {
      if (a.on === a.base) continue;
      const tintBg = compositeTint(a.base, 15, tintSurface);
      expect(contrast(a.on, tintBg), `${name}-300 on 15% tint`).toBeGreaterThanOrEqual(AA_SMALL);
      expect(contrast(a.base, tintBg), `${name} base on 15% tint should fail (why -300 exists)`).toBeLessThan(
        AA_SMALL,
      );
    }
  });

  it("a control boundary clears 3:1 (WCAG 1.4.11) on every surface a control sits on", () => {
    // `--input` is border-control, and controls sit on the canvas, on cards (static-900) and on the
    // muted/accent surfaces (static-800). The last one is where it measured 2.82:1 (#1659).
    for (const surface of ["static-950", "static-900", "static-800"] as const) {
      expect(
        contrast(staticExtras["border-control"], staticScale[surface]),
        `border-control on ${surface}`,
      ).toBeGreaterThanOrEqual(3);
    }
  });

  it("compositeTint blends per-channel toward the surface", () => {
    // 0% → the surface itself; 100% → the pure accent.
    expect(compositeTint("#FFB020", 0, "#131519")).toBe("#131519");
    expect(compositeTint("#FFB020", 100, "#131519")).toBe("#FFB020");
  });
});
