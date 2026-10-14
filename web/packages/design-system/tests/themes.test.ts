import { describe, expect, it } from "vitest";

import { semanticThemes } from "../index";

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
});
