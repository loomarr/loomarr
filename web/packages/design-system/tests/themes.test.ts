import { describe, expect, it } from "vitest";

import { decadeFill, decadeFillColor, semanticThemes } from "../index";

const channel = (hex: string, offset: number) => Number.parseInt(hex.slice(offset, offset + 2), 16) / 255;
const linear = (value: number) => (value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4);
const luminance = (hex: string) =>
  0.2126 * linear(channel(hex, 1)) + 0.7152 * linear(channel(hex, 3)) + 0.0722 * linear(channel(hex, 5));
const contrast = (foreground: string, background: string) => {
  const foregroundLuminance = luminance(foreground);
  const backgroundLuminance = luminance(background);
  const lighter = Math.max(foregroundLuminance, backgroundLuminance);
  const darker = Math.min(foregroundLuminance, backgroundLuminance);
  return (lighter + 0.05) / (darker + 0.05);
};

describe.each(Object.entries(semanticThemes))("%s theme", (_name, theme) => {
  it("keeps informational text at WCAG AA contrast on ordinary surfaces", () => {
    for (const surface of [theme.surface.canvas, theme.surface.raised]) {
      expect(contrast(theme.content.primary, surface)).toBeGreaterThanOrEqual(4.5);
      expect(contrast(theme.content.secondary, surface)).toBeGreaterThanOrEqual(4.5);
    }
  });

  it("keeps secondary and muted text at WCAG AA contrast wherever those roles are used", () => {
    expect(contrast(theme.content.secondary, theme.surface.focus)).toBeGreaterThanOrEqual(4.5);
    expect(contrast(theme.content.muted, theme.surface.canvas)).toBeGreaterThanOrEqual(4.5);
  });

  // A selected secondary action (#1909 Q1/Q2): the focus fill, a 2px `content.primary` border, and
  // the amber focus border once the remote lands on it. Amber is focus, never selection (#1953).
  it("keeps the selected fill neutral, legible and bordered", () => {
    const fill = theme.surface.focus;
    const channels = [1, 3, 5].map((offset) => Number.parseInt(fill.slice(offset, offset + 2), 16));
    expect(Math.max(...channels) - Math.min(...channels)).toBeLessThanOrEqual(16);
    expect(fill).not.toBe(theme.surface.elevated);

    expect(contrast(theme.content.primary, fill)).toBeGreaterThanOrEqual(4.5);
    for (const surface of [fill, theme.surface.canvas, theme.surface.raised, theme.surface.elevated]) {
      expect(contrast(theme.content.primary, surface)).toBeGreaterThanOrEqual(3);
    }
    expect(contrast(theme.action.focus, fill)).toBeGreaterThanOrEqual(3);
  });

  it("keeps semantic text colors at WCAG AA contrast on the canvas", () => {
    for (const color of [...Object.values(theme.state), theme.action.primary]) {
      expect(contrast(color, theme.surface.canvas)).toBeGreaterThanOrEqual(4.5);
    }
  });

  // The amber (`action.primary`) and magenta (`accent.suggest`) text stops: the brand's #FFB020 is
  // 1.8:1 on white, so the light theme carries its own darker text variants (#2782).
  it("keeps amber and magenta text at WCAG AA contrast on every ordinary surface", () => {
    const magentaTint = "#F9E2F1"; // the suggest badge's 15% brand-magenta wash over white
    for (const surface of [theme.surface.canvas, theme.surface.raised]) {
      expect(contrast(theme.action.primary, surface)).toBeGreaterThanOrEqual(4.5);
      expect(contrast(theme.accent.suggest, surface)).toBeGreaterThanOrEqual(4.5);
    }
    if (theme === semanticThemes.light) {
      expect(contrast(theme.accent.suggest, magentaTint)).toBeGreaterThanOrEqual(4.5);
    }
  });

  it("keeps every semantic badge pairing at WCAG AA contrast", () => {
    for (const state of Object.keys(theme.state) as (keyof typeof theme.state)[]) {
      expect(contrast(theme.state[state], theme.stateSurface[state])).toBeGreaterThanOrEqual(4.5);
    }
  });

  it("keeps control boundaries and focus indicators perceptible", () => {
    for (const surface of [theme.surface.canvas, theme.surface.raised, theme.surface.elevated]) {
      expect(contrast(theme.border.control, surface)).toBeGreaterThanOrEqual(3);
    }
    expect(contrast(theme.action.focus, theme.surface.canvas)).toBeGreaterThanOrEqual(3);
  });

  // A channel number on its decade's fill (#2767): small mono text, so AA for small text.
  it.each(["1970", "1980", "1990"] as const)("keeps a channel number on the %s fill at WCAG AA", (decade) => {
    expect(contrast(theme.decade.ink, theme.decade[decade])).toBeGreaterThanOrEqual(4.5);
  });
});

describe("decadeFill", () => {
  it("names the theme colour for a decade the era palette has, and nothing for any other", () => {
    expect(decadeFill(1970)).toBe("decade1970");
    expect(decadeFill(1980)).toBe("decade1980");
    expect(decadeFill(1990)).toBe("decade1990");
    for (const neutral of [undefined, 1960, 2000, 1985]) expect(decadeFill(neutral)).toBeUndefined();
  });

  it("gives the same colour as a value for chrome drawn alike in both themes", () => {
    expect(decadeFillColor(1980)).toBe(semanticThemes.dark.decade["1980"]);
    expect(decadeFillColor(undefined)).toBeUndefined();
  });
});
