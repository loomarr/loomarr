import { describe, expect, it } from "vitest";

import { CHANNEL_IDENT_WASH_PERCENT, channelIdentColors, semanticThemes } from "../index";
import { AA_SMALL, accents, compositeTint, contrast, onTintVar } from "../web-theme";

// The monogram ident (#3011): the letters sit on a 14% wash of their hue over whatever surface the
// tile is on. The guide rail, the surf rail, Home's channel row and the channel header put it on the
// canvas or a raised card. The gallery's axe sweep measures it on every story, so a miss here would
// break every PR's baseline render (#2953).
describe.each(["dark", "light"] as const)("%s theme monogram ident", (mode) => {
  const theme = semanticThemes[mode];
  const surfaces = { canvas: theme.surface.canvas, raised: theme.surface.raised };

  it.each(Object.entries(channelIdentColors(mode)))(
    "%s letters clear AA on their own wash",
    (_hue, colors) => {
      for (const [name, surface] of Object.entries(surfaces)) {
        const wash = compositeTint(colors.wash, CHANNEL_IDENT_WASH_PERCENT, surface);
        expect(contrast(colors.text, wash), `on ${name}`).toBeGreaterThanOrEqual(AA_SMALL);
      }
    },
  );
});

describe("web monogram ident", () => {
  // The web draws the letters with CSS variables (dark only): they must be the same stops the
  // contrast test above proves.
  it("draws each hue's letters in its on-tint stop", () => {
    for (const [hue, colors] of Object.entries(channelIdentColors("dark"))) {
      const accent = hue as keyof typeof accents;
      expect(accents[accent].on, hue).toBe(colors.text);
      expect(accents[accent].base, hue).toBe(colors.wash);
    }
    expect(onTintVar("suggest")).toBe("var(--color-suggest-300)");
    expect(onTintVar("onair")).toBe("var(--color-onair-300)");
    expect(onTintVar("tune")).toBe("var(--color-tune)");
  });
});
